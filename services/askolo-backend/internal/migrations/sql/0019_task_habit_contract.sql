-- Add the Task/Habit contract without rewriting staged V2 rows during Publish.
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  DROP CONSTRAINT IF EXISTS tasks_kind_check;
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  ADD CONSTRAINT tasks_kind_check
    CHECK (kind IN ('task', 'habit', 'action', 'chore', 'commitment'));
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  DROP CONSTRAINT IF EXISTS tasks_priority_check;
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  ALTER COLUMN priority SET DEFAULT 'none',
  ADD CONSTRAINT tasks_priority_check CHECK (priority IN ('none', 'low', 'normal', 'high', 'medium')),
  ADD CONSTRAINT tasks_habit_has_no_priority_check
    CHECK (kind <> 'habit' OR priority IN ('none', 'medium'));
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  ADD COLUMN planned_on date,
  ADD COLUMN planned_start_time time,
  ADD COLUMN duration_minutes integer CHECK (duration_minutes BETWEEN 1 AND 1440),
  ADD COLUMN due_time time,
  ADD COLUMN time_zone text NOT NULL DEFAULT 'UTC',
  ADD COLUMN label text CHECK (label IS NULL OR length(btrim(label)) BETWEEN 1 AND 100);
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  ADD CONSTRAINT tasks_planned_start_requires_date_check
    CHECK (planned_start_time IS NULL OR planned_on IS NOT NULL);
--> statement-breakpoint
CREATE INDEX tasks_workspace_planned_on_idx
  ON @ASKOLO_V2_SCHEMA@.tasks (workspace_id, planned_on, status, id)
  WHERE planned_on IS NOT NULL;
