---
name: AI credit coverage
description: Product scope for which paid AI operations must consume user balance.
---

Every paid AI action initiated for a user must be charged to that user's AI balance. This includes assistant model usage behind email and other tools, AssemblyAI transcription/live voice, and Azure Speech output as a separate character-metered action. Keep the USD ledger and display balances as credits without disclosing the conversion. Distinguish provider-measured usage from proxy estimates in evidence and receipts; reconciliation must adjust the existing reservation and ledger rather than create a second charge.

**Why:** The user explicitly wants all paid AI actions—from email triage to voice calls—covered, not only the obvious chat or voice entry points.

**How to apply:** When adding a paid provider call, reserve before execution, meter the provider usage or a reliable proxy, settle/refund or leave it pending for review, and include the action in user-safe receipts. Keep evidence payloads limited to allow-listed metering metadata—never prompts, transcripts, audio, or other user content.
