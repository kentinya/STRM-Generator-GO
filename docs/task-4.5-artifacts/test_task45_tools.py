#!/usr/bin/env python3
"""Direct tests for the Task 4.5 verification helpers.

These are verification-side helpers, not product code. They assert the parsing
and reporting logic that AC-1 evidence depends on, using tiny fixtures only.
Run from the repository root:

    python3 -m unittest artifacts/task-4.5/tests/test_task45_tools.py -v
"""
from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
TOOLS = ROOT / "artifacts" / "task-4.5" / "tools"


def load(name: str):
    spec = importlib.util.spec_from_file_location(name, TOOLS / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


verify = load("verify_strm_tree")
measure = load("measure_run")


class VerifyParseTests(unittest.TestCase):
    def test_parses_valid_document(self):
        doc = verify.parse(
            b"# strm-probe-version:1\n# strm-probe-size:5\nhttp://h/d/s3/a.mkv\n"
        )
        self.assertEqual(doc["version"], "1")
        self.assertEqual(doc["size"], "5")
        self.assertEqual(doc["url"], "http://h/d/s3/a.mkv")

    def test_rejects_non_utf8(self):
        with self.assertRaises(ValueError):
            verify.parse(b"\xff\xfe# strm-probe-version:1\n")

    def test_rejects_oversize(self):
        with self.assertRaises(ValueError):
            verify.parse(b"x" * (verify.MAX_BYTES + 1))

    def test_rejects_duplicate_url(self):
        with self.assertRaises(ValueError):
            verify.parse(b"# strm-probe-version:1\nhttp://h/a\nhttp://h/b\n")


class VerifyTreeTests(unittest.TestCase):
    def _fixture(self, tmp, url):
        out = Path(tmp) / "out"
        out.mkdir()
        (out / "Movie.strm").write_text(
            f"# strm-probe-version:1\n# strm-probe-size:10\n{url}\n", encoding="utf-8"
        )
        manifest = Path(tmp) / "manifest.jsonl"
        manifest.write_text(
            json.dumps({"path": "Movie.mkv", "size": 10, "sha1": "a" * 40}) + "\n",
            encoding="utf-8",
        )
        return manifest, out

    def test_pass_on_matching_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest, out = self._fixture(tmp, "http://h/d/s3/Movie.mkv")
            result = verify.main([
                "--manifest", str(manifest), "--output", str(out),
                "--public-url", "http://h", "--source-root", "/s3",
            ])
            self.assertEqual(result, 0)

    def test_fail_on_url_mismatch(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest, out = self._fixture(tmp, "http://h/d/s3/Wrong.mkv")
            result = verify.main([
                "--manifest", str(manifest), "--output", str(out),
                "--public-url", "http://h", "--source-root", "/s3",
            ])
            self.assertEqual(result, 2)


class MeasureTests(unittest.TestCase):
    def test_reports_exit_code_and_rss(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = Path(tmp) / "r.json"
            code = measure.main([
                "--label", "unit", "--output", str(result),
                "--", sys.executable, "-c", "import sys; sys.exit(0)",
            ])
            self.assertEqual(code, 0)
            payload = json.loads(result.read_text())
            self.assertEqual(payload["exit_code"], 0)
            self.assertGreater(payload["maxrss_kib"], 0)
            self.assertIn("KiB", payload["maxrss_unit"])

    def test_propagates_nonzero_exit(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = Path(tmp) / "r.json"
            code = measure.main([
                "--label", "unit", "--output", str(result),
                "--", sys.executable, "-c", "import sys; sys.exit(3)",
            ])
            self.assertEqual(code, 3)


if __name__ == "__main__":
    unittest.main()
