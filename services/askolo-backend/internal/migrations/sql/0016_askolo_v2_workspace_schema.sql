-- Add an isolated, workspace-first target schema. This migration creates
-- structure only: legacy rows and API behavior are intentionally untouched.
-- The runner substitutes a database-local namespace for the token below so
-- schema-isolated migration tests do not share objects.
CREATE SCHEMA @ASKOLO_V2_SCHEMA@;
REVOKE ALL ON SCHEMA @ASKOLO_V2_SCHEMA@ FROM PUBLIC;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspaces (
  id text PRIMARY KEY,
  owner_user_id varchar NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  owner_role text NOT NULL DEFAULT 'owner' CHECK (owner_role = 'owner'),
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'suspended', 'archived')),
  is_personal boolean NOT NULL DEFAULT false,
  time_zone text NOT NULL DEFAULT 'UTC' CHECK (length(btrim(time_zone)) > 0),
  default_locale varchar(2) NOT NULL DEFAULT 'en'
    CHECK (default_locale ~ '^[a-z]{2}$'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id, owner_user_id),
  UNIQUE (id, owner_user_id, owner_role)
);
--> statement-breakpoint
CREATE UNIQUE INDEX workspaces_personal_owner_unique_idx
  ON @ASKOLO_V2_SCHEMA@.workspaces (owner_user_id)
  WHERE is_personal;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_memberships (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  user_id varchar NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('invited', 'active', 'suspended', 'revoked')),
  invited_by_user_id varchar REFERENCES users(id) ON DELETE SET NULL,
  joined_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, user_id),
  UNIQUE (workspace_id, user_id, role),
  UNIQUE (workspace_id, id),
  CONSTRAINT workspace_membership_status_timestamps_check CHECK (
    (status = 'revoked') = (revoked_at IS NOT NULL)
  )
);
--> statement-breakpoint
ALTER TABLE @ASKOLO_V2_SCHEMA@.workspaces
  ADD CONSTRAINT workspaces_owner_membership_fk
  FOREIGN KEY (id, owner_user_id, owner_role)
  REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id, role)
  DEFERRABLE INITIALLY DEFERRED;
--> statement-breakpoint
CREATE UNIQUE INDEX workspace_memberships_one_owner_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_memberships (workspace_id)
  WHERE role = 'owner';
--> statement-breakpoint
CREATE INDEX workspace_memberships_user_status_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_memberships (user_id, status, workspace_id);
--> statement-breakpoint
CREATE INDEX workspace_memberships_workspace_status_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_memberships (workspace_id, status, role);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.plan_catalog (
  id text PRIMARY KEY,
  billing_model text NOT NULL CHECK (billing_model IN ('individual', 'team')),
  plan_code varchar(16) NOT NULL CHECK (plan_code IN ('plus', 'pro', 'max')),
  included_credits bigint NOT NULL CHECK (included_credits > 0),
  price_currency varchar(3) NOT NULL DEFAULT 'USD'
    CHECK (price_currency ~ '^[A-Z]{3}$'),
  price_micros bigint NOT NULL CHECK (price_micros >= 0),
  billing_period_days smallint NOT NULL DEFAULT 30 CHECK (billing_period_days > 0),
  effective_from timestamptz NOT NULL,
  effective_until timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (billing_model, plan_code, effective_from),
  UNIQUE (id, billing_model),
  CHECK (effective_until IS NULL OR effective_until > effective_from)
);
--> statement-breakpoint
CREATE INDEX plan_catalog_current_idx
  ON @ASKOLO_V2_SCHEMA@.plan_catalog (billing_model, plan_code, effective_from DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_subscriptions (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  billing_owner_user_id varchar NOT NULL,
  billing_model text NOT NULL CHECK (billing_model IN ('individual', 'team')),
  status text NOT NULL CHECK (status IN ('trialing', 'active', 'past_due', 'cancelled')),
  billing_period_start timestamptz NOT NULL,
  billing_period_end timestamptz NOT NULL,
  cancel_at_period_end boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id, billing_model),
  FOREIGN KEY (workspace_id, billing_owner_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id, owner_user_id)
    ON DELETE RESTRICT,
  CHECK (billing_period_end > billing_period_start)
);
--> statement-breakpoint
CREATE UNIQUE INDEX workspace_subscriptions_one_current_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_subscriptions (workspace_id)
  WHERE status IN ('trialing', 'active', 'past_due');
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_subscription_seats (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  subscription_id text NOT NULL,
  membership_id text NOT NULL,
  billing_model text NOT NULL CHECK (billing_model IN ('individual', 'team')),
  plan_id text NOT NULL,
  seat_source text NOT NULL CHECK (seat_source IN ('invitation', 'slack', 'owner_added')),
  status text NOT NULL CHECK (status IN ('trialing', 'active', 'ending', 'ended')),
  included_credits_snapshot bigint NOT NULL CHECK (included_credits_snapshot > 0),
  price_micros_snapshot bigint NOT NULL CHECK (price_micros_snapshot >= 0),
  trial_started_at timestamptz,
  trial_ends_at timestamptz,
  billing_period_start timestamptz NOT NULL,
  billing_period_end timestamptz NOT NULL,
  paid_through_at timestamptz NOT NULL,
  ended_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, subscription_id, id),
  FOREIGN KEY (workspace_id, subscription_id, billing_model)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_subscriptions(workspace_id, id, billing_model)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, membership_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, id)
    ON DELETE RESTRICT,
  FOREIGN KEY (plan_id, billing_model)
    REFERENCES @ASKOLO_V2_SCHEMA@.plan_catalog(id, billing_model)
    ON DELETE RESTRICT,
  CHECK ((trial_started_at IS NULL) = (trial_ends_at IS NULL)),
  CHECK (trial_ends_at IS NULL OR trial_ends_at > trial_started_at),
  CHECK (seat_source <> 'slack' OR (
    trial_started_at IS NOT NULL
    AND trial_ends_at = trial_started_at + INTERVAL '7 days'
  )),
  CHECK (seat_source = 'slack' OR trial_started_at IS NULL),
  CHECK (billing_period_end > billing_period_start),
  CHECK (paid_through_at >= billing_period_end),
  CHECK (ended_at IS NULL OR ended_at >= paid_through_at),
  CHECK ((status = 'ended') = (ended_at IS NOT NULL))
);
--> statement-breakpoint
CREATE UNIQUE INDEX workspace_subscription_seats_one_current_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_subscription_seats (workspace_id, membership_id)
  WHERE status IN ('trialing', 'active', 'ending');
--> statement-breakpoint
CREATE INDEX workspace_subscription_seats_period_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_subscription_seats (workspace_id, paid_through_at, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_member_credit_limits (
  workspace_id text NOT NULL,
  user_id varchar NOT NULL,
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  spending_limit_credits bigint NOT NULL CHECK (spending_limit_credits >= 0),
  changed_by_user_id varchar NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id, period_start),
  FOREIGN KEY (workspace_id, user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, changed_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK (period_end > period_start)
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_member_permissions (
  workspace_id text NOT NULL,
  user_id varchar NOT NULL,
  permission_key text NOT NULL
    CHECK (permission_key ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*){1,2}$'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id, permission_key),
  FOREIGN KEY (workspace_id, user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.member_preferences (
  user_id varchar PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  preferred_locale varchar(2) CHECK (preferred_locale IS NULL OR preferred_locale ~ '^[a-z]{2}$'),
  time_zone text CHECK (time_zone IS NULL OR length(btrim(time_zone)) > 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.member_ai_preferences (
  user_id varchar PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  retention text NOT NULL DEFAULT 'delete_immediately',
  consent_at timestamptz,
  consent_version varchar(32),
  assistant_processing_consent_at timestamptz,
  assistant_processing_consent_version varchar(32),
  live_agent_consent_at timestamptz,
  live_agent_consent_version varchar(32),
  live_agent_voice varchar(32) NOT NULL DEFAULT 'michael'
    CHECK (live_agent_voice IN (
      'michael', 'mary', 'paul', 'vera', 'giovanni',
      'lola', 'juergen', 'rafael', 'estelle'
    )),
  redaction_location text CHECK (redaction_location IS NULL OR redaction_location = 'app'),
  voice_output_consent_at timestamptz,
  voice_output_consent_version varchar(32),
  auto_speak_enabled boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((consent_at IS NULL) = (consent_version IS NULL)),
  CHECK ((assistant_processing_consent_at IS NULL) = (assistant_processing_consent_version IS NULL)),
  CHECK ((live_agent_consent_at IS NULL) = (live_agent_consent_version IS NULL)),
  CHECK ((voice_output_consent_at IS NULL) = (voice_output_consent_version IS NULL))
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.habits (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
  description text,
  frequency text NOT NULL DEFAULT 'daily'
    CHECK (frequency IN ('daily', 'weekly', 'weekdays', 'custom')),
  days_of_week smallint[] NOT NULL DEFAULT '{}',
  color text,
  icon text,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK (
    (frequency = 'custom' AND cardinality(days_of_week) > 0)
    OR (frequency <> 'custom' AND cardinality(days_of_week) = 0)
  ),
  CHECK (0 <= ALL(days_of_week) AND 6 >= ALL(days_of_week))
);
--> statement-breakpoint
CREATE INDEX habits_workspace_status_idx
  ON @ASKOLO_V2_SCHEMA@.habits (workspace_id, status, created_at DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.habit_completions (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  habit_id text NOT NULL,
  completed_by_user_id varchar NOT NULL,
  completed_on date NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, habit_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.habits(workspace_id, id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, completed_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  UNIQUE (workspace_id, habit_id, completed_on, completed_by_user_id)
);
--> statement-breakpoint
CREATE INDEX habit_completions_workspace_date_idx
  ON @ASKOLO_V2_SCHEMA@.habit_completions (workspace_id, completed_on DESC, habit_id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.goals (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  description text,
  target_on date,
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'paused', 'completed', 'cancelled', 'archived')),
  progress_percent smallint NOT NULL DEFAULT 0
    CHECK (progress_percent BETWEEN 0 AND 100),
  category text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX goals_workspace_status_target_idx
  ON @ASKOLO_V2_SCHEMA@.goals (workspace_id, status, target_on);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.daily_plans (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  plan_on date NOT NULL,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'completed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, plan_on),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX daily_plans_workspace_date_idx
  ON @ASKOLO_V2_SCHEMA@.daily_plans (workspace_id, plan_on DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.daily_plan_items (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  plan_id text NOT NULL,
  created_by_user_id varchar NOT NULL,
  assigned_to_user_id varchar,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  details text,
  start_at timestamptz,
  end_at timestamptz,
  priority text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
  status text NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'in_progress', 'done', 'skipped')),
  position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, plan_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.daily_plans(workspace_id, id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, assigned_to_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK (end_at IS NULL OR start_at IS NULL OR end_at >= start_at),
  UNIQUE (workspace_id, id)
);
--> statement-breakpoint
CREATE INDEX daily_plan_items_workspace_status_idx
  ON @ASKOLO_V2_SCHEMA@.daily_plan_items (workspace_id, status, start_at);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.calendar_events (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  updated_by_user_id varchar NOT NULL,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 240),
  description text,
  starts_at timestamptz NOT NULL,
  ends_at timestamptz,
  all_day boolean NOT NULL DEFAULT false,
  time_zone text NOT NULL DEFAULT 'UTC' CHECK (length(btrim(time_zone)) > 0),
  location text,
  color text,
  source_provider text,
  source_event_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, updated_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK (ends_at IS NULL OR ends_at >= starts_at),
  CHECK ((source_provider IS NULL) = (source_event_id IS NULL))
);
--> statement-breakpoint
CREATE UNIQUE INDEX calendar_events_external_unique_idx
  ON @ASKOLO_V2_SCHEMA@.calendar_events (workspace_id, source_provider, source_event_id)
  WHERE source_provider IS NOT NULL;
--> statement-breakpoint
CREATE INDEX calendar_events_workspace_start_idx
  ON @ASKOLO_V2_SCHEMA@.calendar_events (workspace_id, starts_at);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.chores (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  assigned_to_user_id varchar,
  completed_by_user_id varchar,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  description text,
  recurrence text NOT NULL DEFAULT 'once'
    CHECK (recurrence IN ('once', 'daily', 'weekly', 'monthly', 'custom')),
  recurrence_rule jsonb,
  due_on date,
  status text NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'in_progress', 'done', 'skipped', 'archived')),
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, assigned_to_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, completed_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK ((status = 'done') = (completed_at IS NOT NULL)),
  CHECK (recurrence = 'custom' OR recurrence_rule IS NULL),
  CHECK (recurrence_rule IS NULL OR jsonb_typeof(recurrence_rule) = 'object')
);
--> statement-breakpoint
CREATE INDEX chores_workspace_due_status_idx
  ON @ASKOLO_V2_SCHEMA@.chores (workspace_id, due_on, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.notes (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  updated_by_user_id varchar NOT NULL,
  title text NOT NULL DEFAULT '' CHECK (length(title) <= 240),
  content text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, updated_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX notes_workspace_updated_idx
  ON @ASKOLO_V2_SCHEMA@.notes (workspace_id, updated_at DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.action_items (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  assigned_to_user_id varchar,
  title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 240),
  source_type text,
  source_reference text,
  due_on date,
  priority text NOT NULL DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high')),
  status text NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'in_progress', 'done', 'cancelled', 'archived')),
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, assigned_to_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK ((source_type IS NULL) = (source_reference IS NULL)),
  CHECK ((status = 'done') = (completed_at IS NOT NULL))
);
--> statement-breakpoint
CREATE INDEX action_items_workspace_due_status_idx
  ON @ASKOLO_V2_SCHEMA@.action_items (workspace_id, due_on, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.assistant_conversations (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  created_by_user_id varchar NOT NULL,
  title text NOT NULL DEFAULT 'Assistant' CHECK (length(title) <= 200),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  FOREIGN KEY (workspace_id, created_by_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT
);
--> statement-breakpoint
CREATE INDEX assistant_conversations_workspace_updated_idx
  ON @ASKOLO_V2_SCHEMA@.assistant_conversations (workspace_id, updated_at DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.assistant_messages (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  conversation_id text NOT NULL,
  sender_user_id varchar,
  role text NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content text NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 32768),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, conversation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.assistant_conversations(workspace_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, sender_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  UNIQUE (workspace_id, id)
);
--> statement-breakpoint
CREATE INDEX assistant_messages_conversation_created_idx
  ON @ASKOLO_V2_SCHEMA@.assistant_messages (workspace_id, conversation_id, created_at, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_policy_versions (
  version integer PRIMARY KEY CHECK (version > 0),
  operation_weights jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(operation_weights) = 'object'),
  rate_cards jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(rate_cards) = 'object'),
  overrun_margin_percent integer NOT NULL DEFAULT 0
    CHECK (overrun_margin_percent BETWEEN 0 AND 100),
  created_by_user_id varchar REFERENCES users(id) ON DELETE SET NULL,
  change_reason varchar(160) NOT NULL DEFAULT 'initial',
  created_at timestamptz NOT NULL DEFAULT now()
);
--> statement-breakpoint
CREATE INDEX ai_credit_policy_created_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_policy_versions (created_at DESC, version DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_accounts (
  workspace_id text PRIMARY KEY REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  granted_credits bigint NOT NULL DEFAULT 0,
  adjustment_credits bigint NOT NULL DEFAULT 0,
  reserved_credits bigint NOT NULL DEFAULT 0 CHECK (reserved_credits >= 0),
  spent_credits bigint NOT NULL DEFAULT 0 CHECK (spent_credits >= 0),
  refunded_credits bigint NOT NULL DEFAULT 0 CHECK (refunded_credits >= 0),
  reserved_usd_micros bigint NOT NULL DEFAULT 0 CHECK (reserved_usd_micros >= 0),
  spent_usd_micros bigint NOT NULL DEFAULT 0 CHECK (spent_usd_micros >= 0),
  refunded_usd_micros bigint NOT NULL DEFAULT 0 CHECK (refunded_usd_micros >= 0),
  updated_at timestamptz NOT NULL DEFAULT now()
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_grants (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  subscription_seat_id text,
  actor_user_id varchar,
  source_type varchar(40) NOT NULL,
  amount_credits bigint NOT NULL CHECK (amount_credits > 0),
  entitlement_key varchar(160),
  expires_at timestamptz,
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, idempotency_key_hash),
  FOREIGN KEY (workspace_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, subscription_seat_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_subscription_seats(workspace_id, id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, actor_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  CHECK ((source_type = 'seat_cycle') = (subscription_seat_id IS NOT NULL)),
  CHECK (subscription_seat_id IS NULL OR expires_at IS NOT NULL)
);
--> statement-breakpoint
CREATE INDEX ai_credit_grants_workspace_expiry_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_grants (workspace_id, expires_at, created_at);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_grant_balances (
  workspace_id text NOT NULL,
  grant_id text NOT NULL,
  available_credits bigint NOT NULL CHECK (available_credits >= 0),
  reserved_credits bigint NOT NULL DEFAULT 0 CHECK (reserved_credits >= 0),
  spent_credits bigint NOT NULL DEFAULT 0 CHECK (spent_credits >= 0),
  refunded_credits bigint NOT NULL DEFAULT 0 CHECK (refunded_credits >= 0),
  expired_credits bigint NOT NULL DEFAULT 0 CHECK (expired_credits >= 0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, grant_id),
  FOREIGN KEY (workspace_id, grant_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_grants(workspace_id, id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_adjustments (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  actor_user_id varchar NOT NULL,
  amount_credits bigint NOT NULL CHECK (amount_credits <> 0),
  reason varchar(160) NOT NULL,
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  reversal_of_id text,
  reversal_of_grant_id text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, idempotency_key_hash),
  FOREIGN KEY (workspace_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspaces(id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, actor_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, reversal_of_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_adjustments(workspace_id, id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, reversal_of_grant_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_grants(workspace_id, id)
    ON DELETE RESTRICT,
  CHECK (num_nonnulls(reversal_of_id, reversal_of_grant_id) <= 1)
);
--> statement-breakpoint
CREATE UNIQUE INDEX ai_credit_adjustments_reversal_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_adjustments (workspace_id, reversal_of_id)
  WHERE reversal_of_id IS NOT NULL;
--> statement-breakpoint
CREATE UNIQUE INDEX ai_credit_adjustments_grant_reversal_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_adjustments (workspace_id, reversal_of_grant_id)
  WHERE reversal_of_grant_id IS NOT NULL;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_reservations (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  member_user_id varchar NOT NULL,
  operation_type varchar(40) NOT NULL,
  provider varchar(40) NOT NULL,
  mode varchar(40) NOT NULL,
  model varchar(120),
  status varchar(24) NOT NULL
    CHECK (status IN ('reserved', 'claimed', 'settled', 'refunded', 'expired', 'failed')),
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  request_fingerprint varchar(64) CHECK (request_fingerprint IS NULL OR request_fingerprint ~ '^[0-9a-f]{64}$'),
  currency varchar(8) NOT NULL CHECK (currency IN ('CREDITS', 'USD')),
  credit_limit_period_start timestamptz,
  reserved_credits bigint NOT NULL DEFAULT 0 CHECK (reserved_credits >= 0),
  settled_credits bigint NOT NULL DEFAULT 0 CHECK (settled_credits >= 0),
  refunded_credits bigint NOT NULL DEFAULT 0 CHECK (refunded_credits >= 0),
  reserved_usd_micros bigint NOT NULL DEFAULT 0 CHECK (reserved_usd_micros >= 0),
  settled_usd_micros bigint NOT NULL DEFAULT 0 CHECK (settled_usd_micros >= 0),
  refunded_usd_micros bigint NOT NULL DEFAULT 0 CHECK (refunded_usd_micros >= 0),
  max_credits bigint CHECK (max_credits IS NULL OR max_credits >= 0),
  max_usd_micros bigint CHECK (max_usd_micros IS NULL OR max_usd_micros >= 0),
  unit_rate bigint NOT NULL DEFAULT 1 CHECK (unit_rate > 0),
  duration_seconds bigint CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
  connected_duration_ms bigint CHECK (connected_duration_ms IS NULL OR connected_duration_ms >= 0),
  policy_version integer NOT NULL REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_policy_versions(version) ON DELETE RESTRICT,
  rate_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rate_snapshot) = 'object'),
  expires_at timestamptz,
  provider_token_hash varchar(64) CHECK (provider_token_hash IS NULL OR provider_token_hash ~ '^[0-9a-f]{64}$'),
  provider_token_used_at timestamptz,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  started_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, member_user_id, id),
  UNIQUE (workspace_id, member_user_id, idempotency_key_hash),
  FOREIGN KEY (workspace_id, member_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, member_user_id, credit_limit_period_start)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_member_credit_limits(workspace_id, user_id, period_start)
    ON DELETE RESTRICT,
  CHECK (
    (currency = 'CREDITS' AND reserved_usd_micros = 0 AND settled_usd_micros = 0 AND refunded_usd_micros = 0)
    OR (currency = 'USD' AND reserved_credits = 0 AND settled_credits = 0 AND refunded_credits = 0)
  )
);
--> statement-breakpoint
CREATE INDEX ai_credit_reservations_member_status_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_reservations (workspace_id, member_user_id, status, created_at DESC);
--> statement-breakpoint
CREATE INDEX ai_credit_reservations_expiry_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_reservations (status, expires_at)
  WHERE status IN ('reserved', 'claimed');
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_reservation_allocations (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  member_user_id varchar NOT NULL,
  reservation_id text NOT NULL,
  grant_id text NOT NULL,
  reserved_credits bigint NOT NULL CHECK (reserved_credits > 0),
  settled_credits bigint NOT NULL DEFAULT 0 CHECK (settled_credits >= 0),
  refunded_credits bigint NOT NULL DEFAULT 0 CHECK (refunded_credits >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, member_user_id, reservation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_reservations(workspace_id, member_user_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, grant_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_grants(workspace_id, id)
    ON DELETE RESTRICT,
  UNIQUE (workspace_id, reservation_id, grant_id),
  CHECK (settled_credits + refunded_credits <= reserved_credits)
);
--> statement-breakpoint
CREATE INDEX ai_credit_reservation_allocations_grant_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_reservation_allocations
    (workspace_id, grant_id, created_at);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_credit_reservation_events (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  member_user_id varchar NOT NULL,
  reservation_id text NOT NULL,
  event_type varchar(24) NOT NULL,
  currency varchar(8) NOT NULL CHECK (currency IN ('CREDITS', 'USD')),
  credits bigint NOT NULL DEFAULT 0 CHECK (credits >= 0),
  amount_usd_micros bigint NOT NULL DEFAULT 0 CHECK (amount_usd_micros >= 0),
  duration_ms bigint CHECK (duration_ms IS NULL OR duration_ms >= 0),
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, member_user_id, reservation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_reservations(workspace_id, member_user_id, id)
    ON DELETE CASCADE,
  UNIQUE (workspace_id, reservation_id, idempotency_key_hash),
  CHECK ((currency = 'CREDITS' AND amount_usd_micros = 0) OR (currency = 'USD' AND credits = 0))
);
--> statement-breakpoint
CREATE INDEX ai_credit_reservation_events_timeline_idx
  ON @ASKOLO_V2_SCHEMA@.ai_credit_reservation_events
    (workspace_id, reservation_id, created_at, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.ai_provider_usage_evidence (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  member_user_id varchar NOT NULL,
  reservation_id text NOT NULL,
  provider varchar(40) NOT NULL,
  mode varchar(40) NOT NULL,
  region varchar(80),
  model varchar(120),
  duration_ms bigint CHECK (duration_ms IS NULL OR duration_ms >= 0),
  usage_unit varchar(32),
  usage_units bigint CHECK (usage_units IS NULL OR usage_units >= 0),
  input_tokens bigint CHECK (input_tokens IS NULL OR input_tokens >= 0),
  output_tokens bigint CHECK (output_tokens IS NULL OR output_tokens >= 0),
  provider_request_id varchar(200),
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  evidence_sha256 varchar(64) CHECK (evidence_sha256 IS NULL OR evidence_sha256 ~ '^[0-9a-f]{64}$'),
  recorded_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, member_user_id, reservation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_reservations(workspace_id, member_user_id, id)
    ON DELETE CASCADE,
  UNIQUE (workspace_id, reservation_id, idempotency_key_hash)
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.assistant_runs (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  conversation_id text NOT NULL,
  member_user_id varchar NOT NULL,
  reservation_id text,
  idempotency_key_hash varchar(64) NOT NULL CHECK (idempotency_key_hash ~ '^[0-9a-f]{64}$'),
  transcript_sha256 varchar(64) NOT NULL CHECK (transcript_sha256 ~ '^[0-9a-f]{64}$'),
  intent_sha256 varchar(64) CHECK (intent_sha256 IS NULL OR intent_sha256 ~ '^[0-9a-f]{64}$'),
  state text NOT NULL CHECK (state IN (
    'planning', 'needs_confirmation', 'executing', 'completed',
    'cancelled', 'failed', 'uncertain', 'rejected'
  )),
  currency varchar(8) NOT NULL DEFAULT 'CREDITS' CHECK (currency IN ('CREDITS', 'USD')),
  base_credits bigint NOT NULL DEFAULT 0 CHECK (base_credits >= 0),
  reserved_credits bigint NOT NULL DEFAULT 0 CHECK (reserved_credits >= 0),
  settled_credits bigint NOT NULL DEFAULT 0 CHECK (settled_credits >= 0),
  base_usd_micros bigint,
  reserved_usd_micros bigint,
  settled_usd_micros bigint NOT NULL DEFAULT 0 CHECK (settled_usd_micros >= 0),
  intent jsonb CHECK (intent IS NULL OR jsonb_typeof(intent) = 'object'),
  result jsonb CHECK (result IS NULL OR jsonb_typeof(result) = 'object'),
  risk_level text,
  requires_confirmation boolean NOT NULL DEFAULT false,
  confirmation_expires_at timestamptz,
  provider_started boolean NOT NULL DEFAULT false,
  provider_model varchar(120),
  provider_request_id varchar(200),
  input_tokens bigint CHECK (input_tokens IS NULL OR input_tokens >= 0),
  output_tokens bigint CHECK (output_tokens IS NULL OR output_tokens >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, id),
  UNIQUE (workspace_id, member_user_id, id),
  UNIQUE (workspace_id, member_user_id, idempotency_key_hash),
  FOREIGN KEY (workspace_id, conversation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.assistant_conversations(workspace_id, id)
    ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, member_user_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_memberships(workspace_id, user_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (workspace_id, member_user_id, reservation_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.ai_credit_reservations(workspace_id, member_user_id, id)
    ON DELETE RESTRICT,
  CHECK (
    (currency = 'CREDITS' AND base_credits > 0 AND reserved_credits > 0
      AND base_usd_micros IS NULL AND reserved_usd_micros IS NULL)
    OR
    (currency = 'USD' AND base_credits = 0 AND reserved_credits = 0
      AND COALESCE(base_usd_micros, 0) > 0 AND COALESCE(reserved_usd_micros, 0) > 0)
  )
);
--> statement-breakpoint
CREATE INDEX assistant_runs_workspace_created_idx
  ON @ASKOLO_V2_SCHEMA@.assistant_runs (workspace_id, created_at DESC);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.assistant_audit_events (
  id text PRIMARY KEY,
  workspace_id text NOT NULL,
  run_id text NOT NULL,
  member_user_id varchar NOT NULL,
  event_type text NOT NULL CHECK (event_type IN (
    'run_started', 'plan_ready', 'confirmation_accepted', 'confirmation_cancelled',
    'tool_started', 'tool_completed', 'result_verified', 'provider_failed',
    'run_cancelled', 'intent_rejected'
  )),
  transcript_sha256 varchar(64) CHECK (transcript_sha256 IS NULL OR transcript_sha256 ~ '^[0-9a-f]{64}$'),
  intent_sha256 varchar(64) CHECK (intent_sha256 IS NULL OR intent_sha256 ~ '^[0-9a-f]{64}$'),
  tool_name text,
  tool_args_sha256 varchar(64) CHECK (tool_args_sha256 IS NULL OR tool_args_sha256 ~ '^[0-9a-f]{64}$'),
  result_resource_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (workspace_id, member_user_id, run_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.assistant_runs(workspace_id, member_user_id, id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX assistant_audit_run_created_idx
  ON @ASKOLO_V2_SCHEMA@.assistant_audit_events (workspace_id, run_id, created_at, id);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.integration_connections (
  id text PRIMARY KEY,
  owner_user_id varchar NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider varchar(40) NOT NULL,
  account_label text,
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'revoked', 'error', 'disconnected')),
  granted_scopes text[] NOT NULL DEFAULT '{}',
  capabilities text[] NOT NULL DEFAULT '{}',
  availability_scope text NOT NULL DEFAULT 'private_dm'
    CHECK (availability_scope = 'private_dm'),
  -- Opaque reference only; credential material remains outside this schema.
  credential_ref_id text,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_user_id, id),
  CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);
--> statement-breakpoint
CREATE INDEX integration_connections_owner_provider_idx
  ON @ASKOLO_V2_SCHEMA@.integration_connections (owner_user_id, provider, status);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.user_integration_defaults (
  user_id varchar NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  service text NOT NULL,
  connection_id text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, service),
  FOREIGN KEY (user_id, connection_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.integration_connections(owner_user_id, id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.user_integration_sync_state (
  user_id varchar NOT NULL,
  connection_id text NOT NULL,
  service text NOT NULL,
  sync_cursor text,
  last_sync_at timestamptz,
  status text NOT NULL DEFAULT 'idle' CHECK (status IN ('idle', 'syncing', 'error', 'disabled')),
  last_error_code varchar(80),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, connection_id, service),
  FOREIGN KEY (user_id, connection_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.integration_connections(owner_user_id, id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX user_integration_sync_status_idx
  ON @ASKOLO_V2_SCHEMA@.user_integration_sync_state (user_id, status, last_sync_at);
--> statement-breakpoint
-- Read-only, workspace-scoped query surfaces. These views contain no message
-- text, credentials, or other broad personal content; callers must still
-- authorize a workspace before querying them.
CREATE VIEW @ASKOLO_V2_SCHEMA@.workspace_agenda_items AS
  SELECT workspace_id, 'plan_item'::text AS item_type, id AS item_id, title,
         start_at AS starts_at, NULL::date AS due_on, status, priority,
         assigned_to_user_id, updated_at
  FROM @ASKOLO_V2_SCHEMA@.daily_plan_items
  UNION ALL
  SELECT workspace_id, 'calendar_event'::text, id, title,
         starts_at, NULL::date, 'scheduled'::text, 'normal'::text,
         NULL::varchar, updated_at
  FROM @ASKOLO_V2_SCHEMA@.calendar_events
  UNION ALL
  SELECT workspace_id, 'chore'::text, id, title,
         NULL::timestamptz, due_on, status, 'normal'::text,
         assigned_to_user_id, updated_at
  FROM @ASKOLO_V2_SCHEMA@.chores
  UNION ALL
  SELECT workspace_id, 'action_item'::text, id, title,
         NULL::timestamptz, due_on, status, priority,
         assigned_to_user_id, updated_at
  FROM @ASKOLO_V2_SCHEMA@.action_items;
--> statement-breakpoint
CREATE VIEW @ASKOLO_V2_SCHEMA@.workspace_overview AS
  SELECT w.id AS workspace_id,
    (SELECT count(*) FROM @ASKOLO_V2_SCHEMA@.habits h
      WHERE h.workspace_id = w.id AND h.status = 'active') AS active_habits,
    (SELECT count(*) FROM @ASKOLO_V2_SCHEMA@.goals g
      WHERE g.workspace_id = w.id AND g.status = 'active') AS active_goals,
    (SELECT count(*) FROM @ASKOLO_V2_SCHEMA@.calendar_events e
      WHERE e.workspace_id = w.id AND e.starts_at >= now()) AS upcoming_events,
    (SELECT count(*) FROM @ASKOLO_V2_SCHEMA@.chores c
      WHERE c.workspace_id = w.id AND c.status IN ('open', 'in_progress')) AS open_chores,
    (SELECT count(*) FROM @ASKOLO_V2_SCHEMA@.action_items a
      WHERE a.workspace_id = w.id AND a.status IN ('open', 'in_progress')) AS open_action_items
  FROM @ASKOLO_V2_SCHEMA@.workspaces w
  WHERE w.status = 'active';
