# Task 4.5 Credential Disposition — Non-Secret Handling Record

```text
Task: 4.5
Task Base: 5182443d0c34215deb7bb1e74a413564aa8e6422
Head: 9cdfde2e79dbc9b840ed61fc683d1e7bcef85baa
Status: Historical unknowns accepted by user on 2026-10-21; facts remain UNKNOWN
```

## Purpose and evidence boundary

The user first asked B to correct this document directly on 2026-10-21. That
earlier facts-only correction was not an independent B PASS or acceptance of the
historical handling. The user later explicitly accepted the disclosed unknowns,
as recorded below; neither action changes the Task Base, implementation Head,
future authorization boundaries or the required independent real evidence.

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

On 2026-10-21, B explained that the remaining blocker was the historical
credential record, not a failed product test: dev2 temporary-file disposal details
and dev3 login-password source / applicable authorization remain UNKNOWN. B
explicitly distinguished accepting those unknowns from requiring rotation.
The user replied **“接受” (accept)**.

This is the user's acceptance of the disclosed historical unknowns for Task 4.5.
F-45-03 no longer blocks that Task; further historical investigation or credential
rotation is not required as a condition of its acceptance. This does not establish
complete erasure, absence of disclosure, or historical authorization as verified
facts. The UNKNOWN entries and the prior review record are retained unchanged.

No secret was read, recovered, rotated or supplied to record this decision. It
does not authorize future credential files, account or service changes, production
access, or Task 4.6 credentials. Future handling retains its existing boundaries.

The factual correction work is complete; Developer need not repeat the wording
change, build a forensic tool or rerun accepted media scenarios. B separately
rechecks the unchanged executable inputs and accepted real evidence before
recording the Task review decision in [TASK](../TASK.md).

## Conclusion

The user has made the previously pending disposition decision. Historical facts
remain explicitly UNKNOWN rather than being rewritten as harmless or verified.
Task 4.5's independent product review can now conclude using the already completed
full gates and B real replay; no new product run or credential action follows from
this acceptance. Slice 4 still requires Task 4.6, Slice Final and A review, and is
not approved or closed by this document. Current state is maintained in
[TASK](../TASK.md).
