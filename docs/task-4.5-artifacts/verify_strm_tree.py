#!/usr/bin/env python3
"""Verify the generated STRM tree against the fixed Task 4.4 manifest.

Checks, for all 10000 files:
  * exact one-to-one path mapping (media path -> same relative .strm path)
  * V1 header, UTF-8, LF, total bytes <= 65536
  * stable http(s) URL, no query/fragment/user-info/signature
  * optional sha1 (40 lowercase hex) and size (non-negative int) match the
    manifest exactly when present; nothing fabricated
  * public URL uses the configured public_url host and percent-encoded path

Read-only. Prints a JSON summary.
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.parse
from pathlib import Path

MAX_BYTES = 65536


def parse(raw: bytes):
    if len(raw) > MAX_BYTES:
        raise ValueError("exceeds 65536 bytes")
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as error:
        raise ValueError("not UTF-8") from error
    lines = text.split("\n")
    if lines and lines[-1] == "":
        lines = lines[:-1]
    doc = {"sha1": None, "size": None, "url": None, "version": None}
    for line in lines:
        if line.startswith("# strm-probe-version:"):
            doc["version"] = line[len("# strm-probe-version:"):]
        elif line.startswith("# strm-probe-sha1:"):
            doc["sha1"] = line[len("# strm-probe-sha1:"):]
        elif line.startswith("# strm-probe-size:"):
            doc["size"] = line[len("# strm-probe-size:"):]
        elif line.startswith("#"):
            raise ValueError("unexpected comment")
        elif doc["url"] is None:
            doc["url"] = line
        else:
            raise ValueError("multiple URL lines")
    return doc


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--public-url", required=True)
    parser.add_argument("--source-root", default="/s3")
    args = parser.parse_args(argv)

    records = [json.loads(line) for line in Path(args.manifest).read_text(encoding="utf-8").splitlines() if line.strip()]
    out_root = Path(args.output)
    pub = args.public_url.rstrip("/")
    problems = []
    checked = 0
    sha_present = 0
    size_present = 0
    max_len = 0
    for rec in records:
        rel = rec["path"][:-4] + ".strm"
        target = out_root / rel
        if not target.is_file():
            problems.append(f"missing {rel}")
            continue
        raw = target.read_bytes()
        max_len = max(max_len, len(raw))
        try:
            doc = parse(raw)
        except ValueError as error:
            problems.append(f"{rel}: {error}")
            continue
        if doc["version"] != "1":
            problems.append(f"{rel}: version {doc['version']!r}")
        if doc["sha1"] is not None:
            sha_present += 1
            if doc["sha1"] != rec["sha1"]:
                problems.append(f"{rel}: sha1 mismatch")
        if doc["size"] is not None:
            size_present += 1
            if doc["size"] != str(rec["size"]):
                problems.append(f"{rel}: size mismatch")
        url = doc["url"] or ""
        expected_url = pub + "/d" + args.source_root.rstrip("/") + "/" + "/".join(
            urllib.parse.quote(part, safe="") for part in rec["path"].split("/")
        )
        if url != expected_url:
            problems.append(f"{rel}: url mismatch")
        if any(ch in url for ch in ("?", "#", "@")):
            problems.append(f"{rel}: url has query/fragment/userinfo")
        checked += 1

    summary = {
        "manifest_records": len(records),
        "checked": checked,
        "sha1_fields_present": sha_present,
        "size_fields_present": size_present,
        "max_document_bytes": max_len,
        "problem_count": len(problems),
        "problems_sample": problems[:20],
        "result": "PASS" if not problems and checked == len(records) else "FAIL",
    }
    print(json.dumps(summary, indent=2, ensure_ascii=False))
    return 0 if summary["result"] == "PASS" else 2


if __name__ == "__main__":
    raise SystemExit(main())
