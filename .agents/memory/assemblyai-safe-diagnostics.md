---
name: AssemblyAI safe diagnostics
description: Safe operational logging for voice provider failures.
---

For AssemblyAI failures, log only code-controlled operation stages, failure categories, numeric HTTP statuses, request IDs, and provider-transcript cleanup attempt/status metadata. Never log provider response bodies, uploaded audio, transcript text, API keys, signed audio URLs, or provider transcript IDs. Keep client-facing errors generic.

**Why:** Provider errors may contain untrusted details, while transcript and audio identifiers can expose sensitive voice data. Stage and status metadata are enough to separate upload, submission, polling, and deletion failures without retaining the content.

**How to apply:** When adding or changing AssemblyAI request paths, preserve safe failure classification and verify tests reject provider-body, credential, audio, and transcript-ID leakage from log attributes.