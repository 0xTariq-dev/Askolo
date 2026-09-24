---
name: Google OAuth scope preservation
description: Merge existing granted scopes with new token responses to avoid losing Calendar or Gmail access during refresh or incremental authorization.
---

Google's OAuth token refresh response may omit the `scope` field, and incremental authorization responses may return only newly granted scopes. The current persistence path overwrites stored scopes with the incoming value instead of merging them.

**Invariant:** When storing Google tokens, preserve all previously granted scopes by merging the existing and newly returned scope sets.

**Why:** Calendar access can appear disconnected while Gmail still works if an incremental response replaces the stored scope set.

**Status:** This is an unresolved implementation gap. Do not assume scope preservation is enforced until token storage merges scopes and regression tests cover both refresh and incremental authorization.

**How to apply:** When changing any Google token-persistence path, retain existing scopes when the provider omits `scope` and merge rather than replace when it returns a partial scope set.
