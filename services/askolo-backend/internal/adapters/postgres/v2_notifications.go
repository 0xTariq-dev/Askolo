package postgres

import (
	"context"
	"fmt"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

type V2Notification struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	Workspace   string     `json:"workspaceName"`
	EventKey    string     `json:"eventKey"`
	ResourceID  string     `json:"resourceId"`
	Title       string     `json:"title"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt"`
}

func (s *Store) ListV2Notifications(ctx context.Context, userID string) ([]V2Notification, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT n.id, n.workspace_id, w.name, n.event_key, n.resource_id,
       COALESCE(CASE WHEN n.event_key='membership' THEN 'Member or role change' END, t.title, 'Space update'),
       n.created_at, n.read_at
FROM `+v2Table(schema, "workspace_notifications")+` n
JOIN `+v2Table(schema, "workspace_memberships")+` m
  ON m.workspace_id=n.workspace_id AND m.user_id=n.recipient_user_id AND m.status='active'
JOIN `+v2Table(schema, "workspaces")+` w ON w.id=n.workspace_id AND w.status='active'
LEFT JOIN `+v2Table(schema, "tasks")+` t
  ON n.resource_type='task' AND t.workspace_id=n.workspace_id AND t.id=n.resource_id
WHERE n.recipient_user_id=$1
ORDER BY n.created_at DESC, n.id DESC
LIMIT 100`, userID)
	if err != nil {
		return nil, fmt.Errorf("list Space notifications: %w", err)
	}
	defer rows.Close()
	result := make([]V2Notification, 0)
	for rows.Next() {
		var item V2Notification
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Workspace, &item.EventKey,
			&item.ResourceID, &item.Title, &item.CreatedAt, &item.ReadAt); err != nil {
			return nil, fmt.Errorf("scan Space notification: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) MarkV2NotificationRead(ctx context.Context, userID, notificationID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_notifications")+` n
SET read_at=COALESCE(read_at, now())
WHERE n.id=$1 AND n.recipient_user_id=$2
  AND EXISTS (
    SELECT 1 FROM `+v2Table(schema, "workspace_memberships")+` m
    WHERE m.workspace_id=n.workspace_id AND m.user_id=$2 AND m.status='active'
  )`, notificationID, userID)
	if err != nil {
		return fmt.Errorf("mark Space notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllV2NotificationsRead(ctx context.Context, userID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_notifications")+` n SET read_at=now()
WHERE n.recipient_user_id=$1 AND n.read_at IS NULL
  AND EXISTS (
    SELECT 1 FROM `+v2Table(schema, "workspace_memberships")+` m
    WHERE m.workspace_id=n.workspace_id AND m.user_id=$1 AND m.status='active'
  )`, userID)
	if err != nil {
		return fmt.Errorf("mark Space notifications read: %w", err)
	}
	return nil
}

func (s *Store) ClearV2Notifications(ctx context.Context, userID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_notifications")+` n
WHERE n.recipient_user_id=$1 AND EXISTS (
  SELECT 1 FROM `+v2Table(schema, "workspace_memberships")+` m
  WHERE m.workspace_id=n.workspace_id AND m.user_id=$1 AND m.status='active'
)`, userID)
	if err != nil {
		return fmt.Errorf("clear Space notifications: %w", err)
	}
	return nil
}

func (s *Store) ListV2AssignmentEmailRecipients(
	ctx context.Context, workspaceID, actorID string, userID, groupID *string,
) ([]string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT u.email
FROM `+v2Table(schema, "workspace_memberships")+` m
JOIN users u ON u.id=m.user_id AND u.email IS NOT NULL AND u.email_verified_at IS NOT NULL
WHERE m.workspace_id=$1 AND m.status='active' AND m.user_id<>$2
  AND (
    ($3::varchar IS NOT NULL AND m.user_id=$3)
    OR ($4::text IS NOT NULL AND EXISTS (
      SELECT 1 FROM `+v2Table(schema, "workspace_group_members")+` gm
      WHERE gm.workspace_id=$1 AND gm.group_id=$4 AND gm.user_id=m.user_id
    ))
  )
  AND COALESCE((
    SELECT p.enabled FROM `+v2Table(schema, "workspace_notification_preferences")+` p
    WHERE p.workspace_id=$1 AND p.user_id=m.user_id
      AND p.channel='email' AND p.event_key='assignments'
  ), false)
ORDER BY u.email
LIMIT 51`, workspaceID, actorID, userID, groupID)
	if err != nil {
		return nil, fmt.Errorf("list assignment email recipients: %w", err)
	}
	defer rows.Close()
	addresses := make([]string, 0, 51)
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, err
		}
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

func insertV2MembershipNotifications(ctx context.Context, tx pgx.Tx, schema, workspaceID, actorID, subjectUserID string) error {
	rows, err := tx.Query(ctx, `
SELECT m.user_id
FROM `+v2Table(schema, "workspace_memberships")+` m
WHERE m.workspace_id=$1 AND m.status='active' AND m.role IN ('owner','admin')
  AND m.user_id<>$2
  AND COALESCE((
    SELECT p.enabled FROM `+v2Table(schema, "workspace_notification_preferences")+` p
    WHERE p.workspace_id=$1 AND p.user_id=m.user_id
      AND p.channel='in_app' AND p.event_key='membership'
  ), true)`, workspaceID, actorID)
	if err != nil {
		return fmt.Errorf("select membership notification recipients: %w", err)
	}
	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, userID := range userIDs {
		notificationID, err := id.New()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_notifications")+`
  (id, workspace_id, recipient_user_id, event_key, resource_type, resource_id)
VALUES ($1,$2,$3,'membership','member',$4)`,
			notificationID, workspaceID, userID, subjectUserID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListV2MembershipEmailRecipients(ctx context.Context, workspaceID, actorID string) ([]string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT u.email
FROM `+v2Table(schema, "workspace_memberships")+` m
JOIN users u ON u.id=m.user_id AND u.email IS NOT NULL AND u.email_verified_at IS NOT NULL
WHERE m.workspace_id=$1 AND m.status='active' AND m.role IN ('owner','admin') AND m.user_id<>$2
  AND COALESCE((
    SELECT p.enabled FROM `+v2Table(schema, "workspace_notification_preferences")+` p
    WHERE p.workspace_id=$1 AND p.user_id=m.user_id
      AND p.channel='email' AND p.event_key='membership'
  ), false)
ORDER BY u.email LIMIT 51`, workspaceID, actorID)
	if err != nil {
		return nil, fmt.Errorf("list membership email recipients: %w", err)
	}
	defer rows.Close()
	emails := make([]string, 0)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}
