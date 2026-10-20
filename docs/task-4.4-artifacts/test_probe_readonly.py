#!/usr/bin/env python3
"""Direct behavior tests for the Task 4.4 read-only OpenList probe.

These tests stub only the external HTTP I/O and use temporary local fixtures
under the task-owned artifacts tree. They verify the probe's failure decisions
(non-zero exit on bad Get/type/Size/bytes, incomplete pagination, manifest
mismatch) and its recursive set comparison. They are NOT real OpenList/S3
evidence. Run explicitly:

    python3 -m unittest discover -s artifacts/task-4.4/tests -p 'test_probe*.py' -v
"""
from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
TOOL = REPO_ROOT / "artifacts" / "task-4.4" / "tools" / "openlist_readonly_probe.py"
TMP_ROOT = REPO_ROOT / "artifacts" / "task-4.4" / "tests" / "tmp"

_spec = importlib.util.spec_from_file_location("task44_openlist_probe", TOOL)
probe = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(probe)


class FakeServer:
    """Minimal recursive OpenList stand-in used only for local logic tests."""

    def __init__(self, tree, gets=None, payloads=None, list_pages=None):
        # tree: {dir_rel: [entry, ...]}; entry {"name", "is_dir", "size"}
        self.tree = tree
        self.gets = gets or {}
        self.payloads = payloads or {}
        self.list_pages = list_pages or {}

    def post(self, base, endpoint, payload):
        if endpoint.endswith("/list"):
            path = payload["path"]
            rel = path[len("/owned"):].lstrip("/")
            pages = self.list_pages.get(rel)
            if pages is not None:
                page = payload["page"]
                if page > len(pages):
                    return 200, {"code": 200, "data": {"total": sum(len(p) for p in pages), "content": []}}
                return 200, {"code": 200, "data": {
                    "total": sum(len(p) for p in pages), "content": pages[page - 1]}}
            entries = self.tree.get(rel, [])
            return 200, {"code": 200, "data": {"total": len(entries), "content": entries}}
        if endpoint.endswith("/get"):
            path = payload["path"]
            if path == "/owned":
                return 200, {"code": 200, "data": {"is_dir": True}}
            rel = path[len("/owned"):].lstrip("/")
            return 200, {"code": 200, "data": self.gets.get(rel, {"is_dir": True})}
        raise AssertionError(endpoint)

    def get(self, url):
        name = url.rsplit("/", 1)[-1]
        import urllib.parse
        name = urllib.parse.unquote(name)
        if name not in self.payloads:
            raise FileNotFoundError(name)
        return 200, self.payloads[name]


class ProbeTestCase(unittest.TestCase):
    def setUp(self):
        TMP_ROOT.mkdir(parents=True, exist_ok=True)
        self._tmp = tempfile.TemporaryDirectory(dir=TMP_ROOT)
        self.base = Path(self._tmp.name)
        self.media = self.base / "media"
        self.media.mkdir()
        self.manifest = self.base / "dataset" / "manifest.jsonl"

    def tearDown(self):
        self._tmp.cleanup()
        try:
            TMP_ROOT.rmdir()
        except OSError:
            pass

    def install(self, server):
        self._saved = (probe.post_json, probe.get_bytes)
        probe.post_json = server.post
        probe.get_bytes = server.get

    def restore(self):
        probe.post_json, probe.get_bytes = self._saved

    def run_probe(self, *extra):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = probe.main([
                "--base-url", "http://fixture.invalid",
                "--source-root", "/owned",
                "--media-dir", str(self.media),
                *extra,
            ])
        return code, out.getvalue()

    def write_media(self, rel, data):
        path = self.media / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return hashlib.sha256(data).hexdigest()

    def write_manifest(self, rows):
        self.manifest.parent.mkdir(parents=True, exist_ok=True)
        self.manifest.write_text(
            "".join(json.dumps(r, separators=(",", ":")) + "\n" for r in rows)
        )


class TestFailures(ProbeTestCase):
    def test_bad_sample_get_is_nonzero(self):
        sha = self.write_media("sample.mkv", b"correct-media")
        server = FakeServer(
            {"": [{"name": "sample.mkv", "is_dir": False, "size": 13}]},
            gets={"sample.mkv": {"is_dir": False, "size": 13}},
            payloads={"sample.mkv": b"wrong-media"},
        )
        self.install(server)
        try:
            code, out = self.run_probe("--sample", "sample.mkv")
        finally:
            self.restore()
        self.assertNotEqual(code, 0)
        self.assertIn("mismatch", out)

    def test_sample_get_directory_is_nonzero(self):
        self.write_media("sample.mkv", b"correct-media")
        server = FakeServer(
            {"": [{"name": "sample.mkv", "is_dir": False, "size": 13}]},
            gets={"sample.mkv": {"is_dir": True}},
            payloads={"sample.mkv": b"correct-media"},
        )
        self.install(server)
        try:
            code, _out = self.run_probe("--sample", "sample.mkv")
        finally:
            self.restore()
        self.assertNotEqual(code, 0)

    def test_sample_size_mismatch_is_nonzero(self):
        self.write_media("sample.mkv", b"correct-media")
        server = FakeServer(
            {"": [{"name": "sample.mkv", "is_dir": False, "size": 13}]},
            gets={"sample.mkv": {"is_dir": False, "size": 99}},
            payloads={"sample.mkv": b"correct-media"},
        )
        self.install(server)
        try:
            code, _out = self.run_probe("--sample", "sample.mkv")
        finally:
            self.restore()
        self.assertNotEqual(code, 0)

    def test_incomplete_pagination_is_nonzero(self):
        self.write_media("a.mkv", b"aaaa")
        server = FakeServer(
            {"": [{"name": "a.mkv", "is_dir": False, "size": 4}]},
            list_pages={"": [[{"name": "a.mkv", "is_dir": False, "size": 4}]]},
        )
        # Force a wrong total by declaring two entries but returning one.
        server.tree[""] = [{"name": "a.mkv", "is_dir": False, "size": 4}]
        server.list_pages = {}
        orig_post = server.post

        def bad_post(base, endpoint, payload):
            if endpoint.endswith("/list"):
                return 200, {"code": 200, "data": {"total": 2, "content": [
                    {"name": "a.mkv", "is_dir": False, "size": 4}]}}
            return orig_post(base, endpoint, payload)

        server.post = bad_post
        self.install(server)
        try:
            code, _out = self.run_probe()
        finally:
            self.restore()
        self.assertNotEqual(code, 0)

    def test_manifest_mismatch_is_nonzero(self):
        self.write_media("a.mkv", b"aaaa")
        self.write_manifest([{"path": "a.mkv", "size": 4}])
        server = FakeServer(
            {"": [{"name": "a.mkv", "is_dir": False, "size": 4},
                  {"name": "extra.mkv", "is_dir": False, "size": 5}]},
        )
        self.install(server)
        try:
            code, out = self.run_probe("--manifest", str(self.manifest))
        finally:
            self.restore()
        self.assertNotEqual(code, 0)
        self.assertIn("unplanned", out)


class TestSuccess(ProbeTestCase):
    def test_recursive_manifest_and_special_names_match(self):
        names = ["Plain Movie.mkv", "hash#name.mkv", "percent%name.mkv",
                 'quote"name.mkv', "中文电影.mkv"]
        entries = []
        payloads = {}
        rows = []
        for name in names:
            data = ("data-" + name).encode("utf-8")
            sha = self.write_media(name, data)
            entries.append({"name": name, "is_dir": False, "size": len(data)})
            payloads[name] = data
            rows.append({"path": name, "size": len(data), "sha256": sha})
        # A nested bulk/ directory must be recursed into and matched.
        bulk = [{"name": "000001.mkv", "is_dir": False, "size": 4}]
        self.write_media("bulk/000001.mkv", b"bulk")
        payloads["000001.mkv"] = b"bulk"
        rows.append({"path": "bulk/000001.mkv", "size": 4})
        self.write_manifest(rows)
        server = FakeServer(
            {"": entries + [{"name": "bulk", "is_dir": True, "size": 0}],
             "bulk": bulk},
            gets={name: {"is_dir": False, "size": len(payloads[name])} for name in names}
                 | {"bulk/000001.mkv": {"is_dir": False, "size": 4}},
            payloads=payloads,
        )
        self.install(server)
        try:
            code, out = self.run_probe(
                "--manifest", str(self.manifest),
                *[arg for name in names for arg in ("--sample", name)],
                "--sample", "bulk/000001.mkv",
            )
        finally:
            self.restore()
        self.assertEqual(code, 0, out)
        report = json.loads(out)
        self.assertEqual(report["checks"]["remote_regular_files"], 6)
        self.assertEqual(report["checks"]["manifest_matched"], 6)
        self.assertTrue(all(s["sha256_match"] for s in report["checks"]["samples"]))


if __name__ == "__main__":
    unittest.main()
