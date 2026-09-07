---
name: Transient voice privacy
description: Askolo voice recordings are transient and consent is the only persisted voice preference.
---

Voice recordings must not be persisted by Askolo or represented as retention choices. Keep a completed Blob only in the active browser session for playback, retry, and explicit deletion; send it to AssemblyAI for transcription and delete the provider transcript after processing. Persist only the user's versioned consent decision.

**Why:** The product decision prioritizes no recording retention and a clear first-use consent flow over cross-device playback.

**How to apply:** Do not add object storage, audio metadata tables, retention selectors, or recording-list APIs for this flow. Update the consent version whenever the privacy wording or processing behavior changes.