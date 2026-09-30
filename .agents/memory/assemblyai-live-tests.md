---
name: Opt-in AssemblyAI provider tests
description: Keep live AssemblyAI checks isolated from default tests and tightly bound their provider usage.
---

Keep real-provider tests separate from deterministic mock tests and require an explicit opt-in flag. Use synthetic, short English audio, the application's existing provider client and model, a firm timeout, a 60-second provider session cap, and explicit WebSocket termination. Never log the API key or transcript content.

**Why:** Real-time transcription is billed while its WebSocket remains open and requires a live API secret; making it part of ordinary CI would introduce cost, network flakiness, and secret dependencies. A 60-second token cap also limits exposure if cleanup fails.

**How to apply:** Run mock tests by default. Run live tests only when deliberately requested, use the provider's minimum supported session cap for short fixtures, charge to the configured AssemblyAI account, and keep them separate from Askolo account-credit tests.