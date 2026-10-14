ALTER TABLE @ASKOLO_V2_SCHEMA@.workspace_email_summaries
  ADD COLUMN gmail_history_id text,
  ADD COLUMN baseline_history_id text,
  ADD COLUMN baseline_cutoff_at timestamptz,
  ADD COLUMN baseline_phase text NOT NULL DEFAULT 'complete'
    CHECK (baseline_phase IN ('inbox', 'history', 'complete')),
  ADD COLUMN baseline_page_token text,
  ADD COLUMN history_page_token text,
  ADD COLUMN thread_state_key_version smallint,
  ADD COLUMN thread_state_key_fingerprint char(64),
  ADD COLUMN sync_action_required boolean NOT NULL DEFAULT false;
--> statement-breakpoint
CREATE TABLE @ASKOLO_V2_SCHEMA@.workspace_email_summary_thread_state (
  workspace_id text NOT NULL,
  provider_connection_id text NOT NULL,
  thread_token char(64) NOT NULL,
  unread_message_count integer NOT NULL DEFAULT 0
    CHECK (unread_message_count >= 0),
  in_inbox_window boolean NOT NULL DEFAULT false,
  latest_sender_is_owner boolean NOT NULL DEFAULT false,
  flagged_in_window boolean NOT NULL DEFAULT false,
  last_activity_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, provider_connection_id, thread_token),
  FOREIGN KEY (workspace_id, provider_connection_id)
    REFERENCES @ASKOLO_V2_SCHEMA@.workspace_email_summaries
      (workspace_id, provider_connection_id)
    ON DELETE CASCADE
);
--> statement-breakpoint
CREATE INDEX workspace_email_summary_thread_state_activity_idx
  ON @ASKOLO_V2_SCHEMA@.workspace_email_summary_thread_state
    (workspace_id, provider_connection_id, last_activity_at);
