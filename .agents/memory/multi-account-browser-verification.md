---
name: Multi-account browser verification
description: Distinguishes fixture-based Profile browser coverage from live Google OAuth verification.
---

Fixture-based browser checks with synthetic auth and stubbed Google APIs prove the Profile's account-list states and responsive behavior only. They do not prove the real Development sign-in, Google consent handoff, scopes, or provider token behavior.

**Why:** Route-intercepted browser tests cannot validate external OAuth or a real Google account, even when the displayed UI is signed in.

**How to apply:** Label these checks as fixture-based and pair them with backend integration tests. Use a separate dedicated test-account session for live provider verification; never bypass auth or ask users to paste passwords or tokens into chat.
