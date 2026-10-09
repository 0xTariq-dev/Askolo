package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrV2HomeSpaceLimit        = errors.New("Home supports at most 100 active workspaces")
	ErrV2InvalidHomeFocus      = errors.New("invalid Home focus selection")
	ErrV2InvalidProviderAccess = errors.New("invalid provider access scope")
)

type V2HomeWorkspace struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsPersonal bool   `json:"isPersonal"`
	Role       string `json:"role"`
}

type V2HomeFocusReference struct {
	TaskID       string
	OccurrenceOn *time.Time
	Position     int
}

type V2HomeFocusSelection struct {
	TaskID       string     `json:"taskId"`
	OccurrenceOn *time.Time `json:"occurrenceOn"`
	Position     int        `json:"position"`
}

type V2HomeEmailSummary struct {
	UnreadInboxCount  int64      `json:"unreadInboxCount"`
	ActionNeededCount int64      `json:"actionNeededCount"`
	LastSyncedAt      *time.Time `json:"lastSyncedAt"`
	Freshness         string     `json:"freshness"`
}

type V2HomeProviderConnection struct {
	ConnectionID       string `json:"connectionId"`
	AccountLabel       string `json:"accountLabel"`
	AccessScope        string `json:"access"`
	OwnedByViewer      bool   `json:"ownedByViewer"`
	CanReadMessages    bool   `json:"canReadMessages"`
	HomeSummaryEnabled bool   `json:"homeSummaryEnabled"`
}

func (s *Store) ListV2HomeWorkspaces(ctx context.Context, userID string) ([]V2HomeWorkspace, error) {
	rows, err := s.pool.Query(ctx, `
SELECT w.id, w.name, w.is_personal, m.role
FROM workspaces w
JOIN workspace_memberships m ON m.workspace_id = w.id
WHERE m.user_id = $1 AND m.status = 'active' AND w.status = 'active'
ORDER BY w.is_personal DESC, w.created_at, w.id
LIMIT 101`, userID)
	if err != nil {
		return nil, fmt.Errorf("list Home workspaces: %w", err)
	}
	defer rows.Close()

	workspaces := make([]V2HomeWorkspace, 0)
	for rows.Next() {
		var workspace V2HomeWorkspace
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.IsPersonal, &workspace.Role); err != nil {
			return nil, fmt.Errorf("read Home workspace: %w", err)
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Home workspaces: %w", err)
	}
	if len(workspaces) > 100 {
		return nil, ErrV2HomeSpaceLimit
	}
	return workspaces, nil
}

func (s *Store) V2HomeTimeZone(ctx context.Context, userID string) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	var timeZone string
	err = s.pool.QueryRow(ctx, `
SELECT COALESCE(time_zone, 'UTC')
FROM `+v2Table(schema, "member_preferences")+`
WHERE user_id = $1`, userID).Scan(&timeZone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "UTC", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Home time zone: %w", err)
	}
	if _, err := time.LoadLocation(timeZone); err != nil {
		return "", fmt.Errorf("stored Home time zone is invalid: %w", err)
	}
	return timeZone, nil
}

func (s *Store) V2WorkspaceName(ctx context.Context, workspaceID string) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	var name string
	err = s.pool.QueryRow(ctx, `
SELECT name
FROM `+v2Table(schema, "workspaces")+`
WHERE id = $1 AND status = 'active'`, workspaceID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read Home workspace name: %w", err)
	}
	return name, nil
}

func (s *Store) ListV2HomeFocusSelections(
	ctx context.Context,
	workspaceID, userID string,
	planDay time.Time,
) ([]V2HomeFocusSelection, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT task_id, occurrence_on, position
FROM `+v2Table(schema, "home_focus_selections")+`
WHERE workspace_id = $1 AND user_id = $2 AND plan_day = $3
ORDER BY position`, workspaceID, userID, planDay.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("list Home focus selections: %w", err)
	}
	defer rows.Close()
	selections := make([]V2HomeFocusSelection, 0, 3)
	for rows.Next() {
		var selection V2HomeFocusSelection
		if err := rows.Scan(&selection.TaskID, &selection.OccurrenceOn, &selection.Position); err != nil {
			return nil, fmt.Errorf("read Home focus selection: %w", err)
		}
		selections = append(selections, selection)
	}
	return selections, rows.Err()
}

func (s *Store) ReplaceV2HomeFocusSelections(
	ctx context.Context,
	workspaceID, userID string,
	planDay time.Time,
	selections []V2HomeFocusReference,
) error {
	if len(selections) > 3 {
		return ErrV2InvalidHomeFocus
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Home focus replacement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, userID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		key := selection.TaskID
		if selection.OccurrenceOn != nil {
			key += ":" + selection.OccurrenceOn.Format("2006-01-02")
		}
		if _, exists := seen[key]; exists {
			return ErrV2InvalidHomeFocus
		}
		seen[key] = struct{}{}

		var status string
		var recurrence []byte
		err := tx.QueryRow(ctx, `
SELECT status, recurrence
FROM `+v2Table(schema, "tasks")+`
WHERE workspace_id = $1 AND id = $2
FOR SHARE`, workspaceID, selection.TaskID).Scan(&status, &recurrence)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("validate Home focus task: %w", err)
		}
		if status == "completed" || status == "done" || status == "cancelled" || status == "archived" {
			return ErrV2InvalidHomeFocus
		}
		if (len(recurrence) > 0) != (selection.OccurrenceOn != nil) {
			return ErrV2InvalidHomeFocus
		}
		if selection.OccurrenceOn != nil {
			var occurrenceStatus string
			err := tx.QueryRow(ctx, `
SELECT status
FROM `+v2Table(schema, "task_occurrences")+`
WHERE workspace_id = $1 AND task_id = $2 AND occurrence_on = $3`,
				workspaceID, selection.TaskID, selection.OccurrenceOn.Format("2006-01-02"),
			).Scan(&occurrenceStatus)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("validate Home focus occurrence: %w", err)
			}
			if occurrenceStatus == "completed" || occurrenceStatus == "done" ||
				occurrenceStatus == "cancelled" || occurrenceStatus == "skipped" {
				return ErrV2InvalidHomeFocus
			}
		}
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM `+v2Table(schema, "home_focus_selections")+`
WHERE workspace_id = $1 AND user_id = $2 AND plan_day = $3`,
		workspaceID, userID, planDay.Format("2006-01-02")); err != nil {
		return fmt.Errorf("replace Home focus selections: %w", err)
	}
	for _, selection := range selections {
		_, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "home_focus_selections")+`
(workspace_id, user_id, plan_day, task_id, occurrence_on, position)
VALUES ($1, $2, $3, $4, $5, $6)`,
			workspaceID, userID, planDay.Format("2006-01-02"),
			selection.TaskID, selection.OccurrenceOn, selection.Position)
		if err != nil {
			return fmt.Errorf("save Home focus selection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Home focus replacement: %w", err)
	}
	return nil
}

func (s *Store) ListV2HomeEmailSummaries(
	ctx context.Context,
	userID string,
	workspaceIDs []string,
) (map[string]V2HomeEmailSummary, error) {
	result := make(map[string]V2HomeEmailSummary, len(workspaceIDs))
	if len(workspaceIDs) == 0 {
		return result, nil
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT a.workspace_id,
       COALESCE(SUM(s.unread_inbox_count), 0),
       COALESCE(SUM(s.action_needed_count), 0),
       MAX(s.last_synced_at),
       CASE
         WHEN COUNT(*) FILTER (WHERE s.sync_state = 'error') > 0 THEN 'error'
         WHEN COUNT(*) FILTER (WHERE s.sync_state = 'syncing') > 0 THEN 'syncing'
         WHEN BOOL_AND(s.sync_state = 'current') THEN 'current'
         WHEN BOOL_AND(s.sync_state = 'unavailable') THEN 'unavailable'
         ELSE 'stale'
       END
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN `+v2Table(schema, "workspace_email_summaries")+` s
  ON s.workspace_id = a.workspace_id
 AND s.provider_connection_id = a.provider_connection_id
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
  AND c.granted_scopes @> ARRAY[$3]::text[]
  AND c.capabilities @> ARRAY['gmail.read']::text[]
JOIN `+v2Table(schema, "workspace_memberships")+` viewer
  ON viewer.workspace_id = a.workspace_id
 AND viewer.user_id = $1
 AND viewer.status = 'active'
JOIN `+v2Table(schema, "workspace_memberships")+` account_owner
  ON account_owner.workspace_id = a.workspace_id
 AND account_owner.user_id = a.owner_user_id
 AND account_owner.status = 'active'
WHERE a.workspace_id = ANY($2::text[])
  AND a.home_summary_enabled
  AND (a.access_scope = 'space_shared' OR a.owner_user_id = $1)
 GROUP BY a.workspace_id`, userID, workspaceIDs, gmailReadScope)
	if err != nil {
		return nil, fmt.Errorf("read Home email summaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID string
		var summary V2HomeEmailSummary
		if err := rows.Scan(
			&workspaceID,
			&summary.UnreadInboxCount,
			&summary.ActionNeededCount,
			&summary.LastSyncedAt,
			&summary.Freshness,
		); err != nil {
			return nil, fmt.Errorf("read Home email summary: %w", err)
		}
		result[workspaceID] = summary
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Home email summaries: %w", err)
	}
	return result, nil
}

func (s *Store) AssociateV2ProviderConnection(
	ctx context.Context,
	workspaceID, ownerUserID, connectionID string,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider association: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, ownerUserID); err != nil {
		return err
	}
	var connectionStatus, provider string
	err = tx.QueryRow(ctx, `
SELECT status, provider
FROM provider_connections
WHERE id = $1 AND user_id = $2
FOR SHARE`, connectionID, ownerUserID).Scan(&connectionStatus, &provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("verify provider connection ownership: %w", err)
	}
	if provider != "google" || connectionStatus != "active" {
		return ErrNotFound
	}

	var existingWorkspace, existingOwner string
	err = tx.QueryRow(ctx, `
SELECT workspace_id, owner_user_id
FROM `+v2Table(schema, "workspace_provider_account_access")+`
WHERE provider_connection_id = $1
FOR UPDATE`, connectionID).Scan(&existingWorkspace, &existingOwner)
	if err == nil {
		if existingWorkspace == workspaceID && existingOwner == ownerUserID {
			return tx.Commit(ctx)
		}
		return ErrOwnership
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check existing provider association: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_provider_account_access")+`
(workspace_id, provider_connection_id, owner_user_id)
VALUES ($1, $2, $3)`, workspaceID, connectionID, ownerUserID); err != nil {
		return fmt.Errorf("associate provider connection: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider association: %w", err)
	}
	return nil
}

func (s *Store) RemoveV2ProviderConnection(
	ctx context.Context,
	workspaceID, actorUserID, role, connectionID string,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider disassociation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, actorUserID); err != nil {
		return err
	}
	canManage := role == "owner" || role == "admin"
	tag, err := tx.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_provider_account_access")+`
WHERE workspace_id = $1 AND provider_connection_id = $2
  AND (owner_user_id = $3 OR $4)`,
		workspaceID, connectionID, actorUserID, canManage)
	if err != nil {
		return fmt.Errorf("remove provider association: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider disassociation: %w", err)
	}
	return nil
}

func (s *Store) SetV2ProviderAccess(
	ctx context.Context,
	workspaceID, actorUserID, role, connectionID, access string,
) error {
	if access != "owner_only" && access != "space_shared" {
		return ErrV2InvalidProviderAccess
	}
	if role != "owner" && role != "admin" {
		return ErrV2Forbidden
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider access update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, actorUserID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_provider_account_access")+` a
SET access_scope = $4,
    promoted_by_user_id = CASE WHEN $4 = 'space_shared' THEN $3 ELSE NULL END,
    promoted_at = CASE WHEN $4 = 'space_shared' THEN now() ELSE NULL END,
    email_read_consent_at = CASE WHEN $4 = 'space_shared' THEN a.email_read_consent_at ELSE NULL END,
    email_read_consent_policy_version = CASE WHEN $4 = 'space_shared' THEN a.email_read_consent_policy_version ELSE NULL END,
    email_read_consent_revoked_at = CASE WHEN $4 = 'space_shared' THEN a.email_read_consent_revoked_at ELSE NULL END,
    updated_at = now()
FROM provider_connections c
JOIN `+v2Table(schema, "workspace_memberships")+` owner_member
  ON owner_member.workspace_id = a.workspace_id
 AND owner_member.user_id = a.owner_user_id
 AND owner_member.status = 'active'
WHERE a.workspace_id = $1
  AND a.provider_connection_id = $2
  AND c.id = a.provider_connection_id
  AND c.user_id = a.owner_user_id
  AND c.provider = 'google'
  AND c.status = 'active'`,
		workspaceID, connectionID, actorUserID, access)
	if err != nil {
		return fmt.Errorf("update provider access: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider access update: %w", err)
	}
	return nil
}

const V2HomeEmailConsentPolicyVersion = "v1"
const gmailReadScope = "https://www.googleapis.com/auth/gmail.readonly"

func (s *Store) SetV2EmailReadConsent(
	ctx context.Context,
	workspaceID, ownerUserID, connectionID string,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_provider_account_access")+` a
SET email_read_consent_at = now(),
    email_read_consent_policy_version = $4,
    email_read_consent_revoked_at = NULL,
    updated_at = now()
FROM provider_connections c
WHERE a.workspace_id = $1
  AND a.provider_connection_id = $2
  AND a.owner_user_id = $3
  AND a.access_scope = 'space_shared'
  AND c.id = a.provider_connection_id
  AND c.user_id = a.owner_user_id
  AND c.provider = 'google'
  AND c.status = 'active'
  AND c.granted_scopes @> ARRAY[$5]::text[]
  AND c.capabilities @> ARRAY['gmail.read']::text[]`,
		workspaceID, connectionID, ownerUserID, V2HomeEmailConsentPolicyVersion, gmailReadScope)
	if err != nil {
		return fmt.Errorf("record email read consent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrV2Forbidden
	}
	return nil
}

func (s *Store) RevokeV2EmailReadConsent(
	ctx context.Context,
	workspaceID, ownerUserID, connectionID string,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_provider_account_access")+`
SET email_read_consent_revoked_at = now(), updated_at = now()
WHERE workspace_id = $1 AND provider_connection_id = $2
  AND owner_user_id = $3
  AND email_read_consent_at IS NOT NULL
  AND email_read_consent_revoked_at IS NULL`,
		workspaceID, connectionID, ownerUserID)
	if err != nil {
		return fmt.Errorf("revoke email read consent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListV2ProviderConnections(
	ctx context.Context,
	workspaceID, viewerUserID string,
) ([]V2HomeProviderConnection, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT a.provider_connection_id,
       CASE WHEN a.owner_user_id = $2
            THEN COALESCE(NULLIF(account.display_name, ''), NULLIF(account.email, ''), 'Google account')
            ELSE 'Shared Google account' END,
       a.access_scope,
       a.owner_user_id = $2,
       (a.access_scope = 'space_shared'
        AND a.email_read_consent_at IS NOT NULL
        AND a.email_read_consent_revoked_at IS NULL
        AND a.email_read_consent_policy_version = $3
        AND c.granted_scopes @> ARRAY[$4]::text[]
        AND c.capabilities @> ARRAY['gmail.read']::text[]),
       a.home_summary_enabled
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
JOIN provider_accounts account ON account.id = c.provider_account_id
JOIN `+v2Table(schema, "workspace_memberships")+` viewer
  ON viewer.workspace_id = a.workspace_id
 AND viewer.user_id = $2
 AND viewer.status = 'active'
JOIN `+v2Table(schema, "workspace_memberships")+` owner_member
  ON owner_member.workspace_id = a.workspace_id
 AND owner_member.user_id = a.owner_user_id
 AND owner_member.status = 'active'
WHERE a.workspace_id = $1
  AND (a.access_scope = 'space_shared' OR a.owner_user_id = $2)
ORDER BY a.created_at, a.provider_connection_id`,
		workspaceID, viewerUserID, V2HomeEmailConsentPolicyVersion, gmailReadScope)
	if err != nil {
		return nil, fmt.Errorf("list workspace provider connections: %w", err)
	}
	defer rows.Close()
	connections := make([]V2HomeProviderConnection, 0)
	for rows.Next() {
		var connection V2HomeProviderConnection
		if err := rows.Scan(
			&connection.ConnectionID,
			&connection.AccountLabel,
			&connection.AccessScope,
			&connection.OwnedByViewer,
			&connection.CanReadMessages,
			&connection.HomeSummaryEnabled,
		); err != nil {
			return nil, fmt.Errorf("read workspace provider connection: %w", err)
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace provider connections: %w", err)
	}
	return connections, nil
}

func (s *Store) V2SharedEmailOwner(
	ctx context.Context,
	workspaceID, viewerUserID, connectionID string,
) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	var ownerUserID string
	err = s.pool.QueryRow(ctx, `
SELECT a.owner_user_id
FROM `+v2Table(schema, "workspace_provider_account_access")+` a
JOIN provider_connections c
  ON c.id = a.provider_connection_id
 AND c.user_id = a.owner_user_id
 AND c.provider = 'google'
 AND c.status = 'active'
JOIN `+v2Table(schema, "workspace_memberships")+` viewer
  ON viewer.workspace_id = a.workspace_id
 AND viewer.user_id = $2
 AND viewer.status = 'active'
JOIN `+v2Table(schema, "workspace_memberships")+` owner_member
  ON owner_member.workspace_id = a.workspace_id
 AND owner_member.user_id = a.owner_user_id
 AND owner_member.status = 'active'
WHERE a.workspace_id = $1
  AND a.provider_connection_id = $3
  AND a.access_scope = 'space_shared'
  AND a.email_read_consent_at IS NOT NULL
  AND a.email_read_consent_revoked_at IS NULL
  AND a.email_read_consent_policy_version = $4
  AND c.granted_scopes @> ARRAY[$5]::text[]
  AND c.capabilities @> ARRAY['gmail.read']::text[]`,
		workspaceID, viewerUserID, connectionID, V2HomeEmailConsentPolicyVersion, gmailReadScope,
	).Scan(&ownerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("authorize shared email read: %w", err)
	}
	return ownerUserID, nil
}
