# Askolo Backend

This Go service runs beside the TypeScript API during migration and is designed
to become Askolo's primary backend without a second structural rewrite.

## Current topology

- The TypeScript API remains the public backend and system of record.
- The Go backend listens on port `8090` and exposes health checks publicly.
- Companion-phase REST and WebSocket calls use `/internal/*` and require
  `ASKOLO_INTERNAL_TOKEN`.
- Public `/ws` and `/webhooks/*` paths are reserved but intentionally return
  `501` until their authentication, verification, and application modules are
  implemented.
- AssemblyAI and other providers are deliberately not part of this foundation.
- Google login and Google integration OAuth use separate clients configured with
  `GOOGLE_LOGIN_CLIENT_ID`, `GOOGLE_LOGIN_CLIENT_SECRET`,
  `GOOGLE_INTEGRATION_CLIENT_ID`, and `GOOGLE_INTEGRATION_CLIENT_SECRET`.
- `GOOGLE_TOKEN_ENCRYPTION_KEY` must decode to 32 bytes. Refresh and access
  tokens are encrypted before they are stored in `provider_credentials`.
- `DATABASE_URL` and `SESSION_SECRET` are required for OAuth and account
  operations. Without them, health checks still work but OAuth returns a safe
  configuration error.
- Password signup and recovery require a Resend delivery configuration:
  `RESEND_API_KEY` and `AUTH_EMAIL_FROM`. `AUTH_CHALLENGE_SECRET` may be
  set separately; otherwise the challenge hashes use `SESSION_SECRET`.
  The sender is normalized to `Askolo <AUTH_EMAIL_FROM>` so mail clients show
  the app name even when the configured value is only an address.
  Challenge values, passwords, email bodies, and Resend credentials are never
  written to logs or returned by the API. `/readyz` reports Resend and challenge
  configuration separately; `configured` means settings are present, not that
   a provider has accepted a message. A fresh process can become ready before
   its first delivery attempt; `emailDelivery.status: "unknown"` means no
   attempt has happened yet. After a delivery attempt, it also reports
  aggregate attempt/handoff counts, bounded latency, the last safe outcome, and
  failure counts. The public response never includes an address, code, body,
  provider error, or credential.
- The web API proxies `/api/auth/google` and `/api/integrations/google/*` to
  `ASKOLO_GOOGLE_BACKEND_URL` when configured. Development defaults to the
  local Go service at `http://127.0.0.1:8090`; production must use the
  deployed Go service URL and must not use a loopback target.
- Go is the authorization decision owner. The internal
  `POST /internal/authz/decision` route derives the actor from the native
  session, lazily provisions a personal workspace for active users, and
  evaluates account status, workspace status, membership status, explicit
  capabilities, and registered resource ownership. The TypeScript API calls
  this route before feature requests; provider and WebSocket boundaries use
  the same store and policy primitives.
- `X-Askolo-Workspace-ID` is an authorization scope hint, not an identity
  assertion. Unknown, inactive, revoked, cross-workspace, or unowned scopes
  fail closed with generic client errors. A separate `ASKOLO_INTERNAL_TOKEN`
  should be configured for production service handover; development derives
  the service token from `SESSION_SECRET` when the dedicated token is absent.

## Package boundaries

```text
cmd/askolo-backend/       process entry point only
internal/app/             composition, server lifecycle, graceful shutdown
internal/config/          environment parsing and validation
internal/httpapi/         top-level route assembly
internal/platform/        shared auth, request, logging, and error concerns
internal/transport/rest/  REST protocol adapter
internal/transport/websocket/
                          WebSocket protocol adapter
internal/transport/webhooks/
                          webhook protocol adapter
internal/modules/         future product capabilities and use cases
internal/adapters/        future database and external-provider adapters
```

Transport packages may call application-module interfaces, but application and
domain packages must not import transports. External providers and databases
belong in adapters, not handlers. Only `internal/app` should wire concrete
implementations together.

## Migration path

1. The TypeScript API authenticates users and calls authenticated `/internal/*`
   Go routes.
2. Product capabilities move one at a time with one authoritative owner for
   state changes, provider operations, and credit settlement.
3. Public authentication and compatibility routes are added to Go without
   moving the existing transport or module packages.
4. Traffic shifts at the routing layer.
5. The TypeScript backend is removed only after parity and rollback criteria
   pass.

## Release-1 Google routes

The Go service exposes:

- `GET /api/auth/google` and `/api/auth/google/callback` for native login. The
  optional `intent=signin|signup` query selects the login or explicit signup
  flow, and `returnTo` must be a same-origin path such as `/dashboard`; unsafe
  or missing values fall back to `/dashboard`.
- `GET /api/auth/google/link` and `/api/auth/google/link/callback` for linking
  a Google identity to an existing session
- `GET /api/integrations/google` and `/api/integrations/google/callback` for
  Calendar and Gmail-send authorization
- `GET /api/integrations/google/status` and `/accounts`
- `PATCH /api/integrations/google/{connectionID}/capabilities/{capability}`
- `PUT /api/integrations/google/defaults/{service}`
- `DELETE /api/integrations/google/{connectionID}/services/{service}`
- `DELETE /api/integrations/google/{connectionID}` and
  `DELETE /api/integrations/google` for revoke operations
- Calendar list, event CRUD, and confirmed Gmail send endpoints under
  `/api/integrations/google/calendar/*` and
  `/api/integrations/google/gmail/send`

Mailbox reading, metadata, drafts, sync, push notifications, and imported-data
deletion are intentionally not enabled by these routes.

The public Google login routes are registered ahead of the generic
`/api/auth/` password and session routes. The callback validates the signed
state cookie, matches the persisted one-time state, and exchanges the
authorization code with PKCE before creating the native session. Google login
uses the separate login client and only requests `openid`, `email`, and
`profile`; Calendar and Gmail consent uses the separately configured
integration client and scopes. Outside development, `ASKOLO_CANONICAL_ORIGIN`
must be an HTTPS origin and its host must be registered with Google. Callback
failures clear the state cookie and redirect only to the validated same-origin
`returnTo` path.

### Staging Google OAuth verification

Use a disposable Google OAuth web client for staging. Register exactly this
redirect URI on that client:

```text
https://<staging-canonical-host>/api/auth/google/callback
```

Set `ASKOLO_CANONICAL_ORIGIN` to the same HTTPS origin in the staging backend.
The Go handler uses this configured origin for both the authorization request and
the token exchange, even when a reverse proxy supplies a different forwarded
host. Do not use the frontend preview host or the integration client for native
login.

Run the check with a disposable Google account and remove the account/client
afterward:

1. Open `/api/auth/google?returnTo=/dashboard` and complete Google sign-in.
   Confirm the browser lands on `/dashboard?google=success` (or
   `google=mfa_required`) and that the native session cookie is present.
2. In a clean browser session, open
   `/api/auth/google?intent=signup&returnTo=/sign-up` and complete consent.
   Confirm `/sign-up?google=success` and verify that the new account has a
   verified Google email identity.
3. Repeat with a previously used account to confirm sign-in preserves the
   existing native account instead of creating a duplicate.
4. Inspect the authorization URL before approving consent. Native login must
   use the login client and request exactly `openid email profile`; it must not
   request Calendar or Gmail permissions.
5. While signed in, open `/api/integrations/google?scope=calendar` and inspect
   the authorization URL. It must use the separate integration client and the
   selected Calendar/Gmail service scope set. Deny the consent and confirm the
   existing native session remains active; approving it must connect the
   integration without replacing that session.
6. Confirm denied consent redirects to the validated local path with the
   generic `google=error` status. Replay an expired, already-consumed, or
   cross-flow callback and confirm it returns the generic `INVALID_STATE`
   response.
7. Start each flow with an unsafe `returnTo` such as
   `https://example.invalid/account` and confirm failures land at
   `/dashboard?google=error`, never at the external URL.

Capture only the staging origin, redirect URI, result status, and timestamp in
the release record. Never record authorization codes, tokens, client secrets,
or the disposable account's credentials.

## Commands

```sh
go fmt ./...
go test ./...
go vet ./...
bash ./scripts/build.sh
bash ./scripts/run.sh
```

`build.sh` always writes to a temporary file and atomically replaces
`bin/askolo-backend`; failed builds leave the last known-good binary intact.
It also stops a recorded running backend before rebuilding. `run.sh` records
the managed process under `tmp/askolo-backend.pid`, cleans stale PID state,
forwards shutdown signals, and terminates the child if graceful shutdown
fails. The `bin/` and `tmp/` directories are ignored generated state.

### Native email auth and cleanup readiness integration validation

The native email auth integration suite uses a capture-only sender and creates a
unique temporary schema in the PostgreSQL database; it never sends real email.
Run it against a disposable PostgreSQL database or isolated test database role:

```sh
ASKOLO_TEST_DATABASE_URL='postgres://...' \
  go test ./internal/modules/auth -run 'TestNativeEmailAuth' -count=1 -v
```

The database role needs `LOGIN` and `CONNECT` on the target database, plus
`CREATE` on that database. The suite creates a schema with that role, so the
role owns the schema and can create its tables and indexes and drop the schema
with `CASCADE` during cleanup. It does not need `SUPERUSER`, `CREATEDB`,
`CREATEROLE`, role membership, or privileges on the `public` schema or any
other schema. Regular `go test ./...` skips these tests when
`ASKOLO_TEST_DATABASE_URL` is not set.

Release validation runs the native auth checks and the cleanup readiness
failure/recovery check automatically through
`scripts/run-native-email-auth-validation.sh`. With no environment override,
that script creates a temporary local PostgreSQL cluster and a dedicated
non-superuser test role with exactly that database contract, runs both checks
once with `-count=1`, and stops and removes the cluster on success or failure.
The test's temporary schema is also dropped by the test cleanup. A release
environment may provide `ASKOLO_TEST_DATABASE_URL` instead, but it must point
to a disposable database using a role with the same minimum contract. The
validation never uses `DATABASE_URL`, sends email, or contacts Resend.

### Native email delivery operations

The native auth sender reports these safe delivery outcomes:

- `configuration_missing` or `configuration_invalid`: settings are absent or
  inconsistent. Fix the environment configuration before retrying.
- `connection_failure`: Resend could not be reached.
- `authentication_failure`: Resend rejected the API credential.
- `provider_rejection`: Resend rejected the sender, recipient, or message.
- `timeout`: the bounded network or request deadline elapsed.
- `handoff`: Resend accepted the message and returned a message ID. This is a
  handoff, not proof of inbox delivery.

Every attempted challenge emits one structured, privacy-safe event with
`operation`, `purpose`, `outcome`, `duration_ms`, and `count=1`. Failure logs
contain only the categorized outcome and whether the challenge can be retried;
they never include recipient addresses, codes, message bodies, or raw provider
errors. Latency is capped at 30 seconds in telemetry.

### MFA failure-spike signal

`GET /readyz` includes an `mfaSecurity` object built from the most recent
15-minute rolling window of MFA security events. It is intentionally bounded
to the configured environment, event counts, and the number of affected users;
it never includes user IDs, request IDs, codes, encrypted secrets, or event
metadata. MFA decryption failures are recorded with only a fixed operation
label (`mfa_confirmation` or `mfa_challenge`).

The backend sends this signal to the deployment's structured-log alert
destination. Operators should alert on `mfaSecurity.alert == true`, or on the
structured `MFA verification failure spike` warning. The stable
`operation="mfa_security_spike"` label identifies this alert family, and
`environment` is the only routing dimension. Configure one log-alert rule for
each environment:

| Environment | Alert filter |
| --- | --- |
| `development` | `msg="MFA verification failure spike" AND operation="mfa_security_spike" AND alert=true AND environment="development"` |
| `staging` | `msg="MFA verification failure spike" AND operation="mfa_security_spike" AND alert=true AND environment="staging"` |
| `production` | `msg="MFA verification failure spike" AND operation="mfa_security_spike" AND alert=true AND environment="production"` |

Route all three rules to the team's configured on-call destination. The
aggregate fields in the event are the counts below; the destination must not
forward or group on any user, request, code, email, secret, or raw event
metadata.

The default thresholds are:

- 20 or more MFA failure events in 15 minutes;
- 5 or more replay rejections in 15 minutes;
- 5 or more challenge lockouts in 15 minutes; or
- 3 or more MFA decryption failures in 15 minutes.

The signal logs at most once per threshold-reason set per 15 minutes per
backend process. The on-call destination should deduplicate by
`environment` plus `operation` plus the active `alert_reasons` set. This
prevents multiple backend instances and readiness probes from opening
duplicate incidents while still allowing a new threshold reason to update the
incident. Counts are aggregated across users, so an individual code, secret,
email address, and request is never an alert dimension. A missing or
unavailable signal is reported as `status: "unavailable"` and does not make a
cold-start readiness check fail.

When a later readiness evaluation finds no active threshold, the backend emits
one `MFA verification failure spike recovered` event with
`operation="mfa_security_spike"`, `recovery=true`, `alert=false`, the
environment, and aggregate counts. Use that event as the explicit recovery
condition for all three rules. Recovery is emitted only after the signal is
available again; a database or signal outage does not falsely close an active
incident. If the destination cannot consume explicit recovery events, resolve
after one complete 15-minute evaluation window with no matching alert event.

MFA recovery support is included in the same bounded signal. The readiness
payload reports aggregate `recoverySupportRequests`,
`recoverySupportVerificationFailures`, `recoverySupportRateLimited`, and
`recoverySupportSessionRevocationFailures` counts for the same 15-minute
window. Recovery requests count only after a verification message is handed
off successfully, so they represent normal human-review volume rather than
raw probes. Rate-limit counts include the request and verification IP
guardrails, cooldowns, and locked challenges. The recovery-specific alert
thresholds are:

- 20 or more recovery-support requests in 15 minutes (`recovery_support_request_spike`);
- 5 or more verification failures (`recovery_support_verification_failures`);
- 5 or more rate-limited attempts (`recovery_support_rate_limited`); or
- any session-revocation failure (`recovery_support_session_revocation_failures`).

For a recovery alert, first inspect the aggregate counts and
`alertReasons` in `GET /readyz` from the authorized operational path. A
request spike with no rate-limit or infrastructure failures should be handled
as increased support-review volume. Rate-limit alerts should be correlated
with the request and verification guardrails before changing limits.
Any session-revocation failure is an infrastructure incident: pause manual
MFA recovery approvals, verify database/session-store health, and retry only
after revocation succeeds. The telemetry contains no email addresses, codes,
passwords, user IDs, or request IDs in the readiness response or alert log.

### Delivery retry and release checks

1. Confirm `GET /readyz` from an authorized operational path and inspect
   `emailDelivery.status`, `resendConfiguration`, and
   `challengeConfiguration`. `unknown` means configuration is present but no
   delivery has been attempted; it is not a delivery guarantee.
2. Check the aggregate `deliveryFailures`, `consecutiveFailures`, and
   `lastOutcome` values. One or two recent failures are reported as
   `transient_failure`; three consecutive failures are reported as
   `persistent_failure` and should trigger provider investigation.
3. A challenge is written before Resend handoff so verification remains
   single-use. If the sender reports a retry-safe failure, that exact pending
   challenge is removed, allowing a later resend while preserving the
   per-address 60-second cooldown for successful sends and concurrent requests.
   If handoff is uncertain, the challenge is retained and the normal cooldown
   prevents duplicate sends.
4. Validate in staging with a test mailbox and a disposable account. Confirm
   one successful handoff, a resend during cooldown is rate-limited, and a
   forced provider rejection can be retried without leaving a stale challenge.
5. Before production promotion, verify `RESEND_API_KEY`, from address, and
   `AUTH_CHALLENGE_SECRET` (or `SESSION_SECRET`) in the target environment.
   Never paste credentials or message content into
   logs, tickets, or release records.

### Email challenge retention cleanup

The backend retains terminal native email challenges for 24 hours after their
terminal timestamp: `expires_at` for an unused expired challenge or
`consumed_at` for a consumed challenge. This applies to email verification,
recovery-email enrollment, and password-recovery challenges. Recent expired and
consumed records, active challenges, and other challenge purposes are preserved.

Cleanup runs immediately at service startup and every 15 minutes afterward. Each
transaction deletes at most 100 rows and uses row locks with `SKIP LOCKED`, so
it does not wait for or remove a challenge involved in an active verification
transaction. Each run emits an aggregate structured log with the deleted count,
batch limit, retention period, and duration; failures emit only the operation
and aggregate policy fields.

The service also tracks cleanup health in memory and exposes it under
`GET /readyz` as `emailChallengeCleanup`. The signal contains only bounded
aggregate state: `status`, `consecutiveFailures`,
`persistentFailureThreshold`, `lastSuccessfulCleanupAt`,
`persistentFailureOccurrences`, and `recoveryEvents`. Before the first
completed run, status is `unknown`. One or two consecutive failures are
`transient_failure`; three or more are `persistent_failure` and make readiness
return `503` until a cleanup succeeds. A successful run resets the consecutive
failure count and records its completion time. Challenge IDs, addresses, and
error details are never included in this signal.

Cleanup failures are also connected to the deployment's structured-log alert
path. The service emits one `Email challenge cleanup failure alert` event when
the count reaches the documented threshold of three consecutive failures. Its
`alert=true` record is intentionally aggregate and contains only the service,
environment, fixed cleanup operation, failure count, and threshold. Additional
failures do not create duplicate alert events. After a successful cleanup
resets a persistent failure, the service emits one `Email challenge cleanup
recovered` event with `alert=false`, `recovery=true`, and `status: "healthy"`.
Configure the operational alert to match
`operation="email_challenge_cleanup"` and `alert=true`; use the recovery event
(`operation="email_challenge_cleanup"` and `recovery=true`) as its explicit
recovery condition. Neither event contains a challenge ID, address, or error
detail.

For a deployment monitoring view, poll the token-protected
`GET /internal/monitoring/cleanup` endpoint with the internal service token.
It returns the deployment `environment`, `service`, fixed
`operation: "email_challenge_cleanup"`, the number of persistent-failure
alert occurrences, the number of recovery events, and the latest bounded
`readiness` state. The counters are process-lifetime aggregates: a new
instance starts at zero, while the structured alert and recovery logs remain
the durable deployment history. A dashboard should group or filter by
`environment`, `service`, and `operation`, and must not display challenge IDs,
addresses, or error details.
