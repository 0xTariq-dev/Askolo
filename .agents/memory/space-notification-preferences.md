---
name: Space notification preferences
description: Product rules for Space membership, assignment, and reminder notification audiences and delivery.
---

Notification channels are email and in-app. Membership and role-change notifications are only for Space owners and admins. Assignment and reminder choices are per-user preferences; task @mentions are deferred. Saving task or habit reminder preferences does not activate scheduled reminder delivery.

**Why:** the user selected these audiences and channels, and chose to keep scheduled reminder delivery inactive for this release.

**How to apply:** expose persisted preferences in Space settings, enforce the owner/admin audience for membership notices, and keep reminder dispatch disabled until a scheduler is explicitly approved and scoped.
