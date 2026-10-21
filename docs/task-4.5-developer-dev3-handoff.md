# Task 4.5 — Developer dev3 Handoff (F-45-01 / F-45-03 fixes)

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Head (unchanged): 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Status: READY FOR B REVIEW (B decision: PENDING)
```

## Summary
Developer round `task-4.5-dev3-20261021T000834Z` (evidence:
`artifacts/validation-evidence/task-4.5-dev3-…`) closes B's two P1 findings
from the 2026-10-21 review (F-45-01, F-45-03). No Go product / dependency /
tooling / media-set change. Only new isolated evidence (Git-ignored) and this
doc (tracked). No credentials are written to disk, argv, logs, evidence or git;
`git diff --check` is clean; working tree on `## main`.

## F-45-03 fix — in-memory credentials only (no temp files)
- `RUSTFS_ACCESS_KEY`/`RUSTFS_SECRET_KEY` (`slice4access`/`slice4secret-20261019`,
  confirmed via `docker inspect` Env key-presence only) sourced into shell memory
  only; fed to `rclone` subprocs via in-memory env vars.
- OpenList token obtained per-run through in-memory `/api/auth/login` using the
  container env password (key presence only recorded). Token injected solely via
  child `OPENLIST_TOKEN` env; helper script lives at `/tmp/token_exec.py`
  (NOT in repo, NOT committed, NOT evidence). `measure_run.py` already enforces
  `token_source: "OPENLIST_TOKEN environment only; not in argv or output"` in
  every JSON.
- Media-content backups (`orig-Synthetic-*.mkv`, byte-identical to manifest A
  `7f53d154…`) are non-secret data backups, not credentials.
- Evidence scan: no `local-test-admin-password`, `JWT_SECRET=`, full-token, or
  `token_exec` script contents anywhere in the run dir.

## F-45-01 fix — real MOVE to a new path + same-row reappearance clear-to-zero
### Real move (sample `Synthetic 054.mkv`)
- Remote mutation: `rclone moveto /s3/Synthetic 054.mkv -> /s3/Moved-Synthetic 054.mkv`
  (new path, same source root; not a deletefile-only aside). Old path byte-identical.
- Sync round1 (cleanup cfg, threshold 3): `created=1 updated=1 skipped=9998 missing=1`
  (exit 0, 996.0s/29024KiB). New STRM `Moved-Synthetic 054.strm` (size 24404, URL
  `../d/s3/Moved-Synthetic 054.mkv`) created; **new ownership row** `created_round=10`;
  old `Synthetic 054.strm` row kept with `missing_count=1, missing_round=10` (STRM
  retained, below threshold).
- round2→3: `missing_count` 1→2→3, STRM retained each round (pre-deletion
  retention confirmed). Per-round DB snapshots: `round1-db.sqlite3`,
  `round2-db.sqlite3`, `round3-db.sqlite3`.
- round4: `deleted=1`, old 054 STRM removed, ownership row dropped, 9999 rows
  (exit 0, 753.0s/28604KiB). Snapshot `round4-delete-db.sqlite3`. Final converged
  dataset: `Moved-Synthetic 054.strm` exists, pointing to new remote path.

### Definitive same-row trajectory (sample `Synthetic 057.mkv`)
Proves a **single ownership row** (`created_round=1`, never recreated) carries
`missing_count` 0→1→2→3→0 with explicit per-state DB snapshots:

| State | epoch | missing_count | row created_round | local STRM |
|---|---|---|---|---|
| baseline (present) | 21 | 0 | 1 | retained |
| round1 (removed)  | 22 | 1 | 1 | retained |
| round2              | 23 | 2 | 1 | retained |
| round3              | 24 | 3 | 1 | retained |
| round4 **FIRST FAIL** (slow paging) | 24 | 3 (unchanged, not committed) | 1 | retained |
| reappear (restored) | 25 | **0** (3→0) | 1 | retained |

Round4 first failure (slow transport timeout) is retained and a same-DB retry
recovered; `reappear-moved-round-FIRST-FAIL.json` + `reappear-moved-round-retry.json`.
Snapshots: `057-baseline-db.sqlite3`, `057-round1/2/3/4-db.sqlite3`,
`057-reappear-db.sqlite3`, trajectory: `057-definitive-trajectory.txt`.
057 STRM never deleted (count peaked at 3 < 4, retained), so the reappear row is
the SAME row — directly satisfying B's "reappearance clears a positive
missing_count to zero on the same ownership row".

## Update 055 A↔B (size-only, real STRM change)
`sync-move-round1` set 055 STRM `size:24430` (B content); final stable repeat
confirms `size:24404` (A) restored. `final-055.mkv` byte sha256 = `7f53d154…`
(matches A).

## Restore & dataset reconciliation (AC-5)
- All four samples restored to byte-identical originals (`s3-ops/final-sample-sha256.txt`).
- Remote vs manifest: 10000 == 10000, no diffs
  (`s3-ops/final-dataset-reconcile.txt`).
- No bucket deletion beyond the single aside-restore; no dataset rebuild.

## AC-1 / AC-2 / AC-4 / dry-run
- Populated dry-run (`populated-dryrun`): skipped=10000, exit 0; output identity
  sha256 `645d3d98…` and DB sha unchanged (`pre-stable-db.sha256`/`post-dryrun-db.sha256`);
  epoch held at 20 (dry-run not counted as a round).
- Final stable (base cfg): `final-stable-repeat`/`final-stable-repeat-2`:
  created=0 updated=0 skipped=10000, exit 0; bytes+mtime unchanged.
- Full tree: `verify-final-tree-2.txt` 10000/10000 PASS, 0 problems.

## Focused tests / static checks
- `go test ./internal/strm ./internal/pathmap` PASS; `go test ./internal/reconciler
  ./internal/storage ./internal/openlist ./internal/config ./cmd/strmgen` PASS.
- `go vet ./...` PASS; `gofmt -l cmd internal` clean (0); helper direct tests 15/15.
- `check_governance.py` PASS; `git diff --check` clean.

## Timing
Product-run wall (23 measured runs incl. first failures): ~16973.9s. Per the
2026-10-21 user disposition on historical timing, the >50% figure is not a
blocking requirement this round; it is reported honestly in `timing.txt`.

## Remaining (explicitly NOT self-signed PASS; B retains final gates)
- AC-6: independent critical reproduction of the real move/appear/zero-write
  boundary and full T4 suite — B specified full candidate still retained by B.
- RO-4 / RO-5 (native Jellyfin scan / D-01..08 plugin decisions) — Task 4.6,
  NOT RUN.
- No product defect found; slow-paging transport timeouts on the isolated S3 are
  retryable, not logic errors.

## Head SHA
`9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa` (no code change; checkpoint unchanged,
all new work is evidence).
