---
name: GitHub CLI Git transport
description: The GitHub CLI session and Git's HTTPS credential helper are separate authentication paths.
---

`gh auth status` can report a valid GitHub login while `git fetch` or `git push` still fails with invalid-token errors. Configure Git's credential helper from the existing CLI session with `gh auth setup-git` before refreshing or pushing GitHub remotes.

**Why:** Replit's GitHub connector, the `gh` CLI, and Git's HTTPS transport do not automatically share local credential-helper configuration; branch refreshes can otherwise look like merge conflicts when the real problem is authentication.

**How to apply:** When GitHub API access or `gh` works but Git operations fail, check `git config --get-all credential.helper`, run `gh auth setup-git`, then retry fetch before diagnosing branch divergence.