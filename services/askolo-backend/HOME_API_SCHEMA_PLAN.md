# Home API and forward-schema plan

Status: draft for implementation planning. This document does not create a migration, apply a schema, or authorize a Development database change.

## Decisions recorded

- A provider account has one owning user and one workspace association when connected to Askolo.
- A Space owner or admin can promote the account for use by the Space. Promotion grants Space access; it does not transfer account ownership or OAuth credentials.
- Per-Space Home summaries use only the provider accounts available in that Space and selected for the summary.
- Focus choices are saved per user, workspace/Space, and Plan day. Home shows saved choices first, then suggestions, up to three actions.
- All Spaces reads are authorized and aggregated on the server. Never fan out to each workspace from the browser.
- Email summaries contain only unread Inbox and action-needed counts plus freshness metadata. Do not persist email content or credentials in the V2 schema.

## Proposed API surface

The draft routes and response contract are in `UNIFIED_TASK_V2_CONTRACT.md`, under “Home API draft”:

- `GET /api/v2/workspaces/{workspaceId}/home`
- `GET /api/v2/home`
- `PUT /api/v2/workspaces/{workspaceId}/home/focus`
- `PUT /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/space-access`

All Space-scoped writes must use the existing V2 session and workspace-role authorization. Only owners/admins can promote or unpromote a connection. Focus writes affect only the signed-in user's current-day choices and must not mutate the underlying Task, occurrence, or Plan.

## Proposed staged migration

Add a new forward-only staged migration after the current unified Task/Plan migration. A candidate filename is `0018_home_projection_foundation.sql`; confirm migration ordering and checksum registration before creating the SQL file. Use `@ASKOLO_V2_SCHEMA@` for new V2 relations. Keep migration 0016 and the stable public API unchanged. Do not add canonical views.

### 1. `workspace_provider_account_access`

Store the association between a user-owned legacy provider connection and its single V2 workspace:

- `workspace_id`, `provider_connection_id`, and `owner_user_id`.
- Access scope constrained to `owner_only` or `space_shared`; new rows default to `owner_only`.
- Optional promotion actor and timestamp; both are required only for `space_shared`.
- Creation/update timestamps.
- Unique `provider_connection_id` so the same connection cannot be attached to multiple workspaces.
- Composite workspace/owner foreign key to workspace membership records, plus a workspace/provider-connection primary key. The service must check membership status is active; a foreign key alone does not enforce active status.

The legacy provider connection remains the credential source of truth. The V2 association stores no access/refresh tokens. Application-level ownership checks must join the referenced legacy provider connection/account and verify that its user matches `owner_user_id`; never trust a client-supplied owner. Only an owner/admin may switch access to `space_shared`. An inactive or revoked connection is unavailable regardless of its promotion row.

### 2. `workspace_email_summaries`

Keep one minimal current summary per workspace/provider connection:

- Workspace and provider-connection identifiers.
- Non-negative unread Inbox and action-needed counts.
- Sync state, `last_synced_at`, and `updated_at`.
- Foreign key to the workspace account-access row so an unlinked account cannot leave an active summary.

The row is a current snapshot, not a message store or historical analytics table. Exclude it from reads whenever the underlying connection is inactive or the viewer lacks account access. The exact stale threshold, retention after disconnect, and sync retry policy belong to the provider-sync implementation.

### 3. `home_focus_selections`

Persist ordered choices for one viewer, one workspace, and one Plan day:

- `workspace_id`, `user_id`, `plan_day`, `task_id`, nullable `occurrence_on`, and `position` constrained to 0–2.
- Composite foreign keys to workspace membership and the workspace-scoped Task; when `occurrence_on` is set, require the corresponding Task occurrence.
- Unique position per `(workspace_id, user_id, plan_day)`.
- Separate uniqueness constraints/indexes for Task references with and without an occurrence date so a Task cannot be selected twice for the same day.
- Index by `(workspace_id, user_id, plan_day, position)`.

The API replaces a day's list transactionally after validating all references and the three-item cap. The server derives `plan_day` from the viewer's effective local date; the client cannot choose another date through the Home endpoint. Reads filter selections that are no longer eligible, then fill remaining slots from current Plan entries, habit occurrences, and overdue open Tasks. Suggestions are projection results, not persisted or scheduled.

## Authorization and lifecycle behavior

- For `owner_only`, only the connection owner may use that account in Home. For `space_shared`, active members of the associated workspace may use its Home summary. Promotion grants summary access only; full mailbox content remains outside this contract.
- Every Home request rechecks active workspace membership and the provider connection's active status. All Spaces filters to the viewer's currently authorized workspaces and account grants before aggregation.
- A revoked/disconnected provider connection is immediately excluded from summary reads and sync. The V2 schema must not preserve visible stale counts after revocation.
- If the account owner loses active membership, deny the account to the former owner and the Space, pause sync, and require an active owner/admin to explicitly restore access. Whether to retain the promotion row or remove it during membership revocation should be settled with the broader membership-lifecycle implementation.
- Never log provider account identifiers, message content, tokens, or email addresses. Avoid exposing connection IDs in Home summary payloads.

## Migration and API verification plan

Run only against a fresh disposable PostgreSQL database until a separate Development-schema approval is given:

1. Apply the staged migration after the approved V2 prefix and verify migration checksums; assert migration 0016 is unchanged and no views are created.
2. Test unique one-workspace association, default owner-only access, owner/admin promotion, rejection of member promotion, and inactive-connection exclusion.
3. Test focus limits, ordering, duplicate/cross-workspace rejection, recurring occurrence references, replacement atomicity, and per-user/per-Space/per-day isolation.
4. Test one-Space Home and All Spaces results, current-day timezone behavior, per-Space and combined email totals, saved-action precedence, multi-day calendar overlap, no-store headers, unauthorized membership, and revoked-account filtering.
5. Seed summary snapshots directly for read tests; webhook and provider sync are a separate follow-up and are not required to validate the projection API.

## Items to settle before implementing provider sharing

- Choose the freshness threshold shown by the Home summary and the provider-sync retry/retention behavior.
- Confirm what happens to a promoted account if its owner disconnects or leaves the workspace; the safe read behavior is already to deny and stop syncing immediately.
- Confirm whether Space promotion is limited to Home summary counts, as this draft proposes, or should also authorize opening full provider content.
- Confirm All Spaces tie-breaking when more than three saved choices exist across Spaces. This draft uses Space-list order, then each Space's saved position.

No migration has been applied to Development or Production.
