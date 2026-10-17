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
import signal
import sqlite3
import subprocess
import sys
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from mock_openlist import TOKEN, load_fixture, make_server  # noqa: E402

DEFAULT_PUBLIC = "https://openlist.example.invalid"
ALL_SCENARIOS = [
    "first_generation",
    "repeat_and_compare",
    "ownership_and_paths",
    "api_and_limits",
    "dry_run_boundaries",
    "multiprocess",
    "fault_recovery",
]


class Failure(AssertionError):
    pass


def check(condition, message):
    if not condition:
        raise Failure(message)


def base_directories():
    return copy.deepcopy(load_fixture()["directories"])


class Harness:
    """One scenario's isolated filesystem, service and process space."""

    def __init__(self, binary, workdir):
        self.binary = binary
        self.workdir = workdir
        self.workdir.mkdir(parents=True, exist_ok=True)
        self.processes = []
        # Isolate the cross-process lock namespace so this driver never blocks
        # or is blocked by an unrelated sync. All processes inside one scenario
        # share it, so cross-process exclusion is still exercised.
        self.lock_dir = workdir / "locks"
        self.lock_dir.mkdir(parents=True, exist_ok=True)

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
        env["STRMGEN_LOCK_DIR"] = str(self.lock_dir)
        if extra_env:
            env.update(extra_env)
        args = [str(self.binary), "sync", "--config", str(config)]
        if dry_run:
            args.append("--dry-run")
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout, env=env)

    def start(self, config, dry_run=False, extra_env=None):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = TOKEN
        env["STRMGEN_LOCK_DIR"] = str(self.lock_dir)
        if extra_env:
            env.update(extra_env)
        args = [str(self.binary), "sync", "--config", str(config)]
        if dry_run:
            args.append("--dry-run")
        proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env)
        self.processes.append(proc)
        return proc

    def cleanup(self):
        for proc in self.processes:
            if proc.poll() is None:
                proc.kill()
                proc.wait(timeout=5)
        self.stop_server()


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
    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    try:
        ownership = list(conn.execute(
            "SELECT source_key, relative_path, absolute_path, missing_count FROM ownership ORDER BY relative_path"))
        rounds = list(conn.execute("SELECT source_key, epoch FROM rounds ORDER BY source_key"))
    finally:
        conn.close()
    return {"exists": True, "ownership": ownership, "rounds": rounds, "sidecars": sidecars(db)}


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

    # Different SHA-1 -> needs update, old file preserved, non-zero exit.
    sample["hash_info"] = {"sha1": "cccccccccccccccccccccccccccccccccccccccc"}
    epoch_before_change = read_db(db)["rounds"][0][1]
    changed = h.run(cfg)
    check(changed.returncode == 1, f"hash change must exit 1: {changed.returncode}")
    check("needs_update=1" in changed.stdout, f"needs_update summary: {changed.stdout}")
    check(snapshot(root)["Sample.strm"] == before["Sample.strm"], "old file was not preserved")
    check(read_db(db)["rounds"][0][1] == epoch_before_change, "round advanced despite a pending update")

    # URL change -> update required for every file.
    cfg_url = h.config("repeat-url", api, [("/Movies", root)], db, public="https://other.example.invalid")
    url_change = h.run(cfg_url)
    check(url_change.returncode == 1, f"url change must exit 1: {url_change.returncode}")
    check("needs_update=4" in url_change.stdout, f"url change summary: {url_change.stdout}")

    # SHA-1 incomparable, Size differs -> update required.
    nohash = next(entry for entry in dirs["/Movies"] if entry["name"] == "NoHash.mp4")
    nohash["size"] = nohash["size"] + 5
    incomparable = h.run(cfg)
    check(incomparable.returncode == 1, f"incomparable size change must exit 1: {incomparable.returncode}")
    # Sample.mkv was already changed above, so both files require an update.
    check("needs_update=2" in incomparable.stdout, f"incomparable summary: {incomparable.stdout}")
    results["repeat_and_compare"] = {"anomaly": anomaly.stderr.strip().splitlines()[-1:]}


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
    results["ownership_and_paths"] = {"conflict_exit": conflict.returncode, "symlink_exit": link.returncode,
                                      "same_target_exit": same.returncode, "invalid_exit": invalid.returncode}


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
    # Early empty page before total was reached.
    run_case("earlyempty", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [], "total": 3}))
    # Duplicate entry names.
    run_case("duplicate", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [
                 {"name": "A.mkv", "is_dir": False, "size": 1, "hash_info": None},
                 {"name": "A.mkv", "is_dir": False, "size": 2, "hash_info": None}], "total": 2}))
    # Oversized response body.
    big = "z" * (9 * 1024 * 1024)
    run_case("oversized", {"/Movies": [{"name": "x", "is_dir": False, "size": 1}]},
             _override_list({"content": [{"name": big, "is_dir": False, "size": 1, "hash_info": None}], "total": 1}))

    # 401 for a wrong token.
    hh = Harness(h.binary, h.workdir / "unauthorized")
    api, _ = hh.server()
    root = hh.workdir / "out"
    db = hh.workdir / "data" / "strmgen.db"
    cfg = hh.config("unauthorized", api, [("/Movies", root)], db)
    proc = hh.run(cfg, extra_env={"OPENLIST_TOKEN": "wrong-token"})
    check(proc.returncode == 1, f"wrong token must exit 1: {proc.returncode}")
    check(len(read_db(db)["rounds"]) == 0, "wrong token committed a round")
    hh.cleanup()
    observed["unauthorized"] = proc.returncode

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
    dry = h.run(cfg, dry_run=True)
    check(dry.returncode == 0, f"dry-run exit {dry.returncode}: {dry.stderr}")
    check(not root.exists(), "dry-run created the output root")
    check(not db.exists(), "dry-run created the database")
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
    dry2b = h2b.run(cfg2b, dry_run=True)
    check(dry2b.returncode == 0, f"dry-run on WAL db exit {dry2b.returncode}: {dry2b.stderr}")
    check(snapshot(root2b) == before_wal, "WAL dry-run changed output")
    check(not any(Path(str(db2b) + s).exists() for s in ("-wal", "-shm", "-journal")),
          "WAL dry-run created a sidecar")
    observed["wal_database"] = dry2b.returncode
    h2b.cleanup()

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
    results["dry_run_boundaries"] = observed


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
    proc1 = h.start(cfg1)
    check(entered.wait(timeout=15), "first process never reached the barrier")

    # A second process with a different output range and database must still be
    # excluded by the shared round lock.
    root2 = h.workdir / "second" / "out"
    db2 = h.workdir / "second" / "data" / "strmgen.db"
    cfg2 = h.config("second", api, [("/Movies", root2)], db2)
    second = h.run(cfg2, timeout=30)
    check(second.returncode == 1, f"second process must be excluded: {second.returncode}")
    check("another sync is already running" in second.stderr, f"busy message missing: {second.stderr}")
    check(not root2.exists(), "excluded process created output")
    check(not db2.exists(), "excluded process created a database")

    # An ancestor output range must also be excluded.
    ancestor = h.workdir / "ancestor" / "out"
    cfg3 = h.config("ancestor", api, [("/Movies", ancestor)], h.workdir / "ancestor" / "data" / "strmgen.db")
    third = h.run(cfg3, timeout=30)
    check(third.returncode == 1, f"ancestor range must be excluded: {third.returncode}")
    check(not ancestor.exists(), "ancestor-range process created output")

    # A subdirectory output range and a path alias must also be excluded.
    subdir = h.workdir / "first" / "out" / "nested"
    cfg4 = h.config("subdir", api, [("/Movies", subdir)], h.workdir / "alias" / "data" / "strmgen.db")
    fourth = h.run(cfg4, timeout=30)
    check(fourth.returncode == 1, f"subdirectory range must be excluded: {fourth.returncode}")
    alias_cfg = h.config("alias", api, [("/Movies", h.workdir / "first" / ".." / "first" / "out" / "alias")],
                         h.workdir / "alias2" / "data" / "strmgen.db")
    fifth = h.run(alias_cfg, timeout=30)
    check(fifth.returncode == 1, f"aliased range must be excluded: {fifth.returncode}")

    barrier.set()
    out1, err1 = proc1.communicate(timeout=60)
    check(proc1.returncode == 0, f"first process failed: {proc1.returncode} {err1}")
    check("created=4" in out1, f"first process summary: {out1}")

    # After the first process exits the lock is available again.
    again = h.run(cfg2, timeout=60)
    check(again.returncode == 0, f"rerun after release failed: {again.returncode} {again.stderr}")
    results["multiprocess"] = {"second_exit": second.returncode, "ancestor_exit": third.returncode,
                               "subdir_exit": fourth.returncode, "alias_exit": fifth.returncode,
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
    results["fault_recovery"] = observed


SCENARIOS = {
    "first_generation": scenario_first_generation,
    "repeat_and_compare": scenario_repeat_and_compare,
    "ownership_and_paths": scenario_ownership_and_paths,
    "api_and_limits": scenario_api_and_limits,
    "dry_run_boundaries": scenario_dry_run_boundaries,
    "multiprocess": scenario_multiprocess,
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
    parser.add_argument("--scenario", default="all",
                        help="'all' or a comma-separated subset: " + ", ".join(ALL_SCENARIOS))
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    binary = Path(args.binary).resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise SystemExit(f"binary not executable: {binary}")
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
        harness = Harness(binary, workdir)
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
        results.setdefault(name, {})
        if isinstance(results.get(name), dict):
            results[name]["status"] = status
        else:
            results[name] = {"status": status}

    summary = {
        "binary": str(binary),
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "scenarios": results,
        "failures": failures,
    }
    (output_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"scenarios": {name: results.get(name, {}).get("status") for name in selected},
                      "failures": failures}, ensure_ascii=False))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
