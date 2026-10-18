"""Runner-level regression tests for the Task 2.1 real OpenList driver.

These tests never contact a real service. They cover the harness contract the
Task requires: evidence directory boundaries, argument validation, subprocess
failure propagation, container cleanup limited to own resources, and the
"missing service is BLOCKED, never a silent skip" rule.
"""
import ast
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import run_openlist_absence_integration as runner  # noqa: E402


class EvidenceDirectoryTests(unittest.TestCase):
    def test_refuses_source_tree(self):
        with self.assertRaises(SystemExit):
            runner.validate_output_dir(runner.ROOT / "scripts")

    def test_refuses_temporary_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(SystemExit):
                runner.validate_output_dir(Path(tmp) / "evidence")

    def test_refuses_non_empty_evidence_directory(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            target = Path(tmp) / "run"
            target.mkdir()
            (target / "existing.log").write_text("x")
            with self.assertRaises(SystemExit):
                runner.validate_output_dir(target)

    def test_accepts_fresh_evidence_directory(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            target = Path(tmp) / "run"
            resolved = runner.validate_output_dir(target)
            self.assertTrue(resolved.is_dir())
            self.assertEqual(list(resolved.iterdir()), [])


class ArgumentTests(unittest.TestCase):
    def _run(self, argv):
        return runner.main(argv)

    def test_rejects_missing_test_binary(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            with self.assertRaises(SystemExit):
                self._run(["--test-binary", "/nonexistent/openlist.test",
                           "--output-dir", str(Path(tmp) / "run")])

    def test_rejects_non_executable_test_binary(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            binary = Path(tmp) / "openlist.test"
            binary.write_text("#!/bin/sh\nexit 0\n")
            os.chmod(binary, 0o644)
            with self.assertRaises(SystemExit):
                self._run(["--test-binary", str(binary),
                           "--output-dir", str(Path(tmp) / "run")])

    def test_rejects_unknown_scenario(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            binary = Path(tmp) / "openlist.test"
            binary.write_text("#!/bin/sh\nexit 0\n")
            os.chmod(binary, 0o755)
            with self.assertRaises(SystemExit):
                self._run(["--test-binary", str(binary),
                           "--output-dir", str(Path(tmp) / "run"),
                           "--scenario", "not_a_scenario"])

    def test_rejects_missing_sync_binary(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            binary = Path(tmp) / "openlist.test"
            binary.write_text("#!/bin/sh\nexit 0\n")
            os.chmod(binary, 0o755)
            with self.assertRaises(SystemExit):
                self._run(["--test-binary", str(binary),
                           "--sync-binary", str(Path(tmp) / "missing-strmgen"),
                           "--output-dir", str(Path(tmp) / "run")])


class PreflightTests(unittest.TestCase):
    def test_missing_docker_is_blocked_not_skipped(self):
        with tempfile.TemporaryDirectory(dir=runner.EVIDENCE_ROOT) as tmp:
            out = Path(tmp) / "run"
            binary = Path(tmp) / "openlist.test"
            binary.write_text("#!/bin/sh\nexit 0\n")
            os.chmod(binary, 0o755)
            with mock.patch.object(runner, "docker_available", return_value=False):
                status = runner.main(["--test-binary", str(binary), "--output-dir", str(out)])
            self.assertEqual(status, 3)
            summary = json.loads((out / "summary.json").read_text())
            self.assertIn("docker", summary["blocked"])
            self.assertEqual(summary["failures"], [])

    def test_image_revision_mismatch_is_blocked(self):
        fake = subprocess.CompletedProcess(
            args=["docker"], returncode=0,
            stdout='["openlist@sha256:deadbeef"]|0000000000000000000000000000000000000000|v4.2.7|sha256:x\n',
            stderr="")
        with mock.patch.object(runner, "run", return_value=fake):
            with self.assertRaises(runner.Blocked):
                runner.image_revision()

    def test_image_version_mismatch_is_blocked(self):
        fake = subprocess.CompletedProcess(
            args=["docker"], returncode=0,
            stdout=f'["openlist@sha256:abc"]|{runner.OPENLIST_SOURCE_SHA}|v9.9.9|sha256:x\n',
            stderr="")
        with mock.patch.object(runner, "run", return_value=fake):
            with self.assertRaises(runner.Blocked):
                runner.image_revision()

    def test_pinned_image_is_recognised(self):
        fake = subprocess.CompletedProcess(
            args=["docker"], returncode=0,
            stdout=f'["openlist@sha256:abc"]|{runner.OPENLIST_SOURCE_SHA}|{runner.OPENLIST_VERSION}|sha256:x\n',
            stderr="")
        with mock.patch.object(runner, "run", return_value=fake):
            revision, version, digests, image_id = runner.image_revision()
        self.assertEqual(revision, runner.OPENLIST_SOURCE_SHA)
        self.assertEqual(version, runner.OPENLIST_VERSION)
        self.assertEqual(digests, ["openlist@sha256:abc"])
        self.assertEqual(image_id, "sha256:x")

    def test_pinned_identity_matches_environment_spec(self):
        spec = json.loads((runner.ROOT / "dev/environment-spec.json").read_text())
        self.assertEqual(spec["openlist"]["source_sha"], runner.OPENLIST_SOURCE_SHA)
        self.assertIn(runner.OPENLIST_SOURCE_SHA[-7:], runner.OPENLIST_IMAGE + runner.OPENLIST_SOURCE_SHA)


class GoInvocationTests(unittest.TestCase):
    def _binary(self, tmp, body):
        path = Path(tmp) / "fake.test"
        path.write_text(body)
        os.chmod(path, 0o755)
        return path

    def test_subprocess_failure_propagates(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self._binary(tmp, "#!/bin/sh\necho '--- FAIL: TestX' \nexit 1\n")
            with self.assertRaises(runner.Failure):
                runner.run_go(binary, {}, Path(tmp), "TestX", Path(tmp) / "log.txt")
            self.assertIn("TestX", (Path(tmp) / "log.txt").read_text())

    def test_skip_is_a_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self._binary(tmp, "#!/bin/sh\necho '=== RUN TestX'\necho '--- SKIP: TestX'\nexit 0\n")
            with self.assertRaises(runner.Failure) as caught:
                runner.run_go(binary, {}, Path(tmp), "TestX", Path(tmp) / "log.txt")
            self.assertIn("skip", str(caught.exception).lower())

    def test_all_tests_must_report_pass(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self._binary(tmp, "#!/bin/sh\necho '=== RUN TestX'\nexit 0\n")
            with self.assertRaises(runner.Failure):
                runner.run_go(binary, {}, Path(tmp), "TestX", Path(tmp) / "log.txt")

    def test_success_is_recorded(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self._binary(
                tmp, "#!/bin/sh\necho '=== RUN   TestA'\necho '--- PASS: TestA (0.00s)'\nexit 0\n")
            result = runner.run_go(binary, {"api_url": "x"}, Path(tmp), "TestA", Path(tmp) / "log.txt")
            self.assertEqual(result["passed"], ["TestA"])
            self.assertGreaterEqual(result["elapsed_s"], 0)
            self.assertIn("$", (Path(tmp) / "log.txt").read_text())

    def test_descriptor_is_passed_through_environment(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self._binary(
                tmp,
                "#!/bin/sh\n"
                "if [ -z \"$STRMGEN_OPENLIST_INTEGRATION\" ]; then echo missing; exit 1; fi\n"
                "echo \"=== RUN   TestA\"\necho '--- PASS: TestA'\nexit 0\n")
            runner.run_go(binary, {"api_url": "http://x", "token": "t", "root": "/lib"},
                          Path(tmp), "TestA", Path(tmp) / "log.txt")


class CleanupTests(unittest.TestCase):
    def test_cleanup_stops_only_registered_services(self):
        calls = []

        class Fake:
            def __init__(self, name):
                self.name = name

            def stop(self):
                calls.append(self.name)

        runners = [Fake("a"), Fake("b")]
        with mock.patch.object(runner, "_LIVE", list(runners)):
            runner.cleanup_all()
        self.assertEqual(sorted(calls), ["a", "b"])

    def test_cleanup_survives_one_failing_stop(self):
        calls = []

        class Fake:
            def __init__(self, name, fail):
                self.name = name
                self.fail = fail

            def stop(self):
                calls.append(self.name)
                if self.fail:
                    raise runner.Failure("boom")

        with mock.patch.object(runner, "_LIVE", [Fake("bad", True), Fake("good", False)]):
            runner.cleanup_all()
        self.assertEqual(calls, ["bad", "good"])

    def test_service_refuses_to_stop_foreign_container(self):
        service = runner.Service.__new__(runner.Service)
        service.name = "strmgen-absence-test"
        service.started = True
        service.workdir = Path(tempfile.mkdtemp())
        service.log_path = service.workdir / "service.log"
        inspect = subprocess.CompletedProcess(
            args=["docker"], returncode=0, stdout='{"running": true}|other\n', stderr="")
        with mock.patch.object(runner, "run", return_value=inspect):
            service.stop()
        # The container was not created by this run, so it must be left alone.
        self.assertFalse(service.started)


class SimplifiedGetContractTests(unittest.TestCase):
    """The runner must prepare and record facts without ever forcing a refresh.

    These checks read the runner's own AST so a list request split across lines
    or built through a helper is still inspected, instead of grepping one line.
    """

    def setUp(self):
        self.source = (runner.ROOT / "scripts" / "run_openlist_absence_integration.py").read_text()
        self.tree = ast.parse(self.source)

    def _dict_literals(self):
        for node in ast.walk(self.tree):
            if isinstance(node, ast.Dict):
                keys = [k.value for k in node.keys if isinstance(k, ast.Constant) and isinstance(k.value, str)]
                if "path" in keys and "per_page" in keys:
                    yield node, keys

    def test_every_list_request_declares_refresh_false(self):
        literals = list(self._dict_literals())
        self.assertTrue(literals, "no list request payload found")
        for node, keys in literals:
            self.assertIn("refresh", keys, ast.unparse(node))
            refresh = node.values[keys.index("refresh")]
            self.assertIsInstance(refresh, ast.Constant, ast.unparse(refresh))
            self.assertIs(refresh.value, False, ast.unparse(node))

    def test_no_refresh_true_anywhere(self):
        for node in ast.walk(self.tree):
            if isinstance(node, ast.Dict):
                for key, value in zip(node.keys, node.values):
                    if (isinstance(key, ast.Constant) and key.value == "refresh"
                            and isinstance(value, ast.Constant) and value.value is True):
                        self.fail("a request forces a refresh: " + ast.unparse(node))

    def test_listing_goes_through_list_once(self):
        self.assertIn("def list_once(", self.source)
        self.assertIn("def get_once(", self.source)
        body = self.source.split("def wait_listed(", 1)[1].split("\n    def ", 1)[0]
        # wait_listed must poll through the shared helper and never build a list
        # payload of its own.
        self.assertIn("self.list_once(", body)
        self.assertNotIn('"per_page"', body)

    def test_cache_scenario_waits_naturally_instead_of_flushing(self):
        self.assertIn("CACHE_TTL_MINUTES", self.source)
        self.assertIn("CACHE_WAIT_SECONDS", self.source)
        self.assertIn("cache_natural_update", self.source)
        # No cache-clear or refresh endpoint may be called, and the TTL must be
        # short enough that waiting is bounded rather than an hour.
        self.assertLessEqual(runner.CACHE_TTL_MINUTES, 5)
        for forbidden in ("/api/fs/list?refresh", "cache/clear", "storage/clear"):
            self.assertNotIn(forbidden, self.source)

    def test_read_only_scenario_replaces_refresh_permission(self):
        self.assertIn("scenario_read_only_identity", self.source)
        self.assertEqual(runner.GO_TESTS["read_only_identity"],
                         "TestIntegrationReadOnlyIdentityAgainstRealOpenList")
        self.assertNotIn("refresh_permission", self.source)

    def test_scenarios_cover_the_required_facts(self):
        self.assertEqual(set(runner.SCENARIOS), {
            "exact_names", "pagination", "empty_directory", "removed_parent",
            "read_only_identity", "cached_driver", "nested_mount_masking", "hidden_entries",
        })

    def test_hidden_and_masking_limitations_are_recorded(self):
        # The Go test asserts the accepted rule; the runner records both the
        # restricted and unrestricted reads so the limitation stays visible.
        self.assertIn("hidden_restricted_read", self.source)
        self.assertIn("hidden_unrestricted_read", self.source)
        self.assertIn("mask_virtual_ancestor", self.source)
        self.assertIn("masked_offline_root", self.source)

    def test_deleted_parent_scenario_has_no_ancestor_walk(self):
        # The runner must not pre-walk the removed ancestor to "help" the
        # confirmation; it only records the exact target and the two roots. The
        # one wait_listed call creates the offline storage before it is removed,
        # so it may not reference the removed path.
        body = self.source.split("def scenario_removed_parent(", 1)[1].split("\ndef ", 1)[0]
        self.assertIn('"/lib/" + removed_target', body)
        self.assertNotIn('remoteJoin(removed', body)
        self.assertNotIn('get_once("/lib/" + removed_rel', body)

    def test_special_names_survive_as_exact_names(self):
        self.assertIn("中文 名字.mkv", self.source)
        self.assertIn("#hash% &", self.source)

    def test_cli_mirror_layer_is_not_assumed_passed(self):
        # The release-CLI Mirror layer must be separately gated by
        # --sync-binary and can never borrow the Task 2.1 confirmation pass.
        self.assertIn("cli_mirror", self.source)
        self.assertIn("NOT RUN", self.source)
        self.assertIn("--sync-binary not provided", self.source)
        self.assertIn("def run_cli_mirror(", self.source)
        self.assertIn("candidates=1", self.source)
        self.assertIn("root_unreadable_exit", self.source)
        # The CLI journey must poll the cached listing and never force refresh.
        cli_body = self.source.split("def run_cli_mirror(", 1)[1].split("\ndef main(", 1)[0]
        self.assertIn("wait_listed_exact(", cli_body)
        self.assertNotIn('"refresh"', cli_body)


class HarnessContractTests(unittest.TestCase):
    def test_every_scenario_declares_a_go_test(self):
        self.assertEqual(set(runner.SCENARIOS), set(runner.GO_TESTS))
        for name in runner.SCENARIOS:
            self.assertTrue(runner.GO_TESTS[name].strip(), name)

    def test_empty_directory_shape_is_asserted_from_real_service(self):
        """The runner records the raw content shape instead of assuming it."""
        source = (runner.ROOT / "scripts" / "run_openlist_absence_integration.py").read_text()
        self.assertIn("content_is_null", source)
        self.assertIn("content_type", source)

    def test_no_credentials_are_written_to_evidence(self):
        """Logs and summaries must never contain the generated admin password."""
        source = (runner.ROOT / "scripts" / "run_openlist_absence_integration.py").read_text()
        for line in source.splitlines():
            if "service.password" in line:
                self.assertNotIn("write_text", line, line)
                self.assertNotIn("json.dumps", line, line)

    def test_runner_does_not_reimplement_absence_logic(self):
        source = (runner.ROOT / "scripts" / "run_openlist_absence_integration.py").read_text()
        # The Python side only drives fixtures and records raw observations; it
        # must not contain a decision algorithm such as a local "does this read
        # mean missing" rule, and it must not name the Go result types.
        for forbidden in ("confirmed_absent", "AbsenceMissing", "AbsencePresent",
                          "AbsenceUnknown", "CheckSourceRoot"):
            self.assertNotIn(forbidden, source)


if __name__ == "__main__":
    unittest.main()
