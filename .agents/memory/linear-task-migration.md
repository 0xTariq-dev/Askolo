---
name: Linear task migration
description: Convention for copying canonical Replit work into the Askolo Linear project.
---

Linear is the project-management source of truth for this project. When migrating canonical Replit work into Linear, create one issue in the Askolo project, map a Replit Draft to Linear Backlog, preserve the task title and full plan, and append explicit provenance including the Replit task reference. Keep the Replit record as historical provenance and do not create duplicate Linear issues.

**Why:** The project now uses Linear for active planning, ownership, status, and future task creation while retaining Replit references for traceability.

**How to apply:** Search the target Linear project by title and source reference before creating or updating work. New project tasks belong in Linear first; use the Replit board only to recover historical scope and provenance.

Use a team-scoped `Replit` source label plus a single-select `Migration` label group. The initial lifecycle labels are `Imported` and `Verified`; move a successfully checked copy to `Verified` after read-back confirmation.

**Why:** Source provenance and migration state need to remain searchable without overloading workflow status or issue descriptions.

**How to apply:** Reuse the existing group and labels on later imports. Do not create parallel labels with different spelling or apply both lifecycle values at once.