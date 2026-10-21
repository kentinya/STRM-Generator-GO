#!/usr/bin/env python3
"""Task 4.6 only: native Jellyfin scenario driver for the D-01..08 matrix.

Every action goes through the native Jellyfin surface of the already-verified
isolated instance:
  * native library management / scan
  * the fixed plugin's native scheduled task "STRM 媒体信息检查"
  * the public PlaybackInfo endpoint (this instance's own API description)

No plugin internal interface, no Jellyfin database write and no direct
fingerprint-store write is used. The plugin key is provided only through the
in-memory child environment of artifacts/task-4.6/auth/with_jellyfin_key.py.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import jfapi  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = Path(os.environ["TASK46_EVIDENCE"])
ISOLATED = REPO_ROOT / "artifacts" / "task-4.1" / "isolated-s3"
# The dedicated isolated Jellyfin libraries live outside the historical /strm tree,
# because a nested path under /strm is claimed by the existing "Slice4 STRM" library
# and can never link its own items.
JELLYFIN_CONFIG = ISOLATED / "jellyfin-config"
TASK46_DIR = JELLYFIN_CONFIG / "task46"
ORIGINAL_DIR = TASK46_DIR / "original"
DECISION_DIR = TASK46_DIR / "decision"
DECISION_FILE = DECISION_DIR / "Task46 Decision.strm"
ORIGINAL_FILE = ORIGINAL_DIR / "Plain Movie.strm"
JELLYFIN_LOG_DIR = JELLYFIN_CONFIG / "log"
PLUGIN_ID = "b061643ed17d4c6e8ba26b57e286bd20"
ADMIN_USER = "1737013ebb954e8ebc201d2bbb89c49d"

# The fixed baseline manifest of the labelled decision copy (content A).
BASELINE_TEXT = (
    "# strm-probe-version:1\n"
    "# strm-probe-sha1:1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e\n"
    "# strm-probe-size:24404\n"
    "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
)
# Content B is the same-batch second content of the fixed dataset.
CONTENT_B = {
    "sha1": "4799935022aa2bed93478865689aef92ab116ba5",
    "size": 24430,
    "url": "http://openlist:5244/d/s3/Synthetic%20001.mkv",
}


def utc_now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def log(message: str) -> None:
    print(f"[{utc_now()}] {message}", flush=True)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha1_bytes(data: bytes) -> str:
    return hashlib.sha1(data).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def jf_log_path() -> Path:
    day = time.strftime("%Y%m%d", time.gmtime())
    return JELLYFIN_LOG_DIR / f"log_{day}.log"


def jf_log_size() -> int:
    path = jf_log_path()
    return path.stat().st_size if path.exists() else 0


def read_jf_log_since(offset: int) -> str:
    path = jf_log_path()
    if not path.exists():
        return ""
    with path.open("r", encoding="utf-8", errors="replace") as handle:
        handle.seek(offset)
        return handle.read()


DECISION_RE = re.compile(
    r'STRM source "reused saved media information" for project (?P<project>[0-9a-f-]+), '
    r'source "(?P<source>[0-9a-f]+)"; reason (?P<reason>[A-Za-z0-9]+)'
)
PROBE_RE = re.compile(
    r"Native STRM probe started for item (?P<item>[0-9a-f-]+), source_key \"(?P<source>[0-9a-f]+)\""
)
PROBE_FAILED_RE = re.compile(
    r"Native STRM probe failed for item (?P<item>[0-9a-f-]+), source \"(?P<source>[0-9a-f]+)\""
)
PROBE_ACCEPTED_RE = re.compile(
    r"Native probe accepted for STRM item (?P<item>[0-9a-f-]+), source \"(?P<source>[0-9a-f]+)\""
)
TASK_ENTERED_RE = re.compile(r"STRM media information check entered")
TASK_FINISHED_RE = re.compile(r"STRM media information check finished: (?P<summary>.*)")


def decide_from_log(text: str, item_id: str) -> dict:
    """Extract this item's decision facts from one native log window."""
    item_compact = item_id.replace("-", "")
    facts = {"decision": None, "reason": None, "probe_started": False,
             "probe_failed": False, "probe_accepted": False, "task_summary": None,
             "task_entered": False}
    for line in text.splitlines():
        match = DECISION_RE.search(line)
        if match and match.group("project").replace("-", "") == item_compact:
            facts["decision"] = "reused saved media information"
            facts["reason"] = match.group("reason")
        match = PROBE_RE.search(line)
        if match and match.group("item").replace("-", "") == item_compact:
            facts["probe_started"] = True
        match = PROBE_FAILED_RE.search(line)
        if match and match.group("item").replace("-", "") == item_compact:
            facts["probe_failed"] = True
        match = PROBE_ACCEPTED_RE.search(line)
        if match and match.group("item").replace("-", "") == item_compact:
            facts["probe_accepted"] = True
        if TASK_ENTERED_RE.search(line):
            facts["task_entered"] = True
        match = TASK_FINISHED_RE.search(line)
        if match:
            facts["task_summary"] = match.group("summary")
    return facts


ORIGINAL_LIBRARY = "Task4.6+Original"
DECISION_LIBRARY = "Task4.6+Decision"


def virtual_folders() -> list[dict]:
    status, body = jfapi.get("/Library/VirtualFolders")
    assert status == 200, (status, body)
    return body


def library_id(name: str) -> str:
    for folder in virtual_folders():
        if folder["Name"] == name:
            return folder["ItemId"]
    raise RuntimeError("library not found: " + name)


def items_of(library: str) -> list[dict]:
    """List the movies of one library by its own location prefix.

    The native item parent is the physical folder, not the library id, so the
    library's own location path is the reliable selector.
    """
    location = None
    for folder in virtual_folders():
        if folder["ItemId"] == library:
            location = folder["Locations"][0]
            break
    if location is None:
        raise RuntimeError("library not found: " + library)
    status, body = jfapi.get(
        f"/Users/{ADMIN_USER}/Items",
        query={"Recursive": "true", "IncludeItemTypes": "Movie",
               "Fields": "Path,MediaSources,MediaStreams"},
    )
    assert status == 200, (status, body)
    prefix = location.rstrip("/") + "/"
    return [item for item in body.get("Items", [])
            if (item.get("Path") or "").startswith(prefix)]


def find_item_by_path(path: str) -> dict:
    for item in items_of(library_id(DECISION_LIBRARY)):
        if item.get("Path") == path:
            return item
    for item in items_of(library_id(ORIGINAL_LIBRARY)):
        if item.get("Path") == path:
            return item
    raise RuntimeError("item not found: " + path)


def item_detail(item_id: str) -> dict:
    status, body = jfapi.get(f"/Users/{ADMIN_USER}/Items/{item_id}",
                             query={"Fields": "Path,MediaSources,MediaStreams"})
    assert status == 200, (status, body)
    return body


def task_id() -> str:
    status, body = jfapi.get("/ScheduledTasks")
    assert status == 200
    for task in body:
        if task.get("Key") == "StrmProbeOptimizerBatchCheck":
            return task["Id"]
    raise RuntimeError("native task not found")


def plugin_config() -> dict:
    status, body = jfapi.get(f"/Plugins/{PLUGIN_ID}/Configuration")
    assert status == 200, (status, body)
    return body


def set_plugin_config(**changes) -> dict:
    current = plugin_config()
    current.update(changes)
    status, body = jfapi.post(f"/Plugins/{PLUGIN_ID}/Configuration", body=current)
    assert status in (200, 204), (status, body)
    return plugin_config()


def run_native_task(timeout: int = 900) -> dict:
    """Run the fixed plugin's native scheduled task and wait for it to finish."""
    identifier = task_id()
    offset = jf_log_size()
    started = utc_now()
    status, body = jfapi.post(f"/ScheduledTasks/Running/{identifier}")
    assert status in (200, 204), (status, body)
    deadline = time.time() + timeout
    state = None
    while time.time() < deadline:
        time.sleep(2)
        s, t = jfapi.get(f"/ScheduledTasks/{identifier}")
        assert s == 200, (s, t)
        state = t.get("State")
        if state != "Running":
            break
    ended = utc_now()
    time.sleep(1)
    window = read_jf_log_since(offset)
    facts = decide_from_log(window, "")
    (EVIDENCE / "task-window-last.log").write_text(window, encoding="utf-8")
    return {"task_id": identifier, "state": state, "started_utc": started, "ended_utc": ended,
            "log_window": window, "task_summary": facts["task_summary"],
            "task_entered": facts["task_entered"]}


def playback_info(item_id: str, timeout: int = 300) -> dict:
    offset = jf_log_size()
    started = utc_now()
    try:
        status, body = jfapi.get(f"/Items/{item_id}/PlaybackInfo", query={"userId": ADMIN_USER},
                                 timeout=timeout)
        error = None
    except urllib.error.HTTPError as exc:
        status, body, error = exc.code, None, f"HTTP {exc.code}"
    except Exception as exc:  # pragma: no cover - transport failure
        status, body, error = None, None, type(exc).__name__
    ended = utc_now()
    time.sleep(1)
    window = read_jf_log_since(offset)
    return {"http_status": status, "error": error, "started_utc": started, "ended_utc": ended,
            "body": body, "log_window": window}


def write_decision(text: str) -> dict:
    data = text.encode("utf-8")
    DECISION_FILE.write_bytes(data)
    return {"path": str(DECISION_FILE.relative_to(REPO_ROOT)), "bytes": len(data),
            "sha256": sha256_bytes(data), "text": text}


def read_decision() -> dict:
    data = DECISION_FILE.read_bytes()
    return {"path": str(DECISION_FILE.relative_to(REPO_ROOT)), "bytes": len(data),
            "sha256": sha256_bytes(data), "text": data.decode("utf-8")}


def save(name: str, payload) -> Path:
    path = EVIDENCE / name
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    log(f"saved {path.name}")
    return path
