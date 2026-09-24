# Threat Model — Askolo

## Project Overview

Askolo is a personal/family assistant web app: habits, goals, daily plans, calendar,
chores, notes, action items, and an AI coach, with optional Google Calendar/Gmail and
GitHub integrations. Stack: Go HTTP backend (`services/askolo-backend`, native auth +
sessions + MFA + OAuth) with a PostgreSQL store, and a React/Vite SPA
(`artifacts/personal-assistant`). Deployed as a public Autoscale deployment
(`https://dev.askolo.app`, `https://askolo.replit.app`), so all `/api/...` endpoints are
internet-reachable.

## Assets

- **User accounts and sessions** — email, hashed passwords, session cookies/IDs, MFA/TOTP
  secrets and recovery codes. Compromise enables impersonation and full data access.
- **Personal data** — habits, goals, plans, events, chores, notes, action items, profile.
- **Google/GitHub OAuth tokens** — encrypted at rest; grant access to the user's Calendar/Gmail.
- **Application secrets** — DB connection, session/cookie signing keys, AES-GCM sealing key,
  internal auth token, AssemblyAI API key. AssemblyAI key is operator-funded (billing risk).

## Trust Boundaries

- **Browser → `/api/...`** — untrusted client; auth via session cookie or `Bearer <sessionID>`.
  Every sensitive route calls `sessionUserID` (validates session + MFA state) then `authorize`.
- **`/internal/...`, `/internal/ws`, `/internal/rest`, `/internal/authz`** — gated by
  `internalAuth` token middleware; not for public callers.
- **API → PostgreSQL** — all product CRUD scoped by `user_id` with parameterized queries;
  table/column names are static specs (no injection). Mass assignment blocked via allow-list
  + `DisallowUnknownFields`.
- **API → Google/Gmail/GitHub/AssemblyAI** — outbound provider calls use fixed endpoints;
  OAuth callbacks require signed, expiring, one-time state bound to a browser cookie.

## Scan Anchors

- Production entry points: `internal/httpapi/router.go` (mux + `apiMux`), `modules/product/handler.go`
  (product + AI), `modules/auth/handler.go` (auth/MFA), `modules/google/**`, `modules/github/**`.
- Highest-risk areas: AI/billing endpoints (`/api/ai/*`), auth throttling/recovery, OAuth callbacks.
- Public surface: `/`, `/healthz`, `/readyz`, `/api/auth/*`, `/ws`, `/webhooks/*`.
  Authenticated surface: `/api/{habits,goals,daily-plans,events,chores,notes,action-items,dashboard,ai,user}/*`.
  Internal surface: `/internal/*` (token-gated).
- Dev-only / ignore unless proven reachable: `.agents/skills/**` (SAST path-traversal hits in a
  Python bridge server used by tooling skills, not part of the deployed app).

## Threat Categories

### Spoofing / Authentication
Session IDs are 192-bit CSPRNG values; MFA challenges enforce expiry, ownership, and attempt
limits; OAuth state is signed, one-time, and cookie-bound. **Gap:** password-login and
recovery throttling keys off the first `X-Forwarded-For` value, which an internet caller can
spoof per request to bypass rate limits and grow the in-memory limiter map unbounded
(see `authentication-issues`). Password-recovery responses also allow account/eligibility
enumeration.

### Elevation of Privilege / Access Control
Product CRUD is consistently scoped to the session user in SQL; no cross-user IDOR, mass
assignment, or missing function-level authz was found. Profile updates accept only names;
account deletion is bound to the session user. Internal routes are token-gated.

### Information Disclosure
No client-side secret exposure: `VITE_` values are public config; AssemblyAI key stays
server-side (realtime tokens are short-lived). Auth logs include email at LOW severity only.

### Tampering / Business Logic
AI credit balance is displayed and estimable but **not enforced or reserved** before invoking
the paid AssemblyAI provider in `/api/ai/transcribe-audio` and `/api/ai/realtime-token`,
letting any consented authenticated user drive operator billing (see `ai-billing`).

### Denial of Service
Request bodies for product/audio endpoints are size-capped. The auth limiter map has no
expiry eviction (memory growth) — tied to the forwarded-IP finding above.
