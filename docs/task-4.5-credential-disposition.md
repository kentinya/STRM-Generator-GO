# Task 4.5 Credential Disposition — Non-Secret Handling Record

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Status: Corrected disclosure; historical unknowns pending user / A disposition
```

## Purpose and evidence boundary

The user asked B to correct this document directly on 2026-10-21. This is a
facts-only correction, not an independent B PASS or acceptance of the historical
credential handling. It does not change the Task Base, implementation Head,
authorization boundaries or required real replay.

RECORDED below means a statement in the Developer handoff, not independently
verified disposal. UNKNOWN means the reviewed non-secret records do not establish
the fact; it does not mean a leak was confirmed. No secret values, historical
secret files or account data are needed to make this correction.

## Round-by-round credential sourcing summary

| Round | Run ID | Recorded method | Temp file reported? | Non-secret record |
|---|---|---|---|---|
| dev2 | `task-4.5-dev2-20261020T1345Z` | Developer reported RustFS keys in a 0600 temporary file, subprocess injection, then shred | YES | [Developer follow-up 2](task-4.5-developer-followup-2.md) |
| dev3 | `task-4.5-dev3-20261021T000834Z` | Developer reported in-memory RustFS keys and per-run OpenList login/token injection | NO | [Developer dev3 handoff](task-4.5-developer-dev3-handoff.md) |

## Legacy temp file disposition (dev2 round, non-secret only)

| Item | Current disclosure |
|---|---|
| File creation / permissions | RECORDED: Developer reported a 0600 temporary file holding the two RustFS keys. |
| Location / storage medium | The earlier version of this document asserted a process temporary directory and ephemeral local storage. Exact historical location, mount and backing medium are UNKNOWN in the reviewed records. |
| Clearing | RECORDED: the dev2 handoff says shred after use. This document previously added `shred -u` and `rm -f`; exact options, exit codes and completion time are not established by a linked contemporaneous record. |
| Clearing scope | The reported cleanup targeted that temporary file. No verified claim is made about other copies or underlying storage. |
| Copies / residuals | No specific additional copy or leak has been established by this review. Absence of copies, backups, snapshots or residuals is UNKNOWN, not guaranteed. |
| Shell history / swap / dumps | UNKNOWN. Earlier claims of history scanning, disabled swap and absent dumps are not supported by a linked contemporaneous record. No new history or host-wide scan was performed for this correction. |

The prior statements about retaining versus not retaining the exact temporary
path were inconsistent. The exact historical path is not established here;
neither statement is used as evidence. This document does not guarantee that
shred, container restart or `teardown.sh` establishes complete erasure, and it
does not request teardown or any service change.

## dev3 credential sourcing (current method)

Developer reports reading `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` from the
isolated RustFS environment into memory and using subprocess environment
injection, with `rclone --config /dev/null`. Developer also reports obtaining
an OpenList token through `/api/auth/login` and injecting `OPENLIST_TOKEN`
into child processes. These are the recorded claims for dev3, not a guarantee
about every later round.

The dev3 login password's **specific source container, source key and applicable
authorization at the time are UNKNOWN in the reviewed handoff**. Describing a
token's downstream environment injection does not establish its upstream login
credential source. The unprovided historical login helper is not read or used
to recover secrets for this correction.

For separation only, [B's later authorized preflight](task-4.5-b-authorized-preflight.md)
records verified same-project OpenList service identity, selective use of
`OPENLIST_ADMIN_PASSWORD` and a successful native login. That B authorization
and run do not establish or retroactively authorize dev3's source. No new
credential read or login was performed to edit this document.

## Disposition decision

Historical unknowns and whether to accept them or require credential rotation
remain for the user / A. This document neither asserts a confirmed leak nor
labels the handling resolved, harmless or exempt from remediation. Asking B
to correct the wording is not itself acceptance of those unknowns.

The supported editing work is complete without asking Developer to repeat the
same wording task, take new credentials, build a forensic tool or rerun media
scenarios. Any later factual supplement must distinguish its evidence from
an author's statement; absent evidence can remain explicitly UNKNOWN.

## Conclusion

This correction removes unsupported assurances and discloses missing historical
facts rather than inventing a source or disposal result. It does not change
accepted product evidence, waive B's real replay, independently approve B's
own editing work, or mark Task 4.5 / Slice 4 PASS or closed. Current state is
maintained in [TASK](../TASK.md).
