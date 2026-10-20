---
name: MFA recovery account ownership
description: P09 recovery and trusted-device controls are private to each individual login account.
---

For P09, MFA recovery and trusted-device controls belong to the individual login account holder, including when the account participates in a family Space. Space ownership or membership does not grant access to another account's recovery material or device list. Guardian-managed child-account recovery is out of scope.

**Why:** The user chose account-holder-only MFA recovery and trusted-device management for child accounts.

**How to apply:** Keep recovery endpoints, codes, device listing, and revocation scoped to the authenticated account. Do not add parent/guardian delegation or cross-account recovery without a separate approval.
