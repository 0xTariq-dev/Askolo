---
name: Linear task migration
description: Convention for copying canonical Replit work into the Askolo Linear project.
---

Linear is the project-management source of truth for this project. When migrating canonical Replit work into Linear, create one issue in the Askolo project, map a Replit Draft to Linear Backlog, preserve the task title and full plan, and append explicit provenance including the Replit task reference. Keep the Replit record as historical provenance and do not create duplicate Linear issues.

**Why:** The project now uses Linear for active planning, ownership, status, and future task creation while retaining Replit references for traceability.

**How to apply:** Search the target Linear project by title and source reference before creating or updating work. New project tasks belong in Linear first; use the Replit board only to recover historical scope and provenance.

Use a team-scoped `Replit` source label, the single-select `Migration` label group (`Imported`, `Needs review`, `Verified`), and the single-select `Disposition` label group (`Canonical`, `Superseded`, `Duplicate`, `Rejected`). Canonical migrated issues receive `Replit`, `Canonical`, and `Imported` during creation; move `Imported` to `Verified` after read-back confirmation.

**Why:** Source provenance and migration state need to remain searchable without overloading workflow status or issue descriptions.

**How to apply:** Reuse the existing groups and labels on later imports. Do not create parallel labels with different spelling, apply both lifecycle values at once, or mark superseded source records as canonical.

## Archiving Linear issues

Archiving an issue may clear its blocking and related links on both the archived issue and active counterpart issues. This can change active sequencing, not just hide the archived item.

**Why:** During reconciliation, archiving a stale issue cleared its dependencies and unblocked a surviving issue whose description still assumed the old prerequisite.

**How to apply:** Before archiving, read the issue's blockers, dependents, and related issues. After archiving, read each affected active issue back and decide whether any relationship should remain; never restore a link to archived scope without an explicit reason.

## Askolo-dev production-release review

The Askolo-dev project has a “Production Release” milestone for these grouped review issues:

- Review legacy source references in existing issue descriptions
- Clarify backlog ownership, phase disposition, and automation dependencies
- Decide whether production requires service-wide SLOs and on-call coverage

Keep these review issues in Backlog with no priority or assignee until explicit decisions are made. Treat service-wide SLO and on-call expectations as a conditional decision, not an assumed release requirement.

**Why:** The review needs a findable record without silently changing existing ownership, priority, dependency, or release-scope decisions.

**How to apply:** Search the Askolo-dev project by the milestone and issue titles above. Only update existing issue fields or relations after the review records an explicit decision.