package postgres

import "context"

func (s *Store) AuthorizationSchemaReady(ctx context.Context) bool {
	if s == nil || s.pool == nil {
		return false
	}
	var ready bool
	if err := s.pool.QueryRow(ctx, `
		SELECT to_regclass('public.workspaces') IS NOT NULL
		   AND to_regclass('public.workspace_memberships') IS NOT NULL
		   AND to_regclass('public.authorization_resources') IS NOT NULL
	`).Scan(&ready); err != nil {
		return false
	}
	return ready
}
