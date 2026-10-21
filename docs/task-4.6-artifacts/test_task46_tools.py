#!/usr/bin/env python3
"""Direct behaviour tests for the Task 4.6 helper tools.

These tests stub only the external I/O of the helper tools. They verify the
readable mirror of the locked plugin's decision rules, the manifest parser and
the log-window decision extraction. They are NOT real Jellyfin or S3 evidence.

    python3 -m unittest discover -s artifacts/task-4.6/tests -p 'test_task46*.py' -v
"""
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
TOOLS = REPO_ROOT / "artifacts" / "task-4.6" / "tools"


def _load(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, TOOLS / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


model = _load("task46_decision_model", "decision_model.py")
scenario = _load("task46_scenario", "scenario.py")


class DecisionModelTests(unittest.TestCase):
    """The mirror must reproduce the locked ProbeDecisionService branches."""

    def manifest(self, url, sha1, size):
        return model.parse_manifest(
            "# strm-probe-version:1\n"
            + (f"# strm-probe-sha1:{sha1}\n" if sha1 else "")
            + (f"# strm-probe-size:{size}\n" if size is not None else "")
            + f"{url}\n"
        )

    def test_missing_primary_stream_wins(self):
        manifest = self.manifest("http://x/a.mkv", "a" * 40, 10)
        kind, reason = model.decide(manifest, None, False, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("RequireProbe", "MissingPrimaryStream"))

    def test_native_url_mismatch_wins_over_first_trust(self):
        manifest = self.manifest("http://x/a.mkv", "a" * 40, 10)
        kind, reason = model.decide(manifest, None, True, "http://x/b.mkv")
        self.assertEqual((kind, reason), ("RequireProbe", "NativeUrlMismatch"))

    def test_first_trust_when_no_previous(self):
        manifest = self.manifest("http://x/a.mkv", "a" * 40, 10)
        kind, reason = model.decide(manifest, None, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("TrustExisting", "FirstTrust"))

    def test_url_changed(self):
        manifest = self.manifest("http://x/b.mkv", "a" * 40, 10)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/b.mkv")
        self.assertEqual((kind, reason), ("RequireProbe", "UrlChanged"))

    def test_sha1_matches_ignores_a_different_size(self):
        manifest = self.manifest("http://x/a.mkv", "a" * 40, 999)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("SkipProbe", "Sha1Matches"))

    def test_sha1_changed(self):
        manifest = self.manifest("http://x/a.mkv", "b" * 40, 10)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("RequireProbe", "Sha1Changed"))

    def test_size_changed_when_the_current_sha1_is_missing(self):
        manifest = self.manifest("http://x/a.mkv", None, 20)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("RequireProbe", "SizeChanged"))

    def test_unchanged_when_both_sizes_match_and_the_sha1_is_missing(self):
        manifest = self.manifest("http://x/a.mkv", None, 10)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("SkipProbe", "Unchanged"))

    def test_insufficient_fingerprint_when_both_current_fields_are_missing(self):
        manifest = self.manifest("http://x/a.mkv", None, None)
        previous = model.Fingerprint(
            model.parse_manifest(
                "# strm-probe-version:1\n# strm-probe-sha1:" + "a" * 40 +
                "\n# strm-probe-size:10\nhttp://x/a.mkv\n").url_hash, "a" * 40, 10)
        kind, reason = model.decide(manifest, previous, True, "http://x/a.mkv")
        self.assertEqual((kind, reason), ("SkipProbe", "InsufficientFingerprint"))


class ManifestParsingTests(unittest.TestCase):
    def test_omitted_lines_are_missing_not_placeholders(self):
        manifest = model.parse_manifest(
            "# strm-probe-version:1\nhttp://x/a.mkv\n")
        self.assertIsNone(manifest.sha1)
        self.assertIsNone(manifest.size)
        self.assertFalse(manifest.valid_sha1)
        self.assertFalse(manifest.valid_size)

    def test_an_invalid_sha1_is_treated_as_missing(self):
        manifest = model.parse_manifest(
            "# strm-probe-version:1\n# strm-probe-sha1:not-a-hash\n"
            "# strm-probe-size:10\nhttp://x/a.mkv\n")
        self.assertIsNone(manifest.sha1)
        self.assertEqual(manifest.size, 10)

    def test_the_url_is_the_first_non_comment_line(self):
        manifest = model.parse_manifest(
            "# strm-probe-version:1\n# a comment\nhttp://x/a.mkv\n")
        self.assertEqual(manifest.url, "http://x/a.mkv")

    def test_an_unsupported_version_is_rejected(self):
        with self.assertRaises(ValueError):
            model.parse_manifest("# strm-probe-version:2\nhttp://x/a.mkv\n")


class LogWindowTests(unittest.TestCase):
    def test_only_the_requested_item_is_reported(self):
        window = (
            '[2026-10-21 17:00:00.000 +00:00] [INF] [1] X: STRM source '
            '"reused saved media information" for project 11111111-1111-1111-1111-'
            '111111111111, source "11111111111111111111111111111111"; reason '
            'Sha1Matches, plugin probe calls 0.\n'
            '[2026-10-21 17:00:01.000 +00:00] [INF] [1] X: STRM source '
            '"reused saved media information" for project 22222222-2222-2222-2222-'
            '222222222222, source "22222222222222222222222222222222"; reason '
            'Unchanged, plugin probe calls 0.\n'
        )
        facts = scenario.decide_from_log(window, "11111111111111111111111111111111")
        self.assertEqual(facts["reason"], "Sha1Matches")
        self.assertFalse(facts["probe_started"])

    def test_a_probe_start_is_reported(self):
        window = ('[2026-10-21 17:00:00.000 +00:00] [INF] [1] X: Native STRM probe '
                  'started for item 11111111-1111-1111-1111-111111111111, source_key '
                  '"11111111111111111111111111111111"; running probes 1, active permits 1.\n')
        facts = scenario.decide_from_log(window, "11111111111111111111111111111111")
        self.assertTrue(facts["probe_started"])
        self.assertIsNone(facts["reason"])

    def test_an_empty_window_reports_nothing(self):
        facts = scenario.decide_from_log("", "11111111111111111111111111111111")
        self.assertIsNone(facts["decision"])
        self.assertIsNone(facts["reason"])
        self.assertFalse(facts["probe_started"])


if __name__ == "__main__":
    unittest.main()
