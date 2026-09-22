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
- Password signup and recovery require an SMTP delivery configuration:
  `AUTH_SMTP_HOST`, `AUTH_SMTP_PORT` (default `587`), `AUTH_SMTP_USERNAME`,
  `AUTH_SMTP_PASSWORD`, and `AUTH_EMAIL_FROM`. `AUTH_CHALLENGE_SECRET` may be
  set separately; otherwise the challenge hashes use `SESSION_SECRET`.
  Challenge values, passwords, email bodies, and SMTP credentials are never
  written to logs or returned by the API.
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