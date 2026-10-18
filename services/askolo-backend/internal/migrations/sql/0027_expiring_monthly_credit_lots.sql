-- Recurring monthly grants issued after this migration expire independently.
-- Existing grants remain non-expiring; NULL expires_at is the legacy pool.
ALTER TABLE ai_credit_grants
  ADD COLUMN expires_at timestamptz;

CREATE INDEX ai_credit_grants_active_expiry_idx
  ON ai_credit_grants (user_id, expires_at, id)
  WHERE expires_at IS NOT NULL;

CREATE TABLE ai_credit_grant_allocations (
  reservation_id varchar NOT NULL REFERENCES ai_credit_reservations(id) ON DELETE CASCADE
    DEFERRABLE INITIALLY DEFERRED,
  grant_id integer NOT NULL REFERENCES ai_credit_grants(id) ON DELETE RESTRICT,
  reserved_usd_micros bigint NOT NULL CHECK (reserved_usd_micros > 0),
  settled_usd_micros bigint NOT NULL DEFAULT 0
    CHECK (settled_usd_micros >= 0 AND settled_usd_micros <= reserved_usd_micros),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (reservation_id, grant_id)
);

CREATE INDEX ai_credit_grant_allocations_grant_idx
  ON ai_credit_grant_allocations (grant_id);

CREATE FUNCTION ai_credit_unexpired_balance(p_user_id varchar)
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
  SELECT a.granted_usd_micros+a.adjustment_usd_micros-a.reserved_usd_micros-
         a.spent_usd_micros+a.refunded_usd_micros-
         COALESCE((
           SELECT SUM(g.amount_usd_micros-COALESCE(x.used_usd_micros,0))
           FROM ai_credit_grants g
           LEFT JOIN LATERAL (
             SELECT SUM(al.settled_usd_micros + CASE WHEN r.status IN ('reserved','claimed')
                         THEN al.reserved_usd_micros-al.settled_usd_micros ELSE 0 END) AS used_usd_micros
             FROM ai_credit_grant_allocations al
             JOIN ai_credit_reservations r ON r.id=al.reservation_id
             WHERE al.grant_id=g.id
           ) x ON true
           WHERE g.user_id=a.user_id AND g.currency='USD' AND g.expires_at<=NOW()
         ),0)
  FROM ai_credit_accounts a WHERE a.user_id=p_user_id
$$;
