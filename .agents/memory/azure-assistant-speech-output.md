---
name: Azure assistant speech privacy
description: Safeguards for Azure speech in the complementary semi-live assistant mode and existing Listen flow.
---

Azure speech is a separate, revocable provider choice from AssemblyAI's full-duplex live Voice Agent. In the opt-in semi-live mode, synthesize completed assistant responses for spoken playback rather than relying only on a text message with a Listen button; preserve an accessible text alternative and replay path. Only synthesize an authenticated user's stored assistant response for a run they own; never accept arbitrary client text or SSML. Granting consent alone must not trigger synthesis or playback—the user must initiate an assistant turn in the chosen mode. Keep the existing explicit Listen behavior available outside semi-live mode.

Browser/device speech is only a fallback after an Azure/network or audio-playback failure. Do not use it when consent is absent or when the server returns a 4xx response such as authorization, consent, or rate limiting. Audio stays transient; do not log speech, transcripts, provider response bodies, credentials, or audio.

**Why:** Semi-live Azure replies and managed AssemblyAI live sessions are complementary modes with different data flows. Assistant speech can expose response content to a third party, so each provider needs a clear, independent consent and disclosure.

**How to apply:** Preserve ownership and consent checks when adding or changing speech modes, playback behavior, or provider fallback. Do not silently fall back between Azure and AssemblyAI modes.