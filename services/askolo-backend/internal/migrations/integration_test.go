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

func TestAdoptBaselineMatchesPinnedArchiveAndPreservesData(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "adoption_positive")
	applyUntrackedBaseline(t, ctx, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('adoption-sentinel', 'adoption@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin validation transaction: %v", err)
	}
	got, err := fingerprint(ctx, tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("fingerprint baseline: %v", err)
	}
	if got != archiveBaselineContract.Fingerprint {
		_ = tx.Rollback(ctx)
		t.Fatalf("baseline inventory = %s, want pinned %s", got, archiveBaselineContract.Fingerprint)
	}
	if err := ValidateBaseline(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("validate pinned baseline: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback validation transaction: %v", err)
	}

	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin adoption transaction: %v", err)
	}
	if err := AdoptBaseline(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("adopt exact baseline: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit baseline adoption: %v", err)
	}
	assertAdoptedPrefix(t, ctx, pool)

	// The baseline rows cover migrations 0000 and 0001, so the ordinary runner
	// can now apply only the later, non-idempotent-safe forward migration.
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("run after baseline adoption: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 3)
	var email, status string
	if err := pool.QueryRow(ctx, `SELECT email, status FROM users WHERE id='adoption-sentinel'`).Scan(&email, &status); err != nil {
		t.Fatalf("read sentinel after adoption and forward migration: %v", err)
	}
	if email != "adoption@example.test" || status != "active" {
		t.Fatalf("sentinel changed after adoption: email=%q status=%q", email, status)
	}
}

func TestAdoptBaselineRejectsSchemaMutationsWithoutWritingHistory(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	mutations := []struct {
		name      string
		statement string
	}{
		{"extra column", `ALTER TABLE users ADD COLUMN untracked integer`},
		{"extra table", `CREATE TABLE untracked_archive_object (id integer)`},
		{"extra composite type", `CREATE TYPE untracked_record AS (value text)`},
		{"extra routine", `CREATE FUNCTION untracked_function() RETURNS integer LANGUAGE SQL IMMUTABLE AS 'SELECT 1'`},
		{"missing index", `DROP INDEX auth_email_challenges_email_purpose_idx`},
		{"changed default", `ALTER TABLE users ALTER COLUMN status DROP DEFAULT`},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "adoption_mutation")
			applyUntrackedBaseline(t, ctx, pool)
			if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('mutation-sentinel', 'mutation@example.test')`); err != nil {
				t.Fatalf("insert sentinel user: %v", err)
			}
			if _, err := pool.Exec(ctx, mutation.statement); err != nil {
				t.Fatalf("apply schema mutation: %v", err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin adoption transaction: %v", err)
			}
			err = AdoptBaseline(ctx, tx)
			if err == nil || !strings.Contains(err.Error(), "does not match pinned archive contract") {
				_ = tx.Rollback(ctx)
				t.Fatalf("adoption error = %v, want pinned-contract mismatch", err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatalf("rollback refused adoption: %v", err)
			}
			assertNoAdoptionLedger(t, ctx, pool)
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='mutation-sentinel' AND email='mutation@example.test'`).Scan(&count); err != nil {
				t.Fatalf("check sentinel data: %v", err)
			}
			if count != 1 {
				t.Fatalf("adoption changed or removed sentinel data; matching rows=%d", count)
			}
		})
	}
}

func TestAdoptBaselineRefusesDatabaseWideEventTriggers(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, pool, schema := openMigrationTestSchema(t, ctx, baseURL, "adoption_event_trigger")
	applyUntrackedBaseline(t, ctx, pool)
	functionName := "adoption_event_guard"
	triggerName := fmt.Sprintf("askolo_adoption_event_guard_%d", time.Now().UnixNano())
	function := quoteMigrationIdentifier(schema) + "." + quoteMigrationIdentifier(functionName)
	if _, err := admin.Exec(ctx, `CREATE FUNCTION `+function+`()
		RETURNS event_trigger LANGUAGE plpgsql AS $$ BEGIN END $$`); err != nil {
		t.Skipf("local PostgreSQL does not allow creating the event-trigger fixture: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP FUNCTION `+function+` CASCADE`)
	if _, err := admin.Exec(ctx, `CREATE EVENT TRIGGER `+quoteMigrationIdentifier(triggerName)+`
		ON ddl_command_start EXECUTE FUNCTION `+function+`()`); err != nil {
		t.Skipf("local PostgreSQL does not allow creating the event-trigger fixture: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP EVENT TRIGGER `+quoteMigrationIdentifier(triggerName))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin adoption transaction: %v", err)
	}
	if err := AdoptBaseline(ctx, tx); err == nil || !strings.Contains(err.Error(), "database-wide event triggers") {
		_ = tx.Rollback(ctx)
		t.Fatalf("adoption error = %v, want event-trigger refusal", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback refused adoption: %v", err)
	}
	assertNoAdoptionLedger(t, ctx, pool)
}

func TestAdoptBaselineRefusesEmptySchemaAndExistingHistory(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	t.Run("empty schema", func(t *testing.T) {
		_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "adoption_empty")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin adoption transaction: %v", err)
		}
		if err := AdoptBaseline(ctx, tx); err == nil || !strings.Contains(err.Error(), "does not match pinned archive contract") {
			_ = tx.Rollback(ctx)
			t.Fatalf("adoption error = %v, want pinned-contract mismatch", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rollback refused adoption: %v", err)
		}
		assertNoAdoptionLedger(t, ctx, pool)
		var objectCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=current_schema()`).Scan(&objectCount); err != nil {
			t.Fatalf("count schema objects: %v", err)
		}
		if objectCount != 0 {
			t.Fatalf("empty-schema refusal left %d objects", objectCount)
		}
	})
	t.Run("history already exists", func(t *testing.T) {
		_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "adoption_history")
		applyUntrackedBaseline(t, ctx, pool)
		if _, err := pool.Exec(ctx, `CREATE TABLE askolo_schema_migrations (version integer PRIMARY KEY)`); err != nil {
			t.Fatalf("create preexisting ledger marker: %v", err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin adoption transaction: %v", err)
		}
		if err := AdoptBaseline(ctx, tx); err == nil || !strings.Contains(err.Error(), "migration history already exists") {
			_ = tx.Rollback(ctx)
			t.Fatalf("adoption error = %v, want existing-history refusal", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rollback refused adoption: %v", err)
		}
	})
}

func TestAdoptBaselineNeedsOnlySchemaCreatePrivilege(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, pool, schema := openMigrationTestSchema(t, ctx, baseURL, "adoption_limited_role")
	applyUntrackedBaseline(t, ctx, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('limited-sentinel', 'limited@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}
	role := fmt.Sprintf("askolo_adopter_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE ROLE `+quoteMigrationIdentifier(role)+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`); err != nil {
		t.Skipf("local PostgreSQL does not allow creating a restricted test role: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP ROLE `+quoteMigrationIdentifier(role))
	defer admin.Exec(context.Background(), `DROP OWNED BY `+quoteMigrationIdentifier(role))
	if _, err := admin.Exec(ctx, `GRANT USAGE, CREATE ON SCHEMA `+quoteMigrationIdentifier(schema)+` TO `+quoteMigrationIdentifier(role)); err != nil {
		t.Fatalf("grant schema-only privileges: %v", err)
	}
	var hasTableSelect bool
	userTable := quoteMigrationIdentifier(schema) + `.users`
	if err := admin.QueryRow(ctx, `SELECT has_table_privilege($1, to_regclass($2), 'SELECT')`, role, userTable).Scan(&hasTableSelect); err != nil {
		t.Fatalf("check restricted role table privileges: %v", err)
	}
	if hasTableSelect {
		t.Fatal("restricted role unexpectedly has SELECT on baseline tables")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin restricted adoption transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+quoteMigrationIdentifier(role)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("switch to restricted role: %v", err)
	}
	if err := AdoptBaseline(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("adopt with schema-only CREATE and USAGE: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit restricted adoption: %v", err)
	}
	assertAdoptedPrefix(t, ctx, pool)
	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE id='limited-sentinel'`).Scan(&email); err != nil {
		t.Fatalf("read sentinel data after limited adoption: %v", err)
	}
	if email != "limited@example.test" {
		t.Fatalf("sentinel email changed: %q", email)
	}
}

func TestRunAdvancesLegacyFingerprintHistory(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "legacy_fingerprint")
	if _, err := pool.Exec(ctx, `CREATE TABLE askolo_schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		schema_fingerprint text NOT NULL
	)`); err != nil {
		t.Fatalf("create migration ledger: %v", err)
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
			t.Fatalf("apply migration %04d: %v", migration.Version, err)
		}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin legacy-history setup: %v", err)
	}
	legacy, err := legacyFingerprint(ctx, tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("compute legacy fingerprint: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE askolo_schema_migrations SET schema_fingerprint=$1`, legacy); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("simulate old runner history: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit legacy-history setup: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("advance old history using the legacy drift check: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, 3)
	var stored string
	if err := pool.QueryRow(ctx, `SELECT schema_fingerprint FROM askolo_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatalf("read new versioned fingerprint: %v", err)
	}
	if !strings.HasPrefix(stored, fingerprintPrefix) {
		t.Fatalf("new migration stored fingerprint %q, want prefix %q", stored, fingerprintPrefix)
	}
}

func migrationTestURL(t *testing.T) string {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	return baseURL
}

func openMigrationTestSchema(t *testing.T, ctx context.Context, baseURL, purpose string) (*pgxpool.Pool, *pgxpool.Pool, string) {
	t.Helper()
	admin, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open disposable database: %v", err)
	}
	schema := fmt.Sprintf("askolo_migrations_%s_%d", purpose, time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatalf("create disposable schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		_, _ = admin.Exec(ctx, `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
		admin.Close()
		t.Fatalf("open schema-isolated pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(schema)+` CASCADE`)
		admin.Close()
	})
	return admin, pool, schema
}

func applyUntrackedBaseline(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load baseline migrations: %v", err)
	}
	for _, migration := range migrations[:archiveBaselineContract.LastMigrationVersion+1] {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin baseline setup transaction: %v", err)
		}
		sqlText := strings.ReplaceAll(migration.SQL, `REFERENCES "public".`, `REFERENCES `)
		if _, err := tx.Exec(ctx, sqlText); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply untracked baseline migration %04d: %v", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit untracked baseline migration %04d: %v", migration.Version, err)
		}
	}
}

func assertAdoptedPrefix(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT version, name, checksum, schema_fingerprint FROM askolo_schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("read adopted migration history: %v", err)
	}
	defer rows.Close()
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load expected migration history: %v", err)
	}
	var count int
	for rows.Next() {
		var version int
		var name, checksum, fp string
		if err := rows.Scan(&version, &name, &checksum, &fp); err != nil {
			t.Fatalf("scan adopted migration history: %v", err)
		}
		if version != count || version > archiveBaselineContract.LastMigrationVersion {
			t.Fatalf("adopted version = %d at row %d", version, count)
		}
		if name != migrations[version].Name || checksum != migrations[version].SHA256 {
			t.Fatalf("adopted version %d metadata does not match embedded migration", version)
		}
		if fp != archiveBaselineContract.Fingerprint {
			t.Fatalf("adopted fingerprint = %q, want %q", fp, archiveBaselineContract.Fingerprint)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate adopted migration history: %v", err)
	}
	if count != archiveBaselineContract.LastMigrationVersion+1 {
		t.Fatalf("adopted history rows = %d, want %d", count, archiveBaselineContract.LastMigrationVersion+1)
	}
}

func assertNoAdoptionLedger(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var historyExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.askolo_schema_migrations') IS NOT NULL`).Scan(&historyExists); err != nil {
		t.Fatalf("check migration history: %v", err)
	}
	if historyExists {
		t.Fatal("refused adoption left a migration history table")
	}
}
