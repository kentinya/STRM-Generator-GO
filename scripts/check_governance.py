#!/usr/bin/env python3
"""Check document identity and committed Slice/Task boundaries; not product acceptance."""
import argparse
import json
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
    if capability(committed) != capability(contract):
        raise ValueError("Working Slice contract differs from committed HEAD")
    if status != "PLANNED" and field(committed, "Status") == "PLANNED":
        raise ValueError("Activation must be committed in HEAD")
    if committed_roadmap != roadmap.strip():
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
        if capability(snapshot) != capability(contract):
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
