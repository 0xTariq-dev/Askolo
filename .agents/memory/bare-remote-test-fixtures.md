---
name: Bare remote test fixtures
description: Reliable setup for temporary Git bare remotes used by repository regression tests.
---

Temporary bare remotes may not update their symbolic `HEAD` when the first push targets a branch explicitly. Clone fixtures with an explicit branch instead of relying on the remote's default checkout.

**Why:** A plain clone can produce an empty working tree and fail later when the test tries to create a remote-ahead commit.

**How to apply:** When a regression test seeds a bare remote with a non-default or newly created branch, pass the expected branch explicitly to the clone operation.