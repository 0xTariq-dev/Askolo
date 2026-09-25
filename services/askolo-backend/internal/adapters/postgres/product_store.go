package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const VoiceConsentVersion = "voice-v2"

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
