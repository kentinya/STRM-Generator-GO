#!/usr/bin/env python3
"""Task 4.6 only: native Jellyfin scenario phases for AC-1..AC-6.

Phases
  setup    -- record fixed inputs, identities and the plugin configuration
  ac1      -- original copy: native scan, native task read, plugin decision
  ac2      -- decision copy: normal comparison baseline A
  rows     -- D-01..D-08, one variable per row, restore after every row
  restore  -- restore every temporary change and verify the fixed dataset

Everything uses native Jellyfin surfaces of the verified isolated instance.
"""
from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import jfapi  # noqa: E402
import scenario  # noqa: E402
from scenario import (  # noqa: E402
    ADMIN_USER, BASELINE_TEXT, CONTENT_B, DECISION_FILE, DECISION_DIR, DECISION_LIBRARY,
    EVIDENCE, ORIGINAL_DIR, ORIGINAL_FILE, ORIGINAL_LIBRARY, REPO_ROOT, decide_from_log,
    item_detail, items_of, library_id, log, playback_info, plugin_config, read_decision,
    run_native_task, save, set_plugin_config, sha1_bytes, sha256_bytes, virtual_folders,
    write_decision,
)
import decision_model  # noqa: E402

A_SHA1 = "1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e"
A_SIZE = 24404
A_SHA256 = "7f53d1542382ea9035ca3059315b2ec0d2efd47bc5d70b0cc36cfd24bf96f12d"
B_SHA1 = CONTENT_B["sha1"]
B_SIZE = CONTENT_B["size"]
B_SHA256 = "e1aee32fbd9ddc13b970bbc644861b5fced7b2b8447542d9eedac8e02c4d7600"
A_FILE = REPO_ROOT / "artifacts" / "task-4.4" / "inputs" / "A.mkv"
B_FILE = REPO_ROOT / "artifacts" / "task-4.4" / "inputs" / "B.mkv"
SAMPLE = "Plain Movie.mkv"
ALTERNATE = "Synthetic 001.mkv"
OPENLIST_BASE = "http://127.0.0.1:25244"

# The historical side of the comparison comes from the plugin's own decision
# log: the AC-2 baseline was accepted when the plugin itself recorded an
# explicit Sha1Matches reuse, and every row restore re-confirms that same
# reuse. This is the last reason the plugin held for this item; it is never
# taken from the plugin's database (forbidden, F-46-02).
BASELINE_REASON = "Sha1Matches"
# The baseline A copy URL: the address the plugin's own reuse was recorded
# against, used to mirror the historical side without any database read.
BASELINE_URL = "http://openlist:5244/d/s3/Plain%20Movie.mkv"

# Task Base b599dbc5..69d022e history note: that implementation read the
# plugin's own fingerprint store through the now-retired fingerprint_read.py
# entry to record fingerprint_before/after/after_restore per row. That direct
# database dependency violated the Task boundary (AC-2 forbids Jellyfin / plugin
# database reads; F-46-02 in docs/task-4.6-b-review-2.md) and has been removed.
# The historical side of every row is established by native evidence only: the
# AC-2 baseline reuse and each row's post-restore explicit reuse from the
# plugin's own decision log. The rows in the 2026-10-21 evidence root
# task-4.6-dev-20261021T153126Z keep those recorded values; they are preserved
# as-is and marked non-compliant in the reviewer record, not deleted or
# rewritten. This implementation never touches the plugin's database and never
# predicts a reason from values taken out of it.

RESULTS: dict[str, object] = {}


# ---------------------------------------------------------------- fixed inputs
def sha256_of(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def service_bytes(url_path: str) -> dict:
    """Read the actual bytes the normal stable OpenList entry serves."""
    import urllib.request

    url = f"{OPENLIST_BASE}/d/s3/{url_path.replace(' ', '%20')}"
    with urllib.request.urlopen(url, timeout=60) as response:
        data = response.read()
    return {"url_path": url_path, "size": len(data), "sha1": sha1_bytes(data),
            "sha256": sha256_bytes(data), "http_status": response.status}


def s3_listing() -> dict:
    out = subprocess.run(
        ["python3", str(REPO_ROOT / "artifacts" / "task-4.6" / "tools" / "s3ops.py"), "list"],
        capture_output=True, text=True, timeout=600, check=True)
    return json.loads(out.stdout)


def s3_cat(name: str) -> dict:
    out = subprocess.run(
        ["python3", str(REPO_ROOT / "artifacts" / "task-4.6" / "tools" / "s3ops.py"), "cat", name],
        capture_output=True, text=True, timeout=600, check=True)
    return json.loads(out.stdout)


def s3_put(name: str, source: Path, set_mtime: str | None = None) -> dict:
    """Replace the content of one confirmed own object, keeping its path.

    ``set_mtime`` publishes an explicit object modification time without
    changing a single byte, so a source-side modification time change can be
    produced and then read back from the store.
    """
    if set_mtime is not None:
        stamp = time.mktime(time.strptime(set_mtime, "%Y-%m-%dT%H:%M:%SZ"))
        staged_dir = tempfile.mkdtemp(prefix="task46-mtime-")
        staged = Path(staged_dir) / source.name
        shutil.copy2(source, staged)
        os.utime(staged, (stamp, stamp))
        source = staged
    out = subprocess.run(
        ["python3", str(REPO_ROOT / "artifacts" / "task-4.6" / "tools" / "s3ops.py"), "put",
         name, str(source)],
        capture_output=True, text=True, timeout=600, check=True)
    return json.loads(out.stdout)


def s3_modtime(name: str) -> str | None:
    """Read the object modification time the S3 store itself reports."""
    out = subprocess.run(
        ["python3", str(REPO_ROOT / "artifacts" / "task-4.6" / "tools" / "s3ops.py"), "lsjson", name],
        capture_output=True, text=True, timeout=600, check=True)
    return json.loads(out.stdout)[0].get("ModTime")


def openlist_meta(path: str) -> dict:
    """Read the OpenList metadata of one object (read-only)."""
    import urllib.request

    req = urllib.request.Request(
        OPENLIST_BASE + "/api/fs/get",
        data=json.dumps({"path": path}).encode("utf-8"),
        headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=60) as response:
        body = json.loads(response.read())
    data = body.get("data") or {}
    return {"code": body.get("code"), "name": data.get("name"), "size": data.get("size"),
            "modified": data.get("modified"), "is_dir": data.get("is_dir")}


def phase_setup() -> None:
    manifest = REPO_ROOT / "artifacts" / "task-4.4" / "dataset" / "manifest.jsonl"
    payload = {
        "captured_utc": scenario.utc_now(),
        "repo_head": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO_ROOT,
                                             text=True).strip(),
        "git_status": subprocess.check_output(["git", "status", "--short", "--branch"],
                                              cwd=REPO_ROOT, text=True),
        "fixed_inputs": {
            "manifest_sha256": sha256_of(manifest),
            "manifest_lines": len(manifest.read_text().splitlines()),
            "strmgen_cli_sha256": sha256_of(REPO_ROOT / "artifacts" / "task-4.5" / "bin" / "strmgen-amd64"),
            "plugin_dll_sha256": sha256_of(REPO_ROOT / "artifacts" / "task-4.1" / "isolated-s3" /
                                           "plugin" / "Jellyfin.Plugin.StrmProbeOptimizer_0.1.0.0" /
                                           "Jellyfin.Plugin.StrmProbeOptimizer.dll"),
            "plugin_source_head": subprocess.check_output(
                ["git", "-C", str(REPO_ROOT / "artifacts" / "task-4.1" / "plugin-source"),
                 "rev-parse", "HEAD"], text=True).strip(),
            "A_original": {"path": str(A_FILE.relative_to(REPO_ROOT)), "size": A_FILE.stat().st_size,
                           "sha1": sha1_bytes(A_FILE.read_bytes()), "sha256": sha256_of(A_FILE)},
            "B_original": {"path": str(B_FILE.relative_to(REPO_ROOT)), "size": B_FILE.stat().st_size,
                           "sha1": sha1_bytes(B_FILE.read_bytes()), "sha256": sha256_of(B_FILE)},
        },
        "sample_registry": {
            SAMPLE: {"content": "A", "role": "decision URL target, D-02/04/06 content swap"},
            ALTERNATE: {"content": "A", "role": "D-05 alternate stable URL with the same content"},
        },
        "original_copy": {
            "source": "artifacts/task-4.5/strm-output/Plain Movie.strm",
            "source_sha256": sha256_of(REPO_ROOT / "artifacts" / "task-4.5" / "strm-output" / "Plain Movie.strm"),
            "copy": str(ORIGINAL_FILE.relative_to(REPO_ROOT)),
            "copy_sha256": sha256_of(ORIGINAL_DIR / "Plain Movie.strm"),
            "identical": (sha256_of(REPO_ROOT / "artifacts" / "task-4.5" / "strm-output" / "Plain Movie.strm")
                          == sha256_of(ORIGINAL_DIR / "Plain Movie.strm")),
            "bytes": (ORIGINAL_DIR / "Plain Movie.strm").stat().st_size,
        },
        "plugin_configuration_before": plugin_config(),
        "fingerprint_store_reads": "none: direct plugin database access is forbidden "
                                  "for Task 4.6 (F-46-02)",
        "libraries": [{"Name": v["Name"], "ItemId": v["ItemId"], "Locations": v["Locations"]}
                      for v in virtual_folders()],
    }
    save("00-setup-inputs.json", payload)
    RESULTS["setup"] = payload


# ------------------------------------------------------------------------- ac1
def phase_ac1() -> None:
    original_library = library_id(ORIGINAL_LIBRARY)
    items = items_of(original_library)
    if len(items) != 1:
        raise RuntimeError(f"original library must hold exactly one item, found {len(items)}")
    item = items[0]
    item_id = item["Id"]

    # Native plugin task, scoped to the original library only.
    set_plugin_config(BatchAllLibraries=False, BatchLibraryIds=original_library)
    task = run_native_task()
    task_facts = decide_from_log(task["log_window"], item_id)

    # Native public playback entry: the plugin's own per-item decision log.
    play = playback_info(item_id)
    play_facts = decide_from_log(play["log_window"], item_id)

    payload = {
        "captured_utc": scenario.utc_now(),
        "library": {"Name": ORIGINAL_LIBRARY, "ItemId": original_library},
        "item": {"Id": item_id, "Path": item.get("Path"), "Name": item.get("Name")},
        "source_item": item_detail(item_id).get("MediaSources"),
        "original_bytes_sha256": sha256_of(ORIGINAL_DIR / "Plain Movie.strm"),
        "native_task": {"task_id": task["task_id"], "state": task["state"],
                        "started_utc": task["started_utc"], "ended_utc": task["ended_utc"],
                        "facts": task_facts, "summary": task["task_summary"]},
        "playback_info": {"http_status": play["http_status"], "error": play["error"],
                          "started_utc": play["started_utc"], "ended_utc": play["ended_utc"],
                          "facts": play_facts},
        "remote_url_reachable": {
            "url": "http://127.0.0.1:25244/d/s3/Plain%20Movie.mkv",
            "note": "the original generated URL uses the host loopback address, not the "
                    "container-visible OpenList service name",
        },
    }
    save("01-ac1-original-scan-read.json", payload)
    RESULTS["ac1"] = payload


# ------------------------------------------------------------------------- ac2
def establish_baseline(item_id: str, tag: str) -> dict:
    """Establish the baseline history, then observe one explicit reuse.

    The first check may only be the FirstTrust initialisation. The baseline is
    only accepted once a later check shows a reason-matched explicit reuse, so
    FirstTrust alone never enters the comparison matrix.
    """
    steps = []
    for index in range(1, 5):
        info = playback_info(item_id)
        facts = decide_from_log(info["log_window"], item_id)
        steps.append({"call": index, "http_status": info["http_status"], "facts": facts})
        if facts["reason"] == "Sha1Matches":
            break
    else:
        raise RuntimeError("baseline reuse was not observed")
    return {"tag": tag, "steps": steps,
            "baseline_reason": steps[-1]["facts"]["reason"]}


def phase_ac2() -> None:
    decision_library = library_id(DECISION_LIBRARY)
    set_plugin_config(BatchAllLibraries=False, BatchLibraryIds=decision_library)
    items = items_of(decision_library)
    if len(items) != 1:
        raise RuntimeError(f"decision library must hold exactly one item, found {len(items)}")
    item_id = items[0]["Id"]

    write_decision(BASELINE_TEXT)
    baseline = establish_baseline(item_id, "baseline-A")
    task = run_native_task()
    task_facts = decide_from_log(task["log_window"], item_id)
    payload = {
        "captured_utc": scenario.utc_now(),
        "library": {"Name": DECISION_LIBRARY, "ItemId": decision_library},
        "item": {"Id": item_id, "Path": items[0].get("Path")},
        "decision_copy": read_decision(),
        "service_bytes": service_bytes(SAMPLE),
        "baseline": baseline,
        "native_task": {"task_id": task["task_id"], "state": task["state"],
                        "started_utc": task["started_utc"], "ended_utc": task["ended_utc"],
                        "facts": task_facts, "summary": task["task_summary"]},
        "item_detail": item_detail(item_id),
    }
    save("02-ac2-baseline.json", payload)
    RESULTS["ac2"] = payload


# ------------------------------------------------------------------------ rows
def restore_baseline(item_id: str, tag: str) -> dict:
    """Return the sample and the copy to baseline A and confirm the reuse.

    A row that required a probe may have committed a new fingerprint and a new
    native address, so the restore is confirmed by the same explicit reuse the
    baseline is defined by, and it may take more than one native check.
    """
    steps = []
    remote = s3_put(SAMPLE, A_FILE)
    steps.append({"action": "s3-restore-A", "result": remote})
    served = service_bytes(SAMPLE)
    steps.append({"action": "service-bytes", "result": served})
    if served["sha1"] != A_SHA1:
        raise RuntimeError("service bytes did not return to content A")
    write_decision(BASELINE_TEXT)
    steps.append({"action": "copy-restore-baseline", "result": read_decision()})
    confirmed = None
    for index in range(1, 7):
        info = playback_info(item_id)
        facts = decide_from_log(info["log_window"], item_id)
        steps.append({"action": "playback-restore", "call": index,
                      "http_status": info["http_status"], "facts": facts})
        confirmed = facts
        if facts["reason"] == "Sha1Matches":
            break
    if confirmed is None or confirmed["reason"] != "Sha1Matches":
        raise RuntimeError("the baseline reuse was not restored")
    return {"tag": tag, "steps": steps, "confirmed": confirmed}


def fingerprint_of(item_id: str) -> dict | None:
    """Removed: direct reads of the plugin's fingerprint store are forbidden.

    Retired with F-46-02 (docs/task-4.6-b-review-2.md). Kept as a fail-closed
    stub so any accidental caller fails loudly instead of silently reading the
    plugin's database.
    """
    raise RuntimeError(
        "fingerprint_of was removed: the plugin's own database may not be read "
        "for Task 4.6; see docs/task-4.6-b-review-2.md (F-46-02)")


def predicted_reason(copy_text: str, historical_reason: str | None,
                     native_url: str | None, has_primary_stream: bool,
                     baseline_url: str, baseline_sha1: str | None,
                     baseline_size: int | None) -> dict:
    """Predict the branch the locked plugin must take for this input.

    This only predicts which branch a given (manifest, baseline history,
    native address) pair must produce, so a row whose observed behaviour comes
    from an unrelated condition is detected instead of being accepted.

    The historical side is the reason the plugin itself last recorded for this
    item in native logs (the AC-2 baseline or the previous row's restored
    reuse); it is mirrored from that reason plus the recorded baseline A facts
    and is never taken from the plugin's database.
    """
    manifest = decision_model.parse_manifest(copy_text)
    previous = None
    if historical_reason in ("Sha1Matches", "Unchanged", "InsufficientFingerprint"):
        # The history is bound to the baseline A URL and facts, not to the
        # copy being tested: a changed copy URL must still be a UrlChanged
        # against the mirrored history.
        sha1 = baseline_sha1 if historical_reason == "Sha1Matches" else None
        size = baseline_size if historical_reason in ("Sha1Matches", "Unchanged") else None
        previous = decision_model.Fingerprint(decision_model.hash_url(baseline_url),
                                              sha1, size)
    # FirstTrust means the plugin recorded no comparable history at all, so
    # the mirror stays None and the branch is FirstTrust.
    kind, reason = decision_model.decide(manifest, previous, has_primary_stream, native_url)
    return {"kind": kind, "reason": reason, "manifest_url": manifest.url,
            "manifest_sha1": manifest.sha1, "manifest_size": manifest.size,
            "manifest_url_hash": manifest.url_hash,
            "history_side": {"source": "plugin decision log only",
                             "last_reason": historical_reason,
                             "baseline_url": baseline_url,
                             "fingerprint_store_read": False},
            "previous_url_hash": previous.url_hash if previous else None,
            "previous_sha1": previous.sha1 if previous else None,
            "previous_size": previous.size if previous else None,
            "native_url": native_url, "has_primary_stream": has_primary_stream}


def native_url_of(item_id: str) -> str | None:
    detail = item_detail(item_id)
    for media_source in detail.get("MediaSources") or []:
        if media_source.get("Id") == item_id:
            return media_source.get("Path")
    sources = detail.get("MediaSources") or []
    return sources[0].get("Path") if sources else None


def has_primary_stream(item_id: str) -> bool:
    detail = item_detail(item_id)
    for media_source in detail.get("MediaSources") or []:
        for stream in media_source.get("MediaStreams") or []:
            if stream.get("Type") == "Video":
                return True
    return False


def row_result(row: str, item_id: str, change: dict, expected: str,
               historical_reason: str | None, baseline_sha1: str, baseline_size: int,
               expected_branch: str | list[str] | None = None) -> dict:
    """Apply one single-variable change, observe the decision, restore.

    The per-item decision is taken from the plugin's own per-item decision log
    inside the observation window. The native scheduled task is also executed in
    the same window as auxiliary native-task evidence; its aggregate counters
    never replace the per-item decision.

    ``historical_reason`` is the reason the plugin itself last recorded for
    this item in native logs (the AC-2 baseline or the previous row's restored
    reuse); it is the historical side of the comparison and is never read from
    the plugin's database. ``baseline_sha1`` / ``baseline_size`` are the
    baseline A content facts the copy is restored to between rows; they are
    recorded as row inputs for review, not read from any store.

    When the observation produced no explicit reason because the row required a
    probe, the probe start is the observed decision and the predicted branch
    must still be the row's own branch.
    """
    write_decision(change["copy_text"])
    before = read_decision()
    prediction = predicted_reason(before["text"], historical_reason,
                                  native_url_of(item_id), has_primary_stream(item_id),
                                  BASELINE_URL, baseline_sha1, baseline_size)
    info = playback_info(item_id)
    facts = decide_from_log(info["log_window"], item_id)
    task = run_native_task()
    task_facts = decide_from_log(task["log_window"], item_id)
    detail = item_detail(item_id)
    payload = {
        "row": row,
        "expected": expected,
        "change": {k: v for k, v in change.items() if k != "copy_text"},
        "copy_before": before,
        "baseline_input": {"baseline_sha1": baseline_sha1, "baseline_size": baseline_size,
                           "historical_reason": historical_reason,
                           "historical_side_source": "native plugin decision log",
                           "fingerprint_store_read": False},
        "predicted_branch": prediction,
        "observation": {"entry": "public native PlaybackInfo of this item",
                        "http_status": info["http_status"], "error": info["error"],
                        "started_utc": info["started_utc"], "ended_utc": info["ended_utc"],
                        "facts": facts},
        "native_task": {"entry": "native scheduled task STRM 媒体信息检查",
                        "task_id": task["task_id"], "state": task["state"],
                        "started_utc": task["started_utc"], "ended_utc": task["ended_utc"],
                        "summary": task["task_summary"], "facts": task_facts},
        "item_after": detail.get("MediaSources"),
    }
    restored = restore_baseline(item_id, row + "-restore")
    payload["restore"] = restored
    observed = facts["reason"] or ("Native STRM probe started" if facts["probe_started"] else None)
    payload["observed_decision"] = observed
    payload["expected_branch"] = expected_branch
    accepted = expected_branch if isinstance(expected_branch, (list, tuple)) else [expected_branch]
    payload["causality"] = {
        "predicted_reason": prediction["reason"],
        "predicted_kind": prediction["kind"],
        "predicted_is_accepted_branch": prediction["reason"] in accepted,
        "observed_matches_predicted": (observed == prediction["reason"]
                                       if facts["reason"] else prediction["kind"] == "RequireProbe"),
        "observed_is_accepted_branch": observed in accepted,
    }
    return payload


def phase_rows() -> None:
    decision_library = library_id(DECISION_LIBRARY)
    set_plugin_config(BatchAllLibraries=False, BatchLibraryIds=decision_library)
    items = items_of(decision_library)
    item_id = items[0]["Id"]

    rows: list[dict] = []

    # D-01: nothing changes at all.
    rows.append(row_result("D-01", item_id, {"copy_text": BASELINE_TEXT,
                                             "change": "no change at all"},
                           "reuse / Sha1Matches", BASELINE_REASON,
                           A_SHA1, A_SIZE, "Sha1Matches"))

    # D-02: only the source-side modification time changes; content, size and
    # the copied STRM bytes stay identical. The object is rewritten with the
    # identical A bytes under an explicitly different modification time, and the
    # S3 store's own modification time is read back to prove it really changed.
    before_list = s3_listing()
    before_meta = openlist_meta("/s3/" + SAMPLE)
    before_s3_mtime = s3_modtime(SAMPLE)
    new_mtime = "2026-10-21T18:00:00Z"
    put = s3_put(SAMPLE, A_FILE, new_mtime)
    after_meta = openlist_meta("/s3/" + SAMPLE)
    after_s3_mtime = s3_modtime(SAMPLE)
    served = service_bytes(SAMPLE)
    if before_s3_mtime == after_s3_mtime:
        raise RuntimeError("the source-side modification time did not change")
    if served["sha1"] != A_SHA1 or served["size"] != A_SIZE:
        raise RuntimeError("the rewritten source content is not identical to content A")
    rows.append(row_result("D-02", item_id, {
        "copy_text": BASELINE_TEXT,
        "change": "source-side modification time only; identical bytes re-written to the same key",
        "s3_modified_before": before_s3_mtime,
        "s3_modified_after": after_s3_mtime,
        "s3_modified_changed": before_s3_mtime != after_s3_mtime,
        "openlist_modified_before": before_meta.get("modified"),
        "openlist_modified_after": after_meta.get("modified"),
        "source_size_after": after_meta.get("size"),
        "source_rewrite": put,
        "service_bytes_after": served,
        "s3_set_unchanged": sorted(before_list["paths"]) == sorted(s3_listing()["paths"]),
        "copy_bytes_unchanged": True,
    }, "reuse / Sha1Matches", BASELINE_REASON,
       A_SHA1, A_SIZE, "Sha1Matches"))

    # D-03: identical URL and identical real SHA-1, but the copied Size line is
    # deliberately wrong. The Size branch must not be reached.
    wrong_size_text = (
        "# strm-probe-version:1\n"
        f"# strm-probe-sha1:{A_SHA1}\n"
        f"# strm-probe-size:{B_SIZE}\n"
        "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
    )
    rows.append(row_result("D-03", item_id, {
        "copy_text": wrong_size_text,
        "change": "only the copied Size metadata is deliberately wrong; URL and real SHA-1 unchanged",
        "declared_size": B_SIZE,
        "real_size": A_SIZE,
    }, "reuse / Sha1Matches, never SizeChanged", BASELINE_REASON,
       A_SHA1, A_SIZE, "Sha1Matches"))

    # D-04: same URL, the served content really changes A -> B and the copy
    # carries the real SHA-1 and Size of B.
    put_b = s3_put(SAMPLE, B_FILE)
    served_b = service_bytes(SAMPLE)
    if served_b["sha1"] != B_SHA1:
        raise RuntimeError("service bytes did not change to content B")
    d04_text = (
        "# strm-probe-version:1\n"
        f"# strm-probe-sha1:{B_SHA1}\n"
        f"# strm-probe-size:{B_SIZE}\n"
        "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
    )
    rows.append(row_result("D-04", item_id, {
        "copy_text": d04_text,
        "change": "same URL; the served object content really changed A -> B; the copy carries "
                  "the real SHA-1 and Size of B",
        "source_rewrite": put_b,
        "service_bytes_after": served_b,
    }, "probe / Sha1Changed", BASELINE_REASON,
       A_SHA1, A_SIZE, "Sha1Changed"))

    # D-05: identical content, SHA-1 and Size, but a different verified stable
    # URL of the same batch that serves the same content A.
    alternate_served = service_bytes(ALTERNATE)
    if alternate_served["sha1"] != A_SHA1:
        raise RuntimeError("alternate URL does not serve content A")
    d05_text = (
        "# strm-probe-version:1\n"
        f"# strm-probe-sha1:{A_SHA1}\n"
        f"# strm-probe-size:{A_SIZE}\n"
        f"{CONTENT_B['url']}\n"
    )
    rows.append(row_result("D-05", item_id, {
        "copy_text": d05_text,
        "change": "same content, SHA-1 and Size; the URL changed to another verified stable URL "
                  "of the same batch that serves the same content A",
        "alternate_service_bytes": alternate_served,
    }, "probe / UrlChanged or this change's NativeUrlMismatch", BASELINE_REASON,
       A_SHA1, A_SIZE, ["UrlChanged", "NativeUrlMismatch"]))

    # D-06: current side SHA-1 missing, history A valid, same URL, and the
    # effective Size differs because the same URL now serves content B.
    put_b = s3_put(SAMPLE, B_FILE)
    served_b = service_bytes(SAMPLE)
    if served_b["sha1"] != B_SHA1:
        raise RuntimeError("service bytes did not change to content B for D-06")
    d06_text = (
        "# strm-probe-version:1\n"
        f"# strm-probe-size:{B_SIZE}\n"
        "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
    )
    rows.append(row_result("D-06", item_id, {
        "copy_text": d06_text,
        "change": "the current copy omits the SHA-1 line; the same URL now serves content B so the "
                  "effective Size differs from the valid historical A Size",
        "source_rewrite": put_b,
        "service_bytes_after": served_b,
        "history_sha1_valid": A_SHA1,
        "history_size": A_SIZE,
        "current_sha1": "omitted",
        "current_size": B_SIZE,
    }, "probe / SizeChanged", BASELINE_REASON,
       A_SHA1, A_SIZE, "SizeChanged"))

    # D-07: current side SHA-1 missing, history A valid, same URL, same
    # effective Size.
    put_a = s3_put(SAMPLE, A_FILE)
    served_a = service_bytes(SAMPLE)
    if served_a["sha1"] != A_SHA1:
        raise RuntimeError("service bytes did not return to content A for D-07")
    d07_text = (
        "# strm-probe-version:1\n"
        f"# strm-probe-size:{A_SIZE}\n"
        "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
    )
    rows.append(row_result("D-07", item_id, {
        "copy_text": d07_text,
        "change": "the current copy omits the SHA-1 line; the same URL still serves content A so the "
                  "effective Size is unchanged",
        "source_rewrite": put_a,
        "service_bytes_after": served_a,
        "history_sha1_valid": A_SHA1,
        "history_size": A_SIZE,
        "current_sha1": "omitted",
        "current_size": A_SIZE,
    }, "reuse / Unchanged", BASELINE_REASON,
       A_SHA1, A_SIZE, "Unchanged"))

    # D-08: current side omits SHA-1 and Size, history A valid, same URL.
    d08_text = (
        "# strm-probe-version:1\n"
        "http://openlist:5244/d/s3/Plain%20Movie.mkv\n"
    )
    rows.append(row_result("D-08", item_id, {
        "copy_text": d08_text,
        "change": "the current copy omits both the SHA-1 and the Size line; the same URL still serves "
                  "content A",
        "history_sha1_valid": A_SHA1,
        "history_size": A_SIZE,
        "current_sha1": "omitted",
        "current_size": "omitted",
    }, "reuse / InsufficientFingerprint", BASELINE_REASON,
       A_SHA1, A_SIZE, "InsufficientFingerprint"))

    payload = {"captured_utc": scenario.utc_now(),
               "item_id": item_id,
               "library": {"Name": DECISION_LIBRARY, "ItemId": decision_library},
               "historical_side": "native plugin decision log only; no plugin database read",
               "rows": rows}
    save("03-matrix-rows.json", payload)
    RESULTS["rows"] = payload


# --------------------------------------------------------------------- restore
def phase_restore() -> None:
    steps = []
    remote = s3_put(SAMPLE, A_FILE)
    steps.append({"action": "s3-restore-A", "result": remote})
    served = service_bytes(SAMPLE)
    steps.append({"action": "service-bytes", "result": served})
    write_decision(BASELINE_TEXT)
    steps.append({"action": "copy-restore-baseline", "result": read_decision()})
    listing = s3_listing()
    steps.append({"action": "s3-listing", "count": listing["count"]})
    original_config = (RESULTS.get("setup") or {}).get("plugin_configuration_before")
    if original_config is None:
        raise RuntimeError(
            "no setup-time plugin configuration was captured in this process; "
            "refusing to fall back to the current (possibly narrowed) "
            "configuration as the restore source. Run the setup phase in this "
            "process first, or restore the original plugin configuration "
            "through the native configuration page.")
    set_plugin_config(**original_config)
    steps.append({"action": "plugin-config-restored", "config": plugin_config()})
    payload = {"captured_utc": scenario.utc_now(), "steps": steps}
    save("04-restore.json", payload)
    RESULTS["restore"] = payload


PHASES = {"setup": phase_setup, "ac1": phase_ac1, "ac2": phase_ac2,
          "rows": phase_rows, "restore": phase_restore}


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: matrix.py " + "|".join(PHASES), file=sys.stderr)
        return 2
    started = time.monotonic()
    for name in sys.argv[1:]:
        if name not in PHASES:
            print("unknown phase: " + name, file=sys.stderr)
            return 2
        log(f"phase {name} start")
        PHASES[name]()
        log(f"phase {name} done in {time.monotonic() - started:.1f}s")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
