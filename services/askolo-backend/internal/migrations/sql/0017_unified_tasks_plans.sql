-- Add the unified Task/Plan contract beside the existing staged workspace
-- schema. The operational public-schema migration prefix remains unchanged.
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.tasks (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  assigned_to_user_id varchar,
  kind text NOT NULL CHECK (kind IN ('action', 'chore', 'habit', 'commitment')),
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  details text,
  status text NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'in_progress', 'done', 'cancelled', 'archived')),
  priority text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
  due_on date,
  planned_start_at timestamptz,
  goal_id text,
  parent_task_id text,
  recurrence jsonb,
  source_type text,
  source_reference text,
  completed_at timestamptz,
  archived_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, assigned_to_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, goal_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.goals(workspace_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, parent_task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id) ON DELETE RESTRICT,
  CHECK (parent_task_id IS NULL OR parent_task_id <> id),
  CHECK ((source_type IS NULL) = (source_reference IS NULL)),
  CHECK (source_type IS NULL OR source_type IN ('manual', 'assistant', 'integration')),
  CHECK (recurrence IS NULL OR jsonb_typeof(recurrence) = 'object'),
  CHECK ((status = 'done') = (completed_at IS NOT NULL)),
  CHECK ((status = 'archived') = (archived_at IS NOT NULL))
);
--> statement-breakpoint
CREATE INDEX tasks_workspace_status_due_idx
  ON @ASKOLO_V2_SCHEMA@.tasks (workspace_id, status, due_on, id);
--> statement-breakpoint
CREATE INDEX tasks_workspace_assignee_status_idx
  ON @ASKOLO_V2_SCHEMA@.tasks (workspace_id, assigned_to_user_id, status, updated_at DESC);
--> statement-breakpoint
CREATE INDEX tasks_workspace_goal_idx
  ON @ASKOLO_V2_SCHEMA@.tasks (workspace_id, goal_id)
  WHERE goal_id IS NOT NULL;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.task_dependencies (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  task_id text NOT NULL,
  depends_on_task_id text NOT NULL,
  created_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, depends_on_task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  CHECK (task_id <> depends_on_task_id),
  UNIQUE (workspace_id, task_id, depends_on_task_id)
);
--> statement-breakpoint
CREATE INDEX task_dependencies_workspace_depends_on_idx
  ON @ASKOLO_V2_SCHEMA@.task_dependencies (workspace_id, depends_on_task_id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.task_occurrences (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  task_id text NOT NULL,
  occurrence_on date NOT NULL,
  status text NOT NULL CHECK (status IN ('open', 'done', 'skipped', 'cancelled')),
  resolved_by_user_id varchar NOT NULL,
  completed_by_user_id varchar,
  completed_at timestamptz,
  due_at_override timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, resolved_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, completed_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  UNIQUE (workspace_id, task_id, occurrence_on),
  CHECK ((status = 'done') = (completed_by_user_id IS NOT NULL)),
  CHECK ((status = 'done') = (completed_at IS NOT NULL))
);
--> statement-breakpoint
CREATE INDEX task_occurrences_workspace_date_idx
  ON @ASKOLO_V2_SCHEMA@.task_occurrences (workspace_id, occurrence_on, task_id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.task_occurrence_events (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  task_id text NOT NULL,
  occurrence_on date NOT NULL,
  from_status text CHECK (from_status IS NULL OR from_status IN ('open', 'done', 'skipped', 'cancelled')),
  to_status text NOT NULL CHECK (to_status IN ('open', 'done', 'skipped', 'cancelled')),
  changed_by_user_id varchar NOT NULL,
  changed_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, task_id, occurrence_on)
    REFERENCES @ASKOLO_V2_SCHEMA@.task_occurrences(workspace_id, task_id, occurrence_on) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, changed_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX task_occurrence_events_history_idx
  ON @ASKOLO_V2_SCHEMA@.task_occurrence_events
    (workspace_id, task_id, occurrence_on, changed_at DESC, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.plans (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  owner_user_id varchar NOT NULL,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  details text,
  starts_on date NOT NULL,
  ends_on date NOT NULL,
  time_zone text NOT NULL CHECK (length(btrim(time_zone)) > 0),
  visibility text NOT NULL DEFAULT 'personal' CHECK (visibility IN ('personal', 'shared')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived')),
  updated_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, owner_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, updated_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  CHECK (ends_on >= starts_on)
);
--> statement-breakpoint
CREATE INDEX plans_workspace_visibility_period_idx
  ON @ASKOLO_V2_SCHEMA@.plans (workspace_id, visibility, starts_on, ends_on, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.plan_entries (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  plan_id text NOT NULL,
  task_id text NOT NULL,
  occurrence_on date,
  planned_on date NOT NULL,
  planned_start_at timestamptz,
  planned_end_at timestamptz,
  position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, plan_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.plans(workspace_id, id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT,
  CHECK (planned_end_at IS NULL OR planned_start_at IS NULL OR planned_end_at >= planned_start_at),
  CHECK (occurrence_on IS NULL OR occurrence_on = planned_on)
);
--> statement-breakpoint
CREATE INDEX plan_entries_workspace_schedule_idx
  ON @ASKOLO_V2_SCHEMA@.plan_entries (workspace_id, planned_on, planned_start_at, position, id);
--> statement-breakpoint
CREATE INDEX plan_entries_plan_schedule_idx
  ON @ASKOLO_V2_SCHEMA@.plan_entries (workspace_id, plan_id, planned_on, position, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.reminder_rules (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  owner_user_id varchar NOT NULL,
  kind text NOT NULL CHECK (kind IN ('task', 'goal', 'daily_brief')),
  rule jsonb NOT NULL CHECK (jsonb_typeof(rule) = 'object'),
  status text NOT NULL DEFAULT 'pending_automation'
    CHECK (status IN ('pending_automation', 'disabled')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, owner_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id) ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX reminder_rules_workspace_owner_idx
  ON @ASKOLO_V2_SCHEMA@.reminder_rules (workspace_id, owner_user_id, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.api_request_limits (
  user_id varchar PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  read_window_started_at timestamptz NOT NULL,
  read_request_count integer NOT NULL CHECK (read_request_count >= 0),
  write_window_started_at timestamptz NOT NULL,
  write_request_count integer NOT NULL CHECK (write_request_count >= 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);
