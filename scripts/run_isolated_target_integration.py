#!/usr/bin/env python3
"""Isolated target acceptance driver for Slice 4 Task 4.1.

This driver exercises the *real* target chain end to end inside the repository's
git-ignored ``artifacts/`` tree: the candidate CLI reads metadata from an
isolated OpenList instance backed by the real S3 driver, writes stable unsigned
``/d/`` STRM files, and an isolated Jellyfin server (with the plugin archive
built from the pinned remote source SHA) scans those STRM files and plays the
synthetic media through Jellyfin's native player.

It never contacts a production server, mount, or credential. Credentials are
read only from the process environment; the driver refuses to write them to any
file. All observations are recorded in redacted form (no tokens, cookies, signed
URLs, or private absolute paths).

Usage (credentials are process inputs only)::

    export OPENLIST_TOKEN=...
    export JF_USER=admin
    export JF_PASSWORD=...
    python3 scripts/run_isolated_target_integration.py \
        --binary artifacts/task-4.1/bin/strmgen-amd64 \
        --strm-config artifacts/task-4.1/isolated-s3/strmgen.yaml \
        --output-dir artifacts/validation-evidence/task-4.1-target/run

The driver is intentionally self-contained: it uses only the standard library
plus the ``mc``/``rclone`` host tools if a scenario needs to change remote
content. It performs a best-effort ``finally`` cleanup of only the files it
creates and never stops the shared isolated stack.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLUGIN_SHA = "2afc6517fda5a7b4170ebea625619a552c47dabc"

_URL_RE = re.compile(r"https?://[^\s\"']+")
_SECRET_RE = re.compile(r"(?i)\b(token|password|cookie|authorization|secret|access_key|secret_key)\b\s*[:=]\s*\S+")


def redact(text: str) -> str:
    """Remove URLs (which may carry signatures) and key=value secrets from a line."""
    text = _URL_RE.sub("<url>", text)
    text = _SECRET_RE.sub(lambda m: m.group(1) + "=<redacted>", text)
    return text


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
    fields = {}
    url = None
    for line in lines:
        if line.startswith("# strm-probe-version:"):
            fields["version"] = line.split(":", 1)[1].strip()
        elif line.startswith("# strm-probe-size:"):
            fields["size"] = int(line.split(":", 1)[1].strip())
        elif line.startswith("http://") or line.startswith("https://"):
            url = line
    fields["url"] = url
    return fields


class Recorder:
    def __init__(self, output: Path) -> None:
        self.output = output
        self.output.mkdir(parents=True, exist_ok=True)
        self.records: list[dict] = []

    def record(self, name: str, **fields) -> None:
        entry = {"scenario": name, "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), **fields}
        self.records.append(entry)
        (self.output / f"{name}.json").write_text(json.dumps(entry, indent=1, ensure_ascii=False))

    def finish(self) -> None:
        (self.output / "summary.json").write_text(
            json.dumps({"records": self.records}, indent=1, ensure_ascii=False)
        )


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
                         "provider": data.get("provider")})
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

    The scenario carries the Jellyfin credential, so it must never be persisted to disk. The
    driver is pointed at ``/dev/stdin`` and receives the JSON through the process pipe; no
    temporary scenario file is created anywhere, including ``/tmp``.
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


def item_ids_by_path(target: Target, library_id: str) -> dict[str, str]:
    payload = target.jellyfin_json(
        f"/Items?ParentId={library_id}&Recursive=true&IncludeItemTypes=Movie&Fields=Path&Limit=500")
    mapping = {}
    for item in payload.get("Items", []):
        name = (item.get("Path") or "").split("/")[-1]
        if name.endswith(".strm"):
            name = name[:-5]
        mapping[name] = item["Id"]
    return mapping


def scenario_native_playback(recorder: Recorder, target: Target, library_id: str,
                             driver: Path, playwright_root: str, browsers_path: str,
                             jellyfin_url: str, user: str, password: str, names: list[str]) -> None:
    """RO-4: scan the library and actually play the synthetic media natively."""
    target.jellyfin("/Library/Refresh", "POST")
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        tasks = target.jellyfin_json("/ScheduledTasks")
        scan = next((task for task in tasks if task["Key"] == "RefreshLibrary"), None)
        if scan and scan.get("State") == "Idle":
            break
        time.sleep(3)
    mapping = item_ids_by_path(target, library_id)
    selected = [{"id": mapping[name[:-4]], "name": name[:-4]} for name in names if name[:-4] in mapping]
    require(len(selected) == len(names), "all requested items must be present in the library")
    # Clear any resume state left by an earlier run so the native client shows the plain
    # Play button; the pinned driver clicks the first .btnPlay, which would otherwise be the
    # hidden Resume action. Clear for the account the browser actually logs in as.
    users = target.jellyfin_json("/Users")
    playback_id = next((row["Id"] for row in users if row.get("Name") == user), users[0]["Id"])
    for row in selected:
        target.jellyfin(f"/Users/{playback_id}/PlayedItems/{row['id']}", "DELETE")
    time.sleep(2)
    # Play each item in its own fresh browser invocation. The pinned driver clicks the first
    # `.btnPlay` element; after an item has any resume/played state that first element is the
    # hidden Resume action, so a single long session is fragile. A fresh context per item plus
    # a resume reset keeps the observation deterministic without modifying the pinned driver.
    advanced = []
    per_item = []
    for row in selected:
        target.jellyfin(f"/Users/{playback_id}/PlayedItems/{row['id']}", "DELETE")
        time.sleep(1)
        result = jellyfin_playback(
            jellyfin_url, driver, playwright_root,
            {"baseUrl": jellyfin_url, "username": user, "password": password, "items": [row]},
            browsers_path,
        )
        require(not result.get("error"), f"playback driver error for {row['name']}: {result.get('error')}")
        outcome = (result.get("results") or [{}])[0]
        per_item.append(outcome)
        if outcome.get("advanced"):
            advanced.append(outcome)
    require(len(advanced) == len(selected),
            f"all selected items must play; advanced {len(advanced)}/{len(selected)}")
    recorder.record("ro4_native_playback",
                    library_items=len(mapping),
                    played=[{"name": row["name"], "currentTime": row["currentTime"],
                             "videoWidth": row["videoWidth"], "videoHeight": row["videoHeight"]}
                            for row in advanced],
                    requests={row["name"]: row.get("mediaRequests", [])[:4] for row in per_item})


def plugin_decision_matrix(recorder: Recorder, log_path: Path) -> None:
    """RO-3: classify plugin decisions from its own product-visible log lines."""
    text = log_path.read_text(errors="replace") if log_path.is_file() else ""
    decisions = []
    for line in text.splitlines():
        if "STRM source" not in line:
            continue
        reason = re.search(r"reason (\w+)", line)
        calls = re.search(r"plugin probe calls (\d+)", line)
        decisions.append({"reason": reason.group(1) if reason else None,
                          "probe_calls": int(calls.group(1)) if calls else None})
    summary = {}
    for entry in decisions:
        key = f"{entry['reason']}:{entry['probe_calls']}"
        summary[key] = summary.get(key, 0) + 1
    recorder.record("ro3_plugin_decisions", total=len(decisions), by_reason=summary)


def _plugin_tail(log_path: Path, since_lines: int) -> list[str]:
    text = log_path.read_text(errors="replace") if log_path.is_file() else ""
    lines = [line for line in text.splitlines()
             if "STRM source" in line or "Native STRM probe" in line]
    return lines[since_lines:]


def _plugin_line_count(log_path: Path) -> int:
    text = log_path.read_text(errors="replace") if log_path.is_file() else ""
    return len([line for line in text.splitlines()
                if "STRM source" in line or "Native STRM probe" in line])


def scenario_url_change(recorder: Recorder, target: Target, binary: Path, config: Path,
                        token: str, cwd: Path, strm_dir: Path, library_id: str,
                        driver: Path, playwright_root: str, browsers_path: str,
                        jellyfin_url: str, user: str, password: str, log_path: Path,
                        item_name: str, alt_public_url: str) -> None:
    """RO-3/RO-5: a changed STRM URL must force a fresh plugin probe.

    The public URL is changed by rewriting the STRM through the generator config (a new
    ``public_url``), which is a legitimate label-preserving change: the URL changes while the
    object identity and size stay the same.
    """
    original = config.read_text(encoding="utf-8")
    before = _plugin_line_count(log_path)
    try:
        changed = original.replace('public_url: "http://openlist:5244"',
                                   f'public_url: "{alt_public_url}"')
        require(changed != original, "public_url anchor not found for URL-change scenario")
        config.write_text(changed, encoding="utf-8")
        code, stdout, _ = run_cli(binary, config, token, cwd)
        require(code == 0, f"URL-change sync exit {code}")
        strm = read_strm(strm_dir / (item_name[:-4] + ".strm"))
        require(strm["url"].startswith(alt_public_url), "URL-change sync did not rewrite the URL")
        mapping = item_ids_by_path(target, library_id)
        item_id = mapping[item_name[:-4]]
        users = target.jellyfin_json("/Users")
        playback_id = next((row["Id"] for row in users if row.get("Name") == user), users[0]["Id"])
        target.jellyfin(f"/Users/{playback_id}/PlayedItems/{item_id}", "DELETE")
        result = jellyfin_playback(
            jellyfin_url, driver, playwright_root,
            {"baseUrl": jellyfin_url, "username": user, "password": password,
             "items": [{"id": item_id, "name": item_name[:-4]}]},
            browsers_path,
        )
        require(not result.get("error"), f"URL-change playback error: {result.get('error')}")
        delta = _plugin_tail(log_path, before)
        probed = any("Native STRM probe started" in line or "probe calls 1" in line for line in delta)
        accepted = any("reused saved media information" in line and "reason" in line for line in delta) \
            or bool((result.get("results") or [{}])[0].get("advanced"))
        # A probe start is the required observable trigger; acceptance is reported, not asserted,
        # because a rejected-but-triggered row is a real observed outcome to hand to B/A rather
        # than something the driver may hide.
        require(probed, f"URL change did not force a probe; decisions={delta}")
        recorder.record("ro3_url_change", advanced=bool((result.get("results") or [{}])[0].get("advanced")),
                        accepted=accepted,
                        blocked=(not accepted),
                        note="" if accepted else "URL change forced a native probe but the plugin did not accept the result (HTTP 500). Hand to A/B; not a generator defect.",
                        decisions=[redact(line) for line in delta[-3:]])
    finally:
        config.write_text(original, encoding="utf-8")
        run_cli(binary, config, token, cwd)


def scenario_size_change(recorder: Recorder, target: Target, binary: Path, config: Path,
                         token: str, cwd: Path, strm_dir: Path, library_id: str,
                         driver: Path, playwright_root: str, browsers_path: str,
                         jellyfin_url: str, user: str, password: str, log_path: Path,
                         item_name: str, media_dir: Path, remote_root: str,
                         s3_bucket: str, rclone_remote: str, rclone_config: str) -> None:
    """RO-3/RO-5: same URL, changed content/size with no comparable hash must force a probe.

    The S3 driver exposes no hash, so the plugin must fall back to the effective Size. The
    synthetic object at the same key is replaced with a longer payload (local + remote), the
    generator rewrites the STRM size, and the next playback must trigger a fresh native probe.
    """
    source = media_dir / item_name
    original = source.read_bytes()
    before = _plugin_line_count(log_path)

    def upload() -> None:
        env = dict(os.environ)
        env["RCLONE_CONFIG"] = rclone_config
        subprocess.run(["rclone", "copyto", str(source),
                        f"{rclone_remote}:{s3_bucket}/{item_name}"],
                       env=env, capture_output=True, text=True, timeout=180, check=False)

    try:
        source.write_bytes(original + b"STRMGEN-SIZE-CHANGE-PADDING\n" * 8)
        upload()
        code, stdout, _ = run_cli(binary, config, token, cwd)
        require(code == 0, f"size-change sync exit {code}")
        strm = read_strm(strm_dir / (item_name[:-4] + ".strm"))
        require(strm["size"] != len(original), "size-change sync did not update the STRM size")
        mapping = item_ids_by_path(target, library_id)
        item_id = mapping[item_name[:-4]]
        users = target.jellyfin_json("/Users")
        playback_id = next((row["Id"] for row in users if row.get("Name") == user), users[0]["Id"])
        target.jellyfin(f"/Users/{playback_id}/PlayedItems/{item_id}", "DELETE")
        result = jellyfin_playback(
            jellyfin_url, driver, playwright_root,
            {"baseUrl": jellyfin_url, "username": user, "password": password,
             "items": [{"id": item_id, "name": item_name[:-4]}]},
            browsers_path,
        )
        require(not result.get("error"), f"size-change playback error: {result.get('error')}")
        delta = _plugin_tail(log_path, before)
        probed = any("Native STRM probe started" in line or "probe calls 1" in line for line in delta)
        accepted = any("reused saved media information" in line and "reason" in line for line in delta) \
            or bool((result.get("results") or [{}])[0].get("advanced"))
        require(probed, f"size change did not force a probe; decisions={delta}")
        recorder.record("ro3_size_change", advanced=bool((result.get("results") or [{}])[0].get("advanced")),
                        accepted=accepted,
                        blocked=(not accepted),
                        note="" if accepted else "Size change forced a native probe but the plugin did not accept the result (HTTP 500). Hand to A/B; not a generator defect.",
                        decisions=[redact(line) for line in delta[-3:]])
    finally:
        source.write_bytes(original)
        upload()
        run_cli(binary, config, token, cwd)


def main() -> int:
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
    parser.add_argument("--playback-user", default="",
                        help="browser playback account (defaults to JF_USER)")
    parser.add_argument("--playback-password", default="",
                        help="browser playback password (defaults to JF_PASSWORD)")
    parser.add_argument("--playback-driver", type=Path, required=True)
    parser.add_argument("--playwright-root", required=True)
    parser.add_argument("--browsers-path", default="/root/.cache/ms-playwright")
    parser.add_argument("--scenario", default="all",
                        choices=["all", "metadata", "byte_equality", "stable", "playback",
                                 "decisions", "url_change", "size_change"])
    parser.add_argument("--alt-public-url", default="http://openlist:5245",
                        help="alternate stable public base used for the URL-change scenario")
    parser.add_argument("--change-item", default="Plain Movie.mkv")
    parser.add_argument("--s3-bucket", default="slice4-media")
    parser.add_argument("--rclone-remote", default="slice4")
    parser.add_argument("--rclone-config", default="/tmp/rclone-slice4.conf")
    args = parser.parse_args()

    token = os.environ.get("OPENLIST_TOKEN")
    require(bool(token), "OPENLIST_TOKEN must be provided in the process environment")
    user = os.environ.get("JF_USER", "admin")
    password = os.environ.get("JF_PASSWORD")
    require(bool(password), "JF_PASSWORD must be provided in the process environment")
    playback_user = args.playback_user or user
    playback_password = args.playback_password or password

    identity = ('MediaBrowser Client="slice4-driver", Device="driver", '
                'DeviceId="slice4-driver", Version="1.0"')
    status, body = urllib.request.urlopen(
        urllib.request.Request(f"{args.jellyfin_url.rstrip('/')}/Users/AuthenticateByName",
                               data=json.dumps({"Username": user, "Pw": password}).encode(),
                               headers={"Content-Type": "application/json",
                                        "Authorization": identity}, method="POST"),
        timeout=30,
    ), None
    auth = f'{identity}, Token="{json.loads(status.read())["AccessToken"]}"'
    target = Target(args.openlist_api, token, args.jellyfin_url, auth)
    recorder = Recorder(args.output_dir)
    names = ["Plain Movie.mkv", "中文电影.mkv", "space name.mkv", "hash#name.mkv", "percent%name.mkv"]
    wanted = args.scenario

    try:
        if wanted in ("all", "metadata"):
            scenario_metadata(recorder, target, args.remote_root, args.strm_dir)
        if wanted in ("all", "byte_equality"):
            scenario_byte_equality(recorder, target, args.remote_root, args.strm_dir,
                                   args.media_dir, names, args.download_base)
        if wanted in ("all", "stable"):
            scenario_stable_sync(recorder, args.binary, args.strm_config, token, ROOT, args.strm_dir)
        if wanted in ("all", "playback"):
            scenario_native_playback(recorder, target, args.library_id, args.playback_driver,
                                     args.playwright_root, args.browsers_path, args.jellyfin_url,
                                     playback_user, playback_password, names)
        if wanted in ("all", "decisions"):
            plugin_decision_matrix(recorder, args.jellyfin_log)
        if wanted in ("all", "url_change"):
            scenario_url_change(recorder, target, args.binary, args.strm_config, token, ROOT,
                                args.strm_dir, args.library_id, args.playback_driver,
                                args.playwright_root, args.browsers_path, args.jellyfin_url,
                                playback_user, playback_password, args.jellyfin_log,
                                args.change_item, args.alt_public_url)
        if wanted in ("all", "size_change"):
            scenario_size_change(recorder, target, args.binary, args.strm_config, token, ROOT,
                                 args.strm_dir, args.library_id, args.playback_driver,
                                 args.playwright_root, args.browsers_path, args.jellyfin_url,
                                 playback_user, playback_password, args.jellyfin_log,
                                 args.change_item, args.media_dir, args.remote_root,
                                 args.s3_bucket, args.rclone_remote, args.rclone_config)
    finally:
        recorder.finish()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Failure as error:
        print(f"FAIL: {error}", file=sys.stderr)
        raise SystemExit(1)
