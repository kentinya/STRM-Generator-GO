"""Unit tests for the isolated target acceptance driver.

These tests exercise the driver's pure helpers (redaction, STRM parsing, tree digest)
without starting any container, OpenList, Jellyfin, or browser. The real target chain is
exercised separately by the acceptance run itself.
"""

import hashlib
import json
from pathlib import Path
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


if __name__ == "__main__":
    unittest.main()
