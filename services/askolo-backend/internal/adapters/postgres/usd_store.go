package postgres

import (
	"context"
	"database/sql"
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

const (
	signupWelcomeUSDGrantKey    = "welcome-credit-usd-v1"
	signupWelcomeUSDGrantReason = "Signup welcome credit"
)

type USDRateCard struct {
	Provider                       string `json:"provider"`
	Mode                           string `json:"mode"`
	Model                          string `json:"model"`
	Meter                          string `json:"meter"`
	UsdMicrosPerHour               int64  `json:"usdMicrosPerHour,omitempty"`
	InputUsdMicrosPerMillion       int64  `json:"inputUsdMicrosPerMillion,omitempty"`
	CachedInputUsdMicrosPerMillion int64  `json:"cachedInputUsdMicrosPerMillion,omitempty"`
	OutputUsdMicrosPerMillion      int64  `json:"outputUsdMicrosPerMillion,omitempty"`
	UsdMicrosPerMillionCharacters  int64  `json:"usdMicrosPerMillionCharacters,omitempty"`
}
type USDPolicy struct {
	Version               int                    `json:"version"`
	RateCards             map[string]USDRateCard `json:"rateCards"`
	SignupGrantUsdMicros  int64                  `json:"signupGrantUsdMicros"`
	MonthlyGrantUsdMicros int64                  `json:"monthlyGrantUsdMicros"`
	MonthlyGrantsEnabled  bool                   `json:"monthlyGrantsEnabled"`
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
	ID                string      `json:"id"`
	OperationType     string      `json:"operationType,omitempty"`
	Status            string      `json:"status"`
	ReservedUsdMicros int64       `json:"reservedUsdMicros"`
	SettledUsdMicros  int64       `json:"settledUsdMicros"`
	RefundedUsdMicros int64       `json:"refundedUsdMicros"`
	PolicyVersion     int         `json:"policyVersion"`
	ExpiresAt         *time.Time  `json:"expiresAt"`
	CreatedAt         *time.Time  `json:"createdAt,omitempty"`
	Provider          string      `json:"provider"`
	Mode              string      `json:"mode"`
	Model             string      `json:"model"`
	RateCard          USDRateCard `json:"rateCard"`
	UsageUnit         string      `json:"usageUnit,omitempty"`
	UsageUnits        int64       `json:"usageUnits,omitempty"`
	InputTokens       int64       `json:"inputTokens,omitempty"`
	CachedInputTokens int64       `json:"cachedInputTokens,omitempty"`
	OutputTokens      int64       `json:"outputTokens,omitempty"`
	UsageStatus       string      `json:"usageStatus"`
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
var ErrUSDReservationNotFound = errors.New("USD reservation not found")
var ErrUSDReservationReviewRequired = errors.New("USD reservation is not pending review")

func equalJSON(a, b []byte) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func validateRateCard(c USDRateCard) error {
	if strings.TrimSpace(c.Provider) == "" || strings.TrimSpace(c.Mode) == "" || strings.TrimSpace(c.Model) == "" {
		return errors.New("invalid USD rate card identity")
	}
	if c.Meter != "hour" && c.Meter != "input_tokens" && c.Meter != "output_tokens" && c.Meter != "tokens" && c.Meter != "characters" {
		return errors.New("unknown USD meter")
	}
	for _, n := range []int64{c.UsdMicrosPerHour, c.InputUsdMicrosPerMillion, c.CachedInputUsdMicrosPerMillion, c.OutputUsdMicrosPerMillion, c.UsdMicrosPerMillionCharacters} {
		if n < 0 || n > math.MaxInt64/2 {
			return errors.New("USD rate overflow")
		}
	}
	if c.Meter == "hour" && c.UsdMicrosPerHour <= 0 {
		return errors.New("hourly USD rate is required")
	}
	if c.Meter == "characters" && c.UsdMicrosPerMillionCharacters <= 0 {
		return errors.New("character USD rate is required")
	}
	if (c.Meter == "input_tokens" && c.InputUsdMicrosPerMillion <= 0) || (c.Meter == "output_tokens" && c.OutputUsdMicrosPerMillion <= 0) || (c.Meter == "tokens" && (c.InputUsdMicrosPerMillion <= 0 || c.OutputUsdMicrosPerMillion <= 0)) {
		return errors.New("token USD rate is required")
	}
	return nil
}
func validateUSDPolicy(p USDPolicy) error {
	if p.Version < 1 || len(strings.TrimSpace(p.ChangeReason)) < 1 || len(p.ChangeReason) > 160 || p.SignupGrantUsdMicros < 0 || p.MonthlyGrantUsdMicros < 0 || p.RolloverCapUsdMicros < 0 || p.RolloverExpiryDays < 1 || p.OverrunMarginPercent < 0 || p.OverrunMarginPercent > 100 || len(p.RateCards) > 64 {
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
	err := s.pool.QueryRow(ctx, `SELECT version,rate_cards,signup_grant_usd_micros,monthly_grant_usd_micros,monthly_grants_enabled,rollover_cap_usd_micros,rollover_expiry_days,overrun_margin_percent,created_by,change_reason,created_at FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1`).Scan(&p.Version, &raw, &p.SignupGrantUsdMicros, &p.MonthlyGrantUsdMicros, &p.MonthlyGrantsEnabled, &p.RolloverCapUsdMicros, &p.RolloverExpiryDays, &p.OverrunMarginPercent, &p.CreatedBy, &p.ChangeReason, &p.CreatedAt)
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
	err = s.pool.QueryRow(ctx, `INSERT INTO ai_credit_policy_versions(version,rate_cards,signup_grant_usd_micros,monthly_grant_usd_micros,monthly_grants_enabled,rollover_cap_usd_micros,rollover_expiry_days,overrun_margin_percent,created_by,change_reason,operation_weights,monthly_grant_credits,rollover_cap_credits) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,operation_weights,monthly_grant_credits,rollover_cap_credits FROM ai_credit_policy_versions WHERE version=$11 RETURNING created_at`, p.Version, raw, p.SignupGrantUsdMicros, p.MonthlyGrantUsdMicros, p.MonthlyGrantsEnabled, p.RolloverCapUsdMicros, p.RolloverExpiryDays, p.OverrunMarginPercent, actor, p.ChangeReason, expected).Scan(&created)
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

func grantSignupWelcomeUSDTx(ctx context.Context, tx pgx.Tx, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("welcome grant user is required")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return err
	}
	var lockedUserID string
	if err := tx.QueryRow(ctx, `SELECT user_id FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		return err
	}

	var amount int64
	if err := tx.QueryRow(ctx, `SELECT signup_grant_usd_micros FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1`).Scan(&amount); err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}

	var sourceType, currency, reason string
	var existingAmount int64
	err := tx.QueryRow(ctx, `
		SELECT source_type, currency, amount_usd_micros, COALESCE(reason, '')
		FROM ai_credit_grants
		WHERE user_id=$1 AND idempotency_key=$2
`, userID, signupWelcomeUSDGrantKey).Scan(&sourceType, &currency, &existingAmount, &reason)
	if err == nil {
		if sourceType == "signup" && currency == "USD" &&
			existingAmount > 0 && reason == signupWelcomeUSDGrantReason {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	metadata := json.RawMessage(`{"grantType":"signup_welcome","version":1}`)
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_credit_grants
			(user_id,source_type,amount_credits,entitlement_key,idempotency_key,metadata,
			 actor_user_id,reason,currency,amount_usd_micros)
		VALUES ($1,'signup',1,$2,$2,$3,NULL,$4,'USD',$5)
	`, userID, signupWelcomeUSDGrantKey, metadata, signupWelcomeUSDGrantReason, amount); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_credit_accounts
		SET granted_usd_micros=granted_usd_micros+$2,updated_at=NOW()
		WHERE user_id=$1
	`, userID, amount); err != nil {
		return err
	}
	return nil
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

func (s *Store) ResolvePendingUSDReservation(
	ctx context.Context,
	id, userID, actor, key, reason, decision string,
	actual int64,
) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	id, userID, actor, key, reason = strings.TrimSpace(id), strings.TrimSpace(userID), strings.TrimSpace(actor), strings.TrimSpace(key), strings.TrimSpace(reason)
	if id == "" || userID == "" || actor == "" || key == "" || len(key) > 200 ||
		reason == "" || len(reason) > 160 ||
		(decision != "settle" && decision != "release") ||
		(decision == "settle" && actual <= 0) || (decision == "release" && actual != 0) {
		return errors.New("invalid pending USD usage resolution")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var reserved int64
	var state, provider, mode string
	if err := tx.QueryRow(ctx, `
		SELECT reserved_usd_micros,status,provider,mode FROM ai_credit_reservations
		WHERE id=$1 AND user_id=$2 AND currency='USD' FOR UPDATE
	`, id, userID).Scan(&reserved, &state, &provider, &mode); err != nil {
		return err
	}
	var priorAmount int64
	var priorDetails []byte
	err = tx.QueryRow(ctx, `
		SELECT amount_usd_micros,details FROM ai_credit_reservation_events
		WHERE reservation_id=$1 AND event_type='review_resolved' AND idempotency_key=$2
	`, id, key+":review").Scan(&priorAmount, &priorDetails)
	if err == nil {
		var prior struct {
			Actor    string `json:"actorUserId"`
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		_ = json.Unmarshal(priorDetails, &prior)
		if prior.Actor == actor && prior.Decision == decision && prior.Reason == reason && priorAmount == actual {
			return nil
		}
		return ErrUSDReservationConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if state != "claimed" || (decision == "settle" && actual > reserved) {
		return ErrUSDReservationReviewRequired
	}
	var hasPendingReview bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM ai_provider_usage_evidence
		  WHERE reservation_id=$1 AND user_id=$2 AND evidence->>'reviewStatus'='pending'
		)
	`, id, userID).Scan(&hasPendingReview); err != nil {
		return err
	}
	if !hasPendingReview && !(decision == "release" && provider == "azure" && mode == "assistant_speech") {
		return ErrUSDReservationReviewRequired
	}
	eventType := "released"
	eventAmount := reserved
	if decision == "settle" {
		eventType = "settled"
		eventAmount = actual
		tag, err := tx.Exec(ctx, `
			UPDATE ai_credit_accounts
			SET reserved_usd_micros=reserved_usd_micros-$2,
			    spent_usd_micros=spent_usd_micros+$3, updated_at=NOW()
			WHERE user_id=$1 AND reserved_usd_micros >= $2
		`, userID, reserved, actual)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("pending usage account balance conflict")
		}
		tag, err = tx.Exec(ctx, `
			UPDATE ai_credit_reservations
			SET status='settled', settled_usd_micros=$2,
			    refunded_usd_micros=reserved_usd_micros-$2, closed_at=NOW(), updated_at=NOW()
			WHERE id=$1 AND status='claimed'
		`, id, actual)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("pending usage reservation state conflict")
		}
	} else {
		tag, err := tx.Exec(ctx, `
			UPDATE ai_credit_accounts
			SET reserved_usd_micros=reserved_usd_micros-$2, updated_at=NOW()
			WHERE user_id=$1 AND reserved_usd_micros >= $2
		`, userID, reserved)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("pending usage account balance conflict")
		}
		tag, err = tx.Exec(ctx, `
			UPDATE ai_credit_reservations
			SET status='released', refunded_usd_micros=reserved_usd_micros,
			    closed_at=NOW(), updated_at=NOW()
			WHERE id=$1 AND status='claimed'
		`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("pending usage reservation state conflict")
		}
	}
	details, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason})
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key,details)
		VALUES ($1,$2,$3,'USD',$4,$5,$6)
	`, id, userID, eventType, eventAmount, key+":"+decision, details); err != nil {
		return err
	}
	reviewDetails, _ := json.Marshal(map[string]string{"actorUserId": actor, "decision": decision, "reason": reason})
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,currency,amount_usd_micros,idempotency_key,details)
		VALUES ($1,$2,'review_resolved','USD',$3,$4,$5)
	`, id, userID, actual, key+":review", reviewDetails); err != nil {
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
	rows, err := s.pool.Query(ctx, `
		SELECT r.id,r.operation_type,r.provider,r.mode,COALESCE(r.model,''),r.status,
			r.reserved_usd_micros,r.settled_usd_micros,r.refunded_usd_micros,
			r.policy_version,r.expires_at,r.created_at,r.rate_snapshot,
			COALESCE(e.usage_unit,''),COALESCE(e.usage_units,0),
			COALESCE(e.input_tokens,0),COALESCE((e.evidence->>'cachedInputTokens')::bigint,0),COALESCE(e.output_tokens,0),
CASE
  WHEN r.status='claimed' THEN 'pending_review'
  WHEN e.reservation_id IS NULL THEN 'unavailable'
  ELSE 'measured'
END
		FROM ai_credit_reservations r
		LEFT JOIN LATERAL (
			SELECT reservation_id,usage_unit,usage_units,input_tokens,output_tokens,evidence
			FROM ai_provider_usage_evidence
			WHERE reservation_id=r.id AND user_id=r.user_id
			ORDER BY recorded_at DESC
			LIMIT 1
		) e ON true
		WHERE r.user_id=$1 AND r.currency='USD'
		ORDER BY r.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := make([]USDReservation, 0)
	for rows.Next() {
		var r USDReservation
		var operation string
		var created time.Time
		var rateSnapshot []byte
		if err := rows.Scan(&r.ID, &operation, &r.Provider, &r.Mode, &r.Model, &r.Status, &r.ReservedUsdMicros, &r.SettledUsdMicros, &r.RefundedUsdMicros, &r.PolicyVersion, &r.ExpiresAt, &created, &rateSnapshot, &r.UsageUnit, &r.UsageUnits, &r.InputTokens, &r.CachedInputTokens, &r.OutputTokens, &r.UsageStatus); err != nil {
			return nil, err
		}
		if len(rateSnapshot) > 0 && string(rateSnapshot) != "{}" {
			if err := json.Unmarshal(rateSnapshot, &r.RateCard); err != nil {
				return nil, fmt.Errorf("decode USD receipt rate snapshot: %w", err)
			}
		}
		r.OperationType = operation
		r.CreatedAt = &created
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

type MonthlyUSDGrantBatch struct {
	Processed int
	Issued    int
	Deferred  int
	Skipped   int
}

type MonthlyUSDGrantPreview struct {
	WindowDays    int   `json:"windowDays"`
	DueCount      int64 `json:"dueCount"`
	IssuableCount int64 `json:"issuableCount"`
	DeferredCount int64 `json:"deferredCount"`
	SkippedCount  int64 `json:"skippedCount"`
}

type USDUserPolicyOverride struct {
	UserID                string     `json:"userId"`
	MonthlyGrantsEnabled  *bool      `json:"monthlyGrantsEnabled"`
	MonthlyGrantUsdMicros *int64     `json:"monthlyGrantUsdMicros"`
	RolloverCapUsdMicros  *int64     `json:"rolloverCapUsdMicros"`
	UpdatedBy             string     `json:"updatedBy,omitempty"`
	Reason                string     `json:"reason,omitempty"`
	UpdatedAt             *time.Time `json:"updatedAt,omitempty"`
}

type USDAdminAlert struct {
	AlertKey       string          `json:"alertKey"`
	AlertType      string          `json:"alertType"`
	UserID         *string         `json:"userId,omitempty"`
	Details        json.RawMessage `json:"details"`
	CreatedAt      time.Time       `json:"createdAt"`
	AcknowledgedAt *time.Time      `json:"acknowledgedAt,omitempty"`
}

func (s *Store) USDUserPolicyOverride(ctx context.Context, userID string) (USDUserPolicyOverride, error) {
	override := USDUserPolicyOverride{UserID: userID}
	if s == nil || s.pool == nil {
		return override, errors.New("database is not configured")
	}
	if strings.TrimSpace(userID) == "" {
		return override, errors.New("user id is required")
	}
	var enabled sql.NullBool
	var amount, cap sql.NullInt64
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT monthly_grants_enabled, monthly_grant_usd_micros, rollover_cap_usd_micros,
		       updated_by, reason, updated_at
		FROM ai_credit_user_policy_overrides WHERE user_id=$1
	`, userID).Scan(&enabled, &amount, &cap, &override.UpdatedBy, &override.Reason, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return override, nil
	}
	if err != nil {
		return USDUserPolicyOverride{}, err
	}
	if enabled.Valid {
		override.MonthlyGrantsEnabled = &enabled.Bool
	}
	if amount.Valid {
		override.MonthlyGrantUsdMicros = &amount.Int64
	}
	if cap.Valid {
		override.RolloverCapUsdMicros = &cap.Int64
	}
	override.UpdatedAt = &updatedAt
	return override, nil
}

func (s *Store) UpdateUSDUserPolicyOverride(ctx context.Context, override USDUserPolicyOverride, actor string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	override.UserID = strings.TrimSpace(override.UserID)
	actor = strings.TrimSpace(actor)
	override.Reason = strings.TrimSpace(override.Reason)
	if override.UserID == "" || actor == "" || override.Reason == "" || len(override.Reason) > 160 ||
		(override.MonthlyGrantUsdMicros != nil && (*override.MonthlyGrantUsdMicros < 0 || *override.MonthlyGrantUsdMicros > math.MaxInt64/2)) ||
		(override.RolloverCapUsdMicros != nil && (*override.RolloverCapUsdMicros < 0 || *override.RolloverCapUsdMicros > math.MaxInt64/2)) {
		return errors.New("invalid USD user policy override")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var lockedUser string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, override.UserID).Scan(&lockedUser); err != nil {
		return err
	}
	var oldEnabled sql.NullBool
	var oldAmount, oldCap sql.NullInt64
	err = tx.QueryRow(ctx, `
		SELECT monthly_grants_enabled, monthly_grant_usd_micros, rollover_cap_usd_micros
		FROM ai_credit_user_policy_overrides WHERE user_id=$1 FOR UPDATE
	`, override.UserID).Scan(&oldEnabled, &oldAmount, &oldCap)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	oldValues := map[string]any{}
	if oldEnabled.Valid {
		oldValues["monthlyGrantsEnabled"] = oldEnabled.Bool
	}
	if oldAmount.Valid {
		oldValues["monthlyGrantUsdMicros"] = oldAmount.Int64
	}
	if oldCap.Valid {
		oldValues["rolloverCapUsdMicros"] = oldCap.Int64
	}
	nextValues := map[string]any{}
	if override.MonthlyGrantsEnabled != nil {
		nextValues["monthlyGrantsEnabled"] = *override.MonthlyGrantsEnabled
	}
	if override.MonthlyGrantUsdMicros != nil {
		nextValues["monthlyGrantUsdMicros"] = *override.MonthlyGrantUsdMicros
	}
	if override.RolloverCapUsdMicros != nil {
		nextValues["rolloverCapUsdMicros"] = *override.RolloverCapUsdMicros
	}
	oldJSON, _ := json.Marshal(oldValues)
	nextJSON, _ := json.Marshal(nextValues)
	if len(nextValues) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM ai_credit_user_policy_overrides WHERE user_id=$1`, override.UserID); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `
		INSERT INTO ai_credit_user_policy_overrides
			(user_id,monthly_grants_enabled,monthly_grant_usd_micros,rollover_cap_usd_micros,updated_by,reason,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			monthly_grants_enabled=EXCLUDED.monthly_grants_enabled,
			monthly_grant_usd_micros=EXCLUDED.monthly_grant_usd_micros,
			rollover_cap_usd_micros=EXCLUDED.rollover_cap_usd_micros,
			updated_by=EXCLUDED.updated_by, reason=EXCLUDED.reason, updated_at=NOW()
	`, override.UserID, override.MonthlyGrantsEnabled, override.MonthlyGrantUsdMicros, override.RolloverCapUsdMicros, actor, override.Reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_credit_user_policy_override_events(user_id,actor_user_id,reason,previous_values,next_values)
		VALUES ($1,$2,$3,$4,$5)
	`, override.UserID, actor, override.Reason, oldJSON, nextJSON); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListUSDAdminAlerts(ctx context.Context, limit int) ([]USDAdminAlert, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT alert_key, alert_type, user_id, details, created_at, acknowledged_at
		FROM ai_credit_admin_alerts
		WHERE acknowledged_at IS NULL
		ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	alerts := make([]USDAdminAlert, 0)
	for rows.Next() {
		var alert USDAdminAlert
		if err := rows.Scan(&alert.AlertKey, &alert.AlertType, &alert.UserID, &alert.Details, &alert.CreatedAt, &alert.AcknowledgedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (s *Store) USDReservationForUser(ctx context.Context, id, userID string) (USDReservation, error) {
	var reservation USDReservation
	if s == nil || s.pool == nil {
		return reservation, errors.New("database is not configured")
	}
	var operation string
	var created time.Time
	var snapshot []byte
	err := s.pool.QueryRow(ctx, `
		SELECT r.id,r.operation_type,r.provider,r.mode,COALESCE(r.model,''),r.status,
		       r.reserved_usd_micros,r.settled_usd_micros,r.refunded_usd_micros,
		       r.policy_version,r.expires_at,r.created_at,r.rate_snapshot,
		       COALESCE(e.usage_unit,''),COALESCE(e.usage_units,0),
		       COALESCE(e.input_tokens,0),COALESCE((e.evidence->>'cachedInputTokens')::bigint,0),
		       COALESCE(e.output_tokens,0),
		       CASE WHEN r.status='claimed' THEN 'pending_review'
		            WHEN e.reservation_id IS NULL THEN 'unavailable' ELSE 'measured' END
		FROM ai_credit_reservations r
		LEFT JOIN LATERAL (
		  SELECT reservation_id,usage_unit,usage_units,input_tokens,output_tokens,evidence
		  FROM ai_provider_usage_evidence
		  WHERE reservation_id=r.id AND user_id=r.user_id
		  ORDER BY recorded_at DESC LIMIT 1
		) e ON true
		WHERE r.id=$1 AND r.user_id=$2 AND r.currency='USD'
	`, id, userID).Scan(
		&reservation.ID, &operation, &reservation.Provider, &reservation.Mode, &reservation.Model,
		&reservation.Status, &reservation.ReservedUsdMicros, &reservation.SettledUsdMicros,
		&reservation.RefundedUsdMicros, &reservation.PolicyVersion, &reservation.ExpiresAt,
		&created, &snapshot, &reservation.UsageUnit, &reservation.UsageUnits,
		&reservation.InputTokens, &reservation.CachedInputTokens, &reservation.OutputTokens,
		&reservation.UsageStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return USDReservation{}, ErrUSDReservationNotFound
	}
	if err != nil {
		return USDReservation{}, err
	}
	if len(snapshot) > 0 && string(snapshot) != "{}" {
		if err := json.Unmarshal(snapshot, &reservation.RateCard); err != nil {
			return USDReservation{}, fmt.Errorf("decode USD receipt rate snapshot: %w", err)
		}
	}
	reservation.OperationType = operation
	reservation.CreatedAt = &created
	return reservation, nil
}

func (s *Store) AcknowledgeUSDAdminAlert(ctx context.Context, alertKey, actor string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	alertKey = strings.TrimSpace(alertKey)
	actor = strings.TrimSpace(actor)
	if alertKey == "" || len(alertKey) > 240 || actor == "" {
		return false, errors.New("invalid AI credit alert")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE ai_credit_admin_alerts SET acknowledged_at=NOW(),acknowledged_by=$2
		WHERE alert_key=$1 AND acknowledged_at IS NULL
	`, alertKey, actor)
	return tag.RowsAffected() == 1, err
}

const monthlyUSDGrantInterval = 30 * 24 * time.Hour

func nextMonthlyUSDGrantCycle(scheduledAt, now time.Time) time.Time {
	next := scheduledAt.Add(monthlyUSDGrantInterval)
	if next.After(now) {
		return next
	}
	cycles := now.Sub(scheduledAt)/monthlyUSDGrantInterval + 1
	return scheduledAt.Add(cycles * monthlyUSDGrantInterval)
}

// ProcessDueMonthlyUSDGrants issues one cycle per due account. A cap-blocked
// cycle remains anchored to its original signup date and is retried later.
func (s *Store) ProcessDueMonthlyUSDGrants(ctx context.Context, now time.Time, limit int) (MonthlyUSDGrantBatch, error) {
	var result MonthlyUSDGrantBatch
	if s == nil || s.pool == nil {
		return result, errors.New("database is not configured")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	now = now.UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	// Seed each verified account at its next signup-anchored 30-day boundary;
	// never issue historical cycles when the scheduler is first enabled.
	_, err = tx.Exec(ctx, `
		INSERT INTO ai_credit_monthly_grant_schedules(user_id, next_cycle_at)
		SELECT u.id,
		       u.created_at + (
		         floor(extract(epoch FROM ($1::timestamptz - u.created_at)) / 2592000.0)::bigint + 1
		       ) * interval '30 days'
		FROM users u
		WHERE u.email_verified_at IS NOT NULL AND COALESCE(u.status,'active') <> 'deleted'
		ON CONFLICT (user_id) DO NOTHING
	`, now)
	if err != nil {
		return result, err
	}

	rows, err := tx.Query(ctx, `
		SELECT s.user_id, s.next_cycle_at,
		       p.version, p.monthly_grants_enabled AND COALESCE(o.monthly_grants_enabled, true),
		       COALESCE(o.monthly_grant_usd_micros, p.monthly_grant_usd_micros),
		       COALESCE(o.rollover_cap_usd_micros, p.rollover_cap_usd_micros)
		FROM ai_credit_monthly_grant_schedules s
		JOIN users u ON u.id=s.user_id AND COALESCE(u.status,'active') <> 'deleted'
		CROSS JOIN LATERAL (
		  SELECT version, monthly_grants_enabled, monthly_grant_usd_micros, rollover_cap_usd_micros
		  FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1
		) p
		LEFT JOIN ai_credit_user_policy_overrides o ON o.user_id=s.user_id
		WHERE COALESCE(s.retry_at, s.next_cycle_at) <= $1
		ORDER BY COALESCE(s.retry_at, s.next_cycle_at), s.user_id
		LIMIT $2
		FOR UPDATE OF s SKIP LOCKED
	`, now, limit)
	if err != nil {
		return result, err
	}
	type dueCycle struct {
		userID    string
		scheduled time.Time
		version   int
		enabled   bool
		amount    int64
		cap       int64
	}
	due := make([]dueCycle, 0, limit)
	for rows.Next() {
		var cycle dueCycle
		if err := rows.Scan(&cycle.userID, &cycle.scheduled, &cycle.version, &cycle.enabled, &cycle.amount, &cycle.cap); err != nil {
			rows.Close()
			return result, err
		}
		due = append(due, cycle)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	for _, cycle := range due {
		result.Processed++
		if !cycle.enabled || cycle.amount == 0 {
			status := "skipped_disabled"
			if cycle.enabled {
				status = "skipped_zero_amount"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ai_credit_monthly_grant_cycles(user_id, scheduled_at, policy_version, status)
				VALUES ($1,$2,$3,$4)
				ON CONFLICT (user_id, scheduled_at) DO UPDATE
				SET policy_version=EXCLUDED.policy_version, status=EXCLUDED.status,
				    amount_usd_micros=0, grant_id=NULL, retry_count=0, updated_at=$5
			`, cycle.userID, cycle.scheduled, cycle.version, status, now); err != nil {
				return result, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE ai_credit_monthly_grant_schedules
				SET next_cycle_at=$2, retry_at=NULL, updated_at=$3
				WHERE user_id=$1
			`, cycle.userID, nextMonthlyUSDGrantCycle(cycle.scheduled, now), now); err != nil {
				return result, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE ai_credit_admin_alerts
				SET acknowledged_at=$3
				WHERE user_id=$1 AND alert_type='monthly_grant_deferred'
				  AND details->>'scheduledAt'=$2 AND acknowledged_at IS NULL
			`, cycle.userID, cycle.scheduled.UTC().Format(time.RFC3339Nano), now); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}

		if _, err := tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, cycle.userID); err != nil {
			return result, err
		}
		var balance int64
		if err := tx.QueryRow(ctx, `
			SELECT granted_usd_micros+adjustment_usd_micros-spent_usd_micros+refunded_usd_micros
			FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE
		`, cycle.userID).Scan(&balance); err != nil {
			return result, err
		}
		if cycle.cap <= 0 || balance >= cycle.cap || cycle.amount > cycle.cap-balance {
			alertKey := fmt.Sprintf("monthly-grant-cap:%s:%s:%d", cycle.userID, cycle.scheduled.UTC().Format("20060102T150405Z"), cycle.version)
			details, _ := json.Marshal(map[string]any{
				"scheduledAt":           cycle.scheduled.UTC(),
				"balanceUsdMicros":      balance,
				"monthlyGrantUsdMicros": cycle.amount,
				"rolloverCapUsdMicros":  cycle.cap,
				"policyVersion":         cycle.version,
			})
			if _, err := tx.Exec(ctx, `
				INSERT INTO ai_credit_monthly_grant_cycles(user_id, scheduled_at, policy_version, status, amount_usd_micros, retry_count)
				VALUES ($1,$2,$3,'deferred_cap',$4,1)
				ON CONFLICT (user_id, scheduled_at) DO UPDATE
				SET policy_version=EXCLUDED.policy_version, status='deferred_cap',
				    amount_usd_micros=EXCLUDED.amount_usd_micros,
				    retry_count=ai_credit_monthly_grant_cycles.retry_count+1,
				    updated_at=EXCLUDED.updated_at
			`, cycle.userID, cycle.scheduled, cycle.version, cycle.amount); err != nil {
				return result, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO ai_credit_admin_alerts(alert_key, alert_type, user_id, details)
				VALUES ($1,'monthly_grant_deferred',$2,$3)
				ON CONFLICT (alert_key) DO NOTHING
			`, alertKey, cycle.userID, details); err != nil {
				return result, err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE ai_credit_monthly_grant_schedules
				SET retry_at=$2, updated_at=$3 WHERE user_id=$1
			`, cycle.userID, now.Add(24*time.Hour), now); err != nil {
				return result, err
			}
			result.Deferred++
			continue
		}

		grantKey := fmt.Sprintf("monthly-usd-v1:%s:%s", cycle.userID, cycle.scheduled.UTC().Format("20060102T150405Z"))
		metadata, _ := json.Marshal(map[string]any{
			"grantType":     "monthly_recurring",
			"scheduledAt":   cycle.scheduled.UTC(),
			"policyVersion": cycle.version,
		})
		var grantID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO ai_credit_grants
				(user_id,source_type,amount_credits,entitlement_key,idempotency_key,metadata,actor_user_id,reason,currency,amount_usd_micros)
			VALUES ($1,'monthly',1,$2,$2,$3,NULL,'Monthly recurring credit','USD',$4)
			RETURNING id
		`, cycle.userID, grantKey, metadata, cycle.amount).Scan(&grantID); err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ai_credit_accounts SET granted_usd_micros=granted_usd_micros+$2, updated_at=$3
			WHERE user_id=$1
		`, cycle.userID, cycle.amount, now); err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_credit_monthly_grant_cycles(user_id,scheduled_at,policy_version,status,amount_usd_micros,grant_id)
			VALUES ($1,$2,$3,'issued',$4,$5)
			ON CONFLICT (user_id,scheduled_at) DO UPDATE
			SET policy_version=EXCLUDED.policy_version, status='issued',
			    amount_usd_micros=EXCLUDED.amount_usd_micros, grant_id=EXCLUDED.grant_id,
			    updated_at=$6
		`, cycle.userID, cycle.scheduled, cycle.version, cycle.amount, grantID, now); err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ai_credit_monthly_grant_schedules
			SET next_cycle_at=$2,retry_at=NULL,updated_at=$3
			WHERE user_id=$1
		`, cycle.userID, cycle.scheduled.Add(monthlyUSDGrantInterval), now); err != nil {
			return result, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ai_credit_admin_alerts
			SET acknowledged_at=$3
			WHERE user_id=$1 AND alert_type='monthly_grant_deferred'
			  AND details->>'scheduledAt'=$2 AND acknowledged_at IS NULL
		`, cycle.userID, cycle.scheduled.UTC().Format(time.RFC3339Nano), now); err != nil {
			return result, err
		}
		result.Issued++
	}
	if err := tx.Commit(ctx); err != nil {
		return MonthlyUSDGrantBatch{}, err
	}
	return result, nil
}

func (s *Store) PreviewMonthlyUSDGrants(ctx context.Context, now time.Time) (MonthlyUSDGrantPreview, error) {
	preview := MonthlyUSDGrantPreview{WindowDays: 30}
	if s == nil || s.pool == nil {
		return preview, errors.New("database is not configured")
	}
	now = now.UTC()
	err := s.pool.QueryRow(ctx, `
		WITH policy AS (
		  SELECT version, monthly_grants_enabled, monthly_grant_usd_micros, rollover_cap_usd_micros
		  FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1
		), candidates AS (
		  SELECT COALESCE(s.retry_at, s.next_cycle_at,
		           u.created_at + (
		             floor(extract(epoch FROM ($1::timestamptz-u.created_at)) / 2592000.0)::bigint + 1
		           ) * interval '30 days') AS due_at,
		         p.monthly_grants_enabled AND COALESCE(o.monthly_grants_enabled, true) AS enabled,
		         COALESCE(o.monthly_grant_usd_micros, p.monthly_grant_usd_micros) AS grant_amount,
		         COALESCE(o.rollover_cap_usd_micros, p.rollover_cap_usd_micros) AS rollover_cap,
		         COALESCE(a.granted_usd_micros+a.adjustment_usd_micros-a.spent_usd_micros+a.refunded_usd_micros, 0) AS balance
		  FROM users u
		  CROSS JOIN policy p
		  LEFT JOIN ai_credit_monthly_grant_schedules s ON s.user_id=u.id
		  LEFT JOIN ai_credit_user_policy_overrides o ON o.user_id=u.id
		  LEFT JOIN ai_credit_accounts a ON a.user_id=u.id
		  WHERE u.email_verified_at IS NOT NULL AND COALESCE(u.status,'active') <> 'deleted'
		), due AS (
		  SELECT * FROM candidates WHERE due_at <= $1::timestamptz + interval '30 days'
		)
		SELECT count(*),
		       count(*) FILTER (WHERE enabled AND grant_amount > 0 AND rollover_cap > 0
		         AND balance < rollover_cap AND grant_amount <= rollover_cap-balance),
		       count(*) FILTER (WHERE enabled AND grant_amount > 0
		         AND (rollover_cap <= 0 OR balance >= rollover_cap OR grant_amount > rollover_cap-balance)),
		       count(*) FILTER (WHERE NOT enabled OR grant_amount = 0)
		FROM due
	`, now).Scan(&preview.DueCount, &preview.IssuableCount, &preview.DeferredCount, &preview.SkippedCount)
	return preview, err
}

// RecordUSDEvidence stores provider metering metadata only. Payload must not
// contain transcripts, prompts, audio, or other user content.
func validateUSDUsageEvidence(evidence USDEvidence) error {
	if len(evidence.ProviderRequestId) > 200 || len(evidence.Payload) > 8192 ||
		evidence.UsageUnits < 0 || evidence.DurationMs < 0 || evidence.InputTokens < 0 || evidence.OutputTokens < 0 ||
		(evidence.UsageUnit != "hour" && evidence.UsageUnit != "second" && evidence.UsageUnit != "millisecond" && evidence.UsageUnit != "milliseconds" && evidence.UsageUnit != "characters" &&
			evidence.UsageUnit != "input_tokens" && evidence.UsageUnit != "output_tokens" && evidence.UsageUnit != "tokens") {
		return errors.New("invalid USD usage evidence")
	}
	if len(evidence.Payload) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(evidence.Payload, &fields) != nil {
		return errors.New("usage payload must be a JSON object")
	}
	for field, raw := range fields {
		switch field {
		case "providerRequestId", "usageUnit", "meterSource":
			var value string
			if json.Unmarshal(raw, &value) != nil || len(value) > 200 {
				return errors.New("invalid usage metadata")
			}
		case "usageUnits", "durationMs", "inputTokens", "outputTokens":
			var value int64
			if json.Unmarshal(raw, &value) != nil || value < 0 {
				return errors.New("invalid usage metadata")
			}
		case "reviewStatus":
			var value string
			if json.Unmarshal(raw, &value) != nil || value != "pending" {
				return errors.New("invalid usage review status")
			}
		case "outputStatus":
			var value string
			if json.Unmarshal(raw, &value) != nil || value != "unavailable" {
				return errors.New("invalid usage output status")
			}
		default:
			return errors.New("usage payload contains non-metering data")
		}
	}
	return nil
}

func (s *Store) RecordUSDEvidence(ctx context.Context, reservationID, userID, provider, mode, model, key string, evidence USDEvidence) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if reservationID == "" || userID == "" || provider == "" || mode == "" || key == "" || len(key) > 240 {
		return errors.New("invalid USD usage evidence")
	}
	if err := validateUSDUsageEvidence(evidence); err != nil {
		return err
	}
	payload := evidence.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
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
