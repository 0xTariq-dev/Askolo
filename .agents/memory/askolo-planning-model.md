---
name: Askolo planning model
description: User-defined boundaries for the unified task, goal, plan, calendar, and reminder model.
---

Task and Habit are the only canonical responsibility types. A Task is finished; a Habit is practiced. Bills, messages, calls, and recurring house jobs are Tasks. Chore is a label, not a type; provide preset labels (Home, Bills, Work, Family) and user-defined labels.

Tasks have a planned date distinct from an optional real deadline, and may recur with no end, an end date, or an occurrence count. Habits use daily, selected-weekday, or N-times-per-week cadence, with an optional end date. Habits have no priority; Tasks support low/normal/high and default to no priority. Goals remain optional links.

Store each recurrence rule once and expand it on demand, with a bounded query window; persist only occurrence exceptions/completions. A missed Habit does not roll over. A Task occurrence remains overdue until done or skipped, then closes as missed when its next interval begins. Clamp monthly days 29–31 to each month's last day. Series edits support one occurrence, this-and-following, and all occurrences. Preserve local wall-clock intent with the user's time zone.

Time blocks are local start plus duration, may cross midnight, belong to the start-date occurrence, and appear on every Agenda day they overlap. Cap at 24 hours and less than the recurrence gap. Do not add a day-start preference now; account for it when onboarding captures it later.

Goals are longer-term destinations. A task may optionally link to a goal, but a goal is not an agenda task.

Plans organize tasks for a time period. A plan entry links to an existing task when it schedules that work; it must not clone the task or create duplicate task state.

Support both personal and shared plans. Workspace owners and admins may share plans with workspace members. Task assignment applies to the canonical task whether initiated from a plan or the task itself.

Calendar is the broad agenda view over tasks, plans, and events. Provider-owned events remain distinct and preserve provider, connection/account, calendar, and event identifiers. Use that full source tuple when refreshing events so IDs from different calendars do not collide; local CRUD must not mutate provider-owned rows. Provider writes require explicit user opt-in.

The Plan tab is labeled Actions as an umbrella for Task and Habit records. Actions API routes are aliases for Plan callers; retain the canonical task database model and task-specific routes.

**Why:** The Plan label describes the user-facing surface, not a new data type. Renaming the persisted Task/Habit model would create unnecessary schema and compatibility changes.

**How to apply:** Keep Plan mutations on Actions-named API functions/routes, while task-specific consumers continue using the existing Task model and routes.

**Why:** provider event IDs can collide across connected accounts and calendars, and the provider remains the owner of those events.

**How to apply:** upsert by the full source tuple and keep local event IDs stable across refreshes. Do not issue provider writes until the user has explicitly opted in.

Support multiple connected accounts and multiple calendars per account. Use consistent account filters across provider-backed features and calendar filters wherever calendar data appears.

Provider write scopes are opt-in; read-only remains the default. The Agent requires explicit confirmation before sends or deletes, uses idempotency keys for writes, and records every action in a privacy-conscious audit trail.

Lay a foundation for user-defined reminders, but do not activate dispatch until an automation layer exists. That future layer may send item notifications, daily briefs, and other notifications.

**Why:** The user refined the model to separate completed work from practiced habits, make recurring behavior explicit, preserve provider ownership, and require predictable multi-account filtering and safe Agent writes.

**How to apply:** Keep these boundaries consistent across the V2 schema, API, and tests. Refine type fields and relationships before implementation; do not migrate the UI until the new schema and API are verified stable.
