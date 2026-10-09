# Home and Agenda Projection Brief

**Status:** Home and Agenda read behavior confirmed; one focus-suggestion detail remains open
**Purpose:** Record screen behavior and the information each projection needs. This does not authorize canonical database views or migrations.

This is a screen-level product brief. It does not define API endpoints, response schemas, SQL views, migrations, or implementation details.

## 1. Shared Space and Agent scope

- A **Space** can represent a family or a professional team. Use “Space” in product-facing language; keep existing workspace-scoped technical identifiers unchanged unless separately approved.
- **Home** is the concise daily view for the currently selected Space. **All Spaces** is an option in the existing Space dropdown, not a separate projection or sidebar tab. Its authorized aggregation happens on the server and labels each item with its Space; it is not a browser-side fan-out of Space requests.
- The Agent follows the selected Space or All Spaces scope. It must not silently widen that scope. Shared or assigned items should identify their ownership where relevant.

## 2. Home projection

Home is a concise, factual roll-up, not a list of every task, calendar event, email, and Plan entry. It is not an AI-written narrative by default.

Initial Home content:

1. **Focus actions:** store selections per user, Space, and Plan day. In All Spaces, combine eligible actions across authorized Spaces. Show saved choices first; fill the remaining slots with suggestions, up to three total. Eligible suggestions include unfinished items already in that Space's Plan for the day, that day's habit occurrences, and any overdue open task in that Space. Do not schedule suggestions without approval.
2. **Email summary:** show unread Inbox and action-needed counts. Action-needed includes threads whose latest message is from someone else and messages flagged or starred. A message may count in both totals. Do not list individual emails in this card. Each provider account has one owning user and one workspace association when connected to Askolo. Only a Space owner or admin may promote it for access by that Space; promotion does not transfer ownership or credentials or add another workspace association. Select promoted accounts per Space for its summary. The user chose to let active Space members read full message content from a promoted account on demand, but only when both Space owner/admin promotion and explicit account-owner consent are present. Promotion alone shares summary counts. Keep the Home card summary-only, fetch message content from the provider, and do not persist it. Treat this as read-only; send/delete/draft actions and attachments are outside this scope.
3. **Calendar summary:** show the current day's event count and the next event with its time. Count an event on each day it overlaps, including multi-day events.
4. **Contextual Agent:** provide an inline, day-aware entry on Home. It is a quick way to access the same full Agent, not a separate day-limited assistant. The Agent can use all information available within the active Space or All Spaces scope, subject to normal authorization and privacy boundaries. Keep the full Agent as a separate page rather than opening the global drawer from Home.

Home always shows the focused current day; other dates are available in Agenda, not by changing the day on Home. Use the viewer's effective time zone for day boundaries and event display. The Plan's time zone is the default and is initialized from the creator's device time zone when the Plan is created. Each user can override their own display time zone in Settings, so members of a shared Space see their agenda aligned to their own effective time zone.

## 3. Agenda projection

**Agenda** is the chosen broader product name. It combines daily planning and the full existing Calendar in one calendar object; it is not a Plan-only replacement or two separate side-by-side panels. Use the ReUI event-calendar example `c-event-calendar-1` as the base.

Preserve the existing Calendar's Month, Week, and Day views, previous/next navigation, Google Calendar sync, event create/edit/delete behavior, month-date clicks that open Day view, and time-slot clicks that start event creation. The Today button returns to the current date in Day view; the view dropdown in the calendar element switches among views. All settings controls exposed by the element must work.

The new-event button opens a modal that can create any supported event or task type. Show Plan/task items and calendar events together in the same calendar object while keeping their underlying data types distinct. Within a day, retain the agreed emphasis order: focus actions; scheduled events and timed tasks in chronological order; then flexible tasks and habits.

Planning items should reflect canonical task state across Home, the combined Calendar/planning surface, and other projections. A Plan entry links to its existing Task rather than duplicating the task's title, status, or assignee. Provider-owned calendar events remain events, not editable tasks.

Suggest open calendar slots for unscheduled items, but do not schedule them without the user's approval.

### End-of-day carry-forward

- Automatically move unfinished Plan items with no due date to the next day.
- Ask separately before moving each unfinished item with a due date.
- Without approval, leave a due-dated item on its original date.

## 4. Per-user and per-Space layout

- Each user can customize their Home layout independently in each Space.
- Reuse existing design-system support for card resizing and drag-and-drop ordering.
- A predefined widget catalog is deferred. Reference screenshots are examples, not a required card set or fixed layout.

## 5. Information needed by each projection

This list describes information the screens need, not an API or database schema.

| Projection | Information needed |
| --- | --- |
| Home | Selected Space; focused current day in the viewer's effective time zone; that user's layout and focus selections for the Space/day; up to three selected or suggested actions; unread Inbox and action-needed email totals; calendar events overlapping the day; and the next event and its time. |
| Agenda | One calendar object containing linked Plan/task items and distinct calendar events; the current Month/Week/Day range; scheduled task times; flexible task and habit occurrences; and any carry-forward approval decision. |
| Space dropdown set to All Spaces | Server-authorized tasks and Plan items, calendar events, combined email totals, and per-Space email breakdowns, labeled by Space and relevant ownership/assignment. |
| Home Agent entry | Selected Space or All Spaces scope and the relevant day as entry context, while retaining the same full Agent context available within that scope. |

Provider systems remain the source of truth for provider-owned content. Keep only a minimal local mapping and display metadata needed for instant plotting and summary counts; this is not a local content store. Update summaries from provider events/webhooks with scheduled sync as fallback rather than requiring a provider network request on every page refresh. Do not duplicate full email or calendar content.

## 6. Open decisions and approval gate

Remaining detail:

- Focus suggestions may include any overdue open task in the selected Space, even if it is outside today's Plan. The user-selected focus actions stay first; unfinished Plan items and today's habit occurrences are also eligible, with suggestions filling the remaining slots up to three total.
- The Agenda API supports the full 42-day inclusive Month range and cursor pagination. Its event query uses the viewer's effective time zone and includes events overlapping the range. Page tokens are signed and scope-bound; pages are live reads rather than a cross-request snapshot, so restart after writes or sync updates.
- Each provider account has one owning user and one workspace association when connected to Askolo. Only Space owners/admins may promote the connection for Space access; credentials remain user-scoped and promotion does not add another workspace association. Promotion grants active Space members read-only, provider-backed access to full message content on demand. Promoted-account selection and summaries are scoped per Space. Existing legacy provider-account tables are user-scoped; the promotion/sync foundation, shared message-read routes, and server-side All Spaces email-summary endpoint are not implemented by this change.
- Full-content sharing requires both account-owner consent and owner/admin promotion; consent is scoped to the associated Space. The current Google OAuth code models `gmail.send` and uses it for message reads; a separate, least-privilege read grant and explicit incremental consent must be verified before shared reading is implemented.
- Persisted per-user focus choices are also not implemented here; eligibility is now settled, but selection persistence and the suggestion query remain future work.

Do not cache sensitive aggregate payloads until shared invalidation/versioning exists. Protected V2 HTTP responses use `private, no-store`; optimize with bounded indexed queries first. Any future cache must reauthorize and be scoped to viewer, authorized Spaces, selected provider accounts, date range, effective time zone, and data/access versions. Invalidate it on membership changes, account-selection changes, writes, and provider sync events. Never cache message bodies or credentials.

The product read choices above do **not** approve creating canonical database views, staging migrations, or applying migrations to Development or Production. Keep migration 0016 unchanged.
