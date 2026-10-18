ALTER TABLE ai_credit_user_policy_overrides
  ADD COLUMN grant_expiry_days integer
  CHECK (grant_expiry_days IS NULL OR grant_expiry_days BETWEEN 1 AND 3650);
