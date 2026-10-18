---
name: Grant type cap defaults
description: Compatibility rules for per-grant-type issuance, carryover, and expiry defaults.
---

Per-type caps must not silently disable already-configured signup or monthly grants. Admin grant caps should begin high enough to preserve legacy manual issuance; administrators can set narrower limits explicitly.

**Why:** Existing policies did not cap individual admin grants, while signup and monthly amounts may already be configured independently of the rollover cap.

**How to apply:** When adding or backfilling grant-type policies, ensure active signup/monthly amounts fit both relevant caps and do not derive admin limits solely from the monthly rollover cap.
