import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import run_container_integration as driver


class ContainerDriverUnitTests(unittest.TestCase):
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

    def test_elf_machine_detects_aarch64(self):
        # Minimal little-endian ELF header with e_machine = 183 (EM_AARCH64).
        header = bytearray(64)
        header[:4] = b"\x7fELF"
        header[4] = 2  # 64-bit
        header[5] = 1  # little endian
        header[18] = 183
        self.assertEqual(driver.elf_machine(bytes(header)), 183)

    def test_elf_machine_detects_amd64(self):
        header = bytearray(64)
        header[:4] = b"\x7fELF"
        header[5] = 1
        header[18] = 62  # EM_X86_64
        self.assertEqual(driver.elf_machine(bytes(header)), 62)

    def test_elf_machine_rejects_non_elf(self):
        self.assertIsNone(driver.elf_machine(b"not-an-elf-file"))

    def test_read_db_handles_missing_database(self):
        with tempfile.TemporaryDirectory() as tmp:
            state = driver.read_db(Path(tmp) / "missing.db")
            self.assertFalse(state["exists"])
            self.assertEqual(state["rounds"], [])

    def test_config_document_has_no_token(self):
        document = driver.config_document("http://example.invalid")
        self.assertIn("token_env", document)
        self.assertNotIn(driver.SECRET_MARKER, document)

    def test_release_binary_has_no_testhooks_markers(self):
        # The forbidden markers must only exist in build-tagged or test files.
        source = (driver.ROOT / "cmd/strmgen/serve_hook_integration.go").read_text()
        self.assertIn("//go:build strmgen_testhooks", source)
        self.assertIn(driver.FORBIDDEN_IMAGE_MARKERS[1].decode(), source)
        release = (driver.ROOT / "cmd/strmgen/serve_hook_default.go").read_text()
        self.assertIn("//go:build !strmgen_testhooks", release)
        self.assertNotIn("os.Getenv", release)

    def test_dockerfile_pins_digests_and_cgo(self):
        source = (driver.ROOT / "Dockerfile").read_text()
        self.assertIn("CGO_ENABLED=0", source)
        self.assertIn("golang:1.27.2@sha256:", source)
        self.assertIn("alpine:3.20@sha256:", source)
        self.assertIn("USER 10001:10001", source)
        self.assertNotIn("strmgen_testhooks", source)

    def test_compose_has_no_embedded_token(self):
        source = (driver.ROOT / "deploy/compose.yaml").read_text()
        self.assertIn("OPENLIST_TOKEN", source)
        self.assertNotIn(driver.SECRET_MARKER, source)
        self.assertNotIn("privileged:", source)
        self.assertNotIn("docker.sock", source)
        self.assertIn("/tmp/strmgen-locks", source)
        self.assertIn(":ro", source)

    def test_summary_shape_is_json_serializable(self):
        payload = {"scenarios": {"minimal_run": {"status": "PASS"}}, "failures": []}
        self.assertIn("minimal_run", json.loads(json.dumps(payload))["scenarios"])


if __name__ == "__main__":
    unittest.main()
