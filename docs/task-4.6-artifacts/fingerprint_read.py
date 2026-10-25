#!/usr/bin/env python3
"""RETIRED entry: direct reads of the plugin's fingerprint store are forbidden.

History (kept factual, not re-authorised): an earlier version of this file
opened the isolated Jellyfin plugin's ``strm-fingerprints.db`` read-only and
queried ``strm_fingerprint``. The second B review of Task 4.6 (F-46-02,
``https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/task-4.6-b-review-2.md``) established that this violates the Task
boundary: AC-2 forbids reading or writing the Jellyfin / plugin databases, and
read-only access is still access. Historical evidence that used this entry
stays in place and is marked as non-compliant in the reviewer record; it is
not deleted or rewritten.

This file is now a fail-closed tombstone. It performs no file system or
database access, it never imports ``sqlite3``, and any call raises
``DirectStoreReadForbidden``. The historical side of a comparison is described
by the native baseline evidence (02-ac2 and the per-row restore reuse of the
locked plugin's own decision log), never by this entry.
"""
from __future__ import annotations

import sys

RETIREMENT_REFERENCE = "https://github.com/kentinya/STRM-Generator-GO/blob/4d16846722f31aa9615494b90df715cf4b5bb6df/docs/task-4.6-b-review-2.md (F-46-02)"


class DirectStoreReadForbidden(RuntimeError):
    """Raised because reading the plugin's own store is outside the Task."""


# No store path is configured any more. The previous constant pointed at the
# plugin's database under the isolated Jellyfin config; it is intentionally
# gone so the entry cannot be re-pointed by accident.
STORE = None


def read(item_id: str) -> dict | None:
    raise DirectStoreReadForbidden(
        "direct reads of the plugin fingerprint store are forbidden for "
        "Task 4.6; see " + RETIREMENT_REFERENCE
    )


def main() -> int:
    print(
        "fingerprint_read.py is retired: direct plugin database access is "
        "forbidden for Task 4.6; see " + RETIREMENT_REFERENCE,
        file=sys.stderr,
    )
    return 3


if __name__ == "__main__":
    raise SystemExit(main())
