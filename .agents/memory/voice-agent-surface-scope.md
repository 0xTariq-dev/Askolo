---
name: Voice Agent surface scope
description: Keep the managed AssemblyAI Live Voice Agent limited to the surfaces the user selected.
---

The managed AssemblyAI Live Voice Agent is available in Assistant chat and Planning. It is not offered in Notes/upload. Recorded dictation remains a separate workflow.

The curated voice choices are US English `michael` and `mary`, UK English `paul` and `vera`, Italian `giovanni`, Spanish `lola`, German `juergen`, Portuguese `rafael`, and French `estelle`. Save the user's selected voice in Settings, separately from app UI locale and Azure speech-output preferences. Do not offer a voice preview: there is no dedicated sample endpoint outside a live call or the provider's playground.

Keep the runtime feature flag off until the account-wide model-improvement opt-out and Voice Agent-specific retention coverage are verified. Do not invent an hourly rate or promise retention or regional residency; the rate must be explicitly configured, and provider location/retention details must remain disclosed as unverified until confirmed.

**Why:** The user selected Assistant chat and Planning for full-duplex Live Mode, excluding Notes/upload; chose the listed per-language voices without sample previews; and requires the feature to stay disabled until provider privacy settings are verified and pricing is explicitly set.

**How to apply:** Expose Live Mode start controls only in Assistant chat and Planning. Keep Notes/upload on the existing recorded-transcription path and preserve its destination-specific review and add behavior. Resolve the saved voice to its matching stored agent; do not derive it from the app's UI locale. Leave the environment flag default-off; fail closed when the Voice Agent hourly card is absent or nonpositive.
