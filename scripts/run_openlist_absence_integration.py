#!/usr/bin/env python3
"""Isolated real-OpenList acceptance driver for Task 2.1 absence confirmation.

The driver starts its own fixed-version OpenList container, creates a dedicated
Local storage plus a WebDav storage that re-mounts the same tree through
OpenList's own DAV endpoint with a short cache TTL, materializes every fixture,
and then runs the Go integration test binary against that service. Every list
request, including preparation, polling and evidence capture, is sent with an
explicit refresh:false; a forced refresh or a cache flush is never used.

It never touches the shared dev fixture on 15244, production OpenList, Jellyfin,
its proxy, the companion repository or production media. Every file, database,
container and log lives under the requested evidence directory.

Usage:
    python3 scripts/run_openlist_absence_integration.py \\
        --test-binary artifacts/task-2.1/bin/openlist-integration.test \\
        --output-dir artifacts/validation-evidence/task-2.1-final-XXXXXX/real-openlist

The Go test binary is built with:
    bash scripts/go-local test -c -tags openlist_integration \\
        -o <path> ./internal/openlist
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import secrets
import shutil
import socket
import sqlite3
import string
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
EVIDENCE_ROOT = (ROOT / "artifacts" / "validation-evidence").resolve()

# Fixed identity from dev/environment-spec.json. Panic instead of drifting.
OPENLIST_SOURCE_SHA = "3b28270a4066ea52943a69ecddd850d83f7e8bdc"
OPENLIST_IMAGE = ("openlistteam/openlist@sha256:"
                  "11b490123d57f88c84ecda46285e1921869cf511da3e5bb4062f183e7e1200b7")
OPENLIST_VERSION = "v4.2.7"
CONTAINER_PORT = 5244

PROJECT_PREFIX = "strmgen-absence"
DEFAULT_TIMEOUT = 600

# Every scenario registers here so a failure cannot leak a running container.
_LIVE = []


class Blocked(RuntimeError):
    """A precondition for real-service evidence is missing (not a product bug)."""


class Failure(AssertionError):
    """A real-service observation contradicted the expected safe behaviour."""


def log(message):
    print(message, flush=True)


def run(argv, *, cwd=None, env=None, timeout=DEFAULT_TIMEOUT, check=True, capture=True):
    """Run one command and return its CompletedProcess with full output."""
    result = subprocess.run(argv, cwd=cwd, env=env, timeout=timeout,
                            text=True, capture_output=capture)
    if check and result.returncode != 0:
        raise Failure(f"command failed ({result.returncode}): {' '.join(argv)}\n"
                      f"stdout:\n{result.stdout}\nstderr:\n{result.stderr}")
    return result


def docker_available():
    try:
        run(["docker", "version", "--format", "{{.Server.Version}}"], timeout=60)
        return True
    except (OSError, subprocess.SubprocessError, Failure):
        return False


def image_revision():
    """Return (revision, version, digest) for the pinned image, or raise Blocked."""
    inspect = run(["docker", "image", "inspect", OPENLIST_IMAGE,
                   "--format", "{{json .RepoDigests}}|{{index .Config.Labels \"org.opencontainers.image.revision\"}}|"
                               "{{index .Config.Labels \"org.opencontainers.image.version\"}}|{{.Id}}"],
                  timeout=120)
    parts = inspect.stdout.strip().split("|")
    if len(parts) != 4:
        raise Blocked("unexpected docker image inspect output")
    digests, revision, version, image_id = parts
    if revision != OPENLIST_SOURCE_SHA:
        raise Blocked(f"pinned image revision {revision!r} does not match {OPENLIST_SOURCE_SHA}")
    if version != OPENLIST_VERSION:
        raise Blocked(f"pinned image version {version!r} does not match {OPENLIST_VERSION}")
    digests = json.loads(digests)
    return revision, version, digests, image_id


def version_banner():
    result = run(["docker", "run", "--rm", "--entrypoint", "/opt/openlist/openlist",
                  OPENLIST_IMAGE, "version"], timeout=180)
    return result.stdout.strip()


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def random_text(prefix="", length=16):
    alphabet = string.ascii_lowercase + string.digits
    return prefix + "".join(secrets.choice(alphabet) for _ in range(length))


class Api:
    """Minimal OpenList HTTP client used only by this harness."""

    def __init__(self, base):
        self.base = base
        self.token = ""

    def call(self, path, body=None, *, token=None, method="POST", timeout=60, raw=False):
        data = json.dumps(body if body is not None else {}).encode() if method == "POST" else None
        request = urllib.request.Request(self.base + path, data=data, method=method)
        request.add_header("Content-Type", "application/json")
        auth = token if token is not None else self.token
        if auth:
            request.add_header("Authorization", auth)
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                payload = response.read()
                status = response.status
        except urllib.error.HTTPError as error:
            return error.code, error.read()[:4096].decode("utf-8", "replace")
        except (urllib.error.URLError, TimeoutError, OSError) as error:
            raise Blocked(f"service unreachable: {type(error).__name__}") from error
        if raw:
            return status, payload
        try:
            return status, json.loads(payload)
        except json.JSONDecodeError as error:
            raise Failure(f"non-JSON response from {path}") from error

    def ok(self, path, body=None, **kwargs):
        status, payload = self.call(path, body, **kwargs)
        if status != 200 or not isinstance(payload, dict) or payload.get("code") != 200:
            message = payload.get("message") if isinstance(payload, dict) else str(payload)[:200]
            raise Failure(f"{path} failed: http={status} msg={str(message)[:200]}")
        return payload.get("data")

    def login(self, username, password):
        data = self.ok("/api/auth/login", {"username": username, "password": password}, token="")
        token = data.get("token") if isinstance(data, dict) else None
        if not token:
            raise Failure("login returned no token")
        return token

    def me(self, token=None):
        status, payload = self.call("/api/me", {}, token=token, method="GET")
        if status != 200 or not isinstance(payload, dict):
            raise Failure("could not read /api/me")
        return payload.get("data") or {}


class Service:
    """One dedicated OpenList container plus its media tree and state."""

    def __init__(self, workdir, port=None):
        self.workdir = workdir
        self.name = f"{PROJECT_PREFIX}-{random_text(length=10)}"
        self.port = port or free_port()
        self.data_dir = workdir / "state"
        self.media_dir = workdir / "media"
        self.log_path = workdir / "service.log"
        self.started = False
        self.password = "probe-" + random_text(length=20)
        self.api = Api(f"http://127.0.0.1:{self.port}")

    def start(self):
        self.data_dir.mkdir(parents=True, exist_ok=True)
        self.media_dir.mkdir(parents=True, exist_ok=True)
        # The pinned image runs as the unprivileged `openlist` user (UID/GID
        # 1001) and refuses to start unless its data directory is writable by
        # that identity. The directories are created specifically for this
        # throwaway container under the evidence tree, so granting UID 1001
        # access here widens nothing else on the host.
        for directory in (self.data_dir, self.media_dir):
            os.chmod(directory, 0o777)
        argv = [
            "docker", "run", "-d", "--name", self.name,
            "--label", "strmgen.task=2.1",
            "-p", f"127.0.0.1:{self.port}:{CONTAINER_PORT}",
            "-v", f"{self.data_dir}:/opt/openlist/data",
            "-v", f"{self.media_dir}:/media",
            "--pids-limit", "512",
            OPENLIST_IMAGE,
        ]
        run(argv, timeout=300)
        _LIVE.append(self)
        self.started = True
        self._wait_ready()
        self._initialize()

    def _wait_ready(self, timeout=180):
        deadline = time.monotonic() + timeout
        last = ""
        while time.monotonic() < deadline:
            try:
                status, payload = self.api.call("/api/public/init_status", {}, token="", method="GET", timeout=5)
                if status == 200 and isinstance(payload, dict):
                    return
            except Blocked:
                pass
            last = self._logs_tail()
            time.sleep(0.5)
        raise Blocked("isolated OpenList did not become ready\n" + last)

    def _logs_tail(self, lines=40):
        if not (self.data_dir.parent / "service.log").exists():
            try:
                result = run(["docker", "logs", "--tail", str(lines), self.name], check=False)
                return result.stdout[-4000:] + result.stderr[-4000:]
            except (OSError, subprocess.SubprocessError):
                return ""
        return ""

    def _initialize(self):
        self.api.ok("/api/public/init/setup",
                    {"username": "admin", "password": self.password, "site_title": "strmgen isolation"},
                    token="")
        self.api.token = self.api.login("admin", self.password)

    def add_local_storage(self, mount, root_folder, **extra):
        addition = {"root_folder_path": root_folder, "thumbnail": False}
        addition.update(extra)
        data = self.api.ok("/api/admin/storage/create", {
            "mount_path": mount,
            "driver": "Local",
            "addition": json.dumps(addition),
        })
        return data["id"]

    def add_webdav_storage(self, mount, root_folder, cache_minutes, username, password):
        addition = {
            "root_folder_path": root_folder,
            "address": f"http://127.0.0.1:{CONTAINER_PORT}/dav",
            "username": username,
            "password": password,
            "vendor": "other",
        }
        data = self.api.ok("/api/admin/storage/create", {
            "mount_path": mount,
            "driver": "WebDav",
            "cache_expiration": cache_minutes,
            "addition": json.dumps(addition),
        })
        return data["id"]

    def create_user(self, username, password, permission, role=0):
        self.api.ok("/api/admin/user/create", {
            "username": username,
            "password": password,
            "role": role,
            "base_path": "/",
            "permission": permission,
        })
        return self.api.login(username, password)

    def add_meta(self, path, **fields):
        payload = {"path": path}
        payload.update(fields)
        self.api.ok("/api/admin/meta/create", payload)

    def disable_storage(self, storage_id):
        self.api.ok(f"/api/admin/storage/disable?id={storage_id}")

    def enable_storage(self, storage_id):
        self.api.ok(f"/api/admin/storage/enable?id={storage_id}")

    def list_once(self, path, *, token=None, page=1, per_page=200):
        """List one page with an explicit refresh:false and return (status, payload)."""
        return self.api.call("/api/fs/list",
                             {"path": path, "page": page, "per_page": per_page,
                              "refresh": False}, token=token)

    def get_once(self, path, *, token=None):
        """Read one exact object and return (status, payload) without interpretation."""
        return self.api.call("/api/fs/get", {"path": path}, token=token)

    def wait_listed(self, path, expected_names, timeout=60):
        """Wait until the real service reports exactly the expected names.

        Every poll is a cached list (refresh:false), which is the only listing
        mode this program may use. When the backing store changes, waiting is
        bounded and explicit instead of relying on a forced refresh.
        """
        deadline = time.monotonic() + timeout
        last = None
        while time.monotonic() < deadline:
            status, payload = self.list_once(path)
            if status == 200 and isinstance(payload, dict) and payload.get("code") == 200:
                data = payload["data"]
                names = sorted(entry["name"] for entry in (data.get("content") or []))
                last = names
                if names == sorted(expected_names):
                    return
            time.sleep(0.2)
        raise Failure(f"{path} never listed {sorted(expected_names)} (last={last})")

    def stop(self):
        if not self.started:
            return
        try:
            info = run(["docker", "inspect", self.name, "--format",
                        "{{json .State}}|{{index .Config.Labels \"strmgen.task\"}}"], check=False, timeout=120)
            if info.returncode != 0:
                self.started = False
                return
            state, label = info.stdout.strip().split("|")
            if label != "2.1":
                log(f"refusing to stop {self.name}: label mismatch")
                return
            logs = run(["docker", "logs", self.name], check=False, timeout=120)
            (self.workdir / "container.log").write_text(
                (logs.stdout or "") + "\n--- stderr ---\n" + (logs.stderr or ""))
            run(["docker", "rm", "-f", self.name], check=False, timeout=180)
        finally:
            self.started = False
            if self in _LIVE:
                _LIVE.remove(self)


def cleanup_all():
    for service in list(_LIVE):
        try:
            service.stop()
        except (OSError, subprocess.SubprocessError, Failure) as error:  # pragma: no cover - best effort
            log(f"cleanup warning: {error}")


# --------------------------------------------------------------------------
# fixtures


def write_file(base: Path, name: str, content: bytes = b"x"):
    path = base / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(content)
    return path


def populate(tree: Path):
    """Create the shared fixture tree. Names are deliberately awkward."""
    write_file(tree, "Movies/A Movie.mkv", b"a" * 64)
    write_file(tree, "Movies/a movie.mkv", b"b" * 65)
    write_file(tree, "Movies/Sub Dir/中文 名字.mkv", b"c" * 66)
    write_file(tree, "Movies/Sub Dir/#hash% & \"quote\".mkv", b"d" * 67)
    write_file(tree, "Movies/Sub Dir/Deep/Nested.mkv", b"e" * 68)
    write_file(tree, "TV/Removed Later/Season 1.mkv", b"f" * 69)
    (tree / "Empty").mkdir(parents=True, exist_ok=True)
    # A wide directory forces real pagination: 250 entries spanning two pages.
    wide = tree / "Wide"
    wide.mkdir(parents=True, exist_ok=True)
    for index in range(250):
        (wide / f"E{index:04d}.mkv").write_bytes(b"w")
    return {
        "wide_total": 250,
        "last_page_file": "E0249.mkv",
    }


def sha256_of(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


# --------------------------------------------------------------------------
# scenarios


class Harness:
    """One scenario's own service, so scenarios cannot influence each other."""

    def __init__(self, workdir):
        self.workdir = workdir
        self.workdir.mkdir(parents=True, exist_ok=True)
        self.service = Service(workdir)
        self.tree = self.service.media_dir / "Tree"
        # The media directory is bind-mounted at /media inside the container, so
        # the Local driver must be pointed at the container-side path.
        self.container_tree = "/media/Tree"
        self.facts = {}

    def __enter__(self):
        self.service.start()
        populate(self.tree)
        self.service.add_local_storage("/lib", self.container_tree)
        self.service.wait_listed("/lib", ["Empty", "Movies", "TV", "Wide"])
        return self

    def __exit__(self, *exc):
        self.service.stop()
        return False


def record_get(service, label, path, observations, *, token=None):
    """Record one raw Get observation (status, application code, object shape)."""
    status, payload = service.get_once(path, token=token)
    data = payload.get("data") if isinstance(payload, dict) else None
    observations[label] = {
        "name_label": path.rsplit("/", 1)[-1] or "/",
        "http": status,
        "code": payload.get("code") if isinstance(payload, dict) else None,
        "is_dir": data.get("is_dir") if isinstance(data, dict) else None,
        "provider": data.get("provider") if isinstance(data, dict) else None,
    }


def record_list(service, label, path, observations, *, token=None, page=1):
    """Record one raw List observation, always with an explicit refresh:false."""
    status, payload = service.list_once(path, token=token, page=page)
    data = payload.get("data") if isinstance(payload, dict) else None
    observations[label] = {
        "http": status,
        "code": payload.get("code") if isinstance(payload, dict) else None,
        "names": sorted(entry["name"] for entry in (data.get("content") or []))
        if isinstance(data, dict) and payload.get("code") == 200 else None,
        "refresh": False,
    }


def scenario_exact_names(harness):
    service = harness.service
    special = ["/lib/Movies/Sub Dir/中文 名字.mkv", "/lib/Movies/Sub Dir/#hash% & \"quote\".mkv"]
    facts = {
        "present_file": "/lib/Movies/A Movie.mkv",
        "absent_file": "/lib/Movies/Missing.mkv",
        "case_variant": "/lib/Movies/A MOVIE.mkv",
        "special_files": special,
    }
    # Record what the real API answers for each read so the evidence shows the
    # observed response shape, not only the Go conclusion.
    observations = {}
    for label, path in (("root", "/lib"),
                        ("present", facts["present_file"]),
                        ("case_variant", facts["case_variant"]),
                        ("absent", facts["absent_file"]),
                        ("directory_target", "/lib/Movies")):
        record_get(service, label, path, observations)
    return {"exact_names": facts}, {"exact_names_observations": observations}


def scenario_pagination(harness):
    service = harness.service
    facts = {
        "directory": "/lib/Wide",
        "last_page_file": "/lib/Wide/E0249.mkv",
        "absent_file": "/lib/Wide/NoSuch.mkv",
        "total": 250,
    }
    # Confirm the service really paginates at the client page size, using the
    # only listing mode the program may use.
    observations = {}
    record_list(service, "pagination_page_two", "/lib/Wide", observations, page=2)
    return {"pagination": facts}, observations


def scenario_empty_directory(harness):
    service = harness.service
    status, payload = service.list_once("/lib/Empty")
    data = payload.get("data") if isinstance(payload, dict) else None
    if not isinstance(data, dict):
        raise Failure("empty directory response has no data object")
    observations = {"empty_directory_shape": {
        "http": status,
        "code": payload.get("code"),
        "content_is_null": data.get("content") is None,
        "content_type": type(data.get("content")).__name__,
        "total": data.get("total"),
        "refresh": False,
    }}
    record_get(service, "empty_directory_target", "/lib/Empty", observations)
    return {"empty_directory": {"directory": "/lib/Empty"}}, observations


def scenario_removed_parent(harness):
    """Contrast an unreadable source root with a removed parent inside a source.

    Measured on the pinned build:
    - removing the backing directory of a Local mount leaves its mount path
      reading as a directory (provider Local), so that root stays readable;
    - disabling a storage makes its mount path unreadable (application 500);
    - removing a directory inside a readable source makes the deep target
      unreadable while its source root still reads.
    """
    service = harness.service

    # A removable parent inside the reachable source.
    removed_rel = "TV/Removed Later"
    removed_target = removed_rel + "/Season 1.mkv"
    shutil.rmtree(harness.tree / removed_rel)

    # A Local mount whose backing directory is gone: root still reads.
    gone_root = harness.tree / "GoneRoot"
    write_file(gone_root, "Movies/A.mkv", b"o")
    service.add_local_storage("/gone", harness.container_tree + "/GoneRoot")
    service.wait_listed("/gone", ["Movies"])
    shutil.rmtree(gone_root)

    # A disabled storage: its mount path is unreadable.
    disabled_tree = harness.tree / "Disabled"
    write_file(disabled_tree, "Only.mkv", b"d")
    disabled_id = service.add_local_storage("/disabled", harness.container_tree + "/Disabled")
    service.wait_listed("/disabled", ["Only.mkv"])
    service.disable_storage(disabled_id)

    observations = {}
    record_get(service, "readable_root", "/lib", observations)
    record_get(service, "removed_inside_source", "/lib/" + removed_target, observations)
    record_get(service, "backing_dir_gone_root", "/gone", observations)
    record_get(service, "backing_dir_gone_target", "/gone/Movies/A.mkv", observations)
    record_get(service, "disabled_root", "/disabled", observations)
    return {
        "removed_parent": {
            "readable_root": "/lib",
            "removed_parent": "/lib/" + removed_rel,
            "removed_inside_source": "/lib/" + removed_target,
            "backing_dir_gone_root": "/gone",
            "backing_dir_gone_target": "/gone/Movies/A.mkv",
            "disabled_root": "/disabled",
        },
    }, observations


def scenario_read_only_identity(harness):
    """A read-only identity needs no refresh permission under the new rule."""
    service = harness.service
    read_only = service.create_user("reader-" + random_text(length=6),
                                    "reader-" + random_text(length=12), permission=0)
    observations = {}
    record_list(service, "read_only_list", "/lib", observations, token=read_only)
    record_get(service, "read_only_present", "/lib/Movies/A Movie.mkv", observations, token=read_only)
    record_get(service, "read_only_absent", "/lib/Movies/Missing.mkv", observations, token=read_only)
    return {"read_only_identity": {
        "read_only_token": read_only,
        "present_file": "/lib/Movies/A Movie.mkv",
        "absent_file": "/lib/Movies/Missing.mkv",
    }}, observations


# The cached driver is the WebDav self-mount, the only real driver in the pinned
# build that serves a server-side directory cache. Its cache lifetime comes from
# the storage's cache_expiration; the runner waits that lifetime out so the
# evidence is about a naturally updated cache instead of a forced refresh.
CACHE_TTL_MINUTES = 1
CACHE_WAIT_SECONDS = 150


def scenario_cached_driver(harness):
    service = harness.service
    service.add_webdav_storage("/cached", "/lib", CACHE_TTL_MINUTES, "admin", service.password)
    service.wait_listed("/cached", ["Empty", "Movies", "TV", "Wide"])
    # Warm the server-side directory cache for the target directory.
    service.api.ok("/api/fs/list",
                   {"path": "/cached/Movies", "page": 1, "per_page": 200, "refresh": False})
    observations = {}
    record_list(service, "cache_warm_listing", "/cached/Movies", observations)
    record_get(service, "cache_root", "/cached", observations)
    record_get(service, "cache_directory", "/cached/Movies", observations)
    # Measured limitation: the WebDav self-mount cannot serve a direct object
    # read of a file, so even a present file is not read through this driver.
    record_get(service, "cache_present_file_read", "/cached/Movies/A Movie.mkv", observations)

    deleted_path = "/cached/Movies/A Movie.mkv"
    created_path = "/cached/Movies/Reappeared.mkv"
    (harness.tree / "Movies" / "A Movie.mkv").unlink()
    write_file(harness.tree, "Movies/Reappeared.mkv", b"r")

    # Wait until the still-cached listing naturally reflects the change. No
    # refresh and no cache flush is used; the loop only polls with
    # refresh:false and reports the elapsed time.
    started = time.monotonic()
    updated = False
    polls = 0
    deadline = started + CACHE_WAIT_SECONDS
    while time.monotonic() < deadline:
        polls += 1
        status, payload = service.list_once("/cached/Movies")
        data = payload.get("data") if isinstance(payload, dict) else None
        names = sorted(entry["name"] for entry in (data.get("content") or [])) \
            if isinstance(data, dict) and payload.get("code") == 200 else []
        if "Reappeared.mkv" in names and "A Movie.mkv" not in names:
            updated = True
            break
        time.sleep(1)
    observations["cache_natural_update"] = {
        "updated": updated,
        "polls": polls,
        "elapsed_s": round(time.monotonic() - started, 1),
        "ttl_minutes": CACHE_TTL_MINUTES,
    }
    if not updated:
        raise Failure("the cached listing never reflected the backing-store change within the TTL window")

    record_list(service, "cache_after_update_listing", "/cached/Movies", observations)
    record_get(service, "cache_deleted_read", deleted_path, observations)
    record_get(service, "cache_created_read", created_path, observations)
    return {"cached_driver": {
        "mount": "/cached",
        "deleted_after_cache": deleted_path,
        "created_after_cache": created_path,
        "present_file": "/cached/Movies/A Movie.mkv",
    }}, observations


def scenario_nested_mount_masking(harness):
    """Record a nested-mount view and an unreadable root side by side.

    Measured on the pinned build: when a nested storage exists below a mount
    path and the parent storage is disabled, the parent path still reads as a
    directory with provider "unknown" while only the nested entries list. Under
    the accepted rule that readable directory continues, so a target that is not
    read becomes a missing observation: the hazard is a documented limitation.
    """
    service = harness.service
    nested_root = harness.service.media_dir / "Nested"
    write_file(nested_root, "Inside.mkv", b"n")
    base_id = service.add_local_storage("/mask", harness.container_tree)
    service.wait_listed("/mask", ["Empty", "Movies", "TV", "Wide"])
    service.add_local_storage("/mask/Movies/Nested", "/media/Nested")
    # The nested mount appears in its parent listing next to the real entries.
    service.wait_listed("/mask/Movies", ["A Movie.mkv", "Nested", "Sub Dir", "a movie.mkv"])
    service.disable_storage(base_id)

    # A separate disabled storage with no nested mount: its root is unreadable.
    disabled_tree = harness.tree / "MaskedOffline"
    write_file(disabled_tree, "Only.mkv", b"d")
    disabled_id = service.add_local_storage("/masked-offline", harness.container_tree + "/MaskedOffline")
    service.wait_listed("/masked-offline", ["Only.mkv"])
    service.disable_storage(disabled_id)

    observations = {}
    record_list(service, "masking_list", "/mask/Movies", observations)
    record_list(service, "masking_root_list", "/mask", observations)
    record_get(service, "mask_virtual_root", "/mask", observations)
    record_get(service, "mask_virtual_ancestor", "/mask/Movies", observations)
    record_get(service, "mask_virtual_target", "/mask/Movies/NotThere.mkv", observations)
    record_get(service, "masked_offline_root", "/masked-offline", observations)
    return {"nested_mount_masking": {
        "mount": "/mask",
        "virtual_ancestor": "/mask/Movies",
        "disabled_root": "/masked-offline",
    }}, observations


def scenario_hidden_entries(harness):
    service = harness.service
    service.add_meta("/lib/Movies", hide="A Movie", h_sub=False)
    restricted = service.create_user("hidden-" + random_text(length=6),
                                     "hidden-" + random_text(length=12), permission=0)
    observations = {}
    record_list(service, "hidden_list", "/lib/Movies", observations, token=restricted)
    record_get(service, "hidden_restricted_read", "/lib/Movies/A Movie.mkv", observations, token=restricted)
    record_get(service, "hidden_unrestricted_read", "/lib/Movies/A Movie.mkv", observations)
    return {"hidden_entries": {"hidden_name": "/lib/Movies/A Movie.mkv",
                               "restricted_token": restricted}}, observations


SCENARIOS = {
    "exact_names": scenario_exact_names,
    "pagination": scenario_pagination,
    "empty_directory": scenario_empty_directory,
    "removed_parent": scenario_removed_parent,
    "read_only_identity": scenario_read_only_identity,
    "cached_driver": scenario_cached_driver,
    "nested_mount_masking": scenario_nested_mount_masking,
    "hidden_entries": scenario_hidden_entries,
}

# Which Go test each scenario gates.
GO_TESTS = {
    "exact_names": ("TestIntegrationConfirmPresentAndAbsentAgainstRealOpenList"
                    "|TestIntegrationNoStorageWrites"
                    "|TestIntegrationConfirmationIsBounded"),
    "pagination": "TestIntegrationPaginationUsesCachedPagesAgainstRealOpenList",
    "empty_directory": ("TestIntegrationEmptyDirectoryShapeAgainstRealOpenList"
                        "|TestIntegrationTargetDirectoryIsPresentAgainstRealOpenList"),
    "removed_parent": "TestIntegrationRemovedParentAgainstRealOpenList",
    "read_only_identity": "TestIntegrationReadOnlyIdentityAgainstRealOpenList",
    "cached_driver": "TestIntegrationCachedDriverAgainstRealOpenList",
    "nested_mount_masking": "TestIntegrationNestedMountMaskingAgainstRealOpenList",
    "hidden_entries": "TestIntegrationPermissionFilteredAgainstRealOpenList",
}


def validate_output_dir(output_dir):
    resolved = output_dir.resolve()
    if resolved.exists() and any(resolved.iterdir()):
        raise SystemExit(f"refusing to reuse a non-empty output directory: {resolved}")
    try:
        inside = resolved.is_relative_to(EVIDENCE_ROOT)
    except AttributeError:  # pragma: no cover - Python < 3.9
        inside = str(resolved).startswith(str(EVIDENCE_ROOT))
    if not inside:
        raise SystemExit(f"refusing to write outside {EVIDENCE_ROOT}: {resolved}")
    resolved.mkdir(parents=True, exist_ok=True)
    return resolved


def preflight():
    if not docker_available():
        raise Blocked("docker is not available")
    revision, version, digests, image_id = image_revision()
    return {
        "source_sha": revision,
        "version": version,
        "vendor_digests": digests,
        "image_id": image_id,
        "banner": version_banner(),
        "host_os": platform.platform(),
        "python": sys.version.split()[0],
        "docker": run(["docker", "version", "--format", "{{.Server.Version}}"]).stdout.strip(),
    }


def run_go(go_test_binary, env_descriptor, workdir, names_regex, log_path):
    env = os.environ.copy()
    env["STRMGEN_OPENLIST_INTEGRATION"] = json.dumps(env_descriptor)
    argv = [str(go_test_binary), "-test.run", f"^({names_regex})$", "-test.v", "-test.count=1"]
    started = time.time()
    result = subprocess.run(argv, cwd=workdir, env=env, text=True, capture_output=True, timeout=DEFAULT_TIMEOUT)
    elapsed = time.time() - started
    log_path.write_text(
        "$ " + " ".join(argv) + "\n\n--- stdout ---\n" + result.stdout + "\n--- stderr ---\n" + result.stderr)
    if result.returncode != 0:
        raise Failure(f"Go integration test failed ({result.returncode}); see {log_path.name}")
    if "SKIP" in result.stdout or "--- SKIP" in result.stdout:
        raise Failure("Go integration test skipped; skips are not accepted as evidence")
    ran = re.findall(r"^=== RUN\s+(\S+)$", result.stdout, re.MULTILINE)
    passed = re.findall(r"^--- PASS: (\S+)", result.stdout, re.MULTILINE)
    if not ran or len(passed) != len(ran):
        raise Failure(f"expected every run test to pass (ran={len(ran)} passed={len(passed)})")
    return {"ran": ran, "passed": passed, "elapsed_s": round(elapsed, 2)}


def wait_listed_exact(service, path, *, present=(), absent=(), timeout=120):
    """Poll a cached listing until the expected names appear/disappear.

    Every poll uses the shared list_once helper, which sends an explicit
    refresh:false. A forced refresh or cache flush is never used.
    """
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        status, payload = service.list_once(path)
        if status == 200 and isinstance(payload, dict) and payload.get("code") == 200:
            data = payload.get("data") or {}
            names = sorted(entry["name"] for entry in (data.get("content") or []))
            last = names
            if all(name in names for name in present) and all(name not in names for name in absent):
                return names
        time.sleep(0.5)
    raise Failure(f"{path} never reflected the backing-store change (last={last})")


def run_cli_mirror(sync_binary, workdir):
    """Run the release CLI's Mirror journey inside one dedicated service.

    This layer is separate from the Task 2.1 confirmation observations: it
    proves the real list/get service plus the published binary can generate,
    retain three complete missing rounds, preview without side effects and
    only then delete. It returns facts for the summary and never reuses a
    production mount or token.
    """
    workdir.mkdir(parents=True, exist_ok=True)
    log_lines = []
    facts = {}
    wide_total = 250
    with Harness(workdir) as harness:
        service = harness.service
        freeze_tree = service.media_dir / "CliFreeze"
        write_file(freeze_tree, "One.mkv", b"z" * 32)
        freeze_id = service.add_local_storage("/clifreeze", "/media/CliFreeze")
        service.wait_listed("/clifreeze", ["One.mkv"])

        root = workdir / "out"
        db = workdir / "state" / "strmgen.db"
        env = os.environ.copy()
        env["STRMGEN_OPENLIST_TOKEN"] = service.api.token

        def write_config(name, remote, output, database, cleanup=True):
            path = workdir / f"{name}.yaml"
            path.write_text(
                "openlist:\n"
                f'  api_url: "{service.api.base}"\n'
                '  public_url: "https://openlist.example.invalid"\n'
                '  token_env: "STRMGEN_OPENLIST_TOKEN"\n'
                "sources:\n"
                f'  - remote: "{remote}"\n'
                f'    output: "{output}"\n'
                "sync:\n  directory_workers: 4\n"
                "cleanup:\n"
                f"  enabled: {'true' if cleanup else 'false'}\n"
                "database:\n"
                f'  path: "{database}"\n')
            return path

        def run_cli(config, *extra):
            argv = [str(sync_binary), "sync", "--config", str(config), *extra]
            result = subprocess.run(argv, cwd=workdir, env=env, text=True,
                                    capture_output=True, timeout=DEFAULT_TIMEOUT)
            log_lines.append("$ " + " ".join(argv))
            log_lines.append(result.stdout)
            log_lines.append(result.stderr)
            return result

        def ownership():
            connection = sqlite3.connect(db.resolve().as_uri() + "?mode=ro", uri=True)
            try:
                return {row[0]: row[1] for row in
                        connection.execute("SELECT relative_path, missing_count FROM ownership")}
            finally:
                connection.close()

        config = write_config("cli-mirror", "/lib/Wide", root, db)
        first = run_cli(config)
        if first.returncode != 0 or f"created={wide_total}" not in first.stdout:
            raise Failure(f"real CLI generation failed: {first.returncode} {first.stdout} {first.stderr}")
        facts["generated"] = wide_total

        removed = "E0000.mkv"
        (harness.tree / "Wide" / removed).unlink()
        wait_listed_exact(service, "/lib/Wide", absent=[removed])
        for round_number in range(1, 4):
            run = run_cli(config)
            if run.returncode != 0 or "deleted=0" not in run.stdout:
                raise Failure(f"real CLI missing round {round_number}: {run.stdout} {run.stderr}")
            if not (root / "E0000.strm").exists():
                raise Failure(f"real CLI round {round_number} deleted the target early")
            if ownership()["E0000.strm"] != round_number:
                raise Failure(f"real CLI round {round_number} count: {ownership()}")
        before_file = (root / "E0000.strm").read_bytes()
        before_db = db.read_bytes()
        prior_round = sqlite3.connect(db.resolve().as_uri() + "?mode=ro", uri=True).execute(
            "SELECT epoch FROM rounds").fetchone()[0]
        preview = run_cli(config, "--dry-run")
        if preview.returncode != 0 or "candidates=1" not in preview.stdout:
            raise Failure(f"real CLI dry-run preview: {preview.stdout} {preview.stderr}")
        if (root / "E0000.strm").read_bytes() != before_file or db.read_bytes() != before_db:
            raise Failure("real CLI dry-run changed the candidate or database")
        fourth = run_cli(config)
        if fourth.returncode != 0 or "deleted=1" not in fourth.stdout:
            raise Failure(f"real CLI qualified delete: {fourth.stdout} {fourth.stderr}")
        if (root / "E0000.strm").exists() or "E0000.strm" in ownership():
            raise Failure("real CLI did not remove the qualified target")
        facts["qualified_delete"] = fourth.returncode
        facts["preview_round"] = prior_round

        # Reappearance after a committed missing round clears the old evidence.
        returned = "E0001.mkv"
        (harness.tree / "Wide" / returned).unlink()
        wait_listed_exact(service, "/lib/Wide", absent=[returned])
        run_cli(config)
        if ownership()["E0001.strm"] != 1:
            raise Failure("real CLI reappearance fixture did not count")
        write_file(harness.tree, f"Wide/{returned}", b"w" * 64)
        wait_listed_exact(service, "/lib/Wide", present=[returned])
        back = run_cli(config)
        if back.returncode != 0 or ownership()["E0001.strm"] != 0:
            raise Failure(f"real CLI reappearance did not clear: {back.stdout} {back.stderr}")
        facts["reappearance"] = back.returncode

        # A disabled storage makes the configured source root unreadable; the
        # CLI must preserve the STRM and counts.
        freeze_out = workdir / "freeze-out"
        freeze_db = workdir / "state" / "freeze.db"
        freeze_config = write_config("cli-freeze", "/clifreeze", freeze_out, freeze_db)
        seed = run_cli(freeze_config)
        if seed.returncode != 0 or "created=1" not in seed.stdout:
            raise Failure(f"real CLI freeze seed: {seed.stdout} {seed.stderr}")
        service.disable_storage(freeze_id)
        frozen = run_cli(freeze_config)
        if frozen.returncode != 1 or "partial=true" not in frozen.stdout:
            raise Failure(f"real CLI unreadable root must fail closed: {frozen.stdout} {frozen.stderr}")
        if not (freeze_out / "One.strm").exists():
            raise Failure("real CLI unreadable root removed the target")
        service.enable_storage(freeze_id)
        facts["root_unreadable_exit"] = frozen.returncode
        (workdir / "cli-mirror.log").write_text("\n".join(log_lines))
    return facts


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--test-binary", required=True,
                        help="Go test binary built with -tags openlist_integration")
    parser.add_argument("--sync-binary",
                        help="optional release strmgen binary for the real Mirror CLI scenario")
    parser.add_argument("--output-dir", required=True,
                        help="new empty directory under artifacts/validation-evidence")
    parser.add_argument("--scenario", default="all",
                        help="'all' or a comma-separated subset: " + ", ".join(sorted(SCENARIOS)))
    args = parser.parse_args(argv)

    binary = Path(args.test_binary).resolve()
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise SystemExit(f"test binary is not executable: {binary}")
    sync_binary = None
    if args.sync_binary:
        sync_binary = Path(args.sync_binary).resolve()
        if not sync_binary.is_file() or not os.access(sync_binary, os.X_OK):
            raise SystemExit(f"sync binary is not executable: {sync_binary}")
    output_dir = validate_output_dir(Path(args.output_dir))
    selected = list(SCENARIOS) if args.scenario == "all" else \
        [name.strip() for name in args.scenario.split(",") if name.strip()]
    unknown = [name for name in selected if name not in SCENARIOS]
    if unknown:
        raise SystemExit("unknown scenario(s): " + ", ".join(unknown))

    summary = {
        "test_binary": str(binary),
        "test_binary_sha256": sha256_of(binary),
        "sync_binary": str(sync_binary) if sync_binary is not None else None,
        "sync_binary_sha256": sha256_of(sync_binary) if sync_binary is not None else None,
        "scenarios": {},
        "observations": {},
        "cli_mirror": {"status": "NOT RUN", "detail": "--sync-binary not provided"},
        "failures": [],
        "blocked": None,
    }
    try:
        summary["environment"] = preflight()
    except Blocked as error:
        summary["blocked"] = str(error)
        (output_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        log(f"BLOCKED: {error}")
        return 3

    descriptor_scenarios = {}
    status = 0
    for name in selected:
        workdir = output_dir / "run" / name
        workdir.mkdir(parents=True, exist_ok=True)
        started = time.time()
        try:
            with Harness(workdir) as harness:
                produced = SCENARIOS[name](harness)
                facts, observations = (produced if isinstance(produced, tuple) else (produced, {}))
                descriptor_scenarios.update(facts)
                summary["observations"].update(observations)
                descriptor = {
                    "api_url": harness.service.api.base,
                    "token": harness.service.api.token,
                    "root": "/lib",
                    "scenarios": descriptor_scenarios,
                }
                result = run_go(binary, descriptor, workdir, GO_TESTS[name], workdir / "go-test.log")
            summary["scenarios"][name] = {"status": "PASS", **result,
                                         "elapsed_s": round(time.time() - started, 2)}
            log(f"PASS {name} ({summary['scenarios'][name]['elapsed_s']}s) "
                f"tests={len(result['passed'])}")
        except Blocked as error:
            summary["scenarios"][name] = {"status": "BLOCKED", "detail": str(error)}
            summary["blocked"] = str(error)
            log(f"BLOCKED {name}: {error}")
            status = 3
            break
        except (Failure, subprocess.SubprocessError, OSError) as error:
            summary["scenarios"][name] = {"status": "FAIL", "detail": str(error)}
            summary["failures"].append([name, str(error)])
            log(f"FAIL {name}: {error}")
            status = 1
        finally:
            cleanup_all()

    # The release-CLI Mirror layer is independent of the Go confirmation
    # observations above. Without --sync-binary it stays NOT RUN; it is never
    # reported as a pass through the Task 2.1 evidence.
    if summary["blocked"] is None and sync_binary is not None:
        try:
            cli_facts = run_cli_mirror(sync_binary, output_dir / "run" / "cli-mirror")
            summary["cli_mirror"] = {"status": "PASS", **cli_facts}
            log(f"PASS cli_mirror generated={cli_facts.get('generated')}")
        except Blocked as error:
            summary["cli_mirror"] = {"status": "BLOCKED", "detail": str(error)}
            log(f"BLOCKED cli_mirror: {error}")
            status = 3
        except (Failure, subprocess.SubprocessError, OSError) as error:
            summary["cli_mirror"] = {"status": "FAIL", "detail": str(error)}
            summary["failures"].append(["cli_mirror", str(error)])
            log(f"FAIL cli_mirror: {error}")
            if status == 0:
                status = 1
        finally:
            cleanup_all()

    (output_dir / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    log(json.dumps({"scenarios": {k: v["status"] for k, v in summary["scenarios"].items()},
                    "cli_mirror": summary["cli_mirror"]["status"],
                    "failures": summary["failures"], "blocked": summary["blocked"]},
                   ensure_ascii=False))
    return status


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    finally:
        cleanup_all()
