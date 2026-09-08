---
name: Client voice gating
description: Recorded voice notes must be screened in the browser before provider requests.
---

Recorded voice input uses a short minimum-duration check and decoded-audio RMS/activity check before base64 conversion or the transcription mutation. A failed browser decode is treated as unsafe to send.

**Why:** Push-to-talk users can release immediately or capture silence; sending those clips wastes provider calls and can consume AI credits without producing useful text.

**How to apply:** Keep the browser gate before any backend mutation. The backend still validates every request because client checks are advisory, not a security boundary.