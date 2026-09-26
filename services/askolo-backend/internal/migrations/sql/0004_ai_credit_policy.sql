-- Versioned P07 policy snapshots. Defaults are intentionally visible and temporary.
CREATE TABLE ai_credit_policy_versions (
  version integer PRIMARY KEY,
  operation_weights jsonb NOT NULL DEFAULT '{"voice":1}'::jsonb,
  monthly_grant_credits integer NOT NULL DEFAULT 0,
  rollover_cap_credits integer NOT NULL DEFAULT 0,
  rollover_expiry_days integer NOT NULL DEFAULT 30,
  overrun_margin_percent integer NOT NULL DEFAULT 0,
  created_by varchar NOT NULL,
  change_reason varchar(160) NOT NULL DEFAULT 'initial',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ai_credit_policy_monthly_grant_nonnegative CHECK (monthly_grant_credits >= 0),
  CONSTRAINT ai_credit_policy_rollover_cap_nonnegative CHECK (rollover_cap_credits >= 0),
  CONSTRAINT ai_credit_policy_rollover_expiry_positive CHECK (rollover_expiry_days > 0),
  CONSTRAINT ai_credit_policy_overrun_margin_range CHECK (overrun_margin_percent BETWEEN 0 AND 100)
);
INSERT INTO ai_credit_policy_versions (version, created_by)
VALUES (1, 'system-default')
ON CONFLICT (version) DO NOTHING;

ALTER TABLE ai_credit_adjustments
  ADD COLUMN actor_user_id varchar,
  ADD COLUMN reversal_of_id integer,
  ADD COLUMN reversal_of_grant_id integer,
  ADD CONSTRAINT ai_credit_adjustments_actor_fk FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT,
  ADD CONSTRAINT ai_credit_adjustments_reversal_fk FOREIGN KEY (reversal_of_id) REFERENCES ai_credit_adjustments(id) ON DELETE RESTRICT,
  ADD CONSTRAINT ai_credit_adjustments_grant_reversal_fk FOREIGN KEY (reversal_of_grant_id) REFERENCES ai_credit_grants(id) ON DELETE RESTRICT;
ALTER TABLE ai_credit_grants
  ADD COLUMN actor_user_id varchar,
  ADD COLUMN reason varchar(160),
  ADD CONSTRAINT ai_credit_grants_actor_fk FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE ai_credit_reservations
  ADD COLUMN policy_version integer NOT NULL DEFAULT 1,
  ADD CONSTRAINT ai_credit_reservations_policy_version_fk FOREIGN KEY (policy_version) REFERENCES ai_credit_policy_versions(version) ON DELETE RESTRICT;
CREATE UNIQUE INDEX ai_credit_adjustments_reversal_idx ON ai_credit_adjustments (reversal_of_id) WHERE reversal_of_id IS NOT NULL;
CREATE UNIQUE INDEX ai_credit_adjustments_grant_reversal_idx ON ai_credit_adjustments (reversal_of_grant_id) WHERE reversal_of_grant_id IS NOT NULL;
CREATE INDEX ai_credit_policy_versions_created_idx ON ai_credit_policy_versions (created_at DESC);