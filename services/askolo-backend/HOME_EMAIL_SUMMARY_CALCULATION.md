# Home email summary calculation

This describes the scheduled Gmail summary calculation for Home. It is separate
from the deferred Google webhook work; the initial implementation uses polling.

## Eligibility

Calculate only for an active Google connection that:

- belongs to the account owner;
- has the Gmail read-only scope and `gmail.read` capability;
- is actively associated with the Space and selected for its Home summary;
- has not had its access or account-owner consent revoked.

Use the existing connection/token refresh path. Do not request new OAuth scopes
or change Google Console configuration for this calculation.

## Counts

1. Read Gmail label metadata to obtain the exact unread-message count for
   `INBOX`.
2. Page through Inbox threads and fetch metadata-only thread representations.
   For each thread, find its newest message by Gmail `internalDate`; count that
   thread once when the newest message is from someone other than the connected
   account owner.
3. Page through messages with Gmail's `STARRED` or `IMPORTANT` labels, fetching
   metadata only. Add their thread IDs to the action-needed set.
4. `action_needed_count` is the number of distinct thread IDs in the union of
   the reply-needed and starred/important sets. Unread and action-needed totals
   are independent and can overlap.

Compare parsed mailbox addresses case-insensitively. If a sender header is
missing or invalid, do not infer that the message needs a reply; fail the sync
instead of publishing an uncertain count.

## Safety and freshness

- Never request message bodies or persist message IDs, thread IDs, headers,
  snippets, addresses, or credentials.
- Keep IDs and metadata transient in memory for the duration of a sync only.
- Page to completion before publishing counts. Apply a fixed per-account request
  and time budget; if the budget is exceeded, discard partial totals and mark
  the sync as `error` (or retain the previous completed totals as `stale`).
- Write both counts and `last_synced_at` atomically only after a complete scan.
  A provider or database failure must never make partial values appear current.
- Poll hourly. Treat a successful sync as fresh for two hours; after that mark
  it stale until the next complete sync. These intervals are the initial policy
  and should be revisited with production volume and provider quota data.
- Log account/Space identifiers, duration, request count, and outcome only.
  Never log provider responses, email addresses, IDs, headers, or message data.

Webhook-triggered updates are explicitly deferred.
