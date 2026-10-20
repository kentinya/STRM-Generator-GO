#!/usr/bin/env python3
"""Read-only OpenList probe for Task 4.4 (AC-4).

Uses only the Python standard library and the guest/read path of the isolated
OpenList instance. It never writes, never sends a token, never prints raw_url or
signature fields, and never follows a signed URL.

It is a verification gate, not a formatter: any necessary check that fails
(sample Get type/size, stable bytes, incomplete pagination, manifest mismatch)
returns a non-zero exit code. It checks:

  * the configured source root is a directory,
  * complete recursive pagination with refresh:false (stable total per directory,
    no duplicate names, every directory reaches its total),
  * the recursive regular-file set and per-file Size match the fixed manifest,
  * per-name exact Get and stable public URL bytes for chosen samples,
  * the stable URL path is percent-encoded segment by segment.

Run from the repository root:

    python3 artifacts/task-4.4/tools/openlist_readonly_probe.py \
        --base-url http://127.0.0.1:25244 --source-root /s3 \
        --media-dir artifacts/task-4.1/isolated-s3/media \
        --manifest artifacts/task-4.4/dataset/manifest.jsonl \
        --sample "Synthetic 001.mkv" --sample "bulk/000001.mkv" ...
"""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
import urllib.parse
import urllib.request
from pathlib import Path

PER_PAGE = 200
TIMEOUT = 30


class ProbeError(Exception):
    """A necessary verification failure that maps to a non-zero exit code."""


def post_json(base_url, endpoint, payload):
    data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        base_url.rstrip("/") + endpoint,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=TIMEOUT) as response:
        return response.status, json.loads(response.read().decode("utf-8"))


def get_bytes(url):
    with urllib.request.urlopen(url, timeout=TIMEOUT) as response:
        return response.status, response.read()


def list_directory(base_url, path):
    """Return (entries, total) for one directory, paginating completely."""
    entries = []
    total = None
    page = 1
    while True:
        status, body = post_json(base_url, "/api/fs/list", {
            "path": path, "page": page, "per_page": PER_PAGE, "refresh": False,
        })
        if status != 200 or body.get("code") != 200:
            raise ProbeError(
                f"list failed for {path!r}: http={status} code={body.get('code')}"
            )
        data = body["data"]
        if total is None:
            total = data["total"]
        elif total != data["total"]:
            raise ProbeError(f"total changed during paging for {path!r}")
        content = data.get("content") or []
        if not content:
            break
        entries.extend(content)
        page += 1
        if len(entries) >= total:
            break
    names = [entry["name"] for entry in entries]
    if len(names) != len(set(names)):
        raise ProbeError(f"duplicate names while paging {path!r}")
    if total is not None and len(entries) != total:
        raise ProbeError(f"paged {len(entries)} != total {total} for {path!r}")
    return entries, total


def walk_tree(base_url, source_root):
    """Recursively enumerate regular files; return {rel_path: size}."""
    files = {}
    seen_dirs = set()
    stack = [""]
    while stack:
        rel_dir = stack.pop()
        api_path = source_root.rstrip("/") + ("/" + rel_dir if rel_dir else "")
        if api_path in seen_dirs:
            raise ProbeError(f"directory visited twice: {rel_dir!r}")
        seen_dirs.add(api_path)
        entries, _total = list_directory(base_url, api_path)
        for entry in entries:
            name = entry["name"]
            rel = f"{rel_dir}/{name}" if rel_dir else name
            if entry.get("is_dir"):
                stack.append(rel)
                continue
            if rel in files:
                raise ProbeError(f"duplicate relative file: {rel!r}")
            files[rel] = entry.get("size")
    return files


def stable_url(public_base, source_root, rel):
    segments = [urllib.parse.quote(part, safe="") for part in rel.split("/")]
    return (
        public_base.rstrip("/")
        + "/d"
        + source_root.rstrip("/")
        + "/"
        + "/".join(segments)
    )


def load_manifest(path):
    records = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            records.append(json.loads(line))
    return records


def check_manifest(remote_files, records):
    expected = {record["path"]: record["size"] for record in records}
    missing = sorted(set(expected) - set(remote_files))
    extra = sorted(set(remote_files) - set(expected))
    if missing:
        raise ProbeError(f"remote missing {len(missing)} manifest file(s): {missing[0]!r}")
    if extra:
        raise ProbeError(f"remote has {len(extra)} unplanned file(s): {extra[0]!r}")
    mismatched = [
        rel for rel, size in expected.items() if remote_files.get(rel) != size
    ]
    if mismatched:
        raise ProbeError(f"remote size mismatch: {mismatched[0]!r}")
    return len(expected)


def check_sample(base_url, public_base, source_root, media, rel):
    sample = {"path": rel}
    api_path = source_root.rstrip("/") + "/" + rel
    status, body = post_json(base_url, "/api/fs/get", {"path": api_path, "refresh": False})
    sample["get_http"] = status
    sample["get_code"] = body.get("code")
    if status != 200 or body.get("code") != 200:
        raise ProbeError(f"exact Get failed for {rel!r}: http={status} code={body.get('code')}")
    data = body["data"]
    sample["size"] = data.get("size")
    sample["is_dir"] = data.get("is_dir")
    sample["has_hash"] = bool(data.get("hash_info"))
    if data.get("is_dir"):
        raise ProbeError(f"exact Get returned a directory for {rel!r}")

    local = media / rel
    if not local.is_file() or local.is_symlink():
        raise ProbeError(f"local sample is not a regular file: {rel!r}")
    local_size = local.stat().st_size
    sample["local_size"] = local_size
    if data.get("size") != local_size:
        raise ProbeError(
            f"remote/local size mismatch for {rel!r}: {data.get('size')} != {local_size}"
        )

    url = stable_url(public_base, source_root, rel)
    http, payload = get_bytes(url)
    sample["stable_url_http"] = http
    sample["stable_url_size"] = len(payload)
    if http != 200:
        raise ProbeError(f"stable URL HTTP {http} for {rel!r}")
    if len(payload) != local_size:
        raise ProbeError(
            f"stable URL size mismatch for {rel!r}: {len(payload)} != {local_size}"
        )
    remote_sha = hashlib.sha256(payload).hexdigest()
    local_sha = hashlib.sha256(local.read_bytes()).hexdigest()
    sample["sha256_match"] = remote_sha == local_sha
    if not sample["sha256_match"]:
        raise ProbeError(f"stable URL bytes mismatch for {rel!r}")
    return sample


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--source-root", required=True)
    parser.add_argument("--media-dir", required=True)
    parser.add_argument("--manifest", default=None,
                        help="fixed manifest JSONL to compare the remote set against")
    parser.add_argument("--sample", action="append", default=[])
    parser.add_argument("--public-url", default=None,
                        help="public base URL for stable /d/ bytes; default --base-url")
    args = parser.parse_args(argv)

    media = Path(args.media_dir)
    report = {"source_root": args.source_root, "checks": {}}
    try:
        status, body = post_json(args.base_url, "/api/fs/get",
                                 {"path": args.source_root, "refresh": False})
        root_ok = (
            status == 200
            and body.get("code") == 200
            and body["data"].get("is_dir") is True
        )
        report["checks"]["root_is_directory"] = root_ok
        if not root_ok:
            raise ProbeError("source root is not a readable directory")

        remote_files = walk_tree(args.base_url, args.source_root)
        report["checks"]["remote_regular_files"] = len(remote_files)

        if args.manifest is not None:
            records = load_manifest(Path(args.manifest))
            matched = check_manifest(remote_files, records)
            report["checks"]["manifest_matched"] = matched

        public_base = args.public_url or args.base_url
        samples = []
        for rel in args.sample:
            samples.append(check_sample(
                args.base_url, public_base, args.source_root, media, rel))
        report["checks"]["samples"] = samples
    except ProbeError as error:
        report["checks"]["error"] = str(error)
        print(json.dumps(report, indent=2, ensure_ascii=False))
        return 2
    except Exception as error:  # noqa: BLE001 - report category only
        report["checks"]["error"] = type(error).__name__
        print(json.dumps(report, indent=2, ensure_ascii=False))
        return 2
    print(json.dumps(report, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
