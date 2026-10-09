-- Add the Home projection, saved focus, and consent-gated provider access.
-- Message bodies and provider credentials remain in their existing systems.
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_provider_account_access (
  workspace_id text NOT NULL
    REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  provider_connection_id text NOT NULL
    REFERENCES provider_connections(id) ON DELETE CASCADE,
  owner_user_id varchar NOT NULL,
  access_scope text NOT NULL DEFAULT 'owner_only'
    CHECK (access_scope IN ('owner_only', 'space_shared')),
  promoted_by_user_id varchar REFERENCES users(id) ON DELETE RESTRICT,
  promoted_at timestamptz,
  email_read_consent_at timestamptz,
  email_read_consent_policy_version varchar(32),
  email_read_consent_revoked_at timestamptz,
  home_summary_enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, provider_connection_id),
  UNIQUE (provider_connection_id),
  FOREIGN KEY (workspace_id, owner_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE,
  CHECK ((promoted_by_user_id IS NULL) = (promoted_at IS NULL)),
  CHECK ((access_scope = 'space_shared') = (promoted_at IS NOT NULL)),
  CHECK ((email_read_consent_at IS NULL) = (email_read_consent_policy_version IS NULL)),
  CHECK (email_read_consent_revoked_at IS NULL OR email_read_consent_at IS NOT NULL),
  CHECK (email_read_consent_at IS NULL OR access_scope = 'space_shared')
);
--> statement-breakpoint
CREATE INDEX workspace_provider_account_access_scope_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_provider_account_access
    (workspace_id, access_scope, home_summary_enabled);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_email_summaries (
  workspace_id text NOT NULL,
  provider_connection_id text NOT NULL,
  unread_inbox_count integer NOT NULL DEFAULT 0 CHECK (unread_inbox_count >= 0),
  action_needed_count integer NOT NULL DEFAULT 0 CHECK (action_needed_count >= 0),
  sync_state text NOT NULL DEFAULT 'unavailable'
    CHECK (sync_state IN ('current', 'stale', 'syncing', 'error', 'unavailable')),
  last_synced_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, provider_connection_id),
  FOREIGN KEY (workspace_id, provider_connection_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_provider_account_access
      (workspace_id, provider_connection_id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX workspace_email_summaries_state_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_email_summaries
    (workspace_id, sync_state, last_synced_at);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.home_focus_selections (
  workspace_id text NOT NULL,
  user_id varchar NOT NULL,
  plan_day date NOT NULL,
  task_id text NOT NULL,
  occurrence_on date,
  position smallint NOT NULL CHECK (position BETWEEN 0 AND 2),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id, plan_day, position),
  FOREIGN KEY (workspace_id, user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, task_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.tasks(workspace_id, id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE UNIQUE INDEX home_focus_selections_task_without_occurrence_idx
  ON @ASKOLO_V2_SCHEMA@.home_focus_selections
    (workspace_id, user_id, plan_day, task_id)
  WHERE occurrence_on IS NULL;
--> statement-breakpoint
CREATE UNIQUE INDEX home_focus_selections_task_with_occurrence_idx
  ON @ASKOLO_V2_SCHEMA@.home_focus_selections
    (workspace_id, user_id, plan_day, task_id, occurrence_on)
  WHERE occurrence_on IS NOT NULL;
--> statement-breakpoint
CREATE INDEX home_focus_selections_order_idx
  ON @ASKOLO_V2_SCHEMA@.home_focus_selections
    (workspace_id, user_id, plan_day, position);
