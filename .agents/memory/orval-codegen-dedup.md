---
name: Orval codegen dedup
description: After regenerating clients from the OpenAPI spec, the generated api-zod barrel exports conflicting names and must be trimmed manually.
---

## Rule

After running `pnpm --filter @workspace/api-spec run codegen`, inspect `lib/api-zod/src/index.ts`.

Orval writes an index that re-exports both `./generated/api` (Zod schemas) and `./generated/types` (TypeScript interfaces). Several names—such as `AssistantChatBody`, `MeetingExtractBody`, `VoiceToPlanBody`, and their response counterparts—exist in both, causing TypeScript error `TS2308: Module has already exported a member named ...`.

**Fix:** keep only the Zod schema export:

```ts
export * from "./generated/api";
```

**Why:** The API server uses only the Zod schemas for request/response validation. The frontend consumes typed hooks from `@workspace/api-client-react`, not from `@workspace/api-zod`'s generated types, so dropping the `types` export has no downstream impact.

**How to apply:** Run this after every `api-spec` codegen run, or after any other tool/agent regenerates the OpenAPI clients. If the index file grows duplicate/conflicting exports again, trim it back to the single api export.
