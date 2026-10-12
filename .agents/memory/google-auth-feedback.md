---
name: Google authentication feedback
description: Distinct, actionable user feedback for Google signup collisions and sign-in accounts without a linked Google identity.
---

For Google signup, report an existing Askolo account as an account-exists outcome and direct the user to sign in with their current method before linking Google. For Google sign-in without a linked identity, say Google sign-in is not set up for that account and direct the user to their existing sign-in method. Keep both messages concise and persistent in ReUI Alerts, not transient toasts.

**Why:** the user asked that signup and sign-in failures explain different next steps and remain visually prominent.

**How to apply:** preserve separate backend status values and corresponding login-page alert copy when changing Google OAuth or account-linking behavior.
