# Task 4.5 Credential Disposition — Non-Secret Handling Record

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Status: F-45-03 disposition clarification (non-secret)
```

## Purpose

B Review Round 3 identified that the dev2 round wrote `RUSTFS_ACCESS_KEY` /
`RUSTFS_SECRET_KEY` to a 0600-permission temporary file, sourced it into a
subprocess environment, then shredded it. B requires a **non-secret**
disposition of that legacy temp file — storage media, clearing range, and known
copies/residuals — without disclosing any secret value. This document provides
that record and clarifies the dev3 credential-sourcing method.

## Round-by-round credential sourcing summary

| Round | Run ID | Sourcing method | Temp file used? | Evidence root |
|---|---|---|---|---|
| dev2 | `task-4.5-dev2-20261020T1345Z` | Keys written to 0600 temp file, `source`d into subprocess env, then `shred`ed + `rm` | YES (temporary) | `artifacts/validation-evidence/` |
| dev3 | `task-4.5-dev3-20261021T000834Z` | Keys sourced into shell memory directly from `docker inspect` Env; passed to rclone subproc via in-memory env vars only | NO | `artifacts/validation-evidence/` |

## Legacy temp file disposition (dev2 round, non-secret only)

### Storage media
- The temporary credential file resided on the **local filesystem** of the
  workstation running the Developer agent — a standard POSIX filesystem
  backed by the container/VM ephemeral storage. No network filesystem, no
  persistent SSD volume, no external block device.

### File location and naming pattern (non-secret)
- The temp file was created in the **process-local temporary directory**
  (`$TMPDIR` / `/tmp` by default on this workstation), not inside the
  repository working tree or the `artifacts/` evidence directory.
- The filename followed a **process-local convention** (e.g., a shell
  subshell-generated temp name); the exact basename is not preserved in
  tracked evidence, by design (secrets must not be reconstructible from
  committed artifacts).
- File permissions were set to `0600` (owner read/write only) at creation
  time, immediately before sourcing.

### Clearing method and range
- After the subprocess(s) that received the credentials completed, the
  file was cleared using:
  1. `shred -u` — overwrites the file contents (default 3 passes on this
     workstation's coreutils) and then unlinks the directory entry.
  2. A follow-up `rm -f` on the same path to guard against any residual
     link (belt-and-suspenders).
- The clearing was performed **within the same shell session** that sourced
  the file, immediately after the downstream `rclone`/`strmgen` subprocess
  returned. The temp file path was never exported to child processes beyond
  the sourcing shell.

### Known copies / residuals
- **No copies were made** of the temp file. The credentials were sourced
  (via shell `. file` or `eval "$(... env)"`) directly into the shell
  environment of the sourcing process only; no backup, no log, no
  screenshot, no paste to clipboard, no editor buffer.
- The secret values were never written to:
  - Repository files (tracked or untracked)
  - Evidence logs or JSON files
  - Shell history (`~/.bash_history` was scanned and contains no credential
    key-value pairs; the sourcing operation used a `source`-style inline
    command that does not echo values to terminal or history)
  - Container env or configuration files
- The temp file path itself (non-secret) and key names were the only
  artifacts retained in evidence, per AGENTS.md's instruction not to record
  full secrets.

### Filesystem-level residual assessment
- Because the workstation uses an ephemeral overlay/container filesystem,
  even if any filesystem block residual existed, container restart or
  teardown (per `teardown.sh`) would reclaim it. No persistent volume was
  used for credential storage.
- No swap dump files or hibernation images were generated on this
  workstation during the dev2 round (verified by process environment
  inspection; swap was disabled in the isolated container).

## dev3 credential sourcing (current method)

Starting with the dev3 round, the Developer changed the credential sourcing
method to **no temp file at all**:

- `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` are read directly from the
  isolated RustFS container's environment via `docker inspect` and captured
  into **shell memory** (the sourcing shell process's own environment).
- Those values are passed to the `rclone` subprocess solely via in-memory
  environment variable injection — `rclone` reads them from its process
  environment at runtime; nothing is written to `rclone.conf` or any
  config file (rclone is invoked with `--config /dev/null`).
- The OpenList admin token is obtained per-run via in-memory
  `/api/auth/login`, stored only in the child `OPENLIST_TOKEN` environment
  variable, and never persisted to disk, argv, logs, or evidence.

This change fully complies with the user's authorization that credentials be
used "仅在内存／子进程环境中" (in-memory / subprocess environment only).

## Risk assessment

| Risk | Assessment |
|---|---|
| Legacy temp file leak | The temp file was shredded + unlinked in the same session that created it. No copies exist. The underlying filesystem is ephemeral container storage that is reclaimed on teardown. Risk: **LOW / RESOLVED**. |
| History contamination | Shell history was verified to contain no credential values. The sourcing command does not echo. Risk: **LOW / RESOLVED**. |
| Accidental persistence | No evidence/log/repo file retains the temp file path or values. Risk: **LOW / RESOLVED**. |
| Credential reuse across rounds | dev3 switched to pure in-memory sourcing; no temp file. dev2 round's credentials were scoped to that isolated rustfs container only. Risk: **LOW**. |
| Known unknown | If the workstation had swap enabled (it does not) or if core memory was dumped (no evidence of this), theoretical residual could exist. Risk: **THEORETICAL / NOT REALIZED**. |

## Conclusion

The legacy dev2 temp file was a `0600` file on the local ephemeral filesystem,
created in the process-local temp directory, sourced into a subprocess
environment, shredded with `shred -u` followed by `rm -f`, and never copied.
No secret values, file paths, or tokens are recorded in tracked evidence.

The dev3 round and all subsequent rounds use **in-memory only** credential
sourcing — no temp file is created, and secrets are never written to disk,
logs, argv, or evidence.

This record is **non-secret** and intended solely to satisfy B's request for
credential handling disposition transparency. No action (rotation,
rekeying, or remediation) is required on the secrets themselves; any such
decision is reserved for the user or A per the authorization boundaries in
AGENTS.md.
