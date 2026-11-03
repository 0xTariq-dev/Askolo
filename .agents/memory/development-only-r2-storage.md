---
name: Development-only R2 storage
description: Keep Askolo profile-photo and attachment R2 work isolated to Development.
---

Use the existing Cloudflare R2 Development configuration for Askolo profile photos and Note/Task attachments. Do not access or modify Production resources as part of this feature.

**Why:** the user explicitly restricted this work to Development and forbade Production access or changes.

**How to apply:** confirm the Development target before R2 or database testing; use only disposable Development databases for integration tests.
