#!/usr/bin/env python3
"""Synthetic OpenList list/get fixture; never connects to external services."""
import argparse
import copy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import posixpath

ROOT = Path(__file__).resolve().parents[1]
TOKEN = "strmgen-fixture-only"  # Public test value, not a secret.


def load_fixture():
    return json.loads((ROOT / "dev/fixtures/openlist.json").read_text())


def make_server(host="127.0.0.1", port=15244, fixture=None, hooks=None):
    directories = (fixture or load_fixture())["directories"]
    hooks = hooks or {}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass  # Do not log request paths, bodies or Authorization.

        def respond(self, code, data=None, status=200):
            body = json.dumps({"code": code, "message": "success" if code == 200 else "fixture error",
                               "data": data}, ensure_ascii=False).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try:
                self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError):
                pass  # A cancelled client may close the socket early.

        def do_GET(self):
            self.respond(200 if self.path == "/health" else 404)

        def do_POST(self):
            if self.headers.get("Authorization") != TOKEN:
                self.respond(401)
                return
            if self.path not in ("/api/fs/list", "/api/fs/get"):
                self.respond(404)
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= 16 * 1024:
                    raise ValueError("body size")
                request = json.loads(self.rfile.read(length))
                path = request["path"]
                if not isinstance(path, str) or not path.startswith("/"):
                    raise ValueError("path")
            except (ValueError, KeyError, TypeError):
                self.respond(400, status=400)
                return
            if path in ("/error", "/forbidden", "/offline"):
                code = {"/error": 500, "/forbidden": 403, "/offline": 503}[path]
                self.respond(code, status=503 if path == "/offline" else 200)
                return
            before = hooks.get("before")
            if before is not None:
                before(self.path, path, request)
            override = hooks.get("override")
            if override is not None:
                custom = override(self.path, path, request, directories)
                if custom is not None:
                    status, code, data = custom
                    self.respond(code, data, status=status)
                    return
            if self.path.endswith("/list"):
                if path not in directories:
                    self.respond(404)
                    return
                try:
                    page = request.get("page", 1)
                    per_page = request.get("per_page", 100)
                    if (type(page) is not int or type(per_page) is not int
                            or page < 1 or per_page < 1 or per_page > 1000):
                        raise ValueError("pagination")
                except (ValueError, TypeError):
                    self.respond(400, status=400)
                    return
                items = directories[path]
                start = (page - 1) * per_page
                self.respond(200, {"content": copy.deepcopy(items[start:start + per_page]),
                                   "total": len(items), "provider": "fixture", "write": False})
                return
            parent, name = posixpath.split(path)
            obj = next((copy.deepcopy(item) for item in directories.get(parent or "/", [])
                        if item["name"] == name), None)
            if obj is None and path in directories:
                obj = {"name": name or "/", "is_dir": True, "size": 0, "hash_info": None}
            if obj is None:
                self.respond(404)
                return
            # Deliberately unsuitable as STRM playback URL; client must not copy it.
            obj["raw_url"] = "https://fixture.example.invalid/temporary?sign=fixture-only"
            self.respond(200, obj)

    return ThreadingHTTPServer((host, port), Handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", choices=("127.0.0.1", "0.0.0.0"), default="127.0.0.1")
    parser.add_argument("--port", type=int, default=15244)
    args = parser.parse_args()
    with make_server(args.host, args.port) as server:
        print(f"OpenList fixture listening on {args.host}:{server.server_port}", flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
