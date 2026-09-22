# Askolo

Askolo is a personal and family assistant for calmer days. It brings habits,
goals, daily plans, calendar events, household chores, notes, action items,
email, and AI assistance into one workspace.

Google Calendar and Gmail are optional integrations. Users can connect them
when they want calendar synchronization, inbox triage, or email workflows, but
the core product is designed to work without Google access.

This repository contains the Askolo web application, its Go API server, shared
packages, database schema, generated API clients, and Replit artifact
configuration.

## Table of contents

- [Product at a glance](#product-at-a-glance)
- [Repository structure](#repository-structure)
- [Architecture](#architecture)
- [Technology stack](#technology-stack)
- [Prerequisites](#prerequisites)
- [Quick start](#quick-start)
- [Running the project](#running-the-project)
- [Configuration and environment variables](#configuration-and-environment-variables)
- [Authentication](#authentication)
- [Integrations](#integrations)
- [Database](#database)
- [API reference and code generation](#api-reference-and-code-generation)
- [Frontend development](#frontend-development)
- [Backend development](#backend-development)
- [Testing and verification](#testing-and-verification)
- [Production builds and deployment](#production-builds-and-deployment)
- [Performance notes](#performance-notes)
- [Troubleshooting](#troubleshooting)
- [Security and data-handling guidance](#security-and-data-handling-guidance)
- [Contribution guide](#contribution-guide)
- [License](#license)

## Product at a glance

Askolo is organized around a daily-life management workflow:

1. A user signs in.
2. The dashboard summarizes current habits, goals, plans, events, chores,
   notes, and action items.
3. The user can open focused areas such as Habits, Goals, Plan, Calendar,
   Chores, Notes, Actions, and Email.
4. Optional Google connections make Calendar and Gmail data available.
5. AI features help with planning, coaching, inbox workflows, and structured
   extraction.

The public landing experience and the authenticated workspace are separate
product surfaces:

- Public site: `askolo.app`
- Authenticated application: `web.askolo.app`

The same frontend artifact supports local development, Replit previews, and
production routing. The local artifact preview path is `/`.

## Repository structure

```text
.
├── artifacts/
│   ├── personal-assistant/       # Askolo React/Vite web application
├── lib/
│   ├── api-spec/                 # OpenAPI source and Orval configuration
│   ├── api-client-react/         # React Query API client package
│   └── api-zod/                  # Shared Zod schemas and generated schemas
├── scripts/                      # Small workspace utility package
├── attached_assets/              # User-provided assets; do not commit secrets
├── pnpm-workspace.yaml           # Workspace membership and dependency policy
├── package.json                  # Root scripts
├── replit.md                     # Collaborator-facing project instructions
└── README.md                     # This document
```

### Important web directories

```text
artifacts/personal-assistant/
├── public/                       # Static logo, favicon, and public assets
├── scripts/                      # Frontend build helper scripts, if any
├── src/
│   ├── components/               # Shared UI and layout components
│   ├── contexts/                 # Native and development auth contexts
│   ├── hooks/                    # Reusable frontend hooks
│   ├── lib/                      # Frontend service and utility modules
│   ├── pages/                    # Product and public pages
│   ├── routes/                   # Route composition and lazy page modules
│   ├── App.tsx                   # Application entry and auth mode selection
│   ├── index.css                 # Global design tokens and styles
│   └── main.tsx                  # Browser bootstrap
├── vite.config.ts                # Vite, aliases, dev server, and chunking
└── .replit-artifact/
    └── artifact.toml             # Replit artifact routing and build settings
```

### Important API directories

```text
services/askolo-backend/
├── internal/
│   ├── adapters/                 # PostgreSQL persistence and provider adapters
│   ├── httpapi/                  # Canonical public edge routing
│   └── modules/                  # Auth, product, provider, and realtime APIs
├── cmd/                          # Go backend entrypoint
└── scripts/                      # Build and run helpers
```

## Architecture

### Web and API boundary

Askolo uses a React frontend artifact and a Go public edge:

| Artifact                       | Local port | Routed path | Role                       |
| ------------------------------ | ---------: | ----------- | -------------------------- |
| `artifacts/personal-assistant` |    `18131` | `/`         | React/Vite web application |
| `services/askolo-backend`      |     `8090` | `/api`      | Go API and public edge     |

The browser calls the API through the same application origin using `/api/...`
paths. Go owns the public `/api` edge while the frontend remains a static
artifact. This avoids hard-coding a localhost address or a development domain
into browser code.

### Frontend routing

The web application is a Vite single-page application using Wouter for
client-side routing. Production rewrites send unknown frontend paths to
`/index.html` so routes such as `/dashboard`, `/calendar`, and `/notes` can be
loaded directly.

The frontend uses route-level lazy imports. Public pages, the authenticated
layout, and feature pages are loaded only when their route is needed.

### Authentication

The application uses the native OAuth/session path. Go handles browser login,
callbacks, logout, MFA, and session endpoints. The frontend auth context calls
the Go API through same-origin `/api` paths; there is no competing external
session issuer.

### API and data flow

The normal authenticated request flow is:

1. The browser sends a request to `/api/...`.
2. Go middleware applies request limits, origin/CSRF checks, and authentication.
3. The route validates request data and resolves the current user.
4. The route reads or writes PostgreSQL through the Go store.
5. Responses are returned using the API shapes defined by the OpenAPI spec and
   shared Zod schemas.

AI-backed routes may also call AssemblyAI or other provider-specific helpers.
Voice consent, limits, and provider access are handled server-side.

### Shared contracts

The OpenAPI document is the source of truth for generated API client and schema
code:

- OpenAPI source: `lib/api-spec/openapi.yaml`
- Orval configuration: `lib/api-spec/orval.config.ts`
- Generated React client: `lib/api-client-react/src/generated/`
- Generated Zod code: `lib/api-zod/src/generated/`

When an API contract changes, update the OpenAPI specification and regenerate
the dependent packages rather than hand-editing generated files.

## Technology stack

### Workspace and build tooling

- pnpm workspaces
- Node.js 24
- TypeScript 5.9
- Vite 7
- esbuild for the API production bundle
- Prettier for formatting

### Web application

- React 19
- React DOM 19
- Tailwind CSS 4
- shadcn/ui and Radix UI primitives
- Wouter
- TanStack React Query
- React Hook Form
- Framer Motion
- Recharts
- Lucide React
- AssemblyAI client support for voice features

### API and persistence

- Go 1.25 HTTP server
- PostgreSQL
- Go-owned database schema and migrations
- Zod for API contract validation
- Go structured logging and request middleware
- Orval-generated API contracts and clients

### External services

- Native OAuth support for production authentication
- Google Cloud OAuth for optional Calendar and Gmail connections
- AssemblyAI for transcription workflows

## Prerequisites

Install the following before working on the project:

1. Node.js 24 or a compatible Node.js 24 environment.
2. pnpm.
3. Access to the project PostgreSQL database for API work.
4. Replit Secrets or an equivalent secure environment-variable store for
   service credentials.
5. A browser for the Vite preview.

The repository intentionally uses pnpm. The root `preinstall` script rejects
npm and Yarn installs, and `pnpm-workspace.yaml` enforces a one-day minimum
package release age as a supply-chain defense.

Check the installed tools:

```bash
node --version
pnpm --version
```

## Quick start

### 1. Install dependencies

From the repository root:

```bash
pnpm install
```

Do not use `npm install` or `yarn install`. The repository may remove lockfiles
created by those package managers and will reject the install.

### 2. Configure secrets

Add the environment variables required for the surface you are running. Use
Replit Secrets or another secret manager. Never paste credentials into this
README, source code, chat, commits, or client-side bundles.

At minimum, API and database work requires:

```text
DATABASE_URL
```

The web dev server also requires:

```text
PORT
BASE_PATH
```

The managed workflows provide the web values automatically. Manual commands
are shown below.

### 3. Run checks

```bash
pnpm run typecheck
```

### 4. Start the web and API services

Use two terminals for a manual local run:

```bash
# Terminal 1: web
PORT=18131 BASE_PATH=/ \
  pnpm --filter @workspace/personal-assistant run dev
```

```bash
# Terminal 2: Go API edge
cd services/askolo-backend && bash ./scripts/run.sh
```

Open the Vite preview at:

```text
http://localhost:18131/
```

The browser uses `/api` for API requests. Make sure both services are running
when testing authenticated or data-backed pages.

## Running the project

### Root commands

| Command                                         | Purpose                                                       |
| ----------------------------------------------- | ------------------------------------------------------------- |
| `pnpm install`                                  | Install workspace dependencies using pnpm                     |
| `pnpm run typecheck`                            | Typecheck shared libraries, artifacts, and scripts            |
| `pnpm run typecheck:libs`                       | Typecheck TypeScript project references under `lib/`          |
| `pnpm run build`                                | Typecheck, then build all packages that expose a build script |
| `pnpm --filter @workspace/api-spec run codegen` | Regenerate API clients and schemas                            |

### Web commands

```bash
# Start Vite development mode
PORT=18131 BASE_PATH=/ \
  pnpm --filter @workspace/personal-assistant run dev

# Typecheck the web package
pnpm --filter @workspace/personal-assistant run typecheck

# Build the production frontend
PORT=18131 BASE_PATH=/ \
  pnpm --filter @workspace/personal-assistant run build

# Preview the built frontend
PORT=18131 BASE_PATH=/ \
  pnpm --filter @workspace/personal-assistant run serve
```

`PORT` and `BASE_PATH` are validated by `vite.config.ts`; omitting either
variable causes the Vite command to fail explicitly.

### Go API commands

```bash
# Test the Go backend
cd services/askolo-backend && go test ./...

# Build and refresh the backend binary
cd services/askolo-backend && bash ./scripts/build.sh

# Start the Go backend
cd services/askolo-backend && bash ./scripts/run.sh
```

The Go backend uses port `8090` by default and reads database and provider
configuration from protected environment variables.

### Managed Replit workflows

The workspace currently defines these relevant workflows:

| Workflow                                             | Command or role                        |
| ---------------------------------------------------- | -------------------------------------- |
| `artifacts/personal-assistant: web`                  | Runs the Vite web application          |
| `go-askolo-backend: compile`                         | Builds and validates the Go API        |
| `go-askolo-backend: run`                             | Runs the Go API edge                   |

Prefer the managed workflows in Replit when working inside the hosted
environment. They provide the artifact ports and preview routing expected by
the platform.

## Configuration and environment variables

The table below lists variable names used by the current codebase. It does not
contain values. Set secrets through Replit Secrets or your deployment
provider's protected environment configuration.

### Runtime and routing

| Variable    | Used by                           | Notes                                                               |
| ----------- | --------------------------------- | ------------------------------------------------------------------- |
| `NODE_ENV`  | API and build/runtime code        | Selects environment-specific Go runtime behavior                  |
| `PORT`      | Web and API                       | Required by both server entry points                                |
| `BASE_PATH` | Vite web build                    | Required by the web config; `/` is the current artifact base        |
| `REPL_ID`   | Development Vite plugin selection | Managed by Replit when applicable                                   |
| `LOG_LEVEL` | Server logging                    | Optional logging configuration                                      |

### Database and sessions

| Variable         | Used by                      | Notes                                                       |
| ---------------- | ---------------------------- | ----------------------------------------------------------- |
| `DATABASE_URL`   | Go API and archived Drizzle Kit | PostgreSQL connection string; required for database work     |
| `SESSION_SECRET` | Google OAuth/session helpers | Keep private; used to protect signed redirect/session state |

### Google integrations

| Variable               | Used by                      | Notes                               |
| ---------------------- | ---------------------------- | ----------------------------------- |
| `GOOGLE_CLIENT_ID`     | Google OAuth and native auth | OAuth client identifier             |
| `GOOGLE_CLIENT_SECRET` | Google OAuth and native auth | Secret; never expose to the browser |

Google Calendar and Gmail are opt-in. The rest of the application should
remain usable when these variables are not configured, but Google connect
flows require the corresponding OAuth setup and redirect URIs.

### Transcription

| Variable                          | Used by                           | Notes                                         |
| --------------------------------- | --------------------------------- | --------------------------------------------- |
| `ASSEMBLY_AI_API_KEY` | AssemblyAI server helper | Secret required for transcription           |
| `ASSEMBLYAI_REGION`   | AssemblyAI server helper | Optional/provider-specific region selection |

### Environment rules

- Do not commit `.env` files or secret values.
- Do not put server-only variables in `VITE_*` variables.
- A `VITE_*` variable can be included in the browser bundle; only use it for
  public configuration.
- Restart the relevant workflow after changing environment variables.
- If a value is missing, fix the environment configuration instead of adding a
  silent fallback in application code.

## Authentication

### Native OAuth and sessions

The API exposes browser authentication routes under `/api`:

- `GET /api/login`
- `GET /api/callback`
- `GET /api/logout`
- `GET /api/auth/user`
- `POST /api/auth/logout`
- `POST /api/mobile-auth/token-exchange`

The exact native provider and callback configuration are environment-specific.
The callback host is derived from the incoming request so the same code can
support development and production hosts.

### Authentication troubleshooting

If a protected page does not load:

1. Check that the web workflow and Go backend are both running.
2. Confirm the browser request to `/api/auth/user` is reaching the Go edge.
3. Confirm the correct auth variables are configured for the current
   `NODE_ENV`.
4. Clear stale development auth state and retry the sign-in flow.
5. Inspect the API logs for the request status and request ID.

Do not work around an auth failure by disabling route protection.

## Integrations

### Google Calendar and Gmail

Google functionality uses a custom Google Cloud OAuth application. The API
contains dedicated helpers for:

- OAuth initiation and callback handling
- Calendar event reads and writes
- Gmail message reads and drafts
- Google connection/status checks
- Token persistence and refresh behavior

Google refresh tokens are only reliably returned on the first offline
authorization. Token storage preserves an existing refresh token if Google
does not issue a replacement during a later response.

The OAuth callback also preserves the originating page through signed redirect
state so a user can return to the Calendar or Email page that started the
connection flow.

Relevant code:

```text
services/askolo-backend/internal/modules/google/oauth.go
services/askolo-backend/internal/modules/google/operations.go
services/askolo-backend/internal/modules/google/legacy.go
```

### AssemblyAI

AssemblyAI supports voice/transcription workflows. The server owns provider
access and the browser-side package provides reusable voice input behavior.

Relevant code:

```text
services/askolo-backend/internal/modules/product/handler.go
```

Voice recordings are treated as transient input. Avoid persisting recordings
unless a product requirement explicitly changes that behavior.

## Database

The Go service is the only database owner. Database schemas, migrations, and
database access code live with the Go backend under
`services/askolo-backend/internal/`.

## API reference and code generation

### API route groups

The Go API is mounted under `/api`. Route modules are in
`services/askolo-backend/internal/modules/` and are dispatched by
`services/askolo-backend/internal/httpapi/router.go`.

Current route groups include:

| Module            | Responsibility                                            |
| ----------------- | --------------------------------------------------------- |
| `health.ts`       | Liveness/health endpoint                                  |
| `auth.ts`         | Browser and session authentication                        |
| `user.ts`         | Profile, data deletion, and account deletion              |
| `habits.ts`       | Habits and completions                                    |
| `goals.ts`        | Goal management                                           |
| `daily_plans.ts`  | Daily planning                                            |
| `events.ts`       | Calendar events                                           |
| `chores.ts`       | Household chores                                          |
| `notes.ts`        | Notes                                                     |
| `action_items.ts` | Action items                                              |
| `dashboard.ts`    | Dashboard summary data                                    |
| `google.ts`       | Google status and connection flows                        |
| `ai.ts`           | AI coaching, planning, extraction, and related operations |

The API health endpoint is:

```bash
curl -i http://localhost:8090/api/healthz
```

In a managed Replit preview, use the artifact's routed host rather than
assuming that `localhost` is browser-accessible.

### OpenAPI-driven workflow

The contract source is:

```text
lib/api-spec/openapi.yaml
```

Regenerate clients and schemas after changing the API contract:

```bash
pnpm --filter @workspace/api-spec run codegen
```

The code-generation command also runs the shared library typecheck. Generated
files should be reviewed for unintended changes, especially when schemas
contain names that could collide across generated modules.

Recommended API contract workflow:

1. Update the OpenAPI path, request, response, or schema.
2. Run the code-generation command.
3. Run `pnpm run typecheck`.
4. Update the server route implementation.
5. Update frontend consumers and user-facing error states.
6. Run the relevant tests and build.

## Frontend development

### Finding a page

Feature pages are in:

```text
artifacts/personal-assistant/src/pages/
```

The current product areas include:

- Dashboard
- Habits
- Goals
- Plan
- Calendar
- Chores
- Notes
- Actions
- Email
- Profile

Public pages include the landing page, Privacy Policy, Terms of Service, and
development sign-in/sign-up routes.

### Finding shared UI

Shared layout and navigation live in:

```text
artifacts/personal-assistant/src/components/layout/
```

Reusable primitives and shadcn/Radix components are under:

```text
artifacts/personal-assistant/src/components/ui/
```

Use the existing design tokens and component patterns before introducing a new
visual system. The Askolo logo is:

```text
artifacts/personal-assistant/public/logo.png
```

### Routing and lazy loading

Route composition is centralized in:

```text
artifacts/personal-assistant/src/routes/
```

`lazy-pages.ts` contains the shared lazy imports. Native authentication remains
in the Go API and the main application path.

When adding a new feature page:

1. Add the page under `src/pages/`.
2. Add its lazy export in `src/routes/lazy-pages.ts`.
3. Add its route to the correct authenticated route tree.
4. Add navigation only where the product flow requires it.
5. Make sure the API hook and loading/error/empty states are present.
6. Run typecheck, build, and a preview check.

Avoid importing the entire feature surface into the initial entry module.

### Data fetching

Frontend API hooks use TanStack React Query and the generated API client.
Current caching behavior favors finite freshness windows, bounded garbage
collection, disabled focus refetching, reconnect refetching, and limited
retries.

For a new query:

- use the generated client where a generated operation exists;
- select a stale time that matches how quickly the data changes;
- invalidate or update related queries after mutations;
- expose loading, empty, error, and retry states;
- avoid duplicate requests when a parent summary already contains the data.

## Backend development

### Adding an API endpoint

1. Add or update the OpenAPI contract in `lib/api-spec/openapi.yaml`.
2. Regenerate shared API code.
3. Add the route implementation under
   `services/askolo-backend/internal/modules/`.
4. Reuse the database schema and shared Zod types.
5. Apply the appropriate authentication and user ownership checks.
6. Validate request body, query, and path inputs.
7. Add structured logs for operational failures without logging secrets or
   sensitive user data.
8. Update the frontend client and UI states.
9. Run typecheck, tests, and a production build.

### Request handling

The API currently configures:

- pino request logging;
- CORS with credentials support;
- bounded JSON parsing;
- URL-encoded body parsing;
- route mounting under `/api`.

Voice fallback uploads use a bounded JSON payload, and individual routes
should still enforce their own decoded-audio limits.

### User data operations

The API contains user data and account deletion operations. Treat changes to
these routes as security-sensitive:

```text
DELETE /api/user/data
DELETE /api/user/account
PATCH  /api/user/profile
```

Always scope reads and writes to the authenticated user. Do not trust a user ID
provided by the browser when the server can derive the identity from the
authenticated session.

## Testing and verification

### Standard verification

Run the full workspace typecheck:

```bash
pnpm run typecheck
```

Run the full build:

```bash
pnpm run build
```

The root build typechecks first and then runs available package build scripts.

### API tests

The Go backend test suite covers the public API modules and persistence
adapters:

```bash
cd services/askolo-backend && go test ./...
```

### Before considering a change complete

Check the entire change, not only the file you edited:

- TypeScript passes.
- Generated clients and schemas are current.
- The relevant API package builds.
- The web package builds with `PORT` and `BASE_PATH`.
- The development workflow starts cleanly.
- The affected route renders in the preview.
- Browser and server logs contain no new unexplained errors.
- Authenticated paths do not leak data between users.
- New user input is validated and safely encoded.
- Secrets are not present in the diff or generated client bundle.

## Production builds and deployment

### Web artifact

The web artifact is defined in:

```text
artifacts/personal-assistant/.replit-artifact/artifact.toml
```

Its production behavior is:

- build with `pnpm --filter @workspace/personal-assistant run build`;
- serve `artifacts/personal-assistant/dist/public` using Replit's managed static
  artifact server;
- route `/` and SPA paths to the frontend;
- use `BASE_PATH=/`;
- expose the local service on port `18131`.

### Go public edge

The Go backend is defined in:

```text
services/askolo-backend/
```

Its production behavior is:

- build with `services/askolo-backend/scripts/build.sh`;
- start with `services/askolo-backend/scripts/run.sh`;
- use port `8090` by default;
- serve `/api` requests and the health/readiness endpoints;
- keep the React frontend as a separate static artifact.

### Deployment checklist

Before publishing:

1. Confirm production secrets are configured in the deployment environment.
2. Confirm the Go backend has production database and provider configuration.
3. Confirm native OAuth callback URLs match the deployed host.
4. Confirm Google OAuth redirect URIs include the intended production host.
5. Run `pnpm run typecheck`.
6. Run `pnpm run build`.
7. Start/restart the relevant workflows and inspect logs.
8. Verify `/api/healthz`.
9. Load the public landing page.
10. Verify sign-in, dashboard navigation, and an authenticated API request.
11. Verify Google integrations only if their production credentials and consent
    configuration are ready.
12. Review the deployed app's browser console and server logs.

### Artifact configuration

Artifact configuration is schema-managed by Replit. Do not replace the
managed static web server with an ad hoc custom server without a supported
artifact configuration path; doing so can break SPA routing and the separate
`/api` service.

Use the artifact and workflow tooling when changing:

- preview paths;
- service ports;
- production build commands;
- production rewrites;
- service health checks;
- artifact registration.

## Performance notes

The frontend has several intentional performance protections:

- Native auth code is kept in the Go API and is not duplicated in the frontend.
- Public, authenticated, and feature routes are lazy-loaded.
- Dashboard AI coaching is deferred until browser idle time.
- Redundant Google status loading was removed from the dashboard.
- React Query avoids aggressive focus refetches and unlimited retries.
- Rollup consolidates tiny chunks while keeping larger feature pages lazy.

The latest production build produced approximately:

- native entry: `382 KB` uncompressed;
- native entry: `127 KB` gzip according to Vite's build report;
- dashboard route chunk: `13 KB` uncompressed;
- output chunks: `55`, down from `64` before small-chunk consolidation.

These values are build-report snapshots, not a promise of fixed asset sizes.
Recheck them after dependency or route changes.

The current Replit managed static artifact server controls production response
compression and asset cache headers. Application code can improve request
selection and client caching, but should not assume that immutable cache
headers or Brotli are enabled unless a production response check confirms it.

## Troubleshooting

### Vite refuses to start

Symptom:

```text
PORT environment variable is required
BASE_PATH environment variable is required
```

Fix:

```bash
PORT=18131 BASE_PATH=/ \
  pnpm --filter @workspace/personal-assistant run dev
```

### Go API refuses to start

The Go backend uses `PORT=8090` by default:

```bash
cd services/askolo-backend && bash ./scripts/run.sh
```

If the API starts but database requests fail, check `DATABASE_URL` and confirm
the database is reachable from the current environment.

### Preview is blank

Check in this order:

1. Confirm the web workflow is running.
2. Read the web workflow logs for a Vite startup error.
3. Confirm the preview path is `/`.
4. Confirm the browser can load the Vite entry module.
5. Check browser console errors.
6. If the frontend calls the API, verify the Go backend workflow separately.
7. Restart the web workflow after changing code, package configuration, or the
   run command.

Do not solve a blank preview by adding a second competing web workflow.

### `/api` requests return 404

Confirm:

- the Go backend workflow is running on port `8090`;
- the request includes the `/api` prefix;
- the route exists in `services/askolo-backend/internal/modules/`;
- the Go edge is serving the `/api` path;
- the frontend is not calling a hard-coded localhost or production URL.

### Protected pages stay on auth loading

Check the auth variables, browser console, and API logs. Direct `/sign-in`
rendering is a useful way to distinguish a route
loading problem from an authenticated-session problem.

### Google OAuth redirects to the wrong page

Check:

- the current request host;
- the Google OAuth client redirect URI;
- `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`;
- `SESSION_SECRET`;
- the signed redirect state/cookie behavior;
- whether the callback returned to the originating page.

### Generated API code is inconsistent

Regenerate from the OpenAPI source:

```bash
pnpm --filter @workspace/api-spec run codegen
pnpm run typecheck
```

Review generated diffs before committing. Do not manually patch generated
files as a substitute for correcting the source contract or generator
configuration.

### Build reports sourcemap-location warnings

Vite may report non-fatal warnings when a dependency's source map does not map
back to its original source location. Distinguish these from actual transform,
typecheck, or runtime failures. Investigate if the warnings become build
failures or point to code changed by the project.

## Security and data-handling guidance

Askolo handles authentication state, user productivity data, email-related
data, calendar data, voice input, and AI requests. Treat all of these as
user-controlled or sensitive.

### Secrets

- Store credentials in Replit Secrets or the deployment secret manager.
- Never commit API keys, OAuth secrets, session secrets, database URLs, or
  private tokens.
- Never expose `GOOGLE_CLIENT_SECRET`, `ASSEMBLY_AI_API_KEY`, or
  `DATABASE_URL` to the browser.
- Keep publishable browser configuration separate from server secrets.

### User data

- Derive ownership from the authenticated session.
- Validate all request bodies, query parameters, path parameters, uploads,
  webhooks, provider responses, and AI output before using them.
- Do not log tokens, authorization headers, raw email content, private notes,
  voice recordings, or full provider payloads.
- Keep voice recordings transient unless an explicit product requirement
  changes the retention model.
- Treat account and data deletion as destructive operations and verify scope
  carefully.

### Dependencies

The workspace enforces a minimum package release age through
`pnpm-workspace.yaml`. Do not disable that control casually. When adding a
dependency:

1. Confirm it is necessary.
2. Check its license and maintenance posture.
3. Follow the workspace package-management rules.
4. Use pnpm.
5. Run typecheck and build.
6. Review the lockfile diff.

## Contribution guide

### General workflow

1. Read `replit.md` and this README before changing project structure.
2. Locate the existing route, component, package, or schema that owns the
   behavior.
3. Prefer a focused change that preserves current architecture.
4. Update contracts before generated consumers.
5. Keep API, database, and UI behavior consistent.
6. Add or update tests for high-risk behavior.
7. Run the full verification commands.
8. Inspect the preview and logs.
9. Review the diff for secrets, generated noise, and unrelated changes.

### Naming and organization

- Keep product code in the artifact or shared package that owns it.
- Use existing aliases such as `@/` in the web app where configured.
- Keep shared API and database contracts in `lib/`.
- Keep route-specific logic in the relevant API route module.
- Avoid putting server-only behavior in browser modules.
- Avoid expanding the initial frontend bundle with unrelated feature imports.

### Pull request or handoff checklist

- [ ] The change has a clear user or operational purpose.
- [ ] The relevant OpenAPI and generated files are updated.
- [ ] Database changes are represented in the Drizzle schema.
- [ ] Authentication and ownership checks are preserved.
- [ ] Inputs and provider responses are validated.
- [ ] Loading, empty, error, and success states are covered in the UI.
- [ ] `pnpm run typecheck` passes.
- [ ] `pnpm run build` passes.
- [ ] Relevant focused tests pass.
- [ ] Preview and workflow logs were checked.
- [ ] No secrets or private data were added.

## License

The root workspace declares the MIT license. Review the license status of any
new dependency before adding it to the project.
