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

## Commands

```sh
go fmt ./...
go test ./...
go vet ./...
go build -trimpath -o ./bin/askolo-backend ./cmd/askolo-backend
go run ./cmd/askolo-backend
```