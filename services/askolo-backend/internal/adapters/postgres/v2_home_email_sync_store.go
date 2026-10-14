package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type V2HomeEmailSyncTarget struct {
	WorkspaceID  string
	ConnectionID string
	OwnerUserID  string
	OwnerEmail   string
	LinkedAt     time.Time
}

func (s *Store) HasV2HomeEmailActionRequired(ctx context.Context, userID string) (bool, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	var required bool
	err = s.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM `+v2Table(schema, "workspace_provider_account_access")+` a
  JOIN `+v2Table(schema, "workspace_email_summaries")+` e
    ON e.workspace_id = a.workspace_id
   AND e.provider_connection_id = a.provider_connection_id
  JOIN provider_connections c
    ON c.id = a.provider_connection_id
   AND c.user_id = a.owner_user_id
   AND c.provider = 'google'
   AND c.status = 'active'
  JOIN `+v2Table(schema, "workspace_memberships")+` m
    ON m.workspace_id = a.workspace_id
   AND m.user_id = $1
   AND m.status = 'active'
  WHERE a.owner_user_id = $1
    AND a.home_summary_enabled
    AND e.sync_action_required
)`, userID).Scan(&required)
	if err != nil {
		return false, fmt.Errorf("check Gmail action-required state: %w", err)
	}
	return required, nil
}

// ListV2HomeEmailSyncTargets returns only active, explicitly Gmail-read-enabled
// connections whose owner still belongs to the associated workspace.
func (s *Store) ListV2HomeEmailSyncTargets(ctx context.Context, limit int) ([]V2HomeEmailSyncTarget, error) {
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("invalid Home email sync batch limit")
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT a.workspace_id, a.provider_connection_id, a.owner_user_id, COALESCE(pa.email, ''), c.created_at
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
 AND c.capabilities @> ARRAY['gmail.read']::text[]
 AND c.granted_scopes @> ARRAY[$1]::text[]
JOIN provider_accounts pa ON pa.id = c.provider_account_id
JOIN `+v2Table(schema, "workspace_memberships")+` m
  ON m.workspace_id = a.workspace_id
 AND m.user_id = a.owner_user_id
 AND m.status = 'active'
LEFT JOIN `+v2Table(schema, "workspace_email_summaries")+` existing_summary
  ON existing_summary.workspace_id = a.workspace_id
 AND existing_summary.provider_connection_id = a.provider_connection_id
WHERE a.home_summary_enabled
ORDER BY COALESCE(existing_summary.last_synced_at, existing_summary.updated_at) NULLS FIRST,
         a.workspace_id, a.provider_connection_id
LIMIT $2`, "https://www.googleapis.com/auth/gmail.readonly", limit)
	if err != nil {
		return nil, fmt.Errorf("list Home email sync targets: %w", err)
	}
	defer rows.Close()
	targets := make([]V2HomeEmailSyncTarget, 0)
	for rows.Next() {
		var target V2HomeEmailSyncTarget
		if err := rows.Scan(
			&target.WorkspaceID, &target.ConnectionID, &target.OwnerUserID,
			&target.OwnerEmail, &target.LinkedAt,
		); err != nil {
			return nil, fmt.Errorf("read Home email sync target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Home email sync targets: %w", err)
	}
	return targets, nil
}

// SaveV2HomeEmailSummary rechecks eligibility at write time, so a revoked,
// disconnected, unpromoted, or departed account cannot refresh a projection.
func (s *Store) SaveV2HomeEmailSummary(
	ctx context.Context,
	target V2HomeEmailSyncTarget,
	unreadInboxCount, actionNeededCount int64,
) (bool, error) {
	if unreadInboxCount < 0 || actionNeededCount < 0 {
		return false, fmt.Errorf("invalid Home email summary counts")
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_email_summaries")+` AS current_summary
  (workspace_id, provider_connection_id, unread_inbox_count, action_needed_count, sync_state, last_synced_at, updated_at)
SELECT a.workspace_id, a.provider_connection_id, $4, $5, 'current', now(), now()
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
 AND c.capabilities @> ARRAY['gmail.read']::text[]
 AND c.granted_scopes @> ARRAY[$6]::text[]
JOIN `+v2Table(schema, "workspace_memberships")+` m
  ON m.workspace_id = a.workspace_id
 AND m.user_id = a.owner_user_id
 AND m.status = 'active'
WHERE a.workspace_id = $1
  AND a.provider_connection_id = $2
  AND a.owner_user_id = $3
  AND a.home_summary_enabled
ON CONFLICT (workspace_id, provider_connection_id) DO UPDATE SET
  unread_inbox_count = EXCLUDED.unread_inbox_count,
  action_needed_count = EXCLUDED.action_needed_count,
  sync_state = 'current',
  last_synced_at = now(),
  updated_at = now()`,
		target.WorkspaceID, target.ConnectionID, target.OwnerUserID,
		unreadInboxCount, actionNeededCount, "https://www.googleapis.com/auth/gmail.readonly")
	if err != nil {
		return false, fmt.Errorf("save Home email summary: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ClaimV2HomeEmailSummarySync is an atomic, cross-process lease. Abandoned
// claims become available again after 15 minutes.
func (s *Store) ClaimV2HomeEmailSummarySync(ctx context.Context, target V2HomeEmailSyncTarget) (bool, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	var claimed int
	err = s.pool.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "workspace_email_summaries")+`
  (workspace_id, provider_connection_id, sync_state, updated_at)
SELECT a.workspace_id, a.provider_connection_id, 'syncing', now()
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
 AND c.capabilities @> ARRAY['gmail.read']::text[]
 AND c.granted_scopes @> ARRAY[$4]::text[]
JOIN `+v2Table(schema, "workspace_memberships")+` m
  ON m.workspace_id = a.workspace_id
 AND m.user_id = a.owner_user_id
 AND m.status = 'active'
WHERE a.workspace_id = $1
  AND a.provider_connection_id = $2
  AND a.owner_user_id = $3
  AND a.home_summary_enabled
ON CONFLICT (workspace_id, provider_connection_id) DO UPDATE SET
  sync_state = 'syncing',
  updated_at = now()
WHERE current_summary.sync_state <> 'syncing'
   OR current_summary.updated_at < now() - interval '15 minutes'
RETURNING 1`,
		target.WorkspaceID, target.ConnectionID, target.OwnerUserID,
		"https://www.googleapis.com/auth/gmail.readonly").Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim Home email summary sync: %w", err)
	}
	return claimed == 1, nil
}

func (s *Store) MarkV2HomeEmailSummaryError(
	ctx context.Context, target V2HomeEmailSyncTarget, actionRequired bool,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_email_summaries")+` s
SET sync_state = 'error', sync_action_required = $5, updated_at = now()
WHERE s.workspace_id = $1 AND s.provider_connection_id = $2
  AND EXISTS (
    SELECT 1
    FROM `+v2Table(schema, "workspace_provider_account_access")+` a
    JOIN provider_connections c
      ON c.id = a.provider_connection_id
     AND c.user_id = a.owner_user_id
     AND c.provider = 'google'
     AND c.status = 'active'
     AND c.capabilities @> ARRAY['gmail.read']::text[]
     AND c.granted_scopes @> ARRAY[$3]::text[]
    JOIN `+v2Table(schema, "workspace_memberships")+` m
      ON m.workspace_id = a.workspace_id
     AND m.user_id = a.owner_user_id
     AND m.status = 'active'
    WHERE a.workspace_id = s.workspace_id
      AND a.provider_connection_id = s.provider_connection_id
      AND a.owner_user_id = $4
      AND a.home_summary_enabled
  )`, target.WorkspaceID, target.ConnectionID,
		"https://www.googleapis.com/auth/gmail.readonly", target.OwnerUserID, actionRequired)
	if err != nil {
		return fmt.Errorf("mark Home email summary sync error: %w", err)
	}
	return nil
}
