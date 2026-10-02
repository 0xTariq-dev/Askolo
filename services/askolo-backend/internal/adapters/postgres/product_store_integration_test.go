package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpdateUserProfilePersistsPreferredLocale(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run profile integration tests")
	}

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

	schema := fmt.Sprintf("askolo_profile_locale_%d", time.Now().UnixNano())
	quotedSchema := quoteAICreditIdentifier(schema)
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+quotedSchema+` CASCADE`)
		adminPool.Close()
	})

	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.users (
			id text PRIMARY KEY,
			email text,
			first_name text,
			last_name text,
			profile_image_url text,
			preferred_locale varchar(2)
				CHECK (
					preferred_locale IS NULL
					OR (preferred_locale COLLATE "C") ~ '^[a-z]{2}$'
				),
			status text,
			email_verified_at timestamptz,
			account_created_via text,
			updated_at timestamptz NOT NULL DEFAULT NOW()
		)
	`, quotedSchema)); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `INSERT INTO `+quotedSchema+`.users (id, email, status) VALUES ('locale-user', 'locale@example.test', 'active')`); err != nil {
		t.Fatalf("create profile user: %v", err)
	}

	store, err := New(ctx, aiCreditDatabaseURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("open profile store: %v", err)
	}
	t.Cleanup(store.Close)

	locale := "ar"
	updated, err := store.UpdateUserProfile(ctx, "locale-user", nil, nil, &locale)
	if err != nil {
		t.Fatalf("update preferred locale: %v", err)
	}
	if updated.PreferredLocale == nil || *updated.PreferredLocale != locale {
		t.Fatalf("updated preferred locale = %v, want %q", updated.PreferredLocale, locale)
	}

	reread, err := store.GetUser(ctx, "locale-user")
	if err != nil {
		t.Fatalf("read persisted preferred locale: %v", err)
	}
	if reread.PreferredLocale == nil || *reread.PreferredLocale != locale {
		t.Fatalf("persisted preferred locale = %v, want %q", reread.PreferredLocale, locale)
	}

	unchanged, err := store.UpdateUserProfile(ctx, "locale-user", nil, nil, nil)
	if err != nil {
		t.Fatalf("update profile without a locale: %v", err)
	}
	if unchanged.PreferredLocale == nil || *unchanged.PreferredLocale != locale {
		t.Fatalf("preferred locale after unrelated profile update = %v, want %q", unchanged.PreferredLocale, locale)
	}

	if _, err := adminPool.Exec(ctx, `UPDATE `+quotedSchema+`.users SET preferred_locale = 'fr' WHERE id = 'locale-user'`); err != nil {
		t.Fatalf("format-only database constraint rejected a future two-letter locale: %v", err)
	}
	for _, malformed := range []string{"EN", "e1", "en-US"} {
		if _, err := adminPool.Exec(ctx, `UPDATE `+quotedSchema+`.users SET preferred_locale = $1 WHERE id = 'locale-user'`, malformed); err == nil {
			t.Errorf("database accepted malformed preferred locale %q", malformed)
		}
	}
}

func TestSpendAICreditsEnforcesBalanceAtomically(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run AI credit integration tests")
	}

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

	schema := fmt.Sprintf("askolo_ai_credits_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+quoteAICreditIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+quoteAICreditIdentifier(schema)+` CASCADE`)
		adminPool.Close()
	})

	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.ai_credit_accounts (
			user_id text PRIMARY KEY,
			granted_credits integer NOT NULL DEFAULT 0,
			adjustment_credits integer NOT NULL DEFAULT 0,
			reserved_credits integer NOT NULL DEFAULT 0,
			spent_credits integer NOT NULL DEFAULT 0,
			refunded_credits integer NOT NULL DEFAULT 0
		)
	`, quoteAICreditIdentifier(schema))); err != nil {
		t.Fatalf("create AI credit table: %v", err)
	}

	schemaURL := aiCreditDatabaseURL(t, baseURL, schema)
	store, err := New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open AI credit store: %v", err)
	}
	t.Cleanup(store.Close)

	if balance, spent, err := store.SpendAICredits(ctx, "missing-user", 1); err != nil {
		t.Fatalf("spend for user without an account: %v", err)
	} else if spent || balance != 0 {
		t.Fatalf("missing account spend = (%d, %t), want (0, false)", balance, spent)
	}

	if _, err := adminPool.Exec(ctx, `INSERT INTO `+quoteAICreditIdentifier(schema)+`.ai_credit_accounts (user_id, granted_credits) VALUES ('concurrent-user', 3)`); err != nil {
		t.Fatalf("create AI credit account: %v", err)
	}

	const requests = 24
	var wg sync.WaitGroup
	var mu sync.Mutex
	spentCount := 0
	errorsFound := make([]error, 0)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			balance, spent, err := store.SpendAICredits(ctx, "concurrent-user", 1)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errorsFound = append(errorsFound, err)
				return
			}
			if spent {
				spentCount++
				if balance < 0 {
					errorsFound = append(errorsFound, fmt.Errorf("spend returned negative balance %d", balance))
				}
			}
		}()
	}
	wg.Wait()

	if len(errorsFound) > 0 {
		t.Fatalf("concurrent spends had errors: %v", errorsFound)
	}
	if spentCount != 3 {
		t.Fatalf("successful concurrent spends = %d, want exactly 3", spentCount)
	}
	balance, err := store.AICreditBalance(ctx, "concurrent-user")
	if err != nil {
		t.Fatalf("read remaining balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("remaining balance = %d, want 0", balance)
	}
}

func quoteAICreditIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func aiCreditDatabaseURL(t *testing.T, databaseURL, schema string) string {
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
