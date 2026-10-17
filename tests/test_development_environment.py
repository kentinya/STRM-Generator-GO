import io
import json
from pathlib import Path
import stat
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
import development_environment as env


class ArchiveSafetyTests(unittest.TestCase):
    def test_tar_rejects_escape_and_links(self):
        for name, kind in (("go/../../escape", tarfile.REGTYPE),
                           ("/escape", tarfile.REGTYPE), ("go/bin/go", tarfile.SYMTYPE)):
            with self.subTest(name=name, kind=kind), tempfile.TemporaryDirectory() as tmp:
                archive = Path(tmp) / "sdk.tar.gz"
                with tarfile.open(archive, "w:gz") as bundle:
                    member = tarfile.TarInfo(name)
                    member.type = kind
                    member.linkname = "/etc/passwd" if kind == tarfile.SYMTYPE else ""
                    bundle.addfile(member, io.BytesIO(b""))
                with self.assertRaises(ValueError):
                    env.extract_sdk(archive, Path(tmp) / "out", {})

    def test_zip_rejects_traversal_symlink_and_wrong_prefix(self):
        for name, mode in (("module/../escape", 0), ("other/bin/go", 0),
                           ("module/bin/go", stat.S_IFLNK | 0o777)):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                archive = Path(tmp) / "sdk.zip"
                with zipfile.ZipFile(archive, "w") as bundle:
                    member = zipfile.ZipInfo(name)
                    member.external_attr = mode << 16
                    bundle.writestr(member, "unsafe")
                with self.assertRaises(ValueError):
                    env.extract_sdk(archive, Path(tmp) / "out", {"format": "zip", "prefix": "module"})

    def test_zip_makes_go_tools_executable(self):
        with tempfile.TemporaryDirectory() as tmp:
            archive = Path(tmp) / "sdk.zip"
            with zipfile.ZipFile(archive, "w") as bundle:
                bundle.writestr("module/bin/go", "fixture")
                bundle.writestr("module/pkg/tool/linux-amd64/compile", "fixture")
                bundle.writestr("module/VERSION", "fixture")
            target = Path(tmp) / "out"
            env.extract_sdk(archive, target, {"format": "zip", "prefix": "module"})
            self.assertTrue((target / "go/bin/go").stat().st_mode & 0o111)
            self.assertTrue((target / "go/pkg/tool/linux-amd64/compile").stat().st_mode & 0o111)
            self.assertFalse((target / "go/VERSION").stat().st_mode & 0o111)

    def test_wrong_checksum_never_extracts(self):
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            archive = base / "bad.zip"
            archive.write_bytes(b"not a real archive")
            with patch.object(env, "BASE", base), patch.object(env, "SDK", base / "sdk/go"), \
                    patch.object(env, "host_key", return_value="linux-amd64"), \
                    patch.object(env, "extract_sdk") as extract:
                with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
                    env.install(str(archive))
                extract.assert_not_called()

    def test_toolchain_and_caches_are_local(self):
        settings = env.go_environment()
        for key in ("GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE"):
            self.assertTrue(Path(settings[key]).is_relative_to(env.BASE))
        self.assertEqual(settings["GOTOOLCHAIN"], "local")
        self.assertEqual(settings["GOENV"], "off")

    def test_known_host_mapping(self):
        with patch.object(env.platform, "system", return_value="Linux"), \
                patch.object(env.platform, "machine", return_value="aarch64"):
            self.assertEqual(env.host_key(), "linux-arm64")

    def test_unsupported_host_refused(self):
        with patch.object(env.platform, "system", return_value="Windows"):
            with self.assertRaises(ValueError):
                env.host_key()
