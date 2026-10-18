import json
from pathlib import Path
import sqlite3
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import run_isolated_sync_integration as driver


class DriverUnitTests(unittest.TestCase):
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
            driver.ROOT.joinpath("artifacts")
            resolved = driver.validate_output_dir(base)
            self.assertTrue(resolved.is_dir())

    def test_assert_valid_strm_accepts_good_document(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "A.strm"
            path.write_bytes(b"# strm-probe-version:1\n# strm-probe-size:0\nhttps://e.invalid/d/A.mkv\n")
            driver.assert_valid_strm(path, "https://e.invalid/d/A.mkv")

    def test_assert_valid_strm_rejects_unsafe_url(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "A.strm"
            path.write_bytes(b"# strm-probe-version:1\nhttps://e.invalid/d/A.mkv?sign=x\n")
            with self.assertRaises(driver.Failure):
                driver.assert_valid_strm(path, "https://e.invalid/d/A.mkv")

    def test_assert_valid_strm_rejects_modified(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "A.strm"
            path.write_bytes(b"# strm-probe-version:1\n# Modified: 2020\nhttps://e.invalid/d/A.mkv\n")
            with self.assertRaises(driver.Failure):
                driver.assert_valid_strm(path, "https://e.invalid/d/A.mkv")

    def test_sidecar_detection(self):
        with tempfile.TemporaryDirectory() as tmp:
            db = Path(tmp) / "strmgen.db"
            db.write_bytes(b"")
            (Path(str(db) + "-wal")).write_bytes(b"x")
            names = [name for name, _ in driver.sidecars(db)]
            self.assertIn("strmgen.db-wal", names)

    def test_read_db_escapes_uri_special_characters(self):
        with tempfile.TemporaryDirectory() as tmp:
            db = Path(tmp) / "data ?name#percent%" / "state ?#%.sqlite"
            db.parent.mkdir()
            conn = sqlite3.connect(db)
            try:
                conn.executescript(
                    "CREATE TABLE ownership (source_key TEXT, relative_path TEXT, absolute_path TEXT, missing_count INTEGER);"
                    "CREATE TABLE rounds (source_key TEXT, epoch INTEGER);"
                    "INSERT INTO ownership VALUES ('source', 'A.strm', '/out/A.strm', 0);"
                    "INSERT INTO rounds VALUES ('source', 1);"
                )
                conn.commit()
            finally:
                conn.close()
            state = driver.read_db(db)
            self.assertEqual(len(state["ownership"]), 1)
            self.assertEqual(state["rounds"], [("source", 1)])

    def test_summary_shape_is_json_serializable(self):
        payload = {"scenarios": {"first_generation": {"status": "PASS"}}, "failures": []}
        self.assertIn("first_generation", json.loads(json.dumps(payload))["scenarios"])

    def test_release_ownership_hook_is_inert(self):
        source = (driver.ROOT / "internal/reconciler/syncer_hook_default.go").read_text()
        self.assertIn("//go:build !strmgen_testhooks", source)
        self.assertNotIn("os.Getenv", source)
        self.assertNotIn("syscall", source)

    def test_integration_ownership_hook_is_build_tagged(self):
        source = (driver.ROOT / "internal/reconciler/syncer_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", source)
        self.assertIn("STRMGEN_TEST_OWNERSHIP_HOLD", source)
        self.assertIn("SIGSTOP", source)

    def test_fault_recovery_ownership_interruption_is_deterministic(self):
        # F-9 regression guard: the ownership-interruption sub-case must use the
        # testhooks barrier, not racy sampling of an in-flight process.
        source = (driver.ROOT / "scripts/run_isolated_sync_integration.py").read_text()
        self.assertIn("STRMGEN_TEST_OWNERSHIP_HOLD", source)
        self.assertNotIn("ownership_proc.send_signal(signal.SIGSTOP)", source)

    def test_release_update_hooks_are_inert(self):
        for name in ("fs_update_hook_default.go", "fs_update_after_hook_default.go"):
            source = (driver.ROOT / "internal/reconciler" / name).read_text()
            self.assertIn("//go:build !strmgen_testhooks", source)
            self.assertNotIn("os.Getenv", source)
            self.assertNotIn("syscall", source)

    def test_integration_update_hooks_are_build_tagged(self):
        exchange = (driver.ROOT / "internal/reconciler/fs_update_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", exchange)
        self.assertIn("STRMGEN_TEST_UPDATE_BEFORE_EXCHANGE_TARGET", exchange)
        for name in ("STRMGEN_TEST_UPDATE_PRE_EXCHANGE_HOLD", "STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE",
                     "STRMGEN_TEST_UPDATE_EXCHANGE_HOLD", "STRMGEN_TEST_UPDATE_EXCHANGE_GATE"):
            self.assertIn(name, exchange)
        after = (driver.ROOT / "internal/reconciler/fs_update_after_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", after)
        self.assertIn("STRMGEN_TEST_UPDATE_HOLD", after)
        self.assertIn("STRMGEN_TEST_UPDATE_PUBLISH_GATE", after)
        self.assertIn("SIGSTOP", exchange)

    def test_update_window_interruptions_are_deterministic(self):
        source = (driver.ROOT / "scripts/run_isolated_sync_integration.py").read_text()
        for name in ("STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE", "STRMGEN_TEST_UPDATE_EXCHANGE_GATE",
                     "STRMGEN_TEST_UPDATE_PUBLISH_GATE", "update_before_verification_sigkill",
                     "update_pre_exchange_sigkill", "update_cancel_"):
            self.assertIn(name, source)

    def test_update_interruption_recovery_is_deterministic(self):
        source = (driver.ROOT / "scripts/run_isolated_sync_integration.py").read_text()
        self.assertIn("STRMGEN_TEST_UPDATE_HOLD", source)
        self.assertIn("update-interruption recovery requires --fault-binary", source)

    def test_mirror_scenarios_are_registered(self):
        self.assertIn("mirror_cleanup", driver.ALL_SCENARIOS)
        self.assertIn("mirror_recovery", driver.ALL_SCENARIOS)
        self.assertIn("mirror_cleanup", driver.SCENARIOS)
        self.assertIn("mirror_recovery", driver.SCENARIOS)

    def test_release_delete_hooks_are_inert(self):
        source = (driver.ROOT / "internal/reconciler/fs_delete_hook_default.go").read_text()
        self.assertIn("//go:build !strmgen_testhooks", source)
        self.assertNotIn("os.Getenv", source)
        self.assertNotIn("syscall", source)

    def test_integration_delete_hooks_are_build_tagged(self):
        source = (driver.ROOT / "internal/reconciler/fs_delete_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", source)
        for name in ("STRMGEN_TEST_DELETE_AFTER_UNLINK_FAIL_TARGET",
                     "STRMGEN_TEST_DELETE_AFTER_UNLINK_HOLD",
                     "STRMGEN_TEST_DELETE_BEFORE_UNLINK_GATE",
                     "SIGSTOP"):
            self.assertIn(name, source)

    def test_delete_windows_are_deterministic(self):
        source = (driver.ROOT / "scripts/run_isolated_sync_integration.py").read_text()
        for name in ("STRMGEN_TEST_DELETE_AFTER_UNLINK_FAIL_TARGET",
                     "STRMGEN_TEST_DELETE_AFTER_UNLINK_HOLD",
                     "STRMGEN_TEST_DELETE_BEFORE_UNLINK_GATE",
                     "block_ownership_delete", "cross_file_partial"):
            self.assertIn(name, source)


if __name__ == "__main__":
    unittest.main()
