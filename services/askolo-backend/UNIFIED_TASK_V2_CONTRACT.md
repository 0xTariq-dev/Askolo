# Unified Task V2 Contract

Status: implementation baseline for the additive V2 schema and API.

## Scope and compatibility

V2 adds a canonical task model and a versioned API alongside the existing stable API. The current public schema and existing `/api` behavior are not changed. Migration 0016 remains checksum-pinned; later V2 schema changes use a forward staged migration under the isolated `public_v2` schema.

Askolo is not limited to families or households: a Space can represent a family or a professional team, with Spaces and invitations as core concepts. Use “Space” as product-facing terminology. The current technical schema and API use workspace-scoped identifiers; keep that technical contract explicit unless separately changed.

Home is the concise, factual current-day roll-up for the selected Space, not a list of every task, event, email, and Plan entry or an AI-written narrative by default. It shows at most three focus actions, with each user's choices saved per Space and Plan day. In All Spaces, combine eligible actions across authorized Spaces, showing saved choices first and filling remaining slots with suggestions. Eligible suggestions include unfinished items in that Space's Plan for the day, that day's habit occurrences, and any overdue open task in the Space. Its Email summary shows unread Inbox and action-needed counts (no individual email list); action-needed includes threads whose latest message is from someone else and messages flagged or starred. A message may count in both totals. Its Calendar summary shows the current day's event count and next event/time, counting multi-day events on each day they overlap. Other dates are available in Agenda, not by changing the day on Home. Home's inline Agent entry is a quick way to access the same full Agent, not a day-limited assistant; it may start with day context but retains the Agent's broader context within the active scope and normal authorization/privacy boundaries.

All Spaces is a selection in the existing Space dropdown, not a separate projection or sidebar tab. Its Home read is a server-side authorized aggregation across accessible Spaces, not browser fan-out; it labels items by Space and returns email totals both combined and broken down by Space. For this product model, provider accounts are owned by a Space, and users choose which connected accounts in each Space contribute to its summary. **Agenda** is the chosen broader product name. It combines Plan/task items and Calendar events in one full Calendar object based on ReUI's `c-event-calendar-1`; preserve the existing Month, Week, and Day views, previous/next navigation, Google Calendar sync, event management, month-date clicks that open Day view, and time-slot clicks that start event creation. The Today button returns to the current date in Day view, and the element's view dropdown switches views. All exposed settings controls must work. The new-event button opens a modal to create any supported event or task type. Keep events distinct from Tasks in the underlying model.

The Plan time zone is initialized from the creator's device time zone when the Plan is created and remains the default. Each member can override their own display time zone in Settings; a shared Space's agenda uses each viewer's effective time zone without changing another member's display.

For the target Home model, connected provider accounts are owned by a Space; the user selects which accounts in that Space contribute to its Home summary. This is provider-agnostic so future email and calendar providers fit the same model. Keep only minimal local mapping and display metadata needed for instant plotting and summary counts; this is not a local content store. Update summaries from provider change events/webhooks with scheduled sync as a fallback rather than making provider requests on every page refresh. Do not duplicate full provider content; retrieve full email content from its provider when the user opens a message.

Home references are examples, not fixed templates. Each user's layout is customizable independently for each Space. Reuse the design system's existing card-resizing and drag-and-drop ordering capabilities; a predefined widget catalog is future scope. Do not treat every card visible in a reference as required.

For a source schema named `public`, the migration runner resolves the staged schema token to `public_v2`. Schema-isolated tests use a source-specific `*_public_v2` name so they do not share objects.

The staged schema is not a complete copy of V1. It references the existing `public.users` identity table rather than creating a second user directory. It defines a separate workspace-level AI credit ledger, but does not copy V1 credit balances, grants, reservations, or history. The existing authentication/session system therefore remains a V1 dependency unless a separate identity boundary is designed.

There are no active users and existing data is disposable. This implementation does not convert or clean up legacy records, and it does not migrate the UI. V2 schema and API checks run against a fresh disposable PostgreSQL database.

The user-confirmed Home and Agenda read choices are recorded in `HOME_TODAY_PLAN_PROJECTION_BRIEF.md` and in this contract. They do not authorize creating canonical V2 views or applying staged migrations. The existing `workspace_agenda_items` and `workspace_overview` views in migration 0016 are legacy projections, not the new UI contract. Keep migration 0016 unchanged; any later change to those legacy views must be a forward migration.

A screen-level brief is maintained in `HOME_TODAY_PLAN_PROJECTION_BRIEF.md`. It records product behavior; this file defines the V2 read contract. Neither document authorizes creating views or applying migrations.

## Domain model

### Tasks

`Task` is the single canonical model for actionable work. Its initial kinds are:

- `action` — an action item or general next step
- `chore` — household or workspace upkeep
- `habit` — a repeated behavior tracked by occurrence
- `commitment` — a recurring or one-time responsibility

Kind does not determine recurrence. All kinds may be one-time or recurring. A task owns its title, details, creator, optional assignee, priority, lifecycle, optional due date, optional goal link, optional parent task, and source provenance. Task identity and state are never copied into a Plan.

Task lifecycle values are `open`, `in_progress`, `done`, `cancelled`, and `archived`. A recurring task remains open or in progress while its individual occurrences are completed. A recurring task is stopped by archiving it; completing an occurrence does not complete the task definition.

Dates are calendar dates, not timestamps. A planned time is a separate timestamp. Recurrence uses an IANA time-zone name and local calendar dates so daylight-saving changes do not shift an occurrence to another local day.

### Recurrence and occurrences

The first recurrence contract supports `daily`, `weekly`, and `monthly` frequency, a positive interval, an inclusive start date, an optional inclusive end date, and a time zone. Weekly rules may include selected weekdays; otherwise they use the start date's weekday. Monthly rules use the start date's day of month and skip months that do not contain that day. Occurrences are generated only for requested date ranges; there is no background scheduler.

`TaskOccurrence` is identified by task ID and local occurrence date. It stores the occurrence status (`done`, `skipped`, or `cancelled`), completion actor/time, and any occurrence-specific due-time override. An occurrence without a stored override is open. Completing or reopening an occurrence never changes the recurring task definition. Non-recurring tasks use their Task lifecycle status and do not require synthetic occurrence rows.

Parent tasks and dependency links refer to canonical tasks in the same workspace. A task cannot be its own parent or dependency, and dependency cycles are rejected.

### Goals

Goals are long-term destinations, not agenda tasks. A task may reference zero or one goal in the same workspace. Goal progress and lifecycle remain independent from task completion; completing a task does not silently rewrite goal progress.

### Plans

A Plan has an owner, title, period, time zone, lifecycle, and visibility. Visibility is `personal` or `shared`. Personal Plans are visible to their owner and workspace owners/admins. Shared Plans are visible to active workspace members. Workspace owners/admins may change a Plan to shared or personal.

A Plan entry links to one existing Task and, optionally, one local occurrence date. It may add a planned date/time and ordering position. It never stores a duplicate title, status, or assignee. A Task may appear in multiple plans; all entries reflect the same canonical Task state.

When a Plan day ends, automatically carry unfinished items with no due date to the next day. For unfinished items with a due date, ask the user individually before carrying each one forward; without approval, leave it on its original date.

Assignments are stored on the Task. Assigning from a Plan updates that Task's `assignedToUserId`; it does not assign a copy or change unrelated Plan entries.

### Calendar and agenda

The bounded agenda projection combines:

1. Tasks due in the requested date range, including generated recurrence occurrences.
2. Plan entries scheduled in the requested range, linked to the canonical Task.
3. Calendar event records.

Calendar events remain distinct from Tasks. Provider-owned events preserve both provider and source event IDs. The projection identifies each item kind and includes the source IDs for event records. A Plan entry is a scheduling relationship, not a new task.

The V2 agenda endpoint accepts up to 42 inclusive dates. It requires the viewer's effective IANA time zone, uses local midnight boundaries for calendar events, and includes events that overlap the requested range even if they started earlier. Task occurrences, Plan entries, and events are paged without silently truncating the result; follow `nextPageToken` until it is omitted. Each response contains at most 500 items per category. A token is bound to the user, role, Space, range, and time zone. If the underlying task batch changes enough to invalidate its offset, the API rejects the token and the client restarts the range. This supports the full Month grid without a view or migration.

### Reminder definitions

Reminder rules are stored as user-defined, inactive configuration for task reminders, goal reminders, and daily briefs. New rules have state `pending_automation`. The V2 API cannot activate delivery. No notification, email, daily brief, or other automation is dispatched by this implementation.

## API contract

All endpoints are session-authenticated and workspace-scoped. Paths use `/api/v2/workspaces/{workspaceId}`. Every request verifies that the actor has an active membership in the path workspace. The active workspace and membership records remain the authorization source of truth; after an actor is authorized, the current workspace and memberships are synchronized into the isolated V2 schema so V2 foreign keys and shared-plan access stay current. This is request-scoped synchronization, not a bulk legacy-data conversion. Resource IDs from another workspace are treated as unavailable. Writes record the acting user.

### Endpoints

| Method and path | Purpose |
| --- | --- |
| `GET /api/v2/healthz` | V2 schema readiness; does not block the stable app's health check. |
| `GET /api/v2/workspaces/{workspaceId}/tasks` | List tasks with bounded pagination and optional kind, status, assignee, and date filters. |
| `POST /api/v2/workspaces/{workspaceId}/tasks` | Create a task. |
| `GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}` | Read a task and its relationships. |
| `PATCH /api/v2/workspaces/{workspaceId}/tasks/{taskId}` | Partial task update; omitted fields are unchanged and explicit `null` clears nullable fields. |
| `DELETE /api/v2/workspaces/{workspaceId}/tasks/{taskId}` | Archive a task; task history and plan references remain available. |
| `GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}/occurrences` | List recurrence occurrences for a bounded date range. |
| `PATCH /api/v2/workspaces/{workspaceId}/tasks/{taskId}/occurrences/{date}` | Complete, skip, cancel, or reopen one occurrence. |
| `GET, POST /api/v2/workspaces/{workspaceId}/goals` | List and create goals. |
| `GET, PATCH, DELETE /api/v2/workspaces/{workspaceId}/goals/{goalId}` | Read, update, or archive a goal. |
| `GET, POST /api/v2/workspaces/{workspaceId}/plans` | List and create plans. |
| `GET, PATCH, DELETE /api/v2/workspaces/{workspaceId}/plans/{planId}` | Read, update, or archive a plan. |
| `POST /api/v2/workspaces/{workspaceId}/plans/{planId}/entries` | Link a task or occurrence to a plan and set schedule/position. |
| `PATCH, DELETE /api/v2/workspaces/{workspaceId}/plans/{planId}/entries/{entryId}` | Update schedule or remove a plan link. |
| `GET /api/v2/workspaces/{workspaceId}/agenda?from=YYYY-MM-DD&to=YYYY-MM-DD&timeZone=Area%2FCity&pageToken=...` | Inclusive, up-to-42-day Task, Plan-entry, and overlapping-event projection with complete cursor pagination. `pageToken` is optional. |
| `GET, POST /api/v2/workspaces/{workspaceId}/reminder-rules` | List and create inactive reminder definitions. |
| `PATCH, DELETE /api/v2/workspaces/{workspaceId}/reminder-rules/{ruleId}` | Update or delete an inactive definition; activation is not supported. |

### Home summaries and caching

The Home/All Spaces read must be authorized and aggregated on the server. The result includes each Space's email summary plus a combined All Spaces total. Unread counts cover Inbox messages; action-needed includes flagged/starred messages and threads whose latest message is from someone else. The two counts may overlap. Account selection is scoped to a Space-owned provider account. Provider events/webhooks update the local summary mapping, with scheduled sync as fallback.

Do not cache sensitive aggregate payloads in a shared server cache, browser cache, or CDN until there is a shared invalidation/version mechanism. Protected V2 responses use `Cache-Control: private, no-store`. First optimize with authorization-aware aggregation and bounded, indexed queries. If caching is added later, reauthorize before lookup and include the viewer, currently authorized Spaces, selected provider accounts, date range, effective time zone, and data/access versions in the cache key. Invalidate on membership/access changes, account selection or disconnect, task/calendar writes, and provider sync events. Never cache message bodies or credentials.

The Home/All Spaces projection, Space-owned provider-account selection/sync mapping, and per-user daily focus selection are product-contract decisions, not implemented by this Agenda change. The existing legacy provider-account tables remain user-scoped. Implement those capabilities only with their approved provider/account and focus foundations; do not infer that the current Home/API already supports them.

Agenda continuation tokens are signed and bound to the authorized user, role, workspace, date range, and time zone. Each page request still performs normal authorization. Pages use deterministic ordering but do not hold a database snapshot across requests; clients should restart the range after task, plan, event, or provider-sync changes.

List endpoints default to at most 50 items and accept a maximum of 100. Request bodies are limited to 64 KiB. Invalid enum values, dates, recurrence rules, time zones, cross-workspace references, unknown fields, and unsupported reminder activation requests fail with a client error. Writes are transactional where they affect more than one row.

### Authorization

- All actors must have an active account/session and active workspace membership.
- Members may read shared workspace resources according to their V2 permissions.
- A personal Plan is visible to its owner and workspace owners/admins; a shared Plan is visible to active members.
- Only workspace owners/admins may change Plan visibility.
- A task creator may edit and assign their Task. Workspace owners/admins may manage Tasks in their workspace. An assignee may complete or reopen their own task occurrence.
- Plan-entry writes require permission to edit the Plan. Assignment from a Plan uses the same Task authorization and storage as direct Task assignment.
- Cross-workspace or inactive membership access fails closed.

## Response examples

### Task

```json
{
  "id": "task_123",
  "workspaceId": "workspace_123",
  "kind": "habit",
  "title": "Walk after lunch",
  "details": null,
  "status": "open",
  "priority": "medium",
  "createdByUserId": "user_123",
  "assignedToUserId": "user_123",
  "goalId": null,
  "dueOn": null,
  "recurrence": {
    "frequency": "weekly",
    "interval": 1,
    "weekdays": [1, 3, 5],
    "startsOn": "2026-10-12",
    "endsOn": null,
    "timeZone": "Africa/Cairo"
  },
  "parentTaskId": null,
  "dependencies": [],
  "createdAt": "2026-10-09T12:00:00Z",
  "updatedAt": "2026-10-09T12:00:00Z"
}
```

### Agenda item

```json
{
  "kind": "task_occurrence",
  "taskId": "task_123",
  "occurrenceOn": "2026-10-12",
  "title": "Walk after lunch",
  "status": "open",
  "plannedStartAt": null,
  "dueOn": "2026-10-12",
  "planId": "plan_123",
  "assignedToUserId": "user_123"
}
```

A calendar event uses `kind: calendar_event`, its own event ID, and nullable source provider/source event ID fields. It is never returned as a task occurrence.

## Contract-test matrix

| Area | Required checks |
| --- | --- |
| Task kinds and validation | All four kinds accepted; unknown kinds and malformed fields rejected. |
| Recurrence | Daily/weekly/monthly expansion; interval, end date, leap/month-end, weekday, and time-zone boundaries. |
| Occurrence history | Complete, skip, cancel, reopen, actor/time attribution, and idempotent replay. |
| Relationships | Optional Goal link, parent task, dependencies, cross-workspace rejection, self-link and cycle rejection. |
| Plans | Personal/shared visibility, admin share controls, linked entries, assignment from a Plan updates the Task, and no copied task state. |
| Agenda | Tasks, generated occurrences, Plan scheduling, and provider events appear with distinct kinds and stable source IDs. |
| Reminders | CRUD leaves every rule pending automation; no dispatch or active state is possible. |
| Compatibility | Existing stable routes and active migration prefix remain unchanged; V2 staged migration applies only in disposable PostgreSQL tests. |
| Abuse and errors | Body/range/list caps, strict JSON and query validation, parameterized SQL, timeouts, safe error responses, and no task/event content in logs. |
