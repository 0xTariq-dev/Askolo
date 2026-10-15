---
name: TOTP key and session transitions
description: Security invariants for enabling MFA and migrating encrypted TOTP secrets to a dedicated key.
---

When enabling MFA, revoke every active session in the same database transaction as the enrollment confirmation, then clear the current browser cookie so the user signs in again after saving recovery codes. Keep the legacy session-derived TOTP key as a fallback while encrypted secrets migrate lazily to the dedicated key; do not remove that fallback or rotate the session secret without a deliberate migration plan.

**Why:** The user chose revoking all active sessions as the recommended protection, and existing TOTP ciphertext may have been encrypted before the dedicated key was configured.

**How to apply:** Preserve atomic session revocation in enrollment changes and retain legacy decryption until a separately planned key migration proves no legacy ciphertext remains.
