---
name: Microsoft authentication email trust
description: Verified-address requirements for Microsoft sign-in and account creation.
---

Microsoft Graph `mail` and `userPrincipalName` values are not proof of verified email ownership. Microsoft sign-in may create a session only for an already linked provider identity; new users need a verified Askolo signup before linking Microsoft. Do not retain Microsoft tokens for sign-in when they are not needed.

**Why:** Microsoft Graph profile fields may be unset, mutable, or unsuitable as verified-email evidence, so treating them as verified during automatic account creation could link the wrong Askolo account.

**How to apply:** For Microsoft account provisioning, verify a signed ID token's issuer, audience, signature, expiry, nonce, and verified-email claim with a maintained OIDC library. Until then, require an existing Askolo account and authenticated identity linking.
