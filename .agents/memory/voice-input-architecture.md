---
name: Reusable voice input
description: The cross-app contract for browser speech recognition and recorded-audio transcription.
---

Voice input is a reusable capture-and-review capability, not planner-specific behavior. Consumers should use the shared hook and own only how reviewed text is inserted into their editor.

**Why:** Browser speech recognition is inconsistent, so every consumer needs the same recorded fallback, cancellation semantics, size/duration limits, duplicate-tail cleanup, and explicit review step.

**How to apply:** New voice-enabled surfaces should consume the hook’s start/stop/cancel/reset API and never duplicate MediaRecorder, SpeechRecognition, base64, or transcription request logic.