#!/usr/bin/env bash
set -euo pipefail

# The reviewed policy is intentionally pinned by bytes, not parsed fields.
# Keep failure output limited to the path and reason; never print the policy.

policy_path="${1:-monitoring/askolo-backend/mfa-recovery-alerts.yaml}"
expected_sha256="731b0076dd9d3bc5af08302bea388b252e7cd0bbcb931615dcfa044c5c781859"

if [[ ! -f "$policy_path" ]]; then
  echo "MFA alert-policy validation failed: $policy_path is missing" >&2
  exit 1
fi

actual_sha256="$(sha256sum -- "$policy_path" | awk '{print $1}')"
if [[ "$actual_sha256" != "$expected_sha256" ]]; then
  echo "MFA alert-policy validation failed: $policy_path differs from the reviewed baseline" >&2
  exit 1
fi

echo "MFA alert policy verified: $policy_path"