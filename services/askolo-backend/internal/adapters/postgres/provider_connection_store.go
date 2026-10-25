package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"askolo/backend/internal/platform/crypto"
	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

// UpsertProviderConnection persists provider credentials against the selected
// provider account. Provider is always part of each lookup and write so an
// account or credential from another provider cannot be reused accidentally.
func (s *Store) UpsertProviderConnection(
	ctx context.Context,
	provider, userID, email, externalSubject, displayName, avatarURL string,
	scopes, capabilities []string,
	accessToken, refreshToken string,
	expiresAt time.Time,
	tokenType string,
	encryptionKey []byte,
) (string, error) {
	if s == nil {
		return "", errors.New("database is not configured")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(externalSubject) == "" {
		return "", errors.New("provider connection identity is incomplete")
	}
	if len(encryptionKey) != 32 {
		return "", errors.New("provider token encryption is not configured")
	}
	if len(scopes) == 0 || strings.TrimSpace(accessToken) == "" {
		return "", errors.New("provider connection has no usable authorization")
	}

	encryptedAccess, err := crypto.Seal(encryptionKey, []byte(accessToken))
	if err != nil {
		return "", err
	}
	var encryptedRefresh string
	if refreshToken != "" {
		encryptedRefresh, err = crypto.Seal(encryptionKey, []byte(refreshToken))
		if err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(tokenType) == "" {
		tokenType = "Bearer"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var accountID, accountUserID string
	err = tx.QueryRow(ctx, `
SELECT id, user_id FROM provider_accounts
WHERE provider = $1 AND external_subject = $2
`, provider, externalSubject).Scan(&accountID, &accountUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		accountID, err = id.New()
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `
INSERT INTO provider_accounts
(id, user_id, provider, external_subject, email, display_name, avatar_url)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''))
`, accountID, userID, provider, externalSubject, email, displayName, avatarURL)
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
SET email = NULLIF($2, ''), display_name = NULLIF($3, ''),
    avatar_url = NULLIF($4, ''), updated_at = NOW()
WHERE id = $1 AND user_id = $5 AND provider = $6
`, accountID, email, displayName, avatarURL, userID, provider)
		if err != nil {
			return "", err
		}
	}

	var connectionID string
	var existingScopes, existingCapabilities []string
	err = tx.QueryRow(ctx, `
SELECT id, granted_scopes, capabilities FROM provider_connections
WHERE user_id = $1 AND provider_account_id = $2 AND provider = $3
FOR UPDATE
`, userID, accountID, provider).Scan(&connectionID, &existingScopes, &existingCapabilities)
	if errors.Is(err, pgx.ErrNoRows) {
		if refreshToken == "" {
			return "", errors.New("provider did not issue a refresh token for a new connection")
		}
		connectionID, err = id.New()
		if err != nil {
			return "", err
		}
		_, err = tx.Exec(ctx, `
INSERT INTO provider_connections
(id, user_id, provider_account_id, provider, status, granted_scopes, capabilities, revoked_at)
VALUES ($1, $2, $3, $4, 'active', $5, $6, NULL)
`, connectionID, userID, accountID, provider, scopes, capabilities)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		scopes = mergeProviderStringSets(existingScopes, scopes)
		capabilities = mergeProviderStringSets(existingCapabilities, capabilities)
		_, err = tx.Exec(ctx, `
UPDATE provider_connections
SET status = 'active', granted_scopes = $2, capabilities = $3,
    revoked_at = NULL, updated_at = NOW()
WHERE id = $1 AND user_id = $4 AND provider = $5
`, connectionID, scopes, capabilities, userID, provider)
		if err != nil {
			return "", err
		}
	}

	if refreshToken == "" {
		err = tx.QueryRow(ctx, `
SELECT refresh_token_encrypted
FROM provider_credentials
WHERE connection_id = $1
FOR UPDATE
`, connectionID).Scan(&encryptedRefresh)
		if err != nil || encryptedRefresh == "" {
			return "", errors.New("provider did not issue a refresh token and no saved token is available")
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

func mergeProviderStringSets(existing, incoming []string) []string {
	merged := make([]string, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	for _, value := range append(append([]string(nil), existing...), incoming...) {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

func (s *Store) ListProviderConnections(ctx context.Context, provider, userID string) ([]ProviderConnection, error) {
	if s == nil {
		return nil, errors.New("database is not configured")
	}
	rows, err := s.pool.Query(ctx, `
SELECT c.id, c.user_id, c.provider_account_id, c.provider, c.status,
       c.granted_scopes, c.capabilities, a.email, a.display_name,
       a.external_subject, c.revoked_at, c.updated_at
FROM provider_connections c
JOIN provider_accounts a ON a.id = c.provider_account_id
WHERE c.user_id = $1 AND c.provider = $2
ORDER BY c.updated_at DESC
`, userID, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := make([]ProviderConnection, 0)
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

func (s *Store) GetProviderCredentials(
	ctx context.Context, provider, userID, connectionID string, encryptionKey []byte,
) (ProviderCredentials, error) {
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
WHERE c.id = $1 AND c.user_id = $2 AND c.provider = $3 AND c.status = 'active'
`, connectionID, userID, provider).Scan(
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

func (s *Store) RevokeProviderConnection(ctx context.Context, provider, userID, connectionID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
UPDATE provider_connections
SET status = 'revoked', revoked_at = NOW(), updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND provider = $3
`, connectionID, userID, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM provider_defaults
WHERE user_id = $1 AND connection_id = $2
`, userID, connectionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM provider_credentials WHERE connection_id = $1`, connectionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
