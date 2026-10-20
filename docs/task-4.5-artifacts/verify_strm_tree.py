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
import os
import re
import stat
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
    if "\r" in text:
        raise ValueError("expected LF line endings")
    lines = text.split("\n")
    if lines and lines[-1] == "":
        lines = lines[:-1]
    doc = {"sha1": None, "size": None, "url": None, "version": None}
    for line in lines:
        for field in ("version", "sha1", "size"):
            if line.startswith(f"# strm-probe-{field}:") and doc[field] is not None:
                raise ValueError("duplicate metadata field")
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
    if doc["version"] != "1":
        raise ValueError("missing or invalid version")
    if doc["sha1"] is not None and not re.fullmatch(r"[0-9a-f]{40}", doc["sha1"]):
        raise ValueError("invalid SHA-1")
    if doc["size"] is not None:
        size = doc["size"]
        if not re.fullmatch(r"[0-9]+", size):
            raise ValueError("invalid Size")
        significant = size.lstrip("0") or "0"
        if (len(significant), significant) > (19, "9223372036854775807"):
            raise ValueError("Size exceeds Int64")
    try:
        url = urllib.parse.urlsplit(doc["url"] or "")
        valid_url = (url.scheme in ("http", "https") and url.hostname
                     and not url.username and not url.password
                     and not url.query and not url.fragment)
        url.port  # Reject malformed ports too.
    except ValueError as error:
        raise ValueError("invalid URL") from error
    if not valid_url or any(c.isspace() or ord(c) < 32 for c in (doc["url"] or "")):
        raise ValueError("missing or invalid stable HTTP(S) URL")
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
    expected = set()
    if not records:
        problems.append("empty manifest")
    safe_records = []
    for rec in records:
        path = rec["path"]
        if (not path or "\\" in path or "\x00" in path
                or any(part in ("", ".", "..") for part in path.split("/"))):
            problems.append("unsafe manifest path")
            continue
        rel = str(Path(path).with_suffix(".strm"))
        if rel in expected:
            problems.append("duplicate manifest output mapping")
            continue
        expected.add(rel)
        safe_records.append((rec, rel))
    actual = set()
    if any(p.is_symlink() for p in (out_root, *out_root.parents)):
        problems.append("output root contains symbolic link")
        safe_records = []
    elif not out_root.is_dir():
        problems.append("output root missing or not a directory")
        safe_records = []
    else:
        def walk_error(_error):
            problems.append("output enumeration failed")

        for parent, dirs, files in os.walk(out_root, followlinks=False, onerror=walk_error):
            for name in dirs + files:
                target = Path(parent) / name
                rel = target.relative_to(out_root).as_posix()
                mode = target.lstat().st_mode
                if stat.S_ISLNK(mode) or not (stat.S_ISDIR(mode) or stat.S_ISREG(mode)):
                    problems.append(f"{rel}: not a regular file or directory")
                elif stat.S_ISREG(mode):
                    actual.add(rel)
        for rel in sorted(actual - expected):
            problems.append(f"unexpected output {rel}")
    checked = 0
    sha_present = 0
    size_present = 0
    max_len = 0
    for rec, rel in safe_records:
        target = out_root / rel
        if rel not in actual or any(p.is_symlink() for p in (target, *target.parents)):
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
        checked += 1

    summary = {
        "manifest_records": len(records),
        "checked": checked,
        "output_files": len(actual),
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
