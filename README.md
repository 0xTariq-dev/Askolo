# Askolo

Askolo is a personal and family assistant for calmer days. It brings habits,
goals, daily plans, calendar events, household chores, notes, action items,
email, and AI assistance into one workspace.

Google Calendar and Gmail are optional integrations. Users can connect them
when they want calendar synchronization, inbox triage, or email workflows, but
the core product is designed to work without Google access.

This repository contains the Askolo web application, its API server, shared
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
│   ├── api-server/               # Express API and production API artifact
│   └── mockup-sandbox/           # Isolated component preview environment
├── lib/
│   ├── api-spec/                 # OpenAPI source and Orval configuration
│   ├── api-client-react/         # React Query API client package
│   ├── api-zod/                  # Shared Zod schemas and generated schemas
│   ├── db/                       # Drizzle schema and database package
│   ├── integrations-openai-ai-react/
│   │                               # Browser-facing AI integration helpers
│   ├── integrations-openai-ai-server/
│   │                               # Server-side AI integration helpers
│   ├── integrations/             # Integration-specific workspace code
│   └── replit-auth-web/          # Shared Replit auth web helpers
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
artifacts/api-server/
├── src/
│   ├── lib/                      # OAuth, AI, logging, voice, and server helpers
│   ├── middlewares/              # Clerk proxy and request middleware
│   ├── routes/                   # Express route modules
│   ├── app.ts                    # Express app composition
│   └── index.ts                  # PORT validation and server startup
├── build.mjs                     # API esbuild entry
└── .replit-artifact/
    └── artifact.toml             # API artifact routing and health check
```

## Architecture

### Web and API boundary

Askolo uses two Replit artifacts:

| Artifact                       | Local port | Routed path | Role                       |
| ------------------------------ | ---------: | ----------- | -------------------------- |
| `artifacts/personal-assistant` |    `18131` | `/`         | React/Vite web application |
| `artifacts/api-server`         |     `8080` | `/api`      | Express API server         |

The browser calls the API through the same application origin using `/api/...`
paths. In production, Replit routes `/api` to the API artifact and all other
paths to the web artifact. This avoids hard-coding a localhost address or a
development domain into browser code.

### Frontend routing

The web application is a Vite single-page application using Wouter for
client-side routing. Production rewrites send unknown frontend paths to
`/index.html` so routes such as `/dashboard`, `/calendar`, and `/notes` can be
loaded directly.

The frontend uses route-level lazy imports. Public pages, the authenticated
layout, feature pages, and development-only Clerk routes are loaded only when
their route is needed.

### Authentication modes

The project intentionally supports two authentication modes:

- **Development and testing:** Clerk is used for the browser and API. Clerk
  routes are loaded lazily so development authentication does not increase
  the initial production bundle.
- **Production:** the application uses the native OAuth/session path. The API
  handles the browser login, callback, logout, and mobile token exchange
  endpoints.

The API checks `NODE_ENV` to select the production authentication path. Do not
change the production/development distinction casually; it affects middleware,
session handling, redirect behavior, and which frontend route tree is loaded.

### API and data flow

The normal authenticated request flow is:

1. The browser sends a request to `/api/...`.
2. Express middleware applies logging, CORS, body parsing, and the appropriate
   authentication middleware.
3. The route validates request data and resolves the current user.
4. The route reads or writes PostgreSQL through Drizzle ORM.
5. Responses are returned using the API shapes defined by the OpenAPI spec and
   shared Zod schemas.

AI-backed routes may also call the OpenAI integration, AssemblyAI, or other
provider-specific helpers. AI credit reservation and reconciliation are
handled server-side.

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
- Clerk React for development authentication
- Appwrite client for startup connectivity checks
- AssemblyAI client support for voice features

### API and persistence

- Express 5
- PostgreSQL
- Drizzle ORM and Drizzle Kit
- Zod and drizzle-zod
- Clerk Express middleware for development
- Pino and pino-http structured logging
- Orval-generated API contracts and clients

### External services

- Replit-managed Clerk configuration for development/testing
- Native OAuth support for production authentication
- Google Cloud OAuth for optional Calendar and Gmail connections
- Replit OpenAI AI Integrations for AI functionality
- AssemblyAI for transcription workflows
- Appwrite for frontend connectivity verification

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
# Terminal 2: API
PORT=8080 \
  pnpm --filter @workspace/api-server run dev
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
| `pnpm --filter @workspace/db run push`          | Push development database schema changes                      |
| `pnpm --filter @workspace/db run push-force`    | Force a development schema push; use carefully                |

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

### API commands

```bash
# Development mode: build the API, then start the generated bundle
PORT=8080 pnpm --filter @workspace/api-server run dev

# Typecheck the API package
pnpm --filter @workspace/api-server run typecheck

# Build the API bundle
pnpm --filter @workspace/api-server run build

# Start the built API bundle
PORT=8080 pnpm --filter @workspace/api-server run start
```

The API entry point validates that `PORT` is present and is a positive number.
The managed API artifact uses port `8080`.

### Optional component preview environment

The mockup sandbox is a separate development artifact for isolated component
previews:

```bash
pnpm --filter @workspace/mockup-sandbox run dev
```

It is not required to run the main Askolo web application.

### Managed Replit workflows

The workspace currently defines these relevant workflows:

| Workflow                                             | Command or role                        |
| ---------------------------------------------------- | -------------------------------------- |
| `artifacts/personal-assistant: web`                  | Runs the Vite web application          |
| `artifacts/api-server: API Server`                   | Runs the Express API                   |
| `artifacts/mockup-sandbox: Component Preview Server` | Runs isolated component previews       |
| `ai-credit-ledger`                                   | Runs the AI credit ledger test command |

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
| `NODE_ENV`  | API and build/runtime code        | `production` selects native production auth; development uses Clerk |
| `PORT`      | Web and API                       | Required by both server entry points                                |
| `BASE_PATH` | Vite web build                    | Required by the web config; `/` is the current artifact base        |
| `REPL_ID`   | Development Vite plugin selection | Managed by Replit when applicable                                   |
| `LOG_LEVEL` | Server logging                    | Optional logging configuration                                      |

### Database and sessions

| Variable         | Used by                      | Notes                                                       |
| ---------------- | ---------------------------- | ----------------------------------------------------------- |
| `DATABASE_URL`   | `lib/db` and Drizzle Kit     | PostgreSQL connection string; required for database work    |
| `SESSION_SECRET` | Google OAuth/session helpers | Keep private; used to protect signed redirect/session state |

### Development authentication

| Variable                     | Used by                                    | Notes                                        |
| ---------------------------- | ------------------------------------------ | -------------------------------------------- |
| `CLERK_PUBLISHABLE_KEY`      | API development middleware                 | Server-side Clerk host/key resolution        |
| `CLERK_SECRET_KEY`           | API user operations and Clerk server calls | Secret; never expose to the browser          |
| `VITE_CLERK_PUBLISHABLE_KEY` | Lazy Clerk web routes                      | Browser-safe publishable key for development |
| `VITE_CLERK_PROXY_URL`       | Lazy Clerk web routes                      | Optional Clerk proxy configuration           |

### Google integrations

| Variable               | Used by                      | Notes                               |
| ---------------------- | ---------------------------- | ----------------------------------- |
| `GOOGLE_CLIENT_ID`     | Google OAuth and native auth | OAuth client identifier             |
| `GOOGLE_CLIENT_SECRET` | Google OAuth and native auth | Secret; never expose to the browser |

Google Calendar and Gmail are opt-in. The rest of the application should
remain usable when these variables are not configured, but Google connect
flows require the corresponding OAuth setup and redirect URIs.

### AI and transcription

| Variable                          | Used by                           | Notes                                         |
| --------------------------------- | --------------------------------- | --------------------------------------------- |
| `AI_INTEGRATIONS_OPENAI_API_KEY`  | Replit OpenAI integration helpers | Secret; use the managed integration secret    |
| `AI_INTEGRATIONS_OPENAI_BASE_URL` | Replit OpenAI integration helpers | Provider base URL supplied by the integration |
| `ASSEMBLY_AI_API_KEY`             | AssemblyAI server helper          | Secret required for transcription             |
| `ASSEMBLYAI_REGION`               | AssemblyAI server helper          | Optional/provider-specific region selection   |

### Environment rules

- Do not commit `.env` files or secret values.
- Do not put server-only variables in `VITE_*` variables.
- A `VITE_*` variable can be included in the browser bundle; only use it for
  public configuration.
- Restart the relevant workflow after changing environment variables.
- If a value is missing, fix the environment configuration instead of adding a
  silent fallback in application code.

## Authentication

### Development and testing with Clerk

Development mode keeps Clerk available for local sign-in and sign-up testing.
The frontend loads Clerk routes dynamically, and the API adds Clerk middleware
and the Clerk proxy only outside production mode.

Useful development routes include:

- `/sign-in`
- `/sign-up`
- `/sso-callback`
- `/dashboard` after authentication

Clerk development keys are expected to produce a development-mode warning in
the browser. Do not deploy development keys to production.

### Production native OAuth

Production uses the native OAuth/session path instead of loading the Clerk
frontend runtime. The API exposes browser authentication routes under `/api`:

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

1. Check that the web and API workflows are both running.
2. Confirm the browser request to `/api/auth/user` is reaching the API artifact.
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
artifacts/api-server/src/lib/googleOAuth.ts
artifacts/api-server/src/lib/googleCalendar.ts
artifacts/api-server/src/lib/gmail.ts
artifacts/api-server/src/lib/googleStatus.ts
artifacts/api-server/src/routes/google.ts
```

### OpenAI AI Integrations

AI features use Replit-managed OpenAI integration configuration rather than
hard-coded provider credentials. Shared React and server packages provide
provider helpers for text, image, audio, and batch-related functionality.

Relevant packages:

```text
lib/integrations-openai-ai-react/
lib/integrations-openai-ai-server/
lib/integrations/openai_ai_integrations/
```

AI-backed API operations use server-side credit reservation and reconciliation.
Do not call paid providers directly from the browser.

### AssemblyAI

AssemblyAI supports voice/transcription workflows. The server owns provider
access and the browser-side package provides reusable voice input behavior.

Relevant code:

```text
artifacts/api-server/src/lib/assemblyai.ts
lib/integrations-openai-ai-react/src/audio/
lib/integrations-openai-ai-server/src/audio/
```

Voice recordings are treated as transient input. Avoid persisting recordings
unless a product requirement explicitly changes that behavior.

### Appwrite

The frontend performs a lightweight Appwrite startup ping to verify the
configured Appwrite connection. A successful ping is logged in the browser
console. A failed ping should be investigated separately from API
authentication and database failures.

The Appwrite client is in:

```text
artifacts/personal-assistant/src/lib/appwrite.ts
```

## Database

The database package uses PostgreSQL with Drizzle ORM.

### Schema location

Database schema modules live in:

```text
lib/db/src/schema/
```

Current schema areas include:

- users and authentication
- habits and habit completions
- goals
- daily plans
- calendar events
- chores
- notes
- action items
- conversations and messages
- Gmail tokens
- Google connections
- AI credits
- voice preferences

### Development schema commands

The database package exposes:

```bash
# Push the current schema to the development database
pnpm --filter @workspace/db run push

# Force the schema push when Drizzle requires confirmation
pnpm --filter @workspace/db run push-force
```

Use `push-force` only when you understand the schema change and its impact.
Never point a development schema command at production without following the
project's production database migration procedure.

The Drizzle configuration is:

```text
lib/db/drizzle.config.ts
```

It uses `DATABASE_URL`. Keep the connection string out of shell history,
source control, and documentation.

## API reference and code generation

### API route groups

The Express API is mounted under `/api`. Route modules are in
`artifacts/api-server/src/routes/`.

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
curl -i http://localhost:8080/api/healthz
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

`lazy-pages.ts` contains the shared lazy imports. Development-only Clerk
composition is in `clerk-routes.tsx`. Native production authentication remains
in the main application path.

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
   `artifacts/api-server/src/routes/`.
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
- development-only Clerk proxy and middleware;
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

The API package currently exposes focused tests for the AI credit ledger and
voice consent behavior:

```bash
pnpm --filter @workspace/api-server run test:ledger
pnpm --filter @workspace/api-server run test:voice-consent
```

The managed AI ledger workflow runs the ledger test command.

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

### API artifact

The API artifact is defined in:

```text
artifacts/api-server/.replit-artifact/artifact.toml
```

Its production behavior is:

- build with `pnpm --filter @workspace/api-server run build`;
- start `artifacts/api-server/dist/index.mjs`;
- use port `8080`;
- route `/api` requests to the API service;
- use `/api/healthz` as the startup health check.

### Deployment checklist

Before publishing:

1. Confirm production secrets are configured in the deployment environment.
2. Confirm `NODE_ENV=production` for the production API.
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

- Clerk code is not eagerly loaded in the native production entry.
- Public, authenticated, and feature routes are lazy-loaded.
- Dashboard AI coaching is deferred until browser idle time.
- Redundant Google status loading was removed from the dashboard.
- React Query avoids aggressive focus refetches and unlimited retries.
- Rollup consolidates tiny chunks while keeping larger feature pages lazy.

The latest production build produced approximately:

- native entry: `382 KB` uncompressed;
- native entry: `127 KB` gzip according to Vite's build report;
- Clerk route chunk: `95 KB` uncompressed;
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

### API refuses to start

The API entry point requires a valid positive `PORT`:

```bash
PORT=8080 pnpm --filter @workspace/api-server run dev
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
6. If the frontend calls the API, verify the API workflow separately.
7. Restart the web workflow after changing code, package configuration, or the
   run command.

Do not solve a blank preview by adding a second competing web workflow.

### `/api` requests return 404

Confirm:

- the API workflow is running on port `8080`;
- the request includes the `/api` prefix;
- the API route exists in `artifacts/api-server/src/routes/`;
- the API artifact routes `/api` to the API service;
- the frontend is not calling a hard-coded localhost or production URL.

### Protected pages stay on auth loading

This can happen when the local Clerk development handshake is not complete.
Check the auth variables, Clerk proxy configuration, browser console, and API
logs. Direct `/sign-in` rendering is a useful way to distinguish a route
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
- Never expose `CLERK_SECRET_KEY`, `GOOGLE_CLIENT_SECRET`,
  `AI_INTEGRATIONS_OPENAI_API_KEY`, `ASSEMBLY_AI_API_KEY`, or `DATABASE_URL`
  to the browser.
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
