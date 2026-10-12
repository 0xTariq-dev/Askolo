package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"askolo/backend/internal/migrations"
	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

var (
	ErrV2SchemaNotReady = errors.New("versioned task schema is not ready")
	ErrV2Forbidden      = errors.New("versioned resource access denied")
	ErrV2Cycle          = errors.New("task relationships contain a cycle")
	ErrV2RateLimited    = errors.New("versioned API rate limit exceeded")
)

func (s *Store) v2Schema(ctx context.Context) (string, error) {
	if s == nil || s.pool == nil {
		return "", errors.New("database is not configured")
	}
	var sourceSchema string
	if err := s.pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&sourceSchema); err != nil {
		return "", fmt.Errorf("resolve versioned schema: %w", err)
	}
	return migrations.V2SchemaName(sourceSchema), nil
}

func v2Table(schema, table string) string {
	return pgx.Identifier{schema, table}.Sanitize()
}

func (s *Store) V2SchemaReady(ctx context.Context) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	for _, table := range []string{
		"workspaces", "workspace_memberships", "tasks", "task_dependencies",
		"task_occurrences", "task_occurrence_events", "goals", "plans",
		"plan_entries", "calendar_events", "reminder_rules", "api_request_limits",
		"member_preferences", "workspace_provider_account_access",
		"workspace_email_summaries", "home_focus_selections",
	} {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+"."+table).Scan(&exists); err != nil {
			return fmt.Errorf("check versioned schema readiness: %w", err)
		}
		if !exists {
			return ErrV2SchemaNotReady
		}
	}
	return nil
}

func (s *Store) V2WorkspaceRole(ctx context.Context, workspaceID, userID string) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin versioned workspace authorization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ownerUserID, workspaceName, workspaceStatus, sourceRole, memberStatus string
	var isPersonal bool
	err = tx.QueryRow(ctx, `
SELECT w.owner_user_id, w.name, w.status, w.is_personal, m.role, m.status
FROM workspaces w
JOIN workspace_memberships m ON m.workspace_id = w.id
WHERE w.id = $1 AND m.user_id = $2`,
		workspaceID, userID,
	).Scan(&ownerUserID, &workspaceName, &workspaceStatus, &isPersonal, &sourceRole, &memberStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read active workspace membership: %w", err)
	}
	if workspaceStatus != "active" || memberStatus != "active" {
		return "", ErrNotFound
	}

	// Workspace and membership lifecycle is still managed by the stable
	// workspace subsystem. Mirror only the already-authorized workspace here
	// so V2 rows and their composite membership foreign keys follow its current
	// roles without making the V2 schema a stale, independently managed ACL.
	if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspaces")+` (
  id, owner_user_id, owner_role, name, status, is_personal
) VALUES ($1, $2, 'owner', $3, 'active', $4)
ON CONFLICT (id) DO UPDATE SET
  owner_user_id = EXCLUDED.owner_user_id,
  owner_role = 'owner',
  name = EXCLUDED.name,
  status = EXCLUDED.status,
  is_personal = EXCLUDED.is_personal,
  updated_at = now()`,
		workspaceID, ownerUserID, workspaceName, isPersonal,
	); err != nil {
		return "", fmt.Errorf("synchronize versioned workspace: %w", err)
	}

	// Demote a previous owner before applying a changed source owner; the
	// target schema permits exactly one owner membership per workspace.
	if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_memberships")+`
SET role = 'member', updated_at = now()
WHERE workspace_id = $1 AND role = 'owner' AND user_id <> $2`,
		workspaceID, ownerUserID,
	); err != nil {
		return "", fmt.Errorf("prepare versioned workspace owner sync: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_memberships")+` AS existing (
  id, workspace_id, user_id, role, status, joined_at, revoked_at
)
SELECT
  m.id,
  m.workspace_id,
  m.user_id,
  CASE
    WHEN m.user_id = w.owner_user_id THEN 'owner'
    WHEN m.role IN ('admin', 'member') THEN m.role
    ELSE 'member'
  END,
  CASE
    WHEN m.status IN ('active', 'invited', 'suspended', 'revoked') THEN m.status
    ELSE 'revoked'
  END,
  CASE WHEN m.status = 'active' THEN m.created_at ELSE NULL END,
  CASE
    WHEN m.status IN ('active', 'invited', 'suspended') THEN NULL
    ELSE COALESCE(m.revoked_at, m.updated_at)
  END
FROM workspace_memberships m
JOIN workspaces w ON w.id = m.workspace_id
WHERE m.workspace_id = $1
ON CONFLICT (workspace_id, user_id) DO UPDATE SET
  role = EXCLUDED.role,
  status = EXCLUDED.status,
  joined_at = COALESCE(existing.joined_at, EXCLUDED.joined_at),
  revoked_at = EXCLUDED.revoked_at,
  updated_at = now()`,
		workspaceID,
	); err != nil {
		return "", fmt.Errorf("synchronize versioned workspace memberships: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "workspace_memberships")+` vm
SET role = CASE WHEN vm.user_id = $2 THEN 'owner' ELSE 'member' END,
    status = 'revoked',
    revoked_at = COALESCE(vm.revoked_at, now()),
    updated_at = now()
WHERE vm.workspace_id = $1
  AND NOT EXISTS (
    SELECT 1 FROM workspace_memberships source
    WHERE source.workspace_id = vm.workspace_id
      AND source.user_id = vm.user_id
  )`,
		workspaceID, ownerUserID,
	); err != nil {
		return "", fmt.Errorf("revoke stale versioned workspace memberships: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit versioned workspace authorization: %w", err)
	}

	role := "member"
	if userID == ownerUserID {
		role = "owner"
	} else if sourceRole == "admin" {
		role = "admin"
	}
	return role, nil
}

func requireActiveV2WorkspaceMember(ctx context.Context, tx pgx.Tx, workspaceID, userID string) error {
	var membershipStatus, workspaceStatus string
	err := tx.QueryRow(ctx, `
SELECT m.status, w.status
FROM workspace_memberships m
JOIN workspaces w ON w.id = m.workspace_id
WHERE m.workspace_id = $1 AND m.user_id = $2
FOR SHARE OF m, w`,
		workspaceID, userID,
	).Scan(&membershipStatus, &workspaceStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("verify active workspace member: %w", err)
	}
	if membershipStatus != "active" || workspaceStatus != "active" {
		return ErrNotFound
	}
	return nil
}

func (s *Store) V2WorkspaceTimeZone(ctx context.Context, workspaceID string) (string, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return "", err
	}
	var zone string
	err = s.pool.QueryRow(ctx, `
SELECT time_zone
FROM `+v2Table(schema, "workspaces")+`
WHERE id=$1 AND status='active'`, workspaceID).Scan(&zone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read workspace time zone: %w", err)
	}
	return zone, nil
}

// ConsumeV2Request stores one atomic per-user rolling-minute counter. Reads
// and writes use separate limits so a caller cannot exhaust mutation capacity
// by polling list endpoints.
func (s *Store) ConsumeV2Request(ctx context.Context, userID string, write bool) (bool, int, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return false, 0, err
	}
	table := v2Table(schema, "api_request_limits")
	limit := 120
	var query string
	if write {
		limit = 30
		query = `
WITH current_minute AS (
  SELECT date_trunc('minute', clock_timestamp()) AS bucket
)
INSERT INTO ` + table + ` (
  user_id, read_window_started_at, read_request_count,
  write_window_started_at, write_request_count, updated_at
)
SELECT $1, bucket, 0, bucket, 1, clock_timestamp() FROM current_minute
ON CONFLICT (user_id) DO UPDATE SET
  read_request_count = CASE
    WHEN ` + table + `.read_window_started_at < EXCLUDED.read_window_started_at THEN 1
    ELSE ` + table + `.read_request_count
  END,
  read_window_started_at = EXCLUDED.read_window_started_at,
  write_request_count = CASE
    WHEN ` + table + `.write_window_started_at < EXCLUDED.write_window_started_at THEN 1
    ELSE LEAST(` + table + `.write_request_count + 1, 31)
  END,
  write_window_started_at = EXCLUDED.write_window_started_at,
  updated_at = EXCLUDED.updated_at
RETURNING write_request_count, write_window_started_at`
	} else {
		query = `
WITH current_minute AS (
  SELECT date_trunc('minute', clock_timestamp()) AS bucket
)
INSERT INTO ` + table + ` (
  user_id, read_window_started_at, read_request_count,
  write_window_started_at, write_request_count, updated_at
)
SELECT $1, bucket, 1, bucket, 0, clock_timestamp() FROM current_minute
ON CONFLICT (user_id) DO UPDATE SET
  read_request_count = CASE
    WHEN ` + table + `.read_window_started_at < EXCLUDED.read_window_started_at THEN 1
    ELSE LEAST(` + table + `.read_request_count + 1, 121)
  END,
  read_window_started_at = EXCLUDED.read_window_started_at,
  write_request_count = CASE
    WHEN ` + table + `.write_window_started_at < EXCLUDED.write_window_started_at THEN 0
    ELSE ` + table + `.write_request_count
  END,
  write_window_started_at = EXCLUDED.write_window_started_at,
  updated_at = EXCLUDED.updated_at
RETURNING read_request_count, read_window_started_at`
	}
	var count int
	var started time.Time
	if err := s.pool.QueryRow(ctx, query, userID).Scan(&count, &started); err != nil {
		return false, 0, fmt.Errorf("consume versioned API rate limit: %w", err)
	}
	if count <= limit {
		return true, 0, nil
	}
	retry := int(time.Until(started.Add(time.Minute)).Seconds())
	if retry < 1 {
		retry = 1
	}
	if retry > 60 {
		retry = 60
	}
	return false, retry, nil
}

func (s *Store) GetV2Task(ctx context.Context, workspaceID, taskID string) (V2Task, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Task{}, err
	}
	task, err := scanV2Task(s.pool.QueryRow(ctx, `
SELECT id, workspace_id, created_by_user_id, assigned_to_user_id, kind,
       title, details, status, priority, due_on, planned_start_at, goal_id,
       parent_task_id, recurrence, source_type, source_reference,
       planned_on, planned_start_time, duration_minutes, due_time, time_zone, label,
       completed_at, archived_at, created_at, updated_at
FROM `+v2Table(schema, "tasks")+`
WHERE workspace_id = $1 AND id = $2`, workspaceID, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return V2Task{}, ErrNotFound
	}
	if err != nil {
		return V2Task{}, fmt.Errorf("read versioned task: %w", err)
	}
	task.Dependencies, err = s.v2TaskDependencies(ctx, schema, workspaceID, taskID)
	if err != nil {
		return V2Task{}, err
	}
	if task.Dependencies == nil {
		task.Dependencies = []string{}
	}
	return task, nil
}

func scanV2Task(row pgx.Row) (V2Task, error) {
	var task V2Task
	err := row.Scan(
		&task.ID, &task.WorkspaceID, &task.CreatedByUserID, &task.AssignedToUserID,
		&task.Kind, &task.Title, &task.Details, &task.Status, &task.Priority,
		&task.DueOn, &task.PlannedStartAt, &task.GoalID, &task.ParentTaskID,
		&task.Recurrence, &task.SourceType, &task.SourceReference,
		&task.PlannedOn, &task.PlannedStartTime, &task.DurationMinutes, &task.DueTime,
		&task.TimeZone, &task.Label, &task.CompletedAt,
		&task.ArchivedAt, &task.CreatedAt, &task.UpdatedAt,
	)
	if task.Recurrence == nil {
		task.Recurrence = json.RawMessage("null")
	}
	switch task.Kind {
	case "action", "chore", "commitment":
		task.Kind = "task"
	}
	if task.Priority == "medium" {
		if task.Kind == "habit" {
			task.Priority = "none"
		} else {
			task.Priority = "normal"
		}
	}
	if task.PlannedOn == nil {
		switch {
		case task.DueOn != nil:
			date := time.Date(task.DueOn.Year(), task.DueOn.Month(), task.DueOn.Day(), 0, 0, 0, 0, time.UTC)
			task.PlannedOn = &date
		case task.PlannedStartAt != nil:
			startUTC := task.PlannedStartAt.UTC()
			date := time.Date(startUTC.Year(), startUTC.Month(), startUTC.Day(), 0, 0, 0, 0, time.UTC)
			task.PlannedOn = &date
		}
	}
	return task, err
}

func (s *Store) v2TaskDependencies(ctx context.Context, schema, workspaceID, taskID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT depends_on_task_id
FROM `+v2Table(schema, "task_dependencies")+`
WHERE workspace_id = $1 AND task_id = $2
ORDER BY depends_on_task_id`, workspaceID, taskID)
	if err != nil {
		return nil, fmt.Errorf("read task dependencies: %w", err)
	}
	defer rows.Close()
	dependencies := make([]string, 0)
	for rows.Next() {
		var dependency string
		if err := rows.Scan(&dependency); err != nil {
			return nil, fmt.Errorf("scan task dependency: %w", err)
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

func (s *Store) ListV2Tasks(ctx context.Context, workspaceID string, filter V2TaskFilter) ([]V2Task, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	if filter.Limit < 1 || filter.Limit > 101 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	query := `SELECT id, workspace_id, created_by_user_id, assigned_to_user_id, kind,
 title, details, status, priority, due_on, planned_start_at, goal_id,
 parent_task_id, recurrence, source_type, source_reference,
 planned_on, planned_start_time, duration_minutes, due_time, time_zone, label,
 completed_at, archived_at, created_at, updated_at
FROM ` + v2Table(schema, "tasks") + ` WHERE workspace_id = $1`
	args := []any{workspaceID}
	if filter.Status == "" {
		query += " AND status <> 'archived'"
	}
	add := func(condition string, value any) {
		args = append(args, value)
		query += fmt.Sprintf(" AND %s = $%d", condition, len(args))
	}
	if filter.Kind != "" {
		if filter.Kind == "task" {
			query += " AND kind IN ('task', 'action', 'chore', 'commitment')"
		} else {
			add("kind", filter.Kind)
		}
	}
	if filter.Status != "" {
		add("status", filter.Status)
	}
	if filter.AssigneeID != "" {
		add("assigned_to_user_id", filter.AssigneeID)
	}
	if filter.From != nil && filter.To != nil {
		timeZone := filter.TimeZone
		if timeZone == "" {
			timeZone = "UTC"
		}
		args = append(args, filter.From.Format("2006-01-02"), filter.To.Format("2006-01-02"), timeZone)
		fromArg, toArg, zoneArg := len(args)-2, len(args)-1, len(args)
		query += fmt.Sprintf(` AND (
  due_on BETWEEN $%d AND $%d
  OR planned_on BETWEEN $%d AND $%d
  OR (
    planned_on IS NOT NULL
    AND planned_start_time IS NOT NULL
    AND duration_minutes IS NOT NULL
    AND ((planned_on + planned_start_time) AT TIME ZONE time_zone)
        < (($%d::date + 1)::timestamp AT TIME ZONE $%d)
    AND (((planned_on + planned_start_time) AT TIME ZONE time_zone)
         + make_interval(mins => duration_minutes))
        > ($%d::date::timestamp AT TIME ZONE $%d)
  )
  OR (
    planned_start_at >= ($%d::date::timestamp AT TIME ZONE $%d)
    AND planned_start_at < (($%d::date + 1)::timestamp AT TIME ZONE $%d)
  )
  OR (
    recurrence IS NOT NULL
    AND (recurrence->>'startsOn')::date <= $%d
    AND (recurrence->>'endsOn' IS NULL OR (recurrence->>'endsOn')::date >= $%d)
  )
 )`, fromArg, toArg, fromArg, toArg, toArg, zoneArg, fromArg, zoneArg,
			fromArg, zoneArg, toArg, zoneArg, toArg, fromArg)
	} else if filter.From != nil {
		timeZone := filter.TimeZone
		if timeZone == "" {
			timeZone = "UTC"
		}
		args = append(args, filter.From.Format("2006-01-02"), timeZone)
		fromArg, zoneArg := len(args)-1, len(args)
		query += fmt.Sprintf(` AND (
  due_on >= $%d
  OR planned_on >= $%d
  OR planned_start_at >= ($%d::date::timestamp AT TIME ZONE $%d)
  OR (
    recurrence IS NOT NULL
    AND (recurrence->>'endsOn' IS NULL OR (recurrence->>'endsOn')::date >= $%d)
  )
 )`, fromArg, fromArg, fromArg, zoneArg, fromArg)
	} else if filter.To != nil {
		timeZone := filter.TimeZone
		if timeZone == "" {
			timeZone = "UTC"
		}
		args = append(args, filter.To.Format("2006-01-02"), timeZone)
		toArg, zoneArg := len(args)-1, len(args)
		query += fmt.Sprintf(` AND (
  due_on <= $%d
  OR planned_on <= $%d
  OR planned_start_at < (($%d::date + 1)::timestamp AT TIME ZONE $%d)
  OR (recurrence IS NOT NULL AND (recurrence->>'startsOn')::date <= $%d)
 )`, toArg, toArg, toArg, zoneArg, toArg)
	}
	args = append(args, filter.Limit, filter.Offset)
	query += fmt.Sprintf(" ORDER BY due_on ASC NULLS LAST, updated_at DESC, id ASC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list versioned tasks: %w", err)
	}
	defer rows.Close()
	tasks := make([]V2Task, 0)
	for rows.Next() {
		task, err := scanV2Task(rows)
		if err != nil {
			return nil, fmt.Errorf("scan versioned task: %w", err)
		}
		task.Dependencies, err = s.v2TaskDependencies(ctx, schema, workspaceID, task.ID)
		if err != nil {
			return nil, err
		}
		if task.Dependencies == nil {
			task.Dependencies = []string{}
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versioned tasks: %w", err)
	}
	return tasks, nil
}

func (s *Store) CreateV2Task(ctx context.Context, workspaceID, actorUserID string, input V2TaskWrite) (V2Task, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Task{}, err
	}
	taskID, err := id.New()
	if err != nil {
		return V2Task{}, fmt.Errorf("generate task identifier: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2Task{}, fmt.Errorf("begin task create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if input.AssignedToUserID != nil {
		if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, *input.AssignedToUserID); err != nil {
			return V2Task{}, err
		}
	}
	var completedAt, archivedAt *time.Time
	now := time.Now().UTC()
	if input.Status == "done" {
		completedAt = &now
	}
	if input.Status == "archived" {
		archivedAt = &now
	}
	_, err = tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "tasks")+` (
  id, workspace_id, created_by_user_id, assigned_to_user_id, kind, title,
  details, status, priority, due_on, planned_start_at,
  planned_on, planned_start_time, duration_minutes, due_time, time_zone, label,
  goal_id, parent_task_id, recurrence, source_type, source_reference,
  completed_at, archived_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		taskID, workspaceID, actorUserID, input.AssignedToUserID, input.Kind,
		input.Title, input.Details, input.Status, input.Priority, input.DueOn,
		input.PlannedStartAt, input.PlannedOn, input.PlannedStartTime, input.DurationMinutes,
		input.DueTime, input.TimeZone, input.Label, input.GoalID, input.ParentTaskID, input.Recurrence,
		input.SourceType, input.SourceReference, completedAt, archivedAt,
	)
	if err != nil {
		return V2Task{}, fmt.Errorf("create versioned task: %w", err)
	}
	for _, dependencyID := range input.Dependencies {
		dependencyIDRow, err := id.New()
		if err != nil {
			return V2Task{}, fmt.Errorf("generate task dependency identifier: %w", err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "task_dependencies")+`
  (id, workspace_id, task_id, depends_on_task_id, created_by_user_id)
VALUES ($1,$2,$3,$4,$5)`,
			dependencyIDRow, workspaceID, taskID, dependencyID, actorUserID,
		); err != nil {
			return V2Task{}, fmt.Errorf("create task dependency: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return V2Task{}, fmt.Errorf("commit task create: %w", err)
	}
	return s.GetV2Task(ctx, workspaceID, taskID)
}

func (s *Store) UpdateV2Task(ctx context.Context, workspaceID, taskID, actorUserID string, input V2TaskWrite) (V2Task, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2Task{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2Task{}, fmt.Errorf("begin task update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedTaskID string
	if err := tx.QueryRow(ctx, `
SELECT id FROM `+v2Table(schema, "tasks")+`
WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, workspaceID, taskID).Scan(&lockedTaskID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return V2Task{}, ErrNotFound
		}
		return V2Task{}, fmt.Errorf("lock task for update: %w", err)
	}
	if input.AssignedToUserID != nil {
		if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, *input.AssignedToUserID); err != nil {
			return V2Task{}, err
		}
	}
	if input.ParentTaskID != nil {
		if *input.ParentTaskID == taskID {
			return V2Task{}, ErrV2Cycle
		}
		var ancestor bool
		err := tx.QueryRow(ctx, `
WITH RECURSIVE ancestors(id, parent_task_id) AS (
  SELECT id, parent_task_id FROM `+v2Table(schema, "tasks")+`
  WHERE workspace_id = $1 AND id = $2
  UNION ALL
  SELECT t.id, t.parent_task_id
  FROM `+v2Table(schema, "tasks")+` t
  JOIN ancestors a ON t.workspace_id = $1 AND t.id = a.parent_task_id
  WHERE a.parent_task_id IS NOT NULL
)
SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $3)`,
			workspaceID, *input.ParentTaskID, taskID,
		).Scan(&ancestor)
		if err != nil {
			return V2Task{}, fmt.Errorf("check task parent cycle: %w", err)
		}
		if ancestor {
			return V2Task{}, ErrV2Cycle
		}
	}
	now := time.Now().UTC()
	var completedAt, archivedAt *time.Time
	if input.Status == "done" {
		completedAt = &now
	}
	if input.Status == "archived" {
		archivedAt = &now
	}
	tag, err := tx.Exec(ctx, `
UPDATE `+v2Table(schema, "tasks")+`
SET assigned_to_user_id=$3, kind=$4, title=$5, details=$6, status=$7,
    priority=$8, due_on=$9, planned_start_at=$10,
    planned_on=$11, planned_start_time=$12, duration_minutes=$13, due_time=$14,
    time_zone=$15, label=$16, goal_id=$17, parent_task_id=$18, recurrence=$19,
    source_type=$20, source_reference=$21, completed_at=$22, archived_at=$23, updated_at=now()
WHERE workspace_id=$1 AND id=$2`,
		workspaceID, taskID, input.AssignedToUserID, input.Kind, input.Title,
		input.Details, input.Status, input.Priority, input.DueOn, input.PlannedStartAt,
		input.PlannedOn, input.PlannedStartTime, input.DurationMinutes, input.DueTime,
		input.TimeZone, input.Label, input.GoalID, input.ParentTaskID, input.Recurrence,
		input.SourceType, input.SourceReference, completedAt, archivedAt,
	)
	if err != nil {
		return V2Task{}, fmt.Errorf("update versioned task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return V2Task{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM `+v2Table(schema, "task_dependencies")+` WHERE workspace_id=$1 AND task_id=$2`, workspaceID, taskID); err != nil {
		return V2Task{}, fmt.Errorf("replace task dependencies: %w", err)
	}
	for _, dependencyID := range input.Dependencies {
		var cycle bool
		err := tx.QueryRow(ctx, `
WITH RECURSIVE dependencies(id) AS (
  SELECT $3::text
  UNION
  SELECT d.depends_on_task_id
  FROM `+v2Table(schema, "task_dependencies")+` d
  JOIN dependencies tree ON d.workspace_id=$1 AND d.task_id=tree.id
)
SELECT EXISTS (SELECT 1 FROM dependencies WHERE id=$2)`,
			workspaceID, taskID, dependencyID,
		).Scan(&cycle)
		if err != nil {
			return V2Task{}, fmt.Errorf("check task dependency cycle: %w", err)
		}
		if cycle {
			return V2Task{}, ErrV2Cycle
		}
		dependencyRowID, err := id.New()
		if err != nil {
			return V2Task{}, fmt.Errorf("generate task dependency identifier: %w", err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "task_dependencies")+`
  (id, workspace_id, task_id, depends_on_task_id, created_by_user_id)
VALUES ($1,$2,$3,$4,$5)`,
			dependencyRowID, workspaceID, taskID, dependencyID, actorUserID,
		); err != nil {
			return V2Task{}, fmt.Errorf("update task dependency: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return V2Task{}, fmt.Errorf("commit task update: %w", err)
	}
	return s.GetV2Task(ctx, workspaceID, taskID)
}

func (s *Store) ArchiveV2Task(ctx context.Context, workspaceID, taskID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE `+v2Table(schema, "tasks")+`
SET status='archived', archived_at=now(), completed_at=NULL, updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND status <> 'archived'`, workspaceID, taskID)
	if err != nil {
		return fmt.Errorf("archive versioned task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+v2Table(schema, "tasks")+` WHERE workspace_id=$1 AND id=$2)`, workspaceID, taskID).Scan(&exists); err != nil {
			return fmt.Errorf("check archived task: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) ListV2TaskOccurrenceStates(ctx context.Context, workspaceID, taskID string, from, to time.Time) ([]V2TaskOccurrence, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT task_id, occurrence_on, status, resolved_by_user_id,
       completed_by_user_id, completed_at, due_at_override
FROM `+v2Table(schema, "task_occurrences")+`
WHERE workspace_id=$1 AND task_id=$2 AND occurrence_on BETWEEN $3 AND $4
ORDER BY occurrence_on`, workspaceID, taskID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list task occurrence state: %w", err)
	}
	defer rows.Close()
	result := make([]V2TaskOccurrence, 0)
	for rows.Next() {
		var occurrence V2TaskOccurrence
		if err := rows.Scan(
			&occurrence.TaskID, &occurrence.OccurrenceOn, &occurrence.Status,
			&occurrence.ResolvedByUserID, &occurrence.CompletedByUserID,
			&occurrence.CompletedAt, &occurrence.DueAtOverride,
		); err != nil {
			return nil, fmt.Errorf("scan task occurrence state: %w", err)
		}
		occurrence.History, err = s.v2OccurrenceHistory(ctx, schema, workspaceID, taskID, occurrence.OccurrenceOn)
		if err != nil {
			return nil, err
		}
		result = append(result, occurrence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task occurrence states: %w", err)
	}
	return result, nil
}

func (s *Store) v2OccurrenceHistory(ctx context.Context, schema, workspaceID, taskID string, date time.Time) ([]V2TaskOccurrenceEvent, error) {
	rows, err := s.pool.Query(ctx, `
SELECT from_status, to_status, changed_by_user_id, changed_at
FROM `+v2Table(schema, "task_occurrence_events")+`
WHERE workspace_id=$1 AND task_id=$2 AND occurrence_on=$3
ORDER BY changed_at, id`, workspaceID, taskID, date)
	if err != nil {
		return nil, fmt.Errorf("read task occurrence history: %w", err)
	}
	defer rows.Close()
	history := make([]V2TaskOccurrenceEvent, 0)
	for rows.Next() {
		var event V2TaskOccurrenceEvent
		if err := rows.Scan(&event.FromStatus, &event.ToStatus, &event.ChangedByUserID, &event.ChangedAt); err != nil {
			return nil, fmt.Errorf("scan task occurrence history: %w", err)
		}
		history = append(history, event)
	}
	return history, rows.Err()
}

func (s *Store) UpdateV2TaskOccurrence(ctx context.Context, workspaceID, taskID, actorUserID string, date time.Time, status string, dueAt *time.Time) (V2TaskOccurrence, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2TaskOccurrence{}, err
	}
	rowID, err := id.New()
	if err != nil {
		return V2TaskOccurrence{}, fmt.Errorf("generate occurrence identifier: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return V2TaskOccurrence{}, fmt.Errorf("begin occurrence update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var oldStatus string
	var oldDueAt *time.Time
	var oldCompletedBy *string
	var oldCompletedAt *time.Time
	err = tx.QueryRow(ctx, `
SELECT status, due_at_override, completed_by_user_id, completed_at
FROM `+v2Table(schema, "task_occurrences")+`
WHERE workspace_id=$1 AND task_id=$2 AND occurrence_on=$3
FOR UPDATE`, workspaceID, taskID, date).Scan(&oldStatus, &oldDueAt, &oldCompletedBy, &oldCompletedAt)
	existed := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		oldStatus = "open"
	} else if err != nil {
		return V2TaskOccurrence{}, fmt.Errorf("read occurrence before update: %w", err)
	}
	if existed && oldStatus == status && sameV2Time(oldDueAt, dueAt) {
		if err := tx.Commit(ctx); err != nil {
			return V2TaskOccurrence{}, fmt.Errorf("commit idempotent occurrence update: %w", err)
		}
		states, err := s.ListV2TaskOccurrenceStates(ctx, workspaceID, taskID, date, date)
		if err != nil {
			return V2TaskOccurrence{}, err
		}
		if len(states) != 1 {
			return V2TaskOccurrence{}, ErrNotFound
		}
		return states[0], nil
	}
	now := time.Now().UTC()
	var completedBy *string
	var completedAt *time.Time
	if status == "done" {
		if oldStatus == "done" && existed {
			completedBy = oldCompletedBy
			completedAt = oldCompletedAt
		} else {
			completedBy = &actorUserID
			completedAt = &now
		}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "task_occurrences")+` (
  id, workspace_id, task_id, occurrence_on, status, resolved_by_user_id,
  completed_by_user_id, completed_at, due_at_override, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())
ON CONFLICT (workspace_id, task_id, occurrence_on) DO UPDATE SET
  status=EXCLUDED.status,
  resolved_by_user_id=EXCLUDED.resolved_by_user_id,
  completed_by_user_id=EXCLUDED.completed_by_user_id,
  completed_at=EXCLUDED.completed_at,
  due_at_override=EXCLUDED.due_at_override,
  updated_at=now()`,
		rowID, workspaceID, taskID, date, status, actorUserID, completedBy,
		completedAt, dueAt,
	)
	if err != nil {
		return V2TaskOccurrence{}, fmt.Errorf("persist task occurrence: %w", err)
	}
	if oldStatus != status {
		eventID, err := id.New()
		if err != nil {
			return V2TaskOccurrence{}, fmt.Errorf("generate occurrence history identifier: %w", err)
		}
		var fromStatus *string
		if existed || oldDueAt != nil {
			fromStatus = &oldStatus
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "task_occurrence_events")+`
  (id, workspace_id, task_id, occurrence_on, from_status, to_status, changed_by_user_id)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			eventID, workspaceID, taskID, date, fromStatus, status, actorUserID,
		); err != nil {
			return V2TaskOccurrence{}, fmt.Errorf("record task occurrence history: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return V2TaskOccurrence{}, fmt.Errorf("commit occurrence update: %w", err)
	}
	states, err := s.ListV2TaskOccurrenceStates(ctx, workspaceID, taskID, date, date)
	if err != nil {
		return V2TaskOccurrence{}, err
	}
	if len(states) != 1 {
		return V2TaskOccurrence{}, ErrNotFound
	}
	return states[0], nil
}

func sameV2Time(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (s *Store) ListV2AgendaOccurrenceStates(ctx context.Context, workspaceID string, taskIDs []string, from, to time.Time) ([]V2TaskOccurrence, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	if len(taskIDs) == 0 {
		return []V2TaskOccurrence{}, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT task_id, occurrence_on, status, resolved_by_user_id,
       completed_by_user_id, completed_at, due_at_override
FROM `+v2Table(schema, "task_occurrences")+`
WHERE workspace_id=$1 AND occurrence_on BETWEEN $2 AND $3
  AND task_id = ANY($4::text[])
ORDER BY task_id, occurrence_on`, workspaceID, from, to, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("list agenda occurrence states: %w", err)
	}
	defer rows.Close()
	result := make([]V2TaskOccurrence, 0)
	for rows.Next() {
		var occurrence V2TaskOccurrence
		if err := rows.Scan(
			&occurrence.TaskID, &occurrence.OccurrenceOn, &occurrence.Status,
			&occurrence.ResolvedByUserID, &occurrence.CompletedByUserID,
			&occurrence.CompletedAt, &occurrence.DueAtOverride,
		); err != nil {
			return nil, fmt.Errorf("scan agenda occurrence state: %w", err)
		}
		result = append(result, occurrence)
	}
	return result, rows.Err()
}
