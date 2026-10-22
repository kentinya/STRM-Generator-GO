#!/usr/bin/env python3
"""Direct behaviour tests for the Task 4.6 helper tools.

These tests stub only the external I/O of the helper tools. They verify the
readable mirror of the locked plugin's decision rules, the manifest parser and
the log-window decision extraction. They are NOT real Jellyfin or S3 evidence.

    python3 -m unittest discover -s artifacts/task-4.6/tests -p 'test_task46*.py' -v
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import sys
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
retired = _load("task46_fingerprint_read_retired", "fingerprint_read.py")


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


class HistoryMirroringTests(unittest.TestCase):
    """The historical side comes from native log reasons, never a store read."""

    BASE = "http://openlist:5244/d/s3/Plain%20Movie.mkv"
    # The recorded baseline A facts of the decision copy.
    BASELINE_SHA1 = "1b7aa9d139d059cdcc579d04e9e36a6b005fcd7e"
    BASELINE_SIZE = 24404

    def baseline_text(self, url=None):
        return ("# strm-probe-version:1\n"
                "# strm-probe-sha1:" + self.BASELINE_SHA1 + "\n"
                "# strm-probe-size:" + str(self.BASELINE_SIZE) + "\n"
                + (url or self.BASE) + "\n")

    def predicted(self, text, historical_reason, native_url=None,
                  has_primary_stream=True):
        import matrix
        return matrix.predicted_reason(text, historical_reason,
                                       native_url if native_url is not None else self.BASE,
                                       has_primary_stream,
                                       self.BASE, self.BASELINE_SHA1,
                                       self.BASELINE_SIZE)

    def setUp(self):
        if "matrix" not in sys.modules:
            sys.modules["matrix"] = _load("task46_matrix", "matrix.py")

    def test_sha1_matches_history_mirrors_the_baseline_facts(self):
        p = self.predicted(self.baseline_text(), "Sha1Matches")
        self.assertEqual(p["reason"], "Sha1Matches")
        self.assertEqual(p["history_side"]["fingerprint_store_read"], False)
        self.assertEqual(p["previous_sha1"], self.BASELINE_SHA1)
        self.assertEqual(p["history_side"]["baseline_url"], self.BASE)

    def test_unchanged_history_mirrors_a_size_only_fingerprint(self):
        text = ("# strm-probe-version:1\n"
                "# strm-probe-size:" + str(self.BASELINE_SIZE) + "\n"
                + self.BASE + "\n")
        p = self.predicted(text, "Unchanged")
        self.assertEqual(p["reason"], "Unchanged")
        self.assertIsNone(p["previous_sha1"])
        self.assertEqual(p["previous_size"], self.BASELINE_SIZE)

    def test_first_trust_history_has_no_comparable_fingerprint(self):
        p = self.predicted(self.baseline_text(), "FirstTrust")
        self.assertEqual(p["reason"], "FirstTrust")
        self.assertIsNone(p["previous_sha1"])
        self.assertIsNone(p["previous_url_hash"])

    def test_url_change_with_sha1_history_requires_a_probe(self):
        other = "http://openlist:5244/d/s3/Synthetic%20001.mkv"
        p = self.predicted(self.baseline_text(other), "Sha1Matches",
                           native_url=other)
        self.assertEqual(p["reason"], "UrlChanged")

    def test_native_address_still_the_baseline_url_is_a_mismatch(self):
        other = "http://openlist:5244/d/s3/Synthetic%20001.mkv"
        p = self.predicted(self.baseline_text(other), "Sha1Matches",
                           native_url=self.BASE)
        self.assertEqual(p["reason"], "NativeUrlMismatch")

    def test_missing_primary_stream_wins_over_history(self):
        p = self.predicted(self.baseline_text(), "Sha1Matches",
                           has_primary_stream=False)
        self.assertEqual(p["reason"], "MissingPrimaryStream")

    def test_row_result_records_no_store_read(self):
        """The retired store entry cannot be called from the row path."""
        matrix = sys.modules["matrix"]
        with self.assertRaises(RuntimeError):
            matrix.fingerprint_of("any-item")

    def test_retired_entry_is_fail_closed(self):
        with self.assertRaises(retired.DirectStoreReadForbidden):
            retired.read("any-item")
        self.assertIsNone(retired.STORE)

    def test_retired_entry_main_returns_3(self):
        with contextlib.redirect_stderr(io.StringIO()) as captured:
            self.assertEqual(retired.main(), 3)
        self.assertIn("retired", captured.getvalue())


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
