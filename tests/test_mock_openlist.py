import json
from pathlib import Path
import sys
import threading
import unittest
import urllib.error
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from mock_openlist import make_server, TOKEN


class FixtureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = make_server(port=0)
        cls.worker = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.worker.start()
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.worker.join(timeout=3)

    def request(self, path="/api/fs/list", body=None, token=TOKEN):
        payload = json.dumps(body if body is not None else {"path": "/Movies"}).encode()
        req = urllib.request.Request(self.base + path, data=payload,
                                     headers={"Authorization": token, "Content-Type": "application/json"})
        try:
            response = urllib.request.urlopen(req, timeout=3)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.load(response)

    def test_paginated_list_can_be_completed(self):
        items = []
        for page in (1, 2):
            status, response = self.request(body={"path": "/Movies", "page": page, "per_page": 2})
            self.assertEqual(status, 200)
            self.assertEqual(response["code"], 200)
            self.assertEqual(response["data"]["total"], 4)
            items.extend(response["data"]["content"])
        self.assertEqual(len({item["name"] for item in items}), 4)

    def test_application_error_is_http_success(self):
        status, response = self.request(body={"path": "/error"})
        self.assertEqual(status, 200)
        self.assertEqual(response["code"], 500)
        self.assertIsNone(response["data"])

    def test_transport_failure_differs_from_empty_directory(self):
        status, response = self.request(body={"path": "/offline"})
        self.assertEqual(status, 503)
        self.assertIsNone(response["data"])

    def test_wrong_token_does_not_expose_listing(self):
        _, response = self.request(token="wrong")
        self.assertEqual(response["code"], 401)
        self.assertIsNone(response["data"])

    def test_get_uses_realistic_hash_shape_and_unsafe_raw_url(self):
        _, response = self.request("/api/fs/get", {"path": "/Movies/Sample.mkv"})
        self.assertEqual(response["data"]["size"], 10737418240)
        self.assertEqual(len(response["data"]["hash_info"]["sha1"]), 40)
        self.assertIn("sign=", response["data"]["raw_url"])

    def test_missing_hash_is_null_not_fabricated(self):
        _, response = self.request("/api/fs/get", {"path": "/Movies/NoHash.mp4"})
        self.assertIsNone(response["data"]["hash_info"])

    def test_special_names_survive_json(self):
        _, response = self.request(body={"path": "/Movies/科幻"})
        self.assertEqual(response["data"]["content"][0]["name"], "A #100%.mkv")

    def test_invalid_pagination_and_body_are_rejected(self):
        for body in ({"path": "/Movies", "page": 0}, {"path": "/Movies", "per_page": -1}, []):
            with self.subTest(body=body):
                status, response = self.request(body=body)
                self.assertEqual(status, 400)
                self.assertEqual(response["code"], 400)
