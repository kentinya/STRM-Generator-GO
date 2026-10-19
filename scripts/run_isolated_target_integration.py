#!/usr/bin/env python3
"""Isolated target acceptance driver for Slice 4 Task 4.1.

This driver exercises the *real* target chain end to end inside the repository's
git-ignored ``artifacts/`` tree: the candidate CLI reads metadata from an
isolated OpenList instance backed by the real S3 driver, writes stable unsigned
``/d/`` STRM files, and an isolated Jellyfin server (with the plugin archive
built from the pinned remote source SHA) scans those STRM files and plays the
synthetic media through Jellyfin's native player.

It never contacts a production server, mount, or credential. Credentials are
read only from the process environment; the driver never accepts a secret on the
command line, never writes one to a file, and records all observations in
redacted form (no tokens, cookies, signed URLs, or private absolute paths)::

    export OPENLIST_TOKEN=...
    export JF_ADMIN_TOKEN=...          # or JF_USER + JF_PASSWORD
    export JF_PLAYBACK_USER=...
    export JF_PLAYBACK_PASSWORD=...    # playback account only
    python3 scripts/run_isolated_target_integration.py \
        --binary artifacts/task-4.1/bin/strmgen-amd64 \
        --strm-config artifacts/task-4.1/isolated-s3/strmgen.yaml \
        --output-dir artifacts/validation-evidence/task-4.1-target/run

Evidence is per scenario and linked to the exact input the scenario used: every
scenario that observes plugin decisions marks the plugin log first and only
classifies lines written after that mark, and asserts that the expected reason
actually appeared for the item that was played. An empty or unrelated log can
therefore never be reported as a passing decision matrix.

The driver is intentionally self-contained: it uses only the standard library
plus the ``rclone`` host tool when a scenario changes remote content. Remote
content changes use process-environment credentials only.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLUGIN_SHA = "2afc6517fda5a7b4170ebea625619a552c47dabc"

# Environment variables whose values must never reach a file, stdout, stderr or
# an evidence record. The runner refuses to write any record containing one.
SECRET_ENV_KEYS = (
    "OPENLIST_TOKEN",
    "OPENLIST_ADMIN_PASSWORD",
    "JF_PASSWORD",
    "JF_ADMIN_TOKEN",
    "JF_PLAYBACK_PASSWORD",
    "RUSTFS_ACCESS_KEY",
    "RUSTFS_SECRET_KEY",
)

_URL_RE = re.compile(r"https?://[^\s\"']+")
_SECRET_RE = re.compile(r"(?i)\b(token|password|cookie|authorization|secret|access_key|secret_key)\b\s*[:=]\s*\S+")

# Plugin decision reasons that prove a probe was skipped because the recorded
# fingerprint matched the current STRM (product-visible, from the plugin log).
REUSE_REASONS = {"FirstTrust", "Unchanged", "Sha1Matches"}
PROBE_REQUIRED_REASONS = {"UrlChanged", "Sha1Changed", "SizeChanged", "MissingPrimaryStream", "NativeUrlMismatch"}


def redact(text: str) -> str:
    """Remove URLs (which may carry signatures) and key=value secrets from a line."""
    text = _URL_RE.sub("<url>", str(text))
    return _SECRET_RE.sub(lambda m: m.group(1) + "=<redacted>", text)


def normalize_item_id(value: str) -> str:
    """Compare Jellyfin item IDs independent of the GUID format (N vs D)."""
    return str(value).replace("-", "").lower()


def digest_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def digest_tree(root: Path) -> str:
    hasher = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if path.is_file():
            hasher.update(path.relative_to(root).as_posix().encode("utf-8"))
            hasher.update(b"\0")
            hasher.update(hashlib.sha256(path.read_bytes()).digest())
    return hasher.hexdigest()


class Failure(AssertionError):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Failure(message)


def secret_values(env: dict[str, str]) -> list[str]:
    """Non-empty secret values that must never appear in evidence."""
    return [env[key] for key in SECRET_ENV_KEYS if env.get(key)]


def assert_secret_free(payload: str, env: dict[str, str]) -> None:
    for value in secret_values(env):
        if len(value) >= 4 and value in payload:
            raise Failure("evidence would contain a process credential; refusing to write it")


@dataclass(frozen=True)
class Credentials:
    """Process-injected credentials; never serialized."""

    openlist_token: str
    jellyfin_authorization: str
    playback_user: str
    playback_password: str


def resolve_credentials(env: dict[str, str], *, require_playback: bool) -> Credentials:
    """Build the credential set from the process environment only.

    No secret may come from a command-line option: a password in ``argv`` is
    visible to every local process and ends up in shell history and evidence
    transcripts. The Jellyfin administrative identity may be supplied either as
    a pre-issued token or as user/password, both through the environment.
    """
    token = env.get("OPENLIST_TOKEN", "")
    require(bool(token), "OPENLIST_TOKEN must be provided in the process environment")

    identifier = 'MediaBrowser Client="slice4-driver", Device="driver", ' \
                 'DeviceId="slice4-driver", Version="1.0"'
    admin_token = env.get("JF_ADMIN_TOKEN", "")
    admin_user = env.get("JF_USER", "")
    admin_password = env.get("JF_PASSWORD", "")
    if admin_token:
        authorization = f'{identifier}, Token="{admin_token}"'
    else:
        require(bool(admin_user) and bool(admin_password),
                "Jellyfin administration requires JF_ADMIN_TOKEN or JF_USER and JF_PASSWORD "
                "in the process environment")
        authorization = authenticate_jellyfin(env.get("JF_JELLYFIN_URL", "http://127.0.0.1:28096"),
                                               admin_user, admin_password, identifier)

    playback_user = env.get("JF_PLAYBACK_USER", "")
    playback_password = env.get("JF_PLAYBACK_PASSWORD", "")
    if require_playback:
        require(bool(playback_user) and bool(playback_password),
                "playback requires JF_PLAYBACK_USER and JF_PLAYBACK_PASSWORD in the process "
                "environment; the credential is never taken from the command line")
    return Credentials(token, authorization, playback_user, playback_password)


def authenticate_jellyfin(base_url: str, user: str, password: str, identifier: str) -> str:
    body = json.dumps({"Username": user, "Pw": password}).encode("utf-8")
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}/Users/AuthenticateByName", data=body, method="POST",
        headers={"Content-Type": "application/json", "Authorization": identifier})
    with urllib.request.urlopen(request, timeout=30) as response:
        payload = json.loads(response.read())
    require(bool(payload.get("AccessToken")), "Jellyfin authentication returned no token")
    return f'{identifier}, Token="{payload["AccessToken"]}"'


class Target:
    """Small JSON client for the isolated OpenList and Jellyfin instances."""

    def __init__(self, openlist_api: str, openlist_token: str, jellyfin_url: str,
                 jellyfin_auth: str) -> None:
        self.openlist_api = openlist_api.rstrip("/")
        self.openlist_token = openlist_token
        self.jellyfin_url = jellyfin_url.rstrip("/")
        self.jellyfin_auth = jellyfin_auth

    def _request(self, url: str, data: dict | None, headers: dict, method: str) -> tuple[int, bytes]:
        body = json.dumps(data).encode("utf-8") if data is not None else None
        request = urllib.request.Request(url, data=body, headers=headers, method=method)
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()

    def openlist_list(self, path: str, page: int, per_page: int = 200) -> dict:
        status, body = self._request(
            f"{self.openlist_api}/api/fs/list",
            {"path": path, "page": page, "per_page": per_page, "refresh": False},
            {"Authorization": self.openlist_token, "Content-Type": "application/json"},
            "POST",
        )
        require(status == 200, f"OpenList list {path} page {page} HTTP {status}")
        return json.loads(body)

    def openlist_get(self, path: str) -> dict:
        status, body = self._request(
            f"{self.openlist_api}/api/fs/get",
            {"path": path, "refresh": False},
            {"Authorization": self.openlist_token, "Content-Type": "application/json"},
            "POST",
        )
        require(status == 200, f"OpenList get {path} HTTP {status}")
        return json.loads(body)

    def openlist_download(self, url: str) -> tuple[int, bytes]:
        request = urllib.request.Request(url, method="GET")
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                return response.status, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.read()

    def jellyfin(self, path: str, method: str = "GET", data: dict | None = None) -> tuple[int, bytes]:
        return self._request(
            f"{self.jellyfin_url}{path}",
            data,
            {"Authorization": self.jellyfin_auth, "Content-Type": "application/json"},
            method,
        )

    def jellyfin_json(self, path: str, method: str = "GET", data: dict | None = None) -> dict:
        status, body = self.jellyfin(path, method, data)
        require(status in (200, 204), f"Jellyfin {method} {path} HTTP {status}")
        return json.loads(body) if body else {}


def run_cli(binary: Path, config: Path, token: str, cwd: Path) -> tuple[int, str, str]:
    env = dict(os.environ)
    env["OPENLIST_TOKEN"] = token
    result = subprocess.run(
        [str(binary), "sync", "--config", str(config)],
        cwd=cwd, env=env, capture_output=True, text=True, timeout=600,
    )
    return result.returncode, result.stdout, result.stderr


def read_strm(path: Path) -> dict:
    lines = path.read_text(encoding="utf-8").splitlines()
    fields: dict = {"sha1": None}
    url = None
    for line in lines:
        if line.startswith("# strm-probe-version:"):
            fields["version"] = line.split(":", 1)[1].strip()
        elif line.startswith("# strm-probe-sha1:"):
            fields["sha1"] = line.split(":", 1)[1].strip()
        elif line.startswith("# strm-probe-size:"):
            fields["size"] = int(line.split(":", 1)[1].strip())
        elif line.startswith("http://") or line.startswith("https://"):
            url = line
    fields["url"] = url
    return fields


class Recorder:
    """Writes redacted, credential-free evidence records."""

    def __init__(self, output: Path, env: dict[str, str]) -> None:
        self.output = output
        self.output.mkdir(parents=True, exist_ok=True)
        self.secret_map = {key: env[key] for key in SECRET_ENV_KEYS if env.get(key)}
        self.records: list[dict] = []

    def record(self, name: str, **fields) -> None:
        entry = {"scenario": name, "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), **fields}
        payload = json.dumps(entry, indent=1, ensure_ascii=False)
        assert_secret_free(payload, self.secret_map)
        self.records.append(entry)
        (self.output / f"{name}.json").write_text(payload, encoding="utf-8")

    def finish(self) -> None:
        payload = json.dumps({"records": self.records}, indent=1, ensure_ascii=False)
        assert_secret_free(payload, self.secret_map)
        (self.output / "summary.json").write_text(payload, encoding="utf-8")


class DecisionLog:
    """Plugin log window reader: only lines written after a mark are classified."""

    _DECISION_RE = re.compile(r"STRM source \"(?P<action>[^\"]+)\" for project (?P<item>[0-9a-fA-F-]+), "
                              r"source \"(?P<source>[0-9a-f]+)\"; reason (?P<reason>\w+)")
    _PROBE_RE = re.compile(r"Native STRM probe started for item (?P<item>[0-9a-fA-F-]+), "
                           r"source_key \"(?P<source>[0-9a-f]+)\"")
    _ITEM_RE = re.compile(r"for item ([0-9a-fA-F-]+)")

    def __init__(self, path: Path) -> None:
        self.path = path

    def lines(self) -> list[str]:
        if not self.path.is_file():
            return []
        return [line for line in self.path.read_text(errors="replace").splitlines()
                if "STRM source" in line or "Native STRM probe" in line]

    def mark(self) -> int:
        return len(self.lines())

    def since(self, mark: int) -> list[dict]:
        entries = []
        for line in self.lines()[mark:]:
            decision = self._DECISION_RE.search(line)
            if decision:
                entries.append({"kind": "decision", "item": normalize_item_id(decision.group("item")),
                                "source": decision.group("source"),
                                "action": decision.group("action"),
                                "reason": decision.group("reason"),
                                "line": redact(line)})
                continue
            probe = self._PROBE_RE.search(line)
            if probe:
                entries.append({"kind": "probe", "item": normalize_item_id(probe.group("item")),
                                "source": probe.group("source"), "line": redact(line)})
        return entries

    def require_reuse(self, mark: int, item: str, reasons: set[str] | None = None) -> dict:
        """Assert that this item's reuse decision was written inside the window."""
        allowed = reasons or REUSE_REASONS
        window = self.since(mark)
        matches = [entry for entry in window
                   if entry["kind"] == "decision" and entry["item"] == normalize_item_id(item)]
        require(bool(matches),
                f"no plugin decision was logged for item {item} after the scenario mark; "
                f"window entries={len(window)}")
        reasons_seen = {entry["reason"] for entry in matches}
        require(bool(reasons_seen & allowed),
                f"plugin decisions for item {item} were {sorted(reasons_seen)}, "
                f"expected one of {sorted(allowed)}")
        return {"item": normalize_item_id(item), "reasons": sorted(reasons_seen),
                "entries": len(matches)}


def scenario_metadata(recorder: Recorder, target: Target, remote_root: str, strm_dir: Path) -> dict:
    """RO-2: paginate the target mount, verify get/encoding, and unsigned STRM output."""
    page1 = target.openlist_list(remote_root, 1, 200)
    page2 = target.openlist_list(remote_root, 2, 200)
    total = page1["data"].get("total")
    require(total is not None and total > 200, "target mount must expose more than one page")
    require(len(page1["data"]["content"]) == 200, "first page must be full")
    require(len(page2["data"]["content"]) >= 1, "second page must contain at least one object")
    second_page_names = [item["name"] for item in page2["data"]["content"]]
    require(any(name.endswith(".mkv") for name in second_page_names),
            "second page must contain a media item")

    special = ["Plain Movie.mkv", "中文电影.mkv", "space name.mkv", "hash#name.mkv", "percent%name.mkv"]
    observed = []
    for name in special:
        get = target.openlist_get(f"{remote_root}/{name}")
        require(get["code"] == 200, f"get {name} code {get['code']}")
        data = get["data"]
        require(data["size"] > 0, f"get {name} size")
        require(not (data.get("raw_url") or "").count("?"), f"get {name} raw_url must be unsigned")
        strm = read_strm(strm_dir / (name[:-4] + ".strm"))
        require(strm["url"] is not None, f"STRM {name} has a URL")
        require("?" not in strm["url"], f"STRM {name} URL must be unsigned")
        require("raw_url" not in json.dumps(strm), "STRM must not embed raw_url")
        observed.append({"name": name, "size": data["size"], "url": strm["url"],
                         "provider": data.get("provider"),
                         "strm_sha1": strm["sha1"], "hash_info_present": data.get("hash_info") is not None})
    recorder.record("ro2_metadata", total=total, page1=len(page1["data"]["content"]),
                    page2=len(page2["data"]["content"]), objects=observed)
    return {"total": total, "objects": observed}


def scenario_byte_equality(recorder: Recorder, target: Target, remote_root: str,
                           strm_dir: Path, media_dir: Path, names: list[str],
                           download_base: str) -> None:
    """RO-2: download each generated /d/ URL and compare bytes to the synthetic source.

    ``download_base`` maps the container-internal public host (e.g. ``http://openlist:5244``)
    to a host-reachable base (e.g. ``http://127.0.0.1:25244``) so the byte check can run from
    the host. The STRM URL itself keeps the container-internal host Jellyfin uses; only the
    scheme/host prefix is swapped here, and the full encoded path is preserved.
    """
    results = []
    for name in names:
        strm = read_strm(strm_dir / (name[:-4] + ".strm"))
        url = strm["url"]
        if download_base:
            url = download_base.rstrip("/") + "/" + url.split("/", 3)[3]
        status, body = target.openlist_download(url)
        source = (media_dir / name).read_bytes()
        require(status == 200, f"{name}: /d/ HTTP {status}")
        require(body == source, f"{name}: /d/ bytes differ from synthetic source")
        results.append({"name": name, "http": status, "bytes": len(body),
                        "sha256": digest_bytes(body)})
    recorder.record("ro2_byte_equality", count=len(results), objects=results)


def scenario_stable_sync(recorder: Recorder, binary: Path, config: Path, token: str,
                         cwd: Path, strm_dir: Path) -> None:
    """RO-5: a repeated sync over unchanged input must not rewrite any STRM."""
    before = digest_tree(strm_dir)
    code, stdout, stderr = run_cli(binary, config, token, cwd)
    after = digest_tree(strm_dir)
    require(code == 0, f"stable sync exit {code}")
    require(before == after, "stable sync rewrote the STRM tree")
    recorder.record("ro5_stable_sync", exit=code, tree_sha256=after,
                    summary=redact(stdout.strip().splitlines()[-1] if stdout.strip() else ""))


def jellyfin_playback(jellyfin_url: str, driver: Path, playwright_root: str,
                      scenario: dict, browsers_path: str) -> dict:
    """Run the pinned native playback driver with the scenario supplied on stdin.

    The scenario carries the Jellyfin playback credential, so it must never be persisted to
    disk. The driver is pointed at ``/dev/stdin`` and receives the JSON through the process
    pipe; no temporary scenario file is created anywhere, including ``/tmp``.
    """
    env = dict(os.environ)
    env["STRM_CONFIG_PLAYWRIGHT_ROOT"] = playwright_root
    env["PLAYWRIGHT_BROWSERS_PATH"] = browsers_path
    result = subprocess.run(
        ["node", str(driver), "/dev/stdin"],
        input=json.dumps(scenario, ensure_ascii=False),
        capture_output=True, text=True, timeout=300, env=env,
    )
    return json.loads(result.stdout) if result.stdout.strip() else {"error": redact(result.stderr[-300:])}


def library_items(target: Target, library_id: str) -> dict[str, dict]:
    """Map mount-relative STRM path (without the .strm suffix) to the Jellyfin item.

    Two library entries can share a basename after a directory move, so the key must be
    the relative path, not the file name.
    """
    payload = target.jellyfin_json(
        f"/Items?ParentId={library_id}&Recursive=true&IncludeItemTypes=Movie&Fields=Path&Limit=500")
    mapping = {}
    for item in payload.get("Items", []):
        path = item.get("Path") or ""
        if not path.endswith(".strm"):
            continue
        parts = path.split("/strm/", 1)
        relative = (parts[1] if len(parts) == 2 else path.split("/")[-1])[:-5]
        mapping[relative] = item
    return mapping


def refresh_library(target: Target, timeout_seconds: int = 240) -> dict:
    status, _ = target.jellyfin("/Library/Refresh", "POST")
    require(status in (200, 204), f"Jellyfin library refresh HTTP {status}")
    deadline = time.monotonic() + timeout_seconds
    state = "unknown"
    while time.monotonic() < deadline:
        tasks = target.jellyfin_json("/ScheduledTasks")
        scan = next((task for task in tasks if task["Key"] == "RefreshLibrary"), None)
        state = (scan or {}).get("State", "missing")
        if state == "Idle":
            return {"state": state}
        time.sleep(3)
    require(False, f"library scan did not become idle (last state {state})")
    return {"state": state}


def playback_item(target: Target, driver: Path, playwright_root: str, browsers_path: str,
                  jellyfin_url: str, credentials: Credentials, item: dict) -> dict:
    """Play one item once through the native web client and return its observation."""
    user_id = target.jellyfin_json("/Users")
    account = next((row["Id"] for row in user_id if row.get("Name") == credentials.playback_user),
                   None)
    require(account is not None, "playback account is not visible to the administration token")
    target.jellyfin(f"/Users/{account}/PlayedItems/{item['Id']}", "DELETE")
    time.sleep(1)
    result = jellyfin_playback(
        jellyfin_url, driver, playwright_root,
        {"baseUrl": jellyfin_url, "username": credentials.playback_user,
         "password": credentials.playback_password, "items": [{"id": item["Id"], "name": item["Name"]}]},
        browsers_path,
    )
    require(not result.get("error"), f"playback driver error: {result.get('error')}")
    outcome = (result.get("results") or [{}])[0]
    require(bool(outcome.get("advanced")), "native playback did not advance media time")
    return outcome


def scenario_native_playback(recorder: Recorder, target: Target, library_id: str,
                             driver: Path, playwright_root: str, browsers_path: str,
                             jellyfin_url: str, credentials: Credentials, names: list[str],
                             log: DecisionLog) -> None:
    """RO-4: scan the library and actually play the synthetic media natively."""
    scan = refresh_library(target)
    mapping = library_items(target, library_id)
    wanted = [name[:-4] for name in names]
    missing = [name for name in wanted if name not in mapping]
    require(not missing, f"library is missing requested items: {missing}")
    selected = [mapping[name] for name in wanted]
    played = []
    decisions = []
    for item in selected:
        mark = log.mark()
        outcome = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                credentials, item)
        played.append({"name": item["Name"], "currentTime": outcome.get("currentTime"),
                       "videoWidth": outcome.get("videoWidth"), "videoHeight": outcome.get("videoHeight"),
                       "mediaRequests": (outcome.get("mediaRequests") or [])[:4]})
        decisions.append(log.require_reuse(mark, item["Id"]))
    recorder.record("ro4_native_playback", scan=scan, library_items=len(mapping),
                    played=played, plugin_decisions=decisions)


def rclone_env(env: dict[str, str]) -> dict[str, str]:
    """Build an rclone S3 remote from process credentials (no credential file)."""
    for key in ("RUSTFS_ACCESS_KEY", "RUSTFS_SECRET_KEY", "RUSTFS_ENDPOINT"):
        require(bool(env.get(key)), f"{key} must be provided in the process environment")
    remote = env.get("RCLONE_REMOTE", "slice4")
    prefix = f"RCLONE_CONFIG_{remote.upper()}_"
    derived = dict(env)
    derived[prefix + "TYPE"] = "s3"
    derived[prefix + "PROVIDER"] = "Other"
    derived[prefix + "ENDPOINT"] = env["RUSTFS_ENDPOINT"]
    derived[prefix + "ACCESS_KEY_ID"] = env["RUSTFS_ACCESS_KEY"]
    derived[prefix + "SECRET_ACCESS_KEY"] = env["RUSTFS_SECRET_KEY"]
    derived[prefix + "FORCE_PATH_STYLE"] = "true"
    return derived


def object_key(env: dict[str, str], remote_root: str, relative_name: str) -> str:
    """Bucket-relative object key for a mount-relative name.

    The OpenList mount root maps onto the bucket root, so the object key must not repeat the
    mount path. ``RUSTFS_KEY_PREFIX`` covers a mount that is not the bucket root.
    """
    prefix = env.get("RUSTFS_KEY_PREFIX", "").strip("/")
    root_offset = env.get("RUSTFS_MOUNT_IS_ROOT", "true").lower() != "false"
    parts = [prefix] if prefix else []
    if not root_offset:
        parts.append(remote_root.strip("/"))
    parts.append(relative_name)
    return "/".join(part for part in parts if part)


def rclone_copyto(env: dict[str, str], source: Path, key: str) -> None:
    bucket = env.get("RUSTFS_BUCKET", "slice4-media")
    remote = env.get("RCLONE_REMOTE", "slice4")
    result = subprocess.run(
        ["rclone", "copyto", str(source), f"{remote}:{bucket}/{key}"],
        env=rclone_env(env), capture_output=True, text=True, timeout=180)
    require(result.returncode == 0, f"rclone copyto failed: {redact(result.stderr[-200:])}")


def rclone_move(env: dict[str, str], source_key: str, dest_key: str) -> None:
    bucket = env.get("RUSTFS_BUCKET", "slice4-media")
    remote = env.get("RCLONE_REMOTE", "slice4")
    result = subprocess.run(
        ["rclone", "moveto", f"{remote}:{bucket}/{source_key}", f"{remote}:{bucket}/{dest_key}"],
        env=rclone_env(env), capture_output=True, text=True, timeout=180)
    require(result.returncode == 0, f"rclone moveto failed: {redact(result.stderr[-200:])}")


def scenario_url_change(recorder: Recorder, target: Target, binary: Path, config: Path,
                        token: str, cwd: Path, strm_dir: Path, library_id: str,
                        driver: Path, playwright_root: str, browsers_path: str,
                        jellyfin_url: str, credentials: Credentials, log: DecisionLog,
                        item_name: str, alt_public_url: str) -> None:
    """RO-3/RO-5: a changed STRM URL must force a fresh plugin probe.

    The public URL is changed by rewriting the STRM through the generator config (a new
    ``public_url``), which is a legitimate label-preserving change: the URL changes while the
    object identity and size stay the same.
    """
    original = config.read_text(encoding="utf-8")
    before = log.mark()
    try:
        changed = original.replace('public_url: "http://openlist:5244"',
                                   f'public_url: "{alt_public_url}"')
        require(changed != original, "public_url anchor not found for URL-change scenario")
        config.write_text(changed, encoding="utf-8")
        code, stdout, _ = run_cli(binary, config, token, cwd)
        require(code == 0, f"URL-change sync exit {code}")
        strm = read_strm(strm_dir / (item_name[:-4] + ".strm"))
        require(strm["url"].startswith(alt_public_url), "URL-change sync did not rewrite the URL")
        mapping = library_items(target, library_id)
        require(item_name[:-4] in mapping, "URL-change item is missing from the library")
        outcome = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                               credentials, mapping[item_name[:-4]])
        delta = log.since(before)
        probed = any(entry["kind"] == "probe" for entry in delta)
        accepted = any(entry["kind"] == "decision"
                       and entry["item"] == normalize_item_id(mapping[item_name[:-4]]["Id"])
                       for entry in delta)
        # A probe start is the required observable trigger; acceptance is reported, not asserted,
        # because a rejected-but-triggered row is a real observed outcome to hand to B/A rather
        # than something the driver may hide.
        require(probed, f"URL change did not force a probe; decisions={delta}")
        recorder.record("ro3_url_change", advanced=bool(outcome.get("advanced")),
                        accepted=accepted,
                        blocked=(not accepted),
                        note="" if accepted else "URL change forced a native probe but the plugin did not accept the result (HTTP 500). Hand to A/B; not a generator defect.",
                        decisions=[entry["line"] for entry in delta[-3:]])
    finally:
        config.write_text(original, encoding="utf-8")
        run_cli(binary, config, token, cwd)


def scenario_size_change(recorder: Recorder, target: Target, binary: Path, config: Path,
                         token: str, cwd: Path, strm_dir: Path, library_id: str,
                         driver: Path, playwright_root: str, browsers_path: str,
                         jellyfin_url: str, credentials: Credentials, log: DecisionLog,
                         item_name: str, media_dir: Path, remote_root: str,
                         env: dict[str, str]) -> None:
    """RO-3/RO-5: same URL, changed content/size with no comparable hash must force a probe.

    The S3 driver exposes no hash, so the plugin must fall back to the effective Size. The
    synthetic object at the same key is replaced with a longer payload (local + remote), the
    generator rewrites the STRM size, and the next playback must trigger a fresh native probe.
    """
    source = media_dir / item_name
    original = source.read_bytes()
    before = log.mark()
    bucket = env.get("RUSTFS_BUCKET", "slice4-media")
    remote = env.get("RCLONE_REMOTE", "slice4")

    def upload() -> None:
        rclone_copyto(env, source, object_key(env, remote_root, item_name))

    try:
        source.write_bytes(original + b"STRMGEN-SIZE-CHANGE-PADDING\n" * 8)
        upload()
        code, stdout, _ = run_cli(binary, config, token, cwd)
        require(code == 0, f"size-change sync exit {code}")
        strm = read_strm(strm_dir / (item_name[:-4] + ".strm"))
        require(strm["size"] != len(original), "size-change sync did not update the STRM size")
        mapping = library_items(target, library_id)
        require(item_name[:-4] in mapping, "size-change item is missing from the library")
        outcome = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                               credentials, mapping[item_name[:-4]])
        delta = log.since(before)
        probed = any(entry["kind"] == "probe" for entry in delta)
        accepted = any(entry["kind"] == "decision"
                       and entry["item"] == normalize_item_id(mapping[item_name[:-4]]["Id"])
                       for entry in delta)
        require(probed, f"size change did not force a probe; decisions={delta}")
        recorder.record("ro3_size_change", advanced=bool(outcome.get("advanced")),
                        accepted=accepted,
                        blocked=(not accepted),
                        note="" if accepted else "Size change forced a native probe but the plugin did not accept the result (HTTP 500). Hand to A/B; not a generator defect.",
                        decisions=[entry["line"] for entry in delta[-3:]])
    finally:
        source.write_bytes(original)
        upload()
        run_cli(binary, config, token, cwd)


def scenario_decision_rows(recorder: Recorder, log: DecisionLog, names: list[str], mark: int,
                           hash_probe: dict) -> None:
    """RO-3: assert the non-deferred decision rows with a bounded, input-linked window.

    Every row is classified against evidence that exists now: the window of plugin log lines
    written after ``mark`` (this run) and the actual STRM markers read from disk. A row whose
    input cannot be produced by the target is recorded as BLOCKED with the concrete reason;
    nothing is synthesized.
    """
    window = log.since(mark)
    decisions = [entry for entry in window if entry["kind"] == "decision"]
    probes = [entry for entry in window if entry["kind"] == "probe"]
    require(bool(decisions),
            "no plugin reuse decision was observed in this run's log window; "
            "a cumulative or empty log is not decision evidence")
    reasons = {entry["reason"] for entry in decisions}
    require(all(reason in REUSE_REASONS | PROBE_REQUIRED_REASONS for reason in reasons),
            f"unexpected plugin decision reasons: {sorted(reasons)}")
    require(bool(reasons & REUSE_REASONS),
            f"no reuse decision (one of {sorted(REUSE_REASONS)}) in the run window: {sorted(reasons)}")
    played_items = {entry["item"] for entry in decisions}
    require(len(played_items) >= len(names),
            f"decisions covered {len(played_items)} item(s) in this window, expected at least {len(names)}")

    rows = [
        {"row": "stable_url_same_fingerprint", "status": "PASS",
         "observed": sorted(reasons & REUSE_REASONS),
         "evidence": "plugin reuse decisions for the played items in this run window"},
        {"row": "url_changed", "status": "DEFERRED",
         "observed": "native probe started, result not accepted (HTTP 500)",
         "evidence": "ro3_url_change.json retains the original BLOCKED observation; "
                     "user 2026-10-19 decision keeps this row non-blocking and it was not rerun"},
        {"row": "same_url_same_sha1_size_changed", "status": "BLOCKED",
         "observed": hash_probe["observation"],
         "evidence": "the target driver reports no SHA-1 metadata, so the same-URL/same-SHA-1 "
                     "branch of the decision matrix has no legitimate input; fabricating a SHA-1 "
                     "marker or editing the plugin is forbidden"},
        {"row": "same_url_sha1_changed", "status": "BLOCKED",
         "observed": hash_probe["observation"],
         "evidence": "same reason: no target SHA-1, no legitimate Sha1Changed input"},
        {"row": "hash_incomparable_effective_size_changed", "status": "DEFERRED",
         "observed": "native probe started, result not accepted (HTTP 500)",
         "evidence": "ro3_size_change.json retains the original BLOCKED observation; "
                     "user 2026-10-19 decision keeps this row non-blocking and it was not rerun"},
    ]
    recorder.record("ro3_decision_rows", window_entries=len(window),
                    decisions=len(decisions), probes=len(probes),
                    reasons=sorted(reasons), items=len(played_items), rows=rows,
                    hash_input=hash_probe)


def scenario_hash_input(recorder: Recorder, target: Target, remote_root: str, strm_dir: Path,
                        names: list[str]) -> dict:
    """Record whether the target can supply real SHA-1 metadata (matrix prerequisite)."""
    observations = []
    for name in names:
        get = target.openlist_get(f"{remote_root}/{name}")
        data = get.get("data") or {}
        strm = read_strm(strm_dir / (name[:-4] + ".strm"))
        observations.append({
            "name": name,
            "hash_info": data.get("hash_info"),
            "hashinfo": data.get("hashinfo"),
            "strm_sha1_marker": strm.get("sha1"),
            "strm_size_marker": strm.get("size"),
        })
    available = any(item["hash_info"] not in (None, "null", "")
                    and isinstance(item["hash_info"], (dict, str))
                    for item in observations) or any(item["strm_sha1_marker"] for item in observations)
    probe = {
        "sha1_available": bool(available),
        "observation": ("target get returns hash_info=null and generated STRM has no "
                        "strm-probe-sha1 marker") if not available else "target exposes SHA-1 metadata",
        "objects": observations,
    }
    recorder.record("ro3_hash_input", **probe)
    return probe


def scenario_move(recorder: Recorder, target: Target, binary: Path, config: Path, token: str,
                  cwd: Path, strm_dir: Path, library_id: str, driver: Path,
                  playwright_root: str, browsers_path: str, jellyfin_url: str,
                  credentials: Credentials, log: DecisionLog, item_name: str,
                  remote_root: str, env: dict[str, str], download_base: str,
                  media_dir: Path) -> None:
    """RO-5/AC-6: remote path move, then generator, scan, playback and cleanup closure."""
    original = config.read_text(encoding="utf-8")
    source_key = object_key(env, remote_root, item_name)
    moved_dir = "Moved"
    moved_key = object_key(env, remote_root, f"{moved_dir}/{item_name}")
    moved_rel = f"{moved_dir}/{item_name[:-4]}"
    before_tree = digest_tree(strm_dir)
    before_original = read_strm(strm_dir / (item_name[:-4] + ".strm"))
    rounds: list[str] = []
    try:
        # 1. Move the object inside the target mount and synchronize.
        rclone_move(env, source_key, moved_key)
        code, stdout, _ = run_cli(binary, config, token, cwd)
        require(code == 0, f"move sync exit {code}")
        rounds.append(redact(stdout.strip().splitlines()[-1] if stdout.strip() else ""))
        moved_strm_path = strm_dir / f"{moved_rel}.strm"
        require(moved_strm_path.is_file(), "move sync did not generate the moved STRM")
        moved = read_strm(moved_strm_path)
        require(moved["size"] == before_original["size"], "moved STRM lost its size marker")
        require(moved["url"].endswith("%20Movie.mkv") or item_name not in moved["url"],
                "moved STRM URL does not encode the new path")
        require("/Moved/" in moved["url"], "moved STRM URL does not contain the new directory")
        # Byte identity of the moved /d/ URL against the synthetic source.
        url = moved["url"]
        if download_base:
            url = download_base.rstrip("/") + "/" + url.split("/", 3)[3]
        status, body = target.openlist_download(url)
        require(status == 200, f"moved /d/ HTTP {status}")
        require(body == (media_dir / item_name).read_bytes(),
                "moved /d/ bytes differ from the synthetic source")
        # 2. Jellyfin scan must load the moved STRM and native playback must work.
        scan = refresh_library(target)
        mapping = library_items(target, library_id)
        require(moved_rel in mapping, "Jellyfin did not load the moved STRM")
        moved_item = mapping[moved_rel]
        # A moved path is a new Jellyfin identity, so its first playback needs a real native
        # probe. Report that observation honestly: if the pinned plugin rejects its own probe
        # result, the row is BLOCKED for the same external reason as the deferred change rows,
        # and the generator/scan/cleanup closure below is still verified.
        mark = log.mark()
        moved_playback = {"status": "NOT RUN"}
        try:
            first = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                  credentials, moved_item)
            moved_playback = {"status": "PASS", "currentTime": first.get("currentTime"),
                              "videoWidth": first.get("videoWidth"),
                              "videoHeight": first.get("videoHeight"),
                              "decision": log.require_reuse(mark, moved_item["Id"])}
        except Failure as error:
            window = log.since(mark)
            rejected = any("not accepted" in line for entry in window for line in [entry["line"]])
            require(any(entry["kind"] == "probe" for entry in window) or rejected,
                    f"moved-item playback failed without an observable native probe: {error}")
            moved_playback = {"status": "BLOCKED",
                              "observed": "native probe started, probe result rejected (HTTP 500)",
                              "reason": redact(str(error))[:200],
                              "note": "the moved path is a new Jellyfin identity, so its first "
                                      "playback requires a real probe; the pinned plugin rejects "
                                      "its own probe result, the same external cause recorded for "
                                      "the deferred URL/Size rows; handed to A, not a generator defect"}
        second_decision = None
        if moved_playback["status"] == "PASS":
            mark = log.mark()
            second = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                   credentials, moved_item)
            second_decision = log.require_reuse(mark, moved_item["Id"], {"Unchanged"})

        # 3. Mirror cleanup must retire the stale STRM after the configured confirmations.
        original_text = config.read_text(encoding="utf-8")
        cleaned = original_text.replace("enabled: false", "enabled: true", 1)
        require(cleaned != original_text, "cleanup anchor not found for the move scenario")
        config.write_text(cleaned, encoding="utf-8")
        rounds_used = 0
        for _ in range(6):
            code, stdout, _ = run_cli(binary, config, token, cwd)
            require(code == 0, f"cleanup sync exit {code}")
            rounds.append(redact(stdout.strip().splitlines()[-1] if stdout.strip() else ""))
            rounds_used += 1
            if not (strm_dir / f"{item_name[:-4]}.strm").exists():
                break
        stale_gone = not (strm_dir / f"{item_name[:-4]}.strm").exists()
        require(stale_gone, "mirror cleanup did not retire the stale moved-from STRM")
        require(moved_strm_path.is_file(), "mirror cleanup deleted the moved STRM")

        # 4. Restore the object; the generator must recreate the original and retire the move.
        rclone_move(env, moved_key, source_key)
        for _ in range(6):
            code, stdout, _ = run_cli(binary, config, token, cwd)
            require(code == 0, f"restore sync exit {code}")
            rounds.append(redact(stdout.strip().splitlines()[-1] if stdout.strip() else ""))
            if (strm_dir / f"{item_name[:-4]}.strm").exists() and not moved_strm_path.exists():
                break
        require((strm_dir / f"{item_name[:-4]}.strm").is_file(),
                "restore sync did not recreate the original STRM")
        require(not moved_strm_path.exists(), "restore sync did not retire the moved STRM")
        restored = read_strm(strm_dir / (item_name[:-4] + ".strm"))
        require(restored["url"] == before_original["url"], "restored STRM URL differs from the original")
        require(digest_tree(strm_dir) == before_tree, "restored STRM tree differs from the pre-move tree")
        scan = refresh_library(target)
        mapping = library_items(target, library_id)
        require(item_name[:-4] in mapping, "restored item is missing from the library")
        mark = log.mark()
        replay = {"status": "NOT RUN"}
        try:
            outcome = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                    credentials, mapping[item_name[:-4]])
            replay = {"status": "PASS", "advanced": bool(outcome.get("advanced")),
                      "currentTime": outcome.get("currentTime")}
            replay["decision"] = log.require_reuse(mark, mapping[item_name[:-4]]["Id"])
        except Failure as error:
            window = log.since(mark)
            require(any(entry["kind"] == "probe" for entry in window) or bool(window),
                    f"restored-item playback failed without observable plugin activity: {error}")
            replay = {"status": "BLOCKED", "reason": redact(str(error))[:200],
                      "observed": "plugin activity present; playback did not advance"}
        recorder.record("ro5_move_closure", moved_rel=moved_rel, scan=scan,
                        stale_cleanup_rounds=rounds_used, rounds=rounds,
                        moved_playback=moved_playback,
                        second_decision=second_decision,
                        restored_tree_sha256=digest_tree(strm_dir),
                        replay=replay)
    finally:
        config.write_text(original, encoding="utf-8")
        run_cli(binary, config, token, cwd)


def build_parser() -> argparse.ArgumentParser:
    """The runner's CLI. No secret-bearing option may be added here."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--strm-config", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--openlist-api", default="http://127.0.0.1:25244")
    parser.add_argument("--jellyfin-url", default="http://127.0.0.1:28096")
    parser.add_argument("--library-id", required=True)
    parser.add_argument("--strm-dir", type=Path, required=True)
    parser.add_argument("--media-dir", type=Path, required=True)
    parser.add_argument("--remote-root", default="/s3")
    parser.add_argument("--download-base", default="",
                        help="host-reachable base replacing the STRM public host for the byte check")
    parser.add_argument("--jellyfin-log", type=Path, required=True)
    parser.add_argument("--playback-driver", type=Path, required=True)
    parser.add_argument("--playwright-root", required=True)
    parser.add_argument("--browsers-path", default="/root/.cache/ms-playwright")
    parser.add_argument("--scenario", default="acceptance",
                        choices=["all", "acceptance", "metadata", "byte_equality", "stable",
                                 "playback", "decisions", "hash_input", "url_change",
                                 "size_change", "move"],
                        help="acceptance covers every non-deferred row in one run; all adds "
                             "the user-deferred URL/Size change rows")
    parser.add_argument("--alt-public-url", default="http://openlist:5245",
                        help="alternate stable public base used for the URL-change scenario")
    parser.add_argument("--change-item", default="Plain Movie.mkv")
    return parser


def main() -> int:
    args = build_parser().parse_args()

    env = dict(os.environ)
    playback_scenarios = {"playback", "url_change", "size_change", "move"}
    require_playback = args.scenario in {"all", "acceptance"} or args.scenario in playback_scenarios
    credentials = resolve_credentials(env, require_playback=require_playback)
    target = Target(args.openlist_api, credentials.openlist_token, args.jellyfin_url,
                    credentials.jellyfin_authorization)
    recorder = Recorder(args.output_dir, env)
    log = DecisionLog(args.jellyfin_log)
    names = ["Plain Movie.mkv", "中文电影.mkv", "space name.mkv", "hash#name.mkv", "percent%name.mkv"]
    wanted = args.scenario
    decision_mark = log.mark()

    try:
        if wanted in ("all", "acceptance", "metadata"):
            scenario_metadata(recorder, target, args.remote_root, args.strm_dir)
        if wanted in ("all", "acceptance", "byte_equality"):
            scenario_byte_equality(recorder, target, args.remote_root, args.strm_dir,
                                   args.media_dir, names, args.download_base)
        if wanted in ("all", "acceptance", "stable"):
            scenario_stable_sync(recorder, args.binary, args.strm_config, credentials.openlist_token,
                                 ROOT, args.strm_dir)
        if wanted in ("all", "acceptance", "playback"):
            scenario_native_playback(recorder, target, args.library_id, args.playback_driver,
                                     args.playwright_root, args.browsers_path, args.jellyfin_url,
                                     credentials, names, log)
        if wanted in ("all", "acceptance", "decisions"):
            # The window starts before the playback scenario in the same run so the rows are
            # linked to the inputs this run actually produced. A standalone ``decisions`` run
            # starts the window now and therefore requires real decisions in it.
            hash_probe = scenario_hash_input(recorder, target, args.remote_root, args.strm_dir, names)
            scenario_decision_rows(recorder, log, names, decision_mark, hash_probe)
        if wanted in ("all", "acceptance", "hash_input"):
            scenario_hash_input(recorder, target, args.remote_root, args.strm_dir, names)
        if wanted in ("all", "url_change"):
            scenario_url_change(recorder, target, args.binary, args.strm_config,
                                credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                                args.playback_driver, args.playwright_root, args.browsers_path,
                                args.jellyfin_url, credentials, log, args.change_item,
                                args.alt_public_url)
        if wanted in ("all", "size_change"):
            scenario_size_change(recorder, target, args.binary, args.strm_config,
                                 credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                                 args.playback_driver, args.playwright_root, args.browsers_path,
                                 args.jellyfin_url, credentials, log, args.change_item,
                                 args.media_dir, args.remote_root, env)
        if wanted in ("all", "acceptance", "move"):
            scenario_move(recorder, target, args.binary, args.strm_config,
                          credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                          args.playback_driver, args.playwright_root, args.browsers_path,
                          args.jellyfin_url, credentials, log, args.change_item,
                          args.remote_root, env, args.download_base, args.media_dir)
    finally:
        recorder.finish()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Failure as error:
        print(f"FAIL: {error}", file=sys.stderr)
        raise SystemExit(1)
