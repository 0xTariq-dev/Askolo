package main

import (
	"context"
	"fmt"

	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5"
)

// adoptExisting is deliberately separate from migrations.Run. Adoption
// validates the pinned schema, then creates only the migration ledger and
// records the complete legacy/auth prefix in the caller's transaction.
func adoptExisting(ctx context.Context, tx pgx.Tx) error {
	if err := migrations.AdoptBaseline(ctx, tx); err != nil {
		return fmt.Errorf("refusing baseline adoption: %w", err)
	}
	return nil
}
