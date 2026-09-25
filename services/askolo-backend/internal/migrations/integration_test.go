package migrations

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

func TestRunDisposableDatabase(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()

	schema := fmt.Sprintf("askolo_migrations_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)

	testURL := migrationSchemaURL(t, baseURL, schema)
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()

	if err := Run(ctx, pool); err != nil {
		t.Fatalf("clean migration: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 3)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('sentinel', 'sentinel@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts
		(user_id, granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits)
		VALUES ('sentinel', 17, -2, 3, 4, 5)`); err != nil {
		t.Fatalf("insert sentinel account: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 3)
	var granted, adjustment, reserved, spent, refunded int
	if err := pool.QueryRow(ctx, `SELECT granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits
		FROM ai_credit_accounts WHERE user_id = 'sentinel'`).Scan(&granted, &adjustment, &reserved, &spent, &refunded); err != nil {
		t.Fatalf("read sentinel account: %v", err)
	}
	if granted != 17 || adjustment != -2 || reserved != 3 || spent != 4 || refunded != 5 {
		t.Fatalf("sentinel account changed to %d,%d,%d,%d,%d", granted, adjustment, reserved, spent, refunded)
	}

	if _, err := pool.Exec(ctx, `CREATE INDEX migration_drift_idx ON users (email)`); err != nil {
		t.Fatalf("induce index drift: %v", err)
	}
	if err := Run(ctx, pool); err == nil || !strings.Contains(err.Error(), "schema drift") {
		t.Fatalf("drift run error = %v, want schema drift", err)
	}
}

func TestRunRejectsTamperedHistoryBeforeWrites(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("askolo_migrations_tamper_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("initial migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE askolo_schema_migrations SET checksum = 'tampered' WHERE version = 1`); err != nil {
		t.Fatalf("tamper history: %v", err)
	}
	if err := Run(ctx, pool); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tampered run error = %v, want mismatch", err)
	}
}

func TestRunOneRollsBackFailedSQLAndCanRetry(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("askolo_migrations_failure_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TABLE askolo_schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(), schema_fingerprint text NOT NULL
	)`); err != nil {
		t.Fatalf("create migration history: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()
	failed := Migration{Version: 91, Name: "0091_failure", SQL: `CREATE TABLE rollback_marker (id integer); SELECT 1/0;`, SHA256: "failure"}
	if err := runOne(ctx, conn, failed); err == nil {
		t.Fatal("failed migration succeeded")
	}
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = 'rollback_marker')`).Scan(&exists); err != nil {
		t.Fatalf("check rollback marker: %v", err)
	}
	if exists {
		t.Fatal("failed migration left rollback_marker behind")
	}
	retry := Migration{Version: 91, Name: "0091_failure", SQL: `CREATE TABLE rollback_marker (id integer);`, SHA256: "retry"}
	if err := runOne(ctx, conn, retry); err != nil {
		t.Fatalf("retry migration: %v", err)
	}
}

func TestRunConcurrentCallsSerialize(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("askolo_migrations_lock_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
	testURL := migrationSchemaURL(t, baseURL, schema)
	first, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatalf("open first pool: %v", err)
	}
	defer first.Close()
	second, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatalf("open second pool: %v", err)
	}
	defer second.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pool := range []*pgxpool.Pool{first, second} {
		wg.Add(1)
		go func(pool *pgxpool.Pool) {
			defer wg.Done()
			errs <- Run(ctx, pool)
		}(pool)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migration: %v", err)
		}
	}
	assertMigrationVersions(t, ctx, first, 3)
}

func assertMigrationVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM askolo_schema_migrations`).Scan(&got); err != nil {
		t.Fatalf("count migration history: %v", err)
	}
	if got != want {
		t.Fatalf("migration history count = %d, want %d", got, want)
	}
}

func migrationSchemaURL(t *testing.T, base, schema string) string {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return parsed.String()
}

func quoteMigrationIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func TestLedgerMigrationAddsMissingCounterAndEnforcesContract(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("askolo_migrations_ledger_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TABLE askolo_schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(), schema_fingerprint text NOT NULL
	)`); err != nil {
		t.Fatalf("create migration history: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	for _, migration := range migrations[:2] {
		if err := runOne(ctx, conn, migration); err != nil {
			t.Fatalf("apply %s: %v", migration.Name, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('ledger-sentinel', 'ledger@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts
		(user_id, granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits)
		VALUES ('ledger-sentinel', 17, -2, 3, 4, 5)`); err != nil {
		t.Fatalf("insert sentinel account: %v", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE ai_credit_accounts DROP COLUMN spent_credits`); err != nil {
		t.Fatalf("remove counter for compatibility test: %v", err)
	}
	if err := runOne(ctx, conn, migrations[2]); err != nil {
		t.Fatalf("apply ledger contract migration: %v", err)
	}
	var granted, adjustment, reserved, spent, refunded int
	if err := pool.QueryRow(ctx, `SELECT granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits
		FROM ai_credit_accounts WHERE user_id = 'ledger-sentinel'`).Scan(&granted, &adjustment, &reserved, &spent, &refunded); err != nil {
		t.Fatalf("read sentinel account: %v", err)
	}
	if granted != 17 || adjustment != -2 || reserved != 3 || spent != 0 || refunded != 5 {
		t.Fatalf("counter migration changed existing values or failed to default the missing counter: got %d,%d,%d,%d,%d", granted, adjustment, reserved, spent, refunded)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_reservations
		(id, user_id, operation_type, provider, mode, status, idempotency_key, reserved_credits)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'voice', 'assemblyai', 'realtime', 'reserved', 'ledger-valid-key', 1)`); err != nil {
		t.Fatalf("insert valid reservation: %v", err)
	}

	rejects := func(label, statement string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, statement, args...); err == nil {
			t.Errorf("%s accepted an invalid ledger value", label)
		}
	}
	rejects("grant amount", `INSERT INTO ai_credit_grants (user_id, source_type, amount_credits, idempotency_key)
		VALUES ('ledger-sentinel', 'test', 0, 'invalid-grant')`)
	rejects("adjustment amount", `INSERT INTO ai_credit_adjustments (user_id, amount_credits, reason, idempotency_key)
		VALUES ('ledger-sentinel', 0, 'test', 'invalid-adjustment')`)
	rejects("reserved credits", `INSERT INTO ai_credit_reservations
		(id, user_id, operation_type, provider, mode, status, idempotency_key, reserved_credits)
		VALUES ('invalid-reserved', 'ledger-sentinel', 'voice', 'assemblyai', 'realtime', 'reserved', 'invalid-reserved-key', -1)`)

	reservationFieldCheck := func(label, column string, value int) {
		t.Helper()
		statement := fmt.Sprintf(`INSERT INTO ai_credit_reservations
			(id, user_id, operation_type, provider, mode, status, idempotency_key, reserved_credits, %s)
			VALUES ($1, 'ledger-sentinel', 'voice', 'assemblyai', 'realtime', 'reserved', $2, 1, $3)`, column)
		rejects(label, statement, "invalid-"+label, "invalid-key-"+label, value)
	}
	reservationFieldCheck("settled_credits", "settled_credits", -1)
	reservationFieldCheck("refunded_credits", "refunded_credits", -1)
	reservationFieldCheck("unit_rate", "unit_rate", 0)
	reservationFieldCheck("max_credits", "max_credits", -1)
	reservationFieldCheck("duration_seconds", "duration_seconds", -1)
	reservationFieldCheck("connected_duration_ms", "connected_duration_ms", -1)
	rejects("event credits", `INSERT INTO ai_credit_reservation_events
		(reservation_id, user_id, event_type, credits, idempotency_key)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'test', -1, 'invalid-event-credits')`)
	rejects("event duration", `INSERT INTO ai_credit_reservation_events
		(reservation_id, user_id, event_type, credits, duration_ms, idempotency_key)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'test', 0, -1, 'invalid-event-duration')`)
	rejects("evidence duration", `INSERT INTO ai_provider_usage_evidence
		(reservation_id, user_id, provider, mode, duration_ms, idempotency_key)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'assemblyai', 'realtime', -1, 'invalid-evidence-duration')`)
	rejects("evidence input units", `INSERT INTO ai_provider_usage_evidence
		(reservation_id, user_id, provider, mode, input_units, idempotency_key)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'assemblyai', 'realtime', -1, 'invalid-evidence-input')`)
	rejects("evidence output units", `INSERT INTO ai_provider_usage_evidence
		(reservation_id, user_id, provider, mode, output_units, idempotency_key)
		VALUES ('ledger-valid-reservation', 'ledger-sentinel', 'assemblyai', 'realtime', -1, 'invalid-evidence-output')`)
}

func TestAdoptBaselineRefusesWithoutPinnedContractOrDDL(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("askolo_migrations_adoption_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if err := AdoptBaseline(ctx, tx); err == nil || !strings.Contains(err.Error(), "no independently pinned archive baseline contract") {
		_ = tx.Rollback(ctx)
		t.Fatalf("adoption error = %v, want explicit refusal", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback adoption transaction: %v", err)
	}
	var historyExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('askolo_schema_migrations') IS NOT NULL`).Scan(&historyExists); err != nil {
		t.Fatalf("check migration history: %v", err)
	}
	var tableCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()`).Scan(&tableCount); err != nil {
		t.Fatalf("check schema tables: %v", err)
	}
	if historyExists || tableCount != 0 {
		t.Fatalf("adoption refusal left schema changes: history=%v tables=%d", historyExists, tableCount)
	}
}
