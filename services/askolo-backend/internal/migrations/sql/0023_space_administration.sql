-- Space administration is staged in the isolated V2 schema. The stable
-- workspace rows also keep the public-facing details used by authorization
-- and Space listing, so mirror the small profile fields there.
ALTER TABLE workspaces
  ADD COLUMN description text,
  ADD COLUMN image_object_path text,
  ADD COLUMN time_zone text NOT NULL DEFAULT 'UTC',
  ADD COLUMN slug text,
  ADD CONSTRAINT workspaces_description_length_check
    CHECK (description IS NULL OR length(description) <= 2000),
  ADD CONSTRAINT workspaces_image_object_path_check
    CHECK (image_object_path IS NULL OR image_object_path LIKE '/objects/%'),
  ADD CONSTRAINT workspaces_slug_format_check
    CHECK (slug IS NULL OR slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
--> statement-breakpoint
CREATE UNIQUE INDEX workspaces_slug_unique_idx
  ON workspaces (lower(slug))
  WHERE slug IS NOT NULL;
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.workspaces
  ADD COLUMN description text,
  ADD COLUMN image_object_path text,
  ADD COLUMN slug text,
  ADD CONSTRAINT v2_workspaces_description_length_check
    CHECK (description IS NULL OR length(description) <= 2000),
  ADD CONSTRAINT v2_workspaces_image_object_path_check
    CHECK (image_object_path IS NULL OR image_object_path LIKE '/objects/%'),
  ADD CONSTRAINT v2_workspaces_slug_format_check
    CHECK (slug IS NULL OR slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
--> statement-breakpoint
CREATE UNIQUE INDEX v2_workspaces_slug_unique_idx
  ON @ASKOLO_V2_SCHEMA@.workspaces (lower(slug))
  WHERE slug IS NOT NULL;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_groups (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  description text CHECK (description IS NULL OR length(description) <= 500),
  created_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, name),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX workspace_groups_workspace_name_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_groups (workspace_id, name, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_group_members (
  workspace_id text NOT NULL,
  group_id text NOT NULL,
  user_id varchar NOT NULL,
  added_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, group_id, user_id),
  FOREIGN KEY (workspace_id, group_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_groups(workspace_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, added_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX workspace_group_members_user_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_group_members (workspace_id, user_id, group_id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_group_permissions (
  workspace_id text NOT NULL,
  group_id text NOT NULL,
  permission_key text NOT NULL
    CHECK (permission_key ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*){1,2}$'),
  granted_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, group_id, permission_key),
  FOREIGN KEY (workspace_id, group_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_groups(workspace_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, granted_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_invitations (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  email text NOT NULL CHECK (length(email) BETWEEN 3 AND 320),
  email_normalized text NOT NULL CHECK (email_normalized = lower(btrim(email_normalized))),
  role text NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
  token_hash char(64) NOT NULL UNIQUE CHECK (token_hash ~ '^[a-f0-9]{64}$'),
  invited_by_user_id varchar NOT NULL,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled', 'expired')),
  expires_at timestamptz NOT NULL,
  accepted_by_user_id varchar,
  accepted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, invited_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, accepted_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK ((status = 'accepted') = (accepted_at IS NOT NULL)),
  CHECK ((status = 'accepted') = (accepted_by_user_id IS NOT NULL))
);
--> statement-breakpoint
CREATE UNIQUE INDEX workspace_invitations_pending_email_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_invitations (workspace_id, email_normalized)
  WHERE status = 'pending';
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_notification_preferences (
  workspace_id text NOT NULL,
  user_id varchar NOT NULL,
  channel text NOT NULL CHECK (channel IN ('email', 'in_app')),
  event_key text NOT NULL
    CHECK (event_key IN ('membership', 'assignments', 'reminders')),
  enabled boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id, channel, event_key),
  FOREIGN KEY (workspace_id, user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_notifications (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  recipient_user_id varchar NOT NULL,
  event_key text NOT NULL
    CHECK (event_key IN ('membership', 'assignments', 'reminders')),
  resource_type text,
  resource_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  read_at timestamptz,
  FOREIGN KEY (workspace_id, recipient_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE,
  CHECK ((resource_type IS NULL) = (resource_id IS NULL))
);
--> statement-breakpoint
CREATE INDEX workspace_notifications_inbox_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_notifications
    (workspace_id, recipient_user_id, read_at, created_at DESC, id);
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.tasks
  ADD COLUMN assigned_to_group_id text,
  ADD CONSTRAINT tasks_group_assignee_fk
    FOREIGN KEY (workspace_id, assigned_to_group_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_groups(workspace_id, id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT tasks_single_assignee_check
    CHECK (assigned_to_user_id IS NULL OR assigned_to_group_id IS NULL);
--> statement-breakpoint
CREATE INDEX tasks_workspace_group_assignee_idx
  ON @ASKOLO_V2_SCHEMA@.tasks
    (workspace_id, assigned_to_group_id, status, updated_at DESC)
  WHERE assigned_to_group_id IS NOT NULL;
