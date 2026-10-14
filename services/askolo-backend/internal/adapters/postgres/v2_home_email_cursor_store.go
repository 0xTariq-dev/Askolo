package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type V2HomeEmailSyncCheckpoint struct {
	HistoryID         string
	BaselineHistoryID string
	CutoffAt          time.Time
	Phase             string
	BaselinePageToken string
	HistoryPageToken  string
	KeyVersion        int16
	KeyFingerprint    string
}

type V2HomeEmailThreadState struct {
	Token               string
	UnreadMessageCount  int32
	InInboxWindow       bool
	LatestSenderIsOwner bool
	FlaggedInWindow     bool
	LastActivityAt      time.Time
}

func (s *Store) LoadV2HomeEmailSyncCheckpoint(
	ctx context.Context, target V2HomeEmailSyncTarget,
) (V2HomeEmailSyncCheckpoint, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2HomeEmailSyncCheckpoint{}, err
	}
	var checkpoint V2HomeEmailSyncCheckpoint
	err = s.pool.QueryRow(ctx, `
SELECT COALESCE(gmail_history_id, ''), COALESCE(baseline_history_id, ''),
       COALESCE(baseline_cutoff_at, 'epoch'::timestamptz), baseline_phase,
       COALESCE(baseline_page_token, ''), COALESCE(history_page_token, ''),
       COALESCE(thread_state_key_version, 0), COALESCE(thread_state_key_fingerprint, '')
FROM `+v2Table(schema, "workspace_email_summaries")+`
WHERE workspace_id = $1 AND provider_connection_id = $2`,
		target.WorkspaceID, target.ConnectionID,
	).Scan(&checkpoint.HistoryID, &checkpoint.BaselineHistoryID,
		&checkpoint.CutoffAt, &checkpoint.Phase, &checkpoint.BaselinePageToken,
		&checkpoint.HistoryPageToken, &checkpoint.KeyVersion, &checkpoint.KeyFingerprint)
	if err != nil {
		return V2HomeEmailSyncCheckpoint{}, fmt.Errorf("load Home email sync checkpoint: %w", err)
	}
	return checkpoint, nil
}

// StartV2HomeEmailBaseline preserves the last published counts while replacing
// the opaque thread index and initializing a resumable baseline.
func (s *Store) StartV2HomeEmailBaseline(
	ctx context.Context, target V2HomeEmailSyncTarget, historyID string, cutoff time.Time,
	keyVersion int16, keyFingerprint string,
) (bool, error) {
	if historyID == "" || cutoff.IsZero() || keyVersion < 1 || len(keyFingerprint) != 64 {
		return false, errors.New("invalid Home email baseline checkpoint")
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin Home email baseline: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_email_summary_thread_state")+`
WHERE workspace_id = $1 AND provider_connection_id = $2`,
		target.WorkspaceID, target.ConnectionID)
	if err != nil {
		return false, fmt.Errorf("reset Home email thread index: %w", err)
	}
	_ = tag
	result, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_email_summaries")+` s
SET gmail_history_id = NULL, baseline_history_id = $3, baseline_cutoff_at = $4,
    baseline_phase = 'inbox', baseline_page_token = NULL, history_page_token = NULL,
    thread_state_key_version = $5, thread_state_key_fingerprint = $6,
    sync_action_required = false, sync_state = 'syncing', updated_at = now()
WHERE s.workspace_id = $1 AND s.provider_connection_id = $2
  AND EXISTS (
    SELECT 1 FROM `+v2Table(schema, "workspace_provider_account_access")+` a
    JOIN provider_connections c ON c.id = a.provider_connection_id
      AND c.user_id = a.owner_user_id AND c.provider = 'google'
      AND c.status = 'active' AND c.capabilities @> ARRAY['gmail.read']::text[]
      AND c.granted_scopes @> ARRAY[$7]::text[]
    JOIN `+v2Table(schema, "workspace_memberships")+` m
      ON m.workspace_id = a.workspace_id AND m.user_id = a.owner_user_id AND m.status = 'active'
    WHERE a.workspace_id = s.workspace_id AND a.provider_connection_id = s.provider_connection_id
      AND a.owner_user_id = $8 AND a.home_summary_enabled
  )`,
		target.WorkspaceID, target.ConnectionID, historyID, cutoff, keyVersion,
		keyFingerprint, "https://www.googleapis.com/auth/gmail.readonly", target.OwnerUserID)
	if err != nil {
		return false, fmt.Errorf("start Home email baseline: %w", err)
	}
	if result.RowsAffected() == 0 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit Home email baseline: %w", err)
	}
	return true, nil
}

func (s *Store) SaveV2HomeEmailBaselinePage(
	ctx context.Context, target V2HomeEmailSyncTarget, states []V2HomeEmailThreadState,
	nextPageToken string,
) (bool, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin Home email baseline page: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, state := range states {
		if err := upsertHomeEmailThreadState(ctx, tx, schema, target, state); err != nil {
			return false, err
		}
	}
	nextPhase := "inbox"
	var pageToken any
	if nextPageToken != "" {
		pageToken = nextPageToken
	} else {
		nextPhase = "history"
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_email_summaries")+` s
SET baseline_phase = $3, baseline_page_token = $4, updated_at = now()
WHERE s.workspace_id = $1 AND s.provider_connection_id = $2
  AND s.baseline_phase = 'inbox'
  AND EXISTS (
    SELECT 1 FROM `+v2Table(schema, "workspace_provider_account_access")+` a
    JOIN provider_connections c ON c.id = a.provider_connection_id
      AND c.user_id = a.owner_user_id AND c.provider = 'google'
      AND c.status = 'active' AND c.capabilities @> ARRAY['gmail.read']::text[]
      AND c.granted_scopes @> ARRAY[$5]::text[]
    JOIN `+v2Table(schema, "workspace_memberships")+` m
      ON m.workspace_id = a.workspace_id AND m.user_id = a.owner_user_id AND m.status = 'active'
    WHERE a.workspace_id = s.workspace_id AND a.provider_connection_id = s.provider_connection_id
      AND a.owner_user_id = $6 AND a.home_summary_enabled
  )`,
		target.WorkspaceID, target.ConnectionID, nextPhase, pageToken,
		"https://www.googleapis.com/auth/gmail.readonly", target.OwnerUserID)
	if err != nil {
		return false, fmt.Errorf("checkpoint Home email baseline page: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if nextPhase == "history" {
		if err := refreshHomeEmailSummaryCounts(ctx, tx, schema, target); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit Home email baseline page: %w", err)
	}
	return true, nil
}

func (s *Store) SaveV2HomeEmailHistoryPage(
	ctx context.Context, target V2HomeEmailSyncTarget, states []V2HomeEmailThreadState,
	removeTokens []string, nextPageToken, latestHistoryID string, baseline bool,
) (bool, error) {
	if latestHistoryID == "" {
		return false, errors.New("invalid Gmail history checkpoint")
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin Home email history page: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, state := range states {
		if err := upsertHomeEmailThreadState(ctx, tx, schema, target, state); err != nil {
			return false, err
		}
	}
	for _, token := range removeTokens {
		if len(token) != 64 {
			return false, errors.New("invalid Home email thread token to remove")
		}
		if _, err := tx.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_email_summary_thread_state")+`
WHERE workspace_id = $1 AND provider_connection_id = $2 AND thread_token = $3`,
			target.WorkspaceID, target.ConnectionID, token); err != nil {
			return false, fmt.Errorf("remove Home email thread state: %w", err)
		}
	}
	var historyPage any
	if nextPageToken != "" {
		historyPage = nextPageToken
	}
	var historyID any
	if nextPageToken == "" {
		historyID = latestHistoryID
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_email_summaries")+` s
SET baseline_phase = CASE WHEN $3 THEN 'complete' ELSE baseline_phase END,
    gmail_history_id = COALESCE($4, gmail_history_id),
    history_page_token = $5,
    sync_action_required = CASE WHEN $5 IS NULL THEN false ELSE sync_action_required END,
    sync_state = CASE WHEN $5 IS NULL THEN 'current' ELSE 'syncing' END,
    last_synced_at = CASE WHEN $5 IS NULL THEN now() ELSE last_synced_at END,
    updated_at = now()
WHERE s.workspace_id = $1 AND s.provider_connection_id = $2
  AND EXISTS (
    SELECT 1 FROM `+v2Table(schema, "workspace_provider_account_access")+` a
    JOIN provider_connections c ON c.id = a.provider_connection_id
      AND c.user_id = a.owner_user_id AND c.provider = 'google'
      AND c.status = 'active' AND c.capabilities @> ARRAY['gmail.read']::text[]
      AND c.granted_scopes @> ARRAY[$6]::text[]
    JOIN `+v2Table(schema, "workspace_memberships")+` m
      ON m.workspace_id = a.workspace_id AND m.user_id = a.owner_user_id AND m.status = 'active'
    WHERE a.workspace_id = s.workspace_id AND a.provider_connection_id = s.provider_connection_id
      AND a.owner_user_id = $7 AND a.home_summary_enabled
  )`,
		target.WorkspaceID, target.ConnectionID, baseline && nextPageToken == "",
		historyID, historyPage, "https://www.googleapis.com/auth/gmail.readonly", target.OwnerUserID)
	if err != nil {
		return false, fmt.Errorf("checkpoint Home email history page: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if err := refreshHomeEmailSummaryCounts(ctx, tx, schema, target); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit Home email history page: %w", err)
	}
	return true, nil
}

func upsertHomeEmailThreadState(
	ctx context.Context, tx pgx.Tx, schema string, target V2HomeEmailSyncTarget, state V2HomeEmailThreadState,
) error {
	if len(state.Token) != 64 || state.UnreadMessageCount < 0 || state.LastActivityAt.IsZero() {
		return errors.New("invalid Home email thread state")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_email_summary_thread_state")+`
  (workspace_id, provider_connection_id, thread_token, unread_message_count,
   in_inbox_window, latest_sender_is_owner, flagged_in_window, last_activity_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (workspace_id, provider_connection_id, thread_token) DO UPDATE SET
  unread_message_count = EXCLUDED.unread_message_count,
  in_inbox_window = EXCLUDED.in_inbox_window,
  latest_sender_is_owner = EXCLUDED.latest_sender_is_owner,
  flagged_in_window = EXCLUDED.flagged_in_window,
  last_activity_at = EXCLUDED.last_activity_at, updated_at = now()`,
		target.WorkspaceID, target.ConnectionID, state.Token, state.UnreadMessageCount,
		state.InInboxWindow, state.LatestSenderIsOwner, state.FlaggedInWindow, state.LastActivityAt)
	if err != nil {
		return fmt.Errorf("save Home email thread state: %w", err)
	}
	return nil
}

func refreshHomeEmailSummaryCounts(
	ctx context.Context, tx pgx.Tx, schema string, target V2HomeEmailSyncTarget,
) error {
	_, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_email_summaries")+` s
SET unread_inbox_count = totals.unread_count,
    action_needed_count = totals.action_count,
    updated_at = now()
FROM (
  SELECT COALESCE(sum(unread_message_count), 0)::integer AS unread_count,
         count(*) FILTER (WHERE (in_inbox_window AND NOT latest_sender_is_owner)
                            OR flagged_in_window)::integer AS action_count
  FROM `+v2Table(schema, "workspace_email_summary_thread_state")+`
  WHERE workspace_id = $1 AND provider_connection_id = $2
) totals
WHERE s.workspace_id = $1 AND s.provider_connection_id = $2`,
		target.WorkspaceID, target.ConnectionID)
	if err != nil {
		return fmt.Errorf("refresh Home email summary counts: %w", err)
	}
	return nil
}
