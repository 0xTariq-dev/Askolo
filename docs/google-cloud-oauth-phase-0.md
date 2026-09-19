# Askolo Google Cloud OAuth — Phase 0 Checklist

This checklist covers Google Cloud configuration for the Go Google-integration
cutover. It deliberately does not request restricted Gmail mailbox scopes.

## Current deployment facts

- Published deployment is active and public.
- Production domains currently include:
  - `https://askolo.app`
  - `https://web.askolo.app`
- The product OAuth host selected for this project is:
  - `https://web.askolo.app`
- The current TypeScript API derives callbacks from the incoming host.
- The current TypeScript login and Google integration flows share:
  - `GOOGLE_CLIENT_ID`
  - `GOOGLE_CLIENT_SECRET`
- The Go backend does not yet consume Google OAuth credentials.

## Google Cloud projects

Use separate Google Cloud projects for production and staging when possible.
This keeps consent-screen configuration, test users, OAuth clients, quotas, and
verification state from being mixed across environments.

Record these values outside source control:

| Environment | Google Cloud project ID | Status |
|---|---|---|
| Production | `life-organizer-503015` | Provided |
| Staging | `directed-craft-499422-n6` | Provided |

Stable hosts:

- Production OAuth host: `web.askolo.app`
- Staging OAuth host: `staging.askolo.app`

## APIs to enable

Enable these APIs in both projects:

- Google Calendar API
- Gmail API

No Gmail Pub/Sub or push configuration is needed for release 1 because release
1 uses only `gmail.send`. Pub/Sub belongs to the later restricted-Gmail phase.

## OAuth consent configuration

Configure the Google Auth Platform consent screen in each project:

- App name: `Askolo`
- User support email: the Askolo support address
- Developer contact email: the Askolo operational contact
- Authorized domain: `askolo.app` (the registered root domain covers the
  `web.askolo.app` and `staging.askolo.app` hosts)
- Privacy Policy URL: `https://askolo.app/privacy`
- Terms of Service URL: `https://askolo.app/terms`
- Audience: External unless the product is intentionally restricted to one
  Google Workspace organization
- Add only the release-1 scopes:
  - `openid`
  - `email`
  - `profile`
  - `https://www.googleapis.com/auth/gmail.send`
  - `https://www.googleapis.com/auth/calendar`

The Gmail and Calendar scopes are separate permissions even when requested in
one consent transaction. Do not add `gmail.modify`, `gmail.readonly`,
`gmail.metadata`, `gmail.compose`, or `mail.google.com` during Phase 0.

Add the intended staging and production test accounts to the test-user list
before testing an External consent screen.

## OAuth clients

Create separate Web application OAuth clients for login and integrations.
Use the same client for an environment's login routes across that environment,
and a different client for that environment's Google integrations.

### Production login client

Authorized redirect URI:

```text
https://web.askolo.app/api/auth/google/callback
```

### Production integration client

Canonical Go callback to register for the cutover:

```text
https://web.askolo.app/api/integrations/google/callback
```

Temporary compatibility callback to keep registered until the TypeScript
integration is removed:

```text
https://web.askolo.app/api/google/gmail/callback
```

The compatibility callback is not the target architecture and should be
removed from the OAuth client after the Go cutover is verified.

### Staging login client

The stable staging hostname is `staging.askolo.app`:

```text
https://staging.askolo.app/api/auth/google/callback
```

### Staging integration client

```text
https://staging.askolo.app/api/integrations/google/callback
```

If the old TypeScript integration is exercised in staging during migration,
also register:

```text
https://staging.askolo.app/api/google/gmail/callback
```

Register each URI exactly. Do not use wildcard paths, unstable `*.replit.dev`
hosts, or guessed domains.

## Replit secret mapping

After the OAuth clients exist, add environment-specific secrets through the
workspace secret manager. The names planned for the Go cutover are:

```text
GOOGLE_LOGIN_CLIENT_ID
GOOGLE_LOGIN_CLIENT_SECRET
GOOGLE_INTEGRATION_CLIENT_ID
GOOGLE_INTEGRATION_CLIENT_SECRET
```

Use the same names in the shared code with separate development/staging and
production values supplied by the environment. Do not display or commit any
client secret.

Keep the existing `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` until the
TypeScript compatibility flow has been migrated and removed. Do not overwrite
them with integration-client values before the Go cutover.

## Go backend readiness checks

Before deployment work begins, the Go service still needs:

- Database access through the workspace's supported database configuration
- Authenticated user identity propagation from the public API during the
  companion period
- Separate public OAuth routes for login and integration
- A production-safe token encryption/key strategy
- A stable staging hostname and routing path to the Go service
- Health and readiness checks that do not expose configuration secrets

## Phase 0 exit criteria

- Production and staging Google Cloud project IDs are recorded.
- Stable staging hostname is selected.
- Both projects have Calendar API and Gmail API enabled.
- Consent-screen branding and authorized domains are configured.
- Release-1 scopes are listed and no restricted Gmail scopes are added.
- Four environment-appropriate OAuth clients exist:
  production login, production integration, staging login, and staging
  integration.
- Exact callbacks are registered.
- OAuth client IDs are ready to be stored as Replit secrets, without sharing
  client secrets in chat or source control.

## User-controlled actions still required

1. Create the four OAuth clients in the two provided projects and add their
   IDs/secrets to the appropriate
   Replit environments when requested.
2. Add the intended Google test accounts in each project's consent-screen
   audience configuration.

The project IDs, staging hostname, production OAuth host, privacy URL, and
terms URL have already been provided and recorded above.