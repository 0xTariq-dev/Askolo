---
name: Home Gmail incremental sync
description: Product and privacy decisions for Home Gmail count synchronization.
---

The initial Home Gmail summary baseline covers the 30 days before account linking. Subsequent refreshes should use a persisted Gmail history cursor and resumable pagination. Keep last-known counts visible with an error state; notify in-app only when the user must reconnect or restore permission, not for transient provider failures.

Per-thread state may be persisted only as a purpose-separated HMAC token and minimal count-relevant metadata. Do not store raw Gmail message/thread IDs, sender addresses, subjects, or message bodies. Derive a dedicated HMAC subkey from `AUTH_RATE_LIMIT_HMAC_SECRET`; never use that secret directly for thread tokens. Version the token scheme and rebaseline the 30-day window after root-secret rotation. No new secret is required for the current design.

**Why:** the user approved this bounded lookback and HMAC-keyed state to avoid full-mailbox reads while enabling incremental count updates without retaining raw provider identifiers or content.

**How to apply:** implement cursor persistence, page checkpoints, per-thread token state, expiry of data outside the 30-day window, and action-required notification handling together; do not claim incremental sync is complete until these are wired end to end.
