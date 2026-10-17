-- The verified-signup grant is configured independently from the recurring
-- grant. Zero means the operator has not enabled a signup grant yet.
ALTER TABLE ai_credit_policy_versions
  ADD COLUMN IF NOT EXISTS signup_grant_usd_micros bigint NOT NULL DEFAULT 0
  CHECK (signup_grant_usd_micros >= 0);
