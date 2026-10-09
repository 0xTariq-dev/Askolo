---
name: Askolo planning model
description: User-defined boundaries for the unified task, goal, plan, calendar, and reminder model.
---

Treat chores, recurring commitments, habits, and action items as types within one canonical Task model. Tasks may recur, have due dates, and support subtasks or dependencies; preserve per-occurrence completion history.

Goals are longer-term destinations. A task may optionally link to a goal, but a goal is not an agenda task.

Plans organize tasks for a time period. A plan entry links to an existing task when it schedules that work; it must not clone the task or create duplicate task state.

Calendar is the broad agenda view over tasks, plans, and events. Provider-owned calendar events remain events, preserve their source identifiers, and are not converted into editable tasks.

Lay a foundation for user-defined reminders, but do not activate dispatch until an automation layer exists. That future layer may send item notifications, daily briefs, and other notifications.

**Why:** The user refined the product contract to favor a unified, low-friction planning model while preserving distinctions between goals, plans, and provider-owned events.

**How to apply:** Keep these boundaries consistent across the V2 schema, API, and tests. Refine type fields and relationships before implementation; do not migrate the UI until the new schema and API are verified stable.
