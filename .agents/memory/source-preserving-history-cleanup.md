---
name: Source-preserving history cleanup
description: Safe behavior for removing generated commits without destabilizing the active branch
---

History cleanup should create a new local branch from an isolated temporary worktree and leave the source branch untouched. Removing a middle commit still changes descendant IDs, but the active branch and its remote tracking state remain stable.

**Why:** Rewriting a checked-out or remote-tracking branch makes the Git UI appear unsupported and creates large ahead/behind divergence even when later file changes are preserved.

**How to apply:** Require one linear empty commit, keep a recovery ref, and let the user compare or explicitly replace the source branch after reviewing the cleaned branch.