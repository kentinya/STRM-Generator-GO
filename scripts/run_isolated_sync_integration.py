#!/usr/bin/env python3
"""Isolated end-to-end acceptance driver for the built strmgen CLI.

The driver starts its own synthetic OpenList service on a dynamic port and
drives the compiled binary as a subprocess. It asserts on real exit codes,
STRM bytes, SQLite rows, filesystem snapshots and stderr, never on Go
internals. It never contacts the shared 15244 fixture, production tokens or
production mounts.

Usage:
    python3 scripts/run_isolated_sync_integration.py \
        --binary artifacts/task-1.1/bin/strmgen-amd64 \
        --scenario all \
        --output-dir artifacts/validation-evidence/task-1.1-final
"""
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import resource
import signal
import sqlite3
import stat
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from mock_openlist import TOKEN, load_fixture, make_server  # noqa: E402

DEFAULT_PUBLIC = "https://openlist.example.invalid"
# A valid version-1 STRM that is not owned by the generator. Used to prove that
# content validity alone never lets an interrupted recovery reclaim a foreign
# target.
FOREIGN_VALID_STRM = (b"# strm-probe-version:1\n# strm-probe-size:999\n"
                      b"https://external.example.invalid/user-owned.mkv\n")
ALL_SCENARIOS = [
    "first_generation",
    "repeat_and_compare",
    "ownership_and_paths",
    "api_and_limits",
    "dry_run_boundaries",
    "multiprocess",
    "source_identity",
    "database_uri",
    "fault_recovery",
]

# Every Harness registers here so a failing scenario cannot leak a locked or
# stopped child process into the next scenario.
_LIVE_HARNESSES = []


class Failure(AssertionError):
    pass


def check(condition, message):
    if not condition:
        raise Failure(message)


def wait_for_marker(proc, marker, timeout=60):
    """Wait until the child writes its deterministic barrier marker."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if marker.exists():
            return True
        if proc.poll() is not None:
            return False
        time.sleep(0.005)
    return marker.exists()


def update_staging_names(root):
    """Return the hidden update-staging entries in one output directory."""
    return sorted(path.name for path in Path(root).iterdir()
                  if path.name.startswith(".strmgen-") and path.name.endswith(".update.tmp"))


def base_directories():
    return copy.deepcopy(load_fixture()["directories"])


class Harness:
    """One scenario's isolated filesystem, service and process space."""

    def __init__(self, binary, workdir, fault_binary=None):
        self.binary = binary
        self.fault_binary = fault_binary
        self.workdir = workdir
        self.workdir.mkdir(parents=True, exist_ok=True)
        self.processes = []
        _LIVE_HARNESSES.append(self)

    def server(self, directories=None, hooks=None):
        fixture = {"directories": directories if directories is not None else base_directories()}
        server = make_server(port=0, fixture=fixture, hooks=hooks)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self._server = server
        self._thread = thread
        return f"http://127.0.0.1:{server.server_port}", fixture["directories"]

    def stop_server(self):
        if getattr(self, "_server", None) is not None:
            self._server.shutdown()
            self._server.server_close()
            self._thread.join(timeout=5)
            self._server = None

    def config(self, name, api_url, sources, db, public=DEFAULT_PUBLIC, workers=4, cleanup=False):
        lines = [
            "openlist:",
            f'  api_url: "{api_url}"',
            f'  public_url: "{public}"',
            '  token_env: "OPENLIST_TOKEN"',
            "sources:",
        ]
        for remote, output in sources:
            lines.append(f'  - remote: "{remote}"')
            lines.append(f'    output: "{output}"')
        lines.append("sync:")
        lines.append(f"  directory_workers: {workers}")
        lines.append("cleanup:")
        lines.append(f"  enabled: {'true' if cleanup else 'false'}")
        lines.append("database:")
        lines.append(f'  path: "{db}"')
        path = self.workdir / f"{name}.yaml"
        path.write_text("\n".join(lines) + "\n")
        return path

    def run(self, config, dry_run=False, timeout=60, extra_env=None, capture=True):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = TOKEN
        if extra_env:
            env.update(extra_env)
        args = [str(self.binary), "sync", "--config", str(config)]
        if dry_run:
            args.append("--dry-run")
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout, env=env)

    def start(self, config, dry_run=False, extra_env=None, ignore_sigxfsz=False, binary=None):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = TOKEN
        if extra_env:
            env.update(extra_env)
        args = [str(binary or self.binary), "sync", "--config", str(config)]
        if dry_run:
            args.append("--dry-run")
        if ignore_sigxfsz:
            # exec preserves SIG_IGN. The integration process can then lower
            # the child RLIMIT_FSIZE after DB initialization and exercise a
            # real temporary-file write error without changing the host.
            wrapper = ("import os, signal, sys; "
                       "signal.signal(signal.SIGXFSZ, signal.SIG_IGN); "
                       "os.execv(sys.argv[1], sys.argv[1:])")
            args = [sys.executable, "-c", wrapper, *args]
        proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env)
        self.processes.append(proc)
        return proc

    def cleanup(self):
        for proc in self.processes:
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)
        self.stop_server()
        if self in _LIVE_HARNESSES:
            _LIVE_HARNESSES.remove(self)


def cleanup_all_harnesses():
    """Release every harness still alive after a scenario failure."""
    for harness in list(_LIVE_HARNESSES):
        try:
            harness.cleanup()
        except Exception:
            pass


def snapshot(root):
    """Return rel path -> (sha256, mtime_ns) for every file under root."""
    result = {}
    if not root.exists():
        return result
    for path in sorted(root.rglob("*")):
        if path.is_file():
            rel = path.relative_to(root).as_posix()
            result[rel] = (hashlib.sha256(path.read_bytes()).hexdigest(), path.stat().st_mtime_ns)
    return result


def sidecars(db):
    db = Path(db)
    found = []
    for suffix in ("", "-wal", "-shm", "-journal"):
        candidate = Path(str(db) + suffix)
        if candidate.exists():
            found.append((candidate.name, candidate.stat().st_size))
    return sorted(found)


def read_db(db):
    db = Path(db)
    if not db.exists():
        return {"exists": False, "ownership": [], "rounds": [], "sidecars": []}
    conn = sqlite3.connect(db.resolve().as_uri() + "?mode=ro", uri=True)
    try:
        ownership = list(conn.execute(
            "SELECT source_key, relative_path, absolute_path, missing_count FROM ownership ORDER BY relative_path"))
        rounds = list(conn.execute("SELECT source_key, epoch FROM rounds ORDER BY source_key"))
    finally:
        conn.close()
    return {"exists": True, "ownership": ownership, "rounds": rounds, "sidecars": sidecars(db)}


def snapshot_tree(root):
    """Capture directory entries without following symlinks or opening special files."""
    root = Path(root)
    try:
        root_stat = root.lstat()
    except FileNotFoundError:
        return {"exists": False, "root": None, "entries": {}}

    def info(st, path):
        digest = None
        link = None
        if stat.S_ISREG(st.st_mode):
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
        elif stat.S_ISLNK(st.st_mode):
            link = os.readlink(path)
        return (st.st_mode, st.st_size, st.st_mtime_ns, digest, link)

    entries = {}
    stack = [(root, "")]
    while stack:
        parent, rel_parent = stack.pop()
        with os.scandir(parent) as children:
            for item in children:
                child = Path(item.path)
                rel = (Path(rel_parent) / item.name).as_posix() if rel_parent else item.name
                st = item.stat(follow_symlinks=False)
                entries[rel] = info(st, child)
                if stat.S_ISDIR(st.st_mode):
                    stack.append((child, rel))
    return {"exists": True, "root": info(root_stat, root), "entries": entries}


def assert_valid_strm(path, expect_url):
    raw = path.read_bytes()
    check(len(raw) <= 65536, f"{path} exceeds 65536 bytes")
    check(not raw.startswith(b"\xef\xbb\xbf"), f"{path} has a BOM")
    check(b"\r\n" not in raw, f"{path} uses CRLF")
    text = raw.decode("utf-8")
    check(text.startswith("# strm-probe-version:1\n"), f"{path} version marker is not first: {text!r}")
    check(text.endswith("\n"), f"{path} does not end with LF")
    check("Modified" not in text, f"{path} contains Modified")
    check("sign=" not in text and "raw_url" not in text, f"{path} leaks an unsafe URL")
    lines = [line for line in text.splitlines() if line and not line.startswith("#")]
    check(len(lines) == 1, f"{path} must contain exactly one URL line")
    check(lines[0] == expect_url, f"{path} URL {lines[0]!r} != {expect_url!r}")
    return text


def write_unowned(path, content=b"pre-existing-unowned\n"):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(content)


def precreate_db_with_trigger(db, trigger_sql):
    db.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(db))
    try:
        conn.executescript(
            "CREATE TABLE IF NOT EXISTS ownership ("
            "source_key TEXT NOT NULL, relative_path TEXT NOT NULL, absolute_path TEXT NOT NULL,"
            "created_round INTEGER NOT NULL, missing_count INTEGER NOT NULL DEFAULT 0,"
            "PRIMARY KEY (source_key, relative_path));"
            "CREATE TABLE IF NOT EXISTS rounds ("
            "source_key TEXT NOT NULL PRIMARY KEY, epoch INTEGER NOT NULL, committed_at TEXT NOT NULL);")
        conn.executescript(trigger_sql)
        conn.commit()
    finally:
        conn.close()


def scenario_first_generation(h, results):
    api, dirs = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("first", api, [("/Movies", root)], db)

    dry = h.run(cfg, dry_run=True)
    check(dry.returncode == 0, f"dry-run exit {dry.returncode}: {dry.stderr}")
    check("created=4" in dry.stdout, f"dry-run summary: {dry.stdout}")
    check(not root.exists(), "dry-run created the output root")
    check(not db.exists(), "dry-run created the database")

    real = h.run(cfg)
    check(real.returncode == 0, f"sync exit {real.returncode}: {real.stderr}")
    check("created=4" in real.stdout, f"sync summary: {real.stdout}")
    check("filtered=1" in real.stdout, f"notes.txt must be filtered: {real.stdout}")

    files = snapshot(root)
    check(len(files) == 4, f"expected 4 STRM files, got {sorted(files)}")
    check("Movies/notes.strm" not in files, "notes.txt must not produce a STRM")
    assert_valid_strm(root / "Sample.strm", f"{DEFAULT_PUBLIC}/d/Movies/Sample.mkv")
    assert_valid_strm(root / "科幻" / "A #100%.strm",
                      f"{DEFAULT_PUBLIC}/d/Movies/%E7%A7%91%E5%B9%BB/A%20%23100%25.mkv")
    check("sha1:aabbccddeeff0011223344556677889900112233" in
          (root / "Sample.strm").read_text(), "valid sha1 missing")
    nohash = (root / "NoHash.strm").read_text()
    check("sha1" not in nohash, "missing hash must be omitted, not fabricated")

    state = read_db(db)
    check(len(state["ownership"]) == 4, f"expected 4 ownership rows: {state['ownership']}")
    check(len(state["rounds"]) == 1 and state["rounds"][0][1] == 1, f"round not committed: {state['rounds']}")
    check(state["sidecars"] == [("strmgen.db", state["sidecars"][0][1])] if state["sidecars"] else True,
          f"unexpected database sidecars: {state['sidecars']}")
    results["first_generation"] = {"files": sorted(files), "rounds": state["rounds"]}


def scenario_repeat_and_compare(h, results):
    api, dirs = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("repeat", api, [("/Movies", root)], db)
    check(h.run(cfg).returncode == 0, "initial sync failed")
    before = snapshot(root)

    repeat = h.run(cfg)
    check(repeat.returncode == 0, f"repeat exit {repeat.returncode}: {repeat.stderr}")
    check("skipped=4" in repeat.stdout and "created=0" in repeat.stdout, f"repeat summary: {repeat.stdout}")
    check(snapshot(root) == before, "repeat run changed bytes or mtime")

    # Same SHA-1, different Size -> skip with an anomaly, no rewrite.
    sample = next(entry for entry in dirs["/Movies"] if entry["name"] == "Sample.mkv")
    sample["size"] = sample["size"] + 1
    anomaly = h.run(cfg)
    check(anomaly.returncode == 0, f"size anomaly must not fail: {anomaly.stderr}")
    check("size differs" in anomaly.stderr, f"expected size anomaly: {anomaly.stderr}")
    check(snapshot(root)["Sample.strm"] == before["Sample.strm"], "size anomaly rewrote the file")

    # Different SHA-1 -> atomic update, ownership retained, round advances.
    sample["hash_info"] = {"sha1": "cccccccccccccccccccccccccccccccccccccccc"}
    epoch_before_change = read_db(db)["rounds"][0][1]
    changed = h.run(cfg)
    check(changed.returncode == 0, f"hash change must update and succeed: {changed.returncode} {changed.stderr}")
    check("updated=1" in changed.stdout and "skipped=3" in changed.stdout,
          f"hash change summary: {changed.stdout}")
    sample_after = snapshot(root)["Sample.strm"]
    check(sample_after != before["Sample.strm"], "hash change did not rewrite the file")
    check("cccccccccccccccccccccccccccccccccccccccc" in (root / "Sample.strm").read_text(),
          "updated file lacks the new hash")
    check(read_db(db)["rounds"][0][1] == epoch_before_change + 1, "update did not commit a round")
    check(len(read_db(db)["ownership"]) == 4, "update lost ownership rows")
    # A repeat of the updated state is a no-op.
    settled = snapshot(root)
    settled_run = h.run(cfg)
    check(settled_run.returncode == 0 and "skipped=4" in settled_run.stdout,
          f"post-update repeat summary: {settled_run.stdout}")
    check(snapshot(root) == settled, "post-update repeat changed bytes or mtime")

    # URL change -> every owned file is updated atomically and the round advances.
    url_before = snapshot(root)
    url_epoch = read_db(db)["rounds"][0][1]
    cfg_url = h.config("repeat-url", api, [("/Movies", root)], db, public="https://other.example.invalid")
    url_change = h.run(cfg_url)
    check(url_change.returncode == 0, f"url change must update and succeed: {url_change.returncode} {url_change.stderr}")
    check("updated=4" in url_change.stdout, f"url change summary: {url_change.stdout}")
    check(snapshot(root) != url_before, "url change did not rewrite files")
    check("https://other.example.invalid/d/Movies/Sample.mkv" in (root / "Sample.strm").read_text(),
          "url change did not persist the new URL")
    check(read_db(db)["rounds"][0][1] == url_epoch + 1, "url change did not commit a round")

    # SHA-1 incomparable, Size differs -> update required; same Size skips.
    nohash = next(entry for entry in dirs["/Movies"] if entry["name"] == "NoHash.mp4")
    nohash_size_before = nohash["size"]
    nohash["size"] = nohash_size_before + 5
    incomparable_epoch = read_db(db)["rounds"][0][1]
    incomparable = h.run(cfg_url)
    check(incomparable.returncode == 0, f"incomparable size change must update: {incomparable.returncode} {incomparable.stderr}")
    check("updated=1" in incomparable.stdout, f"incomparable summary: {incomparable.stdout}")
    check(f"# strm-probe-size:{nohash_size_before + 5}" in (root / "NoHash.strm").read_text(),
          "incomparable size change was not applied")
    check(read_db(db)["rounds"][0][1] == incomparable_epoch + 1,
          "incomparable size update did not commit a round")
    results["repeat_and_compare"] = {"anomaly": anomaly.stderr.strip().splitlines()[-1:],
                                    "hash_update_exit": changed.returncode,
                                    "url_update_exit": url_change.returncode,
                                    "incomparable_update_exit": incomparable.returncode}


def scenario_ownership_and_paths(h, results):
    # Unowned existing STRM must be preserved and reported as a conflict.
    api, dirs = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    write_unowned(root / "Sample.strm")
    cfg = h.config("owned", api, [("/Movies", root)], db)
    conflict = h.run(cfg)
    check(conflict.returncode == 1, f"unowned file must conflict: {conflict.returncode}")
    check("conflicts=1" in conflict.stdout, f"conflict summary: {conflict.stdout}")
    check((root / "Sample.strm").read_bytes() == b"pre-existing-unowned\n", "unowned file changed")
    check(all(row[1] != "Sample.strm" for row in read_db(db)["ownership"]), "unowned file was claimed")
    h.stop_server()

    # Symlink target must be refused and the outside sentinel preserved.
    h2 = Harness(h.binary, h.workdir / "symlink")
    api2, _ = h2.server()
    root2 = h2.workdir / "out"
    outside = h2.workdir / "outside"
    outside.mkdir(parents=True, exist_ok=True)
    sentinel = outside / "sentinel"
    sentinel.write_bytes(b"sentinel")
    (root2).mkdir(parents=True, exist_ok=True)
    os.symlink(sentinel, root2 / "Sample.strm")
    cfg2 = h2.config("symlink", api2, [("/Movies", root2)], h2.workdir / "data" / "strmgen.db")
    link = h2.run(cfg2)
    check(link.returncode == 1, f"symlink target must fail: {link.returncode}")
    check(sentinel.read_bytes() == b"sentinel", "sentinel outside the root was modified")
    h2.cleanup()

    # Same target from two inputs (A.mkv / A.mp4) must not pick one arbitrarily.
    h3 = Harness(h.binary, h.workdir / "sametarget")
    conflict_dirs = {"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 10, "hash_info": {"sha1": "a" * 40}},
        {"name": "A.mp4", "is_dir": False, "size": 20, "hash_info": {"sha1": "b" * 40}},
    ]}
    api3, _ = h3.server(directories=conflict_dirs)
    root3 = h3.workdir / "out"
    cfg3 = h3.config("same", api3, [("/Movies", root3)], h3.workdir / "data" / "strmgen.db")
    same = h3.run(cfg3)
    check(same.returncode == 1, f"same-target conflict must fail: {same.returncode}")
    check(not (root3 / "A.strm").exists(), "conflicted target was created")
    check(read_db(h3.workdir / "data" / "strmgen.db")["ownership"] == [], "conflicted target was owned")
    h3.cleanup()

    # The same conflict split across pagination boundaries must also fail.
    h3b = Harness(h.binary, h.workdir / "crosspage")
    wide = [{"name": f"F{i:04d}.mkv", "is_dir": False, "size": i, "hash_info": None} for i in range(250)]
    wide[10] = {"name": "A.mkv", "is_dir": False, "size": 10, "hash_info": {"sha1": "a" * 40}}
    wide[220] = {"name": "A.mp4", "is_dir": False, "size": 20, "hash_info": {"sha1": "b" * 40}}
    api3b, _ = h3b.server(directories={"/Movies": wide})
    root3b = h3b.workdir / "out"
    cfg3b = h3b.config("crosspage", api3b, [("/Movies", root3b)], h3b.workdir / "data" / "strmgen.db")
    crosspage = h3b.run(cfg3b)
    check(crosspage.returncode == 1, f"cross-page conflict must fail: {crosspage.returncode}")
    check("conflicts=1" in crosspage.stdout, f"cross-page conflict summary: {crosspage.stdout}")
    check(not (root3b / "A.strm").exists(), "cross-page conflicted target was created")
    check(len(read_db(h3b.workdir / "data" / "strmgen.db")["rounds"]) == 0,
          "cross-page conflict committed a round")
    h3b.cleanup()

    # A genuinely wide directory completes through the real binary.
    h3c = Harness(h.binary, h.workdir / "wide")
    wide_only = [{"name": f"F{i:04d}.mkv", "is_dir": False, "size": i, "hash_info": None} for i in range(300)]
    api3c, _ = h3c.server(directories={"/Movies": wide_only})
    root3c = h3c.workdir / "out"
    cfg3c = h3c.config("wide", api3c, [("/Movies", root3c)], h3c.workdir / "data" / "strmgen.db", workers=4)
    wide_run = h3c.run(cfg3c)
    check(wide_run.returncode == 0, f"wide directory must complete: {wide_run.returncode} {wide_run.stderr}")
    check("created=300" in wide_run.stdout, f"wide summary: {wide_run.stdout}")
    check(len(snapshot(root3c)) == 300, "wide directory produced the wrong file count")
    h3c.cleanup()

    # A deep directory tree completes through the real binary.
    h3d = Harness(h.binary, h.workdir / "deep")
    deep = {}
    remote = "/Movies"
    for i in range(30):
        child = f"d{i:02d}"
        deep[remote] = [{"name": child, "is_dir": True, "size": 0, "hash_info": None},
                        {"name": f"F{i}.mkv", "is_dir": False, "size": i, "hash_info": None}]
        remote = remote + "/" + child
    deep[remote] = [{"name": "Leaf.mkv", "is_dir": False, "size": 1, "hash_info": None}]
    api3d, _ = h3d.server(directories=deep)
    root3d = h3d.workdir / "out"
    cfg3d = h3d.config("deep", api3d, [("/Movies", root3d)], h3d.workdir / "data" / "strmgen.db", workers=3)
    deep_run = h3d.run(cfg3d)
    check(deep_run.returncode == 0, f"deep directory must complete: {deep_run.returncode} {deep_run.stderr}")
    check("created=31" in deep_run.stdout, f"deep summary: {deep_run.stdout}")
    h3d.cleanup()

    # A breadth-first frontier that would exceed the in-memory directory
    # queue limit fails explicitly before output or a successful round.
    h3e = Harness(h.binary, h.workdir / "directory-queue-limit")
    queue_dirs = {"/Movies": [{"name": f"D{i:04d}", "is_dir": True, "size": 0} for i in range(5000)]}
    for i in range(5000):
        queue_dirs[f"/Movies/D{i:04d}"] = [
            {"name": "L", "is_dir": True, "size": 0},
            {"name": "R", "is_dir": True, "size": 0},
        ]
    queue_dirs["/Movies/D0000/L"] = [
        {"name": "X", "is_dir": True, "size": 0},
        {"name": "Y", "is_dir": True, "size": 0},
    ]
    api3e, _ = h3e.server(directories=queue_dirs)
    root3e = h3e.workdir / "out"
    db3e = h3e.workdir / "data" / "strmgen.db"
    cfg3e = h3e.config("queue-limit", api3e, [("/Movies", root3e)], db3e, workers=1)
    queue_limit = h3e.run(cfg3e, timeout=120)
    check(queue_limit.returncode == 1, f"queue overflow must fail explicitly: {queue_limit.returncode}")
    check("errors=1" in queue_limit.stdout and "partial=true" in queue_limit.stdout,
          f"queue overflow was not summarized as incomplete: {queue_limit.stdout}")
    check(not root3e.exists(), "queue overflow created output despite having no media files")
    queue_state = read_db(db3e)
    check(queue_state["ownership"] == [] and queue_state["rounds"] == [],
          "queue overflow created ownership or a complete round")
    h3e.cleanup()

    # Invalid existing owned file -> conflict on restart.
    h4 = Harness(h.binary, h.workdir / "invalidowned")
    api4, _ = h4.server()
    root4 = h4.workdir / "out"
    db4 = h4.workdir / "data" / "strmgen.db"
    cfg4 = h4.config("invalid", api4, [("/Movies", root4)], db4)
    check(h4.run(cfg4).returncode == 0, "seed sync failed")
    (root4 / "Sample.strm").write_bytes(b"garbage-not-a-strm\n")
    invalid = h4.run(cfg4)
    check(invalid.returncode == 1, f"invalid owned file must conflict: {invalid.returncode}")
    check((root4 / "Sample.strm").read_bytes() == b"garbage-not-a-strm\n", "invalid file was overwritten")
    h4.cleanup()

    # Configuration-level path boundaries are rejected before any output.
    h5 = Harness(h.binary, h.workdir / "configreject")
    api5, _ = h5.server()
    overlap = h5.config("overlap", api5, [("/Movies", h5.workdir / "out"), ("/TV", h5.workdir / "out" / "TV")],
                        h5.workdir / "data" / "strmgen.db")
    over = h5.run(overlap)
    check(over.returncode == 2, f"overlapping roots must be a usage error: {over.returncode}")
    traversal = h5.config("traversal", api5, [("/Movies/../etc", h5.workdir / "out2")],
                          h5.workdir / "data2" / "strmgen.db")
    trav = h5.run(traversal)
    check(trav.returncode == 2, f"traversal remote must be a usage error: {trav.returncode}")
    cleanup = h5.config("cleanup", api5, [("/Movies", h5.workdir / "out3")],
                        h5.workdir / "data3" / "strmgen.db", cleanup=True)
    clean = h5.run(cleanup)
    check(clean.returncode == 2, f"cleanup.enabled must be rejected: {clean.returncode}")
    h5.cleanup()

    # Every existing output ancestor is checked, and a symlink is rejected
    # before database/output creation.
    h6 = Harness(h.binary, h.workdir / "ancestor-symlink")
    api6, _ = h6.server()
    real6 = h6.workdir / "outside"
    real6.mkdir()
    (real6 / "sentinel.txt").write_bytes(b"unchanged")
    alias6 = h6.workdir / "alias"
    alias6.symlink_to(real6, target_is_directory=True)
    before6 = snapshot_tree(real6)
    out6 = alias6 / "out"
    db6 = h6.workdir / "data" / "db.sqlite"
    cfg6 = h6.config("ancestor-symlink", api6, [("/Movies", out6)], db6)
    ancestor_link = h6.run(cfg6)
    check(ancestor_link.returncode == 2, f"ancestor symlink must be rejected: {ancestor_link.returncode}")
    check(snapshot_tree(real6) == before6, "ancestor symlink rejection changed the real target")
    check(not db6.exists(), "ancestor symlink rejection created a database")
    h6.cleanup()

    # Database paths through aliases and sidecar paths under an output root
    # are rejected before any product file is created.
    h7 = Harness(h.binary, h.workdir / "database-alias")
    api7, _ = h7.server()
    real7 = h7.workdir / "real"
    real7.mkdir()
    alias7 = h7.workdir / "alias"
    alias7.symlink_to(real7, target_is_directory=True)
    out7 = real7 / "out"
    db7 = alias7 / "out" / "db.sqlite"
    before7 = snapshot_tree(real7)
    cfg7 = h7.config("database-alias", api7, [("/Movies", out7)], db7)
    db_alias = h7.run(cfg7)
    check(db_alias.returncode == 2, f"database alias must be rejected: {db_alias.returncode}")
    check(snapshot_tree(real7) == before7, "database alias rejection changed the output tree")
    h7.cleanup()

    h8 = Harness(h.binary, h.workdir / "database-sidecar")
    api8, _ = h8.server()
    sidecar_root = h8.workdir / "data.sqlite-wal"
    sidecar_root.mkdir()
    sidecar_cfg = h8.config("database-sidecar", api8, [("/Movies", sidecar_root)],
                            h8.workdir / "data.sqlite")
    sidecar_before = snapshot_tree(sidecar_root)
    sidecar_rejected = h8.run(sidecar_cfg)
    check(sidecar_rejected.returncode == 2,
          f"SQLite sidecar under output must be rejected: {sidecar_rejected.returncode}")
    check(snapshot_tree(sidecar_root) == sidecar_before, "sidecar rejection changed output directory")
    h8.cleanup()

    # The output root is pinned before the API request. A root pathname swap
    # during the HTTP request must not redirect writes into its replacement.
    for initially_exists in (False, True):
        name = "root-swap-existing" if initially_exists else "root-swap-missing"
        hh = Harness(h.binary, h.workdir / name)
        outside = hh.workdir / "sentinel"
        outside.mkdir()
        (outside / "keep.txt").write_bytes(b"stable")
        root = hh.workdir / "out"
        if initially_exists:
            root.mkdir()
        swapped = threading.Event()

        def swap_before(endpoint, path, request):
            if endpoint.endswith("/list") and path == "/Movies" and not swapped.is_set():
                swapped.set()
                if initially_exists:
                    os.rename(root, hh.workdir / "detached-out")
                os.symlink(outside, root, target_is_directory=True)

        api_swap, _ = hh.server(hooks={"before": swap_before})
        db = hh.workdir / "data" / "strmgen.db"
        cfg = hh.config(name, api_swap, [("/Movies", root)], db)
        outside_before = snapshot_tree(outside)
        result = hh.run(cfg)
        check(swapped.is_set(), "root-swap fixture did not reach its HTTP barrier")
        check(result.returncode == 1, f"root swap must fail safely: {result.returncode} {result.stderr}")
        # Each eligible media target attempts a bound-root operation and is
        # rejected after the pathname swap; count those four item failures
        # rather than collapsing them into a false success summary.
        check("errors=4" in result.stdout and "partial=true" in result.stdout,
              f"root swap item failures were not summarized: {result.stdout}")
        check(snapshot_tree(outside) == outside_before, "root swap wrote into the replacement directory")
        state = read_db(db)
        check(state["ownership"] == [] and state["rounds"] == [],
              f"root swap produced ownership or a successful round: {state}")
        hh.cleanup()

    results["ownership_and_paths"] = {"conflict_exit": conflict.returncode, "symlink_exit": link.returncode,
                                      "same_target_exit": same.returncode, "invalid_exit": invalid.returncode,
                                      "ancestor_symlink_exit": ancestor_link.returncode,
                                      "database_alias_exit": db_alias.returncode,
                                      "database_sidecar_exit": sidecar_rejected.returncode}


def _override_list(data, status=200, code=200):
    def override(endpoint, path, request, directories):
        if endpoint.endswith("/list") and path == "/Movies":
            return status, code, data
        return None
    return override


def scenario_api_and_limits(h, results):
    observed = {}

    def run_case(name, directories, override=None, expect_exit=1, expect_rounds=0):
        hh = Harness(h.binary, h.workdir / name)
        hooks = {"override": override} if override else None
        api, _ = hh.server(directories=directories, hooks=hooks)
        root = hh.workdir / "out"
        db = hh.workdir / "data" / "strmgen.db"
        cfg = hh.config(name, api, [("/Movies", root)], db)
        proc = hh.run(cfg)
        state = read_db(db)
        check(proc.returncode == expect_exit,
              f"{name}: exit {proc.returncode} want {expect_exit} ({proc.stderr.strip()})")
        check(len(state["rounds"]) == expect_rounds,
              f"{name}: rounds {state['rounds']} want {expect_rounds}")
        hh.cleanup()
        observed[name] = proc.returncode
        return proc

    # HTTP 200 with application code 500.
    run_case("code500", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list(None, status=200, code=500))
    # HTTP 503 transport failure.
    run_case("http503", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list(None, status=503, code=503))
    # Invalid required field (missing is_dir).
    run_case("badshape", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [{"name": "A.mkv", "size": 1, "hash_info": None}], "total": 1}))
    run_case("contentmissing", {"/Movies": []}, _override_list({"total": 0}))
    run_case("contentnull", {"/Movies": []}, _override_list({"content": None, "total": 0}))
    run_case("contentwrongtype", {"/Movies": []},
             _override_list({"content": {"name": "A.mkv"}, "total": 1}))
    # Early empty page before total was reached.
    run_case("earlyempty", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [], "total": 3}))
    # A second-page failure must leave the first page unprocessed and retryable.
    page_two_calls = {"n": 0}

    def fail_page_two(endpoint, path, request, directories):
        if endpoint.endswith("/list") and path == "/Movies" and request.get("page") == 2 and page_two_calls["n"] == 0:
            page_two_calls["n"] += 1
            return 503, 503, None
        return None

    h_page2 = Harness(h.binary, h.workdir / "page2-failure-retry")
    page_entries = [{"name": f"F{i:03d}.mkv", "is_dir": False, "size": i, "hash_info": None}
                    for i in range(250)]
    api_page2, _ = h_page2.server(directories={"/Movies": page_entries}, hooks={"override": fail_page_two})
    page2_root = h_page2.workdir / "out"
    page2_db = h_page2.workdir / "data" / "strmgen.db"
    page2_cfg = h_page2.config("page2", api_page2, [("/Movies", page2_root)], page2_db)
    page2_failed = h_page2.run(page2_cfg)
    check(page2_failed.returncode == 1 and page_two_calls["n"] == 1,
          f"page-two failure was not observed: {page2_failed.returncode} {page2_failed.stderr}")
    check(not page2_root.exists(), "failed second page created output")
    page2_state = read_db(page2_db)
    check(page2_state["ownership"] == [] and page2_state["rounds"] == [],
          "failed second page created ownership or a successful round")
    page2_retry = h_page2.run(page2_cfg)
    check(page2_retry.returncode == 0, f"page-two retry failed: {page2_retry.returncode} {page2_retry.stderr}")
    check(len(snapshot(page2_root)) == 250 and len(read_db(page2_db)["rounds"]) == 1,
          "page-two retry did not complete from a fresh listing")
    h_page2.cleanup()
    observed["page2_failure_retry"] = page2_retry.returncode
    # Duplicate entry names.
    run_case("duplicate", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [
                 {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None},
                 {"name": "A.mkv", "is_dir": False, "size": 2, "hash_info": None}], "total": 2}))
    # Oversized response body.
    big = "z" * (9 * 1024 * 1024)
    run_case("oversized", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [{"name": big, "is_dir": False, "size": 1, "hash_info": None}], "total": 1}))

    # The per-directory entry count and aggregate name bytes are independently
    # capped, before any STRM or ownership row can be produced.
    too_many = [{"name": f"F{i:05d}.mkv", "is_dir": False, "size": i} for i in range(10001)]
    run_case("directory_entry_limit", {"/Movies": too_many})
    long_name = "N" * ((2 << 20) + 1) + ".mkv"
    run_case("directory_name_byte_limit", {"/Movies": [
        {"name": long_name, "is_dir": False, "size": 1}]})

    # 401 for a wrong token.
    hh = Harness(h.binary, h.workdir / "unauthorized")
    api, _ = hh.server()
    root = hh.workdir / "out"
    db = hh.workdir / "data" / "strmgen.db"
    cfg = hh.config("unauthorized", api, [("/Movies", root)], db)
    token_marker = "SYNTHETIC_TOKEN_COOKIE_SIGN_MARKER_20261018"
    proc = hh.run(cfg, extra_env={"OPENLIST_TOKEN": token_marker})
    check(proc.returncode == 1, f"wrong token must exit 1: {proc.returncode}")
    check(len(read_db(db)["rounds"]) == 0, "wrong token committed a round")
    check(token_marker not in proc.stdout + proc.stderr,
          "authentication failure echoed a token")
    hh.cleanup()
    observed["unauthorized"] = proc.returncode

    # Synthetic API body secrets and a private remote root must never be
    # reflected in diagnostics or the source summary.
    hh_redact = Harness(h.binary, h.workdir / "redaction")
    secret = "SYNTHETIC_COOKIE_SIGN_SECRET_20261018"
    private_remote = f"/private/full/root/{secret}"

    def secret_api(endpoint, path, request, directories):
        if endpoint.endswith("/get") and path == private_remote:
            return 200, 403, {
                "message": secret,
                "url": f"https://private.example.invalid/file?sign={secret}",
            }
        return None

    api_redact, _ = hh_redact.server(hooks={"override": secret_api})
    root_redact = hh_redact.workdir / "PRIVATE_OUTPUT_PATH_MARKER" / "out"
    db_redact = hh_redact.workdir / "PRIVATE_DATABASE_PATH_MARKER" / "db.sqlite"
    cfg_redact = hh_redact.config("redaction", api_redact, [(private_remote, root_redact)], db_redact)
    api_redacted = hh_redact.run(cfg_redact)
    combined = api_redacted.stdout + api_redacted.stderr
    check(api_redacted.returncode == 1, f"API redaction case failed: {api_redacted.returncode}")
    for marker in (secret, private_remote, "private/full/root", "private.example.invalid",
                   "PRIVATE_OUTPUT_PATH_MARKER", "PRIVATE_DATABASE_PATH_MARKER", "sign="):
        check(marker not in combined, f"API/configured path marker leaked: {marker}")
    check("errors=1" in api_redacted.stdout and "partial=true" in api_redacted.stdout,
          f"API root failure was not counted: {api_redacted.stdout}")
    check(read_db(db_redact)["rounds"] == [] and not root_redact.exists(),
          "redacted API failure created output or committed a round")
    hh_redact.cleanup()
    observed["redacted_api_root_failure"] = api_redacted.returncode

    hh_yaml = Harness(h.binary, h.workdir / "yaml-redaction")
    api_yaml, _ = hh_yaml.server()
    yaml_cfg = hh_yaml.config("unknown", api_yaml, [("/Movies", hh_yaml.workdir / "out")],
                              hh_yaml.workdir / "db.sqlite")
    yaml_secret = "SYNTHETIC_UNKNOWN_FIELD_SECRET_20261018"
    with yaml_cfg.open("a", encoding="utf-8") as stream:
        stream.write(f'private_{yaml_secret}: "https://private.example.invalid/?sign={yaml_secret}"\n')
    yaml_result = hh_yaml.run(yaml_cfg)
    check(yaml_result.returncode == 2, f"unknown YAML field should fail: {yaml_result.returncode}")
    check(yaml_secret not in yaml_result.stdout + yaml_result.stderr and
          "private.example.invalid" not in yaml_result.stdout + yaml_result.stderr,
          "unknown-field YAML details leaked")
    missing_path = hh_yaml.workdir / "PRIVATE_CONFIG_PATH_MARKER" / "missing.yaml"
    missing_result = hh_yaml.run(missing_path)
    check(missing_result.returncode == 2 and
          "PRIVATE_CONFIG_PATH_MARKER" not in missing_result.stdout + missing_result.stderr,
          "missing config path leaked")
    hh_yaml.cleanup()
    observed["redacted_yaml_error"] = yaml_result.returncode

    # Duplicate roots should identify only the trusted field/index, not the
    # configured private output path.
    hh_duplicate = Harness(h.binary, h.workdir / "duplicate-root-redaction")
    api_duplicate, _ = hh_duplicate.server()
    duplicate_marker = "PRIVATE_DUPLICATE_OUTPUT_MARKER_20261018"
    duplicate_root = hh_duplicate.workdir / duplicate_marker / "out"
    duplicate_db = hh_duplicate.workdir / "PRIVATE_DUPLICATE_DB_MARKER" / "db.sqlite"
    duplicate_cfg = hh_duplicate.config("duplicate", api_duplicate,
                                        [("/Movies", duplicate_root), ("/TV", duplicate_root)],
                                        duplicate_db)
    duplicate_result = hh_duplicate.run(duplicate_cfg)
    duplicate_text = duplicate_result.stdout + duplicate_result.stderr
    check(duplicate_result.returncode == 2,
          f"duplicate output config should fail before sync: {duplicate_result.returncode}")
    check("sources[1].output" in duplicate_text and duplicate_marker not in duplicate_text and
          str(duplicate_root) not in duplicate_text,
          f"duplicate-root diagnostic leaked a private path: {duplicate_text}")
    check(not duplicate_root.exists() and not duplicate_db.exists(),
          "duplicate-root rejection created output or database")
    hh_duplicate.cleanup()
    observed["redacted_duplicate_root"] = duplicate_result.returncode

    # A database path through a symlinked ancestor is a preflight error; only
    # its field name may be shown, not either path target.
    hh_db_alias = Harness(h.binary, h.workdir / "database-alias-redaction")
    api_db_alias, _ = hh_db_alias.server()
    db_target = hh_db_alias.workdir / "PRIVATE_DATABASE_TARGET_MARKER"
    db_target.mkdir()
    db_alias = hh_db_alias.workdir / "PRIVATE_DATABASE_ALIAS_MARKER"
    db_alias.symlink_to(db_target, target_is_directory=True)
    db_alias_path = db_alias / "db.sqlite"
    db_alias_root = hh_db_alias.workdir / "PRIVATE_OUTPUT_ALIAS_MARKER" / "out"
    db_alias_cfg = hh_db_alias.config("database-alias", api_db_alias,
                                      [("/Movies", db_alias_root)], db_alias_path)
    db_alias_result = hh_db_alias.run(db_alias_cfg)
    db_alias_text = db_alias_result.stdout + db_alias_result.stderr
    check(db_alias_result.returncode == 2,
          f"symlinked database path should be rejected: {db_alias_result.returncode}")
    check("database.path" in db_alias_text and all(marker not in db_alias_text for marker in
          ("PRIVATE_DATABASE_TARGET_MARKER", "PRIVATE_DATABASE_ALIAS_MARKER",
           "PRIVATE_OUTPUT_ALIAS_MARKER", str(db_alias_path))),
          f"database path diagnostic leaked a private path: {db_alias_text}")
    check(not db_alias_root.exists() and list(db_target.iterdir()) == [],
          "database alias rejection changed configured paths")
    hh_db_alias.cleanup()
    observed["redacted_database_alias"] = db_alias_result.returncode

    # A syntactically valid but corrupt SQLite file reaches runtime storage
    # initialization. The CLI must not expose SQLite text/path or change the
    # file, and it must not create output before failing.
    hh_db_error = Harness(h.binary, h.workdir / "database-error-redaction")
    api_db_error, _ = hh_db_error.server()
    private_db_marker = "PRIVATE_DATABASE_RUNTIME_MARKER"
    content_marker = "SYNTHETIC_DATABASE_CONTENT_SECRET_20261018"
    db_error = hh_db_error.workdir / private_db_marker / "strmgen.db"
    db_error.parent.mkdir()
    db_bytes = ("not a sqlite database: " + content_marker).encode()
    db_error.write_bytes(db_bytes)
    db_error_root = hh_db_error.workdir / "PRIVATE_OUTPUT_RUNTIME_MARKER" / "out"
    db_error_cfg = hh_db_error.config("database-error", api_db_error,
                                      [("/Movies", db_error_root)], db_error)
    db_error_result = hh_db_error.run(db_error_cfg)
    db_error_text = db_error_result.stdout + db_error_result.stderr
    check(db_error_result.returncode == 1,
          f"corrupt database should be a runtime error: {db_error_result.returncode}")
    check("database.path is unavailable or unsafe" in db_error_text and
          all(marker not in db_error_text for marker in
              (content_marker, private_db_marker, "PRIVATE_OUTPUT_RUNTIME_MARKER", str(db_error_root), str(db_error))),
          f"database runtime diagnostic leaked a private value: {db_error_text}")
    check(db_error.read_bytes() == db_bytes, "corrupt database file was changed")
    check(sidecars(db_error) == [(db_error.name, len(db_bytes))],
          f"database failure created or changed sidecars: {sidecars(db_error)}")
    check(not db_error_root.exists(), "database error created output")
    hh_db_error.cleanup()
    observed["redacted_database_runtime_error"] = db_error_result.returncode

    # Source root get semantics are required: a file-shaped root is not an
    # empty directory and cannot commit a successful round.
    hh_root = Harness(h.binary, h.workdir / "file-root")

    def file_root(endpoint, path, request, directories):
        if endpoint.endswith("/get") and path == "/Movies":
            return 200, 200, {"name": "Movies", "is_dir": False, "size": 1}
        return None

    api_root, _ = hh_root.server(hooks={"override": file_root})
    root_file = hh_root.workdir / "out"
    db_root = hh_root.workdir / "db.sqlite"
    cfg_root = hh_root.config("file-root", api_root, [("/Movies", root_file)], db_root)
    root_result = hh_root.run(cfg_root)
    check(root_result.returncode == 1, f"file-shaped source root must fail: {root_result.returncode}")
    check("errors=1" in root_result.stdout and "partial=true" in root_result.stdout,
          f"file-shaped root was not counted: {root_result.stdout}")
    check(read_db(db_root)["rounds"] == [], "file-shaped root committed a round")
    check(not root_file.exists(), "file-shaped root created output")
    hh_root.cleanup()
    observed["file_root"] = root_result.returncode

    # A genuinely empty directory is a success, not a failure.
    hh2 = Harness(h.binary, h.workdir / "emptydir")
    api2, _ = hh2.server(directories={"/Movies": []})
    root2 = hh2.workdir / "out"
    db2 = hh2.workdir / "data" / "strmgen.db"
    cfg2 = hh2.config("empty", api2, [("/Movies", root2)], db2)
    empty = hh2.run(cfg2)
    check(empty.returncode == 0, f"empty directory must succeed: {empty.returncode} {empty.stderr}")
    check(len(read_db(db2)["rounds"]) == 1, "empty directory round was not committed")
    check(snapshot(root2) == {}, "empty directory produced files")
    hh2.cleanup()
    observed["emptydir"] = empty.returncode

    # Invalid optional Size values are omitted and reported, not allowed to
    # suppress an otherwise valid media entry. Unknown values do not imply a
    # change on the next run; valid zero and Int64 max remain representable.
    optional_cases = [
        ("missing", "__missing__"),
        ("null", None),
        ("negative", -1),
        ("overflow", 9223372036854775808),
        ("fraction", 1.5),
        ("string", "not-a-size"),
    ]
    for case_name, value in optional_cases:
        hh_opt = Harness(h.binary, h.workdir / f"optional-size-{case_name}")
        item = {"name": "A.mkv", "is_dir": False}
        if value != "__missing__":
            item["size"] = value
        directories_opt = {"/Movies": [item]}
        api_opt, _ = hh_opt.server(directories=directories_opt)
        root_opt = hh_opt.workdir / "out"
        db_opt = hh_opt.workdir / "db.sqlite"
        cfg_opt = hh_opt.config(case_name, api_opt, [("/Movies", root_opt)], db_opt)
        first_opt = hh_opt.run(cfg_opt)
        check(first_opt.returncode == 0,
              f"optional {case_name} size should not block media: {first_opt.returncode} {first_opt.stderr}")
        document = (root_opt / "A.strm").read_text()
        check("strm-probe-size" not in document, f"invalid {case_name} size was serialized")
        check("size" in first_opt.stderr.lower(), f"invalid {case_name} size reason missing")
        check(len(read_db(db_opt)["ownership"]) == 1 and read_db(db_opt)["rounds"][0][1] == 1,
              f"invalid {case_name} size did not create/commit valid media")
        before_opt = snapshot(root_opt)
        item["size"] = "still-unavailable"
        repeat_opt = hh_opt.run(cfg_opt)
        check(repeat_opt.returncode == 0 and "skipped=1" in repeat_opt.stdout,
              f"unknown size information should not infer a change: {repeat_opt.returncode} {repeat_opt.stdout}")
        check(snapshot(root_opt) == before_opt, f"invalid {case_name} size repeat rewrote STRM")
        hh_opt.cleanup()
        observed[f"optional_size_{case_name}"] = first_opt.returncode

    for case_name, size_value in (("zero", 0), ("max", 9223372036854775807)):
        hh_size = Harness(h.binary, h.workdir / f"valid-size-{case_name}")
        api_size, _ = hh_size.server(directories={"/Movies": [
            {"name": "A.mkv", "is_dir": False, "size": size_value}]})
        root_size = hh_size.workdir / "out"
        db_size = hh_size.workdir / "db.sqlite"
        cfg_size = hh_size.config(case_name, api_size, [("/Movies", root_size)], db_size)
        valid_size = hh_size.run(cfg_size)
        check(valid_size.returncode == 0, f"valid {case_name} size failed: {valid_size.stderr}")
        check(f"# strm-probe-size:{size_value}" in (root_size / "A.strm").read_text(),
              f"valid {case_name} size not serialized")
        hh_size.cleanup()
        observed[f"optional_size_{case_name}"] = valid_size.returncode

    # A batch of invalid names bounds diagnostics rather than retaining one
    # message per object; the directory still fails without a success round.
    hh_diag = Harness(h.binary, h.workdir / "diagnostics-bounded")
    invalid_entries = [{"name": f"bad{i}/A.mkv", "is_dir": False, "size": 1} for i in range(150)]
    api_diag, _ = hh_diag.server(directories={"/Movies": invalid_entries})
    root_diag = hh_diag.workdir / "out"
    db_diag = hh_diag.workdir / "db.sqlite"
    cfg_diag = hh_diag.config("diagnostics", api_diag, [("/Movies", root_diag)], db_diag)
    diag_result = hh_diag.run(cfg_diag)
    check(diag_result.returncode == 1, f"invalid-name batch must fail: {diag_result.returncode}")
    check("errors=150" in diag_result.stdout and "diagnostics_omitted=51" in diag_result.stdout,
          f"batch diagnostics were not counted/bounded: {diag_result.stdout}")
    diagnostic_lines = [line for line in diag_result.stderr.splitlines() if line.startswith("source 0:")]
    check(len(diagnostic_lines) == 100, f"expected 100 sampled diagnostics, got {len(diagnostic_lines)}")
    check(read_db(db_diag)["rounds"] == [] and not root_diag.exists(),
          "invalid-name batch produced output or a successful round")
    db_diag_before = db_diag.read_bytes()
    dry_diag = hh_diag.run(cfg_diag, dry_run=True)
    check(dry_diag.returncode == 1, "dry-run should report the same invalid-name failure")
    check(not root_diag.exists() and db_diag.read_bytes() == db_diag_before,
          "dry-run invalid-name batch changed output or database")
    observed["bounded_diagnostics"] = diag_result.returncode
    hh_diag.cleanup()

    # 403 / forbidden.
    run_case("forbidden", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list(None, status=200, code=403))
    results["api_and_limits"] = observed


def scenario_dry_run_boundaries(h, results):
    observed = {}
    # No output and no database.
    api, _ = h.server()
    root = h.workdir / "empty" / "out"
    db = h.workdir / "empty" / "data" / "strmgen.db"
    cfg = h.config("dryempty", api, [("/Movies", root)], db)
    dry = h.run(cfg, dry_run=True, extra_env={
        "STRMGEN_LOCK_DIR": str(root / "locks"),
        "TMPDIR": str(root / "tmp"),
    })
    check(dry.returncode == 0, f"dry-run exit {dry.returncode}: {dry.stderr}")
    check(not root.exists(), "dry-run created the output root")
    check(not db.exists(), "dry-run created the database")
    check(snapshot_tree(root)["entries"] == {}, "dry-run created a lock or temporary output entry")
    observed["no_side_effects"] = dry.returncode

    # With existing ownership: SQL bytes and mtimes must be unchanged.
    h2 = Harness(h.binary, h.workdir / "withdb")
    api2, _ = h2.server()
    root2 = h2.workdir / "out"
    db2 = h2.workdir / "data" / "strmgen.db"
    cfg2 = h2.config("drydb", api2, [("/Movies", root2)], db2)
    check(h2.run(cfg2).returncode == 0, "seed sync failed")
    before_files = snapshot(root2)
    before_db = read_db(db2)
    before_bytes = (db2).read_bytes()
    dry2 = h2.run(cfg2, dry_run=True)
    check(dry2.returncode == 0, f"dry-run with db exit {dry2.returncode}: {dry2.stderr}")
    check(snapshot(root2) == before_files, "dry-run changed output files")
    check(db2.read_bytes() == before_bytes, "dry-run changed database bytes")
    check(read_db(db2) == before_db, "dry-run changed database rows")
    check(not any(Path(str(db2) + s).exists() for s in ("-wal", "-shm", "-journal")),
          "dry-run created a database sidecar")
    observed["with_database"] = dry2.returncode
    h2.cleanup()

    # A WAL-mode database must not gain sidecars from a dry-run read.
    h2b = Harness(h.binary, h.workdir / "withwal")
    api2b, _ = h2b.server()
    root2b = h2b.workdir / "out"
    db2b = h2b.workdir / "data" / "strmgen.db"
    cfg2b = h2b.config("withwal", api2b, [("/Movies", root2b)], db2b)
    check(h2b.run(cfg2b).returncode == 0, "seed sync failed")
    conn = sqlite3.connect(str(db2b))
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    conn.close()
    for suffix in ("-wal", "-shm", "-journal"):
        Path(str(db2b) + suffix).unlink(missing_ok=True)
    before_wal = snapshot(root2b)
    before_wal_tree = snapshot_tree(root2b)
    dry2b = h2b.run(cfg2b, dry_run=True, extra_env={
        "STRMGEN_LOCK_DIR": str(root2b / "locks"),
        "TMPDIR": str(root2b),
    })
    check(dry2b.returncode == 0, f"dry-run on WAL db exit {dry2b.returncode}: {dry2b.stderr}")
    check(snapshot(root2b) == before_wal, "WAL dry-run changed output")
    check(snapshot_tree(root2b) == before_wal_tree, "WAL dry-run changed output directory entries")
    check(not any(Path(str(db2b) + s).exists() for s in ("-wal", "-shm", "-journal")),
          "WAL dry-run created a sidecar")
    observed["wal_database"] = dry2b.returncode
    h2b.cleanup()

    # A configured output equal to the immutable runtime lock namespace is
    # rejected before creating/locking anything, even during preview.
    fixed_lock_root = Path("/tmp/strmgen-locks")
    lock_before = snapshot_tree(fixed_lock_root)
    h2c = Harness(h.binary, h.workdir / "lock-path-overlap")
    api2c, _ = h2c.server()
    db2c = h2c.workdir / "data" / "strmgen.db"
    cfg2c = h2c.config("lock-overlap", api2c, [("/Movies", fixed_lock_root)], db2c)
    rejected = h2c.run(cfg2c, dry_run=True)
    check(rejected.returncode == 1, f"runtime lock overlap must fail: {rejected.returncode}")
    check(not db2c.exists(), "lock-overlap rejection created a database")
    check(snapshot_tree(fixed_lock_root) == lock_before, "lock-overlap rejection changed lock directory")
    observed["lock_path_overlap"] = rejected.returncode
    h2c.cleanup()

    # Dry-run still reports conflicts and needs-update with a non-zero exit.
    h3 = Harness(h.binary, h.workdir / "dryconflict")
    api3, _ = h3.server()
    root3 = h3.workdir / "out"
    write_unowned(root3 / "Sample.strm")
    cfg3 = h3.config("dryconflict", api3, [("/Movies", root3)], h3.workdir / "data" / "strmgen.db")
    conflict = h3.run(cfg3, dry_run=True)
    check(conflict.returncode == 1, f"dry-run conflict must be non-zero: {conflict.returncode}")
    check("conflicts=1" in conflict.stdout, f"dry-run conflict summary: {conflict.stdout}")
    check((root3 / "Sample.strm").read_bytes() == b"pre-existing-unowned\n", "dry-run changed a file")
    observed["conflict"] = conflict.returncode
    h3.cleanup()

    # Dry-run previews real updates with no filesystem or database side effect,
    # and the following real run applies them.
    h4 = Harness(h.binary, h.workdir / "dryupdate")
    api4, dirs4 = h4.server()
    root4 = h4.workdir / "out"
    db4 = h4.workdir / "data" / "strmgen.db"
    cfg4 = h4.config("dryupdate", api4, [("/Movies", root4)], db4)
    check(h4.run(cfg4).returncode == 0, "dry-run update seed failed")
    before4 = snapshot(root4)
    before_bytes4 = db4.read_bytes()
    cfg4b = h4.config("dryupdate-b", api4, [("/Movies", root4)], db4,
                      public="https://changed.example.invalid")
    dry_update = h4.run(cfg4b, dry_run=True)
    check(dry_update.returncode == 0 and "updated=4" in dry_update.stdout,
          f"dry-run should preview 4 updates: {dry_update.returncode} {dry_update.stdout}")
    check(snapshot(root4) == before4, "dry-run update changed output files")
    check(db4.read_bytes() == before_bytes4, "dry-run update changed database bytes")
    check(read_db(db4)["rounds"][0][1] == 1, "dry-run update advanced the round")
    real_update = h4.run(cfg4b)
    check(real_update.returncode == 0 and "updated=4" in real_update.stdout,
          f"real update after dry-run failed: {real_update.returncode} {real_update.stdout}")
    check(snapshot(root4) != before4 and read_db(db4)["rounds"][0][1] == 2,
          "real update did not apply after dry-run")
    observed["update_preview"] = dry_update.returncode
    h4.cleanup()
    results["dry_run_boundaries"] = observed


def scenario_source_identity(h, results):
    directories = {"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 10, "hash_info": {"sha1": "a" * 40}},
        {"name": "B.mp4", "is_dir": False, "size": 20, "hash_info": {"sha1": "b" * 40}},
    ]}
    api, _ = h.server(directories=directories, hooks={"tokens": ["rotated-fixture-token"]})
    out1 = h.workdir / "out-one"
    out2 = h.workdir / "out-two"
    db = h.workdir / "data" / "strmgen.db"
    sources = [("/Movies", out1), ("/Movies", out2)]
    cfg = h.config("two-output-sources", api, sources, db)
    first = h.run(cfg)
    check(first.returncode == 0, f"two-output first sync failed: {first.returncode} {first.stderr}")
    state1 = read_db(db)
    check(len(snapshot(out1)) == 2 and len(snapshot(out2)) == 2, "two independent outputs were not both created")
    check(len(state1["ownership"]) == 4, f"two output mappings lost ownership rows: {state1['ownership']}")
    check(len(state1["rounds"]) == 2, f"two output mappings lost rounds: {state1['rounds']}")
    source_keys = {row[0] for row in state1["ownership"]}
    check(len(source_keys) == 2 and all(key.startswith("source-v1:") for key in source_keys),
          f"source identities were not mapping-bound: {source_keys}")
    before1, before2 = snapshot(out1), snapshot(out2)

    repeat = h.run(cfg)
    check(repeat.returncode == 0 and repeat.stdout.count("skipped=2") == 2,
          f"both source mappings must be repeatable: {repeat.returncode} {repeat.stdout}")
    check(snapshot(out1) == before1 and snapshot(out2) == before2,
          "repeated source run changed output bytes or mtimes")

    # Array order and token rotation are not source identity; both sources
    # remain independently owned under the same two keys.
    reordered = h.config("sources-reordered", api, list(reversed(sources)), db)
    reordered_result = h.run(reordered)
    check(reordered_result.returncode == 0 and reordered_result.stdout.count("skipped=2") == 2,
          f"source reorder changed ownership: {reordered_result.returncode} {reordered_result.stdout}")
    rotated_cfg = h.config("rotated-token", api, [("/Movies", out1)], db)
    rotated = h.run(rotated_cfg, extra_env={"OPENLIST_TOKEN": "rotated-fixture-token"})
    check(rotated.returncode == 0 and "skipped=2" in rotated.stdout,
          f"credential rotation changed source identity: {rotated.returncode} {rotated.stdout}")
    state_after_rotate = read_db(db)
    check({row[0] for row in state_after_rotate["ownership"]} == source_keys,
          "credential rotation created a new source identity")
    check(len(state_after_rotate["rounds"]) == 2, "credential rotation created another round identity")

    # public_url is intentionally excluded from identity; URL comparison still
    # performs a real atomic update without changing the source identity.
    public_cfg = h.config("changed-public-url", api, [("/Movies", out1)], db,
                          public="https://changed.example.invalid")
    public_before = snapshot(out1)
    public_change = h.run(public_cfg)
    check(public_change.returncode == 0 and "updated=2" in public_change.stdout,
          f"public URL change did not update via STRM comparison: {public_change.returncode} {public_change.stdout}")
    check(snapshot(out1) != public_before, "public URL change did not rewrite files")
    check("https://changed.example.invalid/d/Movies/A.mkv" in (out1 / "A.strm").read_text(),
          "public URL change did not persist the new URL")
    check({row[0] for row in read_db(db)["ownership"]} == source_keys,
          "public URL update changed source identity")
    check(len(read_db(db)["rounds"]) == 2, "public URL update created another round identity")

    # Changing the API source creates a new identity and must not borrow old
    # ownership to update or commit against existing files.
    h.stop_server()
    api2, _ = h.server(directories=directories)
    api_changed_cfg = h.config("changed-api", api2, [("/Movies", out1)], db)
    before_api_change = snapshot(out1)
    api_changed = h.run(api_changed_cfg)
    check(api_changed.returncode == 1 and "conflicts=2" in api_changed.stdout,
          f"API change borrowed prior ownership: {api_changed.returncode} {api_changed.stdout}")
    check(snapshot(out1) == before_api_change, "API identity change modified prior files")
    state_after_api = read_db(db)
    check({row[0] for row in state_after_api["ownership"]} == source_keys,
          "API change overwrote or silently migrated old ownership")
    check(len(state_after_api["rounds"]) == 2, "API identity conflict committed a round")

    # A previous release's plain remote key is not trusted as proof of current
    # ownership and cannot turn an orphan into an owned file.
    legacy = Harness(h.binary, h.workdir / "legacy-remote-key")
    api_legacy, _ = legacy.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1}]})
    root_legacy = legacy.workdir / "out"
    db_legacy = legacy.workdir / "db.sqlite"
    write_unowned(root_legacy / "A.strm", b"legacy-owned-by-old-key\n")
    precreate_db_with_trigger(db_legacy, "")
    connection = sqlite3.connect(str(db_legacy))
    connection.execute("INSERT INTO ownership VALUES (?, ?, ?, ?, ?)",
                       ("/Movies", "A.strm", str(root_legacy / "A.strm"), 7, 0))
    connection.execute("INSERT INTO rounds VALUES (?, ?, ?)", ("/Movies", 7, "legacy"))
    connection.commit()
    connection.close()
    legacy_cfg = legacy.config("legacy-key", api_legacy, [("/Movies", root_legacy)], db_legacy)
    legacy_before = snapshot_tree(root_legacy)
    legacy_result = legacy.run(legacy_cfg)
    check(legacy_result.returncode == 1 and "conflicts=1" in legacy_result.stdout,
          f"legacy remote key was trusted: {legacy_result.returncode} {legacy_result.stdout}")
    check(snapshot_tree(root_legacy) == legacy_before, "legacy unowned file was modified")
    legacy_state = read_db(db_legacy)
    check(len(legacy_state["ownership"]) == 1 and legacy_state["ownership"][0][0] == "/Movies",
          "legacy ownership was migrated or overwritten")
    check(legacy_state["rounds"] == [("/Movies", 7)], "legacy record advanced a successful round")
    legacy.cleanup()

    results["source_identity"] = {
        "ownership_rows": len(state1["ownership"]),
        "source_identities": len(source_keys),
        "repeat_exit": repeat.returncode,
        "reordered_exit": reordered_result.returncode,
        "rotated_token_exit": rotated.returncode,
        "public_url_exit": public_change.returncode,
        "changed_api_exit": api_changed.returncode,
        "legacy_key_exit": legacy_result.returncode,
    }


def scenario_database_uri(h, results):
    api, _ = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data ?name#percent%" / "state ?#%.sqlite"
    cfg = h.config("special-db-name", api, [("/Movies", root)], db)
    first = h.run(cfg)
    check(first.returncode == 0, f"special database path failed: {first.returncode} {first.stderr}")
    check(db.is_file(), "configured special database file was not created")
    check(len(read_db(db)["ownership"]) == 4 and len(read_db(db)["rounds"]) == 1,
          f"configured special database did not retain state: {read_db(db)}")
    check([entry.name for entry in db.parent.iterdir()] == [db.name],
          f"SQLite created a file at an interpreted URI location: {[p.name for p in db.parent.iterdir()]}")

    output_before = snapshot(root)
    repeated = h.run(cfg)
    check(repeated.returncode == 0 and "skipped=4" in repeated.stdout,
          f"special database repeat failed: {repeated.returncode} {repeated.stdout}")
    check(snapshot(root) == output_before, "special database repeat changed output")
    db_before_dry = db.read_bytes()
    state_before_dry = read_db(db)
    dry = h.run(cfg, dry_run=True, extra_env={
        "TMPDIR": str(root),
        "STRMGEN_LOCK_DIR": str(root / "locks"),
    })
    check(dry.returncode == 0, f"special database dry-run failed: {dry.returncode} {dry.stderr}")
    check(db.read_bytes() == db_before_dry and read_db(db) == state_before_dry,
          "special database dry-run changed database bytes or rows")
    check(snapshot(root) == output_before, "special database dry-run changed output")
    check([entry.name for entry in db.parent.iterdir()] == [db.name],
          "special database path created an unexpected sidecar or alternate file")
    results["database_uri"] = {"db_name": db.name, "repeat_exit": repeated.returncode,
                               "dry_run_exit": dry.returncode, "ownership_rows": len(state_before_dry["ownership"])}


def scenario_multiprocess(h, results):
    barrier = threading.Event()
    entered = threading.Event()

    def before(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not entered.is_set():
            entered.set()
            barrier.wait(timeout=20)

    api, _ = h.server(hooks={"before": before})
    root1 = h.workdir / "first" / "out"
    db1 = h.workdir / "first" / "data" / "strmgen.db"
    cfg1 = h.config("first", api, [("/Movies", root1)], db1)
    proc1 = h.start(cfg1, extra_env={
        "STRMGEN_LOCK_DIR": str(root1 / "lock-override-one"),
        "TMPDIR": str(root1 / "tmp-one"),
    })
    check(entered.wait(timeout=15), "first process never reached the barrier")
    root_before = snapshot_tree(root1)

    # Same physical output, but a different database, TMPDIR and attempted
    # lock override must all contend for the same production lock namespace.
    db2 = h.workdir / "second.sqlite"
    same_root_cfg = h.config("same-root-second", api, [("/Movies", root1)], db2)
    second = h.run(same_root_cfg, timeout=30, extra_env={
        "STRMGEN_LOCK_DIR": str(root1 / "lock-override-two"),
        "TMPDIR": str(root1 / "tmp-two"),
    })
    check(second.returncode == 1, f"second process must be excluded: {second.returncode}")
    check("another sync is already running" in second.stderr, f"busy message missing: {second.stderr}")
    check(not db2.exists(), "excluded process created a database")
    check(snapshot_tree(root1) == root_before, "excluded process changed the shared output root")

    # A true ancestor and a true child output range are excluded too.
    ancestor = root1.parent
    ancestor_before = snapshot_tree(ancestor)
    cfg3 = h.config("ancestor", api, [("/Movies", ancestor)], h.workdir / "ancestor.sqlite")
    third = h.run(cfg3, timeout=30)
    check(third.returncode == 1, f"ancestor range must be excluded: {third.returncode}")
    check(snapshot_tree(ancestor) == ancestor_before, "ancestor-range process changed output entries")

    subdir = root1 / "nested"
    subdir_before = snapshot_tree(subdir)
    cfg4 = h.config("subdir", api, [("/Movies", subdir)], h.workdir / "subdir.sqlite")
    fourth = h.run(cfg4, timeout=30)
    check(fourth.returncode == 1, f"subdirectory range must be excluded: {fourth.returncode}")
    check(snapshot_tree(subdir) == subdir_before, "subdirectory-range process created output")

    alias_output = root1 / "nested" / ".."
    alias_cfg = h.config("alias", api, [("/Movies", alias_output)], h.workdir / "alias.sqlite")
    fifth = h.run(alias_cfg, timeout=30)
    check(fifth.returncode == 1, f"aliased range must be excluded: {fifth.returncode}")
    check(snapshot_tree(root1) == root_before, "aliased-range process changed shared output")

    # A symlink alias is not followed; it is a preflight safety rejection.
    link_alias = h.workdir / "link-alias"
    link_alias.symlink_to(root1, target_is_directory=True)
    alias_link_cfg = h.config("alias-link", api, [("/Movies", link_alias)], h.workdir / "alias-link.sqlite")
    sixth = h.run(alias_link_cfg, timeout=30)
    check(sixth.returncode == 2, f"symlink alias must be rejected before work: {sixth.returncode}")
    check(snapshot_tree(root1) == root_before, "symlink-alias rejection changed shared output")

    barrier.set()
    out1, err1 = proc1.communicate(timeout=60)
    check(proc1.returncode == 0, f"first process failed: {proc1.returncode} {err1}")
    check("created=4" in out1, f"first process summary: {out1}")

    # After normal exit a different output/database can take the shared lock.
    root2 = h.workdir / "after-release" / "out"
    cfg2 = h.config("after-release", api, [("/Movies", root2)], db2)
    again = h.run(cfg2, timeout=60)
    check(again.returncode == 0, f"rerun after release failed: {again.returncode} {again.stderr}")
    results["multiprocess"] = {"second_exit": second.returncode, "ancestor_exit": third.returncode,
                               "subdir_exit": fourth.returncode, "alias_exit": fifth.returncode,
                               "symlink_alias_exit": sixth.returncode,
                               "rerun_exit": again.returncode}


def scenario_fault_recovery(h, results):
    observed = {}

    # Directory creation failure: a regular file occupies a directory component.
    h1 = Harness(h.binary, h.workdir / "dirmkdir")
    api1, _ = h1.server()
    root1 = h1.workdir / "out"
    (root1).mkdir(parents=True, exist_ok=True)
    (root1 / "科幻").write_bytes(b"blocking-file")
    db1 = h1.workdir / "data" / "strmgen.db"
    cfg1 = h1.config("dirmkdir", api1, [("/Movies", root1)], db1)
    mkdir_fail = h1.run(cfg1)
    check(mkdir_fail.returncode == 1, f"directory failure must exit 1: {mkdir_fail.returncode}")
    check(not (root1 / "科幻" / "A #100%.strm").exists(), "half-created target present")
    check((root1 / "Sample.strm").exists(), "successful files before the failure should remain")
    check(len(read_db(db1)["rounds"]) == 0, "failed round was committed")
    observed["dir_failure"] = mkdir_fail.returncode
    h1.cleanup()

    # File published but ownership commit fails: file remains, not owned.
    h2 = Harness(h.binary, h.workdir / "ownerfail")
    api2, _ = h2.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root2 = h2.workdir / "out"
    db2 = h2.workdir / "data" / "strmgen.db"
    precreate_db_with_trigger(db2, "CREATE TRIGGER block_owner BEFORE INSERT ON ownership "
                                   "BEGIN SELECT RAISE(ABORT, 'injected'); END;")
    cfg2 = h2.config("ownerfail", api2, [("/Movies", root2)], db2)
    owner_fail = h2.run(cfg2)
    check(owner_fail.returncode == 1, f"ownership failure must exit 1: {owner_fail.returncode}")
    check((root2 / "A.strm").exists(), "published file was removed")
    state2 = read_db(db2)
    check(state2["ownership"] == [], f"unrecorded file was owned: {state2['ownership']}")
    check(state2["rounds"] == [], f"failed round was committed: {state2['rounds']}")

    # Restart after that failure must treat the orphan as an unowned conflict.
    conn = sqlite3.connect(str(db2))
    conn.execute("DROP TRIGGER block_owner")
    conn.commit()
    conn.close()
    restart = h2.run(cfg2)
    check(restart.returncode == 1, f"orphan restart must conflict: {restart.returncode}")
    check("conflicts=1" in restart.stdout, f"orphan restart summary: {restart.stdout}")
    check(read_db(db2)["ownership"] == [], "orphan was claimed on restart")
    observed["ownership_failure"] = owner_fail.returncode
    observed["orphan_restart"] = restart.returncode
    h2.cleanup()

    # Mid-traversal failure: no success round, no fabricated ownership.
    h3 = Harness(h.binary, h.workdir / "pagefail")
    calls = {"n": 0}

    def failing(endpoint, path, request, directories):
        if endpoint.endswith("/list") and path == "/Movies/科幻":
            return 503, 503, None
        return None

    api3, _ = h3.server(hooks={"override": failing})
    root3 = h3.workdir / "out"
    db3 = h3.workdir / "data" / "strmgen.db"
    cfg3 = h3.config("pagefail", api3, [("/Movies", root3)], db3)
    page_fail = h3.run(cfg3)
    check(page_fail.returncode == 1, f"pagination failure must exit 1: {page_fail.returncode}")
    check(len(read_db(db3)["rounds"]) == 0, "failed pagination committed a round")
    observed["pagination_failure"] = page_fail.returncode
    h3.cleanup()

    # Cancellation: SIGTERM while the round is held at the barrier.
    h4 = Harness(h.binary, h.workdir / "cancel")
    barrier = threading.Event()
    entered = threading.Event()

    def before(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies":
            entered.set()
            barrier.wait(timeout=20)

    api4, _ = h4.server(hooks={"before": before})
    root4 = h4.workdir / "out"
    db4 = h4.workdir / "data" / "strmgen.db"
    cfg4 = h4.config("cancel", api4, [("/Movies", root4)], db4)
    proc = h4.start(cfg4)
    check(entered.wait(timeout=15), "process never reached the barrier")
    proc.send_signal(signal.SIGTERM)
    try:
        proc.communicate(timeout=15)
    finally:
        barrier.set()
    check(proc.returncode != 0, "cancelled run must not exit 0")
    state4 = read_db(db4)
    check(state4["rounds"] == [], f"cancelled run committed a round: {state4['rounds']}")
    # After a terminated process the lock must be free for the next run.
    rerun = h4.run(cfg4, timeout=60)
    check(rerun.returncode == 0, f"rerun after termination failed: {rerun.returncode} {rerun.stderr}")
    check("created=4" in rerun.stdout, f"rerun summary: {rerun.stdout}")
    observed["cancel_exit"] = proc.returncode
    observed["rerun_after_kill"] = rerun.returncode
    h4.cleanup()

    # FIFO replacement of an owned target must be rejected without blocking;
    # the original special file and prior complete round remain untouched.
    h5 = Harness(h.binary, h.workdir / "owned-fifo")
    directories5 = {"/Movies": [{"name": "A.mkv", "is_dir": False, "size": 1}]}
    api5, _ = h5.server(directories=directories5)
    root5 = h5.workdir / "out"
    db5 = h5.workdir / "db.sqlite"
    cfg5 = h5.config("fifo", api5, [("/Movies", root5)], db5)
    check(h5.run(cfg5).returncode == 0, "FIFO seed sync failed")
    target5 = root5 / "A.strm"
    target5.unlink()
    os.mkfifo(target5, 0o600)
    fifo_before = target5.lstat()
    fifo_run = h5.run(cfg5, timeout=8)
    check(fifo_run.returncode == 1, f"owned FIFO must fail promptly: {fifo_run.returncode}")
    check("conflicts=1" in fifo_run.stdout and "partial=true" in fifo_run.stdout,
          f"owned FIFO was not reported as a conflict: {fifo_run.stdout}")
    fifo_after = target5.lstat()
    check(stat.S_ISFIFO(fifo_after.st_mode), "owned FIFO was replaced")
    check((fifo_after.st_ino, fifo_after.st_size, fifo_after.st_mtime_ns) ==
          (fifo_before.st_ino, fifo_before.st_size, fifo_before.st_mtime_ns),
          "owned FIFO metadata changed during the failed read")
    check(len(read_db(db5)["rounds"]) == 1 and read_db(db5)["rounds"][0][1] == 1,
          "owned FIFO failure advanced the round")
    directories5["/Movies"] = []
    fifo_lock_release = h5.run(cfg5, timeout=8)
    check(fifo_lock_release.returncode == 0, "lock was not released after FIFO failure")
    check(target5.lstat().st_ino == fifo_before.st_ino, "follow-up run changed the FIFO")
    observed["owned_fifo_exit"] = fifo_run.returncode
    observed["after_fifo_lock_release"] = fifo_lock_release.returncode
    h5.cleanup()

    # Oversized regular owned targets are bounded before parse and preserved.
    h6 = Harness(h.binary, h.workdir / "owned-oversized")
    directories6 = {"/Movies": [{"name": "A.mkv", "is_dir": False, "size": 1}]}
    api6, _ = h6.server(directories=directories6)
    root6 = h6.workdir / "out"
    db6 = h6.workdir / "db.sqlite"
    cfg6 = h6.config("oversized", api6, [("/Movies", root6)], db6)
    check(h6.run(cfg6).returncode == 0, "oversized seed sync failed")
    target6 = root6 / "A.strm"
    target6.write_bytes(b"x" * 65537)
    large_before = snapshot_tree(root6)
    large_run = h6.run(cfg6, timeout=8)
    check(large_run.returncode == 1, f"oversized owned target must fail: {large_run.returncode}")
    check(snapshot_tree(root6) == large_before, "oversized owned target was changed")
    check(read_db(db6)["rounds"][0][1] == 1, "oversized target failure advanced the round")
    directories6["/Movies"] = []
    large_lock_release = h6.run(cfg6, timeout=8)
    check(large_lock_release.returncode == 0, "lock was not released after oversized-target failure")
    check(snapshot_tree(root6) == large_before, "follow-up run changed oversized target")
    observed["oversized_target_exit"] = large_run.returncode
    observed["after_oversized_lock_release"] = large_lock_release.returncode
    h6.cleanup()

    # SIGKILL releases the kernel flock without unlinking/replacing the shared
    # lock file, and the next process can acquire it normally.
    h7 = Harness(h.binary, h.workdir / "sigkill")
    kill_barrier = threading.Event()
    kill_entered = threading.Event()

    def wait_for_kill(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not kill_entered.is_set():
            kill_entered.set()
            kill_barrier.wait(timeout=20)

    api7, _ = h7.server(hooks={"before": wait_for_kill})
    root7 = h7.workdir / "out"
    db7 = h7.workdir / "db.sqlite"
    cfg7 = h7.config("sigkill", api7, [("/Movies", root7)], db7)
    killed = h7.start(cfg7)
    check(kill_entered.wait(timeout=15), "SIGKILL process never reached the API barrier")
    killed.kill()
    killed.communicate(timeout=8)
    kill_barrier.set()
    check(killed.returncode == -signal.SIGKILL, f"expected SIGKILL exit, got {killed.returncode}")
    check(read_db(db7)["ownership"] == [] and read_db(db7)["rounds"] == [],
          "SIGKILL process left ownership or a complete round")
    after_kill = h7.run(cfg7, timeout=30)
    check(after_kill.returncode == 0, f"lock was not released after SIGKILL: {after_kill.returncode}")
    check("created=4" in after_kill.stdout, f"post-SIGKILL sync did not recover: {after_kill.stdout}")
    observed["sigkill_exit"] = killed.returncode
    observed["after_sigkill_lock_release"] = after_kill.returncode
    h7.cleanup()

    # Lower the CLI child's file-size soft limit after DB setup but before its
    # first STRM publish. With SIGXFSZ ignored, the real write returns EFBIG;
    # LocalFS must remove the temporary file, leave no target/ownership, and
    # allow a normal retry after the process releases the global flock.
    h8 = Harness(h.binary, h.workdir / "temporary-write-failure")
    write_barrier = threading.Event()
    write_entered = threading.Event()

    def hold_before_write(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not write_entered.is_set():
            write_entered.set()
            write_barrier.wait(timeout=20)

    api8, _ = h8.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None},
    ]}, hooks={"before": hold_before_write})
    root8 = h8.workdir / "out"
    db8 = h8.workdir / "data" / "strmgen.db"
    cfg8 = h8.config("write-failure", api8, [("/Movies", root8)], db8)
    write_proc = h8.start(cfg8, ignore_sigxfsz=True)
    check(write_entered.wait(timeout=15), "write-failure process never reached the API barrier")
    _, hard_limit = resource.prlimit(write_proc.pid, resource.RLIMIT_FSIZE)
    resource.prlimit(write_proc.pid, resource.RLIMIT_FSIZE, (0, hard_limit))
    write_barrier.set()
    try:
        write_out, write_err = write_proc.communicate(timeout=30)
    finally:
        write_barrier.set()
    check(write_proc.returncode == 1,
          f"temporary-file write failure should be a handled runtime error: {write_proc.returncode} {write_err}")
    check("errors=1" in write_out and "partial=true" in write_out,
          f"temporary-file write error was not counted: {write_out}")
    write_tree = snapshot_tree(root8)
    check(write_tree["exists"] and write_tree["entries"] == {},
          f"failed temporary write left a target or temp file: {write_tree}")
    write_state = read_db(db8)
    check(write_state["ownership"] == [] and write_state["rounds"] == [],
          f"failed temporary write registered ownership or a round: {write_state}")
    write_retry = h8.run(cfg8, timeout=30)
    check(write_retry.returncode == 0 and len(snapshot(root8)) == 1,
          f"normal retry after write failure failed: {write_retry.returncode} {write_retry.stderr}")
    observed["temporary_write_failure"] = write_proc.returncode
    observed["write_failure_retry"] = write_retry.returncode
    h8.cleanup()

    # Test-only build-tagged CLI injects a foreign target after the no-follow
    # check and just before Linkat. The ordinary binary is still used for all
    # other scenarios; without --fault-binary the fallback adds the same
    # unowned target at the API barrier and checks preservation.
    race_binary = h.fault_binary or h.binary
    h9 = Harness(race_binary, h.workdir / "target-appears-before-publish")
    race_root = h9.workdir / "out"
    race_db = h9.workdir / "data" / "strmgen.db"
    race_ready = threading.Event()

    def add_target_at_barrier(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not race_ready.is_set():
            race_ready.set()
            if h.fault_binary is None:
                race_root.mkdir(parents=True, exist_ok=True)
                (race_root / "A.strm").write_bytes(b"external")

    api9, _ = h9.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None},
    ]}, hooks={"before": add_target_at_barrier})
    race_cfg = h9.config("target-race", api9, [("/Movies", race_root)], race_db)
    race_env = {"STRMGEN_TEST_PUBLISH_BEFORE_LINK_TARGET": "A.strm"} if h.fault_binary is not None else None
    race_result = h9.run(race_cfg, extra_env=race_env)
    check(race_ready.is_set(), "target-race fixture did not reach its barrier")
    check(race_result.returncode == 1 and "errors=1" in race_result.stdout,
          f"late target should fail safely: {race_result.returncode} {race_result.stdout} {race_result.stderr}")
    check((race_root / "A.strm").read_bytes() == b"external", "late external target was overwritten")
    race_tree = snapshot_tree(race_root)
    check(set(race_tree["entries"]) == {"A.strm"}, f"late-target publish left a temporary file: {race_tree}")
    race_state = read_db(race_db)
    check(race_state["ownership"] == [] and race_state["rounds"] == [],
          f"late target acquired ownership or a round: {race_state}")
    observed["target_appears_after_check"] = race_result.returncode
    h9.cleanup()

    # Cancel after page 1 has been accepted but while page 2 is blocked. The
    # client has not yet handed an incomplete list to the reconciler, so no
    # output/ownership/round may be committed; the same source then retries.
    h10 = Harness(h.binary, h.workdir / "page2-cancellation")
    page_barrier = threading.Event()
    page_entered = threading.Event()

    def hold_second_page(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and request.get("page") == 2 and not page_entered.is_set():
            page_entered.set()
            page_barrier.wait(timeout=20)

    page_data = [{"name": f"P{i:03d}.mkv", "is_dir": False, "size": i, "hash_info": None}
                 for i in range(250)]
    api10, _ = h10.server(directories={"/Movies": page_data}, hooks={"before": hold_second_page})
    root10 = h10.workdir / "out"
    db10 = h10.workdir / "data" / "strmgen.db"
    cfg10 = h10.config("page2-cancel", api10, [("/Movies", root10)], db10)
    page_proc = h10.start(cfg10)
    check(page_entered.wait(timeout=15), "page-two cancellation never reached the barrier")
    page_proc.send_signal(signal.SIGTERM)
    try:
        page_out, page_err = page_proc.communicate(timeout=15)
    finally:
        page_barrier.set()
    check(page_proc.returncode != 0, f"page-two cancellation must fail: {page_proc.returncode}")
    check(not root10.exists(), "cancelled incomplete page created the output root")
    page_state = read_db(db10)
    check(page_state["ownership"] == [] and page_state["rounds"] == [],
          f"page-two cancellation created ownership or a successful round: {page_state}")
    page_retry = h10.run(cfg10, timeout=60)
    check(page_retry.returncode == 0 and len(snapshot(root10)) == 250,
          f"retry after page-two cancellation failed: {page_retry.returncode} {page_retry.stderr}")
    observed["page2_cancel_exit"] = page_proc.returncode
    observed["page2_cancel_retry"] = page_retry.returncode
    h10.cleanup()

    # F-9: interruption at exactly the window after a durable ownership insert
    # and before the complete round is committed. The testhooks-only barrier
    # stops the process itself immediately after RecordOwnership returns, so
    # this no longer depends on sampling SIGSTOP between two publications.
    # The release binary runs the restart, proving no testhook is needed to
    # recover and that owned files are skipped rather than blindly re-claimed.
    if h.fault_binary is None:
        raise Failure("ownership-interruption recovery requires --fault-binary (strmgen_testhooks build)")
    h11 = Harness(h.binary, h.workdir / "ownership-before-round-interruption")
    ownership_data = [{"name": f"O{i:05d}.mkv", "is_dir": False, "size": i, "hash_info": None}
                      for i in range(2000)]
    api11, _ = h11.server(directories={"/Movies": ownership_data})
    root11 = h11.workdir / "out"
    db11 = h11.workdir / "data" / "strmgen.db"
    marker11 = h11.workdir / "ownership-held.marker"
    cfg11 = h11.config("ownership-interrupt", api11, [("/Movies", root11)], db11, workers=1)
    ownership_proc = h11.start(cfg11, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_OWNERSHIP_HOLD": "O00000.strm",
        "STRMGEN_TEST_OWNERSHIP_MARKER": str(marker11),
    })
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline and not marker11.exists():
        if ownership_proc.poll() is not None:
            break
        time.sleep(0.005)
    check(marker11.exists(), "testhooks barrier never recorded a durable ownership insert")
    check(marker11.read_bytes() == b"O00000.strm", f"unexpected barrier marker: {marker11.read_bytes()!r}")
    # The barrier writes the marker and immediately stops the process; confirm
    # the observable stopped state before inspecting the database.
    stopped_state = None
    stop_deadline = time.monotonic() + 10
    while time.monotonic() < stop_deadline:
        if ownership_proc.poll() is not None:
            break
        try:
            status = Path(f"/proc/{ownership_proc.pid}/status").read_text()
        except FileNotFoundError:
            break
        state_line = next((line for line in status.splitlines() if line.startswith("State:")), "")
        fields = state_line.split()
        if len(fields) > 1 and fields[1] in ("T", "t"):
            stopped_state = fields[1]
            break
        time.sleep(0.002)
    check(stopped_state in ("T", "t"),
          f"ownership barrier did not stop the process: {stopped_state}")
    interrupted_state = read_db(db11)
    check(len(interrupted_state["ownership"]) == 1 and interrupted_state["rounds"] == [],
          f"barrier did not isolate one durable ownership from a complete round: {interrupted_state}")
    interrupted_strms = list(root11.rglob("*.strm"))
    check([p.name for p in interrupted_strms] == ["O00000.strm"],
          f"barrier published an unexpected set of targets: {[p.name for p in interrupted_strms]}")
    ownership_proc.kill()
    ownership_proc.communicate(timeout=10)
    check(ownership_proc.returncode == -signal.SIGKILL,
          f"ownership interruption expected SIGKILL, got {ownership_proc.returncode}")
    killed_state = read_db(db11)
    check(len(killed_state["ownership"]) == 1 and killed_state["rounds"] == [],
          f"SIGKILL changed partial ownership/round state: {killed_state}")
    # Restart with the release binary: owned file is skipped, the remaining
    # files are created, and the original incomplete epoch becomes complete.
    ownership_retry = h11.run(cfg11, timeout=120)
    check(ownership_retry.returncode == 0,
          f"restart after ownership interruption failed: {ownership_retry.returncode} {ownership_retry.stderr}")
    check("skipped=1" in ownership_retry.stdout and "created=1999" in ownership_retry.stdout,
          f"restart did not skip the owned file: {ownership_retry.stdout}")
    recovered_state = read_db(db11)
    check(len(recovered_state["ownership"]) == len(ownership_data) and
          recovered_state["rounds"] and recovered_state["rounds"][0][1] == 1,
          f"restart did not finish the original incomplete epoch: {recovered_state}")
    recovered_strms = list(root11.rglob("*.strm"))
    check(len(recovered_strms) == len(ownership_data),
          "restart after ownership interruption lost STRM targets")
    leftover_temps = [path.name for path in root11.iterdir() if path.name.startswith(".strmgen-")]
    check(all(name.endswith(".tmp") for name in leftover_temps),
          f"unexpected interruption residue in output: {leftover_temps}")
    observed["ownership_then_sigkill"] = ownership_proc.returncode
    observed["ownership_interruption_recovery"] = ownership_retry.returncode
    observed["unreferenced_temp_files_after_sigkill"] = len(leftover_temps)
    h11.cleanup()

    # Update temporary-write failure on an existing owned target: the old
    # target must survive byte-for-byte, ownership and round must not advance,
    # no temporary file may remain, and a retry must complete the update. The
    # same API server and identity are reused; only public_url (excluded from
    # identity) changes so a real update is required.
    h12 = Harness(h.binary, h.workdir / "update-write-failure")
    update_barrier = threading.Event()
    update_entered = threading.Event()
    update_hold = {"on": False}
    update_dirs = {"/Movies": [{"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]}

    def hold_update_write(endpoint, path, request):
        if update_hold["on"] and endpoint.endswith("/list") and path == "/Movies" and not update_entered.is_set():
            update_entered.set()
            update_barrier.wait(timeout=20)

    api12, _ = h12.server(directories=update_dirs, hooks={"before": hold_update_write})
    root12 = h12.workdir / "out"
    db12 = h12.workdir / "data" / "strmgen.db"
    cfg12 = h12.config("update-write", api12, [("/Movies", root12)], db12)
    check(h12.run(cfg12).returncode == 0, "update-write seed sync failed")
    update_target = root12 / "A.strm"
    update_before = update_target.read_bytes()
    # Change the public prefix so a real update is required on the next run.
    cfg12b = h12.config("update-write-b", api12, [("/Movies", root12)], db12,
                        public="https://changed.example.invalid")

    update_hold["on"] = True
    update_proc = h12.start(cfg12b, ignore_sigxfsz=True)
    check(update_entered.wait(timeout=15), "update-write process never reached the API barrier")
    _, hard12 = resource.prlimit(update_proc.pid, resource.RLIMIT_FSIZE)
    resource.prlimit(update_proc.pid, resource.RLIMIT_FSIZE, (0, hard12))
    update_barrier.set()
    try:
        update_out, update_err = update_proc.communicate(timeout=30)
    finally:
        update_barrier.set()
    check(update_proc.returncode == 1,
          f"update write failure should be a handled error: {update_proc.returncode} {update_err}")
    check("errors=1" in update_out and "partial=true" in update_out,
          f"update write error was not counted: {update_out}")
    check(update_target.read_bytes() == update_before, "failed update changed the old target")
    update_tree12 = snapshot_tree(root12)
    check(set(update_tree12["entries"]) == {"A.strm"},
          f"failed update left a temporary file: {update_tree12}")
    update_state12 = read_db(db12)
    check(len(update_state12["ownership"]) == 1 and update_state12["rounds"][0][1] == 1,
          f"failed update changed ownership or advanced the round: {update_state12}")
    update_retry = h12.run(cfg12b, timeout=30)
    check(update_retry.returncode == 0 and "updated=1" in update_retry.stdout,
          f"retry after update write failure failed: {update_retry.returncode} {update_retry.stdout}")
    check("https://changed.example.invalid/d/Movies/A.mkv" in update_target.read_text(),
          "retry did not persist the updated URL")
    observed["update_write_failure"] = update_proc.returncode
    observed["update_write_failure_retry"] = update_retry.returncode
    h12.cleanup()

    # Update interrupted after the atomic exchange but before the round is
    # committed. The new bytes are already durable and ownership still maps to
    # the target; the release binary must restart, skip the owned file and
    # commit the original incomplete epoch without reclaiming anything.
    if h.fault_binary is None:
        raise Failure("update-interruption recovery requires --fault-binary (strmgen_testhooks build)")
    h13 = Harness(h.binary, h.workdir / "update-before-round-interruption")
    update_dirs13 = {"/Movies": [{"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]}
    api13, _ = h13.server(directories=update_dirs13)
    root13 = h13.workdir / "out"
    db13 = h13.workdir / "data" / "strmgen.db"
    cfg13 = h13.config("update-interrupt", api13, [("/Movies", root13)], db13)
    check(h13.run(cfg13).returncode == 0, "update-interrupt seed sync failed")
    cfg13b = h13.config("update-interrupt-b", api13, [("/Movies", root13)], db13,
                        public="https://changed.example.invalid")
    marker13 = h13.workdir / "update-held.marker"
    interrupted_proc = h13.start(cfg13b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_HOLD": "A.strm",
        "STRMGEN_TEST_UPDATE_MARKER": str(marker13),
    })
    deadline13 = time.monotonic() + 60
    while time.monotonic() < deadline13 and not marker13.exists():
        if interrupted_proc.poll() is not None:
            break
        time.sleep(0.005)
    check(marker13.exists(), "update barrier never recorded a durable exchange")
    stopped13 = None
    stop_deadline13 = time.monotonic() + 10
    while time.monotonic() < stop_deadline13:
        if interrupted_proc.poll() is not None:
            break
        try:
            status13 = Path(f"/proc/{interrupted_proc.pid}/status").read_text()
        except FileNotFoundError:
            break
        state_line13 = next((line for line in status13.splitlines() if line.startswith("State:")), "")
        fields13 = state_line13.split()
        if len(fields13) > 1 and fields13[1] in ("T", "t"):
            stopped13 = fields13[1]
            break
        time.sleep(0.002)
    check(stopped13 in ("T", "t"), f"update barrier did not stop the process: {stopped13}")
    check("https://changed.example.invalid/d/Movies/A.mkv" in (root13 / "A.strm").read_text(),
          "interrupted update did not persist the new bytes")
    interrupted13 = read_db(db13)
    check(len(interrupted13["ownership"]) == 1 and interrupted13["rounds"][0][1] == 1,
          f"interrupted update changed ownership or advanced the round: {interrupted13}")
    interrupted_proc.kill()
    interrupted_proc.communicate(timeout=10)
    check(interrupted_proc.returncode == -signal.SIGKILL,
          f"update interruption expected SIGKILL, got {interrupted_proc.returncode}")
    update_retry13 = h13.run(cfg13b, timeout=60)
    check(update_retry13.returncode == 0 and "skipped=1" in update_retry13.stdout,
          f"restart after update interruption did not skip the owned file: {update_retry13.returncode} {update_retry13.stdout}")
    recovered13 = read_db(db13)
    check(len(recovered13["ownership"]) == 1 and recovered13["rounds"] and recovered13["rounds"][0][1] == 2,
          f"restart did not complete the update epoch: {recovered13}")
    check(set(snapshot_tree(root13)["entries"]) == {"A.strm"},
          "update interruption left residue in the output")
    observed["update_then_sigkill"] = interrupted_proc.returncode
    observed["update_interruption_recovery"] = update_retry13.returncode
    h13.cleanup()

    # Update publish race: the owned target is replaced by an external actor
    # after the content read and immediately before the atomic exchange. The
    # testhooks-only build injects the foreign bytes; the exchange must be
    # undone and the external content preserved, with no ownership/round
    # change and no temporary residue.
    if h.fault_binary is None:
        raise Failure("update-target race requires --fault-binary (strmgen_testhooks build)")
    h14 = Harness(h.fault_binary, h.workdir / "update-target-race")
    race_dirs14 = {"/Movies": [{"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]}
    api14, _ = h14.server(directories=race_dirs14)
    root14 = h14.workdir / "out"
    db14 = h14.workdir / "data" / "strmgen.db"
    cfg14 = h14.config("update-race", api14, [("/Movies", root14)], db14)
    check(h14.run(cfg14).returncode == 0, "update-race seed sync failed")
    cfg14b = h14.config("update-race-b", api14, [("/Movies", root14)], db14,
                        public="https://changed.example.invalid")
    race_result14 = h14.run(cfg14b, extra_env={
        "STRMGEN_TEST_UPDATE_BEFORE_EXCHANGE_TARGET": "A.strm",
    })
    check(race_result14.returncode == 1 and "errors=1" in race_result14.stdout,
          f"update race should fail safely: {race_result14.returncode} {race_result14.stdout} {race_result14.stderr}")
    check((root14 / "A.strm").read_bytes() == b"external", "racing update overwrote external bytes")
    check(set(snapshot_tree(root14)["entries"]) == {"A.strm"},
          "racing update left a temporary file")
    race_state14 = read_db(db14)
    check(len(race_state14["ownership"]) == 1 and race_state14["rounds"][0][1] == 1,
          f"racing update changed ownership or advanced the round: {race_state14}")
    observed["update_target_race_exit"] = race_result14.returncode
    h14.cleanup()

    # F-11: interruption after the atomic exchange but before the swapped-out
    # content is verified. The next release-binary run must reconcile the
    # staging residual instead of silently skipping and committing a round.
    if h.fault_binary is None:
        raise Failure("update verification interruption requires --fault-binary (strmgen_testhooks build)")
    h15 = Harness(h.binary, h.workdir / "update-before-verification-interruption")
    api15, _ = h15.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root15 = h15.workdir / "out"
    db15 = h15.workdir / "data" / "strmgen.db"
    cfg15 = h15.config("update-verify-interrupt", api15, [("/Movies", root15)], db15)
    check(h15.run(cfg15).returncode == 0, "update-verify-interrupt seed sync failed")
    cfg15b = h15.config("update-verify-interrupt-b", api15, [("/Movies", root15)], db15,
                        public="https://changed.example.invalid")
    marker15 = h15.workdir / "exchange.marker"
    gate15 = h15.workdir / "exchange.gate"
    proc15 = h15.start(cfg15b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_EXCHANGE_GATE": str(gate15),
        "STRMGEN_TEST_UPDATE_EXCHANGE_MARKER": str(marker15),
    })
    check(wait_for_marker(proc15, marker15), "exchange barrier never recorded a durable exchange")
    check(proc15.poll() is None, "exchange barrier process exited early")
    check("https://changed.example.invalid" in (root15 / "A.strm").read_text(),
          "interrupted exchange did not persist the new bytes")
    interrupted15 = read_db(db15)
    check(len(interrupted15["ownership"]) == 1 and interrupted15["rounds"][0][1] == 1,
          f"interrupted exchange changed ownership or advanced the round: {interrupted15}")
    check(update_staging_names(root15), "interrupted exchange left no staging residual")
    proc15.kill()
    proc15.communicate(timeout=10)
    check(proc15.returncode == -signal.SIGKILL,
          f"update verification interruption expected SIGKILL, got {proc15.returncode}")
    retry15 = h15.run(cfg15b, timeout=60)
    check(retry15.returncode == 0 and "updated=1" in retry15.stdout,
          f"restart after verification interruption did not redo the update: {retry15.returncode} {retry15.stdout}")
    check(read_db(db15)["rounds"][0][1] == 2, "restart did not commit the update round")
    check(not update_staging_names(root15), "restart left update staging residue")
    observed["update_before_verification_sigkill"] = proc15.returncode
    observed["update_before_verification_recovery"] = retry15.returncode
    h15.cleanup()

    # F-11 foreign variant: a foreign target displaced by the interrupted
    # exchange must be restored and reported as a conflict, never silently
    # skipped or deleted.
    h16 = Harness(h.binary, h.workdir / "update-verification-foreign-interruption")
    api16, _ = h16.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root16 = h16.workdir / "out"
    db16 = h16.workdir / "data" / "strmgen.db"
    cfg16 = h16.config("update-verify-foreign", api16, [("/Movies", root16)], db16)
    check(h16.run(cfg16).returncode == 0, "update-verify-foreign seed sync failed")
    cfg16b = h16.config("update-verify-foreign-b", api16, [("/Movies", root16)], db16,
                        public="https://changed.example.invalid")
    marker16 = h16.workdir / "exchange.marker"
    gate16 = h16.workdir / "exchange.gate"
    proc16 = h16.start(cfg16b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_BEFORE_EXCHANGE_TARGET": "A.strm",
        "STRMGEN_TEST_UPDATE_EXCHANGE_GATE": str(gate16),
        "STRMGEN_TEST_UPDATE_EXCHANGE_MARKER": str(marker16),
    })
    check(wait_for_marker(proc16, marker16), "foreign exchange barrier never recorded a durable exchange")
    check(proc16.poll() is None, "foreign exchange barrier process exited early")
    displaced16 = {name: (root16 / name).read_text() for name in update_staging_names(root16)}
    check(list(displaced16.values()) == ["external"],
          f"foreign target was not displaced into staging: {displaced16}")
    proc16.kill()
    proc16.communicate(timeout=10)
    check(proc16.returncode == -signal.SIGKILL,
          f"foreign verification interruption expected SIGKILL, got {proc16.returncode}")
    retry16 = h16.run(cfg16b, timeout=60)
    check(retry16.returncode == 1 and "conflicts=1" in retry16.stdout,
          f"foreign residual must be a conservative conflict: {retry16.returncode} {retry16.stdout}")
    check((root16 / "A.strm").read_bytes() == b"external", "foreign bytes were not preserved")
    check(not update_staging_names(root16), "foreign recovery left staging residue")
    check(read_db(db16)["rounds"][0][1] == 1, "foreign residual committed a round")
    observed["update_before_verification_foreign"] = retry16.returncode
    h16.cleanup()

    # F-11 valid variant: a valid but foreign version-1 STRM displaced by the
    # interrupted exchange must be restored and reported as a conflict. Content
    # validity, a matching path and an existing ownership row must never be
    # enough to reclaim it.
    h16b = Harness(h.binary, h.workdir / "update-verification-valid-foreign-interruption")
    api16b, _ = h16b.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root16b = h16b.workdir / "out"
    db16b = h16b.workdir / "data" / "strmgen.db"
    cfg16c = h16b.config("update-verify-valid-foreign", api16b, [("/Movies", root16b)], db16b)
    check(h16b.run(cfg16c).returncode == 0, "update-verify-valid-foreign seed sync failed")
    cfg16d = h16b.config("update-verify-valid-foreign-b", api16b, [("/Movies", root16b)], db16b,
                         public="https://changed.example.invalid")
    pre_gate16 = h16b.workdir / "pre.gate"
    pre_marker16 = h16b.workdir / "pre.marker"
    post_gate16 = h16b.workdir / "post.gate"
    post_marker16 = h16b.workdir / "post.marker"
    proc16b = h16b.start(cfg16d, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE": str(pre_gate16),
        "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_MARKER": str(pre_marker16),
        "STRMGEN_TEST_UPDATE_EXCHANGE_GATE": str(post_gate16),
        "STRMGEN_TEST_UPDATE_EXCHANGE_MARKER": str(post_marker16),
    })
    check(wait_for_marker(proc16b, pre_marker16), "valid-foreign pre barrier never recorded")
    check(proc16b.poll() is None, "valid-foreign pre barrier process exited early")
    (root16b / "A.strm").write_bytes(FOREIGN_VALID_STRM)
    pre_gate16.touch()
    check(wait_for_marker(proc16b, post_marker16),
          "valid-foreign exchange barrier never recorded a durable exchange")
    check(proc16b.poll() is None, "valid-foreign exchange barrier process exited early")
    proc16b.kill()
    proc16b.communicate(timeout=10)
    check(proc16b.returncode == -signal.SIGKILL,
          f"valid-foreign verification interruption expected SIGKILL, got {proc16b.returncode}")
    retry16b = h16b.run(cfg16d, timeout=60)
    check(retry16b.returncode == 1 and "conflicts=1" in retry16b.stdout,
          f"valid foreign residual must be a conservative conflict: {retry16b.returncode} {retry16b.stdout}")
    check((root16b / "A.strm").read_bytes() == FOREIGN_VALID_STRM,
          "valid foreign bytes were not preserved")
    check(not update_staging_names(root16b), "valid foreign recovery left staging residue")
    check(read_db(db16b)["rounds"][0][1] == 1, "valid foreign residual committed a round")
    observed["update_before_verification_valid_foreign"] = retry16b.returncode
    h16b.cleanup()

    # SIGKILL before the exchange: the old target is untouched and the next run
    # simply performs the update.
    h17 = Harness(h.binary, h.workdir / "update-pre-exchange-interruption")
    api17, _ = h17.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root17 = h17.workdir / "out"
    db17 = h17.workdir / "data" / "strmgen.db"
    cfg17 = h17.config("update-pre-exchange", api17, [("/Movies", root17)], db17)
    check(h17.run(cfg17).returncode == 0, "update-pre-exchange seed sync failed")
    cfg17b = h17.config("update-pre-exchange-b", api17, [("/Movies", root17)], db17,
                        public="https://changed.example.invalid")
    marker17 = h17.workdir / "exchange.marker"
    gate17 = h17.workdir / "exchange.gate"
    proc17 = h17.start(cfg17b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE": str(gate17),
        "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_MARKER": str(marker17),
    })
    check(wait_for_marker(proc17, marker17), "pre-exchange barrier never recorded")
    check(proc17.poll() is None, "pre-exchange barrier process exited early")
    check("changed.example.invalid" not in (root17 / "A.strm").read_text(),
          "pre-exchange interruption published new bytes")
    pre_staging = {name: (root17 / name).read_text() for name in update_staging_names(root17)}
    check(len(pre_staging) == 1 and "changed.example.invalid" in next(iter(pre_staging.values())),
          f"pre-exchange interruption did not leave the staged new content: {pre_staging}")
    proc17.kill()
    proc17.communicate(timeout=10)
    check(proc17.returncode == -signal.SIGKILL,
          f"pre-exchange interruption expected SIGKILL, got {proc17.returncode}")
    retry17 = h17.run(cfg17b, timeout=60)
    check(retry17.returncode == 0 and "updated=1" in retry17.stdout,
          f"restart after pre-exchange interruption failed: {retry17.returncode} {retry17.stdout}")
    check(read_db(db17)["rounds"][0][1] == 2, "restart did not commit the update round")
    observed["update_pre_exchange_sigkill"] = proc17.returncode
    observed["update_pre_exchange_recovery"] = retry17.returncode
    h17.cleanup()

    # Changed desired after an exchange interruption: the same interrupted
    # state as h15, but the release-binary restart uses a different public URL.
    # The residual must be reconciled and the target must converge to the
    # newest URL instead of stalling on a permanent conflict.
    if h.fault_binary is None:
        raise Failure("changed-desired recovery requires --fault-binary (strmgen_testhooks build)")
    h18 = Harness(h.binary, h.workdir / "update-changed-desired-interruption")
    api18, _ = h18.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root18 = h18.workdir / "out"
    db18 = h18.workdir / "data" / "strmgen.db"
    cfg18 = h18.config("update-changed", api18, [("/Movies", root18)], db18)
    check(h18.run(cfg18).returncode == 0, "update-changed seed sync failed")
    cfg18b = h18.config("update-changed-b", api18, [("/Movies", root18)], db18,
                        public="https://changed.example.invalid")
    cfg18c = h18.config("update-changed-c", api18, [("/Movies", root18)], db18,
                        public="https://second.example.invalid")
    marker18 = h18.workdir / "exchange.marker"
    gate18 = h18.workdir / "exchange.gate"
    proc18 = h18.start(cfg18b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_UPDATE_EXCHANGE_GATE": str(gate18),
        "STRMGEN_TEST_UPDATE_EXCHANGE_MARKER": str(marker18),
    })
    check(wait_for_marker(proc18, marker18), "changed-desired exchange barrier never recorded")
    check(proc18.poll() is None, "changed-desired exchange barrier process exited early")
    check("changed.example.invalid" in (root18 / "A.strm").read_text(),
          "changed-desired interruption did not persist the intermediate bytes")
    check(update_staging_names(root18), "changed-desired interruption left no staging residual")
    proc18.kill()
    proc18.communicate(timeout=10)
    check(proc18.returncode == -signal.SIGKILL,
          f"changed-desired interruption expected SIGKILL, got {proc18.returncode}")
    retry18 = h18.run(cfg18c, timeout=60)
    check(retry18.returncode == 0 and "updated=1" in retry18.stdout,
          f"restart with changed desired did not converge: {retry18.returncode} {retry18.stdout} {retry18.stderr}")
    check("https://second.example.invalid/d/Movies/A.mkv" in (root18 / "A.strm").read_text(),
          "restart with changed desired did not persist the newest URL")
    check(not update_staging_names(root18), "changed-desired recovery left staging residue")
    check(read_db(db18)["rounds"][0][1] == 2, "changed-desired recovery did not commit the update round")
    stable18 = h18.run(cfg18c, timeout=60)
    check(stable18.returncode == 0 and "skipped=1" in stable18.stdout,
          f"stable rerun after changed-desired recovery did not skip: {stable18.returncode} {stable18.stdout}")
    observed["update_changed_desired_sigkill"] = proc18.returncode
    observed["update_changed_desired_recovery"] = retry18.returncode
    observed["update_changed_desired_stable"] = stable18.returncode
    h18.cleanup()

    # F-15: SIGKILL after Update created its deterministic staging file but
    # before any byte was written. This is the zero-length residual window; the
    # release binary must reclaim it and converge to the newest desired URL, and
    # a stable rerun must then skip.
    if h.fault_binary is None:
        raise Failure("staging-created recovery requires --fault-binary (strmgen_testhooks build)")
    h19 = Harness(h.binary, h.workdir / "update-staging-created-interruption")
    api19, _ = h19.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root19 = h19.workdir / "out"
    db19 = h19.workdir / "data" / "strmgen.db"
    cfg19 = h19.config("update-staging-created", api19, [("/Movies", root19)], db19)
    check(h19.run(cfg19).returncode == 0, "update-staging-created seed sync failed")
    cfg19b = h19.config("update-staging-created-b", api19, [("/Movies", root19)], db19,
                        public="https://changed.example.invalid")
    marker19 = h19.workdir / "staging-created.marker"
    gate19 = h19.workdir / "staging-created.gate"
    proc19 = h19.start(cfg19b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_STAGING_BEFORE_WRITE_GATE": str(gate19),
        "STRMGEN_TEST_STAGING_BEFORE_WRITE_MARKER": str(marker19),
    })
    check(wait_for_marker(proc19, marker19), "staging-created barrier never recorded")
    check(proc19.poll() is None, "staging-created barrier process exited early")
    check("changed.example.invalid" not in (root19 / "A.strm").read_text(),
          "staging-created interruption changed the target")
    created_residuals = update_staging_names(root19)
    check(len(created_residuals) == 1 and (root19 / created_residuals[0]).stat().st_size == 0,
          f"expected one zero-length staging residual: {created_residuals}")
    interrupted19 = read_db(db19)
    check(len(interrupted19["ownership"]) == 1 and interrupted19["rounds"][0][1] == 1,
          f"staging-created interruption changed ownership or advanced the round: {interrupted19}")
    proc19.kill()
    proc19.communicate(timeout=10)
    check(proc19.returncode == -signal.SIGKILL,
          f"staging-created interruption expected SIGKILL, got {proc19.returncode}")
    retry19 = h19.run(cfg19b, timeout=60)
    check(retry19.returncode == 0 and "updated=1" in retry19.stdout,
          f"restart after staging-created interruption failed: {retry19.returncode} {retry19.stdout} {retry19.stderr}")
    check("https://changed.example.invalid/d/Movies/A.mkv" in (root19 / "A.strm").read_text(),
          "restart after staging-created interruption did not persist the newest URL")
    check(not update_staging_names(root19), "staging-created recovery left staging residue")
    check(read_db(db19)["rounds"][0][1] == 2, "staging-created recovery did not commit the update round")
    stable19 = h19.run(cfg19b, timeout=60)
    check(stable19.returncode == 0 and "skipped=1" in stable19.stdout,
          f"stable rerun after staging-created recovery did not skip: {stable19.returncode} {stable19.stdout}")
    observed["update_staging_created_sigkill"] = proc19.returncode
    observed["update_staging_created_recovery"] = retry19.returncode
    observed["update_staging_created_stable"] = stable19.returncode
    h19.cleanup()

    # F-15: SIGKILL after a partial staging write. The truncated residual has no
    # complete hash, so this is the exact case the previous implementation
    # stalled on; the release binary must reclaim it and converge.
    if h.fault_binary is None:
        raise Failure("partial-staging recovery requires --fault-binary (strmgen_testhooks build)")
    h20 = Harness(h.binary, h.workdir / "update-partial-staging-interruption")
    api20, _ = h20.server(directories={"/Movies": [
        {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
    root20 = h20.workdir / "out"
    db20 = h20.workdir / "data" / "strmgen.db"
    cfg20 = h20.config("update-partial-staging", api20, [("/Movies", root20)], db20)
    check(h20.run(cfg20).returncode == 0, "update-partial-staging seed sync failed")
    cfg20b = h20.config("update-partial-staging-b", api20, [("/Movies", root20)], db20,
                        public="https://changed.example.invalid")
    marker20 = h20.workdir / "partial-staging.marker"
    gate20 = h20.workdir / "partial-staging.gate"
    proc20 = h20.start(cfg20b, binary=h.fault_binary, extra_env={
        "STRMGEN_TEST_STAGING_PARTIAL_GATE": str(gate20),
        "STRMGEN_TEST_STAGING_PARTIAL_MARKER": str(marker20),
        "STRMGEN_TEST_STAGING_PARTIAL_BYTES": "17",
    })
    check(wait_for_marker(proc20, marker20), "partial-staging barrier never recorded")
    check(proc20.poll() is None, "partial-staging barrier process exited early")
    check("changed.example.invalid" not in (root20 / "A.strm").read_text(),
          "partial-staging interruption changed the target")
    partial_residuals = update_staging_names(root20)
    check(len(partial_residuals) == 1 and (root20 / partial_residuals[0]).stat().st_size == 17,
          f"expected one 17-byte staging residual: {partial_residuals}")
    interrupted20 = read_db(db20)
    check(len(interrupted20["ownership"]) == 1 and interrupted20["rounds"][0][1] == 1,
          f"partial-staging interruption changed ownership or advanced the round: {interrupted20}")
    proc20.kill()
    proc20.communicate(timeout=10)
    check(proc20.returncode == -signal.SIGKILL,
          f"partial-staging interruption expected SIGKILL, got {proc20.returncode}")
    retry20 = h20.run(cfg20b, timeout=60)
    check(retry20.returncode == 0 and "updated=1" in retry20.stdout,
          f"restart after partial-staging interruption failed: {retry20.returncode} {retry20.stdout} {retry20.stderr}")
    check("https://changed.example.invalid/d/Movies/A.mkv" in (root20 / "A.strm").read_text(),
          "restart after partial-staging interruption did not persist the newest URL")
    check(not update_staging_names(root20), "partial-staging recovery left staging residue")
    check(read_db(db20)["rounds"][0][1] == 2, "partial-staging recovery did not commit the update round")
    stable20 = h20.run(cfg20b, timeout=60)
    check(stable20.returncode == 0 and "skipped=1" in stable20.stdout,
          f"stable rerun after partial-staging recovery did not skip: {stable20.returncode} {stable20.stdout}")
    observed["update_partial_staging_sigkill"] = proc20.returncode
    observed["update_partial_staging_recovery"] = retry20.returncode
    observed["update_partial_staging_stable"] = stable20.returncode
    h20.cleanup()

    # SIGTERM at each update window: the gate keeps the process runnable so
    # context cancellation is processed; the round must not commit and the
    # release binary must finish the update on restart.
    for case_name, gate_env, marker_env, window in (
            ("pre-exchange", "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE",
             "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_MARKER", "pre"),
            ("after-exchange", "STRMGEN_TEST_UPDATE_EXCHANGE_GATE",
             "STRMGEN_TEST_UPDATE_EXCHANGE_MARKER", "exchange"),
            ("after-publish", "STRMGEN_TEST_UPDATE_PUBLISH_GATE",
             "STRMGEN_TEST_UPDATE_MARKER", "publish")):
        hh = Harness(h.binary, h.workdir / f"update-cancel-{case_name}")
        api_c, _ = hh.server(directories={"/Movies": [
            {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None}]})
        root_c = hh.workdir / "out"
        db_c = hh.workdir / "data" / "strmgen.db"
        cfg_c = hh.config(f"cancel-{case_name}", api_c, [("/Movies", root_c)], db_c)
        check(hh.run(cfg_c).returncode == 0, f"cancel-{case_name} seed sync failed")
        cfg_cb = hh.config(f"cancel-{case_name}-b", api_c, [("/Movies", root_c)], db_c,
                           public="https://changed.example.invalid")
        gate = hh.workdir / "gate"
        marker_c = hh.workdir / "window.marker"
        proc_c = hh.start(cfg_cb, binary=h.fault_binary, extra_env={
            gate_env: str(gate),
            marker_env: str(marker_c),
        })
        check(wait_for_marker(proc_c, marker_c, timeout=30),
              f"{case_name} gate never recorded its window")
        proc_c.send_signal(signal.SIGTERM)
        gate.write_text("continue")
        try:
            cancel_out, cancel_err = proc_c.communicate(timeout=30)
        except subprocess.TimeoutExpired:
            proc_c.kill()
            proc_c.communicate(timeout=10)
            raise Failure(f"{case_name} cancellation never finished")
        check(proc_c.returncode != 0, f"{case_name} cancellation exited 0")
        cancel_state = read_db(db_c)
        check(cancel_state["rounds"][0][1] == 1,
              f"{case_name} cancellation committed a round: {cancel_state}")
        retry_c = hh.run(cfg_cb, timeout=60)
        check(retry_c.returncode == 0,
              f"{case_name} restart failed: {retry_c.returncode} {retry_c.stderr}")
        check("https://changed.example.invalid" in (root_c / "A.strm").read_text(),
              f"{case_name} restart did not persist the update")
        check(not update_staging_names(root_c), f"{case_name} restart left staging residue")
        observed[f"update_cancel_{case_name}_exit"] = proc_c.returncode
        observed[f"update_cancel_{case_name}_recovery"] = retry_c.returncode
        hh.cleanup()

    results["fault_recovery"] = observed


SCENARIOS = {
    "first_generation": scenario_first_generation,
    "repeat_and_compare": scenario_repeat_and_compare,
    "ownership_and_paths": scenario_ownership_and_paths,
    "api_and_limits": scenario_api_and_limits,
    "dry_run_boundaries": scenario_dry_run_boundaries,
    "multiprocess": scenario_multiprocess,
    "source_identity": scenario_source_identity,
    "database_uri": scenario_database_uri,
    "fault_recovery": scenario_fault_recovery,
}


def validate_output_dir(output_dir):
    resolved = output_dir.resolve()
    evidence = (ROOT / "artifacts" / "validation-evidence").resolve()
    if resolved.is_relative_to(ROOT) and not resolved.is_relative_to(evidence):
        raise SystemExit(f"refusing to write outside {evidence}: {resolved}")
    if resolved.exists() and any(resolved.iterdir()):
        raise SystemExit(f"refusing to reuse a non-empty output directory: {resolved}")
    resolved.mkdir(parents=True, exist_ok=True)
    return resolved


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--fault-binary",
                        help="optional strmgen_testhooks build used only for exact publish-race injection")
    parser.add_argument("--scenario", default="all",
                        help="'all' or a comma-separated subset: " + ", ".join(ALL_SCENARIOS))
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    binary = Path(args.binary).resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise SystemExit(f"binary not executable: {binary}")
    fault_binary = Path(args.fault_binary).resolve() if args.fault_binary else None
    if fault_binary is not None and (not fault_binary.is_file() or not os.access(fault_binary, os.X_OK)):
        raise SystemExit(f"fault-injection binary not executable: {fault_binary}")
    output_dir = validate_output_dir(Path(args.output_dir))
    if args.scenario == "all":
        selected = ALL_SCENARIOS
    else:
        selected = [name.strip() for name in args.scenario.split(",") if name.strip()]
    unknown = [name for name in selected if name not in SCENARIOS]
    if unknown:
        raise SystemExit("unknown scenario(s): " + ", ".join(unknown))

    results = {}
    failures = []
    for name in selected:
        workdir = output_dir / "run" / name
        if workdir.exists():
            raise SystemExit(f"scenario work directory already exists: {workdir}")
        harness = Harness(binary, workdir, fault_binary=fault_binary)
        started = time.time()
        try:
            SCENARIOS[name](harness, results)
            status = "PASS"
            print(f"PASS {name} ({time.time() - started:.1f}s)", flush=True)
        except (Failure, AssertionError) as error:
            status = "FAIL"
            failures.append((name, str(error)))
            print(f"FAIL {name}: {error}", flush=True)
        except subprocess.TimeoutExpired as error:
            status = "FAIL"
            failures.append((name, f"timeout: {error}"))
            print(f"FAIL {name}: timeout", flush=True)
        finally:
            harness.cleanup()
            cleanup_all_harnesses()
        results.setdefault(name, {})
        if isinstance(results.get(name), dict):
            results[name]["status"] = status
        else:
            results[name] = {"status": status}

    summary = {
        "binary": str(binary),
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "fault_binary": str(fault_binary) if fault_binary is not None else None,
        "fault_binary_sha256": hashlib.sha256(fault_binary.read_bytes()).hexdigest() if fault_binary else None,
        "scenarios": results,
        "failures": failures,
    }
    (output_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"scenarios": {name: results.get(name, {}).get("status") for name in selected},
                      "failures": failures}, ensure_ascii=False))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
