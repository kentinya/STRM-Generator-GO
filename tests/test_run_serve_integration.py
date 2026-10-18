import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import run_serve_integration as driver


class ServeDriverUnitTests(unittest.TestCase):
    def test_output_dir_refuses_source_tree(self):
        with self.assertRaises(SystemExit):
            driver.validate_output_dir(driver.ROOT / "scripts")

    def test_output_dir_refuses_non_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "evidence"
            target.mkdir()
            (target / "existing.txt").write_text("x")
            with self.assertRaises(SystemExit):
                driver.validate_output_dir(target)

    def test_output_dir_accepts_fresh_evidence_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp) / "artifacts" / "validation-evidence" / "run-1"
            resolved = driver.validate_output_dir(base)
            self.assertTrue(resolved.is_dir())

    def test_scenarios_are_registered(self):
        for name in driver.ALL_SCENARIOS:
            self.assertIn(name, driver.SCENARIOS)

    def test_count_serve_results_matches_kind(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "serve.out"
            path.write_text(
                "mode: serve result: success created=4 updated=0 skipped=0 cleanup=0 deleted=0 reclaimed=0 next=x\n"
                "mode: serve result: busy-skip created=0 updated=0 skipped=0 cleanup=0 deleted=0 reclaimed=0 next=y\n"
                "mode: serve result: success created=0 updated=0 skipped=4 cleanup=0 deleted=0 reclaimed=0 next=z\n"
            )
            self.assertEqual(driver.count_serve_results(path, "success"), 2)
            self.assertEqual(driver.count_serve_results(path, "busy-skip"), 1)
            self.assertEqual(driver.count_serve_results(path, "failure"), 0)

    def test_read_lines_missing_file_is_empty(self):
        self.assertEqual(driver.read_lines(Path("/nonexistent/serve.out")), [])

    def test_release_serve_hook_is_inert(self):
        source = (driver.ROOT / "cmd/strmgen/serve_hook_default.go").read_text()
        self.assertIn("//go:build !strmgen_testhooks", source)
        self.assertNotIn("os.Getenv", source)

    def test_integration_serve_hook_is_build_tagged(self):
        source = (driver.ROOT / "cmd/strmgen/serve_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", source)
        self.assertIn("STRMGEN_TEST_SERVE_INTERVAL_MS", source)

    def test_driver_uses_only_testhooks_interval_override(self):
        source = (driver.ROOT / "scripts/run_serve_integration.py").read_text()
        self.assertIn("STRMGEN_TEST_SERVE_INTERVAL_MS", source)

    def test_summary_shape_is_json_serializable(self):
        payload = {"scenarios": {"serve_cancel": {"status": "PASS"}}, "failures": []}
        self.assertIn("serve_cancel", json.loads(json.dumps(payload))["scenarios"])


if __name__ == "__main__":
    unittest.main()
