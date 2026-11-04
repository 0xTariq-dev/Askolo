---
name: MFA alert policy deployment
description: Production verification constraints for MFA recovery alert routing.
---

Check the live Datadog monitor configuration, route, and severity before sending production synthetic alert samples. A repository policy file and its hash validator prove only that the checked-in bytes match the reviewed baseline; they do not prove that production has loaded the policy or that its routing matches.

When changing or rolling back a Datadog monitor through its update API, include the `query` field explicitly. Omitting it from the update body preserves the prior query instead of restoring the desired query.

**Why:** A production monitor may remain active with one generic P2 route while the repository policy specifies distinct warning-ticket and high/critical-page actions. Triggering it without checking can notify the wrong destination or at the wrong severity.

**How to apply:** Compare the live monitor query, priority, notification handle, and observed alert/recovery events with the requested policy. If they differ, do not test the expected policy path until the mismatch is resolved or the user explicitly authorizes a narrower test of the existing route. Include both the new and original query in update and rollback requests.
