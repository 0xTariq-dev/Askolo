package migrations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

	schema := newMigrationTestSchemaName(t, "run")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, schema)

	testURL := migrationSchemaURL(t, baseURL, schema)
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	defer pool.Close()

	if err := ValidateReady(ctx, pool); err == nil || !strings.Contains(err.Error(), "ledger is missing") {
		t.Fatalf("unmigrated schema readiness error = %v, want missing ledger", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("clean migration: %v", err)
	}
	if err := ValidateReady(ctx, pool); err != nil {
		t.Fatalf("ready after clean migration: %v", err)
	}
	var rateLimitColumns string
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(string_agg(column_name, ',' ORDER BY ordinal_position), '')
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'auth_mfa_recovery_rate_limits'
`).Scan(&rateLimitColumns); err != nil {
		t.Fatalf("inspect shared auth rate-limit schema: %v", err)
	}
	if rateLimitColumns != "bucket_hash,window_started_at,request_count" {
		t.Fatalf("shared auth rate-limit columns = %q, want bucket_hash,window_started_at,request_count", rateLimitColumns)
	}
	firstRateLimitInsert, err := pool.Exec(ctx, `
INSERT INTO auth_mfa_recovery_rate_limits (bucket_hash, window_started_at, request_count)
VALUES ('migration-sentinel', NOW(), 1)
ON CONFLICT (bucket_hash) DO NOTHING
`)
	if err != nil || firstRateLimitInsert.RowsAffected() != 1 {
		t.Fatalf("insert shared auth rate-limit bucket: rows=%d err=%v", firstRateLimitInsert.RowsAffected(), err)
	}
	secondRateLimitInsert, err := pool.Exec(ctx, `
INSERT INTO auth_mfa_recovery_rate_limits (bucket_hash, window_started_at, request_count)
VALUES ('migration-sentinel', NOW(), 1)
ON CONFLICT (bucket_hash) DO NOTHING
`)
	if err != nil || secondRateLimitInsert.RowsAffected() != 0 {
		t.Fatalf("shared auth rate-limit bucket is not unique: rows=%d err=%v", secondRateLimitInsert.RowsAffected(), err)
	}
	assertCurrentMigrationVersions(t, ctx, pool)
	v2Schema := askoloV2SchemaName(schema)
	var v2TableCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = $1
		  AND table_name IN (
		    'workspaces', 'workspace_memberships', 'plan_catalog',
		    'workspace_subscriptions', 'workspace_subscription_seats',
		    'workspace_member_credit_limits', 'ai_credit_accounts',
		    'ai_credit_grants', 'ai_credit_grant_balances',
		    'ai_credit_reservation_allocations', 'integration_connections'
		  )`, v2Schema).Scan(&v2TableCount); err != nil {
		t.Fatalf("inspect versioned workspace schema: %v", err)
	}
	if v2TableCount != 11 {
		t.Fatalf("public_v2 required table count = %d, want 11", v2TableCount)
	}
	var seededPlanCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+quoteMigrationIdentifier(v2Schema)+`.plan_catalog`).Scan(&seededPlanCount); err != nil {
		t.Fatalf("check that plan allowances remain unseeded: %v", err)
	}
	if seededPlanCount != 0 {
		t.Fatalf("plan catalog contains %d rows; schema design must not invent prices or allowances", seededPlanCount)
	}
	var accountMemberColumns, connectionWorkspaceColumns, privateScopeColumns int
	if err := pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE table_name='ai_credit_accounts' AND column_name='member_user_id'),
		  count(*) FILTER (WHERE table_name='integration_connections' AND column_name='workspace_id'),
		  count(*) FILTER (WHERE table_name='integration_connections' AND column_name='availability_scope')
		FROM information_schema.columns
		WHERE table_schema=$1
		  AND table_name IN ('ai_credit_accounts', 'integration_connections')`,
		v2Schema).Scan(&accountMemberColumns, &connectionWorkspaceColumns, &privateScopeColumns); err != nil {
		t.Fatalf("inspect shared credit and private integration ownership: %v", err)
	}
	if accountMemberColumns != 0 || connectionWorkspaceColumns != 0 || privateScopeColumns != 1 {
		t.Fatalf("v2 ownership columns: account-member=%d connection-workspace=%d private-scope=%d",
			accountMemberColumns, connectionWorkspaceColumns, privateScopeColumns)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('sentinel', 'sentinel@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}
	for _, locale := range []string{"en", "ar", "fr"} {
		if _, err := pool.Exec(ctx, `UPDATE users SET preferred_locale = $1 WHERE id = 'sentinel'`, locale); err != nil {
			t.Errorf("locale format constraint rejected valid two-letter code %q: %v", locale, err)
		}
	}
	for _, locale := range []string{"EN", "e1", "en-US"} {
		if _, err := pool.Exec(ctx, `UPDATE users SET preferred_locale = $1 WHERE id = 'sentinel'`, locale); err == nil {
			t.Errorf("locale format constraint accepted malformed code %q", locale)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET preferred_locale = NULL WHERE id = 'sentinel'`); err != nil {
		t.Fatalf("restore nullable preferred locale: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts
		(user_id, granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits)
		VALUES ('sentinel', 17, -2, 3, 4, 5)`); err != nil {
		t.Fatalf("insert sentinel account: %v", err)
	}
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("rerun migration: %v", err)
	}
	if err := ValidateReady(ctx, pool); err != nil {
		t.Fatalf("ready after migration rerun: %v", err)
	}
	assertCurrentMigrationVersions(t, ctx, pool)
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
	if err := ValidateReady(ctx, pool); err == nil || !strings.Contains(err.Error(), "schema drift") {
		t.Fatalf("drift readiness error = %v, want schema drift", err)
	}
	if _, err := pool.Exec(ctx, `DROP INDEX migration_drift_idx`); err != nil {
		t.Fatalf("remove source-schema drift index: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE INDEX `+quoteMigrationIdentifier("v2_drift_idx")+
		` ON `+quoteMigrationIdentifier(v2Schema)+`.plan_catalog (plan_code)`); err != nil {
		t.Fatalf("induce public_v2 index drift: %v", err)
	}
	if err := Run(ctx, pool); err == nil || !strings.Contains(err.Error(), "schema drift") {
		t.Fatalf("public_v2 drift run error = %v, want schema drift", err)
	}
	if err := ValidateReady(ctx, pool); err == nil || !strings.Contains(err.Error(), "schema drift") {
		t.Fatalf("public_v2 drift readiness error = %v, want schema drift", err)
	}
}

func TestProductionSchemaCompatibilityDoesNotRequireMigrationLedger(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "publish_compatibility")
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load published schema migrations: %v", err)
	}
	// Preserve coverage for the original reviewed 0000-0013 compatibility
	// baseline; current Production snapshots are covered by the exact allowlist.
	const publishedSchemaMigrationCount = 14
	if len(migrations) < publishedSchemaMigrationCount {
		t.Fatalf("only %d migrations available for published baseline", len(migrations))
	}
	if err := run(ctx, pool, migrations[:publishedSchemaMigrationCount]); err != nil {
		t.Fatalf("apply published schema migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE askolo_schema_migrations`); err != nil {
		t.Fatalf("remove Go migration ledger: %v", err)
	}
	if err := ValidateProductionSchemaCompatibility(ctx, pool); err != nil {
		t.Fatalf("production schema check with no Go ledger: %v", err)
	}

	if _, err := pool.Exec(ctx, `CREATE INDEX publish_compatibility_drift_idx ON users (email)`); err != nil {
		t.Fatalf("induce production schema drift: %v", err)
	}
	if err := ValidateProductionSchemaCompatibility(ctx, pool); err == nil ||
		!strings.Contains(err.Error(), "managed production schema inventory mismatch") {
		t.Fatalf("production schema drift error = %v, want inventory mismatch", err)
	}
	if _, err := pool.Exec(ctx, `DROP INDEX publish_compatibility_drift_idx`); err != nil {
		t.Fatalf("remove production schema drift: %v", err)
	}
	if err := ValidateProductionSchemaCompatibility(ctx, pool); err != nil {
		t.Fatalf("production schema check after removing drift: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE ai_credit_policy_versions
		SET operation_weights = '{"assistant":1}'::jsonb WHERE version = 1`); err != nil {
		t.Fatalf("add assistant weight to historical policy row: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_credit_policy_versions
		SET operation_weights = '{"voice":1}'::jsonb
		WHERE version = (SELECT MAX(version) FROM ai_credit_policy_versions)`); err != nil {
		t.Fatalf("remove assistant weight from the latest policy row: %v", err)
	}
	if err := ValidateProductionSchemaCompatibility(ctx, pool); err == nil ||
		!strings.Contains(err.Error(), "assistant policy seed data is incomplete") {
		t.Fatalf("latest-policy data compatibility error = %v, want seed-data block", err)
	}
}

func TestOperationalReadinessIncludesTheHomeProjectionSchema(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "operational_readiness")
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) < operationalMigrationVersion {
		t.Fatalf("migration list has %d entries, need the active prefix of %d",
			len(migrations), operationalMigrationVersion)
	}
	if err := RunOperational(ctx, pool); err != nil {
		t.Fatalf("apply active API migration prefix: %v", err)
	}
	if err := ValidateOperationalReady(ctx, pool); err != nil {
		t.Fatalf("operational readiness for active schema: %v", err)
	}
	if err := ValidateReady(ctx, pool); err != nil {
		t.Fatalf("full readiness after active migration prefix: %v", err)
	}
	for _, table := range []string{
		"workspace_provider_account_access",
		"workspace_email_summaries",
		"home_focus_selections",
	} {
		var sourceSchema string
		if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&sourceSchema); err != nil {
			t.Fatalf("read migration source schema: %v", err)
		}
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT to_regclass($1) IS NOT NULL`, askoloV2SchemaName(sourceSchema)+"."+table).Scan(&exists); err != nil {
			t.Fatalf("check Home relation %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("Home relation %s was not created", table)
		}
	}
}

func TestReconcileOperationalFingerprintUpdatesOnlyTheActiveTip(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "fingerprint_reconciliation")

	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migration set: %v", err)
	}
	if err := run(ctx, pool, migrations[:askoloV2MigrationVersion]); err != nil {
		t.Fatalf("apply historical migration prefix: %v", err)
	}
	var maxVersion int
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM askolo_schema_migrations`).Scan(&maxVersion); err != nil {
		t.Fatalf("read active migration version: %v", err)
	}
	if maxVersion != askoloV2MigrationVersion-1 {
		t.Fatalf("latest active migration = %04d, want %04d", maxVersion, askoloV2MigrationVersion-1)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('reconcile-sentinel', 'reconcile@example.test')`); err != nil {
		t.Fatalf("insert sentinel row: %v", err)
	}
	var priorVersion14Fingerprint string
	if err := pool.QueryRow(ctx, `SELECT schema_fingerprint FROM askolo_schema_migrations WHERE version = 14`).
		Scan(&priorVersion14Fingerprint); err != nil {
		t.Fatalf("read prior migration fingerprint: %v", err)
	}
	staleFingerprint := fingerprintPrefix + strings.Repeat("0", 64)
	if _, err := pool.Exec(ctx, `UPDATE askolo_schema_migrations SET schema_fingerprint = $1 WHERE version = $2`,
		staleFingerprint, askoloV2MigrationVersion-1); err != nil {
		t.Fatalf("prepare stale active fingerprint: %v", err)
	}

	result, err := ReconcileOperationalFingerprint(ctx, pool)
	if err != nil {
		t.Fatalf("reconcile active fingerprint: %v", err)
	}
	if !result.Changed || result.Previous != staleFingerprint || result.Current == staleFingerprint {
		t.Fatalf("reconciliation result = %+v, want changed fingerprint", result)
	}
	if err := validateReadyWithMigrationPrefix(ctx, pool, migrations, askoloV2MigrationVersion, false); err != nil {
		t.Fatalf("historical active-prefix readiness after reconciliation: %v", err)
	}

	var latestFingerprint, unchangedVersion14Fingerprint string
	if err := pool.QueryRow(ctx, `SELECT schema_fingerprint FROM askolo_schema_migrations WHERE version = $1`,
		askoloV2MigrationVersion-1).Scan(&latestFingerprint); err != nil {
		t.Fatalf("read reconciled fingerprint: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT schema_fingerprint FROM askolo_schema_migrations WHERE version = 14`).
		Scan(&unchangedVersion14Fingerprint); err != nil {
		t.Fatalf("read earlier migration fingerprint: %v", err)
	}
	if latestFingerprint != result.Current || unchangedVersion14Fingerprint != priorVersion14Fingerprint {
		t.Fatalf("ledger fingerprints changed unexpectedly: latest=%q prior-v14=%q want prior-v14=%q",
			latestFingerprint, unchangedVersion14Fingerprint, priorVersion14Fingerprint)
	}
	var sentinelCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = 'reconcile-sentinel'`).Scan(&sentinelCount); err != nil {
		t.Fatalf("verify application row survived reconciliation: %v", err)
	}
	if sentinelCount != 1 {
		t.Fatalf("application row count after reconciliation = %d, want 1", sentinelCount)
	}

	replayed, err := ReconcileOperationalFingerprint(ctx, pool)
	if err != nil {
		t.Fatalf("repeat reconciliation: %v", err)
	}
	if replayed.Changed || replayed.Previous != result.Current || replayed.Current != result.Current {
		t.Fatalf("repeat reconciliation = %+v, want no-op", replayed)
	}
}

func TestReconcileOperationalFingerprintRejectsAppliedStagedMigration(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "fingerprint_reconciliation_staged")
	if err := Run(ctx, pool); err != nil {
		t.Fatalf("apply complete disposable migration set: %v", err)
	}
	if _, err := ReconcileOperationalFingerprint(ctx, pool); err == nil ||
		!strings.Contains(err.Error(), "staged migration") {
		t.Fatalf("reconciliation error = %v, want staged migration refusal", err)
	}
}

func TestRunScopesConstraintLookupsToTheTargetSchema(t *testing.T) {
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

	targetSchema := newMigrationTestSchemaName(t, "target")
	shadowSchema := newMigrationTestSchemaName(t, "shadow")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(targetSchema)); err != nil {
		t.Fatalf("create target schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, targetSchema)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(shadowSchema)); err != nil {
		t.Fatalf("create shadow schema: %v", err)
	}
	defer admin.Exec(context.Background(), `DROP SCHEMA `+quoteMigrationIdentifier(shadowSchema)+` CASCADE`)

	shadow, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, shadowSchema))
	if err != nil {
		t.Fatalf("open shadow schema pool: %v", err)
	}
	defer shadow.Close()
	if _, err := shadow.Exec(ctx, `CREATE TABLE ai_credit_grants (
		amount_credits integer NOT NULL,
		CONSTRAINT ai_credit_grants_amount_positive CHECK (amount_credits > 0)
	)`); err != nil {
		t.Fatalf("create shadow constraint: %v", err)
	}

	target, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, targetSchema))
	if err != nil {
		t.Fatalf("open target schema pool: %v", err)
	}
	defer target.Close()
	if err := Run(ctx, target); err != nil {
		t.Fatalf("run migrations with a same-named constraint in another schema: %v", err)
	}

	var constraintCount int
	if err := target.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_constraint
		WHERE conrelid=to_regclass('ai_credit_grants')
		  AND conname='ai_credit_grants_amount_positive'
	`).Scan(&constraintCount); err != nil {
		t.Fatalf("check target constraint: %v", err)
	}
	if constraintCount != 1 {
		t.Fatalf("target schema constraint count = %d, want 1", constraintCount)
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
	schema := newMigrationTestSchemaName(t, "tamper")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, schema)
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
	if err := ValidateReady(ctx, pool); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tampered readiness error = %v, want mismatch", err)
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
	schema := newMigrationTestSchemaName(t, "failure")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, schema)
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
	schema := newMigrationTestSchemaName(t, "lock")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, schema)
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
	assertCurrentMigrationVersions(t, ctx, first)
}

func TestRunResumesAfterInterruptedMigrationWithForwardFix(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "forward_fix")

	base, err := Load(SQL)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	version := len(base)
	brokenSQL := `CREATE TABLE migration_forward_fix_marker (id integer PRIMARY KEY); SELECT 1/0;`
	broken := syntheticMigration(version, "forward_fix", brokenSQL)
	interrupted := append(append([]Migration(nil), base...), broken)
	if err := run(ctx, pool, interrupted); err == nil {
		t.Fatal("interrupted migration unexpectedly succeeded")
	}
	assertMigrationVersions(t, ctx, pool, len(base))

	var markerExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('migration_forward_fix_marker') IS NOT NULL`).Scan(&markerExists); err != nil {
		t.Fatalf("check failed migration rollback: %v", err)
	}
	if markerExists {
		t.Fatal("failed migration left its schema change behind")
	}

	forwardSQL := `CREATE TABLE migration_forward_fix_marker (id integer PRIMARY KEY);`
	forwardFix := syntheticMigration(version, "forward_fix", forwardSQL)
	recovered := append(append([]Migration(nil), base...), forwardFix)
	if err := run(ctx, pool, recovered); err != nil {
		t.Fatalf("apply forward fix after interrupted migration: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, len(base)+1)
	if err := pool.QueryRow(ctx, `SELECT to_regclass('migration_forward_fix_marker') IS NOT NULL`).Scan(&markerExists); err != nil {
		t.Fatalf("check forward fix result: %v", err)
	}
	if !markerExists {
		t.Fatal("forward fix did not create its schema change")
	}
}

func TestRunAndReadinessAsLeastPrivilegedSchemaOwner(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, _, schema := openMigrationTestSchema(t, ctx, baseURL, "least_privileged_runner")
	role := fmt.Sprintf("askolo_migrator_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE ROLE `+quoteMigrationIdentifier(role)+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`); err != nil {
		t.Skipf("test database cannot create a restricted migration role: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), `DROP OWNED BY `+quoteMigrationIdentifier(role))
		_, _ = admin.Exec(context.Background(), `DROP ROLE `+quoteMigrationIdentifier(role))
	}()
	var databaseName string
	if err := admin.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatalf("read disposable test database name: %v", err)
	}
	if _, err := admin.Exec(ctx, `GRANT CREATE ON DATABASE `+quoteMigrationIdentifier(databaseName)+` TO `+quoteMigrationIdentifier(role)); err != nil {
		t.Skipf("test database cannot grant the schema-creation privilege required by the v2 migration: %v", err)
	}
	if _, err := admin.Exec(ctx, `GRANT USAGE, CREATE ON SCHEMA `+quoteMigrationIdentifier(schema)+` TO `+quoteMigrationIdentifier(role)); err != nil {
		t.Fatalf("grant schema-only migration privileges: %v", err)
	}
	var databaseUser string
	if err := admin.QueryRow(ctx, `SELECT current_user`).Scan(&databaseUser); err != nil {
		t.Fatalf("read test database user: %v", err)
	}
	if _, err := admin.Exec(ctx, `GRANT `+quoteMigrationIdentifier(role)+` TO `+quoteMigrationIdentifier(databaseUser)); err != nil {
		t.Skipf("test database cannot grant the restricted migration role: %v", err)
	}

	config, err := pgxpool.ParseConfig(migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		t.Fatalf("parse limited-role database configuration: %v", err)
	}
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE `+quoteMigrationIdentifier(role))
		return err
	}
	limitedPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open limited-role pool: %v", err)
	}
	defer limitedPool.Close()

	var currentRole string
	var isSuperuser bool
	if err := limitedPool.QueryRow(ctx, `SELECT current_user, rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&currentRole, &isSuperuser); err != nil {
		t.Fatalf("verify limited migration role: %v", err)
	}
	if currentRole != role || isSuperuser {
		t.Fatalf("migration role = %q, superuser=%t; want restricted role", currentRole, isSuperuser)
	}
	if err := Run(ctx, limitedPool); err != nil {
		t.Fatalf("run migrations as restricted role: %v", err)
	}
	if err := ValidateReady(ctx, limitedPool); err != nil {
		t.Fatalf("validate readiness as restricted role: %v", err)
	}
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

func assertCurrentMigrationVersions(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	migrationSet, err := Load(SQL)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	assertMigrationVersions(t, ctx, pool, len(migrationSet))
}

func syntheticMigration(version int, name, sqlText string) Migration {
	sum := sha256.Sum256([]byte(sqlText))
	return Migration{
		Version: version,
		Name:    fmt.Sprintf("%04d_%s", version, name),
		SQL:     sqlText,
		SHA256:  hex.EncodeToString(sum[:]),
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

func TestUSDLedgerMigrationPreservesHistoricalZeroReservation(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run migration integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "usd_legacy_reservation")
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) <= 8 || migrations[8].Name != "0008_usd_micro_ledger" {
		t.Fatalf("USD ledger migration missing from expected position")
	}
	if err := run(ctx, pool, migrations[:8]); err != nil {
		t.Fatalf("apply legacy migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('usd-legacy-sentinel', 'usd-legacy@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_accounts
		(user_id, granted_credits, adjustment_credits, reserved_credits, spent_credits, refunded_credits)
		VALUES ('usd-legacy-sentinel', 0, 0, 0, 0, 0)`); err != nil {
		t.Fatalf("insert sentinel account: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_credit_reservations
		(id, user_id, operation_type, provider, mode, status, idempotency_key, reserved_credits, settled_credits)
		VALUES ('usd-legacy-settled', 'usd-legacy-sentinel', 'voice', 'assemblyai', 'realtime', 'settled', 'usd-legacy-key', 0, 1)`); err != nil {
		t.Fatalf("insert historical settled reservation: %v", err)
	}
	if err := run(ctx, pool, migrations); err != nil {
		t.Fatalf("apply USD ledger migration while preserving historical row: %v", err)
	}
	var currency, status string
	var reservedCredits, settledCredits int
	if err := pool.QueryRow(ctx, `SELECT currency, status, reserved_credits, settled_credits
		FROM ai_credit_reservations WHERE id = 'usd-legacy-settled'`).
		Scan(&currency, &status, &reservedCredits, &settledCredits); err != nil {
		t.Fatalf("read historical reservation: %v", err)
	}
	if currency != "CREDITS" || status != "settled" || reservedCredits != 0 || settledCredits != 1 {
		t.Fatalf("migration changed historical reservation: currency=%q status=%q reserved=%d settled=%d",
			currency, status, reservedCredits, settledCredits)
	}
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
	schema := newMigrationTestSchemaName(t, "ledger")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer cleanupMigrationTestSchemas(t, admin, schema)
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
	assertCurrentMigrationVersions(t, ctx, pool)
	var email, status string
	if err := pool.QueryRow(ctx, `SELECT email, status FROM users WHERE id='adoption-sentinel'`).Scan(&email, &status); err != nil {
		t.Fatalf("read sentinel after adoption and forward migration: %v", err)
	}
	if email != "adoption@example.test" || status != "active" {
		t.Fatalf("sentinel changed after adoption: email=%q status=%q", email, status)
	}
}

func TestArchivedWorkspaceBaselineFingerprintsMatchPinnedContract(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "workspace_contract")
	applyUntrackedWorkspaceBaseline(t, ctx, pool, false)

	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	workspaceMigration := migrations[archivedWorkspaceBaselineContract.MigrationVersion]
	if workspaceMigration.Name != archivedWorkspaceBaselineContract.MigrationName ||
		workspaceMigration.SHA256 != archivedWorkspaceBaselineContract.MigrationChecksum {
		t.Fatalf("workspace migration contract mismatch: got %s %s", workspaceMigration.Name, workspaceMigration.SHA256)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fingerprint transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	logical, err := logicalFingerprint(ctx, tx)
	if err != nil {
		t.Fatalf("compute logical baseline fingerprint: %v", err)
	}
	workspace, err := workspaceFingerprint(ctx, tx)
	if err != nil {
		t.Fatalf("compute workspace fingerprint: %v", err)
	}
	if logical != archivedWorkspaceBaselineContract.LogicalFingerprint ||
		workspace != archivedWorkspaceBaselineContract.WorkspaceFingerprint {
		t.Fatalf("workspace baseline fingerprints are not pinned: logical=%q workspace=%q", logical, workspace)
	}
}

func TestAdoptArchivedWorkspaceBaselineIgnoresLegacyColumnOrder(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "workspace_adoption")
	applyUntrackedWorkspaceBaseline(t, ctx, pool, true)
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email) VALUES ('workspace-adoption-sentinel', 'workspace-adoption@example.test')`); err != nil {
		t.Fatalf("insert sentinel user: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin adoption validation transaction: %v", err)
	}
	actualInventory, err := fingerprint(ctx, tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("fingerprint reordered baseline: %v", err)
	}
	if actualInventory == archiveBaselineContract.Fingerprint {
		_ = tx.Rollback(ctx)
		t.Fatal("reordered workspace baseline unexpectedly matched the legacy order-sensitive fingerprint")
	}
	if err := ValidateBaseline(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("validate archived workspace baseline: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback adoption validation: %v", err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin adoption transaction: %v", err)
	}
	if err := AdoptBaseline(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("adopt archived workspace baseline: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit workspace baseline adoption: %v", err)
	}
	assertAdoptedPrefix(t, ctx, pool, actualInventory)

	if err := Run(ctx, pool); err != nil {
		t.Fatalf("run after workspace baseline adoption: %v", err)
	}
	assertCurrentMigrationVersions(t, ctx, pool)
	var email, status string
	if err := pool.QueryRow(ctx, `SELECT email, status FROM users WHERE id='workspace-adoption-sentinel'`).Scan(&email, &status); err != nil {
		t.Fatalf("read sentinel after adoption and forward migrations: %v", err)
	}
	if email != "workspace-adoption@example.test" || status != "active" {
		t.Fatalf("sentinel changed after workspace adoption: email=%q status=%q", email, status)
	}
}

func TestAdoptArchivedWorkspaceBaselineRejectsUnreviewedDrift(t *testing.T) {
	baseURL := migrationTestURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, pool, _ := openMigrationTestSchema(t, ctx, baseURL, "workspace_adoption_drift")
	applyUntrackedWorkspaceBaseline(t, ctx, pool, false)
	if _, err := pool.Exec(ctx, `CREATE TABLE untracked_workspace_baseline_object (id integer)`); err != nil {
		t.Fatalf("add unreviewed schema object: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin drifted adoption transaction: %v", err)
	}
	err = AdoptBaseline(ctx, tx)
	if err == nil || !strings.Contains(err.Error(), "does not match archived workspace contract") {
		_ = tx.Rollback(ctx)
		t.Fatalf("drifted adoption error = %v, want archived workspace contract mismatch", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback refused drifted adoption: %v", err)
	}
	assertNoAdoptionLedger(t, ctx, pool)
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
	assertCurrentMigrationVersions(t, ctx, pool)
	var stored string
	if err := pool.QueryRow(ctx, `SELECT schema_fingerprint FROM askolo_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatalf("read new versioned fingerprint: %v", err)
	}
	if !strings.HasPrefix(stored, askoloV2FingerprintPrefix) {
		t.Fatalf("new migration stored fingerprint %q, want prefix %q", stored, askoloV2FingerprintPrefix)
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
	schema := newMigrationTestSchemaName(t, purpose)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+quoteMigrationIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatalf("create disposable schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, migrationSchemaURL(t, baseURL, schema))
	if err != nil {
		if cleanupErr := dropMigrationTestSchemas(admin, schema); cleanupErr != nil {
			t.Errorf("clean up schemas after pool setup failure: %v", cleanupErr)
		}
		admin.Close()
		t.Fatalf("open schema-isolated pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if cleanupErr := dropMigrationTestSchemas(admin, schema); cleanupErr != nil {
			t.Errorf("clean up isolated migration schemas: %v", cleanupErr)
		}
		admin.Close()
	})
	return admin, pool, schema
}

func newMigrationTestSchemaName(t *testing.T, purpose string) string {
	t.Helper()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate migration test schema suffix: %v", err)
	}
	purpose = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return '_'
	}, purpose)
	if len(purpose) > 18 {
		purpose = purpose[:18]
	}
	name := "askolo_migrations_" + purpose + "_" + hex.EncodeToString(random)
	if len(name)+len("_public_v2") > 63 {
		t.Fatalf("migration schema name exceeds PostgreSQL's identifier limit")
	}
	return name
}

func cleanupMigrationTestSchemas(t *testing.T, admin *pgxpool.Pool, sourceSchema string) {
	t.Helper()
	if err := dropMigrationTestSchemas(admin, sourceSchema); err != nil {
		t.Errorf("drop migration test schemas for %q: %v", sourceSchema, err)
	}
}

func dropMigrationTestSchemas(admin *pgxpool.Pool, sourceSchema string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, schema := range []string{askoloV2SchemaName(sourceSchema), sourceSchema} {
		if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+quoteMigrationIdentifier(schema)+` CASCADE`); err != nil {
			return fmt.Errorf("drop schema %q: %w", schema, err)
		}
	}
	return nil
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

func applyUntrackedWorkspaceBaseline(t *testing.T, ctx context.Context, pool *pgxpool.Pool, reorderLegacyColumns bool) {
	t.Helper()
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load workspace baseline migrations: %v", err)
	}
	for _, version := range []int{0, 1, archivedWorkspaceBaselineContract.MigrationVersion} {
		migration := migrations[version]
		sqlText := migration.SQL
		if reorderLegacyColumns && version == 0 {
			sqlText, err = reorderLegacyColumnOrderForTest(sqlText)
			if err != nil {
				t.Fatalf("reorder legacy column fixture: %v", err)
			}
		}
		sqlText = strings.ReplaceAll(sqlText, `REFERENCES "public".`, `REFERENCES `)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin untracked workspace migration %04d: %v", version, err)
		}
		if _, err := tx.Exec(ctx, sqlText); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply untracked workspace migration %04d: %v", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit untracked workspace migration %04d: %v", version, err)
		}
	}
}

func reorderLegacyColumnOrderForTest(sqlText string) (string, error) {
	replacements := [][2]string{
		{
			`"google_event_id" text,
"created_at" timestamp with time zone DEFAULT now() NOT NULL,`,
			`"created_at" timestamp with time zone DEFAULT now() NOT NULL,
"google_event_id" text,`,
		},
		{
			`"retention" varchar(32) DEFAULT 'delete_immediately' NOT NULL,
"consent_at" timestamp with time zone,`,
			`"consent_at" timestamp with time zone,
"retention" varchar(32) DEFAULT 'delete_immediately' NOT NULL,`,
		},
	}
	for _, replacement := range replacements {
		if !strings.Contains(sqlText, replacement[0]) {
			return "", fmt.Errorf("legacy schema fixture does not contain expected column sequence %q", replacement[0])
		}
		sqlText = strings.Replace(sqlText, replacement[0], replacement[1], 1)
	}
	return sqlText, nil
}

func assertAdoptedPrefix(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expectedFingerprints ...string) {
	t.Helper()
	expectedFingerprint := archiveBaselineContract.Fingerprint
	if len(expectedFingerprints) > 1 {
		t.Fatal("assertAdoptedPrefix accepts at most one fingerprint")
	}
	if len(expectedFingerprints) == 1 {
		expectedFingerprint = expectedFingerprints[0]
	}
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
		if fp != expectedFingerprint {
			t.Fatalf("adopted fingerprint = %q, want %q", fp, expectedFingerprint)
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
