---
name: AssemblyAI standalone TTS limit
description: AssemblyAI cannot synthesize an existing text response through a standalone TTS API.
---

AssemblyAI speech output is bundled into its managed Voice Agent API; it does not provide a separate endpoint that converts arbitrary text into audio. The Voice Agent API generates conversation responses, so it is not a safe substitute for reading an already-generated assistant reply.

**Why:** Sending an existing reply as voice-agent input can prompt the agent to create a new answer instead of synthesizing the supplied text.

**How to apply:** For assistant-response read-aloud, use browser/device speech synthesis or a dedicated TTS provider; only use AssemblyAI Voice Agent API when the product is intentionally a managed spoken conversation.