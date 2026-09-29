package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const USDMicroUnitsPerDollar int64 = 1_000_000

type USDRateCard struct {
	Provider                  string `json:"provider"`
	Mode                      string `json:"mode"`
	Model                     string `json:"model"`
	Meter                     string `json:"meter"`
	UsdMicrosPerHour          int64  `json:"usdMicrosPerHour,omitempty"`
	InputUsdMicrosPerMillion  int64  `json:"inputUsdMicrosPerMillion,omitempty"`
	OutputUsdMicrosPerMillion int64  `json:"outputUsdMicrosPerMillion,omitempty"`
}
type USDPolicy struct {
	Version               int                    `json:"version"`
	RateCards             map[string]USDRateCard `json:"rateCards"`
	MonthlyGrantUsdMicros int64                  `json:"monthlyGrantUsdMicros"`
	RolloverCapUsdMicros  int64                  `json:"rolloverCapUsdMicros"`
	RolloverExpiryDays    int                    `json:"rolloverExpiryDays"`
	OverrunMarginPercent  int                    `json:"overrunMarginPercent"`
	CreatedBy             string                 `json:"createdBy"`
	ChangeReason          string                 `json:"changeReason"`
	CreatedAt             time.Time              `json:"createdAt"`
}
type USDUsage struct {
	BalanceUsdMicros     int64 `json:"balanceUsdMicros"`
	GrantedUsdMicros     int64 `json:"grantedUsdMicros"`
	AdjustmentsUsdMicros int64 `json:"adjustmentsUsdMicros"`
	ReservedUsdMicros    int64 `json:"reservedUsdMicros"`
	SpentUsdMicros       int64 `json:"spentUsdMicros"`
	RefundedUsdMicros    int64 `json:"refundedUsdMicros"`
}
type USDReservation struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	ReservedUsdMicros int64      `json:"reservedUsdMicros"`
	SettledUsdMicros  int64      `json:"settledUsdMicros"`
	RefundedUsdMicros int64      `json:"refundedUsdMicros"`
	PolicyVersion     int        `json:"policyVersion"`
	ExpiresAt         *time.Time `json:"expiresAt"`
	Provider          string     `json:"provider"`
	Mode              string     `json:"mode"`
	Model             string     `json:"model"`
}
type USDEvidence struct {
	UsageUnit         string          `json:"usageUnit"`
	UsageUnits        int64           `json:"usageUnits"`
	DurationMs        int64           `json:"durationMs"`
	InputTokens       int64           `json:"inputTokens"`
	OutputTokens      int64           `json:"outputTokens"`
	ProviderRequestId string          `json:"providerRequestId"`
	Payload           json.RawMessage `json:"payload"`
}

var ErrUSDPolicyChanged = errors.New("USD policy version conflict")
var ErrUSDReservationConflict = errors.New("USD reservation idempotency conflict")

func equalJSON(a, b []byte) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func validateRateCard(c USDRateCard) error {
	if strings.TrimSpace(c.Provider) == "" || strings.TrimSpace(c.Mode) == "" || strings.TrimSpace(c.Model) == "" {
		return errors.New("invalid USD rate card identity")
	}
	if c.Meter != "hour" && c.Meter != "input_tokens" && c.Meter != "output_tokens" && c.Meter != "tokens" {
		return errors.New("unknown USD meter")
	}
	for _, n := range []int64{c.UsdMicrosPerHour, c.InputUsdMicrosPerMillion, c.OutputUsdMicrosPerMillion} {
		if n < 0 || n > math.MaxInt64/2 {
			return errors.New("USD rate overflow")
		}
	}
	if c.Meter == "hour" && c.UsdMicrosPerHour <= 0 {
		return errors.New("hourly USD rate is required")
	}
	if (c.Meter == "input_tokens" && c.InputUsdMicrosPerMillion <= 0) || (c.Meter == "output_tokens" && c.OutputUsdMicrosPerMillion <= 0) || (c.Meter == "tokens" && (c.InputUsdMicrosPerMillion <= 0 || c.OutputUsdMicrosPerMillion <= 0)) {
		return errors.New("token USD rate is required")
	}
	return nil
}
func validateUSDPolicy(p USDPolicy) error {
	if p.Version < 1 || len(strings.TrimSpace(p.ChangeReason)) < 1 || len(p.ChangeReason) > 160 || p.MonthlyGrantUsdMicros < 0 || p.RolloverCapUsdMicros < 0 || p.RolloverExpiryDays < 1 || p.OverrunMarginPercent < 0 || p.OverrunMarginPercent > 100 || len(p.RateCards) > 64 {
		return errors.New("invalid USD policy")
	}
	for key, card := range p.RateCards {
		if len(key) == 0 || len(key) > 160 {
			return errors.New("invalid USD rate card key")
		}
		if err := validateRateCard(card); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) USDPolicy(ctx context.Context) (USDPolicy, error) {
	var p USDPolicy
	if s == nil || s.pool == nil {
		return p, errors.New("database is not configured")
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT version,rate_cards,monthly_grant_usd_micros,rollover_cap_usd_micros,rollover_expiry_days,overrun_margin_percent,created_by,change_reason,created_at FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1`).Scan(&p.Version, &raw, &p.MonthlyGrantUsdMicros, &p.RolloverCapUsdMicros, &p.RolloverExpiryDays, &p.OverrunMarginPercent, &p.CreatedBy, &p.ChangeReason, &p.CreatedAt)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(raw, &p.RateCards); err != nil {
		return p, err
	}
	return p, nil
}
func (s *Store) UpdateUSDPolicy(ctx context.Context, expected int, p USDPolicy, actor string) (USDPolicy, error) {
	if s == nil || s.pool == nil {
		return USDPolicy{}, errors.New("database is not configured")
	}
	if strings.TrimSpace(actor) == "" {
		return USDPolicy{}, errors.New("policy actor is required")
	}
	if err := validateUSDPolicy(p); err != nil || p.Version != expected+1 {
		if err != nil {
			return USDPolicy{}, err
		}
		return USDPolicy{}, ErrUSDPolicyChanged
	}
	raw, err := json.Marshal(p.RateCards)
	if err != nil {
		return USDPolicy{}, err
	}
	var created time.Time
	err = s.pool.QueryRow(ctx, `INSERT INTO ai_credit_policy_versions(version,rate_cards,monthly_grant_usd_micros,rollover_cap_usd_micros,rollover_expiry_days,overrun_margin_percent,created_by,change_reason,operation_weights,monthly_grant_credits,rollover_cap_credits) SELECT $1,$2,$3,$4,$5,$6,$7,$8,operation_weights,monthly_grant_credits,rollover_cap_credits FROM ai_credit_policy_versions WHERE version=$9 RETURNING created_at`, p.Version, raw, p.MonthlyGrantUsdMicros, p.RolloverCapUsdMicros, p.RolloverExpiryDays, p.OverrunMarginPercent, actor, p.ChangeReason, expected).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return USDPolicy{}, ErrUSDPolicyChanged
	}
	if err != nil {
		return USDPolicy{}, err
	}
	p.CreatedBy, p.CreatedAt = actor, created
	return p, nil
}

func (s *Store) USDUsage(ctx context.Context, userID string) (USDUsage, error) {
	var u USDUsage
	if s == nil || s.pool == nil {
		return u, errors.New("database is not configured")
	}
	err := s.pool.QueryRow(ctx, `SELECT granted_usd_micros,adjustment_usd_micros,reserved_usd_micros,spent_usd_micros,refunded_usd_micros,granted_usd_micros+adjustment_usd_micros-reserved_usd_micros-spent_usd_micros+refunded_usd_micros FROM ai_credit_accounts WHERE user_id=$1`, userID).Scan(&u.GrantedUsdMicros, &u.AdjustmentsUsdMicros, &u.ReservedUsdMicros, &u.SpentUsdMicros, &u.RefundedUsdMicros, &u.BalanceUsdMicros)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil
	}
	return u, err
}

func (s *Store) ReserveUSD(ctx context.Context, id, userID, operation, provider, mode, model, key string, amount, ttl int64, policyVersion int, snapshot json.RawMessage) (USDReservation, bool, error) {
	var r USDReservation
	if s == nil || s.pool == nil {
		return r, false, errors.New("database is not configured")
	}
	if id == "" || userID == "" || operation == "" || provider == "" || mode == "" || key == "" || len(key) > 200 || amount <= 0 || amount > math.MaxInt64/2 || ttl < 1 || ttl > 3600 || policyVersion < 1 || len(snapshot) > 16*1024 {
		return r, false, errors.New("invalid USD reservation")
	}
	if len(snapshot) == 0 {
		snapshot = json.RawMessage(`{}`)
	}
	var normalized any
	if json.Unmarshal(snapshot, &normalized) != nil {
		return r, false, errors.New("invalid USD rate snapshot")
	}
	snapshot, _ = json.Marshal(normalized)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return r, false, err
	}
	defer tx.Rollback(ctx)
	var prior USDReservation
	var priorOperation string
	var priorAmount int64
	var priorSnapshot []byte
	err = tx.QueryRow(ctx, `SELECT id,operation_type,status,reserved_usd_micros,settled_usd_micros,refunded_usd_micros,policy_version,expires_at,provider,mode,COALESCE(model,''),rate_snapshot FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' AND idempotency_key=$2`, userID, key).Scan(&prior.ID, &priorOperation, &prior.Status, &prior.ReservedUsdMicros, &prior.SettledUsdMicros, &prior.RefundedUsdMicros, &prior.PolicyVersion, &prior.ExpiresAt, &prior.Provider, &prior.Mode, &prior.Model, &priorSnapshot)
	if err == nil {
		priorAmount = prior.ReservedUsdMicros
		if priorOperation != operation || prior.Provider != provider || prior.Mode != mode || prior.Model != model || prior.PolicyVersion != policyVersion || priorAmount != amount || !equalJSON(priorSnapshot, snapshot) {
			return r, false, ErrUSDReservationConflict
		}
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' AND status='reserved' AND expires_at<=NOW() FOR UPDATE`, userID); err != nil {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `WITH expired AS (UPDATE ai_credit_reservations SET status='expired',closed_at=NOW(),updated_at=NOW() WHERE user_id=$1 AND currency='USD' AND status='reserved' AND expires_at<=NOW() RETURNING id,user_id,reserved_usd_micros), total AS (SELECT COALESCE(sum(reserved_usd_micros),0) amount FROM expired) UPDATE ai_credit_accounts SET reserved_usd_micros=reserved_usd_micros-(SELECT amount FROM total),updated_at=NOW() WHERE user_id=$1`, userID); err != nil {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key) SELECT id,user_id,'expired','USD',reserved_usd_micros,id||':expired' FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' AND status='expired' AND closed_at >= NOW()-INTERVAL '1 second' ON CONFLICT DO NOTHING`, userID); err != nil {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return r, false, err
	}
	var balance int64
	if err = tx.QueryRow(ctx, `SELECT granted_usd_micros+adjustment_usd_micros-reserved_usd_micros-spent_usd_micros+refunded_usd_micros FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return r, false, err
	}
	// Recheck idempotency only after the account lock, preserving serialized
	// retries and allowing a parameter mismatch to fail deterministically.
	err = tx.QueryRow(ctx, `SELECT id,operation_type,status,reserved_usd_micros,settled_usd_micros,refunded_usd_micros,policy_version,expires_at,provider,mode,COALESCE(model,''),rate_snapshot FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' AND idempotency_key=$2`, userID, key).Scan(&prior.ID, &priorOperation, &prior.Status, &prior.ReservedUsdMicros, &prior.SettledUsdMicros, &prior.RefundedUsdMicros, &prior.PolicyVersion, &prior.ExpiresAt, &prior.Provider, &prior.Mode, &prior.Model, &priorSnapshot)
	if err == nil {
		if priorOperation != operation || prior.Provider != provider || prior.Mode != mode || prior.Model != model || prior.PolicyVersion != policyVersion || prior.ReservedUsdMicros != amount || !equalJSON(priorSnapshot, snapshot) {
			return r, false, ErrUSDReservationConflict
		}
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, false, err
	}
	if balance < amount {
		return r, false, nil
	}
	expires := time.Now().UTC().Add(time.Duration(ttl) * time.Second)
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_usd_micros=reserved_usd_micros+$2,updated_at=NOW() WHERE user_id=$1`, userID, amount); err != nil {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservations(id,user_id,operation_type,provider,mode,model,status,idempotency_key,reserved_credits,settled_credits,refunded_credits,unit_rate,currency,reserved_usd_micros,max_usd_micros,rate_snapshot,expires_at,policy_version) VALUES($1,$2,$3,$4,$5,$6,'reserved',$7,0,0,0,1,'USD',$8,$8,$9,$10,$11)`, id, userID, operation, provider, mode, model, key, amount, snapshot, expires, policyVersion); err != nil {
		return r, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key) VALUES($1,$2,'reserved','USD',$3,$4)`, id, userID, amount, key+":reserved"); err != nil {
		return r, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return r, false, err
	}
	return USDReservation{ID: id, Status: "reserved", ReservedUsdMicros: amount, PolicyVersion: policyVersion, ExpiresAt: &expires, Provider: provider, Mode: mode, Model: model}, true, nil
}

func (s *Store) ClaimUSDReservation(ctx context.Context, id, userID string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE ai_credit_reservations SET status='claimed',started_at=NOW(),updated_at=NOW() WHERE id=$1 AND user_id=$2 AND currency='USD' AND status='reserved' AND (expires_at IS NULL OR expires_at>NOW())`, id, userID)
	return tag.RowsAffected() == 1, err
}

func (s *Store) SettleUSDReservation(ctx context.Context, id, userID, key string, actual int64) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if actual < 0 {
		return errors.New("invalid USD settlement")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var reserved int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT reserved_usd_micros,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "settled" {
		var prior int64
		err = tx.QueryRow(ctx, `SELECT amount_usd_micros FROM ai_credit_reservation_events WHERE reservation_id=$1 AND currency='USD' AND event_type='settled' AND idempotency_key=$2`, id, key).Scan(&prior)
		if err == nil && prior == actual {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if state != "claimed" || actual > reserved {
		return errors.New("invalid USD reservation state")
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' AND status='reserved' AND expires_at<=NOW() FOR UPDATE`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_usd_micros=reserved_usd_micros-$2,spent_usd_micros=spent_usd_micros+$3,updated_at=NOW() WHERE user_id=$1 AND reserved_usd_micros>=$2`, userID, reserved, actual); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_reservations SET status='settled',settled_usd_micros=$2,refunded_usd_micros=reserved_usd_micros-$2,closed_at=NOW(),updated_at=NOW() WHERE id=$1 AND currency='USD'`, id, actual); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key) VALUES($1,$2,'settled','USD',$3,$4)`, id, userID, actual, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReleaseUSDReservation(ctx context.Context, id, userID, key string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var reserved int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT reserved_usd_micros,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "released" {
		var prior int64
		err = tx.QueryRow(ctx, `SELECT amount_usd_micros FROM ai_credit_reservation_events WHERE reservation_id=$1 AND currency='USD' AND event_type='released' AND idempotency_key=$2`, id, key).Scan(&prior)
		if err == nil && prior == reserved {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if state != "claimed" && state != "reserved" {
		return errors.New("invalid USD reservation state")
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_usd_micros=reserved_usd_micros-$2,updated_at=NOW() WHERE user_id=$1 AND reserved_usd_micros>=$2`, userID, reserved); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_reservations SET status='released',refunded_usd_micros=reserved_usd_micros,closed_at=NOW(),updated_at=NOW() WHERE id=$1 AND currency='USD'`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key) VALUES($1,$2,'released','USD',$3,$4)`, id, userID, reserved, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AddUSDAdjustment(ctx context.Context, target, actor string, amount int64, reason, key string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if target == "" || actor == "" || amount == 0 || reason == "" || len(reason) > 160 || key == "" || amount == math.MinInt64 {
		return errors.New("invalid USD adjustment")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, target); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason})
	var priorAmount int64
	var priorActor, priorReason string
	err = tx.QueryRow(ctx, `SELECT amount_usd_micros,COALESCE(actor_user_id,''),reason FROM ai_credit_adjustments WHERE user_id=$1 AND currency='USD' AND idempotency_key=$2`, target, key).Scan(&priorAmount, &priorActor, &priorReason)
	if err == nil {
		if priorAmount == amount && priorActor == actor && priorReason == reason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT amount_usd_micros,COALESCE(actor_user_id,''),COALESCE(reason,'') FROM ai_credit_grants WHERE user_id=$1 AND currency='USD' AND idempotency_key=$2`, target, key).Scan(&priorAmount, &priorActor, &priorReason)
	if err == nil {
		if priorAmount == amount && priorActor == actor && priorReason == reason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var balance int64
	if err = tx.QueryRow(ctx, `SELECT granted_usd_micros+adjustment_usd_micros-reserved_usd_micros-spent_usd_micros+refunded_usd_micros FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, target).Scan(&balance); err != nil {
		return err
	}
	if amount < 0 && balance+amount < 0 {
		return errors.New("insufficient USD balance")
	}
	if amount > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_grants(user_id,source_type,amount_credits,entitlement_key,idempotency_key,metadata,actor_user_id,reason,currency,amount_usd_micros) VALUES($1,'admin',1,$2,$2,$3,$4,$5,'USD',$6)`, target, key, meta, actor, reason, amount); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET granted_usd_micros=granted_usd_micros+$2,updated_at=NOW() WHERE user_id=$1`, target, amount); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments(user_id,amount_credits,reason,idempotency_key,metadata,actor_user_id,currency,amount_usd_micros) VALUES($1,-1,$2,$3,$4,$5,'USD',$6)`, target, reason, key, meta, actor, amount); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET adjustment_usd_micros=adjustment_usd_micros+$2,updated_at=NOW() WHERE user_id=$1`, target, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RefundUSDReservation(ctx context.Context, id, userID, actor, key, reason string, amount int64) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if amount <= 0 || actor == "" || key == "" || reason == "" || len(reason) > 160 {
		return errors.New("invalid USD refund")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var settled, refunded int64
	var state string
	if err = tx.QueryRow(ctx, `SELECT settled_usd_micros,refunded_usd_micros,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE`, id, userID).Scan(&settled, &refunded, &state); err != nil {
		return err
	}
	if state != "settled" || amount > settled-refunded {
		return errors.New("refund exceeds settled USD")
	}
	var prior int64
	var priorDetails []byte
	err = tx.QueryRow(ctx, `SELECT amount_usd_micros,details FROM ai_credit_reservation_events WHERE reservation_id=$1 AND currency='USD' AND event_type='refunded' AND idempotency_key=$2`, id, key).Scan(&prior, &priorDetails)
	if err == nil {
		var details map[string]string
		_ = json.Unmarshal(priorDetails, &details)
		if prior == amount && details["actorUserId"] == actor && details["reason"] == reason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE ai_credit_reservations SET refunded_usd_micros=refunded_usd_micros+$2,updated_at=NOW() WHERE id=$1 AND currency='USD' AND refunded_usd_micros+$2<=settled_usd_micros`, id, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("refund reservation update conflict")
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET refunded_usd_micros=refunded_usd_micros+$2,updated_at=NOW() WHERE user_id=$1`, userID, amount); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key,details) VALUES($1,$2,'refunded','USD',$3,$4,$5)`, id, userID, amount, key, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReverseUSDAdjustment(ctx context.Context, target, actor string, entryID int64, key, reason string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if entryID < 1 || target == "" || actor == "" || key == "" || reason == "" || len(reason) > 160 {
		return errors.New("invalid USD reversal")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var amount int64
	if err = tx.QueryRow(ctx, `SELECT amount_usd_micros FROM ai_credit_adjustments WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE`, entryID, target).Scan(&amount); err != nil {
		return err
	}
	var prior int64
	var priorKey, priorActor, priorReason string
	err = tx.QueryRow(ctx, `SELECT amount_usd_micros,idempotency_key,COALESCE(actor_user_id,''),reason FROM ai_credit_adjustments WHERE reversal_of_id=$1 AND currency='USD'`, entryID).Scan(&prior, &priorKey, &priorActor, &priorReason)
	if err == nil {
		if prior == -amount && priorKey == key && priorActor == actor && priorReason == reason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason, "reversalOf": fmt.Sprint(entryID)})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments(user_id,amount_credits,reason,idempotency_key,metadata,actor_user_id,reversal_of_id,currency,amount_usd_micros) VALUES($1,1,$2,$3,$4,$5,$6,'USD',$7)`, target, reason, key, meta, actor, entryID, -amount); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE ai_credit_accounts SET adjustment_usd_micros=adjustment_usd_micros-$2,updated_at=NOW() WHERE user_id=$1 AND adjustment_usd_micros-$2+granted_usd_micros-reserved_usd_micros-spent_usd_micros+refunded_usd_micros>=0`, target, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("USD reversal balance conflict")
	}
	return tx.Commit(ctx)
}

func (s *Store) ReverseUSDGrant(ctx context.Context, target, actor string, grantID int64, key, reason string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if target == "" || actor == "" || grantID < 1 || key == "" || reason == "" || len(reason) > 160 {
		return errors.New("invalid USD grant reversal")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var amount int64
	if err = tx.QueryRow(ctx, `SELECT amount_usd_micros FROM ai_credit_grants WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE`, grantID, target).Scan(&amount); err != nil {
		return err
	}
	var prior int64
	var priorKey, priorActor, priorReason string
	err = tx.QueryRow(ctx, `SELECT amount_usd_micros,idempotency_key,COALESCE(actor_user_id,''),reason FROM ai_credit_adjustments WHERE reversal_of_grant_id=$1 AND currency='USD'`, grantID).Scan(&prior, &priorKey, &priorActor, &priorReason)
	if err == nil {
		if prior == -amount && priorKey == key && priorActor == actor && priorReason == reason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var available int64
	if err = tx.QueryRow(ctx, `SELECT granted_usd_micros+adjustment_usd_micros-reserved_usd_micros-spent_usd_micros+refunded_usd_micros FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, target).Scan(&available); err != nil {
		return err
	}
	if available < amount {
		return errors.New("insufficient USD balance")
	}
	meta, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason, "reversalOfGrant": fmt.Sprint(grantID)})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments(user_id,amount_credits,reason,idempotency_key,metadata,actor_user_id,reversal_of_grant_id,currency,amount_usd_micros) VALUES($1,-1,$2,$3,$4,$5,$6,'USD',$7)`, target, reason, key, meta, actor, grantID, -amount); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE ai_credit_accounts SET granted_usd_micros=granted_usd_micros-$2,updated_at=NOW() WHERE user_id=$1 AND granted_usd_micros>=$2`, target, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("USD grant reversal balance conflict")
	}
	return tx.Commit(ctx)
}

func (s *Store) USDRecent(ctx context.Context, userID string, limit int) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `SELECT id,operation_type,provider,mode,COALESCE(model,''),status,reserved_usd_micros,settled_usd_micros,refunded_usd_micros,policy_version,expires_at,created_at FROM ai_credit_reservations WHERE user_id=$1 AND currency='USD' ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := make([]USDReservation, 0)
	for rows.Next() {
		var r USDReservation
		var operation string
		var created time.Time
		if err := rows.Scan(&r.ID, &operation, &r.Provider, &r.Mode, &r.Model, &r.Status, &r.ReservedUsdMicros, &r.SettledUsdMicros, &r.RefundedUsdMicros, &r.PolicyVersion, &r.ExpiresAt, &created); err != nil {
			return nil, err
		}
		reservations = append(reservations, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	events := make([]map[string]any, 0)
	erows, err := s.pool.Query(ctx, `SELECT id,reservation_id,event_type,amount_usd_micros,duration_ms,details,created_at FROM ai_credit_reservation_events WHERE user_id=$1 AND currency='USD' ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var id int64
		var rid, typ string
		var amount int64
		var duration *int
		var details []byte
		var created time.Time
		if err := erows.Scan(&id, &rid, &typ, &amount, &duration, &details, &created); err != nil {
			return nil, err
		}
		events = append(events, map[string]any{"id": id, "reservationId": rid, "eventType": typ, "amountUsdMicros": amount, "durationMs": duration, "details": json.RawMessage(details), "createdAt": created})
	}
	if err := erows.Err(); err != nil {
		return nil, err
	}
	adjustments := make([]map[string]any, 0)
	arows, err := s.pool.Query(ctx, `SELECT id,amount_usd_micros,reason,COALESCE(actor_user_id,''),reversal_of_id,created_at FROM ai_credit_adjustments WHERE user_id=$1 AND currency='USD' ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var id int64
		var amount int64
		var reason, actor string
		var reversal *int64
		var created time.Time
		if err := arows.Scan(&id, &amount, &reason, &actor, &reversal, &created); err != nil {
			return nil, err
		}
		adjustments = append(adjustments, map[string]any{"id": id, "amountUsdMicros": amount, "reason": reason, "actorUserId": actor, "reversalOfId": reversal, "createdAt": created})
	}
	if err := arows.Err(); err != nil {
		return nil, err
	}
	grants := make([]map[string]any, 0)
	grows, err := s.pool.Query(ctx, `SELECT id,amount_usd_micros,reason,COALESCE(actor_user_id,''),created_at FROM ai_credit_grants WHERE user_id=$1 AND currency='USD' ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer grows.Close()
	for grows.Next() {
		var id int64
		var amount int64
		var reason, actor string
		var created time.Time
		if err := grows.Scan(&id, &amount, &reason, &actor, &created); err != nil {
			return nil, err
		}
		grants = append(grants, map[string]any{"id": id, "amountUsdMicros": amount, "reason": reason, "actorUserId": actor, "createdAt": created})
	}
	return map[string]any{"reservations": reservations, "events": events, "adjustments": adjustments, "grants": grants}, grows.Err()
}

// RecordUSDEvidence stores provider metering metadata only. Payload must not
// contain transcripts, prompts, audio, or other user content.
func (s *Store) RecordUSDEvidence(ctx context.Context, reservationID, userID, provider, mode, model, key string, evidence USDEvidence) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if reservationID == "" || userID == "" || provider == "" || mode == "" || key == "" ||
		len(key) > 240 || len(evidence.ProviderRequestId) > 200 || len(evidence.Payload) > 8192 ||
		evidence.UsageUnits < 0 || evidence.DurationMs < 0 || evidence.InputTokens < 0 || evidence.OutputTokens < 0 ||
		(evidence.UsageUnit != "hour" && evidence.UsageUnit != "second" && evidence.UsageUnit != "millisecond" && evidence.UsageUnit != "input_tokens" && evidence.UsageUnit != "output_tokens" && evidence.UsageUnit != "tokens") {
		return errors.New("invalid USD usage evidence")
	}
	payload := evidence.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return errors.New("usage payload must be a JSON object")
	}
	for field := range fields {
		if field != "providerRequestId" && field != "usageUnit" && field != "usageUnits" && field != "durationMs" && field != "inputTokens" && field != "outputTokens" {
			return errors.New("usage payload contains non-metering data")
		}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO ai_provider_usage_evidence(
		reservation_id,user_id,provider,mode,model,usage_unit,usage_units,duration_ms,
		input_tokens,output_tokens,provider_request_id,idempotency_key,evidence,payload
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
	ON CONFLICT (reservation_id,idempotency_key) DO NOTHING`,
		reservationID, userID, provider, mode, model, evidence.UsageUnit,
		evidence.UsageUnits, evidence.DurationMs, evidence.InputTokens, evidence.OutputTokens,
		evidence.ProviderRequestId, key, payload)
	return err
}
