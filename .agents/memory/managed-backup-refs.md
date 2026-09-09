---
name: Managed backup refs
description: Constraints on cleaning branches in the workspace's gitsafe backup remote.
---

The `gitsafe-backup` remote is managed and its receive hook rejects all pushes except updates to `main`, including deletion requests for `main-old-*` recovery refs.

**Why:** These refs are protected by the backup service rather than normal Git repository permissions, so attempting to delete them through Git cannot complete.

**How to apply:** Preserve the backup refs unless the backup service provides its own administrative cleanup flow. Do not treat a rejected deletion as successful, and do not bypass the receive hook.