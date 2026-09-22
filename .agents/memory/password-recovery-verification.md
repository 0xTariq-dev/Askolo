---
name: Password recovery verification
description: Password recovery uses a non-consuming verification step followed by one-time consumption during the final password reset.
---

The recovery code verification page must validate the active challenge without consuming it; the final password reset must revalidate and atomically consume the same challenge.

**Why:** A sequential UI needs a real server-side code step, but consuming the code before the password is submitted would make the final reset replay-safe only by adding a separate short-lived authorization record.

**How to apply:** Keep challenge attempt counting and expiry checks in both calls, preserve the generic recovery response for unknown accounts, and do not log codes or passwords.