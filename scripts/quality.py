#!/usr/bin/env python3
"""Current tooling checks, plus real Go gates once product packages exist."""
import argparse
import ast
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def run(args):
    print("+ " + " ".join(args), flush=True)
    subprocess.run(args, cwd=ROOT, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bootstrap", action="store_true")
    args = parser.parse_args()
    try:
        run([sys.executable, "scripts/check_governance.py"] + (["--bootstrap"] if args.bootstrap else []))
        run([sys.executable, "-m", "unittest", "discover", "-s", "tests", "-v"])
        for source in [*ROOT.joinpath("scripts").glob("*.py"), *ROOT.joinpath("tests").glob("*.py")]:
            ast.parse(source.read_text(), filename=str(source))
        run(["bash", "-n", "scripts/go-local"])
        run(["git", "diff", "--check"])
        spec = json.loads((ROOT / "dev/environment-spec.json").read_text())
        for name in spec["required_files"]:
            patch = subprocess.run(["git", "diff", "--no-index", "--check", "--", "/dev/null", name],
                                   cwd=ROOT, text=True, capture_output=True)
            # --no-index returns 1 for a different file even when --check has no diagnostics.
            if patch.returncode not in (0, 1) or patch.stdout or patch.stderr:
                raise ValueError("Whitespace check failed: " + name + "\n" + patch.stdout + patch.stderr)
        print("PASS: syntax and preparation file whitespace (includes untracked files)")
        sources = sorted(p for base in (ROOT / "cmd", ROOT / "internal") for p in base.rglob("*.go"))
        if sources:
            formatted = subprocess.check_output([str(ROOT / "artifacts/dev-env/sdk" / ("go" + (ROOT / ".go-version").read_text().strip()) / "bin/gofmt"), "-l", *map(str, sources)], text=True)
            if formatted:
                raise ValueError("Go files need gofmt")
            run(["bash", "scripts/go-local", "vet", "./..."])
            run(["bash", "scripts/go-local", "test", "./..."])
            run(["bash", "scripts/go-local", "build", "./..."])
        else:
            print("NOT RUN: product Go build/test/vet (no product packages yet)")
    except (subprocess.SubprocessError, OSError, ValueError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
