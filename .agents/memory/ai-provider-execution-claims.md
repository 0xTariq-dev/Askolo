---
name: AI provider execution claims
description: The concurrency rule that prevents replayed ledger requests from invoking a billable provider twice.
---

A one-shot billable operation may invoke its provider only after atomically changing a newly reserved ledger entry into the claimed/active state. Returning an existing active reservation is not authorization to run the provider callback.

**Why:** Balance reservation and settlement idempotency alone do not prevent two concurrent same-key requests from both reaching the provider. The execution claim is the side-effect boundary.

**How to apply:** Any future model, tool, automation, transcription, or similar one-shot integration must use the guarded priced-operation wrapper or an equivalent lock-and-claim transition before making the external call.