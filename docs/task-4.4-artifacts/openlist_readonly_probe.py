#!/usr/bin/env python3
"""Read-only OpenList probe for Task 4.4 (AC-4).

Uses only the Python standard library and the guest/read path of the isolated
OpenList instance. It never writes, never sends a token, never prints raw_url or
signature fields, and never follows a signed URL. It checks:

  * the configured source root is a directory,
  * complete pagination with refresh:false (stable total, no duplicate names),
  * per-name exact Get for chosen samples,
  * stable public URL bytes match the local preserved media.

Run from the repository root:

    python3 artifacts/task-4.4/tools/openlist_readonly_probe.py \
        --base-url http://127.0.0.1:25244 --source-root /s3 \
        --media-dir artifacts/task-4.1/isolated-s3/media \
        --sample "Synthetic 001.mkv" --sample "中文电影.mkv" ...
"""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
import urllib.parse
import urllib.request

PER_PAGE = 200
TIMEOUT = 30


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
    entries = []
    page = 1
    total = None
    while True:
        status, body = post_json(base_url, "/api/fs/list", {
            "path": path, "page": page, "per_page": PER_PAGE, "refresh": False,
        })
        if status != 200 or body.get("code") != 200:
            raise SystemExit(f"list failed for {path}: http={status} code={body.get('code')}")
        data = body["data"]
        if total is None:
            total = data["total"]
        elif total != data["total"]:
            raise SystemExit(f"total changed during paging for {path}")
        content = data.get("content") or []
        if not content:
            break
        entries.extend(content)
        page += 1
        if len(entries) >= total:
            break
    names = [entry["name"] for entry in entries]
    if len(names) != len(set(names)):
        raise SystemExit(f"duplicate names while paging {path}")
    if total is not None and len(entries) != total:
        raise SystemExit(f"paged {len(entries)} != total {total} for {path}")
    return entries, total


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--source-root", required=True)
    parser.add_argument("--media-dir", required=True)
    parser.add_argument("--sample", action="append", default=[])
    parser.add_argument("--public-url", default=None,
                        help="public base URL for stable /d/ bytes; default --base-url")
    args = parser.parse_args(argv)

    from pathlib import Path
    media = Path(args.media_dir)
    report = {"source_root": args.source_root, "checks": {}}

    status, body = post_json(args.base_url, "/api/fs/get",
                             {"path": args.source_root, "refresh": False})
    root_ok = status == 200 and body.get("code") == 200 and body["data"].get("is_dir") is True
    report["checks"]["root_is_directory"] = root_ok
    if not root_ok:
        print(json.dumps(report, indent=2))
        return 2

    entries, total = list_directory(args.base_url, args.source_root)
    files = [e for e in entries if not e.get("is_dir")]
    dirs = [e for e in entries if e.get("is_dir")]
    report["checks"]["total"] = total
    report["checks"]["regular_files"] = len(files)
    report["checks"]["subdirectories"] = [d["name"] for d in dirs]

    public_base = args.public_url or args.base_url
    samples = []
    for name in args.sample:
        path = args.source_root.rstrip("/") + "/" + name
        status, body = post_json(args.base_url, "/api/fs/get", {"path": path, "refresh": False})
        sample = {"name": name, "get_http": status, "get_code": body.get("code")}
        if status == 200 and body.get("code") == 200:
            data = body["data"]
            sample["size"] = data.get("size")
            sample["is_dir"] = data.get("is_dir")
            sample["has_hash"] = bool(data.get("hash_info"))
        escaped = urllib.parse.quote(name, safe="")
        url = public_base.rstrip("/") + "/d" + args.source_root.rstrip("/") + "/" + escaped
        try:
            http, payload = get_bytes(url)
            sample["stable_url_http"] = http
            sample["stable_url_size"] = len(payload)
            local = media / name
            if local.is_file():
                sample["local_size"] = local.stat().st_size
                sample["sha256_match"] = (
                    hashlib.sha256(payload).hexdigest()
                    == hashlib.sha256(local.read_bytes()).hexdigest()
                )
        except Exception as error:  # noqa: BLE001 - report category only
            sample["stable_url_error"] = type(error).__name__
        samples.append(sample)
    report["checks"]["samples"] = samples
    print(json.dumps(report, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
