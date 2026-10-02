---
name: Voice Agent surface scope
description: Keep the managed AssemblyAI Live Voice Agent limited to the surfaces the user selected.
---

The managed AssemblyAI Live Voice Agent is available in Assistant chat and Planning. It is not offered in Notes/upload. Recorded dictation remains a separate workflow.

Keep the runtime feature flag off until the account-wide model-improvement opt-out and Voice Agent-specific retention coverage are verified. Do not invent an hourly rate or promise retention or regional residency; the rate must be explicitly configured, and provider location/retention details must remain disclosed as unverified until confirmed.

**Why:** The user selected Assistant chat and Planning for full-duplex Live Mode, excluding Notes/upload, and requires the feature to stay disabled until provider privacy settings are verified and pricing is explicitly set.

**How to apply:** Expose Live Mode start controls only in Assistant chat and Planning. Keep Notes/upload on the existing recorded-transcription path and preserve its destination-specific review and add behavior. Leave the environment flag default-off; fail closed when the Voice Agent hourly card is absent or nonpositive.