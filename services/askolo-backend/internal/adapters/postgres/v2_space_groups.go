package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"askolo/backend/internal/platform/id"
)

type V2WorkspaceGroup struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Members     []string  `json:"memberUserIds"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (s *Store) ListV2WorkspaceGroups(ctx context.Context, workspaceID string) ([]V2WorkspaceGroup, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, workspace_id, name, description, created_at, updated_at
FROM `+v2Table(schema, "workspace_groups")+`
WHERE workspace_id = $1 ORDER BY lower(name), id LIMIT 501`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list Space groups: %w", err)
	}
	defer rows.Close()
	groups := make([]V2WorkspaceGroup, 0)
	for rows.Next() {
		var group V2WorkspaceGroup
		if err := rows.Scan(&group.ID, &group.WorkspaceID, &group.Name, &group.Description, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Space group: %w", err)
		}
		group.Members = []string{}
		group.Permissions = []string{}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Space groups: %w", err)
	}
	if len(groups) > 500 {
		return nil, errors.New("Space group limit exceeded")
	}
	for i := range groups {
		groups[i].Members, err = s.v2GroupMembers(ctx, schema, workspaceID, groups[i].ID)
		if err != nil {
			return nil, err
		}
		groups[i].Permissions, err = s.v2GroupPermissions(ctx, schema, workspaceID, groups[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func (s *Store) v2GroupMembers(ctx context.Context, schema, workspaceID, groupID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT gm.user_id FROM `+v2Table(schema, "workspace_group_members")+` gm
JOIN `+v2Table(schema, "workspace_memberships")+` m
  ON m.workspace_id = gm.workspace_id AND m.user_id = gm.user_id
WHERE gm.workspace_id = $1 AND gm.group_id = $2 AND m.status = 'active'
ORDER BY gm.user_id`, workspaceID, groupID)
	if err != nil {
		return nil, fmt.Errorf("list Space group members: %w", err)
	}
	defer rows.Close()
	members := []string{}
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		members = append(members, userID)
	}
	return members, rows.Err()
}

func (s *Store) v2GroupPermissions(ctx context.Context, schema, workspaceID, groupID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT permission_key FROM `+v2Table(schema, "workspace_group_permissions")+`
WHERE workspace_id = $1 AND group_id = $2 ORDER BY permission_key`, workspaceID, groupID)
	if err != nil {
		return nil, fmt.Errorf("list Space group permissions: %w", err)
	}
	defer rows.Close()
	permissions := []string{}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

func (s *Store) CreateV2WorkspaceGroup(ctx context.Context, workspaceID, actorID, name string, description *string) (V2WorkspaceGroup, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2WorkspaceGroup{}, err
	}
	groupID, err := id.New()
	if err != nil {
		return V2WorkspaceGroup{}, err
	}
	tag, err := s.pool.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_groups")+`
  (id, workspace_id, name, description, created_by_user_id)
SELECT $1::text, $2::text, $3::text, $4::text, $5::varchar
WHERE EXISTS (
  SELECT 1 FROM `+v2Table(schema, "workspace_memberships")+`
  WHERE workspace_id = $2 AND user_id = $5 AND status = 'active' AND role IN ('owner','admin')
)
AND EXISTS (
  SELECT 1 FROM `+v2Table(schema, "workspaces")+` WHERE id = $2 AND status = 'active' AND NOT is_personal
)`, groupID, workspaceID, name, description, actorID)
	if err != nil {
		return V2WorkspaceGroup{}, fmt.Errorf("create Space group: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return V2WorkspaceGroup{}, ErrV2Forbidden
	}
	groups, err := s.ListV2WorkspaceGroups(ctx, workspaceID)
	if err != nil {
		return V2WorkspaceGroup{}, err
	}
	for _, group := range groups {
		if group.ID == groupID {
			return group, nil
		}
	}
	return V2WorkspaceGroup{}, ErrNotFound
}

func (s *Store) ReplaceV2WorkspaceGroup(ctx context.Context, workspaceID, groupID, actorID, name string, description *string, members, permissions []string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
 SELECT 1 FROM `+v2Table(schema, "workspace_groups")+` g
 JOIN `+v2Table(schema, "workspace_memberships")+` m
   ON m.workspace_id = g.workspace_id AND m.user_id = $3
 JOIN `+v2Table(schema, "workspaces")+` w ON w.id = g.workspace_id
 WHERE g.workspace_id = $1 AND g.id = $2 AND w.status = 'active' AND NOT w.is_personal
   AND m.status = 'active' AND m.role IN ('owner','admin')
)`, workspaceID, groupID, actorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_groups")+`
SET name=$3, description=$4, updated_at=now()
WHERE workspace_id=$1 AND id=$2`, workspaceID, groupID, name, description); err != nil {
		return fmt.Errorf("update Space group: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM `+v2Table(schema, "workspace_group_members")+` WHERE workspace_id=$1 AND group_id=$2`, workspaceID, groupID); err != nil {
		return err
	}
	for _, memberID := range members {
		tag, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_group_members")+` (workspace_id, group_id, user_id, added_by_user_id)
SELECT $1::text,$2::text,$3::varchar,$4::varchar
WHERE EXISTS (SELECT 1 FROM `+v2Table(schema, "workspace_memberships")+` WHERE workspace_id=$1::text AND user_id=$3::varchar AND status='active')`,
			workspaceID, groupID, memberID, actorID)
		if err != nil {
			return fmt.Errorf("set Space group member: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM `+v2Table(schema, "workspace_group_permissions")+` WHERE workspace_id=$1 AND group_id=$2`, workspaceID, groupID); err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_group_permissions")+`
  (workspace_id, group_id, permission_key, granted_by_user_id)
VALUES ($1,$2,$3,$4)`, workspaceID, groupID, permission, actorID); err != nil {
			return fmt.Errorf("set Space group permission: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteV2WorkspaceGroup(ctx context.Context, workspaceID, groupID, actorID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "workspace_groups")+` g
USING `+v2Table(schema, "workspace_memberships")+` m, `+v2Table(schema, "workspaces")+` w
WHERE g.workspace_id=$1 AND g.id=$2 AND m.workspace_id=g.workspace_id AND m.user_id=$3
  AND m.status='active' AND m.role IN ('owner','admin') AND w.id=g.workspace_id
  AND w.status='active' AND NOT w.is_personal`, workspaceID, groupID, actorID)
	if err != nil {
		return fmt.Errorf("delete Space group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) V2WorkspaceHasPermission(ctx context.Context, workspaceID, userID, permission string) (bool, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	var allowed bool
	err = s.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM workspace_memberships m
  WHERE m.workspace_id=$1 AND m.user_id=$2 AND m.status='active'
    AND m.permissions @> ARRAY[$3]::text[]
  UNION ALL
  SELECT 1
  FROM `+v2Table(schema, "workspace_group_members")+` gm
  JOIN `+v2Table(schema, "workspace_group_permissions")+` gp
    ON gp.workspace_id=gm.workspace_id AND gp.group_id=gm.group_id
  JOIN `+v2Table(schema, "workspace_memberships")+` vm
    ON vm.workspace_id=gm.workspace_id AND vm.user_id=gm.user_id
  WHERE gm.workspace_id=$1 AND gm.user_id=$2 AND vm.status='active'
    AND gp.permission_key=$3
)`, workspaceID, userID, permission).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check additive Space permission: %w", err)
	}
	return allowed, nil
}

func (s *Store) IsActiveV2GroupMember(ctx context.Context, workspaceID, groupID, userID string) (bool, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, err
	}
	var active bool
	err = s.pool.QueryRow(ctx, `
SELECT EXISTS (
 SELECT 1 FROM `+v2Table(schema, "workspace_group_members")+` gm
 JOIN `+v2Table(schema, "workspace_memberships")+` m
   ON m.workspace_id=gm.workspace_id AND m.user_id=gm.user_id
 WHERE gm.workspace_id=$1 AND gm.group_id=$2 AND gm.user_id=$3 AND m.status='active'
)`, workspaceID, groupID, userID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check active group membership: %w", err)
	}
	return active, nil
}

func normalizeV2GroupInputs(members, permissions []string) ([]string, []string) {
	memberSet := make(map[string]struct{}, len(members))
	for _, value := range members {
		value = strings.TrimSpace(value)
		if value != "" {
			memberSet[value] = struct{}{}
		}
	}
	permissionSet := make(map[string]struct{}, len(permissions))
	for _, value := range permissions {
		value = strings.TrimSpace(value)
		if value != "" {
			permissionSet[value] = struct{}{}
		}
	}
	memberValues, permissionValues := make([]string, 0, len(memberSet)), make([]string, 0, len(permissionSet))
	for value := range memberSet {
		memberValues = append(memberValues, value)
	}
	for value := range permissionSet {
		permissionValues = append(permissionValues, value)
	}
	return memberValues, permissionValues
}
