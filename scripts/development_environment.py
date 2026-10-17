#!/usr/bin/env python3
"""Repository-local, checksummed Go SDK; no system or production changes."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / "artifacts" / "dev-env"
SPEC = json.loads((ROOT / "dev/environment-spec.json").read_text())
SDK = BASE / "sdk" / ("go" + SPEC["go"]["version"])


def host_key():
    arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    key = f"{platform.system().lower()}-{arch}"
    if key not in SPEC["go"]["archives"]:
        raise ValueError("Only Linux amd64/arm64 SDK installation is configured")
    return key


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def sdk_version(path):
    result = subprocess.run([str(path / "bin/go"), "version"], text=True,
                            capture_output=True, check=True)
    if result.stdout.split()[2] != "go" + SPEC["go"]["version"]:
        raise ValueError("SDK version does not match environment-spec.json")
    return result.stdout.strip()


def extract_sdk(archive, destination, entry):
    if entry.get("format") == "zip":
        prefix = entry["prefix"] + "/"
        with zipfile.ZipFile(archive) as bundle:
            for member in bundle.infolist():
                if not member.filename.startswith(prefix):
                    raise ValueError("Unsafe SDK archive prefix")
                relative = Path(member.filename[len(prefix):])
                mode = member.external_attr >> 16
                if (relative.is_absolute() or ".." in relative.parts
                        or stat.S_ISLNK(mode) or "\\" in member.filename):
                    raise ValueError("Unsafe SDK archive member")
                target = destination / "go" / relative
                if member.is_dir():
                    target.mkdir(parents=True, exist_ok=True)
                    continue
                target.parent.mkdir(parents=True, exist_ok=True)
                with bundle.open(member) as source, target.open("wb") as output:
                    shutil.copyfileobj(source, output)
                executable = (mode & 0o111 or relative.parts[0] == "bin"
                              or relative.parts[:2] == ("pkg", "tool"))
                target.chmod(0o755 if executable else 0o644)
        return
    with tarfile.open(archive, "r:gz") as bundle:
        for member in bundle.getmembers():
            parts = Path(member.name).parts
            if (not parts or parts[0] != "go" or ".." in parts
                    or Path(member.name).is_absolute()
                    or not (member.isfile() or member.isdir())):
                raise ValueError("Unsafe SDK archive member")
        bundle.extractall(destination, filter="data")


def install(offline_archive=None):
    entry = SPEC["go"]["archives"][host_key()]
    # Keep downloaded SDK and smoke sources out of future root-module ./... gates.
    BASE.mkdir(parents=True, exist_ok=True)
    isolation = BASE / "go.mod"
    if not isolation.exists():
        isolation.write_text("module strmgen-development-artifacts\n\ngo " + SPEC["go"]["version"] + "\n")
    if SDK.exists():
        print(sdk_version(SDK))
        return
    downloads = BASE / "downloads"
    downloads.mkdir(parents=True, exist_ok=True)
    archive = Path(offline_archive).resolve() if offline_archive else downloads / entry["url"].rsplit("/", 1)[1]
    if not archive.exists():
        if offline_archive:
            raise ValueError("Offline SDK archive not found")
        print("Downloading pinned Go SDK", flush=True)
        partial = archive.with_suffix(archive.suffix + ".partial")
        try:
            with urllib.request.urlopen(entry["url"], timeout=60) as response, partial.open("wb") as target:
                shutil.copyfileobj(response, target)
            if sha256(partial) != entry["sha256"]:
                raise ValueError("SDK SHA-256 mismatch")
            partial.replace(archive)
        finally:
            partial.unlink(missing_ok=True)
    if sha256(archive) != entry["sha256"]:
        raise ValueError("SDK SHA-256 mismatch; installation refused")
    SDK.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="install-", dir=SDK.parent) as tmp:
        extract_sdk(archive, Path(tmp), entry)
        print(sdk_version(Path(tmp) / "go"))
        (Path(tmp) / "go").rename(SDK)
    print("PASS: checksum and SDK installation")


def go_environment():
    env = os.environ.copy()
    env.update({"GOTOOLCHAIN": "local", "GOENV": "off", "GOWORK": "off",
                "GOROOT": str(SDK), "GOPATH": str(BASE / "gopath"),
                "GOCACHE": str(BASE / "go-build"), "GOMODCACHE": str(BASE / "gopath/pkg/mod"),
                "CGO_ENABLED": "0"})
    return env


def smoke():
    print(sdk_version(SDK), flush=True)
    # This file is a compiler fixture, deliberately outside product packages.
    directory = BASE / "build-smoke"
    directory.mkdir(parents=True, exist_ok=True)
    source = directory / "main.go"
    source.write_text('''package main

import (
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
)

func main() {
    var v struct { Size int64 `json:"size"` }
    if err := json.Unmarshal([]byte(`{"size":10737418240}`), &v); err != nil { panic(err) }
    if v.Size != 10737418240 { panic("int64 mismatch") }
    if _, err := http.NewRequest(http.MethodGet, "https://example.invalid/d/a.mkv", nil); err != nil { panic(err) }
    _ = slog.Default()
    fmt.Println("PASS: Go compiler and standard library smoke")
}
''')
    binary = SDK / "bin/go"
    subprocess.run([str(binary), "run", str(source)], cwd=ROOT, env=go_environment(), check=True)
    for arch in ("amd64", "arm64"):
        env = go_environment() | {"GOOS": "linux", "GOARCH": arch}
        subprocess.run([str(binary), "build", "-trimpath", "-o", str(directory / ("smoke-" + arch)),
                        str(source)], cwd=ROOT, env=env, check=True)
        print(f"PASS: linux/{arch} compiler smoke (not a product build)", flush=True)


def doctor():
    checks = []

    def check(label, action):
        try:
            value = action()
            print(f"PASS: {label}: {value}")
            checks.append(True)
        except (OSError, ValueError, subprocess.SubprocessError) as error:
            print(f"FAIL: {label}: {type(error).__name__}")
            checks.append(False)

    check("Python >= 3.11", lambda: sys.version.split()[0] if sys.version_info >= (3, 11)
          else (_ for _ in ()).throw(ValueError("Python version")))
    check("configured host", host_key)
    check("pinned Go SDK", lambda: sdk_version(SDK))
    check("Git", lambda: subprocess.check_output(["git", "--version"], text=True).strip())
    check("original V4.3 identity", lambda: "unchanged" if sha256(ROOT / SPEC["plan"]["path"]) == SPEC["plan"]["sha256"]
          else (_ for _ in ()).throw(ValueError("Original plan changed")))
    if shutil.which("docker"):
        for args in (["docker", "compose", "version"], ["docker", "version", "--format", "{{.Server.Version}}"]):
            result = subprocess.run(args, text=True, capture_output=True)
            print(("PASS" if result.returncode == 0 else "OPTIONAL UNAVAILABLE") + ": " + " ".join(args))
    else:
        print("OPTIONAL UNAVAILABLE: Docker / Compose (native fixture needs only Python)")
    print("Product / real OpenList / Jellyfin acceptance: NOT RUN")
    return 0 if all(checks) else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("install", "doctor", "smoke"))
    parser.add_argument("--sdk-archive", help="Offline official archive, checked against the same SHA-256")
    args = parser.parse_args()
    try:
        if args.action == "install":
            install(args.sdk_archive)
        elif args.action == "smoke":
            smoke()
        else:
            return doctor()
    except (OSError, ValueError, tarfile.TarError, zipfile.BadZipFile, subprocess.SubprocessError) as error:
        print(f"FAIL: {type(error).__name__}: environment action failed", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
