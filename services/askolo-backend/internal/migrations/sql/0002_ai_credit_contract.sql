-- Non-destructive AI ledger contract validation and constraints.
DO $$
DECLARE
  t text;
  c text;
BEGIN
  IF to_regclass('users') IS NULL THEN RAISE EXCEPTION 'AI ledger preflight: users table is missing'; END IF;
  FOREACH t IN ARRAY ARRAY['ai_credit_accounts','ai_credit_grants','ai_credit_adjustments','ai_credit_reservations','ai_credit_reservation_events','ai_provider_usage_evidence'] LOOP
    IF to_regclass(t) IS NULL THEN RAISE EXCEPTION 'AI ledger preflight: missing table %', t; END IF;
  END LOOP;
  IF NOT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('ai_credit_accounts') AND attname='user_id' AND atttypid='varchar'::regtype AND attnotnull)
    OR NOT EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conrelid=to_regclass('ai_credit_accounts') AND c.contype='p'
        AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass('ai_credit_accounts') AND attname='user_id')]::smallint[]
    )
    OR NOT EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conrelid=to_regclass('ai_credit_accounts') AND c.contype='f'
        AND c.confrelid=to_regclass('users')
        AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass('ai_credit_accounts') AND attname='user_id')]::smallint[]
        AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass('users') AND attname='id')]::smallint[]
        AND c.confdeltype='c'
    ) THEN
    RAISE EXCEPTION 'AI ledger preflight: account user_id PK/FK/type contract is missing';
  END IF;
  FOREACH t IN ARRAY ARRAY['ai_credit_grants','ai_credit_adjustments','ai_credit_reservations','ai_credit_reservation_events','ai_provider_usage_evidence'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conrelid=to_regclass(t) AND c.contype='f'
        AND c.confrelid=to_regclass('users')
        AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(t) AND attname='user_id')]::smallint[]
        AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass('users') AND attname='id')]::smallint[]
        AND c.confdeltype='c'
    ) THEN
      RAISE EXCEPTION 'AI ledger preflight: % user_id must reference users(id) with CASCADE', t;
    END IF;
  END LOOP;
  FOREACH t IN ARRAY ARRAY['ai_credit_reservation_events','ai_provider_usage_evidence'] LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conrelid=to_regclass(t) AND c.contype='f'
        AND c.confrelid=to_regclass('ai_credit_reservations')
        AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(t) AND attname='reservation_id')]::smallint[]
        AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass('ai_credit_reservations') AND attname='id')]::smallint[]
        AND c.confdeltype='c'
    ) THEN
      RAISE EXCEPTION 'AI ledger preflight: % reservation_id must reference ai_credit_reservations(id) with CASCADE', t;
    END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['granted_credits','adjustment_credits','reserved_credits','spent_credits','refunded_credits'] LOOP
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_accounts' AND column_name=c)
      AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_accounts' AND column_name=c AND data_type='integer' AND is_nullable='NO') THEN
      RAISE EXCEPTION 'AI ledger preflight: existing account counter % has incompatible type/nullability', c;
    END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['id','user_id','amount_credits','reason','idempotency_key','metadata','created_at'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_adjustments' AND column_name=c) THEN RAISE EXCEPTION 'AI ledger preflight: adjustment column % missing', c; END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['id','user_id','source_type','amount_credits','entitlement_key','idempotency_key','metadata','created_at'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_grants' AND column_name=c) THEN RAISE EXCEPTION 'AI ledger preflight: grant column % missing', c; END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['id','user_id','operation_type','provider','mode','model','status','idempotency_key','request_fingerprint','reserved_credits','settled_credits','refunded_credits','max_credits','unit_rate','duration_seconds','connected_duration_ms','expires_at','started_at','closed_at','provider_token_hash','provider_token_used_at','metadata','created_at','updated_at'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_reservations' AND column_name=c) THEN RAISE EXCEPTION 'AI ledger preflight: reservation column % missing', c; END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['id','reservation_id','user_id','event_type','credits','duration_ms','idempotency_key','details','created_at'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_reservation_events' AND column_name=c) THEN RAISE EXCEPTION 'AI ledger preflight: event column % missing', c; END IF;
  END LOOP;
  FOREACH c IN ARRAY ARRAY['id','reservation_id','user_id','provider','mode','region','model','duration_ms','input_units','output_units','provider_request_id','idempotency_key','evidence','recorded_at'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_provider_usage_evidence' AND column_name=c) THEN RAISE EXCEPTION 'AI ledger preflight: evidence column % missing', c; END IF;
  END LOOP;
END $$;

DO $$
DECLARE idx record;
BEGIN
  FOR idx IN
    SELECT * FROM (VALUES
      ('ai_credit_grants_idempotency_idx','ai_credit_grants','(user_id, idempotency_key)','CREATE UNIQUE INDEX%'),
      ('ai_credit_adjustments_idempotency_idx','ai_credit_adjustments','(user_id, idempotency_key)','CREATE UNIQUE INDEX%'),
      ('ai_credit_reservations_idempotency_idx','ai_credit_reservations','(user_id, idempotency_key)','CREATE UNIQUE INDEX%'),
      ('ai_credit_reservation_events_idempotency_idx','ai_credit_reservation_events','(reservation_id, idempotency_key)','CREATE UNIQUE INDEX%'),
      ('ai_provider_usage_evidence_idempotency_idx','ai_provider_usage_evidence','(reservation_id, idempotency_key)','CREATE UNIQUE INDEX%'),
      ('ai_credit_grants_user_idx','ai_credit_grants','(user_id, created_at)','CREATE INDEX%'),
      ('ai_credit_adjustments_user_idx','ai_credit_adjustments','(user_id, created_at)','CREATE INDEX%'),
      ('ai_credit_reservations_user_status_idx','ai_credit_reservations','(user_id, status)','CREATE INDEX%'),
      ('ai_credit_reservations_expiry_idx','ai_credit_reservations','(status, expires_at)','CREATE INDEX%'),
      ('ai_credit_reservation_events_reservation_idx','ai_credit_reservation_events','(reservation_id, created_at)','CREATE INDEX%'),
      ('ai_provider_usage_evidence_reservation_idx','ai_provider_usage_evidence','(reservation_id)','CREATE INDEX%')
    ) AS required(index_name, table_name, expected_columns, expected_prefix)
  LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_indexes p
      WHERE p.schemaname=current_schema()
        AND p.tablename=idx.table_name
        AND p.indexname=idx.index_name
        AND p.indexdef LIKE idx.expected_prefix
        AND position(idx.expected_columns IN p.indexdef) > 0
    ) THEN
      RAISE EXCEPTION 'AI ledger preflight: index % is missing or has the wrong definition', idx.index_name;
    END IF;
  END LOOP;
END $$;

ALTER TABLE ai_credit_accounts
  ADD COLUMN IF NOT EXISTS granted_credits integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS adjustment_credits integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS reserved_credits integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS spent_credits integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS refunded_credits integer NOT NULL DEFAULT 0;

DO $$
DECLARE c text;
BEGIN
  FOREACH c IN ARRAY ARRAY['granted_credits','adjustment_credits','reserved_credits','spent_credits','refunded_credits'] LOOP
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_credit_accounts' AND column_name=c AND data_type='integer' AND is_nullable='NO' AND COALESCE(column_default,'') LIKE '0%') THEN
      RAISE EXCEPTION 'AI ledger preflight: account counter % is not integer NOT NULL DEFAULT 0', c;
    END IF;
  END LOOP;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_grants_amount_positive') THEN ALTER TABLE ai_credit_grants ADD CONSTRAINT ai_credit_grants_amount_positive CHECK (amount_credits > 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_adjustments_amount_nonzero') THEN ALTER TABLE ai_credit_adjustments ADD CONSTRAINT ai_credit_adjustments_amount_nonzero CHECK (amount_credits <> 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_reserved_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_reserved_nonnegative CHECK (reserved_credits >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_settled_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_settled_nonnegative CHECK (settled_credits >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_refunded_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_refunded_nonnegative CHECK (refunded_credits >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_unit_rate_positive') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_unit_rate_positive CHECK (unit_rate > 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_max_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_max_nonnegative CHECK (max_credits IS NULL OR max_credits >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_duration_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_duration_nonnegative CHECK (duration_seconds IS NULL OR duration_seconds >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservations_connected_duration_nonnegative') THEN ALTER TABLE ai_credit_reservations ADD CONSTRAINT ai_credit_reservations_connected_duration_nonnegative CHECK (connected_duration_ms IS NULL OR connected_duration_ms >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservation_events_credits_nonnegative') THEN ALTER TABLE ai_credit_reservation_events ADD CONSTRAINT ai_credit_reservation_events_credits_nonnegative CHECK (credits >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_credit_reservation_events_duration_nonnegative') THEN ALTER TABLE ai_credit_reservation_events ADD CONSTRAINT ai_credit_reservation_events_duration_nonnegative CHECK (duration_ms IS NULL OR duration_ms >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_provider_usage_evidence_duration_nonnegative') THEN ALTER TABLE ai_provider_usage_evidence ADD CONSTRAINT ai_provider_usage_evidence_duration_nonnegative CHECK (duration_ms IS NULL OR duration_ms >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_provider_usage_evidence_input_nonnegative') THEN ALTER TABLE ai_provider_usage_evidence ADD CONSTRAINT ai_provider_usage_evidence_input_nonnegative CHECK (input_units IS NULL OR input_units >= 0) NOT VALID; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='ai_provider_usage_evidence_output_nonnegative') THEN ALTER TABLE ai_provider_usage_evidence ADD CONSTRAINT ai_provider_usage_evidence_output_nonnegative CHECK (output_units IS NULL OR output_units >= 0) NOT VALID; END IF;
  ALTER TABLE ai_credit_grants VALIDATE CONSTRAINT ai_credit_grants_amount_positive;
  ALTER TABLE ai_credit_adjustments VALIDATE CONSTRAINT ai_credit_adjustments_amount_nonzero;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_reserved_nonnegative;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_settled_nonnegative;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_refunded_nonnegative;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_unit_rate_positive;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_max_nonnegative;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_duration_nonnegative;
  ALTER TABLE ai_credit_reservations VALIDATE CONSTRAINT ai_credit_reservations_connected_duration_nonnegative;
  ALTER TABLE ai_credit_reservation_events VALIDATE CONSTRAINT ai_credit_reservation_events_credits_nonnegative;
  ALTER TABLE ai_credit_reservation_events VALIDATE CONSTRAINT ai_credit_reservation_events_duration_nonnegative;
  ALTER TABLE ai_provider_usage_evidence VALIDATE CONSTRAINT ai_provider_usage_evidence_duration_nonnegative;
  ALTER TABLE ai_provider_usage_evidence VALIDATE CONSTRAINT ai_provider_usage_evidence_input_nonnegative;
  ALTER TABLE ai_provider_usage_evidence VALIDATE CONSTRAINT ai_provider_usage_evidence_output_nonnegative;
END $$;