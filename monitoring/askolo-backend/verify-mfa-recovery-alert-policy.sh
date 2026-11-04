#!/usr/bin/env bash
set -euo pipefail

# The reviewed policy is intentionally pinned by bytes, not parsed fields.
# Keep failure output limited to the path and reason; never print the policy.

policy_path="${1:-monitoring/askolo-backend/mfa-recovery-alerts.yaml}"
expected_sha256="9e9b9ba2f227ea1bea134ef079ff778b15f5258c550106d8d96112e7d366783b"

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