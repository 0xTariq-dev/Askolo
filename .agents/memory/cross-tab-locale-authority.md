---
name: Cross-tab locale authority
description: How cross-tab language updates interact with account-provided locale state.
---

Within the same account scope, treat the newest cross-tab locale update as authoritative over a stale account snapshot when locale providers remount.

**Why:** An account provider can mount after another tab changes language; blindly syncing its stale account value back to browser storage can undo the user's new choice.

**How to apply:** Preserve the latest locale event per account scope across provider remounts, while keeping account preference writes tied to the user's explicit language selection.
