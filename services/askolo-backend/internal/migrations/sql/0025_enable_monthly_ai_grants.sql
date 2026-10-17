ALTER TABLE ai_credit_policy_versions
  ADD COLUMN IF NOT EXISTS monthly_grants_enabled boolean NOT NULL DEFAULT false;
