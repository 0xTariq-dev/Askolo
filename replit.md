# Askolo

Askolo is a beautifully designed personal and family assistant that helps you manage habits, goals, daily plans, calendar, chores, notes, action items, and an AI coach — all in one place. It optionally connects to Google Calendar and Gmail so you can plan your day and triage messages without leaving the app.

## Run & Operate

- `pnpm --filter @workspace/personal-assistant run dev` — run the web app (requires `PORT` and `BASE_PATH` env vars)
- `cd services/askolo-backend && bash ./scripts/run.sh` — run the Go backend locally (port 8090 by default)
- `cd services/askolo-backend && go test ./...` — run Go backend tests
- `cd services/askolo-backend && bash ./scripts/build.sh` — test, vet, and atomically replace the compiled Go backend binary
- `pnpm run typecheck` — full typecheck across all packages
- `pnpm run build` — typecheck + build all packages
- `pnpm --filter @workspace/api-spec run codegen` — regenerate API hooks and Zod schemas from the OpenAPI spec

## Workspace environment decision

This Repl is the **development Repl**. Use it for active work, local previews,
development integrations, and the development database only.

- Local workflows must use `ASKOLO_ENVIRONMENT=development`,
  `ASKOLO_COOKIE_NAMESPACE=askolo_dev`, and `ASKOLO_DATABASE_ID=development-database`.
- Do not attach staging or production databases, OAuth clients, cookies, provider
  targets, deployment secrets, or webhook destinations to this Repl.
- Staging and production are separate Replit projects. Their later attachment
  and release checks are documented in
  [`docs/three-repl-attachment-plan.md`](docs/three-repl-attachment-plan.md).

## Database schema changes

- The Go SQL migrations in
  `services/askolo-backend/internal/migrations/sql/` are the source of truth for
  Askolo's backend schema. When a feature needs a schema change, add the next
  sequentially numbered migration in the same change; do not use Drizzle schema
  pushes or ad hoc production DDL.
- Treat committed migrations as immutable. Fix an applied migration with a new
  forward migration, and update the migration integration tests for the new
  behavior.
- Validate migrations with
  `cd services/askolo-backend && GOSUMDB=sum.golang.org bash ./scripts/test-migrations.sh`.
  This creates a disposable local PostgreSQL instance; do not point migration
  tests at the app's `DATABASE_URL`.
- The explicit Go migration runner is for development, disposable databases,
  and externally managed PostgreSQL targets. Production migrations must be a
  separate, approved release step after target verification, backup checks,
  and a tested restore. Never run a production target from this development
  Repl.
- Do not run DDL at API startup or as part of an ordinary app build. For
  Replit-managed production PostgreSQL, Publish owns schema synchronization and
  Replit documents no supported opt-out. Do not use the Go runner against that
  production database; resolve the migration-ledger/readiness compatibility
  before selecting this hosting path.
- Follow [`docs/go-migrations-runbook.md`](docs/go-migrations-runbook.md) for
  migration authoring, validation, adoption, and release constraints.

## Stack

- pnpm workspaces, Node.js 24, TypeScript 5.9
- Web: React, Vite, Tailwind CSS v4, shadcn/ui, Framer Motion, Wouter
- API: Go HTTP backend
- DB: PostgreSQL with a Go-owned runtime store
- Validation: Zod (`zod/v4`)
- API codegen: Orval (from OpenAPI spec)
- Build: esbuild (CJS bundle for API), Vite (for web)

## Where things live

- API routes: `services/askolo-backend/internal/httpapi/` and `services/askolo-backend/internal/modules/`
- Google OAuth & API helpers: `services/askolo-backend/internal/modules/google/`
- Web pages: `artifacts/personal-assistant/src/pages/`
- Shared layouts: `artifacts/personal-assistant/src/components/layout/`
- API client hooks: `lib/api-client-react/src/generated/`
- Public assets (logo, favicon): `artifacts/personal-assistant/public/`

## Architecture decisions

- **Native Go auth:** the Go backend owns sign-in/up, OAuth callbacks, sessions, MFA, and authorization.
- **Google services via custom OAuth app:** Calendar and Gmail use the user's own Google Cloud OAuth app and store a single token per user in the Go-owned database.
- **Dynamic redirect URI:** OAuth redirect URIs are built from the request host so dev and production share the same code.
- **SPA path routing:** The app is a Vite SPA; route paths are absolute and match the artifact preview path (`/`).
- **Optional Google integrations:** Calendar and Gmail are opt-in; the app works without them.

## Product

Askolo gives users a calm, focused home for daily life management: track habits, set goals, build a daily plan, view and manage calendar events, keep household chores, take notes, manage action items, and get AI coaching. Optional Google Calendar and Gmail integrations sync events and messages so users can plan and triage email without switching apps.

## User preferences

- The app name is **Askolo** everywhere.
- The logo/icon is the uploaded image at `artifacts/personal-assistant/public/logo.png` and should be used consistently across the app and landing page.
- The public home page (`/`) shows a landing page with a "Go to Dashboard" CTA for signed-in users; it does not auto-redirect.
- The dashboard lives at `/dashboard` and is linked from the sidebar and app logo.
- Privacy Policy and Terms of Service are public pages and should describe the custom Google OAuth app usage.

## Gotchas

- The web dev server requires both `PORT` and `BASE_PATH` env vars to start. The managed workflow sets these; local manual builds need them too, e.g. `PORT=18131 BASE_PATH=/ pnpm --filter @workspace/personal-assistant run build`.
- Google refresh tokens are only returned on the first offline authorization; `storeGoogleTokens` preserves the existing refresh token when Google does not reissue one.
- The OAuth callback redirects to the originating page (via a signed `redirectTo` cookie) so Calendar and Email pages show the correct post-connect toast.

## Pointers

- See the `pnpm-workspace` skill for workspace structure, TypeScript setup, and package details.
