package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	target := flag.String("target", "", "required: development or restore (never staging or production)")
	adopt := flag.Bool("adopt-existing", false, "adopt an already-created schema only after exact offline verification")
	reconcileFingerprint := flag.Bool("reconcile-operational-fingerprint", false,
		"reconcile only the active Development ledger fingerprint after exact 0000-0015 history validation")
	flag.Parse()
	if *target == "" {
		log.Fatal("-target is required")
	}
	if *target != "development" && *target != "restore" {
		log.Fatal("invalid migration target")
	}
	url := strings.TrimSpace(os.Getenv("ASKOLO_MIGRATION_DATABASE_URL"))
	if url == "" {
		log.Fatal("ASKOLO_MIGRATION_DATABASE_URL is required; do not use the application DATABASE_URL")
	}
	if applicationURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); applicationURL != "" && url == applicationURL {
		log.Fatal("refusing to run migrations against the application DATABASE_URL")
	}
	if *target != "development" && strings.TrimSpace(os.Getenv("ASKOLO_MIGRATION_APPROVED")) != "yes" {
		log.Fatalf("refusing %s target without ASKOLO_MIGRATION_APPROVED=yes", *target)
	}
	if *adopt && *reconcileFingerprint {
		log.Fatal("-adopt-existing and -reconcile-operational-fingerprint cannot be used together")
	}
	if *reconcileFingerprint {
		if *target != "development" {
			log.Fatal("-reconcile-operational-fingerprint is allowed only with -target development")
		}
		if strings.TrimSpace(os.Getenv("ASKOLO_MIGRATION_APPROVED")) != "yes" {
			log.Fatal("refusing fingerprint reconciliation without ASKOLO_MIGRATION_APPROVED=yes")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		log.Fatal(err)
	}
	if *reconcileFingerprint {
		previousAfterConnect := poolConfig.AfterConnect
		poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			if previousAfterConnect != nil {
				if err := previousAfterConnect(ctx, conn); err != nil {
					return err
				}
			}
			_, err := conn.Exec(ctx, `SET search_path = public`)
			return err
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if *reconcileFingerprint {
		if err := validateDevelopmentReconciliationTarget(ctx, pool); err != nil {
			log.Fatal(err)
		}
		result, err := migrations.ReconcileOperationalFingerprint(ctx, pool)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("active migration fingerprint reconciliation complete (changed=%t previous=%s current=%s)",
			result.Changed, result.Previous, result.Current)
		return
	}
	if *adopt {
		conn, acquireErr := pool.Acquire(ctx)
		if acquireErr != nil {
			log.Fatal(acquireErr)
		}
		defer conn.Release()
		tx, beginErr := conn.Begin(ctx)
		if beginErr != nil {
			log.Fatal(beginErr)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7261736, 1)`); err != nil {
			log.Fatal(err)
		}
		if err := adoptExisting(ctx, tx); err != nil {
			log.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatal(err)
		}
		log.Printf("existing baseline adopted after exact fingerprint match")
		return
	}
	if *target == "development" {
		err = migrations.RunOperational(ctx, pool)
	} else {
		err = migrations.Run(ctx, pool)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("migrations applied successfully (target=%s)", *target)
}

func validateDevelopmentReconciliationTarget(ctx context.Context, pool *pgxpool.Pool) error {
	var databaseName, schemaName string
	var majorVersion int
	if err := pool.QueryRow(ctx, `
		SELECT current_database(), current_schema(), current_setting('server_version_num')::integer / 10000
	`).Scan(&databaseName, &schemaName, &majorVersion); err != nil {
		return fmt.Errorf("verify Development reconciliation target: %w", err)
	}
	return validateDevelopmentReconciliationIdentity(databaseName, schemaName, majorVersion)
}

func validateDevelopmentReconciliationIdentity(databaseName, schemaName string, majorVersion int) error {
	if databaseName != "heliumdb" || schemaName != "public" || majorVersion != 16 {
		return fmt.Errorf("refusing fingerprint reconciliation outside the verified Development target (database=%q schema=%q PostgreSQL=%d)",
			databaseName, schemaName, majorVersion)
	}
	return nil
}
