package main

import (
	"context"
	"fmt"

	"askolo/backend/internal/migrations"
	"github.com/jackc/pgx/v5"
)

// adoptExisting is deliberately separate from migrations.Run. It records a
// baseline only after the migration package has compared the complete managed
// shape; it never executes DDL or changes application data.
func adoptExisting(ctx context.Context, tx pgx.Tx) error {
	if err := migrations.ValidateBaseline(ctx, tx); err != nil {
		return fmt.Errorf("refusing baseline adoption: %w", err)
	}
	return migrations.AdoptBaseline(ctx, tx)
}
