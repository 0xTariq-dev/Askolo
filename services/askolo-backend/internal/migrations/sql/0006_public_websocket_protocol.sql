-- Durable metadata for the shared public assistant/voice WebSocket.
-- Deliberately contains no transcript, audio, or provider response fields.
CREATE TABLE public_ws_sessions (
  id text PRIMARY KEY NOT NULL,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workspace_id text NOT NULL,
  status text NOT NULL DEFAULT 'active',
  lease_until timestamptz NOT NULL,
  client_sequence bigint NOT NULL DEFAULT 0 CHECK (client_sequence >= 0),
  server_sequence bigint NOT NULL DEFAULT 0 CHECK (server_sequence >= 0),
  terminal_state text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz,
  CONSTRAINT public_ws_sessions_status_check CHECK (status IN ('active','closed','expired')),
  CONSTRAINT public_ws_sessions_terminal_check CHECK (
    terminal_state IS NULL OR terminal_state IN ('completed','cancelled','failed','expired')
  ),
  CONSTRAINT public_ws_sessions_id_user_unique UNIQUE (id,user_id)
);
CREATE INDEX public_ws_sessions_user_active_idx
  ON public_ws_sessions(user_id, lease_until) WHERE status='active';

CREATE TABLE public_ws_client_sequences (
  session_id text NOT NULL REFERENCES public_ws_sessions(id) ON DELETE CASCADE,
  sequence bigint NOT NULL CHECK (sequence > 0),
  message_hash text NOT NULL CHECK (message_hash ~ '^[0-9a-f]{64}$'),
  correlation_id text NOT NULL DEFAULT '',
  message_type text NOT NULL,
  accepted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (session_id, sequence)
);

CREATE TABLE public_ws_server_events (
  session_id text NOT NULL REFERENCES public_ws_sessions(id) ON DELETE CASCADE,
  sequence bigint NOT NULL CHECK (sequence > 0),
  correlation_id text NOT NULL DEFAULT '',
  event_type text NOT NULL,
  event_hash text NOT NULL CHECK (event_hash ~ '^[0-9a-f]{64}$'),
  assistant_run_id text,
  voice_reservation_id text,
  terminal_state text,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (session_id, sequence)
);
CREATE INDEX public_ws_server_events_resume_idx
  ON public_ws_server_events(session_id, sequence);