package postgres

import (
	"context"
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

var ErrNotFound = errors.New("record not found")
var ErrOwnership = errors.New("record does not belong to user")

type Store struct {
	pool *pgxpool.Pool
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

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	sessionID, err := id.New()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"userId":    userID,
		"provider":  "google",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (sid, sess, expire) VALUES ($1, $2::jsonb, $3)
	`, sessionID, payload, time.Now().Add(ttl))
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Store) SessionUserID(ctx context.Context, sessionID string) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	var payload []byte
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `SELECT sess, expire FROM sessions WHERE sid = $1`, sessionID).
		Scan(&payload, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if expiresAt.Before(time.Now()) {
		_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE sid = $1`, sessionID)
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
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE sid = $1`, sessionID)
	return err
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
