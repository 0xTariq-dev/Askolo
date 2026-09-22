package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"askolo/backend/internal/platform/crypto"
	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EmailChallengeRetention is the minimum time a terminal email challenge is
// retained for security investigation and operational troubleshooting.
const EmailChallengeRetention = 24 * time.Hour

// EmailChallengeCleanupBatchSize bounds the amount of work in one cleanup
// transaction. The service repeats cleanup on its next interval if needed.
const EmailChallengeCleanupBatchSize = 100

var ErrNotFound = errors.New("record not found")
var ErrOwnership = errors.New("record does not belong to user")
var ErrEmailExists = errors.New("email already belongs to an account")
var ErrChallengeInvalid = errors.New("email challenge is invalid")
var ErrChallengeLocked = errors.New("email challenge is locked")
var ErrChallengeRecentlySent = errors.New("email challenge was sent recently")
var ErrRecoveryUnavailable = errors.New("recovery method is unavailable")
var ErrMFAAlreadyEnabled = errors.New("multi-factor authentication is already enabled")
var ErrMFANotEnrolled = errors.New("multi-factor authentication is not enrolled")
var ErrMFAEnrollmentUnavailable = errors.New("multi-factor enrollment is unavailable")
var ErrMFAChallengeExpired = errors.New("multi-factor challenge is expired")
var ErrMFAChallengeLocked = errors.New("multi-factor challenge is locked")
var ErrMFAChallengeInvalid = errors.New("multi-factor challenge is invalid")
var ErrMFAReplay = errors.New("multi-factor challenge was already used")
var ErrRecoveryCodeInvalid = errors.New("recovery code is invalid")

type Store struct {
	pool *pgxpool.Pool
}

type SessionMFAState struct {
	UserID    string
	Required  bool
	Verified  bool
	Attempts  int
	ExpiresAt time.Time
}

// MFAEventSummary contains bounded, aggregate MFA security-event counts for
// an observation window. It intentionally does not include user IDs, request
// IDs, event metadata, or any submitted authentication material.
type MFAEventSummary struct {
	FailureEvents                            int64
	ReplayEvents                             int64
	LockoutEvents                            int64
	DecryptionFailureEvents                  int64
	RecoverySupportRequests                  int64
	RecoverySupportVerificationFailures      int64
	RecoverySupportRateLimited               int64
	RecoverySupportSessionRevocationFailures int64
	AffectedUsers                            int64
}

type ProviderConnection struct {
	ID              string
	UserID          string
	AccountID       string
	Provider        string
	Status          string
	Scopes          []string
	Capabilities    []string
	Email           string
	DisplayName     string
	ExternalSubject string
	RevokedAt       *time.Time
	UpdatedAt       time.Time
}

type ProviderCredentials struct {
	ConnectionID string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
}

type ProviderDefault struct {
	Service      string
	ConnectionID string
}

type User struct {
	ID                string
	Email             string
	FirstName         string
	LastName          string
	ProfileImageURL   string
	Status            string
	EmailVerifiedAt   *time.Time
	AccountCreatedVia string
}

type ProviderIdentity struct {
	ID              string
	UserID          string
	Provider        string
	ExternalSubject string
	Email           string
	LoginEnabled    bool
	EmailVerified   bool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, nil
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	config.MaxConns = 8
	config.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	return s.pool.Ping(ctx)
}

func (s *Store) InsertOAuthState(ctx context.Context, nonceHash, flow, userID, returnTo string, expiresAt time.Time) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO google_oauth_states (nonce_hash, flow, user_id, return_to, expires_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
	`, nonceHash, flow, userID, returnTo, expiresAt)
	return err
}

func (s *Store) ConsumeOAuthState(ctx context.Context, nonceHash, flow string) (string, string, bool, error) {
	if s == nil {
		return "", "", false, errors.New("database is not configured")
	}
	var userID, returnTo string
	err := s.pool.QueryRow(ctx, `
		UPDATE google_oauth_states
		SET consumed_at = NOW()
		WHERE nonce_hash = $1 AND flow = $2 AND consumed_at IS NULL AND expires_at > NOW()
		RETURNING COALESCE(user_id, ''), return_to
	`, nonceHash, flow).Scan(&userID, &returnTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return userID, returnTo, true, nil
}

func (s *Store) UpsertUser(ctx context.Context, email, firstName, lastName, imageURL string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var userID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, first_name, last_name, profile_image_url)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''))
		ON CONFLICT (email) DO UPDATE SET
			first_name = COALESCE(EXCLUDED.first_name, users.first_name),
			last_name = COALESCE(EXCLUDED.last_name, users.last_name),
			profile_image_url = COALESCE(EXCLUDED.profile_image_url, users.profile_image_url),
			updated_at = NOW()
		RETURNING id
	`, email, firstName, lastName, imageURL).Scan(&userID)
	return userID, err
}

func (s *Store) CreateSession(ctx context.Context, userID, provider string, ttl time.Duration) (string, error) {
	return s.createSession(ctx, userID, provider, ttl, true, time.Time{})
}

func (s *Store) CreateMFAPendingSession(ctx context.Context, userID, provider string, ttl, challengeTTL time.Duration) (string, error) {
	return s.createSession(ctx, userID, provider, ttl, false, time.Now().UTC().Add(challengeTTL))
}

func (s *Store) createSession(
	ctx context.Context,
	userID, provider string,
	ttl time.Duration,
	mfaVerified bool,
	mfaExpiresAt time.Time,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	sessionID, err := id.New()
	if err != nil {
		return "", err
	}
	payloadMap := map[string]any{
		"userId":      userID,
		"provider":    provider,
		"createdAt":   time.Now().UTC().Format(time.RFC3339),
		"mfaVerified": mfaVerified,
	}
	if !mfaVerified {
		payloadMap["mfaRequired"] = true
		payloadMap["mfaChallengeExpiresAt"] = mfaExpiresAt.Format(time.RFC3339)
		payloadMap["mfaAttempts"] = 0
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (sid, sess, expire) VALUES ($1, $2::jsonb, $3)
	`, sessionStorageKey(sessionID), payload, time.Now().Add(ttl))
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Store) SessionMFAState(ctx context.Context, sessionID string) (SessionMFAState, error) {
	if s == nil {
		return SessionMFAState{}, errors.New("database is not configured")
	}
	var payload []byte
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT sess, expire
		FROM sessions
		WHERE sid = $1 OR sid = $2
		LIMIT 1
	`, sessionStorageKey(sessionID), sessionID).Scan(&payload, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionMFAState{}, ErrNotFound
	}
	if err != nil {
		return SessionMFAState{}, err
	}
	if expiresAt.Before(time.Now()) {
		return SessionMFAState{}, ErrNotFound
	}
	state, err := parseSessionMFAState(payload)
	if err != nil {
		return SessionMFAState{}, ErrNotFound
	}
	state.UserID = sessionUserIDFromPayload(payload)
	if state.UserID == "" {
		return SessionMFAState{}, ErrNotFound
	}
	if state.Required && !state.Verified && !state.ExpiresAt.IsZero() && state.ExpiresAt.Before(time.Now()) {
		return state, ErrMFAChallengeExpired
	}
	return state, nil
}

func (s *Store) SessionUserID(ctx context.Context, sessionID string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var payload []byte
	var expiresAt time.Time
	storageKey := sessionStorageKey(sessionID)
	err := s.pool.QueryRow(ctx, `
		SELECT sess, expire
		FROM sessions
		WHERE sid = $1 OR sid = $2
		LIMIT 1
	`, storageKey, sessionID).
		Scan(&payload, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if expiresAt.Before(time.Now()) {
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE sid = $1 OR sid = $2`, storageKey, sessionID)
		return "", ErrNotFound
	}
	var session struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(payload, &session); err != nil || session.UserID == "" {
		return "", ErrNotFound
	}
	return session.UserID, nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	if s == nil || sessionID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE sid = $1 OR sid = $2`, sessionStorageKey(sessionID), sessionID)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	if s == nil || userID == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE sess->>'userId' = $1
	`, userID)
	return err
}

func sessionStorageKey(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

func parseSessionMFAState(payload []byte) (SessionMFAState, error) {
	var raw struct {
		UserID                string `json:"userId"`
		MFARequired           bool   `json:"mfaRequired"`
		MFAVerified           *bool  `json:"mfaVerified"`
		MFAChallengeExpiresAt string `json:"mfaChallengeExpiresAt"`
		MFAAttempts           int    `json:"mfaAttempts"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return SessionMFAState{}, err
	}
	verified := true
	if raw.MFAVerified != nil {
		verified = *raw.MFAVerified
	}
	state := SessionMFAState{
		UserID:   raw.UserID,
		Required: raw.MFARequired || !verified,
		Verified: verified,
		Attempts: raw.MFAAttempts,
	}
	if raw.MFAChallengeExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, raw.MFAChallengeExpiresAt)
		if err != nil {
			return SessionMFAState{}, err
		}
		state.ExpiresAt = expiresAt
	}
	return state, nil
}

func sessionUserIDFromPayload(payload []byte) string {
	state, err := parseSessionMFAState(payload)
	if err != nil {
		return ""
	}
	return state.UserID
}

func sessionWithMFACompleted(payload []byte) ([]byte, error) {
	var values map[string]any
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, err
	}
	values["mfaRequired"] = false
	values["mfaVerified"] = true
	delete(values, "mfaChallengeExpiresAt")
	delete(values, "mfaAttempts")
	return json.Marshal(values)
}

func (s *Store) TOTPEnabled(ctx context.Context, userID string) (bool, error) {
	if s == nil {
		return false, errors.New("database is not configured")
	}
	var enabled bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM auth_totp
			WHERE user_id = $1 AND enabled_at IS NOT NULL
		)
	`, userID).Scan(&enabled)
	return enabled, err
}

func (s *Store) BeginTOTPEnrollment(ctx context.Context, userID, encryptedSecret string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var enabled bool
	err = tx.QueryRow(ctx, `
		SELECT enabled_at IS NOT NULL
		FROM auth_totp
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `
			INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at, last_used_step)
			VALUES ($1, $2, NULL, NULL)
		`, userID, encryptedSecret)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_totp
		SET secret_encrypted = $2, enabled_at = NULL, last_used_step = NULL, updated_at = NOW()
		WHERE user_id = $1
	`, userID, encryptedSecret); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) EncryptedTOTPSecret(ctx context.Context, userID string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var encryptedSecret string
	err := s.pool.QueryRow(ctx, `
		SELECT secret_encrypted
		FROM auth_totp
		WHERE user_id = $1
	`, userID).Scan(&encryptedSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMFANotEnrolled
	}
	return encryptedSecret, err
}

func (s *Store) ConfirmTOTPEnrollment(
	ctx context.Context,
	userID string,
	recoveryCodeHashes []string,
	lastUsedStep int64,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if len(recoveryCodeHashes) == 0 {
		return ErrMFAEnrollmentUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var enabled bool
	err = tx.QueryRow(ctx, `
		SELECT enabled_at IS NOT NULL
		FROM auth_totp
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFANotEnrolled
	}
	if err != nil {
		return err
	}
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_totp
		SET enabled_at = NOW(), last_used_step = $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, lastUsedStep); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, codeHash := range recoveryCodeHashes {
		codeID, err := id.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_recovery_codes (id, user_id, code_hash)
			VALUES ($1, $2, $3)
		`, codeID, userID, codeHash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) loadPendingMFASession(
	ctx context.Context,
	tx pgx.Tx,
	sessionID, userID string,
	maxAttempts int,
) ([]byte, SessionMFAState, error) {
	var payload []byte
	var expiresAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT sess, expire
		FROM sessions
		WHERE sid = $1 OR sid = $2
		FOR UPDATE
	`, sessionStorageKey(sessionID), sessionID).Scan(&payload, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, SessionMFAState{}, ErrNotFound
	}
	if err != nil {
		return nil, SessionMFAState{}, err
	}
	if expiresAt.Before(time.Now()) {
		return nil, SessionMFAState{}, ErrNotFound
	}
	state, err := parseSessionMFAState(payload)
	if err != nil {
		return nil, SessionMFAState{}, ErrNotFound
	}
	if state.UserID != userID {
		return nil, SessionMFAState{}, ErrOwnership
	}
	if !state.Required || state.Verified {
		return nil, state, ErrMFAChallengeExpired
	}
	if !state.ExpiresAt.IsZero() && state.ExpiresAt.Before(time.Now()) {
		return nil, state, ErrMFAChallengeExpired
	}
	if state.Attempts >= maxAttempts {
		return nil, state, ErrMFAChallengeLocked
	}
	return payload, state, nil
}

func updateSessionPayload(ctx context.Context, tx pgx.Tx, sessionID string, payload []byte) error {
	_, err := tx.Exec(ctx, `
		UPDATE sessions
		SET sess = $3::jsonb
		WHERE sid = $1 OR sid = $2
	`, sessionStorageKey(sessionID), sessionID, payload)
	return err
}

func (s *Store) RecordMFAFailure(ctx context.Context, sessionID, userID string, maxAttempts int) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	payload, state, err := s.loadPendingMFASession(ctx, tx, sessionID, userID, maxAttempts)
	if err != nil {
		return err
	}
	values := map[string]any{}
	if err := json.Unmarshal(payload, &values); err != nil {
		return err
	}
	state.Attempts++
	values["mfaAttempts"] = state.Attempts
	payload, err = json.Marshal(values)
	if err != nil {
		return err
	}
	if err := updateSessionPayload(ctx, tx, sessionID, payload); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if state.Attempts >= maxAttempts {
		return ErrMFAChallengeLocked
	}
	return ErrMFAChallengeInvalid
}

func (s *Store) CompleteTOTPChallenge(ctx context.Context, sessionID, userID string, step int64, maxAttempts int) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	payload, _, err := s.loadPendingMFASession(ctx, tx, sessionID, userID, maxAttempts)
	if err != nil {
		return err
	}
	var lastUsedStep *int64
	err = tx.QueryRow(ctx, `
		SELECT last_used_step
		FROM auth_totp
		WHERE user_id = $1 AND enabled_at IS NOT NULL
		FOR UPDATE
	`, userID).Scan(&lastUsedStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFANotEnrolled
	}
	if err != nil {
		return err
	}
	if lastUsedStep != nil && step <= *lastUsedStep {
		return ErrMFAReplay
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_totp SET last_used_step = $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, step); err != nil {
		return err
	}
	payload, err = sessionWithMFACompleted(payload)
	if err != nil {
		return err
	}
	if err := updateSessionPayload(ctx, tx, sessionID, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ConsumeRecoveryCode(
	ctx context.Context, sessionID, userID, codeHash string, maxAttempts int,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	payload, state, err := s.loadPendingMFASession(ctx, tx, sessionID, userID, maxAttempts)
	if err != nil {
		return err
	}
	var codeID string
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM auth_recovery_codes
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL
		FOR UPDATE
	`, userID, codeHash).Scan(&codeID)
	if errors.Is(err, pgx.ErrNoRows) {
		values := map[string]any{}
		if unmarshalErr := json.Unmarshal(payload, &values); unmarshalErr != nil {
			return unmarshalErr
		}
		state.Attempts++
		values["mfaAttempts"] = state.Attempts
		payload, marshalErr := json.Marshal(values)
		if marshalErr != nil {
			return marshalErr
		}
		if updateErr := updateSessionPayload(ctx, tx, sessionID, payload); updateErr != nil {
			return updateErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return commitErr
		}
		if state.Attempts >= maxAttempts {
			return ErrMFAChallengeLocked
		}
		return ErrRecoveryCodeInvalid
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_recovery_codes SET used_at = NOW() WHERE id = $1
	`, codeID); err != nil {
		return err
	}
	payload, err = sessionWithMFACompleted(payload)
	if err != nil {
		return err
	}
	if err := updateSessionPayload(ctx, tx, sessionID, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DisableTOTPWithRecoveryCode(ctx context.Context, userID, codeHash string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var enabled bool
	err = tx.QueryRow(ctx, `
		SELECT enabled_at IS NOT NULL
		FROM auth_totp
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFANotEnrolled
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrMFANotEnrolled
	}
	var codeID string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM auth_recovery_codes
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL
		FOR UPDATE
	`, userID, codeHash).Scan(&codeID); errors.Is(err, pgx.ErrNoRows) {
		return ErrRecoveryCodeInvalid
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_recovery_codes SET used_at = NOW() WHERE id = $1`, codeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_totp WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RegenerateRecoveryCodes(
	ctx context.Context, userID, currentCodeHash string, newCodeHashes []string,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if len(newCodeHashes) == 0 {
		return ErrMFAEnrollmentUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var enabled bool
	err = tx.QueryRow(ctx, `
		SELECT enabled_at IS NOT NULL
		FROM auth_totp
		WHERE user_id = $1
		FOR UPDATE
	`, userID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFANotEnrolled
	}
	if err != nil {
		return err
	}
	if !enabled {
		return ErrMFANotEnrolled
	}
	var codeID string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM auth_recovery_codes
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL
		FOR UPDATE
	`, userID, currentCodeHash).Scan(&codeID); errors.Is(err, pgx.ErrNoRows) {
		return ErrRecoveryCodeInvalid
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_recovery_codes SET used_at = NOW() WHERE id = $1`, codeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, codeHash := range newCodeHashes {
		newCodeID, err := id.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_recovery_codes (id, user_id, code_hash)
			VALUES ($1, $2, $3)
		`, newCodeID, userID, codeHash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) GetUser(ctx context.Context, userID string) (User, error) {
	if s == nil {
		return User{}, errors.New("database is not configured")
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), COALESCE(first_name, ''), COALESCE(last_name, ''),
		       COALESCE(profile_image_url, ''), COALESCE(status, 'active'),
		       email_verified_at, COALESCE(account_created_via, '')
		FROM users
		WHERE id = $1
	`, userID).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName,
		&user.ProfileImageURL, &user.Status, &user.EmailVerifiedAt, &user.AccountCreatedVia,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (User, error) {
	if s == nil {
		return User{}, errors.New("database is not configured")
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT id, COALESCE(email, ''), COALESCE(first_name, ''), COALESCE(last_name, ''),
		       COALESCE(profile_image_url, ''), COALESCE(status, 'active'),
		       email_verified_at, COALESCE(account_created_via, '')
		FROM users
		WHERE lower(email) = lower($1)
	`, strings.TrimSpace(email)).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName,
		&user.ProfileImageURL, &user.Status, &user.EmailVerifiedAt, &user.AccountCreatedVia,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) FindProviderIdentity(ctx context.Context, provider, externalSubject string) (ProviderIdentity, error) {
	if s == nil {
		return ProviderIdentity{}, errors.New("database is not configured")
	}
	var identity ProviderIdentity
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, provider, external_subject, COALESCE(email, ''),
		       login_enabled, email_verified
		FROM provider_accounts
		WHERE provider = $1 AND external_subject = $2
	`, provider, externalSubject).Scan(
		&identity.ID, &identity.UserID, &identity.Provider, &identity.ExternalSubject,
		&identity.Email, &identity.LoginEnabled, &identity.EmailVerified,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderIdentity{}, ErrNotFound
	}
	return identity, err
}

func (s *Store) CreatePasswordUser(ctx context.Context, email, passwordHash string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&existingID)
	if err == nil {
		return "", ErrEmailExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	userID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email, status, account_created_via)
		VALUES ($1, $2, 'pending_email_verification', 'password')
	`, userID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO auth_passwords (user_id, password_hash, hash_version)
		VALUES ($1, $2, 'argon2id-v1')
	`, userID, passwordHash)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) SetPassword(ctx context.Context, userID, passwordHash string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_passwords (user_id, password_hash, hash_version)
		VALUES ($1, $2, 'argon2id-v1')
		ON CONFLICT (user_id) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			hash_version = EXCLUDED.hash_version,
			updated_at = NOW()
	`, userID, passwordHash)
	return err
}

func (s *Store) CreateEmailChallenge(
	ctx context.Context,
	challengeID, userID, email, purpose, codeHash string,
	expiresAt time.Time,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_email_challenges
			(id, user_id, email, purpose, code_hash, expires_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6)
	`, challengeID, userID, strings.ToLower(strings.TrimSpace(email)), purpose, codeHash, expiresAt)
	return err
}

func (s *Store) CreateEmailChallengeIfAllowed(
	ctx context.Context,
	challengeID, userID, email, purpose, codeHash string,
	expiresAt, since time.Time,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Serialize challenge creation for an address and purpose so two
	// concurrent resend requests cannot both pass the recent-send check.
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended(lower($1) || ':' || $2, 0))
	`, email, purpose); err != nil {
		return err
	}
	var recent bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth_email_challenges
			WHERE lower(email) = lower($1)
			  AND purpose = $2
			  AND created_at >= $3
		)
	`, strings.TrimSpace(email), purpose, since).Scan(&recent); err != nil {
		return err
	}
	if recent {
		return ErrChallengeRecentlySent
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_email_challenges
			(id, user_id, email, purpose, code_hash, expires_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6)
	`, challengeID, userID, strings.ToLower(strings.TrimSpace(email)), purpose, codeHash, expiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteEmailChallenge(ctx context.Context, challengeID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
DELETE FROM auth_email_challenges
WHERE id = $1
`, challengeID)
	return err
}

// CleanupEmailChallenges deletes only terminal email challenges whose terminal
// timestamp is older than EmailChallengeRetention. Active challenges,
// recently expired challenges, recently consumed challenges, and other
// challenge purposes are preserved. Candidates are locked with SKIP LOCKED so
// cleanup cannot wait on or delete a row being consumed by another transaction.
//
// The caller supplies now to keep the cutoff deterministic in tests.
func (s *Store) CleanupEmailChallenges(ctx context.Context, now time.Time, limit int) (int, error) {
	if s == nil {
		return 0, errors.New("database is not configured")
	}
	if limit <= 0 {
		return 0, errors.New("email challenge cleanup limit must be positive")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	cutoff := now.Add(-EmailChallengeRetention)
	result, err := tx.Exec(ctx, `
		WITH candidates AS (
			SELECT id
			FROM auth_email_challenges
			WHERE purpose IN (
				'email_verification',
				'recovery_email_enrollment',
				'password_recovery'
			)
			  AND COALESCE(consumed_at, expires_at) < $1
			ORDER BY COALESCE(consumed_at, expires_at), id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		DELETE FROM auth_email_challenges AS challenges
		USING candidates
		WHERE challenges.id = candidates.id
	`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

func (s *Store) HasRecentEmailChallenge(
	ctx context.Context, email, purpose string, since time.Time,
) (bool, error) {
	if s == nil {
		return false, errors.New("database is not configured")
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth_email_challenges
			WHERE lower(email) = lower($1)
			  AND purpose = $2
			  AND created_at >= $3
		)
	`, strings.TrimSpace(email), purpose, since).Scan(&exists)
	return exists, err
}

func (s *Store) ConsumeEmailChallenge(
	ctx context.Context, email, purpose, codeHash string, maxAttempts int,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var challengeID, userID, storedHash string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id, COALESCE(user_id, ''), code_hash, attempt_count
		FROM auth_email_challenges
		WHERE lower(email) = lower($1)
		  AND purpose = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, strings.TrimSpace(email), purpose).Scan(&challengeID, &userID, &storedHash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrChallengeInvalid
	}
	if err != nil {
		return "", err
	}
	if attempts >= maxAttempts {
		return "", ErrChallengeLocked
	}

	attempts++
	if _, err := tx.Exec(ctx, `
		UPDATE auth_email_challenges
		SET attempt_count = $2
		WHERE id = $1
	`, challengeID, attempts); err != nil {
		return "", err
	}
	if !secureStringEqual(storedHash, codeHash) {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrChallengeInvalid
	}
	if _, err := tx.Exec(ctx, `
		UPDATE auth_email_challenges
		SET consumed_at = NOW()
		WHERE id = $1
	`, challengeID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) VerifyEmailChallenge(
	ctx context.Context, email, purpose, codeHash string, maxAttempts int,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var challengeID, userID, storedHash string
	var attempts int
	err = tx.QueryRow(ctx, `
		SELECT id, COALESCE(user_id, ''), code_hash, attempt_count
		FROM auth_email_challenges
		WHERE lower(email) = lower($1)
		  AND purpose = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, strings.TrimSpace(email), purpose).Scan(&challengeID, &userID, &storedHash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrChallengeInvalid
	}
	if err != nil {
		return "", err
	}
	if attempts >= maxAttempts {
		return "", ErrChallengeLocked
	}

	attempts++
	if _, err := tx.Exec(ctx, `
		UPDATE auth_email_challenges
		SET attempt_count = $2
		WHERE id = $1
	`, challengeID, attempts); err != nil {
		return "", err
	}
	if !secureStringEqual(storedHash, codeHash) {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrChallengeInvalid
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, NOW()),
		    status = CASE
		        WHEN status = 'pending_email_verification' THEN 'active'
		        ELSE status
		    END,
		    updated_at = NOW()
		WHERE id = $1
	`, userID)
	return err
}

func (s *Store) UpsertVerifiedRecoveryEmail(ctx context.Context, userID, email string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	methodID, err := id.New()
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO auth_recovery_methods
			(id, user_id, kind, address, verified_at, revoked_at)
		VALUES ($1, $2, 'email', $3, NOW(), NULL)
		ON CONFLICT (user_id, kind, address) DO UPDATE SET
			verified_at = NOW(),
			revoked_at = NULL,
			updated_at = NOW()
	`, methodID, userID, strings.ToLower(strings.TrimSpace(email)))
	return err
}

func (s *Store) VerifiedRecoveryEmail(ctx context.Context, userID string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var email string
	err := s.pool.QueryRow(ctx, `
		SELECT address
		FROM auth_recovery_methods
		WHERE user_id = $1
		  AND kind = 'email'
		  AND verified_at IS NOT NULL
		  AND revoked_at IS NULL
		ORDER BY verified_at DESC
		LIMIT 1
	`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrRecoveryUnavailable
	}
	return email, err
}

func (s *Store) FindUserByRecoveryEmail(ctx context.Context, email string) (User, error) {
	if s == nil {
		return User{}, errors.New("database is not configured")
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, COALESCE(u.email, ''), COALESCE(u.first_name, ''),
		       COALESCE(u.last_name, ''), COALESCE(u.profile_image_url, ''),
		       COALESCE(u.status, 'active'), u.email_verified_at,
		       COALESCE(u.account_created_via, '')
		FROM users u
		INNER JOIN auth_recovery_methods m ON m.user_id = u.id
		WHERE lower(m.address) = lower($1)
		  AND m.kind = 'email'
		  AND m.verified_at IS NOT NULL
		  AND m.revoked_at IS NULL
		ORDER BY m.verified_at DESC
		LIMIT 1
	`, strings.TrimSpace(email)).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName,
		&user.ProfileImageURL, &user.Status, &user.EmailVerifiedAt, &user.AccountCreatedVia,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) ActivateProviderUserIfReady(ctx context.Context, userID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET status = 'active', updated_at = NOW()
		WHERE id = $1
		  AND status = 'pending_provider_onboarding'
		  AND email_verified_at IS NOT NULL
		  AND EXISTS (
		      SELECT 1 FROM auth_passwords p WHERE p.user_id = users.id
		  )
		  AND EXISTS (
		      SELECT 1
		      FROM auth_recovery_methods m
		      WHERE m.user_id = users.id
		        AND m.kind = 'email'
		        AND m.verified_at IS NOT NULL
		        AND m.revoked_at IS NULL
		  )
	`, userID)
	return err
}

func (s *Store) CreateSecurityEvent(
	ctx context.Context, userID, eventType, requestID string, metadata map[string]any,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	eventID, err := id.New()
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO auth_security_events
			(id, user_id, event_type, request_id, metadata)
		VALUES ($1, NULLIF($2, ''), $3, NULLIF($4, ''), $5::jsonb)
	`, eventID, userID, eventType, requestID, encoded)
	return err
}

// MFAEventSummary returns aggregate MFA security-event counts since since.
// Keep the event types in this query explicit so unrelated authentication
// events cannot unexpectedly change the signal.
func (s *Store) MFAEventSummary(ctx context.Context, since time.Time) (MFAEventSummary, error) {
	if s == nil {
		return MFAEventSummary{}, errors.New("database is not configured")
	}
	var summary MFAEventSummary
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type IN (
				'mfa_challenge_failed',
				'mfa_recovery_code_failed',
				'mfa_enrollment_failed',
				'mfa_disable_failed',
				'recovery_code_regeneration_failed'
			)),
			COUNT(*) FILTER (WHERE event_type = 'mfa_replay_rejected'),
			COUNT(*) FILTER (WHERE event_type = 'mfa_challenge_locked'),
			COUNT(*) FILTER (WHERE event_type = 'mfa_decryption_failed'),
COUNT(*) FILTER (WHERE event_type = 'mfa_recovery_support_challenge_sent'),
COUNT(*) FILTER (WHERE event_type = 'mfa_recovery_support_verification_failed'),
COUNT(*) FILTER (WHERE event_type = 'mfa_recovery_support_rate_limited'),
COUNT(*) FILTER (WHERE event_type = 'mfa_recovery_support_session_revocation_failed'),
			COUNT(DISTINCT user_id) FILTER (WHERE event_type IN (
				'mfa_challenge_failed',
				'mfa_recovery_code_failed',
				'mfa_enrollment_failed',
				'mfa_disable_failed',
				'recovery_code_regeneration_failed',
				'mfa_replay_rejected',
				'mfa_challenge_locked',
'mfa_decryption_failed',
'mfa_recovery_support_challenge_sent',
'mfa_recovery_support_verification_failed',
'mfa_recovery_support_rate_limited',
'mfa_recovery_support_session_revocation_failed'
			))
		FROM auth_security_events
		WHERE created_at >= $1
		  AND event_type IN (
			'mfa_challenge_failed',
			'mfa_recovery_code_failed',
			'mfa_enrollment_failed',
			'mfa_disable_failed',
			'recovery_code_regeneration_failed',
			'mfa_replay_rejected',
			'mfa_challenge_locked',
'mfa_decryption_failed',
'mfa_recovery_support_challenge_sent',
'mfa_recovery_support_verification_failed',
'mfa_recovery_support_rate_limited',
'mfa_recovery_support_session_revocation_failed'
		  )
	`, since).Scan(
		&summary.FailureEvents,
		&summary.ReplayEvents,
		&summary.LockoutEvents,
		&summary.DecryptionFailureEvents,
		&summary.RecoverySupportRequests,
		&summary.RecoverySupportVerificationFailures,
		&summary.RecoverySupportRateLimited,
		&summary.RecoverySupportSessionRevocationFailures,
		&summary.AffectedUsers,
	)
	return summary, err
}

func secureStringEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (s *Store) PasswordHash(ctx context.Context, userID string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var passwordHash string
	err := s.pool.QueryRow(ctx, `
		SELECT password_hash FROM auth_passwords WHERE user_id = $1
	`, userID).Scan(&passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return passwordHash, err
}

func (s *Store) CreateProviderSignupUser(
	ctx context.Context,
	provider, externalSubject, email, firstName, lastName, imageURL string,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, email).Scan(&existingID)
	if err == nil {
		return "", ErrEmailExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	userID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO users
			(id, email, first_name, last_name, profile_image_url, status, email_verified_at, account_created_via)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
		        'pending_provider_onboarding', NOW(), $6)
	`, userID, strings.ToLower(strings.TrimSpace(email)), firstName, lastName, imageURL, provider)
	if err != nil {
		return "", err
	}
	accountID, err := id.New()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO provider_accounts
			(id, user_id, provider, external_subject, email, display_name, avatar_url,
			 login_enabled, email_verified, linked_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
		        TRUE, TRUE, NOW())
	`, accountID, userID, provider, externalSubject, email,
		strings.TrimSpace(strings.TrimSpace(firstName)+" "+strings.TrimSpace(lastName)), imageURL)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) LinkProviderIdentity(
	ctx context.Context,
	userID, provider, externalSubject, email, displayName, imageURL string,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var existingUserID string
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM provider_accounts
		WHERE provider = $1 AND external_subject = $2
		FOR UPDATE
	`, provider, externalSubject).Scan(&existingUserID)
	if err == nil {
		if existingUserID != userID {
			return ErrOwnership
		}
		_, err = tx.Exec(ctx, `
			UPDATE provider_accounts
			SET email = NULLIF($3, ''), display_name = NULLIF($4, ''),
			    avatar_url = NULLIF($5, ''), login_enabled = TRUE,
			    email_verified = TRUE, linked_at = COALESCE(linked_at, NOW()),
			    updated_at = NOW()
			WHERE provider = $1 AND external_subject = $2 AND user_id = $6
		`, provider, externalSubject, email, displayName, imageURL, userID)
	} else if errors.Is(err, pgx.ErrNoRows) {
		accountID, idErr := id.New()
		if idErr != nil {
			return idErr
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO provider_accounts
				(id, user_id, provider, external_subject, email, display_name, avatar_url,
				 login_enabled, email_verified, linked_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
			        TRUE, TRUE, NOW())
		`, accountID, userID, provider, externalSubject, email, displayName, imageURL)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpsertGoogleConnection(
	ctx context.Context,
	userID string,
	email string,
	externalSubject string,
	displayName string,
	avatarURL string,
	scopes []string,
	capabilities []string,
	accessToken string,
	refreshToken string,
	expiresAt time.Time,
	tokenType string,
	encryptionKey []byte,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	if len(encryptionKey) != 32 {
		return "", errors.New("Google token encryption is not configured")
	}
	encryptedAccess, err := crypto.Seal(encryptionKey, []byte(accessToken))
	if err != nil {
		return "", err
	}
	encryptedRefresh, err := crypto.Seal(encryptionKey, []byte(refreshToken))
	if err != nil {
		return "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var accountID, accountUserID string
	err = tx.QueryRow(ctx, `
		SELECT id, user_id FROM provider_accounts
		WHERE provider = 'google' AND external_subject = $1
	`, externalSubject).Scan(&accountID, &accountUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		accountID, err = id.New()
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO provider_accounts
				(id, user_id, provider, external_subject, email, display_name, avatar_url)
			VALUES ($1, $2, 'google', $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''))
		`, accountID, userID, externalSubject, email, displayName, avatarURL)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if accountUserID != userID {
		return "", ErrOwnership
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE provider_accounts
			SET email = NULLIF($2, ''), display_name = NULLIF($3, ''), avatar_url = NULLIF($4, ''), updated_at = NOW()
			WHERE id = $1
		`, accountID, email, displayName, avatarURL)
		if err != nil {
			return "", err
		}
	}

	var connectionID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM provider_connections
		WHERE user_id = $1 AND provider_account_id = $2
	`, userID, accountID).Scan(&connectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		connectionID, err = id.New()
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO provider_connections
				(id, user_id, provider_account_id, provider, status, granted_scopes, capabilities, revoked_at)
			VALUES ($1, $2, $3, 'google', 'active', $4, $5, NULL)
		`, connectionID, userID, accountID, scopes, capabilities)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE provider_connections
			SET status = 'active', granted_scopes = $2, capabilities = $3, revoked_at = NULL, updated_at = NOW()
			WHERE id = $1 AND user_id = $4
		`, connectionID, scopes, capabilities, userID)
		if err != nil {
			return "", err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO provider_credentials
			(connection_id, access_token_encrypted, refresh_token_encrypted, expires_at, token_type)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (connection_id) DO UPDATE SET
			access_token_encrypted = EXCLUDED.access_token_encrypted,
			refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
			expires_at = EXCLUDED.expires_at,
			token_type = EXCLUDED.token_type,
			updated_at = NOW()
	`, connectionID, encryptedAccess, encryptedRefresh, expiresAt, tokenType)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return connectionID, nil
}

func (s *Store) ListGoogleConnections(ctx context.Context, userID string) ([]ProviderConnection, error) {
	if s == nil {
		return nil, errors.New("database is not configured")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.user_id, c.provider_account_id, c.provider, c.status,
		       c.granted_scopes, c.capabilities, a.email, a.display_name,
		       a.external_subject, c.revoked_at, c.updated_at
		FROM provider_connections c
		JOIN provider_accounts a ON a.id = c.provider_account_id
		WHERE c.user_id = $1
		ORDER BY c.updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var connections []ProviderConnection
	for rows.Next() {
		var connection ProviderConnection
		if err := rows.Scan(
			&connection.ID, &connection.UserID, &connection.AccountID, &connection.Provider,
			&connection.Status, &connection.Scopes, &connection.Capabilities,
			&connection.Email, &connection.DisplayName, &connection.ExternalSubject,
			&connection.RevokedAt, &connection.UpdatedAt,
		); err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

func (s *Store) GetGoogleCredentials(ctx context.Context, userID, connectionID string, encryptionKey []byte) (ProviderCredentials, error) {
	if s == nil {
		return ProviderCredentials{}, errors.New("database is not configured")
	}
	var encryptedAccess, encryptedRefresh string
	var credentials ProviderCredentials
	err := s.pool.QueryRow(ctx, `
		SELECT c.id, pc.access_token_encrypted, pc.refresh_token_encrypted,
		       pc.expires_at, pc.token_type
		FROM provider_connections c
		JOIN provider_credentials pc ON pc.connection_id = c.id
		WHERE c.id = $1 AND c.user_id = $2 AND c.status = 'active'
	`, connectionID, userID).Scan(
		&credentials.ConnectionID, &encryptedAccess, &encryptedRefresh,
		&credentials.ExpiresAt, &credentials.TokenType,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderCredentials{}, ErrNotFound
	}
	if err != nil {
		return ProviderCredentials{}, err
	}
	access, err := crypto.Open(encryptionKey, encryptedAccess)
	if err != nil {
		return ProviderCredentials{}, err
	}
	refresh, err := crypto.Open(encryptionKey, encryptedRefresh)
	if err != nil {
		return ProviderCredentials{}, err
	}
	credentials.AccessToken = string(access)
	credentials.RefreshToken = string(refresh)
	return credentials, nil
}

func (s *Store) UpdateGoogleAccessToken(ctx context.Context, userID, connectionID string, accessToken string, expiresAt time.Time, encryptionKey []byte) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	encryptedAccess, err := crypto.Seal(encryptionKey, []byte(accessToken))
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE provider_credentials pc
		SET access_token_encrypted = $3, expires_at = $4, updated_at = NOW()
		FROM provider_connections c
		WHERE pc.connection_id = c.id AND pc.connection_id = $1 AND c.user_id = $2 AND c.status = 'active'
	`, connectionID, userID, encryptedAccess, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeGoogleConnection(ctx context.Context, userID, connectionID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE provider_connections
		SET status = 'revoked', revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, connectionID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM provider_credentials WHERE connection_id = $1`, connectionID)
	return err
}

func (s *Store) UpdateGoogleCapabilities(ctx context.Context, userID, connectionID string, capabilities []string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE provider_connections
		SET capabilities = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status = 'active'
	`, connectionID, userID, capabilities)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetProviderDefault(ctx context.Context, userID, service, capability, connectionID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE provider_connections
		SET updated_at = updated_at
		WHERE id = $1 AND user_id = $2 AND status = 'active' AND $3 = ANY(capabilities)
	`, connectionID, userID, capability)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO provider_defaults (user_id, service, connection_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, service) DO UPDATE SET connection_id = EXCLUDED.connection_id, updated_at = NOW()
	`, userID, service, connectionID)
	return err
}

func (s *Store) ListProviderDefaults(ctx context.Context, userID string) ([]ProviderDefault, error) {
	if s == nil {
		return nil, errors.New("database is not configured")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT service, connection_id
		FROM provider_defaults
		WHERE user_id = $1
		ORDER BY service
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var defaults []ProviderDefault
	for rows.Next() {
		var item ProviderDefault
		if err := rows.Scan(&item.Service, &item.ConnectionID); err != nil {
			return nil, err
		}
		defaults = append(defaults, item)
	}
	return defaults, rows.Err()
}

func (s *Store) RevokeAllGoogleConnections(ctx context.Context, userID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		UPDATE provider_connections
		SET status = 'revoked', revoked_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND provider = 'google' AND status <> 'revoked'
	`, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM provider_credentials
		WHERE connection_id IN (SELECT id FROM provider_connections WHERE user_id = $1 AND provider = 'google')
	`, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM provider_defaults WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
