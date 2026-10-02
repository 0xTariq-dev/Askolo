---
name: Transient voice privacy
description: Keep active voice transports transient; completed Voice Agent artifacts may be copied locally only through a separate consented async memory layer.
---

Recorded-transcription audio and provider transcript payloads remain transient in the existing flow: keep the completed Blob only in the active browser session and delete the provider transcript after processing. Live WebSocket recovery remains metadata-only and must not persist or replay audio or provider transcript content.

The user has approved handling Voice Agent carryover in the existing chat-memory layer, not in the live recovery path. After a session ends, that layer may asynchronously retrieve completed-session artifacts and store a brief, consented summary scoped to the in-app chat. The summary may include the conversation goal and unresolved next step, relevant facts and preferences, and confirmed tool actions and outcomes. A new in-app chat starts fresh; later Voice Agent sessions in the same chat may receive its brief summary when available. Memory processing must not delay a new live session. Define explicit consent, user-scoped access, retention, deletion, and handling of recordings, transcripts, metadata, and tool-call details. Do not treat this exception as permission to persist or replay active-session audio or verbatim transcripts in transport or recovery. Assistant-run text continues to follow its separate conversation-storage contract.

The current public Voice Agent specification documents `agents.assemblyai.com` and no dedicated EU endpoint. The user chose to allow EU profiles to use Live Mode only with a clear disclosure that EU-only processing is not verified; never describe this route as EU-resident or switch silently to another provider. Azure semi-live remains a separate mode with its own consent and verified-region requirements.

**Why:** The product keeps live interaction recovery low-latency and content-free while the user has chosen chat-scoped, consented summary carryover for future Voice Agent sessions.

**How to apply:** Keep active-session recovery metadata-only. Attach only a brief summary from the consented chat-memory layer to a later session in the same in-app chat; start new chats fresh and do not block live startup while a summary is pending. For EU profiles, show the residency limitation before explicit Live Mode consent and do not claim EU-only processing. Maintain explicit consent, retention, deletion, and access controls; update the consent version whenever user-facing privacy behavior changes.