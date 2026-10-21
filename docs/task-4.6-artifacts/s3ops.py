#!/usr/bin/env python3
"""Task 4.6 only: minimal S3 (RustFS) sample access for the fixed dataset.

The RustFS keys are read from the already-verified dedicated container Env and
are used only in this child process environment. They are never printed,
logged, written to a file or placed in argv. Only the least-privileged S3
operations needed by the authorized scenario are used:

  * list   -- recursive listing of the fixed bucket prefix
  * cat    -- read one confirmed own object
  * copyto -- replace the content of one confirmed own object (same key)

Only objects that belong to the fixed Task 4.4 manifest may be touched.
"""
from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
RUSTFS_CONTAINER = "d36dd78342a0"
REMOTE = "b45:slice4-media"
MANIFEST = REPO_ROOT / "artifacts" / "task-4.4" / "dataset" / "manifest.jsonl"


def container_keys() -> dict[str, str]:
    """Read only the two necessary keys from the verified container Env."""
    names = ["RUSTFS_ACCESS_KEY", "RUSTFS_SECRET_KEY"]
    terms = " ".join('(eq (index (split . "=") 0) ' + json.dumps(n) + ")" for n in names)
    test = "(or " + terms + ")"
    fmt = "{{range .Config.Env}}{{if " + test + "}}{{println .}}{{end}}{{end}}"
    proc = subprocess.run(
        ["docker", "inspect", "--format", fmt, RUSTFS_CONTAINER],
        capture_output=True, text=True, timeout=15, check=True,
    )
    found = dict(line.split("=", 1) for line in proc.stdout.splitlines() if line)
    if set(found) != set(names) or not all(found.values()):
        raise RuntimeError("necessary S3 keys unavailable")
    return found


def s3_env() -> dict[str, str]:
    keys = container_keys()
    return dict(
        os.environ,
        RCLONE_CONFIG_B45_TYPE="s3",
        RCLONE_CONFIG_B45_PROVIDER="Other",
        RCLONE_CONFIG_B45_ENDPOINT="http://127.0.0.1:29000",
        RCLONE_CONFIG_B45_REGION="us-east-1",
        RCLONE_CONFIG_B45_ACCESS_KEY_ID=keys["RUSTFS_ACCESS_KEY"],
        RCLONE_CONFIG_B45_SECRET_ACCESS_KEY=keys["RUSTFS_SECRET_KEY"],
        RCLONE_CONFIG_B45_FORCE_PATH_STYLE="true",
    )


def s3(*args: str, timeout: int = 300) -> bytes:
    proc = subprocess.run(
        ["rclone", "--config", "/dev/null", "--retries", "1", "--low-level-retries", "1", *args],
        env=s3_env(), capture_output=True, timeout=timeout,
    )
    if proc.returncode != 0:
        raise RuntimeError("rclone failed: " + proc.stderr.decode("utf-8", "replace")[:200])
    return proc.stdout


def manifest_paths() -> set[str]:
    return {json.loads(line)["path"] for line in MANIFEST.read_text().splitlines()}


def list_objects() -> list[dict]:
    raw = s3("lsjson", "--recursive", REMOTE)
    return [entry for entry in json.loads(raw) if not entry.get("IsDir")]


def read_object(name: str) -> bytes:
    if name not in manifest_paths():
        raise RuntimeError("refusing to read an object outside the fixed manifest")
    return s3("cat", f"{REMOTE}/{name}")


def write_object(name: str, source: Path, set_mtime: str | None = None) -> dict:
    """Replace the content of one confirmed own object, keeping its path.

    ``set_mtime`` asks rclone to publish an explicit object modification time
    instead of inheriting the source file time, so a source-side modification
    time change can be produced without changing a single byte.
    """
    if name not in manifest_paths():
        raise RuntimeError("refusing to write an object outside the fixed manifest")
    args = ["copyto", str(source), f"{REMOTE}/{name}"]
    if set_mtime is not None:
        args = ["--metadata-set", f"mtime={set_mtime}", *args]
    s3(*args)
    data = read_object(name)
    return {
        "path": name,
        "size": len(data),
        "sha1": hashlib.sha1(data).hexdigest(),
        "sha256": hashlib.sha256(data).hexdigest(),
    }


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: s3ops.py list|cat <name>|put <name> <local-file>", file=sys.stderr)
        return 2
    command = sys.argv[1]
    if command == "list":
        entries = list_objects()
        print(json.dumps({"count": len(entries), "paths": sorted(e["Path"] for e in entries)},
                         ensure_ascii=False))
    elif command == "cat":
        data = read_object(sys.argv[2])
        print(json.dumps({"name": sys.argv[2], "size": len(data),
                          "sha1": hashlib.sha1(data).hexdigest(),
                          "sha256": hashlib.sha256(data).hexdigest()}, ensure_ascii=False))
    elif command == "lsjson":
        entries = [entry for entry in json.loads(s3("lsjson", f"{REMOTE}/{sys.argv[2]}"))
                   if not entry.get("IsDir")]
        print(json.dumps(entries, ensure_ascii=False))
    elif command == "put":
        set_mtime = sys.argv[4] if len(sys.argv) > 4 else None
        result = write_object(sys.argv[2], Path(sys.argv[3]), set_mtime)
        print(json.dumps(result, ensure_ascii=False))
    else:
        print("unknown command", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
