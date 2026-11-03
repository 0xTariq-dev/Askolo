---
name: Askolo R2 residency routing
description: Configure separate US/EU R2 buckets per environment and route by residency.
---

Askolo profile photos and attachments must use separate US- and EU-jurisdiction R2 buckets in both Development and Production. Keep each environment's credentials scoped to its own two buckets; do not copy Development credentials to Production. Until user residency data is collected, route all storage to US. Shared Space attachments follow the uploader's residency, and list/download paths must search across regional buckets. Do not use location hints as a substitute for jurisdiction. The existing Development bucket remains untouched.

The Cloudflare API connector cannot set the `cf-r2-jurisdiction` header required by the bucket API. Create jurisdiction buckets through Cloudflare's “Specify jurisdiction” option. Jurisdiction-specific buckets also require their corresponding S3 endpoints (`.us.r2.cloudflarestorage.com` or `.eu.r2.cloudflarestorage.com`).

**Why:** the user explicitly requested separate US/EU buckets per environment, production support, and temporary US routing until residency collection exists.

**How to apply:** route uploads from validated persisted residency once available; keep profile-photo reads tied to their stored region and shared attachments tied to the uploader's region. Preserve the legacy Development bucket for old files unless migration is explicitly approved.

Replit's secure `requestSecrets` flow saved the submitted R2 key names in Shared scope, which makes them visible to both Development and Production. Do not use that flow for credentials that must differ by environment.

**Why:** the user requires separate Cloudflare credentials for the Development and Production bucket pairs; shared secrets break that isolation.

**How to apply:** enter Development credentials through the Project Editor's Development Secrets and Production credentials through Publishing's deployment secrets. Check secret presence by scope before enabling regional bucket variables; never inspect or log values.
