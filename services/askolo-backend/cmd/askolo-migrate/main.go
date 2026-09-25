package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	target := flag.String("target", "", "required: development, staging, production, or restore")
	adopt := flag.Bool("adopt-existing", false, "adopt an already-created schema only after exact offline verification")
	flag.Parse()
	if *target == "" {
		log.Fatal("-target is required")
	}
	if *target != "development" && *target != "staging" && *target != "production" && *target != "restore" {
		log.Fatal("invalid migration target")
	}
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		log.Fatal("DATABASE_URL is required; migrations are explicit and never run at app startup")
	}
	if *target != "development" && strings.TrimSpace(os.Getenv("ASKOLO_MIGRATION_APPROVED")) != "yes" {
		log.Fatalf("refusing %s target without ASKOLO_MIGRATION_APPROVED=yes", *target)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
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
	if err := migrations.Run(ctx, pool); err != nil {
		log.Fatal(err)
	}
	log.Printf("migrations applied successfully (target=%s)", *target)
}
