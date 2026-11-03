---
name: Askolo R2 residency routing
description: Configure separate US/EU R2 buckets per environment and route by residency.
---

Askolo profile photos and attachments use separate US- and EU-jurisdiction R2 buckets in both Development and Production. Keep each environment's credentials scoped to its own two buckets; do not copy Development credentials to Production. Until trusted user residency data exists, route new uploads to US. Shared Space attachments follow the uploader's residency, and list/download paths must search across regional buckets. Do not use location hints as a substitute for jurisdiction. The retired Development bucket `askolo-development-files` was verified empty in the exact Development account before deletion; old profile-photo URLs marked `region=legacy` now resolve to US.

The Cloudflare API connector cannot set the `cf-r2-jurisdiction` header required by the bucket API. Create jurisdiction buckets through Cloudflare's “Specify jurisdiction” option. Jurisdiction-specific buckets also require their corresponding S3 endpoints (`.us.r2.cloudflarestorage.com` or `.eu.r2.cloudflarestorage.com`).

**Why:** the user explicitly requested separate US/EU buckets per environment, production support, and temporary US routing until residency collection exists.

**How to apply:** route uploads from validated persisted residency once available; keep profile-photo reads tied to their stored region and shared attachments tied to the uploader's region. Remove legacy bucket configuration before retiring old storage, and map legacy profile-photo markers to US after confirming no old objects remain.

The user confirmed that Development and Production R2 credentials are unsynced and have different values. Environment secret-status checks reveal key-name presence only; they cannot establish whether values are equal or synced. The current Replit secret callbacks do not delete secret entries.

**Why:** the user corrected an earlier inference from secret-presence output; scope presence is not evidence that secret values are shared.

**How to apply:** never infer credential equality from secret names or presence status. Keep regional credentials environment-scoped, and remove obsolete secrets through Replit's Secrets UI when the available callbacks cannot delete them; never inspect or log values.
