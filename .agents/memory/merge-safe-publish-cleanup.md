---
name: Merge-safe publish cleanup
description: Preserve merges while removing an empty publish commit when branches contain duplicate patch-equivalent commits.
---

For a merge whose branches contain the same non-empty patch, replay the canonical patch once and make it the shared base for both rewritten branches. Then replay each side's remaining commits and recreate the merge. Do not flatten the merge, drop only the side-branch patch when later commits rely on its files, or resolve conflicts with a blanket ours/theirs choice. The generic cleanup script should refuse ranges with merges or duplicate patches unless it can prove a topology-preserving rewrite.

**Why:** Dropping the side copy while keeping its old fork point can make later modifications fail as modify/delete conflicts. Replaying the same patch independently on both branches retains duplicate effects in history and can obscure what the merge introduced.

**How to apply:** Before an authorized history rewrite, protect the source tip, work on a detached candidate, preserve the merge structure, and verify the final tree, patch inventory, merge base, and blocked subjects before moving any branch ref.