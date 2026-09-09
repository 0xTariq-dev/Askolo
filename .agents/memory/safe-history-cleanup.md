---
name: Safe history cleanup
description: Constraints for safely removing generated commits from local Git history
---

History cleanup tools should default to inspection, reject merge or non-empty commits unless explicitly reviewed, create a recovery ref before rewriting, and match abbreviated hashes from interactive rebase todo files against full object IDs.

**Why:** Generated deployment commits can be empty linear entries, but similarly named merge commits may carry real tree changes. Interactive rebase does not invoke a sequence editor unless `--interactive` is supplied.

**How to apply:** Use the cleanup tool only on a clean local branch, keep remotes untouched, and verify the rewritten branch plus its recovery ref in a disposable clone before using it on important history.