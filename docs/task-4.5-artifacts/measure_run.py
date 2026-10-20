#!/usr/bin/env python3
"""Measure one strmgen invocation with Linux wait4 rusage.

Runs the release binary as a child process, captures monotonic wall clock and
the child's peak RSS (ru_maxrss, KiB) plus the exit code. The token is passed
only through the child environment; it never appears in argv or in this tool's
output. No secret value is written anywhere.

Usage:
  OPENLIST_TOKEN=... python3 measure_run.py --label first-sync -- \
      artifacts/task-4.5/bin/strmgen-amd64 sync --config artifacts/task-4.5/strmgen.yaml
"""
from __future__ import annotations

import argparse
import json
import os
import resource
import subprocess
import sys
import time


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--label", required=True)
    parser.add_argument("--output", default=None, help="JSON result path")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args(argv)
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        print("no command supplied", file=sys.stderr)
        return 2

    started = time.monotonic()
    proc = subprocess.Popen(command, env=os.environ.copy())
    pid, status, usage = os.wait4(proc.pid, 0)
    elapsed = time.monotonic() - started
    if os.WIFEXITED(status):
        exit_code = os.WEXITSTATUS(status)
        signal = None
    else:
        exit_code = 128 + os.WTERMSIG(status)
        signal = os.WTERMSIG(status)

    result = {
        "label": args.label,
        "argv": command,
        "exit_code": exit_code,
        "signal": signal,
        "wall_seconds": round(elapsed, 3),
        "maxrss_kib": usage.ru_maxrss,
        "maxrss_unit": "KiB (Linux ru_maxrss)",
        "wall_source": "time.monotonic around os.wait4",
        "rss_source": "resource.struct_rusage.ru_maxrss via os.wait4",
        "token_source": "OPENLIST_TOKEN environment only; not in argv or output",
    }
    text = json.dumps(result, ensure_ascii=False, indent=2)
    print(text)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as handle:
            handle.write(text + "\n")
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
