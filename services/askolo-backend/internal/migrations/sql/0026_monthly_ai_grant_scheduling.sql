CREATE TABLE ai_credit_monthly_grant_schedules (
  user_id varchar PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  next_cycle_at timestamptz NOT NULL,
  retry_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_credit_monthly_grant_schedules_due_idx
  ON ai_credit_monthly_grant_schedules (COALESCE(retry_at, next_cycle_at), user_id);

CREATE TABLE ai_credit_monthly_grant_cycles (
  user_id varchar NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scheduled_at timestamptz NOT NULL,
  policy_version integer NOT NULL REFERENCES ai_credit_policy_versions(version) ON DELETE RESTRICT,
  status varchar(24) NOT NULL CHECK (status IN ('issued', 'skipped_disabled', 'skipped_zero_amount', 'deferred_cap')),
  amount_usd_micros bigint NOT NULL DEFAULT 0 CHECK (amount_usd_micros >= 0),
  grant_id integer REFERENCES ai_credit_grants(id) ON DELETE RESTRICT,
  retry_count integer NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, scheduled_at)
);

CREATE TABLE ai_credit_user_policy_overrides (
  user_id varchar PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  monthly_grants_enabled boolean,
  monthly_grant_usd_micros bigint CHECK (monthly_grant_usd_micros IS NULL OR monthly_grant_usd_micros >= 0),
  rollover_cap_usd_micros bigint CHECK (rollover_cap_usd_micros IS NULL OR rollover_cap_usd_micros >= 0),
  updated_by varchar NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  reason varchar(160) NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ai_credit_user_policy_override_events (
  id bigserial PRIMARY KEY,
  user_id varchar NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  actor_user_id varchar NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  reason varchar(160) NOT NULL,
  previous_values jsonb NOT NULL DEFAULT '{}'::jsonb,
  next_values jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_credit_user_policy_override_events_user_idx
  ON ai_credit_user_policy_override_events (user_id, created_at DESC);

CREATE TABLE ai_credit_admin_alerts (
  alert_key varchar(240) PRIMARY KEY,
  alert_type varchar(40) NOT NULL,
  user_id varchar REFERENCES users(id) ON DELETE SET NULL,
  details jsonb NOT NULL DEFAULT '{}'::jsonb,
  acknowledged_at timestamptz,
  acknowledged_by varchar REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_credit_admin_alerts_unacknowledged_idx
  ON ai_credit_admin_alerts (created_at DESC)
  WHERE acknowledged_at IS NULL;
