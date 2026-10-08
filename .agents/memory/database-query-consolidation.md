---
name: Database query consolidation
description: Captures Askolo's database redesign goal of refining the model and reducing per-page API calls.
---

Askolo's database redesign should create a more refined database and minimize API calls per page to limit backend overload. UI data loading must align with the approved post-refactor API contracts; do not assume the current generated hooks are the final contract.

For unresolved product, database, or API decisions in this refactor, ask the user instead of making assumptions.

**Why:** the user stated that both database refinement and reducing per-page API calls are central goals of the redesign.

**How to apply:** when changing page data loading, API hooks, query shapes, or other unresolved refactor decisions, wait for or ask about the approved direction rather than guessing at endpoints or schema.
