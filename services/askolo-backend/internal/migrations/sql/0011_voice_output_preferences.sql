CREATE TABLE voice_output_preferences (
    user_id varchar PRIMARY KEY NOT NULL,
    consent_at timestamptz,
    consent_version varchar(32),
    updated_at timestamptz NOT NULL DEFAULT now()
);