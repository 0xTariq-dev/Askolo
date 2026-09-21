package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"askolo/backend/internal/platform/authorization"
	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

const personalWorkspacePrefix = "personal:"

type AuthorizationResource struct {
	ID           string
	WorkspaceID  string
	ResourceType string
	ResourceID   string
	OwnerUserID  string
	Status       string
}

// DefaultWorkspaceID is deterministic so existing accounts can be provisioned
// lazily without trusting a workspace id supplied by the browser.
func DefaultWorkspaceID(userID string) string {
	return personalWorkspacePrefix + strings.TrimSpace(userID)
}

func (s *Store) EnsurePersonalWorkspace(ctx context.Context, userID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var accountStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM users WHERE id = $1`, userID).Scan(&accountStatus); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if accountStatus != "active" {
		return nil
	}

	workspaceID := DefaultWorkspaceID(userID)
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspaces (id, name, owner_user_id, status, is_personal)
		VALUES ($1, 'Personal workspace', $2, 'active', TRUE)
		ON CONFLICT (id) DO NOTHING
	`, workspaceID, userID); err != nil {
		return err
	}
	membershipID, err := id.New()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_memberships
			(id, workspace_id, user_id, status, role, permissions)
		VALUES ($1, $2, $3, 'active', 'owner', $4)
		ON CONFLICT (workspace_id, user_id) DO NOTHING
	`, membershipID, workspaceID, userID, authorization.PersonalWorkspaceCapabilities); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateWorkspace(
	ctx context.Context,
	workspaceID, name, ownerUserID string,
	permissions []string,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(ownerUserID) == "" {
		return errors.New("workspace fields are required")
	}
	if len(permissions) == 0 {
		return errors.New("workspace permissions are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var accountStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM users WHERE id = $1`, ownerUserID).Scan(&accountStatus); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if accountStatus != "active" {
		return ErrOwnership
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspaces (id, name, owner_user_id, status, is_personal)
		VALUES ($1, $2, $3, 'active', FALSE)
	`, workspaceID, strings.TrimSpace(name), ownerUserID); err != nil {
		return err
	}
	membershipID, err := id.New()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_memberships
			(id, workspace_id, user_id, status, role, permissions)
		VALUES ($1, $2, $3, 'active', 'owner', $4)
	`, membershipID, workspaceID, ownerUserID, permissions); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetWorkspaceMembership(
	ctx context.Context,
	workspaceID, userID, status, role string, permissions []string,
) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if workspaceID == "" || userID == "" || role == "" || status == "" {
		return errors.New("membership fields are required")
	}
	membershipID, err := id.New()
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO workspace_memberships
			(id, workspace_id, user_id, status, role, permissions, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $4 = 'active' THEN NULL ELSE NOW() END)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			status = EXCLUDED.status,
			role = EXCLUDED.role,
			permissions = EXCLUDED.permissions,
			revoked_at = EXCLUDED.revoked_at,
			updated_at = NOW()
	`, membershipID, workspaceID, userID, status, role, permissions)
	return err
}

func (s *Store) RevokeWorkspaceMembership(ctx context.Context, workspaceID, userID string) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE workspace_memberships
		SET status = 'revoked', revoked_at = NOW(), updated_at = NOW()
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID)
	return err
}

func (s *Store) UpsertAuthorizationResource(ctx context.Context, resource AuthorizationResource) error {
	if s == nil {
		return errors.New("database is not configured")
	}
	if resource.WorkspaceID == "" || resource.ResourceType == "" || resource.ResourceID == "" || resource.OwnerUserID == "" {
		return errors.New("resource ownership fields are required")
	}
	if resource.Status == "" {
		resource.Status = "active"
	}
	resourceID := strings.TrimSpace(resource.ID)
	if resourceID == "" {
		var err error
		resourceID, err = id.New()
		if err != nil {
			return err
		}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO authorization_resources
			(id, workspace_id, resource_type, resource_id, owner_user_id, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (workspace_id, resource_type, resource_id) DO UPDATE SET
			owner_user_id = EXCLUDED.owner_user_id,
			status = EXCLUDED.status,
			updated_at = NOW()
	`, resourceID, resource.WorkspaceID, resource.ResourceType, resource.ResourceID, resource.OwnerUserID, resource.Status)
	return err
}

func (s *Store) Authorize(ctx context.Context, input authorization.Input) (authorization.Decision, error) {
	if s == nil {
		return authorization.Decision{Reason: "authorization_unavailable"}, errors.New("database is not configured")
	}
	if strings.TrimSpace(input.ActorUserID) == "" {
		return authorization.Decision{Reason: "missing_actor"}, nil
	}
	if strings.TrimSpace(input.WorkspaceID) == "" {
		return authorization.Decision{Reason: "missing_workspace_scope"}, nil
	}

	var accountStatus string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM users WHERE id = $1`, input.ActorUserID).Scan(&accountStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.Decision{Reason: "account_missing"}, nil
	}
	if err != nil {
		return authorization.Decision{}, err
	}

	var workspaceStatus string
	err = s.pool.QueryRow(ctx, `SELECT status FROM workspaces WHERE id = $1`, input.WorkspaceID).Scan(&workspaceStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.Evaluate(input, accountStatus, "", nil, nil), nil
	}
	if err != nil {
		return authorization.Decision{}, err
	}

	var membership authorization.Membership
	var revokedAt *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT status, role, permissions, revoked_at
		FROM workspace_memberships
		WHERE workspace_id = $1 AND user_id = $2
	`, input.WorkspaceID, input.ActorUserID).Scan(
		&membership.Status, &membership.Role, &membership.Permissions, &revokedAt,
	)
	var membershipPtr *authorization.Membership
	if errors.Is(err, pgx.ErrNoRows) {
		membershipPtr = nil
	} else if err != nil {
		return authorization.Decision{}, err
	} else {
		membership.Revoked = revokedAt != nil
		membershipPtr = &membership
	}

	var resource *authorization.Resource
	if input.ResourceID != "" || input.ResourceType != "" {
		var candidate authorization.Resource
		err = s.pool.QueryRow(ctx, `
			SELECT workspace_id, resource_type, resource_id, owner_user_id, status
			FROM authorization_resources
			WHERE workspace_id = $1 AND resource_type = $2 AND resource_id = $3
		`, input.WorkspaceID, input.ResourceType, input.ResourceID).Scan(
			&candidate.WorkspaceID, &candidate.ResourceType, &candidate.ResourceID,
			&candidate.OwnerUserID, &candidate.Status,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			resource = nil
		} else if err != nil {
			return authorization.Decision{}, err
		} else {
			resource = &candidate
		}
	}
	return authorization.Evaluate(input, accountStatus, workspaceStatus, membershipPtr, resource), nil
}
