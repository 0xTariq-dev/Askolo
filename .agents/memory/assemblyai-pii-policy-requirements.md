---
name: AssemblyAI explicit PII policy list
description: Provider behavior for PII-redacted prerecorded transcription requests.
---

For prerecorded AssemblyAI transcripts, `redact_pii: true` and `redact_pii_sub: "hash"` are not sufficient for newer accounts; the request must include a non-empty `redact_pii_policies` array. To preserve comprehensive redaction, send the complete current policy set from the provider documentation rather than an empty or partial list.

**Why:** Newer accounts can reject submissions with HTTP 400 when the policy list is omitted, even though older examples may omit it.

**How to apply:** Keep the allowlist synchronized with AssemblyAI's current policy documentation, test the serialized request, and classify provider errors without logging raw response bodies.