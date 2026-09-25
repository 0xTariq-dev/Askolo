---
name: AssemblyAI deletion claims
description: Bound user-facing retention claims to the provider's documented transcript-delete behavior.
---

Treat `DELETE /v2/transcript/{id}` confirmation as confirmation that the transcript record was deleted. Do not claim the uploaded audio copy was also deleted unless AssemblyAI documentation or an account-level retention guarantee explicitly confirms it.

**Why:** The API reference documents removing transcript data and marking the transcript deleted, but does not clearly state that this operation deletes the separately uploaded audio object.

**How to apply:** Keep the app's own raw-audio storage claim separate from provider deletion status. Disclose that provider retention and model-improvement settings apply to audio sent for transcription.