---
name: Database query consolidation
description: Captures Askolo's database redesign goal of refining the model and reducing per-page API calls.
---

Askolo's database redesign should create a more refined database and minimize API calls per page to limit backend overload. UI data loading must align with the approved post-refactor API contracts; do not assume the current generated hooks are the final contract.

For unresolved product, database, or API decisions in this refactor, ask the user instead of making assumptions.

Do not create canonical V2 database views until the Today/Plan UI projections are defined with the user. Update the related Linear task first, then align API and database read projections to the approved UI. Do not apply staged migrations to Development or Production before that design step.

**Why:** the user stated that both database refinement and reducing per-page API calls are central goals of the redesign, and specifically requested that UI views be defined before database views or migration application.

**How to apply:** when changing page data loading, API hooks, query shapes, or other unresolved refactor decisions, update the relevant Linear issue and wait for the approved UI/read contract rather than guessing at endpoints or schema.
