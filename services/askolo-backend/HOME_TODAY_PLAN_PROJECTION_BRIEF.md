# Home and Agenda Projection Brief

**Status:** User choices captured; API read-needs mapping is next
**Purpose:** Agree on the screens and the information they need before designing canonical API read contracts or V2 database views.

This is a screen-level product brief. It does not define API endpoints, response schemas, SQL views, migrations, or implementation details.

## 1. Shared Space and Agent scope

- A **Space** can represent a family or a professional team. Use “Space” in product-facing language; keep existing workspace-scoped technical identifiers unchanged unless separately approved.
- **Home** is the concise daily view for the currently selected Space. **All Spaces** is an option in the existing Space dropdown, not a separate projection or sidebar tab. Selecting it aggregates authorized items across Spaces and labels each item with its Space.
- The Agent follows the selected Space or All Spaces scope. It must not silently widen that scope. Shared or assigned items should identify their ownership where relevant.

## 2. Home projection

Home is a concise, factual roll-up, not a list of every task, calendar event, email, and Plan entry. It is not an AI-written narrative by default.

Initial Home content:

1. **Focus actions:** show user-selected items from the current day's Plan plus suggested items from that Plan, with at most three total. Do not schedule suggestions without the user's approval.
2. **Email summary:** show unread and action-needed counts. Action-needed includes messages awaiting the user's reply and messages flagged or starred. Do not list individual emails in this card. Gmail remains the source of truth; retrieve full email content from Gmail when the user opens an email.
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
| Home | Selected Space; focused current day in the viewer's effective time zone; that user's layout for the Space; up to three user-selected or suggested focus actions from that day's Plan; unread and action-needed email totals; calendar events overlapping the day; and the next event and its time. |
| Agenda | One calendar object containing linked Plan/task items and distinct calendar events; the current Month/Week/Day range; scheduled task times; flexible task and habit occurrences; and any carry-forward approval decision. |
| Space dropdown set to All Spaces | Authorized tasks and Plan items, calendar events, and email summary counts across Spaces, labeled by Space and relevant ownership/assignment. |
| Home Agent entry | Selected Space or All Spaces scope and the relevant day as entry context, while retaining the same full Agent context available within that scope. |

Gmail and Google Calendar remain the systems of record for provider-owned content. Keep only a minimal local mapping and display metadata needed for instant plotting and summary counts; this is not a local content store. Update the mapping when new or changed provider data arrives rather than requiring a provider network request on every page refresh. Do not duplicate full email or calendar content. Retrieve full email content from Gmail when the user opens an email.

## 6. Open decisions and approval gate

Resolve or map these remaining details before the Agenda read contract is final:

- Which connected Gmail accounts contribute to unread and action-needed counts?
- Which unselected Plan items qualify as focus suggestions, and how are they ranked?
- The existing Month view can request 35–42 inclusive visible dates, while the V2 agenda API currently allows at most 31. Preserve the full Month view by supporting its complete range (at least 42 dates) or an equivalent read path before wiring Agenda to V2.

The screen choices are captured for the next API read-needs mapping step. This does **not** approve creating canonical database views, staging migrations, or applying migrations to Development or Production. Keep migration 0016 unchanged.
