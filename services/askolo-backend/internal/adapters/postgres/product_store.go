package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const VoiceConsentVersion = "voice-v3"

type AICreditPolicy struct {
	Version              int            `json:"version"`
	OperationWeights     map[string]int `json:"operationWeights"`
	MonthlyGrantCredits  int            `json:"monthlyGrantCredits"`
	RolloverCapCredits   int            `json:"rolloverCapCredits"`
	RolloverExpiryDays   int            `json:"rolloverExpiryDays"`
	OverrunMarginPercent int            `json:"overrunMarginPercent"`
	CreatedBy            string         `json:"createdBy"`
	ChangeReason         string         `json:"changeReason"`
	CreatedAt            time.Time      `json:"createdAt"`
}

type AICreditUsage struct {
	Balance     int `json:"balance"`
	Granted     int `json:"granted"`
	Adjustments int `json:"adjustments"`
	Reserved    int `json:"reserved"`
	Spent       int `json:"spent"`
	Refunded    int `json:"refunded"`
}

type AICreditReservation struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	ReservedCredits int        `json:"reservedCredits"`
	SettledCredits  int        `json:"settledCredits"`
	RefundedCredits int        `json:"refundedCredits"`
	PolicyVersion   int        `json:"policyVersion"`
	ExpiresAt       *time.Time `json:"expiresAt"`
}

type ProductField struct {
	Column string
	Name   string
}

type ProductSpec struct {
	Name       string
	Table      string
	Fields     []ProductField
	Required   []string
	Defaults   map[string]any
	OrderBy    string
	Filterable map[string]bool
}

func (s *Store) ListProduct(ctx context.Context, spec ProductSpec, userID string, filters map[string]string) ([]map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	columns := productColumns(spec)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE user_id = $1", columns, spec.Table)
	args := []any{userID}
	placeholder := 2
	for column, value := range filters {
		if value == "" || !spec.Filterable[column] {
			continue
		}
		query += fmt.Sprintf(" AND %s = $%d", column, placeholder)
		args = append(args, value)
		placeholder++
	}
	if spec.OrderBy != "" {
		query += " ORDER BY " + spec.OrderBy
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectProductRows(rows, spec), rows.Err()
}

func (s *Store) GetProduct(ctx context.Context, spec ProductSpec, userID string, id int64) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = $1 AND user_id = $2", productColumns(spec), spec.Table)
	row := s.pool.QueryRow(ctx, query, id, userID)
	values, err := rowValues(row, len(spec.Fields))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return productMap(spec, values), nil
}

func (s *Store) CreateProduct(ctx context.Context, spec ProductSpec, userID string, input map[string]any) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	values := make(map[string]any, len(input)+1)
	for key, value := range input {
		values[key] = value
	}
	values["user_id"] = userID
	for key, value := range spec.Defaults {
		if _, ok := values[key]; !ok {
			values[key] = value
		}
	}
	columns := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	placeholders := make([]string, 0, len(values))
	for _, field := range spec.Fields {
		if field.Column == "id" || field.Column == "created_at" || field.Column == "updated_at" {
			continue
		}
		value, ok := values[field.Name]
		if field.Column == "user_id" {
			value, ok = userID, true
		}
		if !ok {
			continue
		}
		columns = append(columns, field.Column)
		args = append(args, value)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		spec.Table, strings.Join(columns, ", "), strings.Join(placeholders, ", "), productColumns(spec),
	)
	valuesRow, err := rowValues(s.pool.QueryRow(ctx, query, args...), len(spec.Fields))
	if err != nil {
		return nil, err
	}
	return productMap(spec, valuesRow), nil
}

func (s *Store) UpdateProduct(ctx context.Context, spec ProductSpec, userID string, id int64, input map[string]any) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	sets := make([]string, 0, len(input)+1)
	args := make([]any, 0, len(input)+2)
	for _, field := range spec.Fields {
		if field.Column == "id" || field.Column == "user_id" || field.Column == "created_at" || field.Column == "updated_at" {
			continue
		}
		value, ok := input[field.Name]
		if !ok {
			continue
		}
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", field.Column, len(args)))
	}
	if len(sets) == 0 {
		return s.GetProduct(ctx, spec, userID, id)
	}
	args = append(args, id, userID)
	query := fmt.Sprintf(
		"UPDATE %s SET %s, updated_at = NOW() WHERE id = $%d AND user_id = $%d RETURNING %s",
		spec.Table, strings.Join(sets, ", "), len(args)-1, len(args), productColumns(spec),
	)
	values, err := rowValues(s.pool.QueryRow(ctx, query, args...), len(spec.Fields))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return productMap(spec, values), nil
}

func (s *Store) DeleteProduct(ctx context.Context, spec ProductSpec, userID string, id int64) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	command, err := s.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND user_id = $2", spec.Table), id, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) HabitCompletedToday(ctx context.Context, userID string, habitID int64, date string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM habit_completions c
			JOIN habits h ON h.id = c.habit_id
			WHERE h.id = $1 AND h.user_id = $2 AND c.date = $3
		)
	`, habitID, userID, date).Scan(&exists)
	return exists, err
}

func (s *Store) ListHabitCompletions(ctx context.Context, userID string, habitID *int64, from, to string) ([]map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	query := `
		SELECT c.id, c.habit_id, c.date, c.created_at
		FROM habit_completions c
		JOIN habits h ON h.id = c.habit_id
		WHERE h.user_id = $1`
	args := []any{userID}
	index := 2
	if habitID != nil {
		query += fmt.Sprintf(" AND c.habit_id = $%d", index)
		args = append(args, *habitID)
		index++
	}
	if from != "" {
		query += fmt.Sprintf(" AND c.date >= $%d", index)
		args = append(args, from)
		index++
	}
	if to != "" {
		query += fmt.Sprintf(" AND c.date <= $%d", index)
		args = append(args, to)
	}
	query += " ORDER BY c.date DESC, c.id DESC"
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, habit int64
		var date string
		var created time.Time
		if err := rows.Scan(&id, &habit, &date, &created); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"id": id, "habitId": habit, "date": date, "createdAt": created})
	}
	return result, rows.Err()
}

func (s *Store) CompleteHabit(ctx context.Context, userID string, habitID int64, date string) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	var id int64
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO habit_completions (habit_id, date)
		SELECT $1, $2 WHERE EXISTS (SELECT 1 FROM habits WHERE id = $1 AND user_id = $3)
		ON CONFLICT (habit_id, date) DO UPDATE SET date = EXCLUDED.date
		RETURNING id, created_at
	`, habitID, date, userID).Scan(&id, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.RecomputeHabitStreak(ctx, userID, habitID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "habitId": habitID, "date": date, "createdAt": created}, nil
}

func (s *Store) DeleteHabitCompletion(ctx context.Context, userID string, habitID int64, date string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	command, err := s.pool.Exec(ctx, `
		DELETE FROM habit_completions c
		USING habits h
		WHERE c.habit_id = h.id AND c.habit_id = $1 AND c.date = $2 AND h.user_id = $3
	`, habitID, date, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return s.RecomputeHabitStreak(ctx, userID, habitID)
}

func (s *Store) RecomputeHabitStreak(ctx context.Context, userID string, habitID int64) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		WITH ordered AS (
			SELECT date::date, date::date - (ROW_NUMBER() OVER (ORDER BY date::date))::int AS group_key
			FROM habit_completions c JOIN habits h ON h.id = c.habit_id
			WHERE c.habit_id = $1 AND h.user_id = $2
		), streaks AS (
			SELECT COUNT(*)::int AS length, MAX(date) AS last_date
			FROM ordered GROUP BY group_key
		), values AS (
			SELECT COALESCE(MAX(length), 0)::int AS longest,
				COALESCE(MAX(length) FILTER (WHERE last_date = CURRENT_DATE), 0)::int AS current
			FROM streaks
		)
		UPDATE habits h SET current_streak = values.current, longest_streak = values.longest, updated_at = NOW()
		FROM values WHERE h.id = $1 AND h.user_id = $2
	`, habitID, userID)
	return err
}

func (s *Store) AICreditBalance(ctx context.Context, userID string) (int, error) {
	if s == nil || s.pool == nil {
		return 0, errors.New("database is not configured")
	}
	var balance int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(granted_credits + adjustment_credits - reserved_credits - spent_credits + refunded_credits, 0)
		FROM ai_credit_accounts WHERE user_id = $1
	`, userID).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return balance, err
}

func (s *Store) AICreditUsage(ctx context.Context, userID string) (AICreditUsage, error) {
	var usage AICreditUsage
	if s == nil || s.pool == nil {
		return usage, errors.New("database is not configured")
	}
	err := s.pool.QueryRow(ctx, `SELECT granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits,
		granted_credits + adjustment_credits - reserved_credits - spent_credits + refunded_credits
		FROM ai_credit_accounts WHERE user_id=$1`, userID).Scan(&usage.Granted, &usage.Adjustments, &usage.Reserved, &usage.Spent, &usage.Refunded, &usage.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return usage, nil
	}
	return usage, err
}

func (s *Store) AICreditRecent(ctx context.Context, userID string, limit int) (map[string]any, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	reservations := make([]map[string]any, 0)
	rows, err := s.pool.Query(ctx, `SELECT id, operation_type, provider, mode, status, reserved_credits, settled_credits, refunded_credits, policy_version, expires_at, created_at
		FROM ai_credit_reservations WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, operation, provider, mode, status string
		var reserved, settled, refunded, policyVersion int
		var expires, created *time.Time
		if err := rows.Scan(&id, &operation, &provider, &mode, &status, &reserved, &settled, &refunded, &policyVersion, &expires, &created); err != nil {
			rows.Close()
			return nil, err
		}
		reservations = append(reservations, map[string]any{"id": id, "operationType": operation, "provider": provider, "mode": mode, "status": status, "reservedCredits": reserved, "settledCredits": settled, "refundedCredits": refunded, "policyVersion": policyVersion, "expiresAt": expires, "createdAt": created})
	}
	rows.Close()
	events := make([]map[string]any, 0)
	rows, err = s.pool.Query(ctx, `SELECT id, reservation_id, event_type, credits, duration_ms, details, created_at FROM ai_credit_reservation_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var reservation, event string
		var credits int
		var duration *int
		var details []byte
		var created *time.Time
		if err := rows.Scan(&id, &reservation, &event, &credits, &duration, &details, &created); err != nil {
			rows.Close()
			return nil, err
		}
		var audit map[string]any
		_ = json.Unmarshal(details, &audit)
		events = append(events, map[string]any{"id": id, "reservationId": reservation, "eventType": event, "credits": credits, "durationMs": duration, "details": audit, "createdAt": created})
	}
	rows.Close()
	adjustments := make([]map[string]any, 0)
	rows, err = s.pool.Query(ctx, `SELECT id,user_id,amount_credits,reason,COALESCE(actor_user_id,''),reversal_of_id,reversal_of_grant_id,created_at FROM ai_credit_adjustments WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var target, reason, actor string
		var amount int
		var reversedAdjustment, reversedGrant *int64
		var created *time.Time
		if err := rows.Scan(&id, &target, &amount, &reason, &actor, &reversedAdjustment, &reversedGrant, &created); err != nil {
			rows.Close()
			return nil, err
		}
		adjustments = append(adjustments, map[string]any{"id": id, "userId": target, "amountCredits": amount, "reason": reason, "actorUserId": actor, "reversalOfId": reversedAdjustment, "reversalOfGrantId": reversedGrant, "createdAt": created})
	}
	rows.Close()
	grants := make([]map[string]any, 0)
	rows, err = s.pool.Query(ctx, `SELECT id,user_id,amount_credits,COALESCE(reason,''),COALESCE(actor_user_id,''),created_at FROM ai_credit_grants WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var target, reason, actor string
		var amount int
		var created *time.Time
		if err := rows.Scan(&id, &target, &amount, &reason, &actor, &created); err != nil {
			rows.Close()
			return nil, err
		}
		grants = append(grants, map[string]any{"id": id, "userId": target, "amountCredits": amount, "reason": reason, "actorUserId": actor, "createdAt": created})
	}
	rows.Close()
	return map[string]any{"reservations": reservations, "events": events, "adjustments": adjustments, "grants": grants}, nil
}

func (s *Store) AICreditPolicy(ctx context.Context) (AICreditPolicy, error) {
	var policy AICreditPolicy
	var weights []byte
	err := s.pool.QueryRow(ctx, `SELECT version, operation_weights, monthly_grant_credits, rollover_cap_credits,
		rollover_expiry_days, overrun_margin_percent, created_by, change_reason, created_at
		FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1`).Scan(&policy.Version, &weights,
		&policy.MonthlyGrantCredits, &policy.RolloverCapCredits, &policy.RolloverExpiryDays,
		&policy.OverrunMarginPercent, &policy.CreatedBy, &policy.ChangeReason, &policy.CreatedAt)
	if err != nil {
		return policy, err
	}
	if err := json.Unmarshal(weights, &policy.OperationWeights); err != nil {
		return policy, err
	}
	return policy, nil
}

func (s *Store) UpdateAICreditPolicy(ctx context.Context, expected int, policy AICreditPolicy, actor string) (AICreditPolicy, error) {
	if policy.Version != expected+1 || policy.MonthlyGrantCredits < 0 || policy.RolloverCapCredits < 0 ||
		policy.RolloverExpiryDays < 1 || policy.OverrunMarginPercent < 0 || policy.OverrunMarginPercent > 100 {
		return AICreditPolicy{}, errors.New("invalid policy")
	}
	if len(policy.OperationWeights) == 0 || len(policy.OperationWeights) > 32 {
		return AICreditPolicy{}, errors.New("invalid policy")
	}
	for key, weight := range policy.OperationWeights {
		if strings.TrimSpace(key) == "" || weight < 1 || weight > 100000 || len(key) > 64 {
			return AICreditPolicy{}, errors.New("invalid policy")
		}
	}
	weights, err := json.Marshal(policy.OperationWeights)
	if err != nil {
		return AICreditPolicy{}, err
	}
	var created time.Time
	err = s.pool.QueryRow(ctx, `INSERT INTO ai_credit_policy_versions
		(version, operation_weights, monthly_grant_credits, rollover_cap_credits, rollover_expiry_days, overrun_margin_percent, created_by, change_reason)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8
		WHERE NOT EXISTS (SELECT 1 FROM ai_credit_policy_versions WHERE version > $9)
		RETURNING created_at`, policy.Version, weights, policy.MonthlyGrantCredits, policy.RolloverCapCredits,
		policy.RolloverExpiryDays, policy.OverrunMarginPercent, actor, policy.ChangeReason, expected).Scan(&created)
	if errors.Is(err, pgx.ErrNoRows) {
		return AICreditPolicy{}, errors.New("policy version conflict")
	}
	if err != nil {
		return AICreditPolicy{}, err
	}
	policy.CreatedBy, policy.CreatedAt = actor, created
	return policy, nil
}

func (s *Store) AddAICreditAdjustment(ctx context.Context, target, actor string, amount int, reason, key string) error {
	if amount == 0 || len(strings.TrimSpace(reason)) < 1 || len(reason) > 160 || len(key) < 1 || len(key) > 200 {
		return errors.New("invalid adjustment")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status='active')`, target).Scan(&exists); err != nil || !exists {
		return errors.New("target user not found")
	}
	var metadata []byte
	var available int
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, target); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT granted_credits+adjustment_credits-reserved_credits-spent_credits+refunded_credits FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, target).Scan(&available); err != nil {
		return err
	}
	var priorGrant int
	var priorGrantReason, priorGrantActor string
	err = tx.QueryRow(ctx, `SELECT amount_credits, COALESCE(reason,''), COALESCE(actor_user_id,'') FROM ai_credit_grants WHERE user_id=$1 AND idempotency_key=$2`, target, key).Scan(&priorGrant, &priorGrantReason, &priorGrantActor)
	if err == nil {
		if priorGrant != amount || priorGrantReason != reason || priorGrantActor != actor {
			return errors.New("idempotency conflict")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var existing int
	err = tx.QueryRow(ctx, `SELECT amount_credits, metadata FROM ai_credit_adjustments WHERE user_id=$1 AND idempotency_key=$2`, target, key).Scan(&existing, &metadata)
	if err == nil {
		var prior map[string]string
		_ = json.Unmarshal(metadata, &prior)
		if existing != amount || prior["actorUserId"] != actor || prior["reason"] != reason {
			return errors.New("idempotency conflict")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if amount < 0 && available+amount < 0 {
		return errors.New("insufficient balance")
	}
	metadata, _ = json.Marshal(map[string]string{"actorUserId": actor, "reason": reason})
	if amount > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_grants (user_id, source_type, amount_credits, entitlement_key, idempotency_key, metadata, actor_user_id, reason) VALUES ($1,'admin',$2,$3,$4,$5,$6,$7)`, target, amount, key, key, metadata, actor, reason); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET granted_credits=granted_credits+$2,updated_at=NOW() WHERE user_id=$1`, target, amount); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments (user_id, amount_credits, reason, idempotency_key, metadata, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,$6)`, target, amount, reason, key, metadata, actor); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts (user_id, adjustment_credits) VALUES ($1,$2)
		ON CONFLICT (user_id) DO UPDATE SET adjustment_credits=ai_credit_accounts.adjustment_credits+EXCLUDED.adjustment_credits, updated_at=NOW()`, target, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReverseAICreditEntry(ctx context.Context, target, actor string, entryID int64, key, reason string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if entryID < 1 || strings.TrimSpace(actor) == "" || strings.TrimSpace(key) == "" || strings.TrimSpace(reason) == "" || len(reason) > 160 {
		return errors.New("invalid reversal")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var amount int
	var originalReason string
	var originalActor string
	if err = tx.QueryRow(ctx, `SELECT amount_credits,reason,COALESCE(actor_user_id,'') FROM ai_credit_adjustments WHERE id=$1 AND user_id=$2 FOR UPDATE`, entryID, target).Scan(&amount, &originalReason, &originalActor); err != nil {
		return err
	}
	var priorAmount int
	var priorReason, priorKey, priorActor string
	err = tx.QueryRow(ctx, `SELECT amount_credits,reason,idempotency_key,COALESCE(actor_user_id,'') FROM ai_credit_adjustments WHERE reversal_of_id=$1`, entryID).Scan(&priorAmount, &priorReason, &priorKey, &priorActor)
	if err == nil {
		if priorAmount == -amount && priorReason == reason && priorKey == key && priorActor == actor {
			return nil
		}
		return errors.New("entry already reversed")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"actorUserId": actor, "reversalOf": fmt.Sprint(entryID), "originalActor": originalActor, "originalReason": originalReason})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments(user_id,amount_credits,reason,idempotency_key,metadata,actor_user_id,reversal_of_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, target, -amount, reason, key, meta, actor, entryID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE ai_credit_accounts SET adjustment_credits=adjustment_credits-$2,updated_at=NOW() WHERE user_id=$1 AND adjustment_credits-$2+granted_credits-reserved_credits-spent_credits+refunded_credits>=0`, target, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("insufficient balance")
	}
	return tx.Commit(ctx)
}

func (s *Store) ReverseAICreditGrant(ctx context.Context, target, actor string, grantID int64, key, reason string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if grantID < 1 || strings.TrimSpace(actor) == "" || strings.TrimSpace(key) == "" || strings.TrimSpace(reason) == "" || len(reason) > 160 {
		return errors.New("invalid reversal")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var amount int
	if err = tx.QueryRow(ctx, `SELECT amount_credits FROM ai_credit_grants WHERE id=$1 AND user_id=$2 FOR UPDATE`, grantID, target).Scan(&amount); err != nil {
		return err
	}
	var priorAmount int
	var priorReason, priorKey, priorActor string
	err = tx.QueryRow(ctx, `SELECT amount_credits,reason,idempotency_key,COALESCE(actor_user_id,'') FROM ai_credit_adjustments WHERE reversal_of_grant_id=$1`, grantID).Scan(&priorAmount, &priorReason, &priorKey, &priorActor)
	if err == nil {
		if priorAmount == -amount && priorReason == reason && priorKey == key && priorActor == actor {
			return nil
		}
		return errors.New("grant already reversed")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status='active')`, target).Scan(&active); err != nil || !active {
		return errors.New("target user not found")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, target); err != nil {
		return err
	}
	var available int
	if err = tx.QueryRow(ctx, `SELECT granted_credits+adjustment_credits-reserved_credits-spent_credits+refunded_credits FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, target).Scan(&available); err != nil {
		return err
	}
	if available < amount {
		return errors.New("insufficient balance")
	}
	metadata, _ := json.Marshal(map[string]string{"actorUserId": actor, "reversalOfGrant": fmt.Sprint(grantID)})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_adjustments(user_id,amount_credits,reason,idempotency_key,metadata,actor_user_id,reversal_of_grant_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, target, -amount, reason, key, metadata, actor, grantID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET adjustment_credits=adjustment_credits-$2,updated_at=NOW() WHERE user_id=$1`, target, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReserveAICredits(ctx context.Context, id, userID, operation, provider, mode, key string, credits, ttlSeconds int, policyVersions ...int) (AICreditReservation, bool, error) {
	var result AICreditReservation
	if s == nil || s.pool == nil {
		return result, false, errors.New("database is not configured")
	}
	policyVersion := 1
	if len(policyVersions) > 0 {
		policyVersion = policyVersions[0]
	}
	if len(policyVersions) > 1 || policyVersion < 1 || id == "" || userID == "" || key == "" || credits < 1 || credits > 100000 || ttlSeconds < 1 || ttlSeconds > 3600 {
		return result, false, errors.New("invalid reservation")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, false, err
	}
	defer tx.Rollback(ctx)
	var prior AICreditReservation
	err = tx.QueryRow(ctx, `SELECT id,status,reserved_credits,settled_credits,refunded_credits,policy_version,expires_at FROM ai_credit_reservations WHERE user_id=$1 AND idempotency_key=$2`, userID, key).
		Scan(&prior.ID, &prior.Status, &prior.ReservedCredits, &prior.SettledCredits, &prior.RefundedCredits, &prior.PolicyVersion, &prior.ExpiresAt)
	if err == nil {
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, false, err
	}
	// Lock stale reservations before the account, matching settlement order.
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credit_reservations WHERE user_id=$1 AND status='reserved' AND expires_at <= NOW() FOR UPDATE`, userID); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `WITH expired AS (UPDATE ai_credit_reservations SET status='expired', closed_at=NOW(), updated_at=NOW() WHERE user_id=$1 AND status='reserved' AND expires_at <= NOW() RETURNING id,user_id,reserved_credits), total AS (SELECT COALESCE(SUM(reserved_credits),0) credits FROM expired), account AS (UPDATE ai_credit_accounts SET reserved_credits=reserved_credits-(SELECT credits FROM total),updated_at=NOW() WHERE user_id=$1 RETURNING user_id) INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key) SELECT id,user_id,'expired',reserved_credits,id||':expired' FROM expired ON CONFLICT (reservation_id,idempotency_key) DO NOTHING`, userID); err != nil {
		return result, false, err
	}
	var balance int
	_, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return result, false, err
	}
	if err = tx.QueryRow(ctx, `SELECT granted_credits+adjustment_credits-reserved_credits-spent_credits+refunded_credits FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return result, false, err
	}
	// A concurrent request with the same key may have passed the initial
	// lookup before it acquired the account lock. Recheck after serialization
	// so it returns the committed reservation instead of hitting the unique
	// idempotency index on INSERT.
	err = tx.QueryRow(ctx, `SELECT id,status,reserved_credits,settled_credits,refunded_credits,policy_version,expires_at
		FROM ai_credit_reservations WHERE user_id=$1 AND idempotency_key=$2`, userID, key).
		Scan(&prior.ID, &prior.Status, &prior.ReservedCredits, &prior.SettledCredits, &prior.RefundedCredits, &prior.PolicyVersion, &prior.ExpiresAt)
	if err == nil {
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, false, err
	}
	if balance < credits {
		return result, false, nil
	}
	expires := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_credits=reserved_credits+$2,updated_at=NOW() WHERE user_id=$1`, userID, credits); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservations(id,user_id,operation_type,provider,mode,status,idempotency_key,reserved_credits,max_credits,expires_at,policy_version) VALUES($1,$2,$3,$4,$5,'reserved',$6,$7,$7,$8,$9)`, id, userID, operation, provider, mode, key, credits, expires, policyVersion); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key) VALUES($1,$2,'reserved',$3,$4)`, id, userID, credits, key+":reserved"); err != nil {
		return result, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, false, err
	}
	result = AICreditReservation{ID: id, Status: "reserved", ReservedCredits: credits, PolicyVersion: policyVersion, ExpiresAt: &expires}
	return result, true, nil
}

func (s *Store) ClaimAICreditReservation(ctx context.Context, id, userID string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE ai_credit_reservations SET status='claimed',started_at=NOW(),updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='reserved' AND (expires_at IS NULL OR expires_at>NOW())`, id, userID)
	return tag.RowsAffected() == 1, err
}

func (s *Store) SettleAICreditReservation(ctx context.Context, id, userID, key string, actual int) error {
	if actual < 0 {
		return errors.New("invalid settlement")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var reserved, status int
	_ = status
	var state string
	if err = tx.QueryRow(ctx, `SELECT reserved_credits,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "settled" {
		var credits int
		err = tx.QueryRow(ctx, `SELECT credits FROM ai_credit_reservation_events WHERE reservation_id=$1 AND event_type='settled' AND idempotency_key=$2`, id, key).Scan(&credits)
		if err == nil && credits == actual {
			return nil
		}
		return errors.New("settlement idempotency conflict")
	}
	if state != "claimed" {
		return errors.New("invalid reservation state")
	}
	if actual > reserved {
		return errors.New("settlement exceeds reservation")
	}
	refund := 0
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credit_reservations WHERE user_id=$1 AND status='reserved' AND expires_at<=NOW() FOR UPDATE`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_credits=reserved_credits-$2,spent_credits=spent_credits+$3,updated_at=NOW() WHERE user_id=$1 AND reserved_credits >= $2`, userID, reserved, actual); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_reservations SET status='settled',settled_credits=$2,refunded_credits=$3,closed_at=NOW(),updated_at=NOW() WHERE id=$1`, id, actual, refund); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key) VALUES($1,$2,'settled',$3,$4) ON CONFLICT DO NOTHING`, id, userID, actual, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ReleaseAICreditReservation(ctx context.Context, id, userID, key string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var reserved int
	var state string
	if err = tx.QueryRow(ctx, `SELECT reserved_credits,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "released" {
		var credits int
		err = tx.QueryRow(ctx, `SELECT credits FROM ai_credit_reservation_events WHERE reservation_id=$1 AND event_type='released' AND idempotency_key=$2`, id, key).Scan(&credits)
		if err == nil && credits == reserved {
			return nil
		}
		return errors.New("release idempotency conflict")
	}
	if state != "claimed" && state != "reserved" {
		return errors.New("invalid reservation state")
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_credits=reserved_credits-$2,updated_at=NOW() WHERE user_id=$1 AND reserved_credits >= $2`, userID, reserved); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_reservations SET status='released',refunded_credits=reserved_credits,closed_at=NOW(),updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key) VALUES($1,$2,'released',$3,$4) ON CONFLICT DO NOTHING`, id, userID, reserved, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RefundAICreditReservation(ctx context.Context, id, userID, actor, key, reason string, amount int) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if amount < 1 || strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || len(reason) > 160 || key == "" {
		return errors.New("invalid refund")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var settled, refunded int
	var state string
	if err = tx.QueryRow(ctx, `SELECT settled_credits,refunded_credits,status FROM ai_credit_reservations WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&settled, &refunded, &state); err != nil {
		return err
	}
	if state != "settled" {
		return errors.New("refund exceeds settled credits")
	}
	var prior int
	var priorDetails []byte
	err = tx.QueryRow(ctx, `SELECT credits,details FROM ai_credit_reservation_events WHERE reservation_id=$1 AND event_type='refunded' AND idempotency_key=$2`, id, key).Scan(&prior, &priorDetails)
	if err == nil {
		var audit map[string]string
		_ = json.Unmarshal(priorDetails, &audit)
		if prior == amount && audit["actorUserId"] == actor && audit["reason"] == reason {
			return nil
		}
		return errors.New("refund idempotency conflict")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if amount > settled-refunded {
		return errors.New("refund exceeds settled credits")
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_reservations SET refunded_credits=refunded_credits+$2,updated_at=NOW() WHERE id=$1`, id, amount); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE ai_credit_accounts SET refunded_credits=refunded_credits+$2,updated_at=NOW() WHERE user_id=$1`, userID, amount); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]string{"actorUserId": actor, "reason": reason})
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key,details) VALUES($1,$2,'refunded',$3,$4,$5)`, id, userID, amount, key, details); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SpendAICredits(ctx context.Context, userID string, credits int) (int, bool, error) {
	if credits < 1 {
		return 0, false, errors.New("AI credit spend must be positive")
	}
	if s == nil || s.pool == nil {
		return 0, false, errors.New("database is not configured")
	}
	var balance int
	err := s.pool.QueryRow(ctx, `
		UPDATE ai_credit_accounts
		SET spent_credits = COALESCE(spent_credits, 0) + $2
		WHERE user_id = $1
			AND COALESCE(granted_credits + adjustment_credits - reserved_credits - spent_credits + refunded_credits, 0) >= $2
		RETURNING COALESCE(granted_credits + adjustment_credits - reserved_credits - spent_credits + refunded_credits, 0)
	`, userID, credits).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return balance, true, nil
}

func (s *Store) VoiceConsent(ctx context.Context, userID string) (bool, string, error) {
	if s == nil || s.pool == nil {
		return false, "", errors.New("database is not configured")
	}
	var consentAt *time.Time
	var version *string
	err := s.pool.QueryRow(ctx, `
		SELECT consent_at, consent_version FROM voice_preferences WHERE user_id = $1
	`, userID).Scan(&consentAt, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if consentAt == nil || version == nil || *version != VoiceConsentVersion {
		if version == nil {
			return false, "", nil
		}
		return false, *version, nil
	}
	return true, *version, nil
}

func (s *Store) SetVoiceConsent(ctx context.Context, userID string, enabled bool, version string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	var consentAt any
	var consentVersion any
	if enabled {
		consentAt, consentVersion = time.Now().UTC(), version
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO voice_preferences (user_id, consent_at, consent_version)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			consent_at = EXCLUDED.consent_at,
			consent_version = EXCLUDED.consent_version,
			updated_at = NOW()
	`, userID, consentAt, consentVersion)
	return err
}

func (s *Store) UpdateUserProfile(ctx context.Context, userID, firstName, lastName string) (User, error) {
	if s == nil || s.pool == nil {
		return User{}, errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET first_name = NULLIF($2, ''), last_name = NULLIF($3, ''), updated_at = NOW()
		WHERE id = $1
	`, userID, firstName, lastName)
	if err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) DeleteUserData(ctx context.Context, userID string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, query := range []string{
		`DELETE FROM habit_completions WHERE habit_id IN (SELECT id FROM habits WHERE user_id = $1)`,
		`DELETE FROM habits WHERE user_id = $1`,
		`DELETE FROM goals WHERE user_id = $1`,
		`DELETE FROM daily_plans WHERE user_id = $1`,
		`DELETE FROM events WHERE user_id = $1`,
		`DELETE FROM chores WHERE user_id = $1`,
		`DELETE FROM notes WHERE user_id = $1`,
		`DELETE FROM assistant_conversations WHERE user_id = $1`,
		`DELETE FROM action_items WHERE user_id = $1`,
		`DELETE FROM voice_preferences WHERE user_id = $1`,
		`DELETE FROM ai_credit_accounts WHERE user_id = $1`,
	} {
		if _, err := tx.Exec(ctx, query, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteAccount(ctx context.Context, userID string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, query := range []string{
		`DELETE FROM sessions WHERE sess->>'userId' = $1`,
		`DELETE FROM auth_email_challenges WHERE user_id = $1`,
		`DELETE FROM auth_passwords WHERE user_id = $1`,
		`DELETE FROM auth_recovery_codes WHERE user_id = $1`,
		`DELETE FROM auth_recovery_methods WHERE user_id = $1`,
		`DELETE FROM auth_security_events WHERE user_id = $1`,
		`DELETE FROM auth_totp WHERE user_id = $1`,
		`DELETE FROM users WHERE id = $1`,
	} {
		if _, err := tx.Exec(ctx, query, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func productColumns(spec ProductSpec) string {
	columns := make([]string, 0, len(spec.Fields))
	for _, field := range spec.Fields {
		columns = append(columns, field.Column)
	}
	return strings.Join(columns, ", ")
}

func collectProductRows(rows pgx.Rows, spec ProductSpec) []map[string]any {
	result := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			continue
		}
		result = append(result, productMap(spec, values))
	}
	return result
}

func rowValues(row pgx.Row, length int) ([]any, error) {
	values := make([]any, length)
	pointers := make([]any, length)
	for index := range values {
		pointers[index] = &values[index]
	}
	if err := row.Scan(pointers...); err != nil {
		return nil, err
	}
	return values, nil
}

func productMap(spec ProductSpec, values []any) map[string]any {
	result := make(map[string]any, len(spec.Fields))
	for index, field := range spec.Fields {
		if index < len(values) {
			result[field.Name] = values[index]
		}
	}
	return result
}
