---
name: Monthly AI credit grants
description: Product rules for recurring AI credit grants and schedule initialization.
---

Recurring grants follow signup-anchored 30-day cycles. Initializing schedules must not backfill cycles from before recurring grants were enabled. The global enable switch remains the master gate; per-user settings may narrow it but cannot bypass it.

**Why:** Backdated bulk grants can create unexpected balances when recurring grants are enabled, while a global off switch must reliably stop all issuance.

**How to apply:** Preserve these rules when changing monthly grant scheduling, admin previews, or per-user overrides.
