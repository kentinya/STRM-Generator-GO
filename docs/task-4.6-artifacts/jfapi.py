#!/usr/bin/env python3
"""Task 4.6 only: minimal authenticated Jellyfin native API helper.

The key is never read here. It arrives in the child process environment as
JELLYFIN_API_KEY from artifacts/task-4.6/auth/with_jellyfin_key.py, which also
sets JELLYFIN_BASE_URL. Nothing secret is printed, stored or written to argv.

Only public native endpoints of this instance are used; no plugin internal
interface is called.
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

AUTH_HEADER = (
    'MediaBrowser Client="STRM-isolated-recovery", Device="B-verification", '
    'DeviceId="slice4-task46-recovery", Version="1.0", Token="{key}"'
)


def base_url() -> str:
    return os.environ["JELLYFIN_BASE_URL"].rstrip("/")


def _headers() -> dict[str, str]:
    return {
        "Authorization": AUTH_HEADER.format(key=os.environ["JELLYFIN_API_KEY"]),
        "Accept": "application/json",
    }


def request(method: str, path: str, body=None, query=None, timeout: int = 60):
    url = base_url() + path
    if query:
        url += "?" + urllib.parse.urlencode(query, doseq=True)
    data = None
    headers = _headers()
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            raw = response.read()
            status = response.status
    except urllib.error.HTTPError as error:
        raw = error.read()
        status = error.code
    payload = None
    if raw:
        try:
            payload = json.loads(raw)
        except ValueError:
            payload = None
    return status, payload


def get(path: str, query=None, timeout: int = 60):
    return request("GET", path, query=query, timeout=timeout)


def post(path: str, body=None, query=None, timeout: int = 60):
    return request("POST", path, body=body, query=query, timeout=timeout)


def main() -> int:
    if len(sys.argv) < 3:
        print("usage: jfapi.py GET|POST <path> [json-body]", file=sys.stderr)
        return 2
    method = sys.argv[1].upper()
    path = sys.argv[2]
    body = json.loads(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3] else None
    status, payload = request(method, path, body=body)
    print(json.dumps({"status": status, "body": payload}, ensure_ascii=False))
    return 0 if 200 <= status < 300 else 1


if __name__ == "__main__":
    raise SystemExit(main())
