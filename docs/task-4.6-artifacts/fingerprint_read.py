#!/usr/bin/env python3
"""Task 4.6 only: read the fixed plugin's fingerprint row for one item.

This is a read-only inspection of the plugin's own SQLite fingerprint store.
It never writes to it. It exists only to record which fingerprint the plugin
actually held for the compared item before and after each matrix row, so the
reported reason can be checked against the real previous values instead of
being inferred.
"""
from __future__ import annotations

import json
import sqlite3
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
STORE = (REPO_ROOT / "artifacts" / "task-4.1" / "isolated-s3" / "jellyfin-config" / "data" /
         "plugins" / "b061643ed17d4c6e8ba26b57e286bd20" / "strm-fingerprints.db")


def read(item_id: str) -> dict | None:
    if not STORE.exists():
        return None
    connection = sqlite3.connect(f"file:{STORE}?mode=ro", uri=True)
    try:
        row = connection.execute(
            "select item_id, source_key, url_hash, sha1, file_size "
            "from strm_fingerprint where item_id = ? and source_key = ?",
            (item_id, item_id),
        ).fetchone()
    finally:
        connection.close()
    if row is None:
        return None
    return {"item_id": row[0], "source_key": row[1], "url_hash": row[2],
            "sha1": row[3], "file_size": row[4]}


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: fingerprint_read.py <item-id>", file=sys.stderr)
        return 2
    print(json.dumps(read(sys.argv[1].replace("-", "")), ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
