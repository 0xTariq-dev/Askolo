package postgres

import (
	"context"
	"fmt"
)

type V2NotificationPreference struct {
	EventKey     string `json:"eventKey"`
	EmailEnabled bool   `json:"emailEnabled"`
	InAppEnabled bool   `json:"inAppEnabled"`
}

func (s *Store) ListV2NotificationPreferences(
	ctx context.Context, workspaceID, userID, role string,
) ([]V2NotificationPreference, error) {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return nil, err
	}
	defaults := []V2NotificationPreference{
		{EventKey: "assignments", InAppEnabled: true},
		{EventKey: "reminders", InAppEnabled: true},
	}
	if role == "owner" || role == "admin" {
		defaults = append(defaults, V2NotificationPreference{EventKey: "membership", InAppEnabled: true})
	}
	values := make(map[string]V2NotificationPreference, len(defaults))
	for _, preference := range defaults {
		values[preference.EventKey] = preference
	}
	rows, err := s.pool.Query(ctx, `
SELECT event_key, channel, enabled
FROM `+v2Table(schema, "workspace_notification_preferences")+`
WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("read Space notification preferences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventKey, channel string
		var enabled bool
		if err := rows.Scan(&eventKey, &channel, &enabled); err != nil {
			return nil, fmt.Errorf("scan Space notification preference: %w", err)
		}
		preference, exists := values[eventKey]
		if !exists {
			continue
		}
		switch channel {
		case "email":
			preference.EmailEnabled = enabled
		case "in_app":
			preference.InAppEnabled = enabled
		}
		values[eventKey] = preference
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Space notification preferences: %w", err)
	}
	result := make([]V2NotificationPreference, 0, len(values))
	for _, preference := range defaults {
		result = append(result, values[preference.EventKey])
	}
	return result, nil
}

func (s *Store) ReplaceV2NotificationPreferences(
	ctx context.Context, workspaceID, userID string, preferences []V2NotificationPreference,
) error {
	schema, err := s.v2Schema(ctx)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification preference update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := requireActiveV2WorkspaceMember(ctx, tx, workspaceID, userID); err != nil {
		return err
	}
	for _, preference := range preferences {
		for _, channel := range []struct {
			name    string
			enabled bool
		}{{"email", preference.EmailEnabled}, {"in_app", preference.InAppEnabled}} {
			if _, err := tx.Exec(ctx, `
INSERT INTO `+v2Table(schema, "workspace_notification_preferences")+`
  (workspace_id, user_id, channel, event_key, enabled, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (workspace_id, user_id, channel, event_key)
DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()`,
				workspaceID, userID, channel.name, preference.EventKey, channel.enabled,
			); err != nil {
				return fmt.Errorf("save Space notification preference: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification preference update: %w", err)
	}
	return nil
}
