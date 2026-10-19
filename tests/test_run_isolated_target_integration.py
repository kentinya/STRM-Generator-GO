"""Unit tests for the isolated target acceptance driver.

These tests exercise the driver's pure helpers (redaction, credential resolution,
decision-log windows, STRM parsing, tree digest, evidence hygiene) without starting
any container, OpenList, Jellyfin, or browser. The real target chain is exercised
separately by the acceptance run itself.
"""

import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest import mock

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

    def test_reads_sha1_marker_when_present(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.strm"
            sha1 = "a" * 40
            path.write_text(f"# strm-probe-version:1\n# strm-probe-sha1:{sha1}\nhttp://h/d/x.mkv\n",
                            encoding="utf-8")
            self.assertEqual(driver.read_strm(path)["sha1"], sha1)

    def test_sha1_absent_is_none(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "x.strm"
            path.write_text("# strm-probe-version:1\nhttp://h/d/x.mkv\n", encoding="utf-8")
            self.assertIsNone(driver.read_strm(path)["sha1"])


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


class CredentialSourceTests(unittest.TestCase):
    """Regression: credentials come from the process environment, never from argv."""

    BASE_ENV = {
        "OPENLIST_TOKEN": "openlist-token-value",
        "JF_PLAYBACK_USER": "playback",
        "JF_PLAYBACK_PASSWORD": "playback-secret-value",
    }

    def test_cli_exposes_no_secret_bearing_option(self):
        options = {action.dest for action in driver.build_parser()._actions}
        for forbidden in ("playback_password", "playback_user", "password", "token", "secret"):
            self.assertNotIn(forbidden, options)
        self.assertIn("playback_driver", options)
        self.assertIn("jellyfin_log", options)

    def test_no_secret_is_required_or_accepted_on_the_command_line(self):
        with self.assertRaises(SystemExit):
            with redirect_stderr(io.StringIO()):
                driver.build_parser().parse_args(["--playback-password", "leaky"])

    def test_credentials_come_from_environment(self):
        env = dict(self.BASE_ENV, JF_ADMIN_TOKEN="admin-token-value")
        creds = driver.resolve_credentials(env, require_playback=True)
        self.assertEqual(creds.openlist_token, "openlist-token-value")
        self.assertIn('Token="admin-token-value"', creds.jellyfin_authorization)
        self.assertEqual(creds.playback_user, "playback")
        self.assertEqual(creds.playback_password, "playback-secret-value")

    def test_missing_openlist_token_is_refused(self):
        env = dict(self.BASE_ENV, JF_ADMIN_TOKEN="x")
        env.pop("OPENLIST_TOKEN")
        with self.assertRaises(driver.Failure):
            driver.resolve_credentials(env, require_playback=False)

    def test_missing_playback_credential_is_refused(self):
        env = dict(self.BASE_ENV, JF_ADMIN_TOKEN="x")
        env.pop("JF_PLAYBACK_PASSWORD")
        with self.assertRaises(driver.Failure):
            driver.resolve_credentials(env, require_playback=True)

    def test_admin_identity_requires_token_or_password_pair(self):
        env = {"OPENLIST_TOKEN": "t", "JF_USER": "admin"}
        with self.assertRaises(driver.Failure):
            driver.resolve_credentials(env, require_playback=False)


class EvidenceHygieneTests(unittest.TestCase):
    """Regression: a process credential must never reach an evidence record."""

    def test_recorder_refuses_secret_in_record(self):
        env = {"OPENLIST_TOKEN": "top-secret-token"}
        with tempfile.TemporaryDirectory() as tmp:
            recorder = driver.Recorder(Path(tmp), env)
            with self.assertRaises(driver.Failure):
                recorder.record("leak", value="top-secret-token")
            self.assertFalse((Path(tmp) / "leak.json").exists())

    def test_recorder_writes_redacted_record(self):
        env = {"OPENLIST_TOKEN": "top-secret-token"}
        with tempfile.TemporaryDirectory() as tmp:
            recorder = driver.Recorder(Path(tmp), env)
            recorder.record("ok", summary=driver.redact("fetched http://h/d/x?sign=1 token=top-secret-token"))
            recorder.finish()
            written = (Path(tmp) / "ok.json").read_text()
            self.assertNotIn("top-secret-token", written)
            self.assertNotIn("sign=1", written)
            self.assertNotIn("top-secret-token", (Path(tmp) / "summary.json").read_text())

    def test_secret_values_ignores_unset(self):
        self.assertEqual(driver.secret_values({"OPENLIST_TOKEN": ""}), [])


class DecisionWindowTests(unittest.TestCase):
    """Regression: cumulative or empty logs must not be reported as a decision matrix."""

    ITEM = "13b9046f-1e07-5fdf-0c2e-e4af320b929c"

    def _log(self, tmp, lines):
        path = Path(tmp) / "log.log"
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")
        return driver.DecisionLog(path)

    def test_window_only_sees_lines_after_mark(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, ["noise"])
            mark = log.mark()
            with open(Path(tmp) / "log.log", "a", encoding="utf-8") as handle:
                handle.write(f'[INF] STRM source "reused saved media information" for project {self.ITEM}, '
                             f'source "13b9046f1e075fdf0c2ee4af320b929c"; reason FirstTrust, plugin probe calls 0.\n')
            self.assertEqual(log.since(mark)[0]["reason"], "FirstTrust")
            self.assertEqual(log.since(log.mark()), [])

    def test_require_reuse_fails_on_empty_window(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, ["unrelated"])
            with self.assertRaises(driver.Failure):
                log.require_reuse(0, self.ITEM)

    def test_require_reuse_fails_for_wrong_item(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, [
                f'[INF] STRM source "reused saved media information" for project {self.ITEM}, '
                f'source "13b9046f1e075fdf0c2ee4af320b929c"; reason Unchanged, plugin probe calls 0.'])
            with self.assertRaises(driver.Failure):
                log.require_reuse(0, "ffffffff-ffff-ffff-ffff-ffffffffffff")

    def test_require_reuse_rejects_probe_required_reason(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, [
                f'[INF] STRM source "reused saved media information" for project {self.ITEM}, '
                f'source "13b9046f1e075fdf0c2ee4af320b929c"; reason UrlChanged, plugin probe calls 0.'])
            with self.assertRaises(driver.Failure):
                log.require_reuse(0, self.ITEM, {"Unchanged"})

    def test_item_id_format_is_normalized(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, [
                f'[INF] STRM source "reused saved media information" for project {self.ITEM}, '
                f'source "13b9046f1e075fdf0c2ee4af320b929c"; reason Unchanged, plugin probe calls 0.'])
            # Jellyfin API responses use the hyphenless N form; the log prints the D form.
            self.assertEqual(log.require_reuse(0, self.ITEM.replace("-", ""))["item"], driver.normalize_item_id(self.ITEM))

    def test_normalize_item_id(self):
        self.assertEqual(driver.normalize_item_id("13B9046F-1E07-5FDF-0C2E-E4AF320B929C"),
                         "13b9046f1e075fdf0c2ee4af320b929c")

    def test_require_reuse_detects_probe_lines(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = self._log(tmp, [
                f'[INF] Native STRM probe started for item {self.ITEM}, '
                f'source_key "13b9046f1e075fdf0c2ee4af320b929c"; running probes 1, active permits 1.'])
            entries = log.since(0)
            self.assertEqual(entries[0]["kind"], "probe")

    def test_missing_log_file_is_empty_not_pass(self):
        log = driver.DecisionLog(Path("/nonexistent/task-4.1.log"))
        self.assertEqual(log.since(0), [])
        with self.assertRaises(driver.Failure):
            log.require_reuse(0, self.ITEM)

    def test_decision_rows_fail_without_decisions(self):
        env = {}
        with tempfile.TemporaryDirectory() as tmp:
            recorder = driver.Recorder(Path(tmp), env)
            log = driver.DecisionLog(Path(tmp) / "missing.log")
            hash_probe = {"sha1_available": False, "observation": "none"}
            with self.assertRaises(driver.Failure):
                driver.scenario_decision_rows(recorder, log, ["Plain Movie.mkv"], 0, hash_probe)
            self.assertFalse((Path(tmp) / "ro3_decision_rows.json").exists())

    def test_decision_rows_pass_with_linked_window(self):
        with tempfile.TemporaryDirectory() as tmp:
            names = ["Plain Movie.mkv"]
            lines = [f'[INF] STRM source "reused saved media information" for project {self.ITEM}, '
                     f'source "13b9046f1e075fdf0c2ee4af320b929c"; reason Unchanged, plugin probe calls 0.']
            path = Path(tmp) / "log.log"
            path.write_text("\n".join(lines), encoding="utf-8")
            recorder = driver.Recorder(Path(tmp) / "out", {})
            driver.scenario_decision_rows(recorder, driver.DecisionLog(path), names, 0,
                                          {"sha1_available": False, "observation": "none"})
            record = json.loads((Path(tmp) / "out" / "ro3_decision_rows.json").read_text())
            rows = {row["row"]: row["status"] for row in record["rows"]}
            self.assertEqual(rows["stable_url_same_fingerprint"], "PASS")
            self.assertEqual(rows["same_url_same_sha1_size_changed"], "BLOCKED")
            self.assertEqual(rows["url_changed"], "DEFERRED")


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
        self.assertNotIn("s3cr3t-value", " ".join(str(part) for part in captured["args"]))
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


class ObjectKeyTests(unittest.TestCase):
    """Regression: S3 keys are bucket-relative, so a mount root must not repeat in the key."""

    def test_mount_root_maps_to_bucket_root(self):
        self.assertEqual(driver.object_key({}, "/s3", "Plain Movie.mkv"), "Plain Movie.mkv")
        self.assertEqual(driver.object_key({}, "/s3", "Moved/Plain Movie.mkv"), "Moved/Plain Movie.mkv")

    def test_explicit_prefix_and_non_root_mount(self):
        env = {"RUSTFS_KEY_PREFIX": "media", "RUSTFS_MOUNT_IS_ROOT": "false"}
        self.assertEqual(driver.object_key(env, "/s3", "a.mkv"), "media/s3/a.mkv")


class LibraryMappingTests(unittest.TestCase):
    ITEM = "6dee4db6-a720-fe6b-21cb-72f1784bee17"

    def test_mapping_keys_on_relative_path(self):
        payload = {"Items": [
            {"Id": self.ITEM, "Name": "Plain Movie", "Path": "/strm/Plain Movie.strm"},
            {"Id": "x", "Name": "Plain Movie", "Path": "/strm/Moved/Plain Movie.strm"},
            {"Id": "y", "Name": "skip", "Path": "/media/Plain Movie.mkv"},
        ]}

        class FakeTarget:
            def jellyfin_json(self, path, method="GET", data=None):
                return payload

        mapping = driver.library_items(FakeTarget(), "lib")
        self.assertIn("Plain Movie", mapping)
        self.assertIn("Moved/Plain Movie", mapping)
        self.assertEqual(len(mapping), 2)


if __name__ == "__main__":
    unittest.main()
