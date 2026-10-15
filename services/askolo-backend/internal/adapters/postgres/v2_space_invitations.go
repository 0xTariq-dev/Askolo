package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

var ErrV2AlreadyMember = errors.New("email already belongs to an active Space member")

type V2WorkspaceInvitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) ListV2WorkspaceInvitations(ctx context.Context, workspaceID string) ([]V2WorkspaceInvitation, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, email, role, status, expires_at, created_at
FROM `+v2Table(schema, "workspace_invitations")+`
WHERE workspace_id = $1 AND status = 'pending' AND expires_at > now()
ORDER BY created_at DESC, id
LIMIT 201`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list Space invitations: %w", err)
	}
	defer rows.Close()
	invitations := make([]V2WorkspaceInvitation, 0)
	for rows.Next() {
		var invitation V2WorkspaceInvitation
		if err := rows.Scan(
			&invitation.ID, &invitation.Email, &invitation.Role, &invitation.Status,
			&invitation.ExpiresAt, &invitation.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan Space invitation: %w", err)
		}
		invitations = append(invitations, invitation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Space invitations: %w", err)
	}
	if len(invitations) > 200 {
		return nil, fmt.Errorf("Space invitation limit exceeded")
	}
	return invitations, nil
}

func (s *Store) CreateV2WorkspaceInvitation(
	ctx context.Context, workspaceID, actorUserID, email, normalizedEmail, role, tokenHash string, expiresAt time.Time,
) (V2WorkspaceInvitation, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2WorkspaceInvitation{}, err
	}
	invitationID, err := id.New()
	if err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("create Space invitation ID: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("begin Space invitation: %w", err)
	}
	defer tx.Rollback(ctx)

	var actorRole string
	if err := tx.QueryRow(ctx, `
SELECT role
FROM `+v2Table(schema, "workspace_memberships")+`
WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'
FOR SHARE`, workspaceID, actorUserID).Scan(&actorRole); errors.Is(err, pgx.ErrNoRows) {
		return V2WorkspaceInvitation{}, ErrNotFound
	} else if err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("authorize Space invitation: %w", err)
	}
	if actorRole != "owner" && actorRole != "admin" {
		return V2WorkspaceInvitation{}, ErrV2Forbidden
	}
	var alreadyMember bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM users u
  JOIN workspace_memberships m ON m.user_id = u.id
  WHERE m.workspace_id = $1 AND m.status = 'active' AND lower(btrim(u.email)) = $2
)`, workspaceID, normalizedEmail).Scan(&alreadyMember); err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("check existing Space membership: %w", err)
	}
	if alreadyMember {
		return V2WorkspaceInvitation{}, ErrV2AlreadyMember
	}
	var invitation V2WorkspaceInvitation
	if err := tx.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "workspace_invitations")+` AS existing
  (id, workspace_id, email, email_normalized, role, token_hash, invited_by_user_id,
   status, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, now(), now())
ON CONFLICT (workspace_id, email_normalized) WHERE status = 'pending'
DO UPDATE SET email = EXCLUDED.email,
              role = EXCLUDED.role,
              token_hash = EXCLUDED.token_hash,
              invited_by_user_id = EXCLUDED.invited_by_user_id,
              expires_at = EXCLUDED.expires_at,
              updated_at = now()
RETURNING id, email, role, status, expires_at, created_at`,
		invitationID, workspaceID, email, normalizedEmail, role, tokenHash, actorUserID, expiresAt,
	).Scan(
		&invitation.ID, &invitation.Email, &invitation.Role, &invitation.Status,
		&invitation.ExpiresAt, &invitation.CreatedAt,
	); err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("save Space invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return V2WorkspaceInvitation{}, fmt.Errorf("commit Space invitation: %w", err)
	}
	return invitation, nil
}

func (s *Store) CancelV2WorkspaceInvitation(ctx context.Context, workspaceID, invitationID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_invitations")+`
SET status = 'cancelled', updated_at = now()
WHERE workspace_id = $1 AND id = $2 AND status = 'pending'`,
		workspaceID, invitationID,
	)
	if err != nil {
		return fmt.Errorf("cancel Space invitation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AcceptV2WorkspaceInvitation(
	ctx context.Context, tokenHash, userID string,
) (string, string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin Space invitation acceptance: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID, expectedEmail, role, status, userEmail string
	var unexpired bool
	if err := tx.QueryRow(ctx, `
SELECT i.workspace_id, i.email_normalized, i.role, i.status, i.expires_at > now(),
       COALESCE(lower(btrim(u.email)), '')
FROM `+v2Table(schema, "workspace_invitations")+` i
JOIN `+v2Table(schema, "workspaces")+` w ON w.id = i.workspace_id AND w.status = 'active'
JOIN users u ON u.id = $2
WHERE i.token_hash = $1
FOR UPDATE OF i`, tokenHash, userID).Scan(
		&workspaceID, &expectedEmail, &role, &status, &unexpired, &userEmail,
	); errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	} else if err != nil {
		return "", "", fmt.Errorf("read Space invitation: %w", err)
	}
	if status != "pending" || !unexpired {
		return "", "", ErrNotFound
	}
	if expectedEmail != userEmail {
		return "", "", ErrV2Forbidden
	}
	var alreadyMember bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM workspace_memberships
  WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'
)`, workspaceID, userID).Scan(&alreadyMember); err != nil {
		return "", "", fmt.Errorf("check current Space membership: %w", err)
	}
	if alreadyMember {
		return "", "", ErrV2AlreadyMember
	}
	membershipID, err := id.New()
	if err != nil {
		return "", "", fmt.Errorf("create accepted Space membership ID: %w", err)
	}
	versionedMembershipID, err := id.New()
	if err != nil {
		return "", "", fmt.Errorf("create accepted versioned Space membership ID: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO workspace_memberships
  (id, workspace_id, user_id, status, role, permissions)
VALUES ($1, $2, $3, 'active', $4, $5)
ON CONFLICT (workspace_id, user_id)
DO UPDATE SET status = 'active', role = EXCLUDED.role, permissions = EXCLUDED.permissions`,
		membershipID, workspaceID, userID, role, []string{},
	); err != nil {
		return "", "", fmt.Errorf("activate accepted Space membership: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_memberships")+`
  (id, workspace_id, user_id, role, status, joined_at, updated_at)
VALUES ($1, $2, $3, $4, 'active', now(), now())
ON CONFLICT (workspace_id, user_id)
DO UPDATE SET role = EXCLUDED.role, status = 'active', joined_at = now(),
              revoked_at = NULL, updated_at = now()`,
		versionedMembershipID, workspaceID, userID, role,
	); err != nil {
		return "", "", fmt.Errorf("activate versioned Space membership: %w", err)
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_invitations")+`
SET status = 'accepted', accepted_by_user_id = $2, accepted_at = now(), updated_at = now()
WHERE workspace_id = $1 AND token_hash = $3 AND status = 'pending'`,
		workspaceID, userID, tokenHash,
	)
	if err != nil {
		return "", "", fmt.Errorf("mark Space invitation accepted: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return "", "", ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit Space invitation acceptance: %w", err)
	}
	return workspaceID, role, nil
}

func (s *Store) DeclineV2WorkspaceInvitation(ctx context.Context, tokenHash, userID string) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin Space invitation decline: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID, expectedEmail, status, userEmail string
	var unexpired bool
	if err := tx.QueryRow(ctx, `
SELECT i.workspace_id, i.email_normalized, i.status, i.expires_at > now(),
       COALESCE(lower(btrim(u.email)), '')
FROM `+v2Table(schema, "workspace_invitations")+` i
JOIN `+v2Table(schema, "workspaces")+` w ON w.id = i.workspace_id AND w.status = 'active'
JOIN users u ON u.id = $2
WHERE i.token_hash = $1
FOR UPDATE OF i`, tokenHash, userID).Scan(
		&workspaceID, &expectedEmail, &status, &unexpired, &userEmail,
	); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("read Space invitation for decline: %w", err)
	}
	if status != "pending" || !unexpired {
		return "", ErrNotFound
	}
	if expectedEmail != userEmail {
		return "", ErrV2Forbidden
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_invitations")+`
SET status = 'declined', updated_at = now()
WHERE workspace_id = $1 AND token_hash = $2 AND status = 'pending'`,
		workspaceID, tokenHash,
	)
	if err != nil {
		return "", fmt.Errorf("decline Space invitation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return "", ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit Space invitation decline: %w", err)
	}
	return workspaceID, nil
}
