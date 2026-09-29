-- Additive USD micro-unit ledger. Legacy credit rows remain unchanged and are
-- explicitly marked as CREDITS; no implicit credit-to-dollar conversion exists.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM ai_credit_accounts
    WHERE granted_credits + adjustment_credits + refunded_credits
      - reserved_credits - spent_credits <> 0
  ) OR EXISTS (
    SELECT 1 FROM ai_credit_reservations
    WHERE status IN ('reserved','claimed')
  ) THEN
    RAISE EXCEPTION 'USD ledger migration refused: legacy balance or open reservation exists';
  END IF;
END $$;

ALTER TABLE ai_credit_accounts
  ADD COLUMN IF NOT EXISTS granted_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS adjustment_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS reserved_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS spent_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS refunded_usd_micros bigint NOT NULL DEFAULT 0;

ALTER TABLE ai_credit_grants
  ADD COLUMN IF NOT EXISTS currency varchar(8) NOT NULL DEFAULT 'CREDITS',
  ADD COLUMN IF NOT EXISTS amount_usd_micros bigint NOT NULL DEFAULT 0;
ALTER TABLE ai_credit_adjustments
  ADD COLUMN IF NOT EXISTS currency varchar(8) NOT NULL DEFAULT 'CREDITS',
  ADD COLUMN IF NOT EXISTS amount_usd_micros bigint NOT NULL DEFAULT 0;
ALTER TABLE ai_credit_reservations
  ADD COLUMN IF NOT EXISTS currency varchar(8) NOT NULL DEFAULT 'CREDITS',
  ADD COLUMN IF NOT EXISTS reserved_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS settled_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS refunded_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS max_usd_micros bigint,
  ADD COLUMN IF NOT EXISTS rate_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE ai_credit_reservation_events
  ADD COLUMN IF NOT EXISTS currency varchar(8) NOT NULL DEFAULT 'CREDITS',
  ADD COLUMN IF NOT EXISTS amount_usd_micros bigint NOT NULL DEFAULT 0;
ALTER TABLE ai_provider_usage_evidence
  ADD COLUMN IF NOT EXISTS usage_unit varchar(32),
  ADD COLUMN IF NOT EXISTS usage_units bigint,
  ADD COLUMN IF NOT EXISTS input_tokens bigint,
  ADD COLUMN IF NOT EXISTS output_tokens bigint,
  ADD COLUMN IF NOT EXISTS payload jsonb;
ALTER TABLE assistant_runs
  ADD COLUMN IF NOT EXISTS currency varchar(8) NOT NULL DEFAULT 'CREDITS',
  ADD COLUMN IF NOT EXISTS base_usd_micros bigint,
  ADD COLUMN IF NOT EXISTS reserved_usd_micros bigint,
  ADD COLUMN IF NOT EXISTS settled_usd_micros bigint NOT NULL DEFAULT 0;
ALTER TABLE assistant_runs
  DROP CONSTRAINT IF EXISTS assistant_runs_base_credits_check,
  DROP CONSTRAINT IF EXISTS assistant_runs_reserved_credits_check,
  ADD CONSTRAINT assistant_runs_currency_amounts_check CHECK (
    (currency = 'CREDITS' AND base_credits > 0 AND reserved_credits > 0 AND base_usd_micros IS NULL AND reserved_usd_micros IS NULL)
    OR
    (currency = 'USD' AND base_credits = 0 AND reserved_credits = 0 AND COALESCE(base_usd_micros,0) > 0 AND COALESCE(reserved_usd_micros,0) > 0)
  );
ALTER TABLE ai_credit_reservations
  DROP CONSTRAINT IF EXISTS ai_credit_reservations_reserved_credits_check,
  DROP CONSTRAINT IF EXISTS ai_credit_reservations_settled_credits_check,
  DROP CONSTRAINT IF EXISTS ai_credit_reservations_refunded_credits_check,
  ADD CONSTRAINT ai_credit_reservations_currency_amounts_check CHECK (
    (currency = 'CREDITS' AND reserved_credits >= 0 AND settled_credits >= 0 AND refunded_credits >= 0)
    OR
    (currency = 'USD' AND reserved_credits = 0 AND settled_credits = 0 AND refunded_credits = 0)
  );
ALTER TABLE ai_credit_policy_versions
  ADD COLUMN IF NOT EXISTS rate_cards jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS monthly_grant_usd_micros bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS rollover_cap_usd_micros bigint NOT NULL DEFAULT 0;

ALTER TABLE ai_credit_policy_versions
  ALTER COLUMN rate_cards SET DEFAULT
    '{"assemblyai:recorded:universal-3-5-pro":{"provider":"assemblyai","mode":"recorded","model":"universal-3-5-pro","meter":"hour","usdMicrosPerHour":210000},"assemblyai:realtime:universal-3-5-pro":{"provider":"assemblyai","mode":"realtime","model":"universal-3-5-pro","meter":"hour","usdMicrosPerHour":450000}}'::jsonb;
-- Keep all historical policy snapshots immutable. The first USD-aware policy
-- is a new version carrying the prior credit-policy values verbatim.
INSERT INTO ai_credit_policy_versions (
  version, operation_weights, monthly_grant_credits, rollover_cap_credits,
  rollover_expiry_days, overrun_margin_percent, created_by, change_reason,
  rate_cards, monthly_grant_usd_micros, rollover_cap_usd_micros
)
SELECT version + 1, operation_weights, monthly_grant_credits, rollover_cap_credits,
  rollover_expiry_days, overrun_margin_percent, 'system-migration',
  'Initialize USD micro-unit policy',
  '{"assemblyai:recorded:universal-3-5-pro":{"provider":"assemblyai","mode":"recorded","model":"universal-3-5-pro","meter":"hour","usdMicrosPerHour":210000},"assemblyai:realtime:universal-3-5-pro":{"provider":"assemblyai","mode":"realtime","model":"universal-3-5-pro","meter":"hour","usdMicrosPerHour":450000}}'::jsonb,
  0, 0
FROM ai_credit_policy_versions
WHERE version = (SELECT max(version) FROM ai_credit_policy_versions)
ON CONFLICT (version) DO NOTHING;

ALTER TABLE ai_credit_grants ADD CONSTRAINT ai_usd_grants_amount_nonnegative CHECK (amount_usd_micros >= 0) NOT VALID;
ALTER TABLE ai_credit_adjustments ADD CONSTRAINT ai_usd_adjustments_amount_bounded CHECK (amount_usd_micros BETWEEN -9223372036854775807 AND 9223372036854775807) NOT VALID;
ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_usd_reservations_amount_nonnegative CHECK (reserved_usd_micros >= 0 AND settled_usd_micros >= 0 AND refunded_usd_micros >= 0 AND (max_usd_micros IS NULL OR max_usd_micros >= 0)) NOT VALID;
ALTER TABLE ai_credit_reservation_events ADD CONSTRAINT ai_usd_events_amount_nonnegative CHECK (amount_usd_micros >= 0) NOT VALID;
ALTER TABLE ai_provider_usage_evidence ADD CONSTRAINT ai_usage_units_nonnegative CHECK (usage_units IS NULL OR usage_units >= 0) NOT VALID;
ALTER TABLE ai_provider_usage_evidence ADD CONSTRAINT ai_token_units_nonnegative CHECK ((input_tokens IS NULL OR input_tokens >= 0) AND (output_tokens IS NULL OR output_tokens >= 0)) NOT VALID;
CREATE INDEX IF NOT EXISTS ai_credit_reservations_usd_user_idx ON ai_credit_reservations (user_id, currency, created_at DESC);
CREATE INDEX IF NOT EXISTS ai_credit_events_usd_reservation_idx ON ai_credit_reservation_events (reservation_id, currency, created_at DESC);