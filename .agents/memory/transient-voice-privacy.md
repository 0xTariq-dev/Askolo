---
name: Transient voice privacy
description: Keep active voice transports transient; completed Voice Agent artifacts may be copied locally only through a separate consented async memory layer.
---

Recorded-transcription audio and provider transcript payloads remain transient in the existing flow: keep the completed Blob only in the active browser session and delete the provider transcript after processing. Live WebSocket recovery remains metadata-only and must not persist or replay audio or provider transcript content.

The user has approved a separate follow-up for an asynchronous Voice Agent session-memory layer that may retrieve completed-session artifacts and store the minimum data needed locally. That layer must not block or replay the live interaction, and must define explicit consent, user-scoped access, retention, deletion, and the handling of recordings, transcripts, metadata, and tool-call details. Do not treat this future exception as permission to persist active-session data in the current transport or recovery path. Assistant-run text continues to follow its separate conversation-storage contract.

**Why:** The product keeps live interaction recovery low-latency and content-free while allowing completed Voice Agent context to be considered later under a distinct, consented storage policy.

**How to apply:** Keep active-session recovery metadata-only. Implement completed-session retrieval and local storage only in the separate memory-layer task, with durable ownership, retention, deletion, and access controls; update the consent version whenever user-facing privacy behavior changes.