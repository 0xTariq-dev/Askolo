package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListV2Goals(ctx context.Context, workspaceID string, limit, offset int) ([]V2GoalRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset = clampV2Page(limit, offset)
	rows, err := s.pool.Query(ctx, `
SELECT id, workspace_id, created_by_user_id, title, description, target_on,
       status, progress_percent, category, created_at, updated_at
FROM `+v2Table(schema, "goals")+`
WHERE workspace_id=$1 AND status <> 'archived'
ORDER BY target_on NULLS LAST, updated_at DESC, id
LIMIT $2 OFFSET $3`, workspaceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list versioned goals: %w", err)
	}
	defer rows.Close()
	goals := make([]V2GoalRecord, 0)
	for rows.Next() {
		var goal V2GoalRecord
		if err := rows.Scan(
			&goal.ID, &goal.WorkspaceID, &goal.CreatedByUserID, &goal.Title,
			&goal.Description, &goal.TargetOn, &goal.Status, &goal.ProgressPercent,
			&goal.Category, &goal.CreatedAt, &goal.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan versioned goal: %w", err)
		}
		goals = append(goals, goal)
	}
	return goals, rows.Err()
}

func (s *Store) CreateV2Goal(ctx context.Context, workspaceID, actorUserID string, input V2GoalWrite) (V2GoalRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2GoalRecord{}, err
	}
	goalID, err := id.New()
	if err != nil {
		return V2GoalRecord{}, fmt.Errorf("generate goal identifier: %w", err)
	}
	var targetOn any
	if input.TargetOn != nil {
		targetOn = input.TargetOn
	}
	var category any
	if input.Category != nil {
		category = input.Category
	}
	var description any
	if input.Description != nil {
		description = input.Description
	}
	var result V2GoalRecord
	err = s.pool.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "goals")+` (
  id, workspace_id, created_by_user_id, title, description, target_on,
  status, progress_percent, category
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id, workspace_id, created_by_user_id, title, description, target_on,
          status, progress_percent, category, created_at, updated_at`,
		goalID, workspaceID, actorUserID, input.Title, description, targetOn,
		input.Status, input.ProgressPercent, category,
	).Scan(
		&result.ID, &result.WorkspaceID, &result.CreatedByUserID, &result.Title,
		&result.Description, &result.TargetOn, &result.Status, &result.ProgressPercent,
		&result.Category, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return V2GoalRecord{}, fmt.Errorf("create versioned goal: %w", err)
	}
	return result, nil
}

func (s *Store) GetV2Goal(ctx context.Context, workspaceID, goalID string) (V2GoalRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2GoalRecord{}, err
	}
	var goal V2GoalRecord
	err = s.pool.QueryRow(ctx, `
SELECT id, workspace_id, created_by_user_id, title, description, target_on,
       status, progress_percent, category, created_at, updated_at
FROM `+v2Table(schema, "goals")+`
WHERE workspace_id=$1 AND id=$2`, workspaceID, goalID).Scan(
		&goal.ID, &goal.WorkspaceID, &goal.CreatedByUserID, &goal.Title,
		&goal.Description, &goal.TargetOn, &goal.Status, &goal.ProgressPercent,
		&goal.Category, &goal.CreatedAt, &goal.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2GoalRecord{}, ErrNotFound
	}
	if err != nil {
		return V2GoalRecord{}, fmt.Errorf("read versioned goal: %w", err)
	}
	return goal, nil
}

func (s *Store) UpdateV2Goal(ctx context.Context, workspaceID, goalID string, input V2GoalWrite) (V2GoalRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2GoalRecord{}, err
	}
	var description any
	if input.Description != nil {
		description = input.Description
	}
	var targetOn any
	if input.TargetOn != nil {
		targetOn = input.TargetOn
	}
	var category any
	if input.Category != nil {
		category = input.Category
	}
	var goal V2GoalRecord
	err = s.pool.QueryRow(ctx, `
UPDATE `+v2Table(schema, "goals")+`
SET title=$3, description=$4, target_on=$5, status=$6,
    progress_percent=$7, category=$8, updated_at=now()
WHERE workspace_id=$1 AND id=$2
RETURNING id, workspace_id, created_by_user_id, title, description, target_on,
          status, progress_percent, category, created_at, updated_at`,
		workspaceID, goalID, input.Title, description, targetOn, input.Status,
		input.ProgressPercent, category,
	).Scan(
		&goal.ID, &goal.WorkspaceID, &goal.CreatedByUserID, &goal.Title,
		&goal.Description, &goal.TargetOn, &goal.Status, &goal.ProgressPercent,
		&goal.Category, &goal.CreatedAt, &goal.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2GoalRecord{}, ErrNotFound
	}
	if err != nil {
		return V2GoalRecord{}, fmt.Errorf("update versioned goal: %w", err)
	}
	return goal, nil
}

func (s *Store) ArchiveV2Goal(ctx context.Context, workspaceID, goalID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "goals")+`
SET status='archived', updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND status <> 'archived'`, workspaceID, goalID)
	if err != nil {
		return fmt.Errorf("archive versioned goal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+v2Table(schema, "goals")+` WHERE workspace_id=$1 AND id=$2)`, workspaceID, goalID).Scan(&exists); err != nil {
			return fmt.Errorf("check archived goal: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) ListV2Plans(ctx context.Context, workspaceID, actorUserID, role string, limit, offset int) ([]V2Plan, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset = clampV2Page(limit, offset)
	rows, err := s.pool.Query(ctx, `
SELECT id, workspace_id, owner_user_id, title, details, starts_on, ends_on,
       time_zone, visibility, status, updated_by_user_id, created_at, updated_at
FROM `+v2Table(schema, "plans")+`
WHERE workspace_id=$1 AND status <> 'archived'
  AND (visibility='shared' OR owner_user_id=$2 OR $3 IN ('owner','admin'))
ORDER BY starts_on DESC, updated_at DESC, id
LIMIT $4 OFFSET $5`, workspaceID, actorUserID, role, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list versioned plans: %w", err)
	}
	defer rows.Close()
	plans := make([]V2Plan, 0)
	for rows.Next() {
		plan, err := scanV2Plan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan versioned plan: %w", err)
		}
		plan.Entries, err = s.listV2PlanEntries(ctx, schema, workspaceID, plan.ID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func scanV2Plan(row pgx.Row) (V2Plan, error) {
	var plan V2Plan
	err := row.Scan(
		&plan.ID, &plan.WorkspaceID, &plan.OwnerUserID, &plan.Title,
		&plan.Details, &plan.StartsOn, &plan.EndsOn, &plan.TimeZone,
		&plan.Visibility, &plan.Status, &plan.UpdatedByUserID,
		&plan.CreatedAt, &plan.UpdatedAt,
	)
	if plan.Entries == nil {
		plan.Entries = []V2PlanEntry{}
	}
	return plan, err
}

func (s *Store) GetV2Plan(ctx context.Context, workspaceID, planID, actorUserID, role string) (V2Plan, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Plan{}, err
	}
	plan, err := scanV2Plan(s.pool.QueryRow(ctx, `
SELECT id, workspace_id, owner_user_id, title, details, starts_on, ends_on,
       time_zone, visibility, status, updated_by_user_id, created_at, updated_at
FROM `+v2Table(schema, "plans")+`
WHERE workspace_id=$1 AND id=$2
  AND (visibility='shared' OR owner_user_id=$3 OR $4 IN ('owner','admin'))`,
		workspaceID, planID, actorUserID, role,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return V2Plan{}, ErrNotFound
	}
	if err != nil {
		return V2Plan{}, fmt.Errorf("read versioned plan: %w", err)
	}
	plan.Entries, err = s.listV2PlanEntries(ctx, schema, workspaceID, planID)
	if err != nil {
		return V2Plan{}, err
	}
	return plan, nil
}

func (s *Store) CreateV2Plan(ctx context.Context, workspaceID, actorUserID string, input V2PlanWrite) (V2Plan, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Plan{}, err
	}
	planID, err := id.New()
	if err != nil {
		return V2Plan{}, fmt.Errorf("generate plan identifier: %w", err)
	}
	var details any
	if input.Details != nil {
		details = input.Details
	}
	plan, err := scanV2Plan(s.pool.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "plans")+` (
  id, workspace_id, owner_user_id, title, details, starts_on, ends_on,
  time_zone, visibility, status, updated_by_user_id
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$3)
RETURNING id, workspace_id, owner_user_id, title, details, starts_on, ends_on,
          time_zone, visibility, status, updated_by_user_id, created_at, updated_at`,
		planID, workspaceID, actorUserID, input.Title, details, input.StartsOn,
		input.EndsOn, input.TimeZone, input.Visibility, input.Status,
	))
	if err != nil {
		return V2Plan{}, fmt.Errorf("create versioned plan: %w", err)
	}
	return plan, nil
}

func (s *Store) UpdateV2Plan(ctx context.Context, workspaceID, planID, actorUserID string, role string, input V2PlanWrite) (V2Plan, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Plan{}, err
	}
	var owner string
	err = s.pool.QueryRow(ctx, `
SELECT owner_user_id FROM `+v2Table(schema, "plans")+`
WHERE workspace_id=$1 AND id=$2`, workspaceID, planID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2Plan{}, ErrNotFound
	}
	if err != nil {
		return V2Plan{}, fmt.Errorf("check versioned plan owner: %w", err)
	}
	if owner != actorUserID && role != "owner" && role != "admin" {
		return V2Plan{}, ErrV2Forbidden
	}
	if input.Visibility != "" && role != "owner" && role != "admin" {
		var visibility string
		if err := s.pool.QueryRow(ctx, `SELECT visibility FROM `+v2Table(schema, "plans")+` WHERE workspace_id=$1 AND id=$2`, workspaceID, planID).Scan(&visibility); err != nil {
			return V2Plan{}, fmt.Errorf("read plan visibility: %w", err)
		}
		if input.Visibility != visibility {
			return V2Plan{}, ErrV2Forbidden
		}
	}
	var details any
	if input.Details != nil {
		details = input.Details
	}
	plan, err := scanV2Plan(s.pool.QueryRow(ctx, `
UPDATE `+v2Table(schema, "plans")+`
SET title=$3, details=$4, starts_on=$5, ends_on=$6, time_zone=$7,
    visibility=$8, status=$9, updated_by_user_id=$10, updated_at=now()
WHERE workspace_id=$1 AND id=$2
RETURNING id, workspace_id, owner_user_id, title, details, starts_on, ends_on,
          time_zone, visibility, status, updated_by_user_id, created_at, updated_at`,
		workspaceID, planID, input.Title, details, input.StartsOn, input.EndsOn,
		input.TimeZone, input.Visibility, input.Status, actorUserID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return V2Plan{}, ErrNotFound
	}
	if err != nil {
		return V2Plan{}, fmt.Errorf("update versioned plan: %w", err)
	}
	plan.Entries, err = s.listV2PlanEntries(ctx, schema, workspaceID, planID)
	return plan, err
}

func (s *Store) ArchiveV2Plan(ctx context.Context, workspaceID, planID, actorUserID, role string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "plans")+`
SET status='archived', updated_by_user_id=$3, updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND (owner_user_id=$3 OR $4 IN ('owner','admin'))`,
		workspaceID, planID, actorUserID, role,
	)
	if err != nil {
		return fmt.Errorf("archive versioned plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var accessible bool
		if err := s.pool.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM `+v2Table(schema, "plans")+`
WHERE workspace_id=$1 AND id=$2 AND (owner_user_id=$3 OR $4 IN ('owner','admin')))`,
			workspaceID, planID, actorUserID, role,
		).Scan(&accessible); err != nil {
			return fmt.Errorf("check archived plan: %w", err)
		}
		if !accessible {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) listV2PlanEntries(ctx context.Context, schema, workspaceID, planID string) ([]V2PlanEntry, error) {
	rows, err := s.pool.Query(ctx, `
SELECT pe.id, pe.workspace_id, pe.plan_id, pe.task_id, pe.occurrence_on,
       pe.planned_on, pe.planned_start_at, pe.planned_end_at, pe.position,
       pe.created_by_user_id, t.assigned_to_user_id, t.title, t.status,
       pe.created_at, pe.updated_at
FROM `+v2Table(schema, "plan_entries")+` pe
JOIN `+v2Table(schema, "tasks")+` t
  ON t.workspace_id=pe.workspace_id AND t.id=pe.task_id
WHERE pe.workspace_id=$1 AND pe.plan_id=$2
ORDER BY pe.planned_on, pe.planned_start_at NULLS LAST, pe.position, pe.id`,
		workspaceID, planID,
	)
	if err != nil {
		return nil, fmt.Errorf("list plan entries: %w", err)
	}
	defer rows.Close()
	entries := make([]V2PlanEntry, 0)
	for rows.Next() {
		var entry V2PlanEntry
		if err := rows.Scan(
			&entry.ID, &entry.WorkspaceID, &entry.PlanID, &entry.TaskID,
			&entry.OccurrenceOn, &entry.PlannedOn, &entry.PlannedStartAt,
			&entry.PlannedEndAt, &entry.Position, &entry.CreatedByUserID,
			&entry.AssignedToUserID, &entry.TaskTitle, &entry.TaskStatus,
			&entry.CreatedAt, &entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan plan entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) CreateV2PlanEntry(ctx context.Context, workspaceID, planID, actorUserID, role string, input V2PlanEntryWrite) (V2PlanEntry, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2PlanEntry{}, err
	}
	entryID, err := id.New()
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("generate plan entry identifier: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("begin plan entry create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner, visibility string
	if err := tx.QueryRow(ctx, `
SELECT owner_user_id, visibility FROM `+v2Table(schema, "plans")+`
WHERE workspace_id=$1 AND id=$2 AND status <> 'archived'
FOR UPDATE`, workspaceID, planID).Scan(&owner, &visibility); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return V2PlanEntry{}, ErrNotFound
		}
		return V2PlanEntry{}, fmt.Errorf("read plan before adding entry: %w", err)
	}
	if visibility != "shared" && owner != actorUserID && role != "owner" && role != "admin" {
		return V2PlanEntry{}, ErrNotFound
	}
	var startsOn, endsOn time.Time
	if err := tx.QueryRow(ctx, `SELECT starts_on, ends_on FROM `+v2Table(schema, "plans")+` WHERE workspace_id=$1 AND id=$2`, workspaceID, planID).Scan(&startsOn, &endsOn); err != nil {
		return V2PlanEntry{}, fmt.Errorf("read plan period: %w", err)
	}
	if input.PlannedOn.Before(startsOn) || input.PlannedOn.After(endsOn) {
		return V2PlanEntry{}, ErrV2InvalidRange
	}
	if input.AssignedToUserID != nil {
		if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, *input.AssignedToUserID); err != nil {
			return V2PlanEntry{}, err
		}
		var taskCreator string
		if err := tx.QueryRow(ctx, `SELECT created_by_user_id FROM `+v2Table(schema, "tasks")+` WHERE workspace_id=$1 AND id=$2`, workspaceID, input.TaskID).Scan(&taskCreator); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return V2PlanEntry{}, ErrNotFound
			}
			return V2PlanEntry{}, fmt.Errorf("authorize task assignment from plan: %w", err)
		}
		if taskCreator != actorUserID && role != "owner" && role != "admin" {
			return V2PlanEntry{}, ErrV2Forbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE `+v2Table(schema, "tasks")+` SET assigned_to_user_id=$3, updated_at=now() WHERE workspace_id=$1 AND id=$2`, workspaceID, input.TaskID, input.AssignedToUserID); err != nil {
			return V2PlanEntry{}, fmt.Errorf("assign canonical task from plan: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "plan_entries")+` (
  id, workspace_id, plan_id, task_id, occurrence_on, planned_on,
  planned_start_at, planned_end_at, position, created_by_user_id
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		entryID, workspaceID, planID, input.TaskID, input.OccurrenceOn,
		input.PlannedOn, input.PlannedStartAt, input.PlannedEndAt, input.Position,
		actorUserID,
	)
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("create plan entry: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return V2PlanEntry{}, fmt.Errorf("commit plan entry create: %w", err)
	}
	return s.getV2PlanEntry(ctx, schema, workspaceID, planID, entryID)
}

func (s *Store) GetV2PlanEntry(ctx context.Context, workspaceID, planID, entryID string) (V2PlanEntry, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2PlanEntry{}, err
	}
	return s.getV2PlanEntry(ctx, schema, workspaceID, planID, entryID)
}

func (s *Store) getV2PlanEntry(ctx context.Context, schema, workspaceID, planID, entryID string) (V2PlanEntry, error) {
	var entry V2PlanEntry
	err := s.pool.QueryRow(ctx, `
SELECT pe.id, pe.workspace_id, pe.plan_id, pe.task_id, pe.occurrence_on,
       pe.planned_on, pe.planned_start_at, pe.planned_end_at, pe.position,
       pe.created_by_user_id, t.assigned_to_user_id, t.title, t.status,
       pe.created_at, pe.updated_at
FROM `+v2Table(schema, "plan_entries")+` pe
JOIN `+v2Table(schema, "tasks")+` t ON t.workspace_id=pe.workspace_id AND t.id=pe.task_id
WHERE pe.workspace_id=$1 AND pe.plan_id=$2 AND pe.id=$3`,
		workspaceID, planID, entryID,
	).Scan(
		&entry.ID, &entry.WorkspaceID, &entry.PlanID, &entry.TaskID,
		&entry.OccurrenceOn, &entry.PlannedOn, &entry.PlannedStartAt,
		&entry.PlannedEndAt, &entry.Position, &entry.CreatedByUserID,
		&entry.AssignedToUserID, &entry.TaskTitle, &entry.TaskStatus,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2PlanEntry{}, ErrNotFound
	}
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("read plan entry: %w", err)
	}
	return entry, nil
}

func (s *Store) UpdateV2PlanEntry(ctx context.Context, workspaceID, planID, entryID, actorUserID, role string, input V2PlanEntryWrite, assignChanged bool) (V2PlanEntry, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2PlanEntry{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("begin plan entry update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var owner, visibility string
	var startsOn, endsOn time.Time
	if err := tx.QueryRow(ctx, `
SELECT p.owner_user_id, p.visibility, p.starts_on, p.ends_on
FROM `+v2Table(schema, "plans")+` p
JOIN `+v2Table(schema, "plan_entries")+` e
  ON e.workspace_id=p.workspace_id AND e.plan_id=p.id
WHERE p.workspace_id=$1 AND p.id=$2 AND e.id=$3
FOR UPDATE OF p, e`, workspaceID, planID, entryID).Scan(&owner, &visibility, &startsOn, &endsOn); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return V2PlanEntry{}, ErrNotFound
		}
		return V2PlanEntry{}, fmt.Errorf("lock plan entry: %w", err)
	}
	if visibility != "shared" && owner != actorUserID && role != "owner" && role != "admin" {
		return V2PlanEntry{}, ErrNotFound
	}
	if input.PlannedOn.Before(startsOn) || input.PlannedOn.After(endsOn) {
		return V2PlanEntry{}, ErrV2InvalidRange
	}
	if assignChanged {
		if input.AssignedToUserID != nil {
			if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, *input.AssignedToUserID); err != nil {
				return V2PlanEntry{}, err
			}
		}
		var taskCreator string
		if err := tx.QueryRow(ctx, `SELECT created_by_user_id FROM `+v2Table(schema, "tasks")+` WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspaceID, input.TaskID).Scan(&taskCreator); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return V2PlanEntry{}, ErrNotFound
			}
			return V2PlanEntry{}, fmt.Errorf("authorize task assignment from plan: %w", err)
		}
		if taskCreator != actorUserID && role != "owner" && role != "admin" {
			return V2PlanEntry{}, ErrV2Forbidden
		}
		if _, err := tx.Exec(ctx, `UPDATE `+v2Table(schema, "tasks")+` SET assigned_to_user_id=$3, updated_at=now() WHERE workspace_id=$1 AND id=$2`, workspaceID, input.TaskID, input.AssignedToUserID); err != nil {
			return V2PlanEntry{}, fmt.Errorf("assign canonical task from plan: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "plan_entries")+`
SET occurrence_on=$4, planned_on=$5, planned_start_at=$6,
    planned_end_at=$7, position=$8, updated_at=now()
WHERE workspace_id=$1 AND plan_id=$2 AND id=$3`,
		workspaceID, planID, entryID, input.OccurrenceOn, input.PlannedOn,
		input.PlannedStartAt, input.PlannedEndAt, input.Position,
	)
	if err != nil {
		return V2PlanEntry{}, fmt.Errorf("update plan entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return V2PlanEntry{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return V2PlanEntry{}, fmt.Errorf("commit plan entry update: %w", err)
	}
	return s.getV2PlanEntry(ctx, schema, workspaceID, planID, entryID)
}

func (s *Store) DeleteV2PlanEntry(ctx context.Context, workspaceID, planID, entryID, actorUserID, role string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "plan_entries")+` e
USING `+v2Table(schema, "plans")+` p
WHERE e.workspace_id=$1 AND e.plan_id=$2 AND e.id=$3
  AND p.workspace_id=e.workspace_id AND p.id=e.plan_id
  AND (p.visibility='shared' OR p.owner_user_id=$4 OR $5 IN ('owner','admin'))`,
		workspaceID, planID, entryID, actorUserID, role,
	)
	if err != nil {
		return fmt.Errorf("delete plan entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListV2ReminderRules(ctx context.Context, workspaceID, actorUserID, role string, limit, offset int) ([]V2ReminderRule, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset = clampV2Page(limit, offset)
	rows, err := s.pool.Query(ctx, `
SELECT id, workspace_id, owner_user_id, kind, rule, status, created_at, updated_at
FROM `+v2Table(schema, "reminder_rules")+`
WHERE workspace_id=$1 AND (owner_user_id=$2 OR $3 IN ('owner','admin'))
ORDER BY updated_at DESC, id
LIMIT $4 OFFSET $5`, workspaceID, actorUserID, role, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reminder rules: %w", err)
	}
	defer rows.Close()
	result := make([]V2ReminderRule, 0)
	for rows.Next() {
		var item V2ReminderRule
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.OwnerUserID, &item.Kind,
			&item.Rule, &item.Status, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan reminder rule: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateV2ReminderRule(ctx context.Context, workspaceID, actorUserID string, kind string, rule json.RawMessage) (V2ReminderRule, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2ReminderRule{}, err
	}
	ruleID, err := id.New()
	if err != nil {
		return V2ReminderRule{}, fmt.Errorf("generate reminder rule identifier: %w", err)
	}
	var result V2ReminderRule
	err = s.pool.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "reminder_rules")+` (
  id, workspace_id, owner_user_id, kind, rule, status
) VALUES ($1,$2,$3,$4,$5,'pending_automation')
RETURNING id, workspace_id, owner_user_id, kind, rule, status, created_at, updated_at`,
		ruleID, workspaceID, actorUserID, kind, rule,
	).Scan(
		&result.ID, &result.WorkspaceID, &result.OwnerUserID, &result.Kind,
		&result.Rule, &result.Status, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return V2ReminderRule{}, fmt.Errorf("create reminder rule: %w", err)
	}
	return result, nil
}

func (s *Store) GetV2ReminderRule(ctx context.Context, workspaceID, ruleID, actorUserID, role string) (V2ReminderRule, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2ReminderRule{}, err
	}
	var result V2ReminderRule
	err = s.pool.QueryRow(ctx, `
SELECT id, workspace_id, owner_user_id, kind, rule, status, created_at, updated_at
FROM `+v2Table(schema, "reminder_rules")+`
WHERE workspace_id=$1 AND id=$2
  AND (owner_user_id=$3 OR $4 IN ('owner','admin'))`,
		workspaceID, ruleID, actorUserID, role,
	).Scan(
		&result.ID, &result.WorkspaceID, &result.OwnerUserID, &result.Kind,
		&result.Rule, &result.Status, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2ReminderRule{}, ErrNotFound
	}
	if err != nil {
		return V2ReminderRule{}, fmt.Errorf("read reminder rule: %w", err)
	}
	return result, nil
}

func (s *Store) UpdateV2ReminderRule(ctx context.Context, workspaceID, ruleID, actorUserID, role string, kind string, rule json.RawMessage, status string) (V2ReminderRule, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2ReminderRule{}, err
	}
	var result V2ReminderRule
	err = s.pool.QueryRow(ctx, `
UPDATE `+v2Table(schema, "reminder_rules")+`
SET kind=$4, rule=$5, status=$6, updated_at=now()
WHERE workspace_id=$1 AND id=$2
  AND (owner_user_id=$3 OR $7 IN ('owner','admin'))
RETURNING id, workspace_id, owner_user_id, kind, rule, status, created_at, updated_at`,
		workspaceID, ruleID, actorUserID, kind, rule, status, role,
	).Scan(
		&result.ID, &result.WorkspaceID, &result.OwnerUserID, &result.Kind,
		&result.Rule, &result.Status, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return V2ReminderRule{}, ErrNotFound
	}
	if err != nil {
		return V2ReminderRule{}, fmt.Errorf("update reminder rule: %w", err)
	}
	return result, nil
}

func (s *Store) DeleteV2ReminderRule(ctx context.Context, workspaceID, ruleID, actorUserID, role string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "reminder_rules")+`
WHERE workspace_id=$1 AND id=$2
  AND (owner_user_id=$3 OR $4 IN ('owner','admin'))`,
		workspaceID, ruleID, actorUserID, role,
	)
	if err != nil {
		return fmt.Errorf("delete reminder rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListV2AgendaEvents(ctx context.Context, workspaceID string, from, to time.Time, timeZone string, limit, offset int) ([]V2CalendarEventAgendaItem, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 501 {
		limit = 501
	}
	if offset < 0 || offset > 1000000 {
		return nil, errors.New("agenda event offset is outside the supported range")
	}
	rows, err := s.pool.Query(ctx, `
SELECT e.id, e.title, e.description, e.starts_at, e.ends_at, e.all_day, e.time_zone,
       e.location, e.color, e.source_provider, e.source_connection_id,
       e.source_calendar_id, e.source_event_id
FROM `+v2Table(schema, "calendar_events")+` e
WHERE e.workspace_id=$1
  AND e.starts_at < (($3::date + 1)::timestamp AT TIME ZONE $4)
  AND (
    e.ends_at > ($2::date::timestamp AT TIME ZONE $4)
    OR (e.ends_at IS NULL AND e.starts_at >= ($2::date::timestamp AT TIME ZONE $4))
  )
ORDER BY e.starts_at, e.id LIMIT $5 OFFSET $6`,
		workspaceID, from.Format("2006-01-02"), to.Format("2006-01-02"), timeZone, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list calendar events for agenda: %w", err)
	}
	defer rows.Close()
	items := make([]V2CalendarEventAgendaItem, 0)
	for rows.Next() {
		var item V2CalendarEventAgendaItem
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.StartsAt, &item.EndsAt,
			&item.AllDay, &item.TimeZone, &item.Location, &item.Color,
			&item.SourceProvider, &item.SourceConnectionID, &item.SourceCalendarID, &item.SourceEventID,
		); err != nil {
			return nil, fmt.Errorf("scan calendar agenda event: %w", err)
		}
		item.Kind = "calendar_event"
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListV2AgendaPlanEntries(ctx context.Context, workspaceID, actorUserID, role string, from, to time.Time, limit, offset int) ([]V2PlanEntry, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 501 {
		limit = 501
	}
	if offset < 0 || offset > 1000000 {
		return nil, errors.New("agenda plan-entry offset is outside the supported range")
	}
	rows, err := s.pool.Query(ctx, `
SELECT pe.id, pe.workspace_id, pe.plan_id, pe.task_id, pe.occurrence_on,
       pe.planned_on, pe.planned_start_at, pe.planned_end_at, pe.position,
       pe.created_by_user_id, t.assigned_to_user_id, t.title, t.status,
       pe.created_at, pe.updated_at
FROM `+v2Table(schema, "plan_entries")+` pe
JOIN `+v2Table(schema, "plans")+` p
  ON p.workspace_id=pe.workspace_id AND p.id=pe.plan_id
JOIN `+v2Table(schema, "tasks")+` t
  ON t.workspace_id=pe.workspace_id AND t.id=pe.task_id
WHERE pe.workspace_id=$1 AND pe.planned_on BETWEEN $2 AND $3
  AND p.status <> 'archived'
  AND (p.visibility='shared' OR p.owner_user_id=$4 OR $5 IN ('owner','admin'))
ORDER BY pe.planned_on, pe.planned_start_at NULLS LAST, pe.position, pe.id
LIMIT $6 OFFSET $7`, workspaceID, from, to, actorUserID, role, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list plan entries for agenda: %w", err)
	}
	defer rows.Close()
	entries := make([]V2PlanEntry, 0)
	for rows.Next() {
		var entry V2PlanEntry
		if err := rows.Scan(
			&entry.ID, &entry.WorkspaceID, &entry.PlanID, &entry.TaskID,
			&entry.OccurrenceOn, &entry.PlannedOn, &entry.PlannedStartAt,
			&entry.PlannedEndAt, &entry.Position, &entry.CreatedByUserID,
			&entry.AssignedToUserID, &entry.TaskTitle, &entry.TaskStatus,
			&entry.CreatedAt, &entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agenda plan entry: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func clampV2Page(limit, offset int) (int, int) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}
	return limit, offset
}

var ErrV2InvalidRange = errors.New("date is outside the supported range")
