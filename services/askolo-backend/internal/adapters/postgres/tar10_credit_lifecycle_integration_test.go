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

	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TAR-10 deliberately uses the migration runner against a throw-away schema.
// It must never be pointed at an application database.
func openTAR10(t *testing.T) (context.Context, *pgxpool.Pool, *Store) {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if base == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run TAR-10 credit lifecycle integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	schema := fmt.Sprintf("askolo_tar10_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		admin.Close()
		t.Fatalf("create disposable schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		admin.Close()
	})
	schemaURL := tar10SchemaURL(t, base, schema)
	pool, err := pgxpool.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open disposable schema: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Run(ctx, pool); err != nil {
		t.Fatalf("apply migrations to disposable schema: %v", err)
	}
	store, err := New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open store on disposable schema: %v", err)
	}
	t.Cleanup(store.Close)
	return ctx, pool, store
}

func tar10SchemaURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse disposable database URL: %v", err)
	}
	q := u.Query()
	q.Set("options", "-c search_path="+schema)
	u.RawQuery = strings.ReplaceAll(q.Encode(), "+", "%20")
	return u.String()
}

func tar10User(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id,email,status) VALUES ($1,$2,'active')`, id, id+"@tar10.test"); err != nil {
		t.Fatalf("insert TAR-10 user: %v", err)
	}
}

func tar10Account(t *testing.T, ctx context.Context, pool *pgxpool.Pool, user string, granted int) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts (user_id,granted_credits) VALUES ($1,$2)`, user, granted); err != nil {
		t.Fatalf("insert TAR-10 account: %v", err)
	}
}

func TestTAR10CreditLifecycleIntegration(t *testing.T) {
	t.Run("same key reservation and claim has one fresh execution", func(t *testing.T) {
		ctx, pool, store := openTAR10(t)
		tar10User(t, ctx, pool, "same-key")
		tar10Account(t, ctx, pool, "same-key", 4)
		var wg sync.WaitGroup
		var mu sync.Mutex
		fresh, claims := 0, 0
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r, created, err := store.ReserveAICredits(ctx, fmt.Sprintf("same-%d", i), "same-key", "voice", "test", "realtime", "request-key", 2, 60)
				if err != nil {
					t.Errorf("reserve: %v", err)
					return
				}
				mu.Lock()
				if created {
					fresh++
				}
				mu.Unlock()
				if created || r.ID != "" {
					ok, err := store.ClaimAICreditReservation(ctx, r.ID, "same-key")
					if err != nil {
						t.Errorf("claim: %v", err)
					} else if ok {
						mu.Lock()
						claims++
						mu.Unlock()
					}
				}
			}(i)
		}
		wg.Wait()
		if fresh != 1 || claims != 1 {
			t.Fatalf("fresh reservations=%d claims=%d, want one each", fresh, claims)
		}
	})

	t.Run("distinct keys contend without overdraw", func(t *testing.T) {
		ctx, pool, store := openTAR10(t)
		tar10User(t, ctx, pool, "contention")
		tar10Account(t, ctx, pool, "contention", 3)
		var wg sync.WaitGroup
		var mu sync.Mutex
		successes := 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, ok, err := store.ReserveAICredits(ctx, fmt.Sprintf("contention-%d", i), "contention", "voice", "test", "realtime", fmt.Sprintf("key-%d", i), 2, 60)
				if err != nil {
					t.Errorf("reserve contention: %v", err)
				} else if ok {
					mu.Lock()
					successes++
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()
		if successes != 1 {
			t.Fatalf("successful distinct-key reservations=%d, want 1", successes)
		}
		var reserved int
		if err := pool.QueryRow(ctx, `SELECT reserved_credits FROM ai_credit_accounts WHERE user_id='contention'`).Scan(&reserved); err != nil {
			t.Fatal(err)
		}
		if reserved != 2 {
			t.Fatalf("reserved aggregate=%d, want 2", reserved)
		}
	})

	t.Run("reserve settle release expiry and provider failure preserve balance math", func(t *testing.T) {
		ctx, pool, store := openTAR10(t)
		tar10User(t, ctx, pool, "math")
		tar10Account(t, ctx, pool, "math", 10)
		r, ok, err := store.ReserveAICredits(ctx, "math-settle", "math", "voice", "test", "realtime", "math-key", 4, 60)
		if err != nil || !ok {
			t.Fatalf("reserve: %v, %t", err, ok)
		}
		if got, _ := store.AICreditBalance(ctx, "math"); got != 6 {
			t.Fatalf("balance after reserve=%d, want 6", got)
		}
		if ok, err := store.ClaimAICreditReservation(ctx, r.ID, "math"); err != nil || !ok {
			t.Fatalf("claim: %v, %t", err, ok)
		}
		if err := store.SettleAICreditReservation(ctx, r.ID, "math", "settle-key", 3); err != nil {
			t.Fatalf("settle: %v", err)
		}
		if got, _ := store.AICreditBalance(ctx, "math"); got != 7 {
			t.Fatalf("balance after settlement=%d, want 7", got)
		}
		r, ok, err = store.ReserveAICredits(ctx, "math-release", "math", "voice", "test", "realtime", "release-key", 2, 60)
		if err != nil || !ok {
			t.Fatalf("reserve for provider failure: %v, %t", err, ok)
		}
		if err := store.ReleaseAICreditReservation(ctx, r.ID, "math", "provider-failure"); err != nil {
			t.Fatalf("release provider failure: %v", err)
		}
		if got, _ := store.AICreditBalance(ctx, "math"); got != 7 {
			t.Fatalf("balance after release=%d, want 7", got)
		}
		r, ok, err = store.ReserveAICredits(ctx, "math-expire", "math", "voice", "test", "realtime", "expire-key", 2, 60)
		if err != nil || !ok {
			t.Fatalf("reserve for expiry: %v, %t", err, ok)
		}
		if _, err := pool.Exec(ctx, `UPDATE ai_credit_reservations SET expires_at=NOW()-interval '1 second' WHERE id=$1`, r.ID); err != nil {
			t.Fatal(err)
		}
		_, ok, err = store.ReserveAICredits(ctx, "math-after-expiry", "math", "voice", "test", "realtime", "after-expiry", 1, 60)
		if err != nil || !ok {
			t.Fatalf("lazy expiry reservation: %v, %t", err, ok)
		}
		var status string
		var reserved, events int
		if err := pool.QueryRow(ctx, `SELECT status FROM ai_credit_reservations WHERE id=$1`, r.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT reserved_credits FROM ai_credit_accounts WHERE user_id='math'`).Scan(&reserved); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_credit_reservation_events WHERE reservation_id=$1 AND event_type='expired'`, r.ID).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if status != "expired" || reserved != 1 || events != 1 {
			t.Fatalf("expiry status=%q reserved=%d events=%d, want expired/1/1", status, reserved, events)
		}
	})

	t.Run("refund bounds and idempotency", func(t *testing.T) {
		ctx, pool, store := openTAR10(t)
		tar10User(t, ctx, pool, "refund")
		tar10Account(t, ctx, pool, "refund", 8)
		r, _, _ := store.ReserveAICredits(ctx, "refund-r", "refund", "voice", "test", "realtime", "refund-reserve", 5, 60)
		_, _ = store.ClaimAICreditReservation(ctx, r.ID, "refund")
		if err := store.SettleAICreditReservation(ctx, r.ID, "refund", "refund-settle", 5); err != nil {
			t.Fatal(err)
		}
		if err := store.RefundAICreditReservation(ctx, r.ID, "refund", "refund", "refund-key", "duplicate settlement", 3); err != nil {
			t.Fatal(err)
		}
		if err := store.RefundAICreditReservation(ctx, r.ID, "refund", "refund", "refund-key", "duplicate settlement", 3); err != nil {
			t.Fatalf("idempotent refund: %v", err)
		}
		if err := store.RefundAICreditReservation(ctx, r.ID, "refund", "refund", "too-much", "excess refund", 3); err == nil {
			t.Fatal("refund beyond settled amount succeeded")
		}
	})

	t.Run("adjustment grant idempotency reversal and policy boundaries", func(t *testing.T) {
		ctx, pool, store := openTAR10(t)
		tar10User(t, ctx, pool, "actor")
		tar10User(t, ctx, pool, "target")
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", 5, "grant", "grant-key"); err != nil {
			t.Fatal(err)
		}
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", 5, "grant", "grant-key"); err != nil {
			t.Fatalf("grant idempotency: %v", err)
		}
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", 6, "grant", "grant-key"); err == nil {
			t.Fatal("grant idempotency conflict accepted")
		}
		var grantID int64
		if err := pool.QueryRow(ctx, `SELECT id FROM ai_credit_grants WHERE user_id='target'`).Scan(&grantID); err != nil {
			t.Fatal(err)
		}
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", -2, "debit", "debit-key"); err != nil {
			t.Fatal(err)
		}
		var adjustmentID int64
		if err := pool.QueryRow(ctx, `SELECT id FROM ai_credit_adjustments WHERE idempotency_key='debit-key'`).Scan(&adjustmentID); err != nil {
			t.Fatal(err)
		}
		if err := store.ReverseAICreditEntry(ctx, "target", "actor", adjustmentID, "reverse-key", "undo"); err != nil {
			t.Fatal(err)
		}
		if err := store.ReverseAICreditEntry(ctx, "target", "actor", adjustmentID, "reverse-key-2", "undo"); err == nil {
			t.Fatal("duplicate reversal succeeded")
		}
		if err := store.ReverseAICreditGrant(ctx, "target", "actor", grantID, "reverse-grant", "undo"); err != nil {
			t.Fatalf("grant reversal: %v", err)
		}
		if err := store.ReverseAICreditGrant(ctx, "target", "actor", grantID, "reverse-grant", "undo"); err != nil {
			t.Fatalf("idempotent grant reversal: %v", err)
		}
		if err := store.ReverseAICreditGrant(ctx, "target", "actor", grantID, "reverse-grant-2", "duplicate undo"); err == nil {
			t.Fatal("duplicate grant reversal succeeded")
		}
		policy, err := store.AICreditPolicy(ctx)
		if err != nil {
			t.Fatal(err)
		}
		policy.Version++
		policy.OverrunMarginPercent = 100
		if _, err := store.UpdateAICreditPolicy(ctx, policy.Version-1, policy, "actor"); err != nil {
			t.Fatalf("100%% margin policy: %v", err)
		}
		policy.Version++
		policy.OverrunMarginPercent = 101
		if _, err := store.UpdateAICreditPolicy(ctx, policy.Version-1, policy, "actor"); err == nil {
			t.Fatal("101%% margin accepted")
		}
		policy.Version = 3
		policy.OverrunMarginPercent = 0
		if _, err := store.UpdateAICreditPolicy(ctx, 1, policy, "actor"); err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("stale policy expectedVersion error=%v", err)
		}
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", -100, "overdraw", "overdraw-key"); err == nil {
			t.Fatal("negative adjustment overdraw succeeded")
		}
	})
}
