---
name: Google OAuth scope preservation
description: Merge existing granted scopes with new token responses to avoid losing Calendar or Gmail access during refresh or incremental authorization.
---

Google's OAuth token refresh response may omit the `scope` field, and incremental authorization responses may return only the newly granted scope. If the stored scope is overwritten, the app will report Calendar or Gmail as disconnected even though the user previously granted access.

**Rule:** when storing Google tokens, merge the existing scope string with the new scope string, preserving all previously granted scopes.

**Why:** We saw Calendar appear disconnected while Gmail worked because the stored scope was overwritten and lost. Merging scopes ensures incremental authorization and token refresh do not revoke previously granted access.

**How to apply:** Any function that persists Google tokens (e.g., `storeGoogleTokens`) must use `mergeScopes(existingScope, newScope)` instead of `newScope` alone.
