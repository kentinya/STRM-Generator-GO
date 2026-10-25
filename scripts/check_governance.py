#!/usr/bin/env python3
"""Check document identity and committed Slice/Task boundaries; not product acceptance."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
SHA = re.compile(r"^[0-9a-f]{40}$")
LINK = re.compile(r"(?<!!)\[[^\]]+\]\((?:<([^>]+)>|([^\s)]+))\)")
SLICE_STATES = {"PLANNED", "ACTIVE", "READY FOR A REVIEW", "FIX REQUIRED", "PASS / CLOSED"}
TASK_STATES = {"PLANNED", "IN PROGRESS", "READY FOR B REVIEW", "FIX REQUIRED", "PASS"}


def field(text, key):
    values = re.findall(r"^" + re.escape(key) + r":\s*(.+)$", text, re.MULTILINE)
    if len(values) != 1:
        raise ValueError(f"Expected one {key} field")
    return values[0].strip()


def git(root, *args):
    result = subprocess.run(["git", "-C", str(root), *args], text=True, capture_output=True)
    if result.returncode:
        raise ValueError("Git boundary unavailable: " + " ".join(args[:2]))
    return result.stdout.strip()


def check_sha(root, value, label):
    if not SHA.fullmatch(value):
        raise ValueError(f"{label} must be a full commit SHA")
    git(root, "cat-file", "-e", value + "^{commit}")
    git(root, "merge-base", "--is-ancestor", value, "HEAD")


def capability(text):
    obligations = text.split("## Closure Packet")[0].strip()
    return re.sub(r"^(Status|Implementation Head):.*$", "", obligations, flags=re.MULTILINE)


def normalize_history_links(root, document, text):
    """Treat a pinned copy of the same committed document as its local link.

    Only this repository's reachable commits with identical HEAD blobs qualify;
    changing obligations, labels, anchors or referenced content still fails.
    """
    try:
        remote = git(root, "remote", "get-url", "origin").removesuffix(".git")
    except ValueError:
        return text
    if remote.startswith("git@github.com:"):
        remote = "https://github.com/" + remote.removeprefix("git@github.com:")
    if not re.fullmatch(r"https://github\.com/[^/]+/[^/]+", remote):
        return text
    prefix = remote + "/blob/"

    def replace(match):
        target = match.group(1) or match.group(2)
        if not target.startswith(prefix):
            return match.group(0)
        revision, slash, rest = target[len(prefix):].partition("/")
        path, fragment, anchor = rest.partition("#")
        path = unquote(path)
        if (not slash or not SHA.fullmatch(revision) or not path.endswith(".md")
                or Path(path).is_absolute() or ".." in Path(path).parts):
            raise ValueError("History link must pin a repository Markdown file")
        check_sha(root, revision, "History revision")
        pinned_blob = git(root, "rev-parse", revision + ":" + path)
        try:
            current_blob = git(root, "rev-parse", "HEAD:" + path)
        except ValueError:
            # Once removal is committed, retain the full URL as contract identity.
            return match.group(0)
        if pinned_blob != current_blob:
            raise ValueError("History link content differs from committed HEAD: " + path)
        local = os.path.relpath(root / path, (root / document).parent)
        value = local + fragment + anchor
        # Preserve the original label and angle-bracket style.
        start, end = match.span(1 if match.group(1) is not None else 2)
        return match.group(0)[:start - match.start()] + value + match.group(0)[end - match.start():]

    return LINK.sub(replace, text)


def validate(root=ROOT, bootstrap=False):
    spec = json.loads((root / "dev/environment-spec.json").read_text())
    required = spec["required_files"]
    if len(required) != len(set(required)):
        raise ValueError("Duplicate required file")
    for path in required:
        item = root / path
        if not item.is_file() or item.is_symlink() or not item.resolve().is_relative_to(root.resolve()):
            raise ValueError("Required file missing or unsafe: " + path)
    version = spec["go"]["version"]
    if (root / ".go-version").read_text().strip() != version:
        raise ValueError("Go SDK pin mismatch")
    if not re.search(r"^go " + re.escape(version) + "$", (root / "go.mod").read_text(), re.MULTILINE):
        raise ValueError("go.mod version mismatch")
    for path in required:
        if not path.endswith(".md"):
            continue
        for match in LINK.finditer((root / path).read_text()):
            target = (match.group(1) or match.group(2)).split("#", 1)[0]
            if not target or re.match(r"[a-zA-Z][a-zA-Z0-9+.-]*:", target):
                continue
            destination = ((root / path).parent / unquote(target)).resolve()
            if not destination.is_relative_to(root.resolve()) or not destination.exists():
                raise ValueError(f"Broken or escaping Markdown link: {path} -> {target}")

    contract = (root / "SLICE.md").read_text()
    task = (root / "TASK.md").read_text()
    roadmap = (root / "docs/roadmap.md").read_text()
    slice_id = field(contract, "Slice ID")
    status = field(contract, "Status")
    if status not in SLICE_STATES:
        raise ValueError("Invalid Slice status")
    rows = re.findall(r"^\|\s*" + re.escape(slice_id) + r"\s*\|[^|]+\|\s*([^|]+?)\s*\|", roadmap, re.MULTILINE)
    if rows != [status]:
        raise ValueError("Roadmap status does not match current Slice")
    active_rows = re.findall(r"^\|\s*(\d+)\s*\|[^|]+\|\s*(?:ACTIVE|FIX REQUIRED|READY FOR A REVIEW)\s*\|", roadmap, re.MULTILINE)
    if active_rows and active_rows != [slice_id]:
        raise ValueError("Roadmap has a different or additional active Slice")
    if status == "PLANNED" and active_rows:
        raise ValueError("PLANNED current Slice cannot coexist with active roadmap entries")
    inactive = task.splitlines()[0] == "# NO ACTIVE IMPLEMENTATION TASK"
    base = field(contract, "Base SHA")
    implementation = field(contract, "Implementation Head")
    if bootstrap:
        if status != "PLANNED" or base != "NOT SET" or implementation != "NOT SET" or not inactive:
            raise ValueError("Bootstrap requires PLANNED / NOT SET / no active Task")
        return "PASS: preparation governance (not implementation authorization)"

    git(root, "rev-parse", "--verify", "HEAD")
    committed = git(root, "show", "HEAD:SLICE.md")
    committed_roadmap = git(root, "show", "HEAD:docs/roadmap.md")
    # Review/report sections may change; capability obligations must already be committed.
    normalized_contract = normalize_history_links(root, "SLICE.md", contract)
    if capability(normalize_history_links(root, "SLICE.md", committed)) != capability(normalized_contract):
        raise ValueError("Working Slice contract differs from committed HEAD")
    if status != "PLANNED" and field(committed, "Status") == "PLANNED":
        raise ValueError("Activation must be committed in HEAD")
    if (normalize_history_links(root, "docs/roadmap.md", committed_roadmap)
            != normalize_history_links(root, "docs/roadmap.md", roadmap).strip()):
        raise ValueError("Working roadmap differs from committed HEAD")
    if status == "PLANNED":
        if not inactive or implementation != "NOT SET":
            raise ValueError("PLANNED Slice cannot carry implementation or active Task")
        if base != "NOT SET":
            check_sha(root, base, "Slice Base")
        return "PASS: committed planning governance (no active implementation)"
    check_sha(root, base, "Slice Base")
    if implementation != "NOT SET":
        check_sha(root, implementation, "Implementation Head")
        git(root, "merge-base", "--is-ancestor", base, implementation)
    if status in {"READY FOR A REVIEW", "PASS / CLOSED"} and implementation == "NOT SET":
        raise ValueError("Review or closure requires an implementation SHA")
    if status == "PASS / CLOSED":
        if not inactive or field(contract, "Decision") != "PASS":
            raise ValueError("Closed Slice requires A PASS and no active Task")
    if not inactive:
        task_status = field(task, "Status")
        if status not in {"ACTIVE", "FIX REQUIRED"} and not (status == "READY FOR A REVIEW" and task_status == "PASS"):
            raise ValueError("Active Task requires an ACTIVE / FIX REQUIRED Slice")
        if (field(task, "Parent Slice") != slice_id
                or not re.fullmatch(re.escape(slice_id) + r"\.[1-9]\d*", field(task, "Task ID"))):
            raise ValueError("Task parent mismatch")
        if task_status not in TASK_STATES:
            raise ValueError("Invalid Task status")
        if field(task, "Test Level") not in {"T0", "T1", "T2", "T3", "T4"}:
            raise ValueError("Invalid Task test level")
        if field(task, "Difficulty") not in {"Low", "Medium", "High"}:
            raise ValueError("Invalid Task difficulty")
        task_base = field(task, "Task Base")
        check_sha(root, task_base, "Task Base")
        git(root, "merge-base", "--is-ancestor", base, task_base)
        checkpoint = field(task, "Contract Checkpoint")
        check_sha(root, checkpoint, "Contract Checkpoint")
        snapshot = git(root, "show", checkpoint + ":SLICE.md")
        if field(snapshot, "Status") != "ACTIVE" or field(snapshot, "Base SHA") != base:
            raise ValueError("Task checkpoint must identify the active contract")
        if capability(normalize_history_links(root, "SLICE.md", snapshot)) != capability(normalized_contract):
            raise ValueError("Task checkpoint does not match current capability contract")
    return "PASS: committed contract governance (not product or review acceptance)"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bootstrap", action="store_true")
    args = parser.parse_args()
    try:
        print(validate(bootstrap=args.bootstrap))
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
