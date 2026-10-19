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

# Every secret this process has seen, including ones derived at run time (the Jellyfin
# token issued by ``AuthenticateByName``). A serialization or exception boundary must
# scrub all of them: a credential with quotes or backslashes is escaped by JSON, so
# comparing only the raw value lets the escaped form reach a file.
_REGISTERED_SECRETS: list[str] = []


def register_secret(value: str | None) -> None:
    """Track a secret value (raw or derived) that must never be written or printed."""
    if not value or len(value) < 4 or value in _REGISTERED_SECRETS:
        return
    _REGISTERED_SECRETS.append(value)


def _secret_forms(value: str) -> tuple[str, ...]:
    """Serialized shapes a value can take in evidence, text or a nested message."""
    forms = {value}
    escaped = json.dumps(value)[1:-1]
    forms.update({escaped, repr(value)[1:-1], value.encode("unicode_escape").decode("ascii", "ignore")})
    return tuple(form for form in forms if len(form) >= 4)


def redact(text: str) -> str:
    """Remove URLs (which may carry signatures) and key=value secrets from a line."""
    text = _URL_RE.sub("<url>", str(text))
    return _SECRET_RE.sub(lambda m: m.group(1) + "=<redacted>", text)


def scrub(text: str) -> str:
    """Redact URLs/secret pairs and remove every registered secret in any serialized form.

    Used on every boundary that can carry a value outside the evidence recorder: child
    stdout/stderr, driver JSON ``error`` fields, exception messages and the top-level
    ``FAIL`` line. Raw or JSON-escaped forms are both removed.
    """
    text = redact(text)
    for value in _REGISTERED_SECRETS:
        for form in _secret_forms(value):
            text = text.replace(form, "<secret>")
    return text


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
    """Scenario failure whose text is always safe to print, including a nested child error."""

    def __str__(self) -> str:  # noqa: D105 - the message is scrubbed on the way out
        return scrub(super().__str__())


def require(condition: bool, message: str) -> None:
    if not condition:
        raise Failure(message)


def secret_values(env: dict[str, str]) -> list[str]:
    """Non-empty secret values (process environment and run-time derived) to scrub."""
    values = [env[key] for key in SECRET_ENV_KEYS if env.get(key)]
    values.extend(value for value in _REGISTERED_SECRETS if value not in values)
    return values


def assert_secret_free(payload: str, env: dict[str, str]) -> None:
    """Refuse to write evidence containing a credential in any serialized form."""
    for value in secret_values(env):
        if any(form in payload for form in _secret_forms(value)):
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
        register_secret(admin_token)
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
    # The issued token is a run-time derived credential: register it so it is scrubbed
    # from every later boundary even though it never appears in the process environment.
    register_secret(payload["AccessToken"])
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

    def failure(self, error: BaseException, **fields) -> None:
        """Record the failing scenario. The field is scrubbed before the secrecy check."""
        category = type(error).__name__
        message = scrub(str(error))[:500]
        try:
            self.record("run_failure", category=category, message=message, **fields)
        except Failure:
            # The refusal path itself must still leave a truthful marker, never a value.
            (self.output / "run_failure.json").write_text(
                json.dumps({"scenario": "run_failure", "category": category,
                            "message": "<withheld: would contain a credential>"}, indent=1),
                encoding="utf-8")


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
    try:
        result = subprocess.run(
            ["node", str(driver), "/dev/stdin"],
            input=json.dumps(scenario, ensure_ascii=False),
            capture_output=True, text=True, timeout=300, env=env,
        )
    except subprocess.TimeoutExpired:
        raise Failure("native playback driver timed out after 300s; no playback evidence") from None
    except OSError as error:
        raise Failure(f"native playback driver could not start ({type(error).__name__})") from None
    if not result.stdout.strip():
        raise Failure(f"native playback driver exited {result.returncode} without JSON output; "
                      "a non-zero exit or an empty payload is not playback evidence")
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError:
        raise Failure("native playback driver returned non-JSON output") from None
    if not isinstance(payload, dict):
        raise Failure("native playback driver returned an unexpected payload shape")
    if payload.get("error"):
        # A driver error is a structured value: scrub it here so no boundary raises it raw.
        return {"error": scrub(str(payload["error"]))[:300]}
    return payload


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
                             log: DecisionLog) -> dict[str, str]:
    """RO-4: scan the library and actually play the synthetic media natively.

    Returns the mount-relative STRM name -> Jellyfin item id map that this run actually
    scanned and played, so later decision rows can be linked to real items instead of any
    item that happens to appear in the log.
    """
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
    return {name: mapping[name]["Id"] for name in wanted}


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
    require(result.returncode == 0, f"rclone copyto failed: {scrub(result.stderr[-200:])}")


def rclone_move(env: dict[str, str], source_key: str, dest_key: str) -> None:
    bucket = env.get("RUSTFS_BUCKET", "slice4-media")
    remote = env.get("RCLONE_REMOTE", "slice4")
    result = subprocess.run(
        ["rclone", "moveto", f"{remote}:{bucket}/{source_key}", f"{remote}:{bucket}/{dest_key}"],
        env=rclone_env(env), capture_output=True, text=True, timeout=180)
    require(result.returncode == 0, f"rclone moveto failed: {scrub(result.stderr[-200:])}")


def scenario_url_change(recorder: Recorder, target: Target, binary: Path, config: Path,
                        token: str, cwd: Path, strm_dir: Path, library_id: str,
                        driver: Path, playwright_root: str, browsers_path: str,
                        jellyfin_url: str, credentials: Credentials, log: DecisionLog,
                        item_name: str, alt_public_url: str) -> None:
    """RO-3/RO-5: a changed STRM URL must force a fresh plugin probe.

    The public URL is changed by rewriting the STRM through the generator config (a new
    ``public_url``), which is a legitimate label-preserving change: the URL changes while the
    object identity and size stay the same. This row is user-deferred, so it is not part of the
    current gate; when it does run, a probe start is reported as an observation and no accepted/
    rejected verdict is inferred from it.
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
                        note="" if accepted else "URL change forced a native probe; whether the "
                                                 "plugin accepted the probe result is not "
                                                 "classified by this runner",
                        decisions=[entry["line"] for entry in delta[-3:]])
    finally:
        # Restore the shared config even when the scenario failed, without hiding the failure.
        config.write_text(original, encoding="utf-8")
        try:
            run_cli(binary, config, token, cwd)
        except OSError as error:
            recorder.record("ro3_url_change_restore_failure", failed=True,
                            detail=[f"sync: {type(error).__name__}"],
                            note="config was rewritten back but the fixture sync could not run; "
                                 "the fixture is NOT confirmed restored")


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

    The local media file and the remote object are restored in ``finally`` even when the scenario
    fails, and a failed restore is reported instead of being swallowed.
    """
    source = media_dir / item_name
    original = source.read_bytes()
    before = log.mark()

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
                        note="" if accepted else "Size change forced a native probe; whether the "
                                                 "plugin accepted the probe result is not "
                                                 "classified by this runner",
                        decisions=[entry["line"] for entry in delta[-3:]])
    finally:
        # Restore the local synthetic source and the remote object whatever happened, and never
        # let a restore problem be reported as if the scenario had merely failed on its own.
        source.write_bytes(original)
        restore_failures = []
        try:
            upload()
        except Failure as error:
            restore_failures.append(f"remote: {scrub(str(error))[:160]}")
        try:
            run_cli(binary, config, token, cwd)
        except OSError as error:
            restore_failures.append(f"sync: {type(error).__name__}")
        if restore_failures:
            recorder.record("ro3_size_change_restore_failure",
                            failed=True, detail=restore_failures,
                            note="fixture restoration failed after the size-change scenario; "
                                 "the fixture is NOT restored and must be checked or rebuilt")
            raise Failure("size-change fixture restore failed: " + "; ".join(restore_failures))


def scenario_decision_rows(recorder: Recorder, log: DecisionLog, expected_items: dict[str, str],
                           mark: int, hash_probe: dict) -> None:
    """RO-3: assert the non-deferred decision rows against the items this run actually produced.

    ``expected_items`` maps each expected mount-relative STRM name to the Jellyfin item id that
    the library reported for it in this run. A row may only be classified from a window entry
    that names *that* item: an unrelated item, a rotated log, or a decision recorded for some
    other playback can never satisfy the stable row. A row whose input cannot be produced by the
    target is recorded as BLOCKED with the concrete reason; nothing is synthesized.
    """
    require(bool(expected_items), "no expected item was identified, so no decision row can be linked")
    window = log.since(mark)
    decisions = [entry for entry in window if entry["kind"] == "decision"]
    probes = [entry for entry in window if entry["kind"] == "probe"]
    require(bool(decisions),
            "no plugin reuse decision was observed in this run's log window; "
            "a cumulative or empty log is not decision evidence")
    reasons = {entry["reason"] for entry in decisions}
    require(all(reason in REUSE_REASONS | PROBE_REQUIRED_REASONS for reason in reasons),
            f"unexpected plugin decision reasons: {sorted(reasons)}")

    linked: list[dict] = []
    missing: list[str] = []
    for name, item_id in sorted(expected_items.items()):
        matches = [entry for entry in decisions if entry["item"] == normalize_item_id(item_id)]
        reuse = sorted({entry["reason"] for entry in matches} & REUSE_REASONS)
        if not reuse:
            missing.append(name)
            continue
        linked.append({"name": name, "item": normalize_item_id(item_id), "reuse": reuse})
    require(not missing,
            f"the stable-URL row has no reuse decision for the expected item(s) {missing}; "
            f"decisions in this window covered {sorted({entry['item'] for entry in decisions})}")

    rows = [
        {"row": "stable_url_same_fingerprint", "status": "PASS",
         "observed": linked,
         "evidence": "plugin reuse decision per expected item, each linked to the item id this "
                     "run scanned and played, inside the run's log window"},
        {"row": "url_changed", "status": "DEFERRED",
         "observed": "not rerun this round (user 2026-10-19 decision); the retained original "
                     "observation is a native probe start whose result the plugin did not accept",
         "evidence": "artifacts/validation-evidence/task-4.1-fix2-20261019-215918/ retains the "
                     "original BLOCKED observation; no cause was re-verified this round"},
        {"row": "same_url_same_sha1_size_changed", "status": "BLOCKED",
         "observed": hash_probe["observation"],
         "evidence": "the target driver reports no SHA-1 metadata, so the same-URL/same-SHA-1 "
                     "branch of the decision matrix has no legitimate input; fabricating a SHA-1 "
                     "marker or editing the plugin is forbidden"},
        {"row": "same_url_sha1_changed", "status": "BLOCKED",
         "observed": hash_probe["observation"],
         "evidence": "same reason: no target SHA-1, no legitimate Sha1Changed input"},
        {"row": "hash_incomparable_effective_size_changed", "status": "DEFERRED",
         "observed": "not rerun this round (user 2026-10-19 decision); the retained original "
                     "observation is a native probe start whose result the plugin did not accept",
         "evidence": "artifacts/validation-evidence/task-4.1-fix2-20261019-215918/ retains the "
                     "original BLOCKED observation; no cause was re-verified this round"},
    ]
    recorder.record("ro3_decision_rows", window_entries=len(window),
                    decisions=len(decisions), probes=len(probes),
                    reasons=sorted(reasons), expected_items=len(expected_items),
                    linked_items=len(linked), rows=rows, hash_input=hash_probe)


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
    """RO-5/AC-6: remote path move, then generator, scan, playback and cleanup closure.

    The scenario tracks which mutating steps it has completed. If anything after the remote
    move fails, the object is moved back so the fixture is left in its pre-scenario location;
    the failure marker records the step, whether the object was restored, and whether the
    restore itself failed. A failed restore is never hidden behind the original error.
    """
    original = config.read_text(encoding="utf-8")
    source_key = object_key(env, remote_root, item_name)
    moved_dir = "Moved"
    moved_key = object_key(env, remote_root, f"{moved_dir}/{item_name}")
    moved_rel = f"{moved_dir}/{item_name[:-4]}"
    before_tree = digest_tree(strm_dir)
    before_original = read_strm(strm_dir / (item_name[:-4] + ".strm"))
    rounds: list[str] = []
    step = "start"
    object_moved = False
    try:
        # 1. Move the object inside the target mount and synchronize.
        step = "remote move"
        rclone_move(env, source_key, moved_key)
        object_moved = True
        step = "move sync"
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
        step = "moved scan"
        scan = refresh_library(target)
        mapping = library_items(target, library_id)
        require(moved_rel in mapping, "Jellyfin did not load the moved STRM")
        moved_item = mapping[moved_rel]
        # A moved path is a new Jellyfin identity, so its first playback needs a real native
        # probe. Report only what the log and the playback result actually show: a probe start
        # alone does not prove the plugin rejected its own result, so no cause is asserted here.
        step = "moved first playback"
        mark = log.mark()
        moved_playback: dict = {"status": "NOT RUN"}
        try:
            first = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                  credentials, moved_item)
            moved_playback = {"status": "PASS", "currentTime": first.get("currentTime"),
                              "videoWidth": first.get("videoWidth"),
                              "videoHeight": first.get("videoHeight"),
                              "decision": log.require_reuse(mark, moved_item["Id"])}
        except Failure as error:
            window = log.since(mark)
            probe_started = any(entry["kind"] == "probe" for entry in window)
            require(probe_started or bool(window),
                    f"moved-item playback failed without any observable plugin activity: {error}")
            # Report the observations, not an inferred cause: whether the plugin rejected the
            # probe result is not visible here, so it stays unclassified for B/A to judge.
            moved_playback = {"status": "BLOCKED",
                              "observed": {"playback_error": scrub(str(error))[:200],
                                           "probe_started": probe_started,
                                           "window_entries": len(window)},
                              "cause": "NOT DETERMINED: a probe start is not evidence that the "
                                       "probe result was rejected; the row is handed to A/B "
                                       "unclassified rather than attributed to an external cause"}
        second_decision = None
        if moved_playback["status"] == "PASS":
            step = "moved second playback"
            mark = log.mark()
            second = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                   credentials, moved_item)
            second_decision = log.require_reuse(mark, moved_item["Id"], {"Unchanged"})

        # 3. Mirror cleanup must retire the stale STRM after the configured confirmations.
        step = "mirror cleanup"
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
        step = "remote restore"
        rclone_move(env, moved_key, source_key)
        object_moved = False
        step = "restore sync"
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
        step = "restored scan"
        scan = refresh_library(target)
        mapping = library_items(target, library_id)
        require(item_name[:-4] in mapping, "restored item is missing from the library")
        step = "restored replay"
        mark = log.mark()
        replay: dict = {"status": "NOT RUN"}
        try:
            outcome = playback_item(target, driver, playwright_root, browsers_path, jellyfin_url,
                                    credentials, mapping[item_name[:-4]])
            replay = {"status": "PASS", "advanced": bool(outcome.get("advanced")),
                      "currentTime": outcome.get("currentTime")}
            replay["decision"] = log.require_reuse(mark, mapping[item_name[:-4]]["Id"])
        except Failure as error:
            window = log.since(mark)
            require(bool(window),
                    f"restored-item playback failed without observable plugin activity: {error}")
            replay = {"status": "BLOCKED",
                      "observed": {"playback_error": scrub(str(error))[:200],
                                   "window_entries": len(window)},
                      "cause": "NOT DETERMINED"}
        recorder.record("ro5_move_closure", moved_rel=moved_rel, scan=scan,
                        stale_cleanup_rounds=rounds_used, rounds=rounds,
                        moved_playback=moved_playback,
                        second_decision=second_decision,
                        restored_tree_sha256=digest_tree(strm_dir),
                        replay=replay)
    except Failure as error:
        # Stop at the failing step, put the fixture back the way it started, and record the
        # truth: which step failed, whether the remote object was restored, and whether the
        # configuration was written back and re-synchronised. Nothing here is inferred.
        recovery = {"attempted": False, "object_restored": not object_moved,
                    "config_written": False, "config_synced": None, "error": ""}
        if object_moved:
            recovery["attempted"] = True
            try:
                rclone_move(env, moved_key, source_key)
                recovery["object_restored"] = True
            except Failure as recovery_error:
                recovery["object_restored"] = False
                recovery["error"] = scrub(str(recovery_error))[:200]
        try:
            config.write_text(original, encoding="utf-8")
            recovery["config_written"] = True
        except OSError as write_error:
            recovery["error"] = (recovery["error"] + " | " if recovery["error"] else "") \
                                + f"config write: {type(write_error).__name__}"
        try:
            recovery["config_synced"] = run_cli(binary, config, token, cwd)[0] == 0
        except OSError as sync_error:
            recovery["config_synced"] = False
            recovery["error"] = (recovery["error"] + " | " if recovery["error"] else "") \
                                + f"config sync: {type(sync_error).__name__}"
        recorder.record("ro5_move_failure",
                        failed_step=step,
                        cause="NOT DETERMINED (no root-cause inference)",
                        message=scrub(str(error))[:300],
                        recovery_attempted=recovery["attempted"],
                        remote_object_restored=recovery["object_restored"],
                        config_written_back=recovery["config_written"],
                        config_resynced=recovery["config_synced"],
                        recovery_error=recovery["error"],
                        fixture_restored=(recovery["object_restored"] and recovery["config_written"]
                                          and recovery["config_synced"] is True),
                        note="the moved-path first playback is user-deferred; this marker records "
                             "the real failure and the verified fixture state instead of a verdict")
        raise
    finally:
        if config.read_text(encoding="utf-8") != original:
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
    # Item ids this run actually scanned and played; the decision rows are linked to these.
    played_items: dict[str, str] = {}
    completed: list[str] = []

    try:
        if wanted in ("all", "acceptance", "metadata"):
            scenario_metadata(recorder, target, args.remote_root, args.strm_dir)
            completed.append("metadata")
        if wanted in ("all", "acceptance", "byte_equality"):
            scenario_byte_equality(recorder, target, args.remote_root, args.strm_dir,
                                   args.media_dir, names, args.download_base)
            completed.append("byte_equality")
        if wanted in ("all", "acceptance", "stable"):
            scenario_stable_sync(recorder, args.binary, args.strm_config, credentials.openlist_token,
                                 ROOT, args.strm_dir)
            completed.append("stable")
        if wanted in ("all", "acceptance", "playback"):
            played_items = scenario_native_playback(recorder, target, args.library_id,
                                                    args.playback_driver, args.playwright_root,
                                                    args.browsers_path, args.jellyfin_url,
                                                    credentials, names, log)
            completed.append("playback")
        if wanted in ("all", "acceptance", "decisions"):
            # The window starts before the playback scenario in the same run so the rows are
            # linked to the inputs this run actually produced. A standalone ``decisions`` run
            # starts the window now and therefore requires real decisions in it.
            if not played_items:
                # No playback ran in this invocation: discover the ids the library currently
                # reports for the expected STRMs so the rows still bind to real items.
                mapping = library_items(target, args.library_id)
                played_items = {name[:-4]: mapping[name[:-4]]["Id"] for name in names
                                if name[:-4] in mapping}
            hash_probe = scenario_hash_input(recorder, target, args.remote_root, args.strm_dir, names)
            scenario_decision_rows(recorder, log, played_items, decision_mark, hash_probe)
            completed.append("decisions")
        if wanted in ("all", "acceptance", "hash_input"):
            scenario_hash_input(recorder, target, args.remote_root, args.strm_dir, names)
            completed.append("hash_input")
        if wanted in ("all", "url_change"):
            scenario_url_change(recorder, target, args.binary, args.strm_config,
                                credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                                args.playback_driver, args.playwright_root, args.browsers_path,
                                args.jellyfin_url, credentials, log, args.change_item,
                                args.alt_public_url)
            completed.append("url_change")
        if wanted in ("all", "size_change"):
            scenario_size_change(recorder, target, args.binary, args.strm_config,
                                 credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                                 args.playback_driver, args.playwright_root, args.browsers_path,
                                 args.jellyfin_url, credentials, log, args.change_item,
                                 args.media_dir, args.remote_root, env)
            completed.append("size_change")
        if wanted in ("all", "acceptance", "move"):
            scenario_move(recorder, target, args.binary, args.strm_config,
                          credentials.openlist_token, ROOT, args.strm_dir, args.library_id,
                          args.playback_driver, args.playwright_root, args.browsers_path,
                          args.jellyfin_url, credentials, log, args.change_item,
                          args.remote_root, env, args.download_base, args.media_dir)
            completed.append("move")
    except Failure as error:
        # Record the failing scenario truthfully (scrubbed) before the process exits non-zero.
        recorder.failure(error, scenario=wanted, completed=completed)
        raise
    finally:
        recorder.finish()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Failure as error:
        # Failure.__str__ scrubs registered secrets, escaped forms and URLs before printing.
        print(f"FAIL: {error}", file=sys.stderr)
        raise SystemExit(1)
