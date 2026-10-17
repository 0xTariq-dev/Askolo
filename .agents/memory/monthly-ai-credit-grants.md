---
name: Monthly AI credit grants
description: Product rules for recurring AI credit grants and schedule initialization.
---

Recurring grants follow signup-anchored 30-day cycles. Initializing schedules must not backfill cycles from before recurring grants were enabled. The global enable switch remains the master gate; per-user settings may narrow it but cannot bypass it.

Grandfather existing unspent balances when per-grant expiry is introduced. Expiry applies only to grants issued after the change.

**Why:** Backdated bulk grants can create unexpected balances when recurring grants are enabled, while a global off switch must reliably stop all issuance. Applying expiry retroactively could remove credits users already hold.

**How to apply:** Preserve these rules when changing monthly grant scheduling, admin previews, per-user overrides, or grant expiry/FEFO accounting.
