#!/usr/bin/env python3
"""Isolated container acceptance driver for the strmgen deployment package.

The driver builds no product artifact itself. It consumes an already-built
Linux image (amd64 by default) and, optionally, an arm64 OCI archive produced by
`docker buildx build --output type=oci`. It starts its own synthetic OpenList
service on the host, binds every product path to a fresh directory under
`artifacts/validation-evidence/`, and asserts on real container exit codes,
mounts, image config, STRM bytes, SQLite rows and stderr. It never contacts the
shared 15244 fixture, production tokens, production mounts or a registry.

Every container, network and directory it creates is labeled with the run id
and removed in `finally`. A missing Docker engine is reported as BLOCKED with a
non-zero exit, never silently skipped.

Usage:
    python3 scripts/run_container_integration.py \
        --image strmgen:task-3.2-amd64 \
        --arm64-oci artifacts/task-3.2/strmgen-arm64.oci.tar \
        --output-dir artifacts/validation-evidence/task-3.2-final/container
"""
import argparse
import copy
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import secrets
import shutil
import sqlite3
import struct
import subprocess
import sys
import tarfile
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from mock_openlist import TOKEN, load_fixture, make_server  # noqa: E402

DEFAULT_PUBLIC = "https://openlist.example.invalid"
# A distinctive synthetic value. It must never appear in the image, a Compose
# render, stdout/stderr or the evidence index.
SECRET_MARKER = "SYNTHETIC_CONTAINER_TOKEN_MARKER_20261019"
# Strings that only exist in a testhooks build or in the source tree.
FORBIDDEN_IMAGE_MARKERS = [
    b"strmgen_testhooks",
    b"STRMGEN_TEST_SERVE_INTERVAL_MS",
    b"STRMGEN_TEST_OWNERSHIP_HOLD",
    b"serve_hook_integration.go",
]
ALL_SCENARIOS = [
    "image_static",
    "minimal_run",
    "serve_lifecycle",
    "cross_container_busy",
    "compose_lifecycle",
]
_LIVE_HARNESSES = []


class Failure(AssertionError):
    pass


class Blocked(RuntimeError):
    pass


def check(condition, message):
    if not condition:
        raise Failure(message)


def base_directories():
    return copy.deepcopy(load_fixture()["directories"])


def docker_available():
    try:
        result = subprocess.run(["docker", "version", "--format", "{{.Server.Version}}"],
                                capture_output=True, text=True, timeout=30)
    except (OSError, subprocess.SubprocessError):
        return False
    return result.returncode == 0


def docker(args, timeout=120, input_text=None, env=None):
    return subprocess.run(["docker", *args], capture_output=True, text=True,
                          timeout=timeout, input=input_text, env=env)


def wait_for(predicate, timeout, message):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            return True
        time.sleep(0.05)
    return False


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


def epoch(db):
    """Return the committed round epoch (0 when no round exists)."""
    state = read_db(db)
    return state["rounds"][0][1] if state["rounds"] else 0


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


def config_document(api_url, output="/output/Movies", interval_minutes=60):
    return "\n".join([
        "openlist:",
        f'  api_url: "{api_url}"',
        f'  public_url: "{DEFAULT_PUBLIC}"',
        '  token_env: "OPENLIST_TOKEN"',
        "sources:",
        '  - remote: "/Movies"',
        f'    output: "{output}"',
        "sync:",
        f"  interval_minutes: {interval_minutes}",
        "  directory_workers: 4",
        "cleanup:",
        "  enabled: false",
        "database:",
        '  path: "/state/strmgen.db"',
    ]) + "\n"


class Harness:
    """One scenario's isolated directories, service and container space."""

    def __init__(self, image, workdir, run_id, uid=10001, gid=10001):
        self.image = image
        self.workdir = workdir
        self.workdir.mkdir(parents=True, exist_ok=True)
        self.run_id = run_id
        self.uid = uid
        self.gid = gid
        self.containers = []
        self.compose_project = None
        self.out = workdir / "out"
        self.state = workdir / "state"
        self.locks = workdir / "locks"
        self.cfg_dir = workdir / "cfg"
        for path in (self.out, self.state, self.locks, self.cfg_dir):
            path.mkdir(parents=True, exist_ok=True)
            os.chown(path, uid, gid)
            os.chmod(path, 0o755)
        _LIVE_HARNESSES.append(self)

    # -- service -----------------------------------------------------------
    def server(self, directories=None, hooks=None):
        fixture = {"directories": directories if directories is not None else base_directories()}
        # The driver injects SECRET_MARKER as the runtime token. The synthetic
        # service accepts it so the container can authenticate; the marker never
        # appears in an image, a committed file or the evidence index.
        merged = dict(hooks or {})
        tokens = list(merged.get("tokens", []))
        if SECRET_MARKER not in tokens:
            tokens.append(SECRET_MARKER)
        merged["tokens"] = tokens
        server = make_server(host="0.0.0.0", port=0, fixture=fixture, hooks=merged)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self._server = server
        self._thread = thread
        return f"http://host.docker.internal:{server.server_port}"

    def stop_server(self):
        if getattr(self, "_server", None) is not None:
            self._server.shutdown()
            self._server.server_close()
            self._thread.join(timeout=5)
            self._server = None

    # -- configuration -----------------------------------------------------
    def write_config(self, name, api_url, interval_minutes=60):
        path = self.cfg_dir / f"{name}.yaml"
        path.write_text(config_document(api_url, interval_minutes=interval_minutes), encoding="utf-8")
        os.chmod(path, 0o444)
        return path

    # -- containers --------------------------------------------------------
    def common_run_args(self, config_path, name, rm=True):
        args = ["run"]
        if rm:
            args.append("--rm")
        args += [
            "--name", name,
            "--label", "strmgen.task=3.2",
            "--label", f"strmgen.run={self.run_id}",
            "--add-host", "host.docker.internal:host-gateway",
            "--user", f"{self.uid}:{self.gid}",
            "--read-only",
            "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=64m,mode=1777",
            "-v", f"{config_path}:/etc/strmgen/config.yaml:ro",
            "-v", f"{self.out}:/output",
            "-v", f"{self.state}:/state",
            "-v", f"{self.locks}:/tmp/strmgen-locks",
        ]
        return args

    def _env(self):
        env = dict(os.environ)
        env["OPENLIST_TOKEN"] = SECRET_MARKER
        return env

    def run_once(self, config_path, name, command=None, timeout=120):
        # The token is passed through the parent environment (`-e VAR`), never
        # as a literal command-line value, so it cannot appear in host argv.
        args = self.common_run_args(config_path, name)
        args += ["-e", "OPENLIST_TOKEN", self.image]
        if command is None:
            command = ["sync", "--config", "/etc/strmgen/config.yaml"]
        if command:
            args = args[:-1] + ["--entrypoint", "/usr/local/bin/strmgen", self.image, *command]
        result = docker(args, timeout=timeout, env=self._env())
        self.containers.append(name)
        return result

    def start_detached(self, config_path, name, command=None):
        args = self.common_run_args(config_path, name, rm=False)
        args = args[:1] + ["-d"] + args[1:]
        args += ["-e", "OPENLIST_TOKEN", self.image]
        if command:
            args = args[:-1] + ["--entrypoint", "/usr/local/bin/strmgen", self.image, *command]
        result = docker(args, timeout=120, env=self._env())
        check(result.returncode == 0, f"container {name} failed to start: {result.stderr}")
        self.containers.append(name)
        return name

    def stop(self, name, timeout=60):
        result = docker(["stop", "-t", str(timeout), name], timeout=timeout + 30)
        check(result.returncode == 0, f"docker stop {name} failed: {result.stderr}")
        code = docker(["inspect", "--format", "{{.State.ExitCode}}", name], timeout=30)
        return int(code.stdout.strip())

    def remove(self, name):
        docker(["rm", "-f", name], timeout=60)

    def cleanup(self):
        for name in self.containers:
            self.remove(name)
        # Remove anything else this run created, even if a timeout prevented it
        # from being tracked.
        listed = docker(["ps", "-aq", "--filter", f"label=strmgen.run={self.run_id}"], timeout=30)
        for container_id in listed.stdout.split():
            docker(["rm", "-f", container_id], timeout=60)
        if self.compose_project:
            env = dict(os.environ)
            subprocess.run(["docker", "compose", "-f", str(ROOT / "deploy" / "compose.yaml"),
                            "-p", self.compose_project, "down", "-v", "--remove-orphans"],
                           capture_output=True, text=True, timeout=120, env=env)
        self.stop_server()
        if self in _LIVE_HARNESSES:
            _LIVE_HARNESSES.remove(self)


def cleanup_all_harnesses():
    for harness in list(_LIVE_HARNESSES):
        try:
            harness.cleanup()
        except Exception:
            pass


def compose_env(harness, config_path, image):
    env = dict(os.environ)
    env.update({
        "STRMGEN_IMAGE": image,
        "STRMGEN_CONFIG": str(config_path),
        "STRMGEN_OUTPUT": str(harness.out),
        "STRMGEN_STATE": str(harness.state),
        "STRMGEN_LOCK_DIR": str(harness.locks),
        "STRMGEN_UID": str(harness.uid),
        "STRMGEN_GID": str(harness.gid),
        "OPENLIST_TOKEN": SECRET_MARKER,
    })
    return env


# -- image / OCI static checks --------------------------------------------
def scan_image_for_markers(image, marker):
    """Return (found_markers, layer_count, image_size) by scanning the image tar."""
    tmp = Path("/tmp") / f"strmgen-image-scan-{secrets.token_hex(6)}.tar"
    try:
        result = docker(["save", "-o", str(tmp), image], timeout=300)
        check(result.returncode == 0, f"docker save failed: {result.stderr}")
        found = set()
        layer_count = 0
        with tarfile.open(tmp, "r") as archive:
            for member in archive:
                if not member.isfile():
                    continue
                if member.name.startswith("blobs/sha256/"):
                    layer_count += 1
                handle = archive.extractfile(member)
                if handle is None:
                    continue
                data = handle.read()
                if marker in data:
                    found.add("secret")
                for forbidden in FORBIDDEN_IMAGE_MARKERS:
                    if forbidden in data:
                        found.add(forbidden.decode())
        return sorted(found), layer_count, tmp.stat().st_size
    finally:
        tmp.unlink(missing_ok=True)


def elf_machine(data):
    if len(data) < 20 or data[:4] != b"\x7fELF":
        return None
    if data[5] == 1:  # little endian
        return struct.unpack_from("<H", data, 18)[0]
    if data[5] == 2:
        return struct.unpack_from(">H", data, 18)[0]
    return None


def inspect_oci(path):
    """Return a summary of an OCI image archive, including binary ELF machine."""
    path = Path(path)
    check(path.is_file(), f"arm64 OCI archive missing: {path}")
    summary = {"path": str(path), "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
               "manifests": [], "binaries": []}
    with tarfile.open(path, "r") as archive:
        def read_blob(digest):
            name = "blobs/sha256/" + digest.split(":")[-1]
            member = archive.getmember(name)
            return archive.extractfile(member).read()

        index = json.loads(archive.extractfile(archive.getmember("index.json")).read())
        for manifest in index.get("manifests", []):
            platform = manifest.get("platform", {})
            entry = {"digest": manifest["digest"],
                     "os": platform.get("os"), "architecture": platform.get("architecture")}
            if platform.get("os") == "linux" and platform.get("architecture") in ("amd64", "arm64"):
                body = json.loads(read_blob(manifest["digest"]))
                config = json.loads(read_blob(body["config"]["digest"]))
                entry["config_architecture"] = config.get("architecture")
                entry["config_os"] = config.get("os")
                entry["config_user"] = config.get("config", {}).get("User")
                summary["manifests"].append(entry)
                for layer in body.get("layers", []):
                    raw = read_blob(layer["digest"])
                    blob = gzip.decompress(raw) if layer["mediaType"].endswith("gzip") else raw
                    with tarfile.open(fileobj=io.BytesIO(blob), mode="r:") as layer_tar:
                        for item in layer_tar:
                            if item.isfile() and item.name.endswith("bin/strmgen"):
                                content = layer_tar.extractfile(item).read()
                                summary["binaries"].append({
                                    "path": item.name,
                                    "size": len(content),
                                    "elf_machine": elf_machine(content),
                                })
            else:
                summary["manifests"].append(entry)
    return summary


def read_index_digest(archive):
    """Read blobs/sha256/index.json, which OCI layouts reference by manifest."""
    member = archive.getmember("index.json")
    return json.loads(archive.extractfile(member).read())

# -- scenarios -------------------------------------------------------------
def scenario_image_static(h, results, image):
    info = docker(["image", "inspect", image,
                   "--format", "{{json .Config}}|{{.Architecture}}|{{.Os}}|{{.Size}}|{{json .RootFS}}"], timeout=60)
    check(info.returncode == 0, f"image inspect failed: {info.stderr}")
    config_json, arch, os_name, size, rootfs = info.stdout.strip().split("|")
    config = json.loads(config_json)
    check(config.get("User") == f"{h.uid}:{h.gid}",
          f"image must run as the unprivileged account, got {config.get('User')!r}")
    check(config.get("Entrypoint") == ["/usr/local/bin/strmgen"],
          f"unexpected entrypoint: {config.get('Entrypoint')}")
    check(config.get("Cmd") == ["serve", "--config", "/etc/strmgen/config.yaml"],
          f"unexpected default command: {config.get('Cmd')}")
    env_blob = "\n".join(config.get("Env") or [])
    check(SECRET_MARKER not in env_blob, "image environment contains the token marker")

    found, layer_count, image_size = scan_image_for_markers(image, SECRET_MARKER.encode())
    check(found == [], f"image layers contain forbidden markers: {found}")

    # A non-root identity is also proven at runtime, not only in the config.
    run = docker(["run", "--rm", "--entrypoint", "/bin/sh", image, "-c", "id -u; id -g; ls -ld /tmp/strmgen-locks"],
                 timeout=60)
    check(run.returncode == 0, f"image identity probe failed: {run.stderr}")
    uid_line, gid_line, lock_line = (run.stdout.strip().splitlines() + ["", "", ""])[:3]
    check(uid_line == str(h.uid) and gid_line == str(h.gid),
          f"container did not run as {h.uid}:{h.gid}: {run.stdout}")

    results["image_static"] = {
        "architecture": arch, "os": os_name, "size": size,
        "config_user": config.get("User"), "layer_count": layer_count,
        "image_tar_bytes": image_size, "forbidden_markers": found,
    }


def scenario_minimal_run(h, results, image):
    api = h.server()
    cfg = h.write_config("minimal", api)
    result = h.run_once(cfg, f"strmgen-3-2-min-{h.run_id}", timeout=120)
    check(result.returncode == 0, f"minimal sync failed: {result.returncode} {result.stderr}")
    check("created=4" in result.stdout, f"minimal run summary: {result.stdout}")
    combined = result.stdout + result.stderr
    for marker in (SECRET_MARKER, "host.docker.internal"):
        check(marker not in combined, f"minimal run leaked {marker}: {combined}")

    files = snapshot(h.out)
    check(len(files) == 4, f"expected 4 STRM files, got {sorted(files)}")
    state = read_db(h.state / "strmgen.db")
    check(len(state["ownership"]) == 4 and state["rounds"][0][1] == 1,
          f"container run did not persist ownership/round: {state}")
    check(state["sidecars"] and state["sidecars"][0][0] == "strmgen.db",
          f"database sidecars unexpected: {state['sidecars']}")
    check(list(h.cfg_dir.iterdir()) == [cfg], "config directory gained files")
    check((h.locks / "sync.lock").exists(), "fixed lock file was not created in the lock mount")

    # Mounts, read-only config and token injection are checked on a live container.
    name = f"strmgen-3-2-mounts-{h.run_id}"
    args = h.common_run_args(cfg, name, rm=False)
    args = args[:1] + ["-d"] + args[1:]
    args += ["-e", "OPENLIST_TOKEN", "--entrypoint", "/bin/sh",
             image, "-c", "mount | grep strmgen; if [ -n \"$OPENLIST_TOKEN\" ]; then echo TOKEN_SET; else echo TOKEN_MISSING; fi; sleep 30"]
    started = docker(args, timeout=60, env=h._env())
    check(started.returncode == 0, f"mount probe failed to start: {started.stderr}")
    h.containers.append(name)
    inspect = docker(["inspect", "--format", "{{json .Mounts}}", name], timeout=30)
    mounts = json.loads(inspect.stdout)
    targets = {m["Destination"]: m for m in mounts}
    for target, expected in (("/etc/strmgen/config.yaml", True), ("/output", False),
                             ("/state", False), ("/tmp/strmgen-locks", False)):
        check(target in targets, f"missing mount {target}: {sorted(targets)}")
        if expected:
            check(targets[target]["RW"] is False, f"{target} must be read-only")
    logs = docker(["logs", name], timeout=30)
    h.remove(name)
    # The token must be present in the running container but must never be
    # printed: the probe reports only a boolean marker, and the marker value
    # itself must not appear in the container logs.
    check("TOKEN_SET" in logs.stdout, "token environment was not injected into the container")
    check("TOKEN_MISSING" not in logs.stdout, "token environment was empty in the container")
    check(SECRET_MARKER not in logs.stdout and SECRET_MARKER not in logs.stderr,
          "container logs leaked the runtime token value")
    check("strmgen-locks" in logs.stdout and "ro" in logs.stdout,
          f"read-only config mount not visible: {logs.stdout}")

    results["minimal_run"] = {
        "exit": result.returncode, "ownership": len(state["ownership"]),
        "rounds": state["rounds"][0][1], "files": len(files),
    }


def scenario_serve_lifecycle(h, results, image):
    api = h.server()
    cfg = h.write_config("serve", api, interval_minutes=60)
    name = h.start_detached(cfg, f"strmgen-3-2-serve-{h.run_id}")
    db = h.state / "strmgen.db"
    check(wait_for(lambda: (h.out / "Movies" / "Sample.strm").exists(), 90,
                   "serve container did not deliver its first round"),
          "serve first round did not create output")
    check(wait_for(lambda: epoch(db) == 1, 30,
                   "serve first round did not commit a round"),
          f"serve round not committed: {read_db(db)['rounds']}")
    before = snapshot(h.out)
    exit_code = h.stop(name, timeout=60)
    check(exit_code != 0, f"cancelled serve container must exit non-zero, got {exit_code}")

    # The flock must be free: a one-shot container completes and skips.
    rerun = h.run_once(cfg, f"strmgen-3-2-after-{h.run_id}", timeout=120)
    check(rerun.returncode == 0 and "skipped=4" in rerun.stdout,
          f"post-stop sync failed or did not skip: {rerun.returncode} {rerun.stdout}")
    check(snapshot(h.out) == before, "post-stop sync changed output bytes or mtime")
    rounds_after_rerun = epoch(db)
    check(rounds_after_rerun == 2, f"post-stop sync should commit round 2: {read_db(db)['rounds']}")

    # Restart the same container from persisted state: stable skip, no rewrite.
    restarted = docker(["start", name], timeout=60)
    check(restarted.returncode == 0, f"docker start failed: {restarted.stderr}")
    check(wait_for(lambda: epoch(db) == rounds_after_rerun + 1, 60,
                   "restarted serve did not complete a further round"),
          f"restart did not advance the round: {read_db(db)['rounds']}")
    check(snapshot(h.out) == before, "restart rewrote stable output")
    exit_code2 = h.stop(name, timeout=60)
    check(exit_code2 != 0, f"restarted serve must exit non-zero, got {exit_code2}")
    results["serve_lifecycle"] = {
        "first_stop_exit": exit_code, "restart_stop_exit": exit_code2,
        "post_stop_exit": rerun.returncode, "rounds": epoch(db),
    }


def scenario_cross_container_busy(h, results, image):
    entered = threading.Event()
    release = threading.Event()

    def before(endpoint, path, request):
        if endpoint.endswith("/list") and path == "/Movies" and not entered.is_set():
            entered.set()
            release.wait(timeout=60)

    api = h.server(hooks={"before": before})
    cfg = h.write_config("busy", api)
    holder = h.start_detached(cfg, f"strmgen-3-2-holder-{h.run_id}",
                              command=["sync", "--config", "/etc/strmgen/config.yaml"])
    check(entered.wait(timeout=30), "holder container never reached the API barrier")

    # Second container, same output and same lock mount, must be excluded.
    # The holder already created the database before the barrier, so compare the
    # bytes and rows around the contender instead of asserting absence.
    db = h.state / "strmgen.db"
    db_bytes_before = db.read_bytes() if db.exists() else None
    rounds_before = read_db(db)["rounds"]
    contender = h.run_once(cfg, f"strmgen-3-2-contender-{h.run_id}", timeout=120)
    check(contender.returncode == 1, f"contender must be excluded, got {contender.returncode}")
    check("another sync is already running" in contender.stderr,
          f"busy message missing: {contender.stderr}")
    check((db.read_bytes() if db.exists() else None) == db_bytes_before,
          "busy contender changed the database bytes")
    check(read_db(db)["rounds"] == rounds_before,
          "busy contender committed a round before the holder finished")

    release.set()
    check(wait_for(lambda: docker(["inspect", "--format", "{{.State.Running}}", holder],
                                  timeout=30).stdout.strip() == "false", 60,
                   "holder did not finish"), "holder did not finish")
    holder_logs = docker(["logs", holder], timeout=30)
    check("created=4" in holder_logs.stdout, f"holder did not create output: {holder_logs.stdout}")
    h.remove(holder)
    check(epoch(db) == 1, "holder did not commit exactly one round")

    # After release a fresh container can run and skip.
    after = h.run_once(cfg, f"strmgen-3-2-afterbusy-{h.run_id}", timeout=120)
    check(after.returncode == 0 and "skipped=4" in after.stdout,
          f"post-busy sync failed: {after.returncode} {after.stdout}")
    results["cross_container_busy"] = {
        "contender_exit": contender.returncode, "post_busy_exit": after.returncode,
        "rounds": epoch(db),
    }


def scenario_compose_lifecycle(h, results, image):
    api = h.server()
    cfg = h.write_config("compose", api, interval_minutes=60)
    project = f"strmgen32{h.run_id}"
    h.compose_project = project
    env = compose_env(h, cfg, image)
    up = subprocess.run(["docker", "compose", "-f", str(ROOT / "deploy" / "compose.yaml"),
                         "-p", project, "up", "-d"], capture_output=True, text=True,
                        timeout=180, env=env)
    check(up.returncode == 0, f"compose up failed: {up.stderr}")
    container = f"{project}-strmgen-1"
    try:
        db = h.state / "strmgen.db"
        check(wait_for(lambda: (h.out / "Movies" / "Sample.strm").exists(), 90,
                       "compose serve did not deliver its first round"),
              "compose first round did not create output")
        check(wait_for(lambda: epoch(db) == 1, 60,
                       "compose serve did not commit its first round"),
              f"compose first round not committed: {read_db(db)['rounds']}")
        before = snapshot(h.out)
        check(len(before) == 4, f"compose first round wrote {len(before)} files, want 4")
        # Runtime token injection is proven without printing the value: the
        # synthetic service only accepts SECRET_MARKER, so a delivered first
        # round already required it. Confirm the running container has the
        # variable using a boolean marker only.
        token_probe = docker(["exec", container, "sh", "-c",
                              'if [ -n "$OPENLIST_TOKEN" ]; then echo TOKEN_SET; else echo TOKEN_MISSING; fi'],
                             timeout=30)
        check(token_probe.returncode == 0, f"compose token probe failed: {token_probe.stderr}")
        check(token_probe.stdout.strip() == "TOKEN_SET",
              f"compose container did not receive the runtime token: {token_probe.stdout!r}")
        check(SECRET_MARKER not in token_probe.stdout, "compose token probe leaked the value")
        stop = subprocess.run(["docker", "compose", "-f", str(ROOT / "deploy" / "compose.yaml"),
                               "-p", project, "stop", "-t", "60"], capture_output=True,
                              text=True, timeout=120, env=env)
        check(stop.returncode == 0, f"compose stop failed: {stop.stderr}")
        exit_code = int(docker(["inspect", "--format", "{{.State.ExitCode}}", container],
                               timeout=30).stdout.strip())
        check(exit_code != 0, f"compose serve must exit non-zero on stop, got {exit_code}")
        # The committed compose must use a runtime pass-through declaration and
        # must never bake a value into a rendered file or the evidence. On this
        # Compose version plain `config` resolves the operator's own host
        # environment (by design for `environment:` injection), so the
        # redaction assertion uses `--no-interpolate`, which renders the
        # declaration without any value. The pass-through form is also checked
        # so a value interpolation cannot silently return.
        nointerp = subprocess.run(["docker", "compose", "-f", str(ROOT / "deploy" / "compose.yaml"),
                                   "-p", project, "config", "--no-interpolate"], capture_output=True,
                                  text=True, timeout=60, env=env)
        check(nointerp.returncode == 0, f"compose config --no-interpolate failed: {nointerp.stderr}")
        check(SECRET_MARKER not in nointerp.stdout and SECRET_MARKER not in nointerp.stderr,
              "compose --no-interpolate render leaked the runtime token")
        check("${OPENLIST_TOKEN" not in nointerp.stdout,
              "compose must use a pass-through declaration, not value interpolation")
        # The running container did receive the token: the synthetic service
        # only accepts SECRET_MARKER, so the committed first round above already
        # required it, and the boolean probe below confirms the variable without
        # printing its value.
        check(snapshot(h.out) == before, "compose stop changed output")
    finally:
        subprocess.run(["docker", "compose", "-f", str(ROOT / "deploy" / "compose.yaml"),
                        "-p", project, "down", "-v", "--remove-orphans"],
                       capture_output=True, text=True, timeout=120, env=env)
        h.compose_project = None
    results["compose_lifecycle"] = {"stop_exit": exit_code}


def scenario_arm64_oci(h, results, arm64_oci):
    check(arm64_oci is not None, "arm64 OCI archive not provided (--arm64-oci)")
    summary = inspect_oci(arm64_oci)
    arm = [m for m in summary["manifests"]
           if m.get("os") == "linux" and m.get("architecture") == "arm64"]
    check(arm, f"no linux/arm64 manifest in OCI archive: {summary['manifests']}")
    check(arm[0].get("config_architecture") == "arm64",
          f"arm64 config architecture mismatch: {arm[0]}")
    check(arm[0].get("config_user") == f"{h.uid}:{h.gid}",
          f"arm64 image user mismatch: {arm[0].get('config_user')}")
    binaries = [b for b in summary["binaries"] if b["path"].endswith("bin/strmgen")]
    check(binaries, f"no strmgen binary found in arm64 layers: {summary['binaries']}")
    check(binaries[0]["elf_machine"] == 183,  # EM_AARCH64
          f"arm64 binary ELF machine {binaries[0]['elf_machine']} != 183")
    results["arm64_oci"] = {
        "sha256": summary["sha256"], "manifest_digest": arm[0]["digest"],
        "config_architecture": arm[0]["config_architecture"],
        "elf_machine": binaries[0]["elf_machine"], "binary_size": binaries[0]["size"],
    }


SCENARIOS = {
    "image_static": scenario_image_static,
    "minimal_run": scenario_minimal_run,
    "serve_lifecycle": scenario_serve_lifecycle,
    "cross_container_busy": scenario_cross_container_busy,
    "compose_lifecycle": scenario_compose_lifecycle,
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
    parser.add_argument("--image", required=True, help="loaded product image tag")
    parser.add_argument("--arm64-oci", help="arm64 OCI archive produced by buildx")
    parser.add_argument("--scenario", default="all",
                        help="'all' or a comma-separated subset: " + ", ".join(ALL_SCENARIOS))
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    output_dir = validate_output_dir(Path(args.output_dir))
    image = args.image
    if not docker_available():
        payload = {"status": "BLOCKED", "reason": "docker engine unavailable", "image": image}
        (output_dir / "summary.json").write_text(json.dumps(payload, indent=2) + "\n")
        print(json.dumps(payload))
        return 3

    inspect = docker(["image", "inspect", image, "--format", "{{.Id}}"], timeout=60)
    if inspect.returncode != 0:
        payload = {"status": "BLOCKED", "reason": f"image {image} not loaded", "image": image}
        (output_dir / "summary.json").write_text(json.dumps(payload, indent=2) + "\n")
        print(json.dumps(payload))
        return 3

    if args.scenario == "all":
        selected = list(ALL_SCENARIOS)
        if args.arm64_oci:
            selected.append("arm64_oci")
    else:
        selected = [name.strip() for name in args.scenario.split(",") if name.strip()]
    unknown = [name for name in selected if name not in SCENARIOS and name != "arm64_oci"]
    if unknown:
        raise SystemExit("unknown scenario(s): " + ", ".join(unknown))

    run_id = secrets.token_hex(4)
    results = {}
    failures = []
    for name in selected:
        workdir = output_dir / "run" / name
        if workdir.exists():
            raise SystemExit(f"scenario work directory already exists: {workdir}")
        harness = Harness(image, workdir, run_id)
        started = time.time()
        try:
            if name == "arm64_oci":
                scenario_arm64_oci(harness, results, Path(args.arm64_oci) if args.arm64_oci else None)
            else:
                SCENARIOS[name](harness, results, image)
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
        "image": image,
        "image_id": inspect.stdout.strip(),
        "arm64_oci": args.arm64_oci,
        "scenarios": results,
        "failures": failures,
    }
    (output_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({"scenarios": {name: results.get(name, {}).get("status") for name in selected},
                      "failures": failures}, ensure_ascii=False))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
