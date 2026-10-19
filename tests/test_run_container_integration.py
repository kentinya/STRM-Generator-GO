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

    def test_compose_token_is_runtime_passthrough_not_interpolation(self):
        # The supported evidence command is config --no-interpolate. Keep the
        # environment passthrough contract; ordinary config may still expose values.
        source = (driver.ROOT / "deploy/compose.yaml").read_text()
        self.assertNotIn("${OPENLIST_TOKEN", source)
        lines = [line.strip() for line in source.splitlines()]
        self.assertIn("OPENLIST_TOKEN:", lines)

    def test_driver_probe_reports_boolean_marker_only(self):
        source = (driver.ROOT / "scripts/run_container_integration.py").read_text()
        # The mount/token probe must never echo the runtime value.
        self.assertNotIn("echo TOKEN=${OPENLIST_TOKEN}", source)
        self.assertIn("TOKEN_SET", source)
        self.assertIn("TOKEN_MISSING", source)
        # The rendered compose is asserted to exclude the secret, not include it.
        self.assertIn("SECRET_MARKER not in nointerp.stdout", source)
        self.assertNotIn("SECRET_MARKER in rendered.stdout", source)


if __name__ == "__main__":
    unittest.main()
