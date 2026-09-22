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
  a provider has accepted a message. Readiness remains degraded until a
  successful provider handoff. After a delivery attempt, it also reports
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

- `GET /api/auth/google` and `/api/auth/google/callback` for login
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

### Native email auth integration validation

The native email auth integration suite uses a capture-only sender and creates a
unique temporary schema in the PostgreSQL database; it never sends real email.
Run it against a disposable PostgreSQL database or isolated test database role:

```sh
ASKOLO_TEST_DATABASE_URL='postgres://...' \
  go test ./internal/modules/auth -run 'TestNativeEmailAuth' -count=1 -v
```

The test role needs permission to create and drop schemas. The suite drops its
temporary schema during cleanup. Regular `go test ./...` skips these tests when
`ASKOLO_TEST_DATABASE_URL` is not set.

Release validation runs the same checks automatically through
`scripts/run-native-email-auth-validation.sh`. With no environment override,
that script creates a temporary local PostgreSQL cluster, grants the test its
own database superuser connection, runs the checks once with `-count=1`, and
stops and removes the cluster on success or failure. The test's temporary
schema is also dropped by the test cleanup. A release environment may provide
`ASKOLO_TEST_DATABASE_URL` instead, but it must point to a disposable database
whose role can create and drop schemas. The validation never uses
`DATABASE_URL`, sends email, or contacts Resend.

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

Operators should alert on `mfaSecurity.alert == true`, or on the structured
`MFA verification failure spike` warning with `environment` as a required
aggregation label. The default thresholds are:

- 20 or more MFA failure events in 15 minutes;
- 5 or more replay rejections in 15 minutes;
- 5 or more challenge lockouts in 15 minutes; or
- 3 or more MFA decryption failures in 15 minutes.

The signal logs at most once per threshold-reason set per 15 minutes per
backend process. Counts are aggregated across users, so an individual code,
secret, email address, and request is never an alert dimension. A missing or
unavailable signal is reported as `status: "unavailable"` and does not make a
cold-start readiness check fail.

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
`persistentFailureThreshold`, and `lastSuccessfulCleanupAt`. Before the first
completed run, status is `unknown`. One or two consecutive failures are
`transient_failure`; three or more are `persistent_failure` and make readiness
return `503` until a cleanup succeeds. A successful run resets the consecutive
failure count and records its completion time. Challenge IDs, addresses, and
error details are never included in this signal.
