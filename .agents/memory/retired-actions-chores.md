---
name: Retired Actions and Chores routes
description: Product decision for removing Actions and Chores routes and aliases without removing stored records.
---

Remove Actions and Chores routes and aliases entirely; do not redirect them. Preserve existing task and calendar data during route cleanup.

**Why:** The user said these surfaces are completely retired and do not need routes, while explicitly preserving existing task/calendar data.

**How to apply:** Remove navigation, page routes, API aliases, and OpenAPI operations for these retired surfaces. Do not add redirects or destructive data migrations.
