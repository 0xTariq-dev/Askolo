package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidateReady checks the complete embedded migration set. It is used by the
// explicit migration runner and disposable-database validation.
func ValidateReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("migration readiness unavailable: database is not configured")
	}
	expected, err := Load(SQL)
	if err != nil {
		return fmt.Errorf("load migration set: %w", err)
	}
	return validateReadyWithMigrations(ctx, pool, expected)
}

// ValidateOperationalReady keeps runtime health checks on the active schema
// contract while the public_v2 design remains staged and not yet promoted.
func ValidateOperationalReady(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("migration readiness unavailable: database is not configured")
	}
	expected, err := Load(SQL)
	if err != nil {
		return fmt.Errorf("load migration set: %w", err)
	}
	if len(expected) < askoloV2MigrationVersion {
		return fmt.Errorf("active migration prefix is incomplete: have %d versions, need %d",
			len(expected), askoloV2MigrationVersion)
	}
	return validateReadyWithMigrationPrefix(ctx, pool, expected, askoloV2MigrationVersion, false)
}

func validateReadyWithMigrations(ctx context.Context, pool *pgxpool.Pool, expected []Migration) error {
	return validateReadyWithMigrationPrefix(ctx, pool, expected, len(expected), true)
}

func validateReadyWithMigrationPrefix(ctx context.Context, pool *pgxpool.Pool, expected []Migration, activePrefix int, requireCompleteHistory bool) error {
	if len(expected) == 0 {
		return errors.New("migration readiness unavailable: no migrations are embedded")
	}
	if activePrefix <= 0 || activePrefix > len(expected) {
		return fmt.Errorf("migration readiness unavailable: invalid active prefix %d for %d embedded migrations", activePrefix, len(expected))
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration readiness connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin migration readiness check: %w", err)
	}
	defer tx.Rollback(ctx)

	var ledgerExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema()
			  AND c.relname = 'askolo_schema_migrations'
			  AND c.relkind IN ('r', 'p')
		)`).Scan(&ledgerExists); err != nil {
		return fmt.Errorf("check migration ledger: %w", err)
	}
	if !ledgerExists {
		return errors.New("migration ledger is missing")
	}

	rows, err := tx.Query(ctx, `
		SELECT version, name, checksum, schema_fingerprint
		FROM askolo_schema_migrations
		ORDER BY version`)
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	applied := 0
	var activeFingerprint string
	for rows.Next() {
		var version int
		var name, checksum, schemaFingerprint string
		if err := rows.Scan(&version, &name, &checksum, &schemaFingerprint); err != nil {
			rows.Close()
			return fmt.Errorf("read migration ledger row: %w", err)
		}
		if applied >= len(expected) {
			rows.Close()
			return fmt.Errorf("unexpected applied migration %04d", version)
		}
		if version != applied {
			rows.Close()
			return fmt.Errorf("migration history missing version %04d", applied)
		}
		if name != expected[applied].Name || checksum != expected[applied].SHA256 {
			rows.Close()
			return fmt.Errorf("migration %04d checksum, name, or ordering mismatch", version)
		}
		if strings.TrimSpace(schemaFingerprint) == "" {
			rows.Close()
			return fmt.Errorf("migration %04d has no schema fingerprint", version)
		}
		if applied == activePrefix-1 {
			activeFingerprint = schemaFingerprint
		}
		applied++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate migration ledger: %w", err)
	}
	if requireCompleteHistory && applied != len(expected) {
		return fmt.Errorf("migration history is incomplete: applied %d of %d versions", applied, len(expected))
	}
	if applied < activePrefix {
		return fmt.Errorf("migration history is incomplete: applied %d of active %d versions", applied, activePrefix)
	}

	currentFingerprint, err := fingerprintForStoredVersion(ctx, tx, activeFingerprint)
	if err != nil {
		return fmt.Errorf("fingerprint current schema: %w", err)
	}
	if currentFingerprint != activeFingerprint {
		return fmt.Errorf("managed schema drift detected: recorded %s, current %s", activeFingerprint, currentFingerprint)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("finish migration readiness check: %w", err)
	}
	return nil
}
