---
name: AssemblyAI deletion and retention claims
description: Distinguish transcript deletion from Voice Agent session deletion, account opt-out, and verified retention guarantees.
---

For recorded transcription, treat `DELETE /v2/transcript/{id}` confirmation as confirmation that the transcript record was deleted. Do not claim the separately uploaded audio copy was also deleted unless documentation or an account-level retention guarantee explicitly confirms it.

Voice Agent session retrieval can expose recordings, conversation timelines, metadata, and tool-call details. Its delete endpoint is documented as a soft-delete that makes artifacts inaccessible; do not describe it as confirmed physical erasure. AssemblyAI says model-improvement opt-out applies across APIs, but it is prospective and does not cover earlier requests. Enable it before new sessions. Configure TTLs only when their scope is verified for Voice Agent artifacts; do not assume asynchronous-transcription or streaming-STT policies apply.

**Why:** Provider deletion endpoints and product-specific retention controls have different guarantees; treating soft-delete, account opt-out, and TTL as equivalent would overstate what happens to voice data.

**How to apply:** Keep Askolo storage claims separate from provider status. Disclose provider-specific processing and deletion limits, and verify the account configuration and product scope before promising retention or erasure behavior.