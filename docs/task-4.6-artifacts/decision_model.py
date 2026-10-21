#!/usr/bin/env python3
"""Task 4.6 only: a readable mirror of the locked plugin decision rules.

This mirrors ``ProbeDecisionService.Decide`` and ``StrmManifestReader`` of the
locked plugin source ``2afc6517fda5a7b4170ebea625619a552c47dabc``
(``artifacts/task-4.1/plugin-source``). It exists so each matrix row can be
checked against the exact branch the locked plugin would take, instead of the
row's expected reason being asserted by hand.

It never decides anything for the product: the observed behaviour always comes
from the plugin's own log in the real instance. This model only predicts which
branch a given (manifest, previous fingerprint) pair must take, so a row whose
observed behaviour comes from an unrelated condition is detected.
"""
from __future__ import annotations

import hashlib
import re

SHA1_RE = re.compile(r"^[0-9a-fA-F]{40}$")
VERSION = "1"
MARKERS = ("# strm-probe-version:", "# strm-probe-sha1:", "# strm-probe-size:")


class Manifest:
    """The fields of one STRM file the locked plugin actually reads."""

    def __init__(self, url: str, sha1: str | None, size: int | None, version: str = VERSION):
        self.url = url
        self.sha1 = sha1
        self.size = size
        self.version = version

    @property
    def url_hash(self) -> str:
        return hashlib.sha256(self.url.encode("utf-8")).hexdigest()

    @property
    def valid_sha1(self) -> bool:
        return bool(self.sha1) and bool(SHA1_RE.match(self.sha1))

    @property
    def valid_size(self) -> bool:
        return self.size is not None and self.size >= 0


class Fingerprint:
    """The plugin's held previous values for one item and source."""

    def __init__(self, url_hash: str, sha1: str | None, size: int | None):
        self.url_hash = url_hash
        self.sha1 = sha1
        self.size = size

    @property
    def valid_sha1(self) -> bool:
        return bool(self.sha1) and bool(SHA1_RE.match(self.sha1))

    @property
    def valid_size(self) -> bool:
        return self.size is not None and self.size >= 0


def parse_manifest(text: str) -> Manifest:
    """Read the same fields the locked StrmManifestReader reads."""
    version = None
    sha1 = None
    size = None
    url = None
    for raw_line in text.split("\n"):
        line = raw_line.replace("\t", "").replace("\r", "").strip()
        if not line:
            continue
        matched = False
        for marker, target in (MARKERS[0], "version"), (MARKERS[1], "sha1"), (MARKERS[2], "size"):
            if line.startswith(marker):
                value = line[len(marker):].strip()
                if target == "version":
                    version = value
                elif target == "sha1":
                    sha1 = value
                else:
                    size = int(value) if value.isdigit() else None
                matched = True
                break
        if matched or line.startswith("#"):
            continue
        if url is None:
            url = line
    if version != VERSION or not url:
        raise ValueError("unsupported or invalid STRM manifest")
    if sha1 is not None and not SHA1_RE.match(sha1):
        sha1 = None
    return Manifest(url, sha1.lower() if sha1 else None, size)


def decide(manifest: Manifest, previous: Fingerprint | None,
           has_primary_stream: bool, native_url: str | None) -> tuple[str, str]:
    """Return (kind, reason) exactly as the locked ProbeDecisionService does."""
    if not has_primary_stream:
        return "RequireProbe", "MissingPrimaryStream"
    if native_url != manifest.url:
        return "RequireProbe", "NativeUrlMismatch"
    if previous is None:
        return "TrustExisting", "FirstTrust"
    if previous.url_hash != manifest.url_hash:
        return "RequireProbe", "UrlChanged"
    if previous.valid_sha1 and manifest.valid_sha1:
        if previous.sha1.lower() == manifest.sha1.lower():
            return "SkipProbe", "Sha1Matches"
        return "RequireProbe", "Sha1Changed"
    if previous.valid_size and manifest.valid_size and previous.size != manifest.size:
        return "RequireProbe", "SizeChanged"
    if previous.valid_size and manifest.valid_size:
        return "SkipProbe", "Unchanged"
    return "SkipProbe", "InsufficientFingerprint"
