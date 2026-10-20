package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/migrations"
	"askolo/backend/internal/testdb"
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
	testdb.RequireDisposable(t, base)
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
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`_public_v2" CASCADE`)
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
		policy.Version = 2
		policy.OverrunMarginPercent = 0
		if _, err := store.UpdateAICreditPolicy(ctx, 1, policy, "actor"); err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("stale policy expectedVersion error=%v", err)
		}
		if err := store.AddAICreditAdjustment(ctx, "target", "actor", -100, "overdraw", "overdraw-key"); err == nil {
			t.Fatal("negative adjustment overdraw succeeded")
		}
	})
}

func TestTAR10USDMicroLedgerLifecycleIntegration(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "usd-user")
	tar10User(t, ctx, pool, "usd-admin")
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id,granted_credits) VALUES('usd-user',3)`); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUSDAdjustment(ctx, "usd-user", "usd-admin", 2_000_000, "USD grant", "usd-grant-key"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUSDAdjustment(ctx, "usd-user", "usd-admin", 2_000_000, "USD grant", "usd-grant-key"); err != nil {
		t.Fatal(err)
	}
	r, created, err := store.ReserveUSD(ctx, "usd-reservation", "usd-user", "voice", "assemblyai", "recorded", "universal-3-5-pro", "usd-key", 1_000_000, 60, 2, json.RawMessage(`{"usdMicrosPerHour":210000}`))
	if err != nil || !created {
		t.Fatalf("reserve: %v created=%v", err, created)
	}
	if replay, replayed, err := store.ReserveUSD(ctx, "different-id", "usd-user", "voice", "assemblyai", "recorded", "universal-3-5-pro", "usd-key", 1_000_000, 60, 2, json.RawMessage(`{"usdMicrosPerHour":210000}`)); err != nil || replayed || replay.ID != r.ID {
		t.Fatalf("idempotent reserve replay: %#v created=%v err=%v", replay, replayed, err)
	}
	if _, _, err := store.ReserveUSD(ctx, "different-id", "usd-user", "voice", "assemblyai", "recorded", "universal-3-5-pro", "usd-key", 9_000_000, 60, 2, json.RawMessage(`{"usdMicrosPerHour":210000}`)); !errors.Is(err, ErrUSDReservationConflict) {
		t.Fatalf("mismatched replay error=%v", err)
	}
	if claimed, err := store.ClaimUSDReservation(ctx, r.ID, "usd-user"); err != nil || !claimed {
		t.Fatal(err)
	}
	if err := store.SettleUSDReservation(ctx, r.ID, "usd-user", "usd-settle", 600_000); err != nil {
		t.Fatal(err)
	}
	if err := store.RefundUSDReservation(ctx, r.ID, "usd-user", "usd-admin", "usd-refund", "provider correction", 100_000); err != nil {
		t.Fatal(err)
	}
	if err := store.RefundUSDReservation(ctx, r.ID, "usd-user", "usd-admin", "usd-refund", "provider correction", 100_000); err != nil {
		t.Fatal(err)
	}
	usage, err := store.USDUsage(ctx, "usd-user")
	if err != nil {
		t.Fatal(err)
	}
	if usage.GrantedUsdMicros != 2_000_000 || usage.SpentUsdMicros != 600_000 || usage.RefundedUsdMicros != 100_000 {
		t.Fatalf("unexpected USD usage: %#v", usage)
	}
	var legacy int
	if err := pool.QueryRow(ctx, `SELECT granted_credits FROM ai_credit_accounts WHERE user_id='usd-user'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 3 {
		t.Fatalf("legacy credits changed: %d", legacy)
	}
	recent, err := store.USDRecent(ctx, "usd-user", 20)
	if err != nil {
		t.Fatal(err)
	}
	reservations, ok := recent["reservations"].([]USDReservation)
	if !ok {
		t.Fatalf("USD recent reservations have unexpected type: %T", recent["reservations"])
	}
	foundReservation := false
	for _, recentReservation := range reservations {
		if recentReservation.ID != r.ID {
			continue
		}
		foundReservation = true
		if recentReservation.OperationType != "voice" || recentReservation.CreatedAt == nil {
			t.Fatalf("recent reservation activity fields = %#v, want operation and timestamp", recentReservation)
		}
	}
	if !foundReservation {
		t.Fatalf("recent reservations do not contain %q: %#v", r.ID, reservations)
	}
	for _, key := range []string{"reservations", "events", "adjustments", "grants"} {
		if _, ok := recent[key]; !ok {
			t.Fatalf("recent missing %s", key)
		}
	}
}

func TestTAR10EstimatedAssistantUsageReconcilesIdempotently(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "reconcile-user")
	tar10User(t, ctx, pool, "reconcile-admin")
	if err := store.AddUSDAdjustment(ctx, "reconcile-user", "reconcile-admin", 1_000_000, "test balance", "reconcile-balance"); err != nil {
		t.Fatal(err)
	}
	card := USDRateCard{
		Provider: "openai", Mode: "assistant", Model: "gpt-5.6-luna", Meter: "tokens",
		InputUsdMicrosPerMillion: 1_000_000, CachedInputUsdMicrosPerMillion: 500_000,
		OutputUsdMicrosPerMillion: 2_000_000,
	}
	snapshot, _ := json.Marshal(card)
	reservation, created, err := store.ReserveUSD(
		ctx, "assistant-estimated", "reconcile-user", "assistant", "openai", "assistant",
		card.Model, "assistant-estimated-key", 1_000, 60, 1, snapshot,
	)
	if err != nil || !created {
		t.Fatalf("reserve estimated assistant operation: created=%v err=%v", created, err)
	}
	if claimed, err := store.ClaimUSDReservation(ctx, reservation.ID, "reconcile-user"); err != nil || !claimed {
		t.Fatalf("claim assistant operation: claimed=%v err=%v", claimed, err)
	}
	estimatedPayload := json.RawMessage(`{"usageStatus":"estimated","cachedInputTokens":0}`)
	if err := store.RecordUSDEvidence(ctx, reservation.ID, "reconcile-user", "openai", "assistant", card.Model,
		reservation.ID+":usage", USDEvidence{
			UsageUnit: "tokens", InputTokens: 10, OutputTokens: 1, Payload: estimatedPayload,
		}); err != nil {
		t.Fatal(err)
	}
	estimatedCost, err := assistantUSDMicroCost(card, 10, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SettleUSDReservation(ctx, reservation.ID, "reconcile-user", reservation.ID+":settle", estimatedCost); err != nil {
		t.Fatal(err)
	}
	reconcile := func(input, cached, output int64) error {
		return store.ReconcileEstimatedAssistantUsage(
			ctx, reservation.ID, "reconcile-user", "reconcile-admin", "provider-usage-v1",
			"provider dashboard usage", "req-provider-1", input, cached, output,
		)
	}
	if err := reconcile(20, 4, 5); err != nil {
		t.Fatalf("reconcile estimated usage: %v", err)
	}
	actualCost, err := assistantUSDMicroCost(card, 20, 4, 5)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := store.USDUsage(ctx, "reconcile-user")
	if err != nil {
		t.Fatal(err)
	}
	if usage.SpentUsdMicros != actualCost {
		t.Fatalf("spent = %d; want reconciled actual %d", usage.SpentUsdMicros, actualCost)
	}
	if err := reconcile(20, 4, 5); err != nil {
		t.Fatalf("idempotent reconciliation replay: %v", err)
	}
	if err := reconcile(21, 4, 5); !errors.Is(err, ErrUSDReservationConflict) {
		t.Fatalf("changed reconciliation replay error = %v; want conflict", err)
	}
	receipt, err := store.USDReservationForUser(ctx, reservation.ID, "reconcile-user")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.UsageStatus != "measured" || receipt.SettledUsdMicros != actualCost ||
		receipt.InputTokens != 20 || receipt.CachedInputTokens != 4 || receipt.OutputTokens != 5 {
		t.Fatalf("reconciled receipt = %#v", receipt)
	}

	voiceCard := USDRateCard{
		Provider: "assemblyai", Mode: "realtime", Model: "universal-3-5-pro",
		Meter: "hour", UsdMicrosPerHour: 210_000,
	}
	voiceSnapshot, _ := json.Marshal(voiceCard)
	voice, created, err := store.ReserveUSD(
		ctx, "voice-estimated", "reconcile-user", "voice", "assemblyai", "realtime",
		voiceCard.Model, "voice-estimated-key", 20_000, 60, 1, voiceSnapshot,
	)
	if err != nil || !created {
		t.Fatalf("reserve estimated voice operation: created=%v err=%v", created, err)
	}
	if claimed, err := store.ClaimUSDReservation(ctx, voice.ID, "reconcile-user"); err != nil || !claimed {
		t.Fatalf("claim voice operation: claimed=%v err=%v", claimed, err)
	}
	if err := store.RecordUSDEvidence(ctx, voice.ID, "reconcile-user", "assemblyai", "realtime", voiceCard.Model,
		voice.ID+":usage", USDEvidence{
			UsageUnit: "milliseconds", UsageUnits: 5_000, DurationMs: 5_000,
			Payload: json.RawMessage(`{"usageStatus":"estimated","meterSource":"server_elapsed"}`),
		}); err != nil {
		t.Fatal(err)
	}
	estimatedVoiceCost, ok := ceilVoiceCost(voiceCard.UsdMicrosPerHour, 5_000)
	if !ok {
		t.Fatal("failed to calculate estimated voice cost")
	}
	if err := store.SettleUSDReservation(ctx, voice.ID, "reconcile-user", voice.ID+":settle", estimatedVoiceCost); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileEstimatedVoiceUsage(ctx, voice.ID, "reconcile-user", "reconcile-admin",
		"provider-voice-v1", "provider session duration", "req-voice-1", 10_000); err != nil {
		t.Fatalf("reconcile estimated voice usage: %v", err)
	}
	if err := store.ReconcileEstimatedVoiceUsage(ctx, voice.ID, "reconcile-user", "reconcile-admin",
		"provider-voice-v1", "provider session duration", "req-voice-1", 10_000); err != nil {
		t.Fatalf("idempotent voice reconciliation replay: %v", err)
	}
}

func TestTAR10UnknownProviderUsageCanBeReleased(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "unknown-usage-user")
	tar10User(t, ctx, pool, "unknown-usage-admin")
	if err := store.AddUSDAdjustment(ctx, "unknown-usage-user", "unknown-usage-admin", 100_000, "test balance", "unknown-usage-balance"); err != nil {
		t.Fatal(err)
	}
	r, created, err := store.ReserveUSD(
		ctx, "unknown-openai-reservation", "unknown-usage-user", "assistant", "openai", "assistant",
		"gpt-5.6-luna", "unknown-openai-key", 10_000, 60, 1, json.RawMessage(`{"meter":"tokens"}`),
	)
	if err != nil || !created {
		t.Fatalf("reserve unknown OpenAI usage: created=%v err=%v", created, err)
	}
	if claimed, err := store.ClaimUSDReservation(ctx, r.ID, "unknown-usage-user"); err != nil || !claimed {
		t.Fatalf("claim unknown usage: claimed=%v err=%v", claimed, err)
	}
	pending, err := store.USDReservationForUser(ctx, r.ID, "unknown-usage-user")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != "claimed" || pending.UsageStatus != "pending_review" || pending.UsageUnit != "" {
		t.Fatalf("unknown provider usage receipt = %#v; want a claimed pending-review reservation without usage evidence", pending)
	}
	resolve := func() error {
		return store.ResolvePendingUSDReservation(
			ctx, r.ID, "unknown-usage-user", "unknown-usage-admin", "unknown-release-v1",
			"provider usage could not be verified", "release", 0,
		)
	}
	if err := resolve(); !errors.Is(err, ErrUSDReservationReviewRequired) {
		t.Fatalf("release before reservation expiry = %v; want review-required", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_credit_reservations SET expires_at=NOW()-interval '1 second' WHERE id=$1
	`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := resolve(); err != nil {
		t.Fatalf("release unknown provider usage: %v", err)
	}
	if err := resolve(); err != nil {
		t.Fatalf("idempotent release replay: %v", err)
	}
	receipt, err := store.USDReservationForUser(ctx, r.ID, "unknown-usage-user")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "released" {
		t.Fatalf("reservation status = %q; want released", receipt.Status)
	}
	usage, err := store.USDUsage(ctx, "unknown-usage-user")
	if err != nil {
		t.Fatal(err)
	}
	if usage.BalanceUsdMicros != 100_000 {
		t.Fatalf("balance after releasing unknown usage = %d; want 100000", usage.BalanceUsdMicros)
	}
}

func TestTAR10MonthlyGrantExpiryUsesFEFOAndPreservesLegacyBalance(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "expiry-user")
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id,granted_usd_micros) VALUES('expiry-user',2_500_000)`); err != nil {
		t.Fatal(err)
	}
	var earlierID, laterID int64
	for _, item := range []struct {
		key    string
		amount int64
		expiry string
		id     *int64
	}{
		{key: "expiry-later", amount: 1_000_000, expiry: "3 days", id: &laterID},
		{key: "expiry-earlier", amount: 1_000_000, expiry: "1 day", id: &earlierID},
	} {
		if err := pool.QueryRow(ctx, `
			INSERT INTO ai_credit_grants(user_id,source_type,amount_credits,idempotency_key,currency,amount_usd_micros,expires_at)
			VALUES('expiry-user','monthly',1,$1,'USD',$2,NOW()+$3::interval) RETURNING id
		`, item.key, item.amount, item.expiry).Scan(item.id); err != nil {
			t.Fatal(err)
		}
	}
	r, created, err := store.ReserveUSD(ctx, "expiry-reservation", "expiry-user", "voice", "assemblyai", "recorded", "test-model", "expiry-reserve-key", 1_200_000, 60, 1, json.RawMessage(`{}`))
	if err != nil || !created {
		t.Fatalf("reserve: err=%v created=%v", err, created)
	}
	var earliestAllocated, laterAllocated int64
	if err := pool.QueryRow(ctx, `SELECT reserved_usd_micros FROM ai_credit_grant_allocations WHERE reservation_id=$1 AND grant_id=$2`, r.ID, earlierID).Scan(&earliestAllocated); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT reserved_usd_micros FROM ai_credit_grant_allocations WHERE reservation_id=$1 AND grant_id=$2`, r.ID, laterID).Scan(&laterAllocated); err != nil {
		t.Fatal(err)
	}
	if earliestAllocated != 1_000_000 || laterAllocated != 200_000 {
		t.Fatalf("FEFO allocations = earlier:%d later:%d; want 1000000 and 200000", earliestAllocated, laterAllocated)
	}
	if claimed, err := store.ClaimUSDReservation(ctx, r.ID, "expiry-user"); err != nil || !claimed {
		t.Fatalf("claim: err=%v claimed=%v", err, claimed)
	}
	if err := store.SettleUSDReservation(ctx, r.ID, "expiry-user", "expiry-settle-key", 500_000); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_credit_grants SET expires_at=NOW()-interval '1 second' WHERE user_id='expiry-user' AND expires_at IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	usage, err := store.USDUsage(ctx, "expiry-user")
	if err != nil {
		t.Fatal(err)
	}
	if usage.BalanceUsdMicros != 500_000 {
		t.Fatalf("balance after expiring new grants=%d, want the untouched 500000 legacy USD micros", usage.BalanceUsdMicros)
	}
}

func TestTAR10SignupWelcomeUSDGrantIsIdempotent(t *testing.T) {
	ctx, pool, _ := openTAR10(t)
	tar10User(t, ctx, pool, "welcome-user")
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_credit_accounts(user_id,granted_usd_micros)
		VALUES('welcome-user',1250000)
	`); err != nil {
		t.Fatalf("create test credit account: %v", err)
	}
	disabledTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := grantSignupWelcomeUSDTx(ctx, disabledTx, "welcome-user"); err != nil {
		_ = disabledTx.Rollback(ctx)
		t.Fatalf("unconfigured signup grant: %v", err)
	}
	if err := disabledTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var grants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_credit_grants WHERE user_id='welcome-user' AND idempotency_key='welcome-credit-usd-v1'`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("unconfigured signup policy issued %d grants, want none", grants)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_credit_policy_versions
SET signup_grant_usd_micros=5000000,
    grant_type_policies=jsonb_set(
      jsonb_set(grant_type_policies,'{signup,issuanceCapUsdMicros}',to_jsonb(5000000::bigint),true),
      '{signup,carryoverCapUsdMicros}',to_jsonb(10000000::bigint),true)
		WHERE version=(SELECT max(version) FROM ai_credit_policy_versions)
	`); err != nil {
		t.Fatalf("configure test signup grant: %v", err)
	}
	const attempts = 12
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(ctx)
			if err := grantSignupWelcomeUSDTx(ctx, tx, "welcome-user"); err != nil {
				errs <- err
				return
			}
			errs <- tx.Commit(ctx)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent welcome grant: %v", err)
		}
	}

	var balance int64
	if err := pool.QueryRow(ctx, `
		SELECT granted_usd_micros FROM ai_credit_accounts WHERE user_id='welcome-user'
	`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 6_250_000 {
		t.Fatalf("welcome account balance = %d USD micros, want existing 1250000 plus exactly one 5000000 grant", balance)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM ai_credit_grants
		WHERE user_id='welcome-user' AND idempotency_key='welcome-credit-usd-v1'
	`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 1 {
		t.Fatalf("welcome grant row count = %d, want exactly 1", grants)
	}
}

func TestTAR10UserGrantExpiryOverrideAppliesToManualAndMonthlyGrants(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "expiry-override-user")
	tar10User(t, ctx, pool, "expiry-override-admin")

	days := 7
	if err := store.UpdateUSDUserPolicyOverride(ctx, USDUserPolicyOverride{
		UserID:          "expiry-override-user",
		GrantExpiryDays: &days,
		Reason:          "shorter support grant expiry",
	}, "expiry-override-admin"); err != nil {
		t.Fatalf("save expiry override: %v", err)
	}
	override, err := store.USDUserPolicyOverride(ctx, "expiry-override-user")
	if err != nil || override.GrantExpiryDays == nil || *override.GrantExpiryDays != days {
		t.Fatalf("expiry override = %#v, err=%v; want %d days", override, err, days)
	}

	if err := store.AddUSDAdjustment(ctx, "expiry-override-user", "expiry-override-admin", 2_000_000, "manual grant", "expiry-override-manual"); err != nil {
		t.Fatalf("issue manual grant: %v", err)
	}
	var manualExpiry time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM ai_credit_grants WHERE user_id=$1 AND idempotency_key=$2`, "expiry-override-user", "expiry-override-manual").Scan(&manualExpiry); err != nil {
		t.Fatalf("read manual grant expiry: %v", err)
	}
	if remaining := time.Until(manualExpiry); remaining < 6*24*time.Hour || remaining > 8*24*time.Hour {
		t.Fatalf("manual grant expires in %s, want about 7 days", remaining)
	}

	policy, err := store.USDPolicy(ctx)
	if err != nil {
		t.Fatalf("read grant policy: %v", err)
	}
	policy.Version++
	policy.ChangeReason = "configure isolated recurring grant test"
	policy.MonthlyGrantsEnabled = true
	policy.MonthlyGrantUsdMicros = 1_000_000
	policy.RolloverCapUsdMicros = 10_000_000
	policy.GrantTypePolicies["monthly"] = USDGrantTypePolicy{
		IssuanceCapUsdMicros:  1_000_000,
		CarryoverCapUsdMicros: 10_000_000,
		ExpiryDays:            policy.GrantTypePolicies["monthly"].ExpiryDays,
	}
	if _, err := store.UpdateUSDPolicy(ctx, policy.Version-1, policy, "expiry-override-admin"); err != nil {
		t.Fatalf("configure recurring grant test: %v", err)
	}
	due := time.Now().UTC().Add(-time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_credit_monthly_grant_schedules(user_id,next_cycle_at)
		VALUES($1,$2)
	`, "expiry-override-user", due); err != nil {
		t.Fatalf("schedule recurring grant: %v", err)
	}
	batch, err := store.ProcessDueMonthlyUSDGrants(ctx, time.Now().UTC(), 10)
	if err != nil || batch.Issued != 1 {
		t.Fatalf("process monthly grant: batch=%#v err=%v; want one issued", batch, err)
	}
	var monthlyExpiry time.Time
	if err := pool.QueryRow(ctx, `
		SELECT g.expires_at FROM ai_credit_grants g
		WHERE g.user_id=$1 AND g.source_type='monthly'
	`, "expiry-override-user").Scan(&monthlyExpiry); err != nil {
		t.Fatalf("read monthly grant expiry: %v", err)
	}
	if remaining := time.Until(monthlyExpiry); remaining < 6*24*time.Hour || remaining > 8*24*time.Hour {
		t.Fatalf("monthly grant expires in %s, want about 7 days", remaining)
	}
}

func TestTAR10BulkGrantPreviewAndAtomicIdempotentIssuance(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "bulk-user-a")
	tar10User(t, ctx, pool, "bulk-user-b")
	tar10User(t, ctx, pool, "bulk-admin")
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id,granted_usd_micros) VALUES('bulk-user-a',500000),('bulk-user-b',1000000)`); err != nil {
		t.Fatal(err)
	}
	days := 14
	if err := store.UpdateUSDUserPolicyOverride(ctx, USDUserPolicyOverride{
		UserID: "bulk-user-a", GrantExpiryDays: &days, Reason: "bulk preview expiry",
	}, "bulk-admin"); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewUSDManualGrant(ctx, "bulk-user-a", 250000, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if preview.BalanceUsdMicros != 500000 || preview.ProjectedBalance != 750000 || preview.GrantExpiryDays != days {
		t.Fatalf("manual preview = %#v", preview)
	}
	items := []USDGrantBatchItem{
		{UserID: "bulk-user-a", AmountUsdMicros: 250000},
		{UserID: "bulk-user-b", AmountUsdMicros: 250000},
	}
	if err := store.AddUSDGrantBatch(ctx, items, "bulk-admin", "support batch", "bulk-key-1"); err != nil {
		t.Fatalf("issue batch: %v", err)
	}
	if err := store.AddUSDGrantBatch(ctx, items, "bulk-admin", "support batch", "bulk-key-1"); err != nil {
		t.Fatalf("idempotent batch replay: %v", err)
	}
	for _, user := range []struct {
		id   string
		want int64
	}{{"bulk-user-a", 750000}, {"bulk-user-b", 1250000}} {
		usage, err := store.USDUsage(ctx, user.id)
		if err != nil || usage.BalanceUsdMicros != user.want {
			t.Fatalf("%s usage = %#v err=%v, want balance %d", user.id, usage, err, user.want)
		}
	}
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM ai_credit_grants WHERE user_id='bulk-user-a' AND reason='support batch'`).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(expiresAt); remaining < 13*24*time.Hour || remaining > 15*24*time.Hour {
		t.Fatalf("bulk grant expires in %s, want about 14 days", remaining)
	}
	conflict := []USDGrantBatchItem{{UserID: "bulk-user-a", AmountUsdMicros: 300000}, {UserID: "bulk-user-b", AmountUsdMicros: 250000}}
	if err := store.AddUSDGrantBatch(ctx, conflict, "bulk-admin", "support batch", "bulk-key-1"); !errors.Is(err, ErrUSDReservationConflict) {
		t.Fatalf("mismatched batch replay error = %v, want idempotency conflict", err)
	}
}

func TestTAR10AdminGrantTypeCapsAndExpiryAreEnforced(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	tar10User(t, ctx, pool, "capped-admin-user")
	tar10User(t, ctx, pool, "capped-admin-actor")
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id,granted_usd_micros) VALUES('capped-admin-user',100000)`); err != nil {
		t.Fatal(err)
	}
	policy, err := store.USDPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy.Version++
	policy.ChangeReason = "test admin grant caps"
	policy.GrantTypePolicies["admin"] = USDGrantTypePolicy{
		IssuanceCapUsdMicros: 200000, CarryoverCapUsdMicros: 300000, ExpiryDays: 10,
	}
	if _, err := store.UpdateUSDPolicy(ctx, policy.Version-1, policy, "capped-admin-actor"); err != nil {
		t.Fatalf("update grant policy: %v", err)
	}
	if err := store.AddUSDAdjustment(ctx, "capped-admin-user", "capped-admin-actor", 200001, "over issue cap", "capped-key-1"); err == nil {
		t.Fatal("grant above issuance cap succeeded")
	}
	if err := store.AddUSDAdjustment(ctx, "capped-admin-user", "capped-admin-actor", 200000, "within caps", "capped-key-2"); err != nil {
		t.Fatalf("grant at configured caps: %v", err)
	}
	// Idempotent replay remains valid after the balance reaches the carryover cap.
	if err := store.AddUSDAdjustment(ctx, "capped-admin-user", "capped-admin-actor", 200000, "within caps", "capped-key-2"); err != nil {
		t.Fatalf("idempotent grant replay: %v", err)
	}
	var expiry time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM ai_credit_grants WHERE user_id='capped-admin-user' AND idempotency_key='capped-key-2'`).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	if left := time.Until(expiry); left < 9*24*time.Hour || left > 11*24*time.Hour {
		t.Fatalf("grant expiry = %s from now, want about 10 days", left)
	}
}
