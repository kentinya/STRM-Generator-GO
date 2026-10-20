#!/usr/bin/env python3
"""Direct behavior tests for the Task 4.4 local dataset tool.

These tests use only temporary directories under the task-owned artifacts tree
with synthetic A/B bytes. They verify tool logic (fill-missing, conflict
rejection, no-overwrite, deterministic reruns) and are NOT real media or
OpenList/S3 evidence. Run explicitly:

    python3 -m unittest discover -s artifacts/task-4.4/tests -p 'test_*.py' -v
"""
from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import os
import shutil
import sys
import tempfile
import unittest
from contextlib import redirect_stdout, redirect_stderr
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
TOOL = REPO_ROOT / "artifacts" / "task-4.1" / "isolated-s3" / "generate_media.py"
# Keep every temporary fixture under the task-owned artifacts tree; never use
# the real media root or the host /tmp for these logic tests.
TMP_ROOT = REPO_ROOT / "artifacts" / "task-4.4" / "tests" / "tmp"

_spec = importlib.util.spec_from_file_location("task44_generate_media", TOOL)
gm = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gm)

A_BYTES = b"task44-content-A-" + b"a" * 100
B_BYTES = b"task44-content-B-" + b"b" * 130


def sha256(data):
    return hashlib.sha256(data).hexdigest()


class ToolTestCase(unittest.TestCase):
    def setUp(self):
        TMP_ROOT.mkdir(parents=True, exist_ok=True)
        self._tmp = tempfile.TemporaryDirectory(dir=TMP_ROOT)
        self.base = Path(self._tmp.name)
        self.media = self.base / "media"
        self.media.mkdir()
        self.manifest = self.base / "dataset" / "manifest.jsonl"
        self.source = self.base / "inputs" / "A.mkv"
        self.alternate = self.base / "inputs" / "B.mkv"
        self.source.parent.mkdir()
        self.source.write_bytes(A_BYTES)
        self.alternate.write_bytes(B_BYTES)

    def tearDown(self):
        self._tmp.cleanup()
        try:
            TMP_ROOT.rmdir()
        except OSError:
            pass

    def run_tool(self, *args):
        out, err = io.StringIO(), io.StringIO()
        with redirect_stdout(out), redirect_stderr(err):
            code = gm.main(list(args))
        return code, out.getvalue(), err.getvalue()

    def args(self, count=8, *extra):
        return (
            "--source", str(self.source),
            "--alternate-source", str(self.alternate),
            "--media-dir", str(self.media),
            "--count", str(count),
            "--manifest", str(self.manifest),
            *extra,
        )

    def snapshot(self):
        result = {}
        for root, _dirs, files in os.walk(self.media):
            for name in files:
                path = Path(root) / name
                stat = path.stat()
                result[path.relative_to(self.media).as_posix()] = (
                    hashlib.sha256(path.read_bytes()).hexdigest(), stat.st_mtime_ns,
                )
        return result


class TestPlanAndFill(ToolTestCase):
    def test_default_preview_writes_nothing(self):
        code, out, _ = self.run_tool(*self.args())
        self.assertEqual(code, 0)
        self.assertIn("plan 8 media", out)
        self.assertEqual(self.snapshot(), {})
        self.assertFalse(self.manifest.exists())

    def test_help_exits_zero_without_writes(self):
        with self.assertRaises(SystemExit) as caught:
            with redirect_stdout(io.StringIO()):
                gm.build_parser().parse_args(["--help"])
        self.assertEqual(caught.exception.code, 0)
        self.assertEqual(self.snapshot(), {})

    def test_first_fill_preserves_existing_and_includes_b(self):
        (self.media / "keep-one.mkv").write_bytes(A_BYTES)
        (self.media / "keep-two.mkv").write_bytes(A_BYTES)
        before = self.snapshot()
        code, out, err = self.run_tool(*self.args(8, "--prepare"))
        self.assertEqual(code, 0, err)
        self.assertIn("preserved 2", out)
        self.assertEqual(len(self.snapshot()), 8)
        # The final manifest is intentionally fixed only after S3 reconciliation;
        # --prepare leaves the candidate manifest, not manifest.jsonl.
        self.assertFalse(self.manifest.exists())
        candidate = self.base / "dataset" / "manifest.candidate.jsonl"
        self.assertTrue(candidate.exists())
        records = [json.loads(line) for line in candidate.read_text().splitlines()]
        self.assertEqual(len(records), 8)
        b_records = [r for r in records if r["content_id"] == "B"]
        self.assertEqual(len(b_records), 1)
        for rel, (digest, mtime) in before.items():
            self.assertEqual(self.snapshot()[rel], (digest, mtime))
        for record in records:
            data = (self.media / record["path"]).read_bytes()
            expected = B_BYTES if record["content_id"] == "B" else A_BYTES
            self.assertEqual(data, expected)
            self.assertEqual(record["sha256"], sha256(expected))

    def test_rerun_and_verify_only_are_zero_write(self):
        self.run_tool(*self.args(8, "--prepare"))
        candidate = self.base / "dataset" / "manifest.candidate.jsonl"
        candidate_bytes = candidate.read_bytes()
        first = self.snapshot()
        code, out, _ = self.run_tool(*self.args(8, "--prepare"))
        self.assertEqual(code, 0)
        self.assertIn("created 0", out)
        self.assertEqual(self.snapshot(), first)
        self.assertEqual(candidate.read_bytes(), candidate_bytes)
        # Fixing the final manifest is a later, separate step; simulate it here to
        # exercise --verify-only against the frozen bytes.
        self.manifest.parent.mkdir(parents=True, exist_ok=True)
        self.manifest.write_bytes(candidate_bytes)
        code, out, _ = self.run_tool("--verify-only", "--media-dir", str(self.media),
                                     "--manifest", str(self.manifest))
        self.assertEqual(code, 0)
        self.assertIn("verified 8", out)
        self.assertEqual(self.snapshot(), first)

    def test_candidate_reused_after_partial_fill(self):
        # Simulate an interrupted first run: only the candidate manifest and one
        # correctly-published file exist.
        self.run_tool(*self.args(8))
        records, sources = gm.build_plan(self.media, 8, self.source, self.alternate)
        candidate = self.base / "dataset" / "manifest.candidate.jsonl"
        gm.write_manifest(candidate, records)
        first = records[0]
        gm._publish(sources[first["content_id"]], self.media / first["path"])
        plan_before = candidate.read_bytes()
        code, out, err = self.run_tool(*self.args(8, "--prepare"))
        self.assertEqual(code, 0, err)
        self.assertEqual(candidate.read_bytes(), plan_before)
        self.assertEqual(len(self.snapshot()), 8)


class TestRejection(ToolTestCase):
    def test_missing_source_rejected_before_write(self):
        code, _out, err = self.run_tool(
            "--source", str(self.base / "nope.mkv"),
            "--media-dir", str(self.media), "--count", "8",
            "--manifest", str(self.manifest), "--prepare",
        )
        self.assertNotEqual(code, 0)
        self.assertIn("input", err)
        self.assertEqual(self.snapshot(), {})

    def test_count_below_existing_rejected(self):
        for index in range(3):
            (self.media / f"existing-{index}.mkv").write_bytes(A_BYTES)
        before = self.snapshot()
        code, _out, err = self.run_tool(*self.args(2, "--prepare"))
        self.assertNotEqual(code, 0)
        self.assertIn("count", err)
        self.assertEqual(self.snapshot(), before)

    def test_existing_target_conflict_same_size_different_content(self):
        self.run_tool(*self.args(8))
        records, _sources = gm.build_plan(self.media, 8, self.source, self.alternate)
        target = self.media / records[0]["path"]
        target.parent.mkdir(parents=True, exist_ok=True)
        conflicting = b"X" * records[0]["size"]  # same size, different bytes
        target.write_bytes(conflicting)
        before = target.read_bytes()
        code, _out, err = self.run_tool(*self.args(8, "--prepare"))
        self.assertNotEqual(code, 0)
        self.assertIn("conflict", err)
        self.assertEqual(target.read_bytes(), before)

    def test_symbolic_link_rejected(self):
        (self.media / "link.mkv").symlink_to(self.source)
        code, _out, err = self.run_tool(*self.args(8, "--prepare"))
        self.assertNotEqual(code, 0)
        self.assertIn("conflict", err)
        self.assertTrue((self.media / "link.mkv").is_symlink())

    def test_unplanned_media_rejected(self):
        self.run_tool(*self.args(8, "--prepare"))
        (self.media / "surprise.mkv").write_bytes(A_BYTES)
        code, _out, err = self.run_tool(*self.args(8, "--prepare"))
        self.assertNotEqual(code, 0)
        self.assertIn("conflict", err)

    def test_manifest_inside_media_root_is_not_required_but_unsafe_path_rejected(self):
        code, _out, err = self.run_tool(
            "--source", str(self.source), "--media-dir", str(self.media),
            "--count", "2", "--manifest", str(self.manifest), "--prepare",
        )
        self.assertEqual(code, 0, err)
        # A candidate manifest with an escaping path must be rejected on reuse.
        candidate = self.base / "dataset" / "manifest.candidate.jsonl"
        candidate.write_text(
            json.dumps({"path": "../escape.mkv", "size": 1, "sha256": "0" * 64,
                        "sha1": "0" * 40, "content_id": "A", "origin": "added"}) + "\n"
        )
        code, _out, err = self.run_tool(*self.args(2, "--prepare"))
        self.assertNotEqual(code, 0)
        self.assertIn("manifest", err)


if __name__ == "__main__":
    unittest.main()
