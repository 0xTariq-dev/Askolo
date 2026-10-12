package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

const v2CalendarEventColumns = `id, workspace_id, created_by_user_id, updated_by_user_id,
 title, description, starts_at, ends_at, all_day, time_zone, location, color,
 source_provider, source_connection_id, source_calendar_id, source_event_id, created_at, updated_at`

func scanV2CalendarEvent(row pgx.Row) (V2CalendarEventRecord, error) {
	var event V2CalendarEventRecord
	err := row.Scan(
		&event.ID, &event.WorkspaceID, &event.CreatedByUserID, &event.UpdatedByUserID,
		&event.Title, &event.Description, &event.StartsAt, &event.EndsAt, &event.AllDay,
		&event.TimeZone, &event.Location, &event.Color, &event.SourceProvider,
		&event.SourceConnectionID, &event.SourceCalendarID, &event.SourceEventID,
		&event.CreatedAt, &event.UpdatedAt,
	)
	return event, err
}

func (s *Store) ListV2CalendarEvents(ctx context.Context, workspaceID string, from, to time.Time, timeZone string, limit, offset int) ([]V2CalendarEventRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 || offset > 100000 {
		return nil, errors.New("calendar event offset is outside the supported range")
	}
	rows, err := s.pool.Query(ctx, `
SELECT `+v2CalendarEventColumns+`
FROM `+v2Table(schema, "calendar_events")+`
WHERE workspace_id=$1
  AND starts_at < (($3::date + 1)::timestamp AT TIME ZONE $4)
  AND (ends_at > ($2::date::timestamp AT TIME ZONE $4)
       OR (ends_at IS NULL AND starts_at >= ($2::date::timestamp AT TIME ZONE $4)))
ORDER BY starts_at, id LIMIT $5 OFFSET $6`,
		workspaceID, from.Format("2006-01-02"), to.Format("2006-01-02"), timeZone, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list versioned calendar events: %w", err)
	}
	defer rows.Close()
	events := make([]V2CalendarEventRecord, 0)
	for rows.Next() {
		event, scanErr := scanV2CalendarEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan versioned calendar event: %w", scanErr)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) CreateV2CalendarEvent(ctx context.Context, workspaceID, actorUserID string, input V2CalendarEventWrite) (V2CalendarEventRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2CalendarEventRecord{}, err
	}
	eventID, err := id.New()
	if err != nil {
		return V2CalendarEventRecord{}, fmt.Errorf("generate calendar event identifier: %w", err)
	}
	return scanV2CalendarEvent(s.pool.QueryRow(ctx, `
INSERT INTO `+v2Table(schema, "calendar_events")+` (
 id, workspace_id, created_by_user_id, updated_by_user_id, title, description,
 starts_at, ends_at, all_day, time_zone, location, color
) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING `+v2CalendarEventColumns,
		eventID, workspaceID, actorUserID, input.Title, input.Description,
		input.StartsAt, input.EndsAt, input.AllDay, input.TimeZone, input.Location, input.Color))
}

func (s *Store) UpdateV2CalendarEvent(ctx context.Context, workspaceID, eventID, actorUserID, role string, input V2CalendarEventWrite) (V2CalendarEventRecord, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return V2CalendarEventRecord{}, err
	}
	event, err := scanV2CalendarEvent(s.pool.QueryRow(ctx, `
UPDATE `+v2Table(schema, "calendar_events")+`
SET updated_by_user_id=$3, title=$5, description=$6, starts_at=$7, ends_at=$8,
    all_day=$9, time_zone=$10, location=$11, color=$12, updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND source_provider IS NULL
  AND (created_by_user_id=$3 OR $4 IN ('owner','admin'))
RETURNING `+v2CalendarEventColumns,
		workspaceID, eventID, actorUserID, role, input.Title, input.Description,
		input.StartsAt, input.EndsAt, input.AllDay, input.TimeZone, input.Location, input.Color))
	if errors.Is(err, pgx.ErrNoRows) {
		return V2CalendarEventRecord{}, ErrNotFound
	}
	if err != nil {
		return V2CalendarEventRecord{}, fmt.Errorf("update versioned calendar event: %w", err)
	}
	return event, nil
}

func (s *Store) DeleteV2CalendarEvent(ctx context.Context, workspaceID, eventID, actorUserID, role string) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
DELETE FROM `+v2Table(schema, "calendar_events")+`
WHERE workspace_id=$1 AND id=$2 AND source_provider IS NULL
  AND (created_by_user_id=$3 OR $4 IN ('owner','admin'))`,
		workspaceID, eventID, actorUserID, role)
	if err != nil {
		return fmt.Errorf("delete versioned calendar event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
