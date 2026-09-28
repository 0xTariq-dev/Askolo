-- Durable, user-scoped assistant conversations and guarded agent runs.
CREATE TABLE assistant_conversations (
  id text PRIMARY KEY NOT NULL,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workspace_id text NOT NULL,
  title text NOT NULL DEFAULT 'Assistant',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assistant_conversations_id_user_unique UNIQUE (id, user_id)
);
--> statement-breakpoint
CREATE INDEX assistant_conversations_user_updated_idx
  ON assistant_conversations (user_id, updated_at DESC);
--> statement-breakpoint
CREATE TABLE assistant_runs (
  id text PRIMARY KEY NOT NULL,
  conversation_id text NOT NULL,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idempotency_key_hash text NOT NULL,
  transcript text NOT NULL,
  transcript_sha256 text NOT NULL,
  state text NOT NULL,
  reservation_id text NOT NULL,
  base_credits integer NOT NULL CHECK (base_credits > 0),
  reserved_credits integer NOT NULL CHECK (reserved_credits > 0),
  settled_credits integer NOT NULL DEFAULT 0 CHECK (settled_credits >= 0),
  policy_version integer NOT NULL CHECK (policy_version > 0),
  provider_started boolean NOT NULL DEFAULT false,
  intent jsonb,
  intent_sha256 text,
  risk_level text,
  requires_confirmation boolean NOT NULL DEFAULT false,
  confirmation_expires_at timestamptz,
  result jsonb,
  assistant_message text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assistant_runs_state_check CHECK (
    state IN ('planning', 'needs_confirmation', 'completed', 'cancelled', 'failed', 'rejected')
  ),
  CONSTRAINT assistant_runs_transcript_length_check CHECK (
    octet_length(transcript) BETWEEN 1 AND 4096
  ),
  CONSTRAINT assistant_runs_transcript_hash_check CHECK (
    transcript_sha256 ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT assistant_runs_intent_hash_check CHECK (
    intent_sha256 IS NULL OR intent_sha256 ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT assistant_runs_idempotency_hash_check CHECK (
    idempotency_key_hash ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT assistant_runs_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT assistant_runs_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES assistant_conversations (id, user_id) ON DELETE CASCADE
);
--> statement-breakpoint
CREATE UNIQUE INDEX assistant_runs_user_idempotency_idx
  ON assistant_runs (user_id, idempotency_key_hash);
--> statement-breakpoint
CREATE INDEX assistant_runs_user_created_idx
  ON assistant_runs (user_id, created_at DESC);
--> statement-breakpoint
CREATE TABLE assistant_messages (
  id text PRIMARY KEY NOT NULL,
  conversation_id text NOT NULL,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  run_id text,
  role text NOT NULL CHECK (role IN ('user', 'assistant')),
  content text NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 4096),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assistant_messages_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES assistant_conversations (id, user_id) ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX assistant_messages_conversation_created_idx
  ON assistant_messages (conversation_id, created_at, id);
--> statement-breakpoint
CREATE TABLE assistant_audit_events (
  id text PRIMARY KEY NOT NULL,
  run_id text NOT NULL,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type text NOT NULL CHECK (
    event_type IN (
      'run_started', 'plan_ready', 'confirmation_accepted', 'confirmation_cancelled',
      'tool_started', 'tool_completed', 'result_verified', 'provider_failed',
      'run_cancelled', 'intent_rejected'
    )
  ),
  transcript_sha256 text,
  intent_sha256 text,
  tool_name text,
  tool_args_sha256 text,
  result_resource_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT assistant_audit_run_owner_fk
    FOREIGN KEY (run_id, user_id)
    REFERENCES assistant_runs (id, user_id) ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX assistant_audit_run_created_idx
  ON assistant_audit_events (run_id, created_at, id);
--> statement-breakpoint
-- Add a visible operation rate without rewriting administrator policy history.
INSERT INTO ai_credit_policy_versions (
  version, operation_weights, monthly_grant_credits, rollover_cap_credits,
  rollover_expiry_days, overrun_margin_percent, created_by, change_reason
)
SELECT
  current.version + 1,
  current.operation_weights || '{"assistant":1}'::jsonb,
  current.monthly_grant_credits,
  current.rollover_cap_credits,
  current.rollover_expiry_days,
  current.overrun_margin_percent,
  'system-migration',
  'Add assistant operation weight'
FROM ai_credit_policy_versions AS current
WHERE current.version = (SELECT MAX(version) FROM ai_credit_policy_versions)
  AND NOT (current.operation_weights ? 'assistant')
ON CONFLICT (version) DO NOTHING;