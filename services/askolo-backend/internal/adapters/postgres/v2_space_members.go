package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type V2SpaceMember struct {
	UserID   string     `json:"userId"`
	Name     *string    `json:"name"`
	Email    *string    `json:"email,omitempty"`
	Role     string     `json:"role"`
	JoinedAt *time.Time `json:"joinedAt"`
}

func (s *Store) ListV2SpaceMembers(
	ctx context.Context, workspaceID, role string,
) ([]V2SpaceMember, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT m.user_id,
       NULLIF(BTRIM(CONCAT_WS(' ', u.first_name, u.last_name)), ''),
       CASE WHEN $2 IN ('owner', 'admin') THEN u.email ELSE NULL END,
       m.role, m.joined_at
FROM `+v2Table(schema, "workspace_memberships")+` m
LEFT JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = $1 AND m.status = 'active'
ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
         lower(COALESCE(u.last_name, '')), lower(COALESCE(u.first_name, '')), m.user_id
LIMIT 501`, workspaceID, role)
	if err != nil {
		return nil, fmt.Errorf("list Space members: %w", err)
	}
	defer rows.Close()
	members := make([]V2SpaceMember, 0)
	for rows.Next() {
		var member V2SpaceMember
		if err := rows.Scan(&member.UserID, &member.Name, &member.Email, &member.Role, &member.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan Space member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Space members: %w", err)
	}
	if len(members) > 500 {
		return nil, fmt.Errorf("Space member limit exceeded")
	}
	return members, nil
}

func (s *Store) UpdateV2SpaceMemberRole(
	ctx context.Context, workspaceID, actorUserID, targetUserID, role string,
) error {
	if role != "admin" && role != "member" {
		return ErrV2Forbidden
	}
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Space member role update: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerUserID string
	if err := tx.QueryRow(ctx, `
SELECT owner_user_id FROM `+v2Table(schema, "workspaces")+`
WHERE id = $1 AND status = 'active'
FOR UPDATE`, workspaceID).Scan(&ownerUserID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock Space for role update: %w", err)
	}
	var actorRole string
	if err := tx.QueryRow(ctx, `
SELECT role FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id=$1 AND user_id=$2 AND status='active'`, workspaceID, actorUserID).Scan(&actorRole); errors.Is(err, pgx.ErrNoRows) {
		return ErrV2Forbidden
	} else if err != nil {
		return fmt.Errorf("read Space role editor: %w", err)
	}
	if actorRole != "owner" && actorRole != "admin" {
		return ErrV2Forbidden
	}
	if targetUserID == ownerUserID {
		return ErrV2Forbidden
	}
	var targetActive bool
	if err := tx.QueryRow(ctx, `
SELECT status = 'active'
FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id = $1 AND user_id = $2
FOR UPDATE`, workspaceID, targetUserID).Scan(&targetActive); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("read target Space member: %w", err)
	}
	if !targetActive {
		return ErrNotFound
	}
	var targetRole string
	if err := tx.QueryRow(ctx, `
SELECT role FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id=$1 AND user_id=$2`, workspaceID, targetUserID).Scan(&targetRole); err != nil {
		return fmt.Errorf("read target Space role: %w", err)
	}
	if targetRole == "owner" || (actorRole == "admin" && (targetRole != "member" || role != "member")) {
		return ErrV2Forbidden
	}
	sourceTag, err := tx.Exec(ctx, `
UPDATE workspace_memberships
SET role = $3, updated_at = now()
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active' AND role <> 'owner'`,
		workspaceID, targetUserID, role,
	)
	if err != nil {
		return fmt.Errorf("update source Space member role: %w", err)
	}
	if sourceTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_memberships")+`
SET role = $3, updated_at = now()
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active' AND role <> 'owner'`,
		workspaceID, targetUserID, role,
	)
	if err != nil {
		return fmt.Errorf("update versioned Space member role: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err := insertV2MembershipNotifications(ctx, tx, schema, workspaceID, actorUserID, targetUserID); err != nil {
		return fmt.Errorf("create Space role-change notifications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Space member role update: %w", err)
	}
	return nil
}

func (s *Store) RemoveV2SpaceMember(ctx context.Context, workspaceID, actorUserID, targetUserID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	if actorUserID == targetUserID {
		return ErrV2Forbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Space member removal: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerUserID string
	var isPersonal bool
	if err := tx.QueryRow(ctx, `
SELECT owner_user_id, is_personal FROM `+v2Table(schema, "workspaces")+`
WHERE id=$1 AND status='active' FOR UPDATE`, workspaceID).Scan(&ownerUserID, &isPersonal); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock Space for member removal: %w", err)
	}
	if isPersonal {
		return ErrV2Forbidden
	}
	var actorRole, targetRole string
	if err := tx.QueryRow(ctx, `
SELECT role FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id=$1 AND user_id=$2 AND status='active'`, workspaceID, actorUserID).Scan(&actorRole); errors.Is(err, pgx.ErrNoRows) {
		return ErrV2Forbidden
	} else if err != nil {
		return fmt.Errorf("read Space remover role: %w", err)
	}
	if actorRole != "owner" && actorRole != "admin" {
		return ErrV2Forbidden
	}
	if err := tx.QueryRow(ctx, `
SELECT role FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id=$1 AND user_id=$2 AND status='active' FOR UPDATE`, workspaceID, targetUserID).Scan(&targetRole); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("read target Space member role: %w", err)
	}
	if targetUserID == ownerUserID || (actorRole == "admin" && targetRole != "member") {
		return ErrV2Forbidden
	}
	for _, table := range []string{"workspace_memberships"} {
		if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, table)+`
SET status='revoked', revoked_at=now(), updated_at=now()
WHERE workspace_id=$1 AND user_id=$2 AND status='active'`, workspaceID, targetUserID); err != nil {
			return fmt.Errorf("revoke versioned Space member: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE workspace_memberships SET status='revoked', revoked_at=now(), updated_at=now()
WHERE workspace_id=$1 AND user_id=$2 AND status='active'`, workspaceID, targetUserID); err != nil {
		return fmt.Errorf("revoke Space member: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM `+v2Table(schema, "workspace_group_members")+` WHERE workspace_id=$1 AND user_id=$2`, workspaceID, targetUserID); err != nil {
		return fmt.Errorf("remove revoked member from Space groups: %w", err)
	}
	if err := insertV2MembershipNotifications(ctx, tx, schema, workspaceID, actorUserID, targetUserID); err != nil {
		return fmt.Errorf("create Space member-removal notifications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Space member removal: %w", err)
	}
	return nil
}

func (s *Store) LeaveV2Workspace(ctx context.Context, workspaceID, userID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Space leave: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerUserID string
	var isPersonal bool
	if err := tx.QueryRow(ctx, `
SELECT owner_user_id, is_personal
FROM `+v2Table(schema, "workspaces")+`
WHERE id = $1 AND status = 'active'
FOR UPDATE`, workspaceID).Scan(&ownerUserID, &isPersonal); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock Space for leave: %w", err)
	}
	if isPersonal || ownerUserID == userID {
		return ErrV2Forbidden
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_memberships")+`
SET status = 'revoked', revoked_at = now(), updated_at = now()
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active' AND role <> 'owner'`,
		workspaceID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke versioned membership after leaving Space: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err := insertV2MembershipNotifications(ctx, tx, schema, workspaceID, userID, userID); err != nil {
		return fmt.Errorf("create Space leave notifications: %w", err)
	}
	tag, err = tx.Exec(ctx, `
UPDATE workspace_memberships
SET status = 'revoked', revoked_at = now(), updated_at = now()
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active' AND role <> 'owner'`,
		workspaceID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke membership after leaving Space: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_group_members")+`
WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID); err != nil {
		return fmt.Errorf("remove Space group membership after leave: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Space leave: %w", err)
	}
	return nil
}

func (s *Store) DeleteV2Workspace(ctx context.Context, workspaceID, userID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Space deletion: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerUserID string
	var isPersonal bool
	if err := tx.QueryRow(ctx, `
SELECT owner_user_id, is_personal
FROM `+v2Table(schema, "workspaces")+`
WHERE id = $1 AND status = 'active'
FOR UPDATE`, workspaceID).Scan(&ownerUserID, &isPersonal); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock Space for deletion: %w", err)
	}
	if isPersonal || ownerUserID != userID {
		return ErrV2Forbidden
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspaces")+`
SET status = 'archived', updated_at = now()
WHERE id = $1 AND status = 'active'`, workspaceID)
	if err != nil {
		return fmt.Errorf("archive Space: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	tag, err = tx.Exec(ctx, `
UPDATE workspaces
SET status = 'archived', updated_at = now()
WHERE id = $1 AND status = 'active'`, workspaceID)
	if err != nil {
		return fmt.Errorf("archive source Space: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_invitations")+`
SET status = 'cancelled', updated_at = now()
WHERE workspace_id = $1 AND status = 'pending'`, workspaceID); err != nil {
		return fmt.Errorf("cancel invitations for archived Space: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_memberships")+`
SET status = 'revoked', revoked_at = COALESCE(revoked_at, now()), updated_at = now()
WHERE workspace_id = $1 AND status <> 'revoked'`, workspaceID); err != nil {
		return fmt.Errorf("revoke versioned memberships for archived Space: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE workspace_memberships
SET status = 'revoked', revoked_at = COALESCE(revoked_at, now()), updated_at = now()
WHERE workspace_id = $1 AND status <> 'revoked'`, workspaceID); err != nil {
		return fmt.Errorf("revoke memberships for archived Space: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Space deletion: %w", err)
	}
	return nil
}
