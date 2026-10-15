package postgres

import (
	"encoding/json"
	"time"
)

type V2Task struct {
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspaceId"`
	CreatedByUserID   string          `json:"createdByUserId"`
	AssignedToUserID  *string         `json:"assignedToUserId"`
	AssignedToGroupID *string         `json:"assignedToGroupId"`
	Kind              string          `json:"kind"`
	Title             string          `json:"title"`
	Details           *string         `json:"details"`
	Status            string          `json:"status"`
	Priority          string          `json:"priority"`
	DueOn             *time.Time      `json:"dueOn"`
	PlannedStartAt    *time.Time      `json:"plannedStartAt"`
	PlannedOn         *time.Time      `json:"plannedOn"`
	PlannedStartTime  *string         `json:"plannedStartTime"`
	DurationMinutes   *int            `json:"durationMinutes"`
	DueTime           *string         `json:"dueTime"`
	TimeZone          string          `json:"timeZone"`
	Label             *string         `json:"label"`
	GoalID            *string         `json:"goalId"`
	ParentTaskID      *string         `json:"parentTaskId"`
	Recurrence        json.RawMessage `json:"recurrence"`
	SourceType        *string         `json:"sourceType"`
	SourceReference   *string         `json:"sourceReference"`
	CompletedAt       *time.Time      `json:"completedAt"`
	ArchivedAt        *time.Time      `json:"archivedAt"`
	Dependencies      []string        `json:"dependencies"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type V2TaskWrite struct {
	AssignedToUserID  *string
	AssignedToGroupID *string
	Kind              string
	Title             string
	Details           *string
	Status            string
	Priority          string
	DueOn             *time.Time
	PlannedStartAt    *time.Time
	PlannedOn         *time.Time
	PlannedStartTime  *string
	DurationMinutes   *int
	DueTime           *string
	TimeZone          string
	Label             *string
	GoalID            *string
	ParentTaskID      *string
	Recurrence        json.RawMessage
	SourceType        *string
	SourceReference   *string
	Dependencies      []string
}

type V2TaskFilter struct {
	Kind       string
	Status     string
	AssigneeID string
	From       *time.Time
	To         *time.Time
	TimeZone   string
	Limit      int
	Offset     int
}

type V2TaskOccurrence struct {
	TaskID            string                  `json:"taskId"`
	OccurrenceOn      time.Time               `json:"occurrenceOn"`
	Status            string                  `json:"status"`
	ResolvedByUserID  *string                 `json:"resolvedByUserId"`
	CompletedByUserID *string                 `json:"completedByUserId"`
	CompletedAt       *time.Time              `json:"completedAt"`
	DueAtOverride     *time.Time              `json:"dueAtOverride"`
	History           []V2TaskOccurrenceEvent `json:"history"`
}

type V2TaskOccurrenceEvent struct {
	FromStatus      *string   `json:"fromStatus"`
	ToStatus        string    `json:"toStatus"`
	ChangedByUserID string    `json:"changedByUserId"`
	ChangedAt       time.Time `json:"changedAt"`
}

type V2GoalRecord struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspaceId"`
	CreatedByUserID string     `json:"createdByUserId"`
	Title           string     `json:"title"`
	Description     *string    `json:"description"`
	TargetOn        *time.Time `json:"targetOn"`
	Status          string     `json:"status"`
	ProgressPercent int16      `json:"progressPercent"`
	Category        *string    `json:"category"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type V2GoalWrite struct {
	Title           string
	Description     *string
	TargetOn        *time.Time
	Status          string
	ProgressPercent int16
	Category        *string
}

type V2Plan struct {
	ID              string        `json:"id"`
	WorkspaceID     string        `json:"workspaceId"`
	OwnerUserID     string        `json:"ownerUserId"`
	Title           string        `json:"title"`
	Details         *string       `json:"details"`
	StartsOn        time.Time     `json:"startsOn"`
	EndsOn          time.Time     `json:"endsOn"`
	TimeZone        string        `json:"timeZone"`
	Visibility      string        `json:"visibility"`
	Status          string        `json:"status"`
	UpdatedByUserID string        `json:"updatedByUserId"`
	Entries         []V2PlanEntry `json:"entries"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type V2PlanWrite struct {
	Title      string
	Details    *string
	StartsOn   time.Time
	EndsOn     time.Time
	TimeZone   string
	Visibility string
	Status     string
}

type V2PlanEntry struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspaceId"`
	WorkspaceName    string     `json:"workspaceName,omitempty"`
	PlanID           string     `json:"planId"`
	TaskID           string     `json:"taskId"`
	OccurrenceOn     *time.Time `json:"occurrenceOn"`
	PlannedOn        time.Time  `json:"plannedOn"`
	PlannedStartAt   *time.Time `json:"plannedStartAt"`
	PlannedEndAt     *time.Time `json:"plannedEndAt"`
	Position         int        `json:"position"`
	CreatedByUserID  string     `json:"createdByUserId"`
	AssignedToUserID *string    `json:"assignedToUserId"`
	TaskTitle        string     `json:"taskTitle"`
	TaskStatus       string     `json:"taskStatus"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type V2PlanEntryWrite struct {
	TaskID           string
	OccurrenceOn     *time.Time
	PlannedOn        time.Time
	PlannedStartAt   *time.Time
	PlannedEndAt     *time.Time
	Position         int
	AssignedToUserID *string
}

type V2ReminderRule struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	OwnerUserID string          `json:"ownerUserId"`
	Kind        string          `json:"kind"`
	Rule        json.RawMessage `json:"rule"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type V2CalendarEventAgendaItem struct {
	Kind               string     `json:"kind"`
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspaceId,omitempty"`
	WorkspaceName      string     `json:"workspaceName,omitempty"`
	Title              string     `json:"title"`
	Description        *string    `json:"description"`
	StartsAt           time.Time  `json:"startsAt"`
	EndsAt             *time.Time `json:"endsAt"`
	AllDay             bool       `json:"allDay"`
	TimeZone           string     `json:"timeZone"`
	Location           *string    `json:"location"`
	Color              *string    `json:"color"`
	SourceProvider     *string    `json:"sourceProvider"`
	SourceConnectionID *string    `json:"sourceConnectionId"`
	SourceCalendarID   *string    `json:"sourceCalendarId"`
	SourceEventID      *string    `json:"sourceEventId"`
	SourceEventETag    *string    `json:"sourceEventEtag"`
}

type V2CalendarEventRecord struct {
	ID                 string     `json:"id"`
	WorkspaceID        string     `json:"workspaceId"`
	CreatedByUserID    string     `json:"createdByUserId"`
	UpdatedByUserID    string     `json:"updatedByUserId"`
	Title              string     `json:"title"`
	Description        *string    `json:"description"`
	StartsAt           time.Time  `json:"startsAt"`
	EndsAt             *time.Time `json:"endsAt"`
	AllDay             bool       `json:"allDay"`
	TimeZone           string     `json:"timeZone"`
	Location           *string    `json:"location"`
	Color              *string    `json:"color"`
	SourceProvider     *string    `json:"sourceProvider"`
	SourceConnectionID *string    `json:"sourceConnectionId"`
	SourceCalendarID   *string    `json:"sourceCalendarId"`
	SourceEventID      *string    `json:"sourceEventId"`
	ProviderETag       *string    `json:"providerEtag"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type V2CalendarEventWrite struct {
	Title       string
	Description *string
	StartsAt    time.Time
	EndsAt      *time.Time
	AllDay      bool
	TimeZone    string
	Location    *string
	Color       *string
}

type V2ProviderCalendarEvent struct {
	ProviderConnectionID string
	ProviderCalendarID   string
	ProviderEventID      string
	ProviderETag         string
	Title                string
	Description          *string
	StartsAt             time.Time
	EndsAt               *time.Time
	AllDay               bool
	TimeZone             string
	Location             *string
}

type V2TaskAgendaItem struct {
	Kind             string     `json:"kind"`
	TaskID           string     `json:"taskId"`
	WorkspaceID      string     `json:"workspaceId,omitempty"`
	WorkspaceName    string     `json:"workspaceName,omitempty"`
	OccurrenceOn     *time.Time `json:"occurrenceOn"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DueOn            *time.Time `json:"dueOn"`
	PlannedStartAt   *time.Time `json:"plannedStartAt"`
	PlannedOn        *time.Time `json:"plannedOn"`
	PlannedStartTime *string    `json:"plannedStartTime"`
	DurationMinutes  *int       `json:"durationMinutes"`
	TimeZone         string     `json:"timeZone"`
	Label            *string    `json:"label"`
	AssignedToUserID *string    `json:"assignedToUserId"`
	PlanID           *string    `json:"planId"`
}

type V2Agenda struct {
	WorkspaceID    string                      `json:"workspaceId"`
	From           string                      `json:"from"`
	To             string                      `json:"to"`
	TimeZone       string                      `json:"timeZone"`
	Tasks          []V2TaskAgendaItem          `json:"tasks"`
	PlanEntries    []V2PlanEntry               `json:"planEntries"`
	CalendarEvents []V2CalendarEventAgendaItem `json:"calendarEvents"`
	NextPageToken  string                      `json:"nextPageToken,omitempty"`
}
