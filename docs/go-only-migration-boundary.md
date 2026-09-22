# Go-only migration boundary

This document records the cleanup boundary for the native Go backend migration.
It is an ownership and retention record, not a replacement for the API
contract or database migration history.

## Removed verified remnants

| Area | Evidence | Disposition |
| --- | --- | --- |
| `lib/replit-auth-web` | No source, package, workflow, or generated consumer imports it. Its helper still pointed at the removed TypeScript `/api/login` and `/api/logout` surface. | Removed. Go owns browser authentication and sessions. |
| `lib/integrations-openai-ai-server` | No runtime, script, artifact, or generated consumer imports it. The package constructed an OpenAI client at module load and duplicated an unreferenced provider mirror. | Removed. Provider access remains behind active Go-owned routes and the AssemblyAI frontend flow. |
| `lib/integrations/openai_ai_integrations` | No package manifest or repository consumer references this source mirror. Its server files constructed OpenAI clients but were not reachable from a workflow or build. | Removed with the unused server package mirror. |
| Appwrite startup ping | `main.tsx` invoked the ping on every browser startup, but no product feature consumed its result and the endpoint was not part of the current runtime boundary. | Removed the startup side effect, client module, and frontend dependency. |

## Retained by design

| Area | Owner | Reason and removal condition |
| --- | --- | --- |
| `lib/api-client-react`, `lib/api-zod`, and OpenAPI outputs | Frontend/API contract owner | Retained because active frontend pages import the generated client and schemas. |
| `/api/google/*` aliases | Go Google integration owner | Retained until active callers are migrated and route smoke tests prove the compatibility surface is no longer needed. |

## Authority checks

- The Go service owns authentication, authorization, sessions, database writes,
  and provider boundaries.
- The frontend calls same-origin `/api` routes and contains no competing
  TypeScript session issuer.
- No TypeScript backend process or provider startup workflow remains.