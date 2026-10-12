---
name: Google OAuth scope preservation
description: Preserve authorized Google scopes and refresh credentials across OAuth reauthorization.
---

Google OAuth responses can omit `scope` when it is unchanged, and incremental authorization may return only newly granted scopes. Reauthorization may also omit a refresh token even though the connection already has a usable one.

**Invariant:** Use the signed OAuth state's requested scopes only when the token response omits `scope`; trust a nonempty response, then merge it with scopes already stored for that connection. Preserve the existing refresh token when reauthorization omits one.

**Why:** Replacing scopes with an omitted or partial response silently removes working capabilities; replacing a saved refresh token with an empty value breaks operations after the access token expires.

**How to apply:** Keep token-response scope fallback, existing-scope/capability merging, and refresh-token retention in every Google reauthorization path. Never infer a scope that was not requested or reported.

**Login/signup rule:** Request only `openid`, `email`, and `profile` for identity, and do not set `include_granted_scopes`, offline access, or forced consent on login/link. Reserve incremental grants and refresh-token requests for Google integrations.

**Why:** Users should not see previously granted Calendar/Gmail permissions during account signup or sign-in; identity authentication is separate from connecting Google services.
