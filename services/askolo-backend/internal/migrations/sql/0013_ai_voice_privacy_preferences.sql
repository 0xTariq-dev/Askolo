ALTER TABLE voice_preferences
    ADD COLUMN assistant_processing_consent_at timestamptz,
    ADD COLUMN assistant_processing_consent_version varchar(32),
    ADD COLUMN live_agent_consent_at timestamptz,
    ADD COLUMN live_agent_consent_version varchar(32),
    ADD COLUMN redaction_location varchar(16);

ALTER TABLE voice_preferences
    ADD CONSTRAINT voice_preferences_redaction_location_check
    CHECK (redaction_location IS NULL OR redaction_location = 'app');

ALTER TABLE voice_output_preferences
    ADD COLUMN auto_speak_enabled boolean NOT NULL DEFAULT false;
