---
name: Google OAuth scope preservation
description: Preserve authorized Google scopes and refresh credentials across OAuth reauthorization.
---

Google OAuth responses can omit `scope` when it is unchanged, and incremental authorization may return only newly granted scopes. Reauthorization may also omit a refresh token even though the connection already has a usable one.

**Invariant:** Use the signed OAuth state's requested scopes only when the token response omits `scope`; trust a nonempty response, then merge it with scopes already stored for that connection. Preserve the existing refresh token when reauthorization omits one.

**Why:** Replacing scopes with an omitted or partial response silently removes working capabilities; replacing a saved refresh token with an empty value breaks operations after the access token expires.

**How to apply:** Keep token-response scope fallback, existing-scope/capability merging, and refresh-token retention in every Google reauthorization path. Never infer a scope that was not requested or reported.

**Integration rollout policy:** Begin Google integrations with a limited scope set that avoids sensitive or restricted permissions, then expand incrementally as needed.

**Why:** The user wants Google access to grow gradually rather than requesting broad permissions at the start.

**How to apply:** For each Google integration, request only the least-permissive scopes needed for its initial capabilities and add scopes only when a later feature requires them.

**Login/signup rule:** Request only `openid`, `email`, and `profile` for identity. Use `prompt=select_account` on login/link, but do not set `include_granted_scopes`, offline access, or forced consent; reserve incremental grants and refresh-token requests for Google integrations.

**Why:** Account selection should remain explicit without forcing repeat consent or carrying Calendar/Gmail permissions into identity authentication, which is separate from connecting Google services.
