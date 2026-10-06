---
name: Azure assistant speech privacy
description: Safeguards for Azure speech in the complementary semi-live assistant mode and existing Listen flow.
---

Azure speech is a separate, revocable provider choice from AssemblyAI's full-duplex live Voice Agent. In the opt-in semi-live mode, synthesize completed assistant responses for spoken playback rather than relying only on a text message with a Listen button; preserve an accessible text alternative and replay path. Only synthesize an authenticated user's stored assistant response for a run they own; never accept arbitrary client text or SSML. Granting consent alone must not trigger synthesis or playback—the user must initiate an assistant turn in the chosen mode. Keep the existing explicit Listen behavior available outside semi-live mode.

Do not automatically fall back to browser/device speech after Azure synthesis, network, or playback failure. Keep the response text accessible, report the error, and let the user retry explicitly. Audio stays transient; do not log speech, transcripts, provider response bodies, credentials, or audio.

**Why:** Azure consent authorizes the disclosed Azure flow only; silent fallback would switch speech engines without a separate user choice.

**How to apply:** Preserve ownership and consent checks when changing speech modes or playback. Keep Azure failures on the text-and-retry path; never invoke browser speech automatically.