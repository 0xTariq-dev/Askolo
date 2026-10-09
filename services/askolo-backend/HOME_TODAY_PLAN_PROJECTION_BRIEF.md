# Home and Today/Plan Projection Brief

**Status:** Draft for user review
**Purpose:** Agree on the screens and the information they need before designing canonical API read contracts or V2 database views.

This is a screen-level product brief. It does not define API endpoints, response schemas, SQL views, migrations, or implementation details.

## 1. Shared Space and Agent scope

- A **Space** can represent a family or a professional team. Use “Space” in product-facing language; keep existing workspace-scoped technical identifiers unchanged unless separately approved.
- **Home** is the concise daily view for the currently selected Space. **All Spaces** is a separate view that aggregates the member's items and labels each item with its Space.
- The Agent follows the selected Space or All Spaces scope. It must not silently widen that scope. Shared or assigned items should identify their ownership where relevant.

## 2. Home projection

Home is a concise, factual roll-up, not a list of every task, calendar event, email, and Plan entry. It is not an AI-written narrative by default.

Initial Home content:

1. **Focus actions:** user-chosen actions already in the selected day's Plan. These lead the view; Home should not invent or silently schedule them.
2. **Email summary:** show unread and action-needed counts. Do not list individual emails in this card. Gmail remains the source of truth; retrieve full email content from Gmail when the user opens an email.
3. **Calendar summary:** show the selected day's event count and the next event with its time. Count an event on each selected day it overlaps, including multi-day events.
4. **Contextual Agent:** provide an inline, day-aware entry on Home. Keep the full Agent as a separate page rather than opening the global drawer from Home.

The Home date-selection and navigation behavior is not yet specified. The Calendar summary uses a selected-day concept; the date boundary and time zone still need agreement.

## 3. Today/Plan projection

Today/Plan is a Today-first planning surface integrated with a calendar view. Its initial content order is:

1. User-chosen focus actions.
2. A chronological timeline of scheduled calendar events and timed tasks.
3. Flexible tasks and habits.

Planning items should reflect canonical task state across Home, Today/Plan, and other projections. A Plan entry links to its existing Task rather than duplicating the task's title, status, or assignee. Provider-owned calendar events remain events, not editable tasks.

Suggest open calendar slots for unscheduled items, but do not schedule them without the user's approval. Do not carry the existing calendar-first side-panel layout forward as the default. Exact week/month navigation and the interaction for changing the selected day remain open.

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
| Home | Selected Space; selected-day context; that user's layout for the Space; focus actions and their canonical task/Plan state; unread and action-needed email totals; calendar events overlapping the day; and the next event and its time. |
| Today/Plan | Selected Plan and day; linked tasks and their state; user-chosen focus actions; scheduled times for tasks and events; flexible task and habit occurrences; and any carry-forward approval decision. |
| All Spaces | Items in the member's authorized Spaces, with a clear Space label and relevant ownership/assignment information on each item. |
| Contextual Agent | Selected Space or All Spaces scope, the relevant day, and only the planning context allowed by that scope. |

Gmail and Google Calendar remain the systems of record for their provider-owned content. Do not create a duplicate local store of full email or calendar content for these projections.

## 6. Open decisions and approval gate

Resolve these details before treating the screen projections and their data needs as final:

- What qualifies as an email that needs action, and which connected Gmail accounts contribute to the counts?
- Which time zone defines the selected day and the overlap of multi-day calendar events?
- How should users change the Home/Plan date, and what week/month navigation is required?
- Which item types and filters belong in All Spaces, beyond the currently agreed aggregate with Space labels?
- How are focus actions selected, and what is the appropriate limit or empty state when none are selected?

Reviewing or approving this brief authorizes only the next design step: mapping the approved screens to API read needs and candidate database projections. It does **not** approve creating canonical database views, staging migrations, or applying migrations to Development or Production. Keep migration 0016 unchanged.
