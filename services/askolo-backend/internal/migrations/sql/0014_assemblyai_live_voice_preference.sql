ALTER TABLE voice_preferences
    ADD COLUMN live_agent_voice varchar(32) NOT NULL DEFAULT 'michael';

ALTER TABLE voice_preferences
    ADD CONSTRAINT voice_preferences_live_agent_voice_check
    CHECK (live_agent_voice IN (
        'michael', 'mary', 'paul', 'vera', 'giovanni',
        'lola', 'juergen', 'rafael', 'estelle'
    ));
