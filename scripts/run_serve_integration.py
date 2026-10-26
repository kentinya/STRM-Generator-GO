#!/usr/bin/env python3
"""Isolated acceptance driver for the long-running `serve` scheduling entry.

The driver starts its own synthetic OpenList service on a dynamic port and
drives the compiled binary as a subprocess. It asserts on real exit codes, STRM
bytes, SQLite rows, filesystem snapshots, request counts, signal timing and
stderr, never on Go internals. It never contacts the shared 15244 fixture,
production tokens or production mounts.

The scheduling interval is shortened only through the strmgen_testhooks build
tag (`STRMGEN_TEST_SERVE_INTERVAL_MS`). The release binary has no such override
and is used to verify the immediate first round, configuration rejection and
cancellation; it always uses the validated `sync.interval_minutes`.

Usage:
    python3 scripts/run_serve_integration.py \
        --binary artifacts/task-3.1/bin/strmgen-amd64 \
        --fault-binary artifacts/task-3.1/bin/strmgen-testhooks-amd64 \
        --output-dir artifacts/validation-evidence/task-3.1-final/serve
"""
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
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
ALL_SCENARIOS = [
    "serve_release_first_round",
    "serve_interval_and_skip",
    "serve_busy_skip",
    "serve_failure_continue",
    "serve_cancel",
    "serve_redaction",
    "serve_custom_extensions",
]

_LIVE_HARNESSES = []


class Failure(AssertionError):
    pass


def check(condition, message):
    if not condition:
        raise Failure(message)


def base_directories():
    return copy.deepcopy(load_fixture()["directories"])


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


def sidecars(db):
    db = Path(db)
    found = []
    for suffix in ("", "-wal", "-shm", "-journal"):
        candidate = Path(str(db) + suffix)
        if candidate.exists():
            found.append((candidate.name, candidate.stat().st_size))
    return sorted(found)


def snapshot(root):
    result = {}
    root = Path(root)
    if not root.exists():
        return result
    for path in sorted(root.rglob("*")):
        if path.is_file():
            rel = path.relative_to(root).as_posix()
            result[rel] = (hashlib.sha256(path.read_bytes()).hexdigest(), path.stat().st_mtime_ns)
    return result


def snapshot_tree(root):
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

    def config(self, name, api_url, sources, db, public=DEFAULT_PUBLIC, workers=4,
               interval_minutes=60, media_extensions=None):
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
        lines.append(f"  interval_minutes: {interval_minutes}")
        lines.append(f"  directory_workers: {workers}")
        if media_extensions is not None:
            rendered = ", ".join(json.dumps(ext) for ext in media_extensions)
            lines.append(f"  media_extensions: [{rendered}]")
        lines.append("cleanup:")
        lines.append("  enabled: false")
        lines.append("database:")
        lines.append(f'  path: "{db}"')
        path = self.workdir / f"{name}.yaml"
        path.write_text("\n".join(lines) + "\n")
        return path

    def sync(self, config, dry_run=False, timeout=60, extra_env=None, binary=None):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = TOKEN
        if extra_env:
            env.update(extra_env)
        args = [str(binary or self.binary), "sync", "--config", str(config)]
        if dry_run:
            args.append("--dry-run")
        return subprocess.run(args, capture_output=True, text=True, timeout=timeout, env=env)

    def start_serve(self, config, extra_env=None, binary=None, stdout_path=None, stderr_path=None):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = TOKEN
        if extra_env:
            env.update(extra_env)
        args = [str(binary or self.fault_binary or self.binary), "serve", "--config", str(config)]
        out = open(stdout_path, "w", encoding="utf-8") if stdout_path else subprocess.PIPE
        err = open(stderr_path, "w", encoding="utf-8") if stderr_path else subprocess.PIPE
        proc = subprocess.Popen(args, stdout=out, stderr=err, text=True, env=env)
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
    for harness in list(_LIVE_HARNESSES):
        try:
            harness.cleanup()
        except Exception:
            pass


def wait_for(predicate, timeout, message):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            return True
        time.sleep(0.01)
    return False


def wait_for_file(path, timeout=30):
    return wait_for(lambda: Path(path).exists(), timeout, "file did not appear")


def read_lines(path):
    path = Path(path)
    if not path.exists():
        return []
    return path.read_text(encoding="utf-8", errors="replace").splitlines()


def count_serve_results(path, kind):
    return len([line for line in read_lines(path) if f"result: {kind} " in line])


def stop_serve(proc, timeout=15):
    proc.send_signal(signal.SIGTERM)
    try:
        out, err = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.kill()
        out, err = proc.communicate(timeout=5)
    return proc.returncode, out, err


def scenario_serve_release_first_round(h, results):
    # The release binary runs the immediate first round and rejects invalid
    # intervals before touching any product file. A long interval is used so no
    # second tick can interfere with the cancellation check.
    api, dirs = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("release", api, [("/Movies", root)], db, interval_minutes=60)

    for value in ("0", "-1", "9223372036854775808"):
        bad = h.workdir / f"bad-{value.replace('-', 'n')}.yaml"
        body = cfg.read_text().replace("interval_minutes: 60", f"interval_minutes: {value}")
        bad.write_text(body)
        bad_root = h.workdir / "out-bad"
        bad_db = h.workdir / "data-bad" / "strmgen.db"
        bad.write_text(body.replace(str(root), str(bad_root)).replace(str(db), str(bad_db)))
        proc = subprocess.run([str(h.binary), "serve", "--config", str(bad)],
                              capture_output=True, text=True, env={**os.environ, "OPENLIST_TOKEN": TOKEN})
        check(proc.returncode == 2, f"interval {value} must exit 2, got {proc.returncode}: {proc.stderr}")
        check(not bad_root.exists(), f"invalid interval {value} created output")
        check(not bad_db.exists(), f"invalid interval {value} created a database")

    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, binary=h.binary, stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: (root / "Sample.strm").exists(), 30, "first round did not create output"),
          "release serve did not run the immediate first round")
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 1, 15, "no success summary"),
          f"first round success summary missing: {read_lines(stdout_path)}")
    code, _, _ = stop_serve(proc)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    check(count_serve_results(stdout_path, "cancelled") >= 1, "cancelled result missing")
    state = read_db(db)
    check(len(state["ownership"]) == 4 and state["rounds"][0][1] == 1,
          f"release first round did not commit exactly one round: {state}")
    check(snapshot(root) == snapshot(root), "snapshot stability")
    results["serve_release_first_round"] = {
        "bad_interval_exit": 2, "cancel_exit": code, "ownership": len(state["ownership"])}


def scenario_serve_interval_and_skip(h, results):
    # Two shortened intervals: the immediate round creates the files; the
    # following ticks skip with zero writes. A manual sync afterwards proves the
    # lock was released by the cancelled serve process.
    api, dirs = h.server()
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("interval", api, [("/Movies", root)], db, interval_minutes=60)
    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, extra_env={"STRMGEN_TEST_SERVE_INTERVAL_MS": "1200"},
                         stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 3, 40,
                   "serve did not complete three rounds"),
          f"expected three rounds: {read_lines(stdout_path)}")
    before = snapshot(root)
    first_success = [line for line in read_lines(stdout_path) if "result: success " in line]
    check("created=4" in first_success[0], f"first round did not create: {first_success[0]}")
    for line in first_success[1:]:
        check("skipped=4" in line and "created=0" in line,
              f"repeat tick did not skip: {line}")
    check(snapshot(root) == before, "repeat ticks changed output bytes or mtime")
    check(read_db(db)["rounds"][0][1] == 3, f"three rounds not committed: {read_db(db)['rounds']}")
    code, _, _ = stop_serve(proc)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    # After cancellation the shared lock must be free for a one-shot sync.
    rerun = h.sync(cfg, timeout=60)
    check(rerun.returncode == 0 and "skipped=4" in rerun.stdout,
          f"post-serve sync failed or did not skip: {rerun.returncode} {rerun.stdout}")
    check(snapshot(root) == before, "post-serve sync changed output")
    results["serve_interval_and_skip"] = {
        "successes": len(first_success), "rounds": read_db(db)["rounds"][0][1], "cancel_exit": code}


def scenario_serve_busy_skip(h, results):
    # A manual sync holds the shared lock at the API barrier. The serve tick
    # must report busy-skip without entering the reconciler (no DB write), and
    # the following tick must run once the lock is released.
    entered = threading.Event()
    release = threading.Event()

    def before(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not entered.is_set():
            entered.set()
            release.wait(timeout=30)

    api, dirs = h.server(hooks={"before": before})
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("busy", api, [("/Movies", root)], db, interval_minutes=60)

    holder = subprocess.Popen([str(h.binary), "sync", "--config", str(cfg)],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                              env={**os.environ, "OPENLIST_TOKEN": TOKEN})
    h.processes.append(holder)
    check(entered.wait(timeout=20), "manual sync never reached the API barrier")

    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    serve = h.start_serve(cfg, extra_env={"STRMGEN_TEST_SERVE_INTERVAL_MS": "1200"},
                          stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: count_serve_results(stdout_path, "busy-skip") >= 1, 20, "no busy-skip summary"),
          f"busy-skip was not reported: {read_lines(stdout_path)}")
    check(read_db(db)["rounds"] == [], "busy-skip tick entered the reconciler and committed a round")
    # The holder is still blocked, so the busy tick could not have written.
    release.set()
    holder_out, holder_err = holder.communicate(timeout=60)
    check(holder.returncode == 0, f"holder sync failed: {holder.returncode} {holder_err}")
    check("created=4" in holder_out, f"holder summary: {holder_out}")
    # A later tick must now run and skip the already-created files.
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 1, 30,
                   "serve did not run after the lock was released"),
          f"no success after release: {read_lines(stdout_path)}")
    code, _, _ = stop_serve(serve)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    results["serve_busy_skip"] = {
        "busy_skips": count_serve_results(stdout_path, "busy-skip"),
        "successes": count_serve_results(stdout_path, "success"), "cancel_exit": code}


def scenario_serve_failure_continue(h, results):
    # The first list call fails, so the first round fails; the next tick must
    # succeed and create the files. The service must keep running.
    calls = {"n": 0}
    lock = threading.Lock()

    def override(endpoint, path, request, directories):
        if endpoint.endswith("/list") and path == "/Movies":
            with lock:
                calls["n"] += 1
                first = calls["n"] == 1
            if first:
                return 503, 503, None
        return None

    api, dirs = h.server(hooks={"override": override})
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("failure", api, [("/Movies", root)], db, interval_minutes=60)
    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, extra_env={"STRMGEN_TEST_SERVE_INTERVAL_MS": "1200"},
                         stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: count_serve_results(stdout_path, "failure") >= 1, 20, "no failure summary"),
          f"failure not reported: {read_lines(stdout_path)}")
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 1, 30,
                   "serve did not recover on the next tick"),
          f"no recovery success: {read_lines(stdout_path)}")
    check(read_db(db)["rounds"][0][1] == 1, f"recovery did not commit one round: {read_db(db)['rounds']}")
    check(snapshot(root).get("Sample.strm") is not None, "recovery did not create output")
    code, _, _ = stop_serve(proc)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    results["serve_failure_continue"] = {
        "failures": count_serve_results(stdout_path, "failure"),
        "successes": count_serve_results(stdout_path, "success"), "cancel_exit": code}


def scenario_serve_cancel(h, results):
    # SIGTERM during the immediate round: the round must not commit, the exit
    # is a non-zero cancelled result, the lock is released and a restart
    # produces exactly one new complete round.
    entered = threading.Event()
    release = threading.Event()

    def before(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not entered.is_set():
            entered.set()
            release.wait(timeout=30)

    api, dirs = h.server(hooks={"before": before})
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("cancel", api, [("/Movies", root)], db, interval_minutes=60)
    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, stdout_path=stdout_path, stderr_path=stderr_path)
    check(entered.wait(timeout=20), "serve never reached the API barrier")
    code, _, _ = stop_serve(proc)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    state = read_db(db)
    check(state["rounds"] == [], f"cancelled round committed: {state['rounds']}")
    check(state["ownership"] == [], f"cancelled round wrote ownership: {state['ownership']}")
    check(count_serve_results(stdout_path, "cancelled") >= 1, "cancelled result missing")
    # Release the blocked handler and stop the server so the retry is clean.
    release.set()
    # Lock must be free: a one-shot sync completes.
    rerun = h.sync(cfg, timeout=60)
    check(rerun.returncode == 0 and "created=4" in rerun.stdout,
          f"post-cancel sync failed: {rerun.returncode} {rerun.stdout}")
    check(read_db(db)["rounds"][0][1] == 1, f"post-cancel sync did not commit one round: {read_db(db)['rounds']}")
    results["serve_cancel"] = {"cancel_exit": code, "recovery_exit": rerun.returncode}


def scenario_serve_redaction(h, results):
    # A private output path and a token marker must never appear in the serve
    # output. The API body also carries a signed URL that must not be echoed.
    secret = "SYNTHETIC_SERVE_TOKEN_SIGN_MARKER_20261019"
    private_root = h.workdir / "PRIVATE_SERVE_OUTPUT_MARKER" / "out"
    private_db = h.workdir / "PRIVATE_SERVE_DB_MARKER" / "strmgen.db"

    def secret_api(endpoint, path, request, directories):
        if endpoint.endswith("/get") and path == "/Movies":
            return 200, 403, {
                "message": secret,
                "url": f"https://private.example.invalid/file?sign={secret}",
            }
        return None

    api, _ = h.server(hooks={"override": secret_api})
    cfg = h.config("redaction", api, [("/Movies", private_root)], private_db, interval_minutes=60)
    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, extra_env={"OPENLIST_TOKEN": secret},
                         stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: count_serve_results(stdout_path, "failure") >= 1, 20, "no failure summary"),
          f"redaction case produced no failure: {read_lines(stdout_path)}")
    code, _, _ = stop_serve(proc)
    combined = "\n".join(read_lines(stdout_path) + read_lines(stderr_path))
    for marker in (secret, "private.example.invalid", "sign=",
                   "PRIVATE_SERVE_OUTPUT_MARKER", "PRIVATE_SERVE_DB_MARKER"):
        check(marker not in combined, f"serve output leaked {marker}: {combined}")
    check(not private_root.exists(), "redaction failure created output")
    results["serve_redaction"] = {"cancel_exit": code}


def scenario_serve_custom_extensions(h, results):
    """The serve entry uses the resolved extension list for every round.

    The first serve process runs two shortened ticks with one list. Editing the
    configuration file while it runs must change nothing; only a restart adopts
    the new list. Both serve runs use the release binary and the testhooks build
    solely for the shortened interval.
    """
    entries = [
        {"name": "Sample.mkv", "is_dir": False, "size": 10,
         "hash_info": {"sha1": "a" * 40}},
        {"name": "Clip.flv", "is_dir": False, "size": 20,
         "hash_info": {"sha1": "b" * 40}},
    ]
    api, dirs = h.server(directories={"/Movies": list(entries)})
    root = h.workdir / "out"
    db = h.workdir / "data" / "strmgen.db"
    cfg = h.config("extensions", api, [("/Movies", root)], db,
                   media_extensions=[".flv"])
    stdout_path = h.workdir / "serve.out"
    stderr_path = h.workdir / "serve.err"
    proc = h.start_serve(cfg, extra_env={"STRMGEN_TEST_SERVE_INTERVAL_MS": "1200"},
                         stdout_path=stdout_path, stderr_path=stderr_path)
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 2, 40,
                   "serve did not complete two rounds"),
          f"expected two rounds: {read_lines(stdout_path)}")
    successes = [line for line in read_lines(stdout_path) if "result: success " in line]
    check("created=1" in successes[0], f"first serve round did not create the custom extension: {successes[0]}")
    for line in successes[1:]:
        check("skipped=1" in line and "created=0" in line,
              f"repeat serve tick did not skip: {line}")
    check((root / "Clip.strm").exists(), "the custom extension was not generated")
    check(not (root / "Sample.strm").exists(), "an excluded default extension was generated")
    before = snapshot(root)

    # Editing the configuration file must not affect the running process.
    cfg.write_text(cfg.read_text().replace('media_extensions: [".flv"]',
                                           'media_extensions: [".mkv"]'))
    check(wait_for(lambda: count_serve_results(stdout_path, "success") >= 3, 20,
                   "serve did not run another tick after the config edit"),
          f"no further tick after the edit: {read_lines(stdout_path)}")
    check(snapshot(root) == before, "a running serve adopted an edited configuration file")
    code, _, _ = stop_serve(proc)
    check(code != 0, f"cancelled serve must be non-zero, got {code}")
    results["serve_custom_extensions"] = {"recipe_rounds": len(successes)}

    # A restart adopts the edited list. Both outputs are now products of
    # different config generations; only the new list is generated.
    restarted_out = h.workdir / "serve-restart.out"
    restarted_err = h.workdir / "serve-restart.err"
    restart = h.start_serve(cfg, extra_env={"STRMGEN_TEST_SERVE_INTERVAL_MS": "1200"},
                            stdout_path=restarted_out, stderr_path=restarted_err)
    check(wait_for(lambda: (root / "Sample.strm").exists(), 30,
                   "restart did not adopt the edited extension list"),
          f"restart did not adopt the list: {read_lines(restarted_out)}")
    check((root / "Clip.strm").exists(), "restart removed an existing output without cleanup")
    restart_code, _, _ = stop_serve(restart)
    check(restart_code != 0, f"cancelled serve must be non-zero, got {restart_code}")
    results["serve_custom_extensions"]["restart_adopted"] = True
    results["serve_custom_extensions"]["restart_exit"] = restart_code


SCENARIOS = {
    "serve_release_first_round": scenario_serve_release_first_round,
    "serve_interval_and_skip": scenario_serve_interval_and_skip,
    "serve_busy_skip": scenario_serve_busy_skip,
    "serve_failure_continue": scenario_serve_failure_continue,
    "serve_cancel": scenario_serve_cancel,
    "serve_redaction": scenario_serve_redaction,
    "serve_custom_extensions": scenario_serve_custom_extensions,
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
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--binary", required=True, help="release binary")
    parser.add_argument("--fault-binary", help="strmgen_testhooks binary used for shortened scheduling")
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
