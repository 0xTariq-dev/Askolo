package postgres

import (
	"context"
	"fmt"
	"strings"

	"askolo/backend/internal/platform/id"
)

func (s *Store) UpsertV2GoogleCalendarEvents(ctx context.Context, workspaceID, actorUserID string, events []V2ProviderCalendarEvent) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	if len(events) > 2500 {
		return fmt.Errorf("provider calendar sync exceeds the supported page size")
	}
	for _, event := range events {
		if strings.TrimSpace(event.ProviderConnectionID) == "" ||
			strings.TrimSpace(event.ProviderCalendarID) == "" ||
			strings.TrimSpace(event.ProviderEventID) == "" {
			return fmt.Errorf("provider calendar event identity is incomplete")
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin provider calendar sync: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, event := range events {
		eventID, err := id.New()
		if err != nil {
			return fmt.Errorf("generate provider calendar event identifier: %w", err)
		}
		_, err = tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "calendar_events")+` (
 id, workspace_id, created_by_user_id, updated_by_user_id, title, description,
 starts_at, ends_at, all_day, time_zone, location, source_provider,
 source_connection_id, source_calendar_id, source_event_id
) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,$9,$10,'google',$11,$12,$13)
ON CONFLICT (workspace_id, source_provider, source_connection_id, source_calendar_id, source_event_id)
  WHERE source_provider IS NOT NULL
    AND source_connection_id IS NOT NULL AND source_calendar_id IS NOT NULL
DO UPDATE SET title=EXCLUDED.title, description=EXCLUDED.description,
  starts_at=EXCLUDED.starts_at, ends_at=EXCLUDED.ends_at,
  all_day=EXCLUDED.all_day, time_zone=EXCLUDED.time_zone,
  location=EXCLUDED.location, updated_by_user_id=EXCLUDED.updated_by_user_id,
  updated_at=now()`,
			eventID, workspaceID, actorUserID, event.Title, event.Description,
			event.StartsAt, event.EndsAt, event.AllDay, event.TimeZone,
			event.Location, event.ProviderConnectionID, event.ProviderCalendarID, event.ProviderEventID)
		if err != nil {
			return fmt.Errorf("upsert Google calendar event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit provider calendar sync: %w", err)
	}
	return nil
}

func (s *Store) DeleteV2GoogleCalendarEvent(ctx context.Context, workspaceID, connectionID, calendarID, providerEventID string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "calendar_events")+`
WHERE workspace_id=$1 AND source_provider='google' AND source_connection_id=$2
  AND source_calendar_id=$3 AND source_event_id=$4`,
		workspaceID, connectionID, calendarID, providerEventID)
	if err != nil {
		return fmt.Errorf("delete cancelled Google calendar event: %w", err)
	}
	return nil
}
