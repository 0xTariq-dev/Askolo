---
name: Linear task migration
description: Convention for copying canonical Replit work into the Askolo Linear project.
---

When copying a canonical Replit task into Linear, create one issue in the Askolo project, map a Replit Draft to Linear Backlog, preserve the task title and full plan, and append explicit provenance including the Replit task reference. Leave the Replit task unchanged during the migration test.

**Why:** Linear is being tested as the external project-management source of truth, but the Replit task board remains the canonical source until the migration is verified.

**How to apply:** Search the target Linear project by title and source reference before creating an issue. Avoid duplicate issues and do not archive or edit the originating Replit task as part of the copy.

Use a team-scoped `Replit` source label plus a single-select `Migration` label group. The initial lifecycle labels are `Imported` and `Verified`; move a successfully checked copy to `Verified` after read-back confirmation.

**Why:** Source provenance and migration state need to remain searchable without overloading workflow status or issue descriptions.

**How to apply:** Reuse the existing group and labels on later imports. Do not create parallel labels with different spelling or apply both lifecycle values at once.