#!/usr/bin/env python3
"""Prepare the fixed Slice 4 media dataset without overwriting existing media.

Task 4.4 (Slice 4 RO-1/RO-2). This tool manages only the local fixture media
tree. It never uploads objects, never contacts OpenList/Jellyfin, and never
touches the generator product; the isolated S3 objects are reconciled
separately. The tool keeps existing media bytes and mtimes, only fills missing
planned paths from already-verified real source files, and rejects conflicts
instead of overwriting.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import sys
from pathlib import Path, PurePosixPath

TEMP_PREFIX = ".strmgen-tmp-"
NEW_PATH_TEMPLATE = "bulk/{index:06d}.mkv"
DEFAULT_MEDIA_DIR = Path(__file__).resolve().parent / "media"
MANIFEST_FIELDS = ("path", "size", "sha256", "sha1", "content_id", "origin")
CHUNK = 1 << 20


class ToolError(Exception):
    """A fixed-category tool failure that maps to a non-zero exit code."""

    def __init__(self, category, message):
        super().__init__(message)
        self.category = category


def _digest(path, algorithm):
    digest = hashlib.new(algorithm)
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(CHUNK), b""):
            digest.update(chunk)
    return digest.hexdigest()


def file_identity(path):
    stat = path.stat()
    return {
        "size": stat.st_size,
        "sha256": _digest(path, "sha256"),
        "sha1": _digest(path, "sha1"),
    }


def _safe_rel(rel):
    pure = PurePosixPath(rel)
    if pure.is_absolute() or not pure.parts or ".." in pure.parts:
        raise ToolError("manifest", f"unsafe relative path: {rel!r}")
    return pure


def _is_within(path, parent):
    """True when path resolves inside parent (used to keep manifests out)."""
    try:
        Path(path).resolve().relative_to(Path(parent).resolve())
        return True
    except ValueError:
        return False


def _assert_owned_path(media_dir, rel):
    """Reject links/type conflicts on the media root and every path ancestor.

    A static symlink below the media root must be rejected before any write, so
    candidate reuse cannot publish one object through the link and only fail on
    a later enumeration. Concurrent external replacement is out of scope.
    """
    pure = _safe_rel(rel)
    if media_dir.is_symlink() or not media_dir.is_dir():
        raise ToolError("input", "media root is not a regular directory")
    current = media_dir
    for part in pure.parts[:-1]:
        current = current / part
        if current.is_symlink():
            raise ToolError("conflict", f"symbolic link in path: {rel}")
        if current.exists() and not current.is_dir():
            raise ToolError("conflict", f"non-directory in path: {rel}")
    target = media_dir / pure
    if target.is_symlink():
        raise ToolError("conflict", f"symbolic link is not allowed: {rel}")
    if target.exists() and not target.is_file():
        raise ToolError("conflict", f"non-regular file is not allowed: {rel}")
    return target


def iter_media(media_dir):
    """Return sorted relative paths of regular media, rejecting links/oddities."""
    if not media_dir.exists():
        return []
    if media_dir.is_symlink() or not media_dir.is_dir():
        raise ToolError("input", "media root is not a regular directory")
    found = []
    for root, dirs, files in os.walk(media_dir, followlinks=False):
        root_path = Path(root)
        for name in list(dirs):
            candidate = root_path / name
            if candidate.is_symlink():
                raise ToolError(
                    "conflict",
                    "symbolic link directory is not allowed: "
                    + candidate.relative_to(media_dir).as_posix(),
                )
        for name in files:
            candidate = root_path / name
            rel = candidate.relative_to(media_dir).as_posix()
            if name.startswith(TEMP_PREFIX):
                # Never delete a temp file we cannot prove this run created; an
                # unknown object is a conflict to keep, not to sweep away.
                raise ToolError("conflict", f"unowned temporary file present: {rel}")
            if candidate.is_symlink():
                raise ToolError("conflict", f"symbolic link is not allowed: {rel}")
            if not candidate.is_file():
                raise ToolError("conflict", f"non-regular file is not allowed: {rel}")
            found.append(rel)
    found.sort()
    return found


def load_source(path, label):
    if not path.exists() or path.is_symlink() or not path.is_file():
        raise ToolError("input", f"{label} is not a regular file: {path.name}")
    return path


def _content_of(ident, identity):
    if ident["sha256"] == identity["A"]["sha256"]:
        return "A"
    if "B" in identity and ident["sha256"] == identity["B"]["sha256"]:
        return "B"
    return "preserved"


def build_plan(media_dir, count, source, alternate):
    existing = iter_media(media_dir)
    if len(existing) > count:
        raise ToolError("count", f"existing media {len(existing)} exceeds target {count}")
    identity = {"A": file_identity(source)}
    sources = {"A": source}
    if alternate is not None:
        if file_identity(alternate)["sha256"] == identity["A"]["sha256"]:
            raise ToolError("input", "alternate source must differ from primary source")
        identity["B"] = file_identity(alternate)
        sources["B"] = alternate
    needed = count - len(existing)
    existing_set = set(existing)
    planned = []
    for index in range(1, needed + 1):
        rel = NEW_PATH_TEMPLATE.format(index=index)
        if rel in existing_set:
            raise ToolError("conflict", f"planned path already exists: {rel}")
        planned.append(rel)
    records = []
    for rel in existing:
        ident = file_identity(media_dir / rel)
        records.append({
            "path": rel,
            "size": ident["size"],
            "sha256": ident["sha256"],
            "sha1": ident["sha1"],
            "content_id": _content_of(ident, identity),
            "origin": "preserved",
        })
    for index, rel in enumerate(planned, start=1):
        content = "B" if (index == 1 and "B" in identity) else "A"
        ident = identity[content]
        records.append({
            "path": rel,
            "size": ident["size"],
            "sha256": ident["sha256"],
            "sha1": ident["sha1"],
            "content_id": content,
            "origin": "added",
        })
    records.sort(key=lambda record: record["path"])
    return records, sources


def load_manifest(path):
    if not path.exists():
        return None
    records = []
    seen = set()
    for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as error:
            raise ToolError("manifest", f"invalid JSONL at line {lineno}") from error
        for key in MANIFEST_FIELDS:
            if key not in record:
                raise ToolError("manifest", f"missing field {key} at line {lineno}")
        _safe_rel(record["path"])
        if record["path"] in seen:
            raise ToolError("manifest", f"duplicate path: {record['path']}")
        seen.add(record["path"])
        records.append(record)
    return records


def write_manifest(path, records):
    path.parent.mkdir(parents=True, exist_ok=True)
    body = "".join(
        json.dumps(record, ensure_ascii=False, separators=(",", ":"), sort_keys=True) + "\n"
        for record in records
    )
    tmp = path.with_name(TEMP_PREFIX + path.name)
    tmp.write_text(body, encoding="utf-8")
    os.replace(tmp, path)


def _verify_target(media_dir, path, record):
    _assert_owned_path(media_dir, record["path"])
    if not path.exists():
        raise ToolError("missing", f"missing media: {record['path']}")
    if path.is_symlink() or not path.is_file():
        raise ToolError("conflict", f"non-regular media: {record['path']}")
    if path.stat().st_size != record["size"]:
        raise ToolError("conflict", f"size mismatch: {record['path']}")
    if _digest(path, "sha256") != record["sha256"]:
        raise ToolError("conflict", f"content mismatch: {record['path']}")


def _publish(source, target, media_dir, rel):
    _assert_owned_path(media_dir, rel)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.parent / (TEMP_PREFIX + str(os.getpid()) + "-" + target.name)
    try:
        with open(source, "rb") as reader, open(tmp, "wb") as writer:
            shutil.copyfileobj(reader, writer)
            writer.flush()
            os.fsync(writer.fileno())
        os.link(tmp, target)
    finally:
        if tmp.exists():
            tmp.unlink()


def _reconcile_present(media_dir, records):
    present = set(iter_media(media_dir))
    planned = {record["path"] for record in records}
    extra = sorted(present - planned)
    if extra:
        raise ToolError("conflict", f"unplanned media present: {extra[0]}")
    if len(present) != len(records):
        raise ToolError("count", f"media count {len(present)} != planned {len(records)}")


def prepare(media_dir, records, sources):
    if media_dir.is_symlink():
        raise ToolError("input", "media root is a symbolic link")
    media_dir.mkdir(parents=True, exist_ok=True)
    # Validate every planned target and ancestor before any write, so a static
    # symlink/type conflict cannot be traversed for an earlier object first.
    for record in records:
        _assert_owned_path(media_dir, record["path"])
    created = 0
    for record in records:
        target = media_dir / _safe_rel(record["path"])
        if target.exists():
            _verify_target(media_dir, target, record)
            continue
        content = record["content_id"]
        if content not in sources:
            raise ToolError("missing", f"cannot reproduce media: {record['path']}")
        try:
            _publish(sources[content], target, media_dir, record["path"])
            created += 1
        except FileExistsError:
            _verify_target(media_dir, target, record)
    _reconcile_present(media_dir, records)
    for record in records:
        _verify_target(media_dir, media_dir / _safe_rel(record["path"]), record)
    return created


def verify_only(media_dir, manifest):
    records = load_manifest(manifest)
    if records is None:
        raise ToolError("manifest", "final manifest does not exist")
    for record in records:
        _assert_owned_path(media_dir, record["path"])
    for record in records:
        _verify_target(media_dir, media_dir / _safe_rel(record["path"]), record)
    _reconcile_present(media_dir, records)
    return len(records)


def reuse_candidate(records, source, alternate):
    sources = {"A": source}
    expected = {"A": file_identity(source)["sha256"]}
    if alternate is not None:
        sources["B"] = alternate
        expected["B"] = file_identity(alternate)["sha256"]
    for record in records:
        content = record["content_id"]
        if content in expected and record["sha256"] != expected[content]:
            raise ToolError("conflict", f"source content changed for {record['path']}")
    return records, sources


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", help="verified primary (A) source media")
    parser.add_argument("--alternate-source", help="verified alternate (B) source media")
    parser.add_argument("--media-dir", default=str(DEFAULT_MEDIA_DIR), help="owned media root")
    parser.add_argument("--count", type=int, help="total media target (real run: 10000)")
    parser.add_argument("--manifest", help="final manifest path outside the media root")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--prepare", action="store_true", help="fill missing planned media")
    group.add_argument("--verify-only", action="store_true", help="verify without writing")
    return parser


def main(argv=None):
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        if args.manifest is None:
            raise ToolError("usage", "--manifest is required")
        media_dir = Path(args.media_dir)
        manifest = Path(args.manifest)
        if _is_within(manifest, media_dir):
            raise ToolError("usage", "manifest must not be inside the media root")
        candidate = manifest.with_name(manifest.stem + ".candidate" + manifest.suffix)
        if args.verify_only:
            total = verify_only(media_dir, manifest)
            print(f"verified {total} media against {manifest.name}")
            return 0
        if args.count is None:
            raise ToolError("usage", "--count is required")
        if args.count < 0:
            raise ToolError("usage", "--count must be non-negative")
        if args.source is None:
            raise ToolError("usage", "--source is required")
        source = load_source(Path(args.source), "source")
        alternate = None
        if args.alternate_source is not None:
            alternate = load_source(Path(args.alternate_source), "alternate source")
        records = load_manifest(candidate)
        if records is not None:
            if len(records) != args.count:
                raise ToolError(
                    "count",
                    f"candidate has {len(records)} media but --count is {args.count}",
                )
            records, sources = reuse_candidate(records, source, alternate)
        else:
            records, sources = build_plan(media_dir, args.count, source, alternate)
        preserved = sum(1 for record in records if record["origin"] == "preserved")
        added = len(records) - preserved
        if not args.prepare:
            print(
                f"plan {len(records)} media (preserved {preserved}, added {added}); "
                f"candidate {'present' if candidate.exists() else 'absent'}"
            )
            return 0
        if manifest.exists():
            raise ToolError("manifest", "final manifest already exists; use --verify-only")
        if not candidate.exists():
            write_manifest(candidate, records)
        created = prepare(media_dir, records, sources)
        print(
            f"prepared {len(records)} media (preserved {preserved}, planned-new {added}, "
            f"created {created})"
        )
        return 0
    except ToolError as error:
        print(f"error: {error.category}: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
