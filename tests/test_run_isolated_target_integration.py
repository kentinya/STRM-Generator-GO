"""Unit tests for the isolated target acceptance driver.

These tests exercise the driver's pure helpers (redaction, STRM parsing, tree digest)
without starting any container, OpenList, Jellyfin, or browser. The real target chain is
exercised separately by the acceptance run itself.
"""

import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import run_isolated_target_integration as driver


class RedactionTests(unittest.TestCase):
    def test_redacts_urls(self):
        line = "fetched http://openlist:5244/d/s3/Plain%20Movie.mkv?sign=abc123 done"
        self.assertNotIn("sign=abc123", driver.redact(line))
        self.assertIn("<url>", driver.redact(line))

    def test_redacts_secret_pairs(self):
        line = "token=supersecret password=hunter2"
        redacted = driver.redact(line)
        self.assertNotIn("supersecret", redacted)
        self.assertNotIn("hunter2", redacted)
        self.assertIn("<redacted>", redacted)

    def test_preserves_plain_text(self):
        self.assertEqual(driver.redact("plain message"), "plain message")


class StrmParsingTests(unittest.TestCase):
    def test_reads_version_size_and_url(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "Plain Movie.strm"
            path.write_text(
                "# strm-probe-version:1\n"
                "# strm-probe-size:24404\n"
                "http://openlist:5244/d/s3/Plain%20Movie.mkv\n",
                encoding="utf-8",
            )
            parsed = driver.read_strm(path)
            self.assertEqual(parsed["version"], "1")
            self.assertEqual(parsed["size"], 24404)
            self.assertEqual(parsed["url"], "http://openlist:5244/d/s3/Plain%20Movie.mkv")

    def test_unsigned_url_has_no_query(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.strm"
            path.write_text("# strm-probe-version:1\nhttp://openlist:5244/d/s3/x.mkv\n", encoding="utf-8")
            self.assertNotIn("?", driver.read_strm(path)["url"])


class TreeDigestTests(unittest.TestCase):
    def test_digest_changes_with_content(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "a.strm").write_text("one", encoding="utf-8")
            first = driver.digest_tree(root)
            (root / "a.strm").write_text("two", encoding="utf-8")
            self.assertNotEqual(first, driver.digest_tree(root))

    def test_digest_stable_for_identical_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "a.strm").write_text("one", encoding="utf-8")
            (root / "b.strm").write_text("two", encoding="utf-8")
            self.assertEqual(driver.digest_tree(root), driver.digest_tree(root))

    def test_digest_matches_manual_sha256(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "a.strm").write_bytes(b"payload")
            expected = hashlib.sha256()
            expected.update(b"a.strm")
            expected.update(b"\0")
            expected.update(hashlib.sha256(b"payload").digest())
            self.assertEqual(driver.digest_tree(root), expected.hexdigest())


class FailureTests(unittest.TestCase):
    def test_require_raises_failure(self):
        with self.assertRaises(driver.Failure):
            driver.require(False, "boom")
        driver.require(True, "fine")


class PlaybackScenarioExchangeTests(unittest.TestCase):
    """Regression: the playback scenario carries the Jellyfin password and must never touch disk."""

    def _run(self, scenario):
        captured = {}

        def fake_run(args, **kwargs):
            captured["args"] = args
            captured["input"] = kwargs.get("input")
            return subprocess.CompletedProcess(args, 0, stdout=json.dumps({"results": [{"advanced": True}]}), stderr="")

        original_run = driver.subprocess.run
        original_write_text = Path.write_text

        def forbidden_write_text(self, *args, **kwargs):
            raise AssertionError(f"scenario must not be persisted: write_text called on {self}")

        driver.subprocess.run = fake_run
        Path.write_text = forbidden_write_text
        try:
            result = driver.jellyfin_playback(
                "http://127.0.0.1:28096", Path("/driver.mjs"), "/playwright", scenario, "/browsers")
        finally:
            driver.subprocess.run = original_run
            Path.write_text = original_write_text
        return captured, result

    def test_scenario_passed_via_stdin_not_a_file(self):
        captured, result = self._run({"username": "admin", "password": "s3cr3t-value", "items": []})
        self.assertEqual(captured["args"][0], "node")
        self.assertEqual(captured["args"][2], "/dev/stdin")
        self.assertIn("s3cr3t-value", captured["input"])
        self.assertEqual(json.loads(captured["input"])["password"], "s3cr3t-value")
        self.assertTrue(result["results"][0]["advanced"])

    def test_no_tmp_scenario_file_is_created(self):
        before = set(Path("/tmp").glob("strmgen-play-*.json"))
        self._run({"username": "admin", "password": "another-secret", "items": []})
        after = set(Path("/tmp").glob("strmgen-play-*.json"))
        self.assertEqual(before, after, "jellyfin_playback left a scenario file behind")

    def test_driver_error_is_redacted(self):
        def fake_run(args, **kwargs):
            return subprocess.CompletedProcess(args, 1, stdout="", stderr="failed http://x/y?token=abc123")

        original_run = driver.subprocess.run
        driver.subprocess.run = fake_run
        try:
            result = driver.jellyfin_playback(
                "http://127.0.0.1:28096", Path("/driver.mjs"), "/playwright", {}, "/browsers")
        finally:
            driver.subprocess.run = original_run
        self.assertIn("error", result)
        self.assertNotIn("abc123", result["error"])


if __name__ == "__main__":
    unittest.main()
