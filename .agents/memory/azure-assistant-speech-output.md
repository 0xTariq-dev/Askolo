---
name: Azure assistant speech privacy
description: Durable safeguards for the custom Azure TTS path used to read assistant responses.
---

Azure speech output is separate from transcription consent and must be revocable. Only synthesize an authenticated user's stored assistant response for a run they own; never accept arbitrary client text or SSML for Azure synthesis. Consent acceptance saves permission but must not trigger playback. The user starts playback explicitly.

Browser/device speech is only a fallback after an Azure/network or audio-playback failure. Do not use it when consent is absent or when the server returns a 4xx response such as authorization, consent, or rate limiting. Audio stays transient; do not log speech, transcripts, provider response bodies, credentials, or audio.

**Why:** Assistant speech can expose personal response content to a third-party provider or a device speech engine, so the user needs clear control over each disclosure and playback.

**How to apply:** Preserve these boundaries when adding another speech provider, changing fallback behavior, or modifying assistant-run ownership and consent checks.