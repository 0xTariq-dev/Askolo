package postgres

import (
	"context"
	"errors"

	"askolo/backend/internal/migrations"
)

func (s *Store) MigrationSchemaReady(ctx context.Context) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	if err := migrations.ValidateReady(ctx, s.pool); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ProductionSchemaCompatible(ctx context.Context) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	if err := migrations.ValidateProductionSchemaCompatibility(ctx, s.pool); err != nil {
		return false, err
	}
	return true, nil
}
