# Threat Model — Askolo

## Project Overview

Askolo is a personal and family assistant for planning, habits, calendar, notes, and AI
features, with optional Google Calendar/Gmail and GitHub integrations. It uses a Go API
(`services/askolo-backend`), PostgreSQL, and a React/Vite web app
(`artifacts/personal-assistant`). This workspace is the Development Repl; production is
managed separately. Do not infer production behavior from Development-only tests.

## Assets

- **Accounts and sessions** — verified email addresses, password hashes, session IDs, and
  OAuth identities. A compromise can enable account takeover.
- **MFA recovery material** — encrypted TOTP secrets; one-time recovery-code hashes; email
  challenge hashes; trusted-device credential and browser-fingerprint HMACs. These control
  recovery and MFA bypass and must not be exposed in logs or API responses.
- **Personal data** — plans, habits, calendar events, notes, profile details, and related
  user-created content.
- **Provider access and billing** — encrypted Google/GitHub tokens, provider credentials,
  AI credit balances and reservations, and operator-funded API usage.
- **Application secrets** — database URLs, session and encryption keys, internal-service
  credentials, and third-party API credentials.

## Trust Boundaries

- **Browser → API** — treat request bodies, headers, cookies, and client-side permissions as
  untrusted. Protected routes must authenticate and authorize on the server.
- **API → PostgreSQL** — the backend handles sessions, personal data, MFA state, credits,
  and provider tokens. Queries must be parameterized and user-scoped.
- **API → external providers** — OAuth and AI provider calls cross an external trust boundary;
  use fixed provider endpoints, validated callback state, bounded timeouts, and server-held
  credentials.
- **Public → authenticated → internal/admin** — public auth and health routes are distinct
  from user-scoped routes and token-gated internal operations. Preserve these boundaries
  in both routing and authorization checks.
- **Development → production** — this workspace is not authority to migrate or alter
  production data or configuration.

## Scan Anchors

- API routing and request boundaries: `services/askolo-backend/internal/httpapi/`.
- Highest-risk code: `internal/modules/auth/` (sessions, MFA, recovery, throttling),
  `internal/modules/google/` and `internal/modules/github/` (OAuth), and
  `internal/modules/product/` (personal data, AI, and credit enforcement).
- The web client is `artifacts/personal-assistant/`; `.agents/skills/**` contains tooling
  and is not part of the deployed application unless a deployment path proves otherwise.

## Threat Categories

### Spoofing

Password, Google, and GitHub sign-ins must enforce MFA when it is enabled. A pending MFA
session must not authorize ordinary app routes. Recovery codes are single-use and stored
only as keyed hashes. Email recovery requires the account password and an independently
verified recovery email; success removes MFA and revokes sessions and trusted devices
without signing the requester in. Trusted devices use a random, HttpOnly credential plus
a browser-generated random fingerprint, store only keyed hashes, expire after 30 days,
and can be revoked. Sensitive actions require a fresh TOTP. Recovery and login controls
must use shared server-side limits and generic responses to prevent guessing and account
enumeration; proxy-derived client IPs must only be trusted from verified proxies.

For P09, MFA recovery and trusted-device controls belong to the individual login account
holder, including in family Spaces. A Space owner, parent, or guardian cannot view another
account's recovery material or manage its trusted devices; guardian-managed child-account
recovery is outside this scope.

### Tampering

Untrusted input must be validated and encoded at the server boundary; SQL must remain
parameterized and authorization must not depend on client-provided ownership. MFA changes,
recovery-code rotation, session revocation, and credit-ledger operations must be atomic
and replay-safe. Paid AI operations must reserve credits before provider execution and
reconcile provider usage without charging twice.

### Repudiation

Sensitive authentication events, including recovery, MFA failures, and trusted-device
changes, should record the account and action without recording passwords, codes, raw
fingerprints, session credentials, or provider secrets. Logs and event records must not
be treated as proof of tamper-resistant storage unless that property is separately
verified.

### Information Disclosure

Never return or log raw recovery codes, email challenge codes, TOTP secrets, trusted-device
credentials, or browser fingerprints. Encrypt TOTP secrets at rest; retain only keyed
hashes for one-time codes and trusted-device values. Keep provider and internal credentials
server-side, return only user-scoped data, and keep public errors generic where account
eligibility could otherwise be inferred.

### Denial of Service

Public auth and recovery routes need shared IP/account throttles, challenge attempt limits,
email cooldowns, and bot checks where configured. Bound request bodies and provider calls.
Process-local throttles may supplement these controls, but must not be the only control
for multi-instance or cross-process abuse.

### Elevation of Privilege

Every user-data route must enforce a valid, fully authenticated session and server-side
resource authorization. Internal routes must require the internal credential. Workspace
or object access must fail closed when ownership or permission context is incomplete.
