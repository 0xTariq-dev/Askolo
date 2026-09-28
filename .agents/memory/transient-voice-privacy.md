---
name: Transient voice privacy
description: Voice audio and provider transcripts stay transient; live-session recovery persists metadata only.
---

Voice recordings and AssemblyAI provider transcript payloads must not be persisted by Askolo or represented as retention choices. Keep a completed Blob only in the active browser session for playback, retry, and explicit deletion; send it to AssemblyAI for transcription and delete the provider transcript after processing. Live WebSocket recovery may persist session/event metadata, identifiers, sequence numbers, and HMAC digests, but must never persist or replay audio or provider transcripts. Assistant-run text follows the separate existing assistant conversation storage contract.

**Why:** The product decision prioritizes no recording or provider-transcript retention while allowing safe status recovery without storing sensitive message content.

**How to apply:** Do not add object storage, audio metadata tables, retention selectors, or recording-list APIs for this flow. Keep live-session resume metadata-only; update the consent version whenever the privacy wording or processing behavior changes.