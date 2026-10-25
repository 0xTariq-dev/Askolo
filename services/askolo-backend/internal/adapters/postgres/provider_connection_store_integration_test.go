package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProviderConnectionCredentialsAndRevocationStayScoped(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run provider connection integration tests")
	}
	testdb.RequireDisposable(t, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping integration database: %v", err)
	}
	schema := fmt.Sprintf("askolo_provider_connections_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quoteProviderTestIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+quoteProviderTestIdentifier(schema)+" CASCADE")
		adminPool.Close()
	})
	if _, err := adminPool.Exec(ctx, providerConnectionTestSchemaSQL(schema)); err != nil {
		t.Fatalf("create provider connection tables: %v", err)
	}
	schemaURL := providerConnectionTestDatabaseURL(t, baseURL, schema)
	store, err := New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open schema-scoped provider store: %v", err)
	}
	t.Cleanup(store.Close)

	encryptionKey := []byte("01234567890123456789012345678901")
	credentials := map[string]ProviderCredentials{}
	for _, account := range []struct {
		subject string
		email   string
		access  string
		refresh string
	}{
		{"microsoft-subject-one", "one@example.com", "ms-access-one", "ms-refresh-one"},
		{"microsoft-subject-two", "two@example.com", "ms-access-two", "ms-refresh-two"},
	} {
		connectionID, err := store.UpsertProviderConnection(
			ctx, "microsoft", "user-one", account.email, account.subject, account.email, "",
			[]string{"openid", "User.Read", "offline_access"}, []string{"profile.read"},
			account.access, account.refresh, time.Now().Add(time.Hour), "Bearer", encryptionKey,
		)
		if err != nil {
			t.Fatalf("persist Microsoft account %s: %v", account.subject, err)
		}
		credentials[account.subject] = ProviderCredentials{
			ConnectionID: connectionID, AccessToken: account.access, RefreshToken: account.refresh,
		}
	}
	googleConnectionID, err := store.UpsertProviderConnection(
		ctx, "google", "user-one", "google@example.com", "google-subject", "Google account", "",
		[]string{"openid", "email"}, []string{"profile.read"},
		"google-access", "google-refresh", time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("persist Google account fixture: %v", err)
	}

	connections, err := store.ListProviderConnections(ctx, "microsoft", "user-one")
	if err != nil {
		t.Fatalf("list Microsoft accounts: %v", err)
	}
	if len(connections) != 2 {
		t.Fatalf("Microsoft account count = %d, want two independent accounts", len(connections))
	}
	for subject, want := range credentials {
		got, err := store.GetProviderCredentials(ctx, "microsoft", "user-one", want.ConnectionID, encryptionKey)
		if err != nil || got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
			t.Fatalf("credentials for %s = %#v, %v", subject, got, err)
		}
	}
	if _, err := store.GetProviderCredentials(ctx, "microsoft", "another-user", credentials["microsoft-subject-one"].ConnectionID, encryptionKey); err != ErrNotFound {
		t.Fatalf("another user's Microsoft lookup error = %v, want not found", err)
	}
	if _, err := store.GetProviderCredentials(ctx, "microsoft", "user-one", googleConnectionID, encryptionKey); err != ErrNotFound {
		t.Fatalf("Google connection looked up as Microsoft: %v", err)
	}
	if _, err := store.GetProviderCredentials(ctx, "google", "user-one", credentials["microsoft-subject-one"].ConnectionID, encryptionKey); err != ErrNotFound {
		t.Fatalf("Microsoft connection looked up as Google: %v", err)
	}

	firstID := credentials["microsoft-subject-one"].ConnectionID
	if err := store.RevokeProviderConnection(ctx, "google", "user-one", firstID); err != ErrNotFound {
		t.Fatalf("Google revocation targeted a Microsoft connection: %v", err)
	}
	if _, err := store.GetProviderCredentials(ctx, "microsoft", "user-one", firstID, encryptionKey); err != nil {
		t.Fatalf("cross-provider disconnect affected Microsoft credentials: %v", err)
	}
	if err := store.RevokeProviderConnection(ctx, "microsoft", "user-one", firstID); err != nil {
		t.Fatalf("disconnect first Microsoft account: %v", err)
	}
	if _, err := store.GetProviderCredentials(ctx, "microsoft", "user-one", firstID, encryptionKey); err != ErrNotFound {
		t.Fatalf("disconnected Microsoft credentials remain available: %v", err)
	}
	if _, err := store.GetProviderCredentials(
		ctx, "microsoft", "user-one", credentials["microsoft-subject-two"].ConnectionID, encryptionKey,
	); err != nil {
		t.Fatalf("disconnecting first account affected second account: %v", err)
	}
	if _, err := store.GetProviderCredentials(ctx, "google", "user-one", googleConnectionID, encryptionKey); err != nil {
		t.Fatalf("disconnecting Microsoft account affected Google credentials: %v", err)
	}
	connections, err = store.ListProviderConnections(ctx, "microsoft", "user-one")
	if err != nil {
		t.Fatalf("list Microsoft accounts after disconnect: %v", err)
	}
	revoked, active := 0, 0
	for _, connection := range connections {
		switch connection.Status {
		case "revoked":
			revoked++
		case "active":
			active++
		}
	}
	if len(connections) != 2 || revoked != 1 || active != 1 {
		t.Fatalf("Microsoft connection status after disconnect = %#v", connections)
	}
}

func quoteProviderTestIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func providerConnectionTestSchemaSQL(schema string) string {
	prefix := quoteProviderTestIdentifier(schema) + "."
	return fmt.Sprintf(`
CREATE TABLE %sprovider_accounts (
  id text PRIMARY KEY,
  user_id text NOT NULL,
  provider text NOT NULL,
  external_subject text NOT NULL,
  email text,
  display_name text,
  avatar_url text,
  updated_at timestamptz DEFAULT NOW() NOT NULL,
  UNIQUE (provider, external_subject)
);
CREATE TABLE %sprovider_connections (
  id text PRIMARY KEY,
  user_id text NOT NULL,
  provider_account_id text NOT NULL,
  provider text NOT NULL,
  status text DEFAULT 'active' NOT NULL,
  granted_scopes text[] DEFAULT '{}' NOT NULL,
  capabilities text[] DEFAULT '{}' NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz DEFAULT NOW() NOT NULL,
  updated_at timestamptz DEFAULT NOW() NOT NULL,
  UNIQUE (user_id, provider_account_id)
);
CREATE TABLE %sprovider_credentials (
  connection_id text PRIMARY KEY,
  access_token_encrypted text NOT NULL,
  refresh_token_encrypted text NOT NULL,
  expires_at timestamptz NOT NULL,
  token_type text DEFAULT 'Bearer' NOT NULL,
  created_at timestamptz DEFAULT NOW() NOT NULL,
  updated_at timestamptz DEFAULT NOW() NOT NULL
);
CREATE TABLE %sprovider_defaults (
  user_id text NOT NULL,
  service text NOT NULL,
  connection_id text NOT NULL,
  updated_at timestamptz DEFAULT NOW() NOT NULL,
  PRIMARY KEY (user_id, service)
);
`, prefix, prefix, prefix, prefix)
}

func providerConnectionTestDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return parsed.String()
}
