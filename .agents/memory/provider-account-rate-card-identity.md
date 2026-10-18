---
name: Provider account rate-card identity
description: Product decision for identifying provider accounts when selecting AI credit rates.
---

Use an administrator-configured stable account ID for each connected provider credential and bind rate cards to that ID. Do not add provider credential management to AI credit policy.

**Why:** the user selected admin-configured account IDs while keeping credential/account lifecycle management out of this scope.

**How to apply:** resolve rates using the configured credential identity and reject a rate card whose account ID does not match the active provider credential.
