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
- `pnpm --filter @workspace/db run push` — push DB schema changes (dev only)
- Required env: `DATABASE_URL` — Postgres connection string

## Stack

- pnpm workspaces, Node.js 24, TypeScript 5.9
- Web: React, Vite, Tailwind CSS v4, shadcn/ui, Framer Motion, Wouter, Clerk
- API: Go HTTP backend
- DB: PostgreSQL + Drizzle ORM
- Validation: Zod (`zod/v4`), `drizzle-zod`
- API codegen: Orval (from OpenAPI spec)
- Build: esbuild (CJS bundle for API), Vite (for web)

## Where things live

- API routes: `services/askolo-backend/internal/httpapi/` and `services/askolo-backend/internal/modules/`
- Google OAuth & API helpers: `services/askolo-backend/internal/modules/google/`
- Web pages: `artifacts/personal-assistant/src/pages/`
- Shared layouts: `artifacts/personal-assistant/src/components/layout/`
- DB schema: `lib/db/src/schema/`
- API client hooks: `lib/api-client-react/src/generated/`
- Public assets (logo, favicon): `artifacts/personal-assistant/public/`

## Architecture decisions

- **Clerk for auth:** Replit-managed Clerk handles sign-in/up; the API verifies the session token cookie.
- **Google services via custom OAuth app:** Calendar and Gmail use the user's own Google Cloud OAuth app and store a single token per user in `gmail_tokens`.
- **Dynamic redirect URI:** OAuth redirect URIs are built from the request host so dev and production share the same code.
- **SPA path routing:** The app is a Vite SPA; route paths are absolute and match the artifact preview path (`/`).
- **Optional Google integrations:** Calendar and Gmail are opt-in; the app works without them.

## Product

Askolo gives users a calm, focused home for daily life management: track habits, set goals, build a daily plan, view and manage calendar events, keep household chores, take notes, manage action items, and get AI coaching. Optional Google Calendar and Gmail integrations sync events and messages so users can plan and triage email without switching apps.

## User preferences

- The app name is **Askolo** everywhere.
- The logo/icon is the uploaded image at `artifacts/personal-assistant/public/logo.png` and should be used consistently across the app, landing page, and Clerk sign-in UI.
- The public home page (`/`) shows a landing page with a "Go to Dashboard" CTA for signed-in users; it does not auto-redirect.
- The dashboard lives at `/dashboard` and is linked from the sidebar and app logo.
- Privacy Policy and Terms of Service are public pages and should describe the custom Google OAuth app usage.

## Gotchas

- The web dev server requires both `PORT` and `BASE_PATH` env vars to start. The managed workflow sets these; local manual builds need them too, e.g. `PORT=18131 BASE_PATH=/ pnpm --filter @workspace/personal-assistant run build`.
- Google refresh tokens are only returned on the first offline authorization; `storeGoogleTokens` preserves the existing refresh token when Google does not reissue one.
- The OAuth callback redirects to the originating page (via a signed `redirectTo` cookie) so Calendar and Email pages show the correct post-connect toast.

## Pointers

- See the `pnpm-workspace` skill for workspace structure, TypeScript setup, and package details.
