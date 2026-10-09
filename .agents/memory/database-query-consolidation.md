---
name: Database query consolidation
description: Captures Askolo's database redesign goal of refining the model and reducing per-page API calls.
---

Askolo's database redesign should create a more refined database and minimize API calls per page to limit backend overload. UI data loading must align with the approved post-refactor API contracts; do not assume the current generated hooks are the final contract.

For provider-owned Gmail and Calendar data, keep only the minimal local mapping and display metadata needed for instant plotting and summary counts; this is not a local content store. Update it when new or changed provider data arrives rather than making a provider network request on each page refresh. Keep the provider as the source of truth and do not duplicate full email or calendar content; retrieve full email content when opened.

For unresolved product, database, or API decisions in this refactor, ask the user instead of making assumptions.

Do not create canonical V2 database views until the Today/Plan UI projections are defined with the user. Update the related Linear task first, then align API and database read projections to the approved UI. Do not apply staged migrations to Development or Production before that design step.

**Why:** the user stated that database refinement and reducing per-page API calls are central goals, specifically requested UI views before database views or migration application, and clarified the performance boundary for provider-owned data.

**How to apply:** when changing page data loading, API hooks, query shapes, or other unresolved refactor decisions, update the relevant Linear issue and wait for the approved UI/read contract rather than guessing at endpoints or schema. Use a minimal provider-data mapping for quick rendering and refresh it on provider changes, not every page load.
