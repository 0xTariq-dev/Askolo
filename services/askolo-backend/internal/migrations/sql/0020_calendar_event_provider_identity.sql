ALTER TABLE @ASKOLO_V2_SCHEMA@.calendar_events
  ADD COLUMN source_connection_id text,
  ADD COLUMN source_calendar_id text;
--> statement-breakpoint
DROP INDEX @ASKOLO_V2_SCHEMA@.calendar_events_external_unique_idx;
--> statement-breakpoint
CREATE UNIQUE INDEX calendar_events_external_unique_idx
  ON @ASKOLO_V2_SCHEMA@.calendar_events
    (workspace_id, source_provider, source_connection_id, source_calendar_id, source_event_id)
  WHERE source_provider IS NOT NULL
    AND source_connection_id IS NOT NULL
    AND source_calendar_id IS NOT NULL;
--> statement-breakpoint
CREATE UNIQUE INDEX calendar_events_external_legacy_unique_idx
  ON @ASKOLO_V2_SCHEMA@.calendar_events (workspace_id, source_provider, source_event_id)
  WHERE source_provider IS NOT NULL
    AND (source_connection_id IS NULL OR source_calendar_id IS NULL);
