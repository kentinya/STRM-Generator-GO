"""Local Bot API stand-in for integration drivers.

This module exists only for isolated test drivers that exercise binaries
built with the strmgen_testhooks tag. It records the raw requests a product
process sends to the notification transport so the driver can assert on the
exact endpoint, chat id and message bodies. It never contacts the real
Telegram network and never handles real credentials.
"""
import html
import json
import re
import threading
import urllib.parse
from email.parser import BytesParser
from email.policy import default as email_default_policy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class TelegramReceiver:
    """One receiver per scenario. Requests are appended under a lock."""

    def __init__(self, host="127.0.0.1"):
        self._lock = threading.Lock()
        self.requests = []
        # One-shot hold: the next request is accepted and recorded but its
        # response is delayed until release_held(), so drivers can observe a
        # product process while its notification send is blocked.
        self._hold_next = False
        self._release = threading.Event()
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def read_body(self):
                length = self.headers.get("Content-Length")
                if length is not None:
                    return self.rfile.read(int(length))
                # Go streams an unknown-length multipart body with chunked
                # transfer encoding; decode it manually.
                chunks = []
                while True:
                    size_line = self.rfile.readline().strip()
                    if b";" in size_line:
                        size_line = size_line.split(b";", 1)[0]
                    size = int(size_line, 16)
                    if size == 0:
                        self.rfile.readline()
                        break
                    chunks.append(self.rfile.read(size))
                    self.rfile.readline()
                return b"".join(chunks)

            def do_POST(self):
                body = self.read_body()
                content_type = self.headers.get("Content-Type", "")
                record = {"path": self.path, "multipart": content_type.startswith("multipart/"),
                          "content_type": content_type, "raw_length": len(body)}
                if record["multipart"]:
                    message = BytesParser(policy=email_default_policy).parsebytes(
                        b"Content-Type: " + content_type.encode() + b"\r\n\r\n" + body)
                    for part in message.iter_parts():
                        name = part.get_param("name", header="content-disposition")
                        payload = part.get_payload(decode=True) or b""
                        if name == "chat_id":
                            record["chat_id"] = payload.decode("utf-8", "replace")
                        elif name == "document":
                            record["document"] = payload.decode("utf-8", "replace")
                else:
                    values = urllib.parse.parse_qs(body.decode("utf-8", "replace"))
                    record["chat_id"] = values.get("chat_id", [""])[0]
                    record["text"] = values.get("text", [""])[0]
                    record["parse_mode"] = values.get("parse_mode", [""])[0]
                with outer._lock:
                    outer.requests.append(record)
                    hold = outer._hold_next
                    if hold:
                        outer._hold_next = False
                        outer._release.clear()
                        record["held"] = True
                if hold:
                    outer._release.wait(timeout=60)
                payload = json.dumps({"ok": True, "result": {"message_id": 1}}).encode()
                try:
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(payload)))
                    self.end_headers()
                    self.wfile.write(payload)
                except (BrokenPipeError, ConnectionResetError):
                    # The client may be gone (for example cancelled during a
                    # held send); the request stays recorded.
                    pass

            def log_message(self, *args):
                pass

        self._server = ThreadingHTTPServer((host, 0), Handler)
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)
        self._thread.start()

    def hold_next_request(self):
        """Delay the response of the next request until release_held."""
        with self._lock:
            self._hold_next = True
            self._release.clear()

    def release_held(self):
        self._release.set()

    def held_requests(self):
        with self._lock:
            return [r for r in self.requests if r.get("held")]

    @property
    def url(self):
        return f"http://127.0.0.1:{self._server.server_port}"

    def _all(self):
        with self._lock:
            return list(self.requests)

    def count(self):
        return len(self._all())

    def messages(self):
        return [r for r in self._all() if not r["multipart"]]

    def documents(self):
        return [r for r in self._all() if r["multipart"]]

    def close(self):
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=5)


def telegram_env(receiver):
    """The environment every notification run shares with the receiver."""
    return {
        "TELEGRAM_BOT_TOKEN": "integration-token",
        "TELEGRAM_CHAT_ID": "-10042",
        "STRMGEN_TEST_TELEGRAM_API_BASE": receiver.url,
    }


def notify_temp_dirs():
    """Current strmgen notification directories in the system temp area."""
    import tempfile
    from pathlib import Path
    root = Path(tempfile.gettempdir())
    return {path.name for path in root.glob("strmgen-notify-*")}


_CONTROLLED_TAGS = re.compile(r"</?(?:b|code|blockquote(?:\s+expandable)?)>")


def visible_text(text: str) -> str:
    """Read the controlled HTML summary as user-visible text.

    This is a test reader for assertions, not a Telegram renderer. It strips
    only the tags the product is allowed to emit and decodes their entities.
    """
    return html.unescape(_CONTROLLED_TAGS.sub("", text))
