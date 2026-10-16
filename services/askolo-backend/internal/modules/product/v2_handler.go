package product

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	v2RequestTimeout     = 4 * time.Second
	v2MaxBodyBytes       = 64 * 1024
	v2MaxDateRange       = 31
	v2MaxAgendaDateRange = 42
)

var v2IdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var v2WorkspaceIdentifierPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_-]{1,128}|personal:[A-Za-z0-9_-]{1,128})$`)
var v2SpaceSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type v2ActorHandler func(http.ResponseWriter, *http.Request, string, string)
type v2AllActorHandler func(http.ResponseWriter, *http.Request, string)

type v2WorkspaceSettingsRequest struct {
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	ImageObjectPath *string `json:"imageObjectPath"`
	TimeZone        string  `json:"timeZone"`
	Slug            *string `json:"slug"`
}

type v2PatchField[T any] struct {
	Set   bool
	Null  bool
	Value T
}

func (field *v2PatchField[T]) UnmarshalJSON(data []byte) error {
	field.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		field.Null = true
		return nil
	}
	return json.Unmarshal(data, &field.Value)
}

type v2RecurrenceRule struct {
	Frequency string  `json:"frequency"`
	Interval  int     `json:"interval"`
	Weekdays  []int   `json:"weekdays,omitempty"`
	StartsOn  string  `json:"startsOn"`
	EndsOn    *string `json:"endsOn"`
	Count     *int    `json:"count,omitempty"`
	TimeOfDay string  `json:"timeOfDay,omitempty"`
	TimeZone  string  `json:"timeZone"`
}

type v2TaskRequest struct {
	AssignedToUserID  *string           `json:"assignedToUserId"`
	AssignedToGroupID *string           `json:"assignedToGroupId"`
	Kind              string            `json:"kind"`
	Title             string            `json:"title"`
	Details           *string           `json:"details"`
	Status            string            `json:"status"`
	Priority          string            `json:"priority"`
	DueOn             *string           `json:"dueOn"`
	PlannedStartAt    *time.Time        `json:"plannedStartAt"`
	PlannedOn         *string           `json:"plannedOn"`
	PlannedStartTime  *string           `json:"plannedStartTime"`
	DurationMinutes   *int              `json:"durationMinutes"`
	DueTime           *string           `json:"dueTime"`
	TimeZone          string            `json:"timeZone"`
	Label             *string           `json:"label"`
	GoalID            *string           `json:"goalId"`
	ParentTaskID      *string           `json:"parentTaskId"`
	Recurrence        *v2RecurrenceRule `json:"recurrence"`
	SourceType        *string           `json:"sourceType"`
	SourceReference   *string           `json:"sourceReference"`
	Dependencies      []string          `json:"dependencies"`
}

type v2TaskPatchRequest struct {
	AssignedToUserID  v2PatchField[string]           `json:"assignedToUserId"`
	AssignedToGroupID v2PatchField[string]           `json:"assignedToGroupId"`
	Kind              v2PatchField[string]           `json:"kind"`
	Title             v2PatchField[string]           `json:"title"`
	Details           v2PatchField[string]           `json:"details"`
	Status            v2PatchField[string]           `json:"status"`
	Priority          v2PatchField[string]           `json:"priority"`
	DueOn             v2PatchField[string]           `json:"dueOn"`
	PlannedStartAt    v2PatchField[time.Time]        `json:"plannedStartAt"`
	PlannedOn         v2PatchField[string]           `json:"plannedOn"`
	PlannedStartTime  v2PatchField[string]           `json:"plannedStartTime"`
	DurationMinutes   v2PatchField[int]              `json:"durationMinutes"`
	DueTime           v2PatchField[string]           `json:"dueTime"`
	TimeZone          v2PatchField[string]           `json:"timeZone"`
	Label             v2PatchField[string]           `json:"label"`
	GoalID            v2PatchField[string]           `json:"goalId"`
	ParentTaskID      v2PatchField[string]           `json:"parentTaskId"`
	Recurrence        v2PatchField[v2RecurrenceRule] `json:"recurrence"`
	SourceType        v2PatchField[string]           `json:"sourceType"`
	SourceReference   v2PatchField[string]           `json:"sourceReference"`
	Dependencies      v2PatchField[[]string]         `json:"dependencies"`
}

type v2OccurrencePatchRequest struct {
	Status        v2PatchField[string]    `json:"status"`
	DueAtOverride v2PatchField[time.Time] `json:"dueAtOverride"`
}

func (h *Handler) mountV2(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/healthz", h.v2Health)
	mux.HandleFunc("GET /api/v2/notifications", h.v2ProtectAll(h.v2ListNotifications))
	mux.HandleFunc("POST /api/v2/notifications/read-all", h.v2ProtectAll(h.v2MarkAllNotificationsRead))
	mux.HandleFunc("DELETE /api/v2/notifications", h.v2ProtectAll(h.v2ClearNotifications))
	mux.HandleFunc("POST /api/v2/notifications/{notificationId}/read", h.v2ProtectAll(h.v2MarkNotificationRead))
	mux.HandleFunc("GET /api/v2/workspaces", h.v2ProtectAll(h.v2ListWorkspaces))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}", h.v2Protect(false, h.v2GetWorkspaceSettings))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}", h.v2Protect(true, h.v2UpdateWorkspaceSettings))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/notification-preferences", h.v2Protect(false, h.v2GetNotificationPreferences))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/notification-preferences", h.v2Protect(true, h.v2UpdateNotificationPreferences))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/members", h.v2Protect(false, h.v2ListSpaceMembers))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/members/{userId}/role", h.v2Protect(true, h.v2UpdateSpaceMemberRole))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/members/{userId}", h.v2Protect(true, h.v2RemoveSpaceMember))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/membership", h.v2Protect(true, h.v2LeaveWorkspace))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}", h.v2Protect(true, h.v2DeleteWorkspace))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/groups", h.v2Protect(false, h.v2ListWorkspaceGroups))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/groups", h.v2Protect(true, h.v2CreateWorkspaceGroup))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/groups/{groupId}", h.v2Protect(true, h.v2UpdateWorkspaceGroup))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/groups/{groupId}", h.v2Protect(true, h.v2DeleteWorkspaceGroup))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/invitations", h.v2Protect(false, h.v2ListSpaceInvitations))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/invitations", h.v2Protect(true, h.v2CreateSpaceInvitation))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/invitations/{invitationId}", h.v2Protect(true, h.v2CancelSpaceInvitation))
	mux.HandleFunc("POST /api/v2/invitations/accept", h.v2ProtectAll(h.v2AcceptSpaceInvitation))
	mux.HandleFunc("POST /api/v2/invitations/decline", h.v2ProtectAll(h.v2DeclineSpaceInvitation))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/tasks", h.v2Protect(false, h.v2ListTasks))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/tasks", h.v2Protect(true, h.v2CreateTask))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}", h.v2Protect(false, h.v2GetTask))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/tasks/{taskId}", h.v2Protect(true, h.v2UpdateTask))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/tasks/{taskId}", h.v2Protect(true, h.v2ArchiveTask))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/tasks/{taskId}/occurrences", h.v2Protect(false, h.v2ListTaskOccurrences))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/tasks/{taskId}/occurrences/{date}", h.v2Protect(true, h.v2UpdateTaskOccurrence))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/goals", h.v2Protect(false, h.v2ListGoals))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/goals", h.v2Protect(true, h.v2CreateGoal))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/goals/{goalId}", h.v2Protect(false, h.v2GetGoal))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/goals/{goalId}", h.v2Protect(true, h.v2UpdateGoal))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/goals/{goalId}", h.v2Protect(true, h.v2ArchiveGoal))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/plans", h.v2Protect(false, h.v2ListPlans))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/plans", h.v2Protect(true, h.v2CreatePlan))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/plans/{planId}", h.v2Protect(false, h.v2GetPlan))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/plans/{planId}", h.v2Protect(true, h.v2UpdatePlan))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/plans/{planId}", h.v2Protect(true, h.v2ArchivePlan))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/plans/{planId}/entries", h.v2Protect(true, h.v2CreatePlanEntry))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/plans/{planId}/entries/{entryId}", h.v2Protect(true, h.v2UpdatePlanEntry))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/plans/{planId}/entries/{entryId}", h.v2Protect(true, h.v2DeletePlanEntry))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/agenda", h.v2Protect(false, h.v2GetAgenda))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/calendar-events", h.v2Protect(false, h.v2ListCalendarEvents))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/calendar-events", h.v2Protect(true, h.v2CreateCalendarEvent))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/calendar-events/{eventId}", h.v2Protect(true, h.v2UpdateCalendarEvent))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/calendar-events/{eventId}", h.v2Protect(true, h.v2DeleteCalendarEvent))
	mux.HandleFunc("GET /api/v2/agenda", h.v2ProtectAll(h.v2GetAllSpacesAgenda))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/reminder-rules", h.v2Protect(false, h.v2ListReminderRules))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/reminder-rules", h.v2Protect(true, h.v2CreateReminderRule))
	mux.HandleFunc("PATCH /api/v2/workspaces/{workspaceId}/reminder-rules/{ruleId}", h.v2Protect(true, h.v2UpdateReminderRule))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/reminder-rules/{ruleId}", h.v2Protect(true, h.v2DeleteReminderRule))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/home", h.v2Protect(false, h.v2GetWorkspaceHome))
	mux.HandleFunc("GET /api/v2/home", h.v2ProtectAll(h.v2GetAllSpacesHome))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/home/focus", h.v2Protect(true, h.v2ReplaceHomeFocus))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/provider-connections", h.v2Protect(false, h.v2ListHomeProviderConnections))
	mux.HandleFunc("POST /api/v2/workspaces/{workspaceId}/provider-connections", h.v2Protect(true, h.v2AssociateHomeProviderConnection))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}", h.v2Protect(true, h.v2RemoveHomeProviderConnection))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/space-access", h.v2Protect(true, h.v2SetHomeProviderAccess))
	mux.HandleFunc("PUT /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/email-read-consent", h.v2Protect(true, h.v2SetHomeEmailReadConsent))
	mux.HandleFunc("DELETE /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/email-read-consent", h.v2Protect(true, h.v2RevokeHomeEmailReadConsent))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/messages", h.v2Protect(false, h.v2ListSharedEmailMessages))
	mux.HandleFunc("GET /api/v2/workspaces/{workspaceId}/provider-connections/{connectionId}/messages/{messageId}", h.v2Protect(false, h.v2GetSharedEmailMessage))
}

func (h *Handler) v2ListNotifications(w http.ResponseWriter, r *http.Request, userID string) {
	notifications, err := h.store.ListV2Notifications(r.Context(), userID)
	if err != nil {
		h.v2InternalError(w, r, userID, "list notifications", err)
		return
	}
	writeJSON(w, http.StatusOK, notifications)
}

func (h *Handler) v2MarkNotificationRead(w http.ResponseWriter, r *http.Request, userID string) {
	notificationID := r.PathValue("notificationId")
	if !validV2Identifier(notificationID) {
		writeError(w, http.StatusBadRequest, "INVALID_NOTIFICATION_ID", "A valid notification is required.")
		return
	}
	if err := h.store.MarkV2NotificationRead(r.Context(), userID, notificationID); errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Notification not found.")
		return
	} else if err != nil {
		h.v2InternalError(w, r, userID, "mark notification read", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request, userID string) {
	if err := h.store.MarkAllV2NotificationsRead(r.Context(), userID); err != nil {
		h.v2InternalError(w, r, userID, "mark notifications read", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2ClearNotifications(w http.ResponseWriter, r *http.Request, userID string) {
	if err := h.store.ClearV2Notifications(r.Context(), userID); err != nil {
		h.v2InternalError(w, r, userID, "clear notifications", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2GetWorkspaceSettings(w http.ResponseWriter, r *http.Request, userID, _ string) {
	workspace, err := h.store.GetV2HomeWorkspace(r.Context(), r.PathValue("workspaceId"), userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "read Space settings", err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) v2UpdateWorkspaceSettings(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can change these settings.")
		return
	}
	var input v2WorkspaceSettingsRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 120 {
		writeError(w, http.StatusBadRequest, "INVALID_NAME", "Space name must contain 1 to 120 characters.")
		return
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if len(description) > 2000 {
			writeError(w, http.StatusBadRequest, "INVALID_DESCRIPTION", "Space description must be at most 2,000 characters.")
			return
		}
		if description == "" {
			input.Description = nil
		} else {
			input.Description = &description
		}
	}
	if input.ImageObjectPath != nil {
		objectPath := strings.TrimSpace(*input.ImageObjectPath)
		if objectPath == "" {
			input.ImageObjectPath = nil
		} else if len(objectPath) > 500 || !strings.HasPrefix(objectPath, "/objects/") ||
			strings.Contains(objectPath, "..") || strings.ContainsAny(objectPath, "?#\r\n") {
			writeError(w, http.StatusBadRequest, "INVALID_IMAGE_PATH", "Space image must use a valid App Storage object path.")
			return
		} else {
			input.ImageObjectPath = &objectPath
		}
	}
	input.TimeZone = strings.TrimSpace(input.TimeZone)
	if input.TimeZone == "" {
		input.TimeZone = "UTC"
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TIME_ZONE", "Choose a valid time zone.")
		return
	}
	if input.Slug != nil {
		slug := strings.ToLower(strings.TrimSpace(*input.Slug))
		if slug == "" {
			input.Slug = nil
		} else if len(slug) > 63 || !v2SpaceSlugPattern.MatchString(slug) {
			writeError(w, http.StatusBadRequest, "INVALID_SLUG", "Space link must use lowercase letters, numbers, and single hyphens.")
			return
		} else {
			input.Slug = &slug
		}
	}
	workspace, err := h.store.UpdateV2HomeWorkspace(
		r.Context(), r.PathValue("workspaceId"), userID, input.Name,
		input.Description, input.ImageObjectPath, input.Slug, input.TimeZone,
	)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" {
		writeError(w, http.StatusConflict, "SLUG_TAKEN", "That Space link is already in use.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "update Space settings", err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (h *Handler) v2GetNotificationPreferences(w http.ResponseWriter, r *http.Request, userID, role string) {
	preferences, err := h.store.ListV2NotificationPreferences(
		r.Context(), r.PathValue("workspaceId"), userID, role,
	)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "read Space notification preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

type v2NotificationPreferencesRequest struct {
	Preferences []postgres.V2NotificationPreference `json:"preferences"`
}

func (h *Handler) v2UpdateNotificationPreferences(w http.ResponseWriter, r *http.Request, userID, role string) {
	var input v2NotificationPreferencesRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	if len(input.Preferences) < 2 || len(input.Preferences) > 3 {
		writeError(w, http.StatusBadRequest, "INVALID_PREFERENCES", "Choose notification settings for assignments, reminders, and membership where available.")
		return
	}
	seen := make(map[string]bool, len(input.Preferences))
	for _, preference := range input.Preferences {
		if preference.EventKey != "assignments" && preference.EventKey != "reminders" &&
			preference.EventKey != "membership" {
			writeError(w, http.StatusBadRequest, "INVALID_PREFERENCES", "A notification category is invalid.")
			return
		}
		if seen[preference.EventKey] {
			writeError(w, http.StatusBadRequest, "INVALID_PREFERENCES", "A notification category was repeated.")
			return
		}
		if preference.EventKey == "membership" && role != "owner" && role != "admin" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Membership notifications are available only to Space owners and admins.")
			return
		}
		seen[preference.EventKey] = true
	}
	if !seen["assignments"] || !seen["reminders"] {
		writeError(w, http.StatusBadRequest, "INVALID_PREFERENCES", "Assignment and reminder settings are required.")
		return
	}
	if (role == "owner" || role == "admin") && !seen["membership"] {
		writeError(w, http.StatusBadRequest, "INVALID_PREFERENCES", "Membership settings are required for Space owners and admins.")
		return
	}
	if err := h.store.ReplaceV2NotificationPreferences(
		r.Context(), r.PathValue("workspaceId"), userID, input.Preferences,
	); errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	} else if err != nil {
		h.v2InternalError(w, r, userID, "update Space notification preferences", err)
		return
	}
	preferences, err := h.store.ListV2NotificationPreferences(
		r.Context(), r.PathValue("workspaceId"), userID, role,
	)
	if err != nil {
		h.v2InternalError(w, r, userID, "read updated Space notification preferences", err)
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) v2ListSpaceMembers(w http.ResponseWriter, r *http.Request, userID, role string) {
	members, err := h.store.ListV2SpaceMembers(r.Context(), r.PathValue("workspaceId"), role)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "list Space members", err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

type v2SpaceMemberRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) v2UpdateSpaceMemberRole(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only the Space owner can change member roles.")
		return
	}
	targetUserID := r.PathValue("userId")
	if !v2IdentifierPattern.MatchString(targetUserID) {
		writeError(w, http.StatusBadRequest, "INVALID_MEMBER_ID", "A valid member is required.")
		return
	}
	var input v2SpaceMemberRoleRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	input.Role = strings.TrimSpace(input.Role)
	if input.Role != "admin" && input.Role != "member" {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "Choose Admin or Member.")
		return
	}
	err := h.store.UpdateV2SpaceMemberRole(
		r.Context(), r.PathValue("workspaceId"), userID, targetUserID, input.Role,
	)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active Space member not found.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "The Space owner cannot be changed or removed here.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "change Space member role", err)
		return
	}
	h.sendV2MembershipEmails(r.Context(), r.PathValue("workspaceId"), userID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2RemoveSpaceMember(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can remove members.")
		return
	}
	targetUserID := r.PathValue("userId")
	if !v2IdentifierPattern.MatchString(targetUserID) {
		writeError(w, http.StatusBadRequest, "INVALID_MEMBER_ID", "A valid member is required.")
		return
	}
	err := h.store.RemoveV2SpaceMember(r.Context(), r.PathValue("workspaceId"), userID, targetUserID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active Space member not found.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You cannot remove this Space member.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "remove Space member", err)
		return
	}
	h.sendV2MembershipEmails(r.Context(), r.PathValue("workspaceId"), userID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2LeaveWorkspace(w http.ResponseWriter, r *http.Request, userID, _ string) {
	err := h.store.LeaveV2Workspace(r.Context(), r.PathValue("workspaceId"), userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active Space membership not found.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Owners cannot leave their Space. Personal Spaces cannot be left.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "leave Space", err)
		return
	}
	h.sendV2MembershipEmails(r.Context(), r.PathValue("workspaceId"), userID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2DeleteWorkspace(w http.ResponseWriter, r *http.Request, userID, _ string) {
	err := h.store.DeleteV2Workspace(r.Context(), r.PathValue("workspaceId"), userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Active Space not found.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only the owner can delete a shared Space.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "delete Space", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type v2WorkspaceGroupRequest struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	MemberIDs   []string `json:"memberUserIds"`
	Permissions []string `json:"permissions"`
}

func (h *Handler) v2ListWorkspaceGroups(w http.ResponseWriter, r *http.Request, userID, _ string) {
	groups, err := h.store.ListV2WorkspaceGroups(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		h.v2InternalError(w, r, userID, "list Space groups", err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) v2CreateWorkspaceGroup(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can manage groups.")
		return
	}
	var input v2WorkspaceGroupRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	if !validateV2WorkspaceGroup(w, &input) {
		return
	}
	group, err := h.store.CreateV2WorkspaceGroup(r.Context(), r.PathValue("workspaceId"), userID, input.Name, input.Description)
	if err != nil {
		if errors.Is(err, postgres.ErrV2Forbidden) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Groups are available only in shared Spaces.")
		} else {
			h.v2InternalError(w, r, userID, "create Space group", err)
		}
		return
	}
	if err := h.store.ReplaceV2WorkspaceGroup(r.Context(), group.WorkspaceID, group.ID, userID, input.Name, input.Description, input.MemberIDs, input.Permissions); err != nil {
		h.v2InternalError(w, r, userID, "set new Space group details", err)
		return
	}
	groups, err := h.store.ListV2WorkspaceGroups(r.Context(), group.WorkspaceID)
	if err != nil {
		h.v2InternalError(w, r, userID, "read created Space group", err)
		return
	}
	for _, saved := range groups {
		if saved.ID == group.ID {
			writeJSON(w, http.StatusCreated, saved)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The group was created but could not be read.")
}

func (h *Handler) v2UpdateWorkspaceGroup(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can manage groups.")
		return
	}
	if !validV2Identifier(r.PathValue("groupId")) {
		writeError(w, http.StatusBadRequest, "INVALID_GROUP_ID", "A valid group is required.")
		return
	}
	var input v2WorkspaceGroupRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	if !validateV2WorkspaceGroup(w, &input) {
		return
	}
	err := h.store.ReplaceV2WorkspaceGroup(r.Context(), r.PathValue("workspaceId"), r.PathValue("groupId"), userID,
		input.Name, input.Description, input.MemberIDs, input.Permissions)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Group not found or unavailable.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "update Space group", err)
		return
	}
	groups, err := h.store.ListV2WorkspaceGroups(r.Context(), r.PathValue("workspaceId"))
	if err != nil {
		h.v2InternalError(w, r, userID, "read updated Space group", err)
		return
	}
	for _, group := range groups {
		if group.ID == r.PathValue("groupId") {
			writeJSON(w, http.StatusOK, group)
			return
		}
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", "Group not found.")
}

func (h *Handler) v2DeleteWorkspaceGroup(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can manage groups.")
		return
	}
	if !validV2Identifier(r.PathValue("groupId")) {
		writeError(w, http.StatusBadRequest, "INVALID_GROUP_ID", "A valid group is required.")
		return
	}
	err := h.store.DeleteV2WorkspaceGroup(r.Context(), r.PathValue("workspaceId"), r.PathValue("groupId"), userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Group not found or unavailable.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "delete Space group", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateV2WorkspaceGroup(w http.ResponseWriter, input *v2WorkspaceGroupRequest) bool {
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 80 {
		writeError(w, http.StatusBadRequest, "INVALID_GROUP", "Group name must contain 1 to 80 characters.")
		return false
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if len([]rune(value)) > 500 {
			writeError(w, http.StatusBadRequest, "INVALID_GROUP", "Group description must be at most 500 characters.")
			return false
		}
		if value == "" {
			input.Description = nil
		} else {
			input.Description = &value
		}
	}
	if len(input.MemberIDs) > 500 || len(input.Permissions) > 40 {
		writeError(w, http.StatusBadRequest, "INVALID_GROUP", "Group member or permission limit exceeded.")
		return false
	}
	input.MemberIDs = uniqueV2Strings(input.MemberIDs)
	input.Permissions = uniqueV2Strings(input.Permissions)
	for _, memberID := range input.MemberIDs {
		if !validV2Identifier(memberID) {
			writeError(w, http.StatusBadRequest, "INVALID_GROUP", "A group member ID is invalid.")
			return false
		}
	}
	for _, permission := range input.Permissions {
		if len(permission) > 100 || !regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*){1,2}$`).MatchString(permission) {
			writeError(w, http.StatusBadRequest, "INVALID_GROUP", "A permission key is invalid.")
			return false
		}
	}
	return true
}

func uniqueV2Strings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

type v2SpaceInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) v2ListSpaceInvitations(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can view invitations.")
		return
	}
	invitations, err := h.store.ListV2WorkspaceInvitations(r.Context(), r.PathValue("workspaceId"))
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space not found.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "list Space invitations", err)
		return
	}
	writeJSON(w, http.StatusOK, invitations)
}

func (h *Handler) v2CreateSpaceInvitation(w http.ResponseWriter, r *http.Request, userID, actorRole string) {
	if actorRole != "owner" && actorRole != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can invite members.")
		return
	}
	if h.spaceInviteMailer == nil || strings.TrimSpace(h.canonicalOrigin) == "" {
		writeError(w, http.StatusServiceUnavailable, "INVITATIONS_UNAVAILABLE", "Invitation email is not configured.")
		return
	}
	var input v2SpaceInvitationRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	input.Email = strings.TrimSpace(input.Email)
	parsed, err := mail.ParseAddress(input.Email)
	if err != nil || parsed.Address != input.Email || len(input.Email) > 320 ||
		strings.ContainsAny(input.Email, "\r\n") {
		writeError(w, http.StatusBadRequest, "INVALID_EMAIL", "Enter a valid email address.")
		return
	}
	input.Role = strings.TrimSpace(input.Role)
	if input.Role == "" {
		input.Role = "member"
	}
	if input.Role != "member" && input.Role != "admin" {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "Choose Member or Admin.")
		return
	}
	if input.Role == "admin" && actorRole != "owner" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only the Space owner can invite an admin.")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.v2InternalError(w, r, userID, "create Space invitation token", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenDigest := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(tokenDigest[:])
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	invitation, err := h.store.CreateV2WorkspaceInvitation(
		r.Context(), r.PathValue("workspaceId"), userID, input.Email,
		strings.ToLower(input.Email), input.Role, tokenHash, expiresAt,
	)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Space is unavailable.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You cannot invite members to this Space.")
		return
	}
	if errors.Is(err, postgres.ErrV2AlreadyMember) {
		writeError(w, http.StatusConflict, "ALREADY_MEMBER", "That email already belongs to a Space member.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "create Space invitation", err)
		return
	}
	inviteURL := strings.TrimRight(h.canonicalOrigin, "/") + "/invite/" + token
	body := "You have been invited to join a Space in Askolo.\n\n" +
		"Open this link to review the invitation after signing in with " + input.Email + ":\n" +
		inviteURL + "\n\nThis invitation expires in 7 days. If you were not expecting it, you can ignore this message."
	if err := h.spaceInviteMailer(r.Context(), input.Email, "You’re invited to join an Askolo Space", body); err != nil {
		if h.logger != nil {
			h.logger.Warn("Space invitation email delivery failed", "workspace_id", r.PathValue("workspaceId"))
		}
		writeError(w, http.StatusBadGateway, "INVITATION_EMAIL_FAILED", "The invitation could not be delivered. Retry to send a fresh link.")
		return
	}
	writeJSON(w, http.StatusCreated, invitation)
}

func (h *Handler) v2CancelSpaceInvitation(w http.ResponseWriter, r *http.Request, userID, role string) {
	if role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Only Space owners and admins can cancel invitations.")
		return
	}
	invitationID := r.PathValue("invitationId")
	if !v2IdentifierPattern.MatchString(invitationID) {
		writeError(w, http.StatusBadRequest, "INVALID_INVITATION_ID", "A valid invitation is required.")
		return
	}
	err := h.store.CancelV2WorkspaceInvitation(r.Context(), r.PathValue("workspaceId"), invitationID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Pending invitation not found.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "cancel Space invitation", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type v2AcceptSpaceInvitationRequest struct {
	Token string `json:"token"`
}

func (h *Handler) v2AcceptSpaceInvitation(w http.ResponseWriter, r *http.Request, userID string) {
	var input v2AcceptSpaceInvitationRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	tokenBytes, err := base64.RawURLEncoding.DecodeString(input.Token)
	if err != nil || len(tokenBytes) != 32 {
		writeError(w, http.StatusBadRequest, "INVALID_INVITATION", "The invitation link is invalid.")
		return
	}
	tokenDigest := sha256.Sum256([]byte(input.Token))
	tokenHash := hex.EncodeToString(tokenDigest[:])
	workspaceID, role, err := h.store.AcceptV2WorkspaceInvitation(r.Context(), tokenHash, userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "INVITATION_UNAVAILABLE", "This invitation is expired or no longer available.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "INVITATION_EMAIL_MISMATCH", "Sign in with the email address this invitation was sent to.")
		return
	}
	if errors.Is(err, postgres.ErrV2AlreadyMember) {
		writeError(w, http.StatusConflict, "ALREADY_MEMBER", "You already belong to this Space.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "accept Space invitation", err)
		return
	}
	h.sendV2MembershipEmails(r.Context(), workspaceID, userID)
	writeJSON(w, http.StatusOK, map[string]string{"workspaceId": workspaceID, "role": role})
}

func (h *Handler) v2DeclineSpaceInvitation(w http.ResponseWriter, r *http.Request, userID string) {
	var input v2AcceptSpaceInvitationRequest
	if !decodeV2JSON(w, r, &input) {
		return
	}
	tokenBytes, err := base64.RawURLEncoding.DecodeString(input.Token)
	if err != nil || len(tokenBytes) != 32 {
		writeError(w, http.StatusBadRequest, "INVALID_INVITATION", "The invitation link is invalid.")
		return
	}
	tokenDigest := sha256.Sum256([]byte(input.Token))
	tokenHash := hex.EncodeToString(tokenDigest[:])
	workspaceID, err := h.store.DeclineV2WorkspaceInvitation(r.Context(), tokenHash, userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "INVITATION_UNAVAILABLE", "This invitation is expired or no longer available.")
		return
	}
	if errors.Is(err, postgres.ErrV2Forbidden) {
		writeError(w, http.StatusForbidden, "INVITATION_EMAIL_MISMATCH", "Sign in with the email address this invitation was sent to.")
		return
	}
	if err != nil {
		h.v2InternalError(w, r, userID, "decline Space invitation", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"workspaceId": workspaceID})
}

func (h *Handler) v2ProtectAll(next v2AllActorHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), v2RequestTimeout)
		defer cancel()
		r = r.WithContext(ctx)
		requestID, idErr := id.New()
		if idErr != nil {
			requestID = "unavailable"
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "private, no-store")

		userID, status := h.sessionUserID(r)
		if status != http.StatusOK {
			switch status {
			case http.StatusUnauthorized:
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized.")
			case http.StatusForbidden:
				writeError(w, http.StatusForbidden, "MFA_REQUIRED", "Complete the required security challenge.")
			default:
				writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
			}
			return
		}
		if h.store == nil {
			writeError(w, http.StatusServiceUnavailable, "V2_UNAVAILABLE", "The versioned API is temporarily unavailable.")
			return
		}
		allowed, retryAfter, err := h.store.ConsumeV2Request(ctx, userID, false)
		if err != nil {
			if errors.Is(err, postgres.ErrV2SchemaNotReady) {
				writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned API is not ready.")
				return
			}
			h.v2InternalError(w, r, userID, "All Spaces rate limit", err)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again shortly.")
			return
		}
		next(w, r, userID)
	}
}

func (h *Handler) v2Protect(write bool, next v2ActorHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), v2RequestTimeout)
		defer cancel()
		r = r.WithContext(ctx)
		requestID, idErr := id.New()
		if idErr != nil {
			requestID = "unavailable"
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("Cache-Control", "private, no-store")

		userID, status := h.sessionUserID(r)
		if status != http.StatusOK {
			switch status {
			case http.StatusUnauthorized:
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized.")
			case http.StatusForbidden:
				writeError(w, http.StatusForbidden, "MFA_REQUIRED", "Complete the required security challenge.")
			default:
				writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
			}
			return
		}
		workspaceID := r.PathValue("workspaceId")
		if !v2WorkspaceIdentifierPattern.MatchString(workspaceID) {
			writeError(w, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "A valid workspace ID is required.")
			return
		}
		if h.store == nil {
			writeError(w, http.StatusServiceUnavailable, "V2_UNAVAILABLE", "The versioned API is temporarily unavailable.")
			return
		}
		role, err := h.store.V2WorkspaceRole(ctx, workspaceID, userID)
		if err != nil {
			if errors.Is(err, postgres.ErrNotFound) {
				writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "Workspace is unavailable.")
				return
			}
			if errors.Is(err, postgres.ErrV2SchemaNotReady) {
				writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned API is not ready.")
				return
			}
			h.v2InternalError(w, r, userID, "workspace authorization", err)
			return
		}
		allowed, retryAfter, err := h.store.ConsumeV2Request(ctx, userID, write)
		if err != nil {
			if errors.Is(err, postgres.ErrV2SchemaNotReady) {
				writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned API is not ready.")
				return
			}
			h.v2InternalError(w, r, userID, "rate limit", err)
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again shortly.")
			return
		}
		next(w, r, userID, role)
	}
}

func (h *Handler) v2Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned API is not ready.")
		return
	}
	if err := h.store.V2SchemaReady(ctx); err != nil {
		if errors.Is(err, postgres.ErrV2SchemaNotReady) {
			writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned API is not ready.")
			return
		}
		h.v2InternalError(w, r, "", "schema readiness", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "apiVersion": 2})
}

func (h *Handler) v2InternalError(w http.ResponseWriter, r *http.Request, userID, operation string, err error) {
	requestID := r.Header.Get("X-Request-ID")
	errorKind := fmt.Sprintf("%T", err)
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		errorKind = "postgres:" + postgresError.Code
	}
	if h.logger != nil {
		h.logger.Error("versioned API request failed",
			"operation", operation,
			"request_id", requestID,
			"user_id", userID,
			"workspace_id", r.PathValue("workspaceId"),
			"error_kind", errorKind,
		)
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed.")
}

func v2HandleStoreError(w http.ResponseWriter, r *http.Request, h *Handler, userID, operation string, err error) {
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "The requested resource is unavailable.")
	case errors.Is(err, postgres.ErrV2Forbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this resource.")
	case errors.Is(err, postgres.ErrV2Cycle):
		writeError(w, http.StatusConflict, "RELATIONSHIP_CYCLE", "Task relationships cannot contain a cycle.")
	case errors.Is(err, postgres.ErrV2InvalidRange):
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "The date is outside the resource period.")
	case errors.Is(err, postgres.ErrV2InvalidHomeFocus):
		writeError(w, http.StatusBadRequest, "INVALID_FOCUS", "The selected focus actions are unavailable.")
	case errors.Is(err, postgres.ErrV2InvalidProviderAccess):
		writeError(w, http.StatusBadRequest, "INVALID_PROVIDER_ACCESS", "The provider access setting is invalid.")
	default:
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			h.logger.Warn(
				"V2 request rejected by database constraint",
				"operation", operation,
				"constraint", databaseError.ConstraintName,
				"sql_state", databaseError.Code,
			)
			switch databaseError.Code {
			case "23505":
				writeError(w, http.StatusConflict, "RESOURCE_CONFLICT", "The requested resource conflicts with existing data.")
				return
			case "23503":
				writeError(w, http.StatusBadRequest, "INVALID_REFERENCE", "A referenced resource is unavailable.")
				return
			case "23514", "22001", "22P02":
				writeError(w, http.StatusBadRequest, "INVALID_RESOURCE", "The request contains an invalid value.")
				return
			}
		}
		h.v2InternalError(w, r, userID, operation, err)
	}
}

func decodeV2JSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, v2MaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body is too large.")
			return false
		}
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body must contain one JSON value.")
		return false
	}
	return true
}

func v2Page(r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	query := r.URL.Query()
	if values, exists := query["limit"]; exists && (len(values) != 1 || values[0] == "") {
		return 0, 0, false
	}
	if values, exists := query["offset"]; exists && (len(values) != 1 || values[0] == "") {
		return 0, 0, false
	}
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, false
		}
		limit = value
	}
	if raw := query.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 100000 {
			return 0, 0, false
		}
		offset = value
	}
	return limit, offset, true
}

func v2QueryValid(r *http.Request, allowed ...string) bool {
	allowedNames := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedNames[name] = struct{}{}
	}
	for name, values := range r.URL.Query() {
		if _, ok := allowedNames[name]; !ok || len(values) != 1 || values[0] == "" {
			return false
		}
	}
	return true
}

func parseV2Date(value string) (time.Time, bool) {
	date, err := time.Parse("2006-01-02", value)
	return date, err == nil
}

func validV2Identifier(value string) bool {
	return value == "" || v2IdentifierPattern.MatchString(value)
}

func (h *Handler) v2ListTasks(w http.ResponseWriter, r *http.Request, userID, _ string) {
	if !v2QueryValid(r, "limit", "offset", "kind", "status", "assignedToUserId", "from", "to") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	limit, offset, ok := v2Page(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PAGINATION", "Use a limit from 1 to 100 and a non-negative offset.")
		return
	}
	filter := postgres.V2TaskFilter{Limit: limit, Offset: offset}
	query := r.URL.Query()
	filter.Kind = query.Get("kind")
	filter.Status = query.Get("status")
	filter.AssigneeID = query.Get("assignedToUserId")
	if !validV2Identifier(filter.AssigneeID) {
		writeError(w, http.StatusBadRequest, "INVALID_ASSIGNEE", "A valid assignee ID is required.")
		return
	}
	if filter.Kind != "" && !oneOf(filter.Kind, "task", "habit") {
		writeError(w, http.StatusBadRequest, "INVALID_KIND", "Task kind is invalid.")
		return
	}
	if filter.Status != "" && !oneOf(filter.Status, "open", "in_progress", "done", "cancelled", "archived") {
		writeError(w, http.StatusBadRequest, "INVALID_STATUS", "Task status is invalid.")
		return
	}
	fromRaw, toRaw := query.Get("from"), query.Get("to")
	if fromRaw != "" || toRaw != "" {
		from, fromOK := parseV2Date(fromRaw)
		to, toOK := parseV2Date(toRaw)
		if !fromOK || !toOK || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
			writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "Provide a valid date range of no more than 367 days.")
			return
		}
		filter.From, filter.To = &from, &to
	}
	result, err := h.store.ListV2Tasks(r.Context(), r.PathValue("workspaceId"), filter)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list tasks", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) v2CreateTask(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var request v2TaskRequest
	if !decodeV2JSON(w, r, &request) {
		return
	}
	input, err := buildV2TaskWrite(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TASK", err.Error())
		return
	}
	task, err := h.store.CreateV2Task(r.Context(), r.PathValue("workspaceId"), userID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "create task", err)
		return
	}
	h.sendV2AssignmentEmails(r.Context(), userID, task)
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) sendV2AssignmentEmails(ctx context.Context, actorID string, task postgres.V2Task) {
	if h.spaceInviteMailer == nil || (task.AssignedToUserID == nil && task.AssignedToGroupID == nil) {
		return
	}
	recipients, err := h.store.ListV2AssignmentEmailRecipients(
		ctx, task.WorkspaceID, actorID, task.AssignedToUserID, task.AssignedToGroupID,
	)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("task assignment email recipients unavailable", "workspace_id", task.WorkspaceID)
		}
		return
	}
	if len(recipients) > 50 {
		if h.logger != nil {
			h.logger.Warn("task assignment email recipient limit reached", "workspace_id", task.WorkspaceID)
		}
		recipients = recipients[:50]
	}
	body := "A task was assigned to you in Askolo:\n\n" + task.Title
	if strings.TrimSpace(h.canonicalOrigin) != "" {
		body += "\n\nOpen Askolo: " + strings.TrimRight(h.canonicalOrigin, "/") + "/agenda"
	}
	for _, recipient := range recipients {
		sendCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := h.spaceInviteMailer(sendCtx, recipient, "A task was assigned to you", body)
		cancel()
		if err != nil && h.logger != nil {
			h.logger.Warn("task assignment email delivery failed", "workspace_id", task.WorkspaceID)
		}
	}
}

func (h *Handler) sendV2MembershipEmails(ctx context.Context, workspaceID, actorID string) {
	if h.spaceInviteMailer == nil {
		return
	}
	recipients, err := h.store.ListV2MembershipEmailRecipients(ctx, workspaceID, actorID)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("Space membership email recipients unavailable", "workspace_id", workspaceID)
		}
		return
	}
	if len(recipients) > 50 {
		if h.logger != nil {
			h.logger.Warn("Space membership email recipient limit reached", "workspace_id", workspaceID)
		}
		recipients = recipients[:50]
	}
	for _, recipient := range recipients {
		sendCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		err := h.spaceInviteMailer(sendCtx, recipient, "Space membership update", "A member or role change occurred in one of your Askolo Spaces.")
		cancel()
		if err != nil && h.logger != nil {
			h.logger.Warn("Space membership email delivery failed", "workspace_id", workspaceID)
		}
	}
}

func v2TaskAssignmentChanged(a, b postgres.V2Task) bool {
	return !sameV2NullableString(a.AssignedToUserID, b.AssignedToUserID) ||
		!sameV2NullableString(a.AssignedToGroupID, b.AssignedToGroupID)
}

func sameV2NullableString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (h *Handler) v2GetTask(w http.ResponseWriter, r *http.Request, userID, _ string) {
	if !validV2Identifier(r.PathValue("taskId")) {
		writeError(w, http.StatusBadRequest, "INVALID_TASK_ID", "A valid task ID is required.")
		return
	}
	task, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), r.PathValue("taskId"))
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) v2UpdateTask(w http.ResponseWriter, r *http.Request, userID, role string) {
	taskID := r.PathValue("taskId")
	if !validV2Identifier(taskID) {
		writeError(w, http.StatusBadRequest, "INVALID_TASK_ID", "A valid task ID is required.")
		return
	}
	current, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task before update", err)
		return
	}
	canUpdate := current.CreatedByUserID == userID || role == "owner" || role == "admin"
	if !canUpdate && current.AssignedToGroupID != nil {
		canUpdate, err = h.store.IsActiveV2GroupMember(r.Context(), current.WorkspaceID, *current.AssignedToGroupID, userID)
		if err != nil {
			h.v2InternalError(w, r, userID, "check group task access", err)
			return
		}
	}
	if !canUpdate {
		canUpdate, err = h.store.V2WorkspaceHasPermission(r.Context(), current.WorkspaceID, userID, "resource.update.any")
		if err != nil {
			h.v2InternalError(w, r, userID, "check additive task permission", err)
			return
		}
	}
	if !canUpdate {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this task.")
		return
	}
	var patch v2TaskPatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	input, err := mergeV2TaskPatch(current, patch)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TASK", err.Error())
		return
	}
	task, err := h.store.UpdateV2Task(r.Context(), r.PathValue("workspaceId"), taskID, userID, input)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update task", err)
		return
	}
	if v2TaskAssignmentChanged(current, task) {
		h.sendV2AssignmentEmails(r.Context(), userID, task)
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) v2ArchiveTask(w http.ResponseWriter, r *http.Request, userID, role string) {
	taskID := r.PathValue("taskId")
	task, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task before archive", err)
		return
	}
	canArchive := task.CreatedByUserID == userID || role == "owner" || role == "admin"
	if !canArchive && task.AssignedToGroupID != nil {
		canArchive, err = h.store.IsActiveV2GroupMember(r.Context(), task.WorkspaceID, *task.AssignedToGroupID, userID)
		if err != nil {
			h.v2InternalError(w, r, userID, "check group task access", err)
			return
		}
	}
	if !canArchive {
		canArchive, err = h.store.V2WorkspaceHasPermission(r.Context(), task.WorkspaceID, userID, "resource.delete.any")
		if err != nil {
			h.v2InternalError(w, r, userID, "check additive task permission", err)
			return
		}
	}
	if !canArchive {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this task.")
		return
	}
	if err := h.store.ArchiveV2Task(r.Context(), r.PathValue("workspaceId"), taskID); err != nil {
		v2HandleStoreError(w, r, h, userID, "archive task", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2ListTaskOccurrences(w http.ResponseWriter, r *http.Request, userID, _ string) {
	if !v2QueryValid(r, "from", "to") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	taskID := r.PathValue("taskId")
	task, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task for occurrences", err)
		return
	}
	rule, err := recurrenceFromTask(task)
	if err != nil || rule == nil {
		writeError(w, http.StatusConflict, "TASK_NOT_RECURRING", "Occurrence history is available for recurring tasks.")
		return
	}
	from, fromOK := parseV2Date(r.URL.Query().Get("from"))
	to, toOK := parseV2Date(r.URL.Query().Get("to"))
	if !fromOK || !toOK || to.Before(from) || to.Sub(from) > (v2MaxDateRange-1)*24*time.Hour {
		writeError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "Provide a valid occurrence range of no more than 31 days.")
		return
	}
	dates, err := expandV2Recurrence(*rule, from, to)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_RECURRENCE", "The task recurrence is invalid.")
		return
	}
	states, err := h.store.ListV2TaskOccurrenceStates(r.Context(), r.PathValue("workspaceId"), taskID, from, to)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list task occurrence history", err)
		return
	}
	stateByDate := make(map[string]postgres.V2TaskOccurrence, len(states))
	for _, state := range states {
		stateByDate[state.OccurrenceOn.Format("2006-01-02")] = state
	}
	result := make([]postgres.V2TaskOccurrence, 0, len(dates))
	for _, date := range dates {
		key := date.Format("2006-01-02")
		state, exists := stateByDate[key]
		if !exists {
			state = postgres.V2TaskOccurrence{TaskID: taskID, OccurrenceOn: date, Status: "open", History: []postgres.V2TaskOccurrenceEvent{}}
		}
		result = append(result, state)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) v2UpdateTaskOccurrence(w http.ResponseWriter, r *http.Request, userID, role string) {
	taskID := r.PathValue("taskId")
	task, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task before occurrence update", err)
		return
	}
	rule, err := recurrenceFromTask(task)
	if err != nil || rule == nil {
		writeError(w, http.StatusConflict, "TASK_NOT_RECURRING", "Occurrence history is available for recurring tasks.")
		return
	}
	date, ok := parseV2Date(r.PathValue("date"))
	if !ok || !v2RecurrenceOccurs(*rule, date) {
		writeError(w, http.StatusBadRequest, "INVALID_OCCURRENCE", "The date is not an occurrence of this task.")
		return
	}
	if task.CreatedByUserID != userID && task.AssignedToUserID == nil && role != "owner" && role != "admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this occurrence.")
		return
	}
	if task.CreatedByUserID != userID && role != "owner" && role != "admin" &&
		(task.AssignedToUserID == nil || *task.AssignedToUserID != userID) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to change this occurrence.")
		return
	}
	var patch v2OccurrencePatchRequest
	if !decodeV2JSON(w, r, &patch) {
		return
	}
	currentStates, err := h.store.ListV2TaskOccurrenceStates(r.Context(), r.PathValue("workspaceId"), taskID, date, date)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read occurrence before update", err)
		return
	}
	status := "open"
	var dueAt *time.Time
	if len(currentStates) == 1 {
		status = currentStates[0].Status
		dueAt = currentStates[0].DueAtOverride
	}
	if patch.Status.Set {
		if patch.Status.Null || !oneOf(patch.Status.Value, "open", "done", "skipped", "cancelled") {
			writeError(w, http.StatusBadRequest, "INVALID_OCCURRENCE_STATUS", "Occurrence status is invalid.")
			return
		}
		status = patch.Status.Value
	}
	if patch.DueAtOverride.Set {
		if patch.DueAtOverride.Null {
			dueAt = nil
		} else {
			local, err := time.LoadLocation(rule.TimeZone)
			if err != nil || patch.DueAtOverride.Value.In(local).Format("2006-01-02") != date.Format("2006-01-02") {
				writeError(w, http.StatusBadRequest, "INVALID_DUE_TIME", "The due-time override must fall on the occurrence date.")
				return
			}
			dueAt = &patch.DueAtOverride.Value
		}
	}
	occurrence, err := h.store.UpdateV2TaskOccurrence(
		r.Context(), r.PathValue("workspaceId"), taskID, userID, date, status, dueAt,
	)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update task occurrence", err)
		return
	}
	writeJSON(w, http.StatusOK, occurrence)
}

func buildV2TaskWrite(request v2TaskRequest) (postgres.V2TaskWrite, error) {
	input := postgres.V2TaskWrite{
		AssignedToUserID:  request.AssignedToUserID,
		AssignedToGroupID: request.AssignedToGroupID,
		Kind:              request.Kind,
		Title:             strings.TrimSpace(request.Title),
		Details:           request.Details,
		Status:            request.Status,
		Priority:          request.Priority,
		PlannedStartAt:    request.PlannedStartAt,
		PlannedStartTime:  request.PlannedStartTime,
		DurationMinutes:   request.DurationMinutes,
		DueTime:           request.DueTime,
		TimeZone:          strings.TrimSpace(request.TimeZone),
		Label:             request.Label,
		GoalID:            request.GoalID,
		ParentTaskID:      request.ParentTaskID,
		SourceType:        request.SourceType,
		SourceReference:   request.SourceReference,
		Dependencies:      request.Dependencies,
	}
	if input.Status == "" {
		input.Status = "open"
	}
	if input.Priority == "" {
		input.Priority = "none"
	}
	if input.TimeZone == "" {
		input.TimeZone = "UTC"
	}
	if request.PlannedOn != nil {
		date, ok := parseV2Date(*request.PlannedOn)
		if !ok {
			return input, errors.New("plannedOn must be a calendar date in YYYY-MM-DD format")
		}
		input.PlannedOn = &date
	}
	if request.DueOn != nil {
		date, ok := parseV2Date(*request.DueOn)
		if !ok {
			return input, errors.New("dueOn must be a calendar date in YYYY-MM-DD format")
		}
		input.DueOn = &date
	}
	if request.Recurrence != nil {
		if err := validateV2Recurrence(*request.Recurrence); err != nil {
			return input, err
		}
		encoded, err := json.Marshal(request.Recurrence)
		if err != nil {
			return input, errors.New("recurrence could not be encoded")
		}
		input.Recurrence = encoded
	}
	if err := validateV2TaskWrite(input, ""); err != nil {
		return input, err
	}
	return input, nil
}

func mergeV2TaskPatch(current postgres.V2Task, patch v2TaskPatchRequest) (postgres.V2TaskWrite, error) {
	input := postgres.V2TaskWrite{
		AssignedToUserID:  current.AssignedToUserID,
		AssignedToGroupID: current.AssignedToGroupID,
		Kind:              current.Kind,
		Title:             current.Title,
		Details:           current.Details,
		Status:            current.Status,
		Priority:          current.Priority,
		DueOn:             current.DueOn,
		PlannedStartAt:    current.PlannedStartAt,
		PlannedOn:         current.PlannedOn,
		PlannedStartTime:  normalizeV2TimeForWrite(current.PlannedStartTime),
		DurationMinutes:   current.DurationMinutes,
		DueTime:           normalizeV2TimeForWrite(current.DueTime),
		TimeZone:          current.TimeZone,
		Label:             current.Label,
		GoalID:            current.GoalID,
		ParentTaskID:      current.ParentTaskID,
		Recurrence:        normalizeV2NullableJSON(current.Recurrence),
		SourceType:        current.SourceType,
		SourceReference:   current.SourceReference,
		Dependencies:      current.Dependencies,
	}
	if patch.AssignedToUserID.Set {
		if patch.AssignedToUserID.Null {
			input.AssignedToUserID = nil
		} else {
			value := strings.TrimSpace(patch.AssignedToUserID.Value)
			input.AssignedToUserID = &value
			input.AssignedToGroupID = nil
		}
	}
	if patch.AssignedToGroupID.Set {
		if patch.AssignedToGroupID.Null {
			input.AssignedToGroupID = nil
		} else {
			value := strings.TrimSpace(patch.AssignedToGroupID.Value)
			input.AssignedToGroupID = &value
			input.AssignedToUserID = nil
		}
	}
	if patch.Kind.Set {
		if patch.Kind.Null {
			return input, errors.New("kind cannot be null")
		}
		input.Kind = patch.Kind.Value
	}
	if patch.Title.Set {
		if patch.Title.Null {
			return input, errors.New("title cannot be null")
		}
		input.Title = strings.TrimSpace(patch.Title.Value)
	}
	if patch.Details.Set {
		if patch.Details.Null {
			input.Details = nil
		} else {
			input.Details = &patch.Details.Value
		}
	}
	if patch.Status.Set {
		if patch.Status.Null {
			return input, errors.New("status cannot be null")
		}
		input.Status = patch.Status.Value
	}
	if patch.Priority.Set {
		if patch.Priority.Null {
			return input, errors.New("priority cannot be null")
		}
		input.Priority = patch.Priority.Value
	}
	if patch.DueOn.Set {
		input.DueOn = nil
		if !patch.DueOn.Null {
			date, ok := parseV2Date(patch.DueOn.Value)
			if !ok {
				return input, errors.New("dueOn must be a calendar date in YYYY-MM-DD format")
			}
			input.DueOn = &date
		}
	}
	if patch.PlannedStartAt.Set {
		input.PlannedStartAt = nil
		if !patch.PlannedStartAt.Null {
			value := patch.PlannedStartAt.Value
			input.PlannedStartAt = &value
		}
	}
	if patch.PlannedOn.Set {
		input.PlannedOn = nil
		if !patch.PlannedOn.Null {
			date, ok := parseV2Date(patch.PlannedOn.Value)
			if !ok {
				return input, errors.New("plannedOn must be a calendar date in YYYY-MM-DD format")
			}
			input.PlannedOn = &date
		}
	}
	if patch.PlannedStartTime.Set {
		input.PlannedStartTime = nil
		if !patch.PlannedStartTime.Null {
			value := strings.TrimSpace(patch.PlannedStartTime.Value)
			input.PlannedStartTime = &value
		}
	}
	if patch.DurationMinutes.Set {
		input.DurationMinutes = nil
		if !patch.DurationMinutes.Null {
			value := patch.DurationMinutes.Value
			input.DurationMinutes = &value
		}
	}
	if patch.DueTime.Set {
		input.DueTime = nil
		if !patch.DueTime.Null {
			value := strings.TrimSpace(patch.DueTime.Value)
			input.DueTime = &value
		}
	}
	if patch.TimeZone.Set {
		if patch.TimeZone.Null {
			return input, errors.New("timeZone cannot be null")
		}
		input.TimeZone = strings.TrimSpace(patch.TimeZone.Value)
	}
	if patch.Label.Set {
		input.Label = nil
		if !patch.Label.Null {
			value := strings.TrimSpace(patch.Label.Value)
			input.Label = &value
		}
	}
	if patch.GoalID.Set {
		input.GoalID = nil
		if !patch.GoalID.Null {
			value := strings.TrimSpace(patch.GoalID.Value)
			input.GoalID = &value
		}
	}
	if patch.ParentTaskID.Set {
		input.ParentTaskID = nil
		if !patch.ParentTaskID.Null {
			value := strings.TrimSpace(patch.ParentTaskID.Value)
			input.ParentTaskID = &value
		}
	}
	if patch.Recurrence.Set {
		input.Recurrence = nil
		if !patch.Recurrence.Null {
			if err := validateV2Recurrence(patch.Recurrence.Value); err != nil {
				return input, err
			}
			encoded, err := json.Marshal(patch.Recurrence.Value)
			if err != nil {
				return input, errors.New("recurrence could not be encoded")
			}
			input.Recurrence = encoded
		}
	}
	if patch.SourceType.Set {
		input.SourceType = nil
		if !patch.SourceType.Null {
			value := strings.TrimSpace(patch.SourceType.Value)
			input.SourceType = &value
		}
	}
	if patch.SourceReference.Set {
		input.SourceReference = nil
		if !patch.SourceReference.Null {
			value := strings.TrimSpace(patch.SourceReference.Value)
			input.SourceReference = &value
		}
	}
	if patch.Dependencies.Set {
		if patch.Dependencies.Null {
			return input, errors.New("dependencies cannot be null")
		}
		input.Dependencies = patch.Dependencies.Value
	}
	if err := validateV2TaskWrite(input, current.ID); err != nil {
		return input, err
	}
	return input, nil
}

func normalizeV2TimeForWrite(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if parsed, err := time.Parse("15:04:05", normalized); err == nil {
		normalized = parsed.Format("15:04")
	}
	return &normalized
}

func normalizeV2NullableJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil
	}
	return value
}

func validateV2TaskWrite(input postgres.V2TaskWrite, currentID string) error {
	if !oneOf(input.Kind, "task", "habit") {
		return errors.New("kind must be task or habit")
	}
	if len(input.Title) == 0 || len([]rune(input.Title)) > 200 {
		return errors.New("title must contain between 1 and 200 characters")
	}
	if input.Details != nil && len([]rune(*input.Details)) > 6000 {
		return errors.New("details cannot exceed 6000 characters")
	}
	if input.Status == "" {
		input.Status = "open"
	}
	if !oneOf(input.Status, "open", "in_progress", "done", "cancelled", "archived") {
		return errors.New("status is invalid")
	}
	if input.Priority == "" {
		input.Priority = "none"
	}
	if !oneOf(input.Priority, "none", "low", "normal", "high") {
		return errors.New("priority must be none, low, normal, or high")
	}
	if input.Kind == "habit" && input.Priority != "none" {
		return errors.New("habits do not have priority")
	}
	if input.Kind == "habit" && input.Details != nil {
		return errors.New("habits do not have notes")
	}
	if input.TimeZone == "" || len(input.TimeZone) > 64 {
		return errors.New("timeZone must be a valid IANA time zone")
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		return errors.New("timeZone must be a valid IANA time zone")
	}
	if input.PlannedStartTime != nil {
		if _, err := time.Parse("15:04", *input.PlannedStartTime); err != nil {
			return errors.New("plannedStartTime must use HH:MM")
		}
		if input.PlannedOn == nil {
			return errors.New("plannedStartTime requires plannedOn")
		}
	}
	if (input.PlannedStartTime == nil) != (input.DurationMinutes == nil) {
		return errors.New("plannedStartTime and durationMinutes must be provided together")
	}
	if input.DurationMinutes != nil && (*input.DurationMinutes < 1 || *input.DurationMinutes > 1440) {
		return errors.New("durationMinutes must be between 1 and 1440")
	}
	if input.DurationMinutes != nil && len(input.Recurrence) > 0 && string(input.Recurrence) != "null" && *input.DurationMinutes >= 1440 {
		return errors.New("recurring task duration must be less than 24 hours")
	}
	if input.DueTime != nil {
		if _, err := time.Parse("15:04", *input.DueTime); err != nil {
			return errors.New("dueTime must use HH:MM")
		}
		if input.DueOn == nil {
			return errors.New("dueTime requires dueOn")
		}
	}
	if input.Label != nil && (len([]rune(strings.TrimSpace(*input.Label))) < 1 || len([]rune(*input.Label)) > 100) {
		return errors.New("label must contain between 1 and 100 characters")
	}
	if input.Details != nil && len([]rune(strings.TrimSpace(*input.Details))) > 6000 {
		return errors.New("notes cannot exceed 6000 characters")
	}
	for _, value := range []*string{input.AssignedToUserID, input.AssignedToGroupID, input.GoalID, input.ParentTaskID} {
		if value != nil && !v2IdentifierPattern.MatchString(*value) {
			return errors.New("task relationship contains an invalid identifier")
		}
	}
	if input.ParentTaskID != nil && *input.ParentTaskID == currentID {
		return errors.New("a task cannot be its own parent")
	}
	if input.SourceType == nil != (input.SourceReference == nil) {
		return errors.New("sourceType and sourceReference must be provided together")
	}
	if input.SourceType != nil {
		if !oneOf(*input.SourceType, "manual", "assistant", "integration") {
			return errors.New("sourceType is invalid")
		}
		if len(*input.SourceReference) == 0 || len([]rune(*input.SourceReference)) > 200 {
			return errors.New("sourceReference must contain between 1 and 200 characters")
		}
	}
	if input.Recurrence != nil && len(input.Recurrence) > 0 && string(input.Recurrence) != "null" {
		var rule v2RecurrenceRule
		decoder := json.NewDecoder(bytes.NewReader(input.Recurrence))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&rule); err != nil {
			return errors.New("recurrence is invalid")
		}
		if err := validateV2Recurrence(rule); err != nil {
			return err
		}
		if input.DueOn != nil || input.PlannedStartAt != nil {
			return errors.New("recurring tasks use recurrence dates and occurrence overrides, not dueOn or plannedStartAt")
		}
	}
	if len(input.Dependencies) > 25 {
		return errors.New("a task may have at most 25 dependencies")
	}
	seen := make(map[string]struct{}, len(input.Dependencies))
	for _, dependencyID := range input.Dependencies {
		if !v2IdentifierPattern.MatchString(dependencyID) || dependencyID == currentID {
			return errors.New("dependencies contain an invalid task ID")
		}
		if _, exists := seen[dependencyID]; exists {
			return errors.New("dependencies cannot contain duplicate task IDs")
		}
		seen[dependencyID] = struct{}{}
	}
	return nil
}

func validateV2Recurrence(rule v2RecurrenceRule) error {
	if !oneOf(rule.Frequency, "daily", "weekly", "monthly", "yearly") {
		return errors.New("recurrence frequency must be daily, weekly, monthly, or yearly")
	}
	if rule.Interval < 1 || rule.Interval > 366 {
		return errors.New("recurrence interval must be between 1 and 366")
	}
	start, ok := parseV2Date(rule.StartsOn)
	if !ok {
		return errors.New("recurrence startsOn must use YYYY-MM-DD")
	}
	if rule.TimeZone == "" || len(rule.TimeZone) > 64 {
		return errors.New("recurrence timeZone must be a valid IANA time zone")
	}
	if _, err := time.LoadLocation(rule.TimeZone); err != nil {
		return errors.New("recurrence timeZone must be a valid IANA time zone")
	}
	if rule.EndsOn != nil {
		end, ok := parseV2Date(*rule.EndsOn)
		if !ok || end.Before(start) {
			return errors.New("recurrence endsOn must be on or after startsOn")
		}
	}
	if rule.Count != nil && (*rule.Count < 1 || *rule.Count > 3660 || rule.EndsOn != nil) {
		return errors.New("recurrence count must be between 1 and 3660 and cannot be combined with endsOn")
	}
	if rule.Frequency != "weekly" && len(rule.Weekdays) > 0 {
		return errors.New("weekdays are only valid for weekly recurrence")
	}
	if len(rule.Weekdays) > 7 {
		return errors.New("weekly recurrence cannot contain more than seven weekdays")
	}
	if rule.TimeOfDay != "" {
		if _, err := time.Parse("15:04", rule.TimeOfDay); err != nil {
			return errors.New("timeOfDay must use HH:MM")
		}
	}
	seen := make(map[int]struct{}, len(rule.Weekdays))
	for _, day := range rule.Weekdays {
		if day < 1 || day > 7 {
			return errors.New("weekdays use ISO values from 1 for Monday through 7 for Sunday")
		}
		if _, exists := seen[day]; exists {
			return errors.New("weekdays cannot contain duplicates")
		}
		seen[day] = struct{}{}
	}
	return nil
}

func recurrenceFromTask(task postgres.V2Task) (*v2RecurrenceRule, error) {
	if len(task.Recurrence) == 0 || string(task.Recurrence) == "null" {
		return nil, nil
	}
	var rule v2RecurrenceRule
	decoder := json.NewDecoder(bytes.NewReader(task.Recurrence))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rule); err != nil {
		return nil, err
	}
	if err := validateV2Recurrence(rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func expandV2Recurrence(rule v2RecurrenceRule, from, to time.Time) ([]time.Time, error) {
	if err := validateV2Recurrence(rule); err != nil {
		return nil, err
	}
	start, _ := parseV2Date(rule.StartsOn)
	end := to
	if rule.EndsOn != nil {
		ruleEnd, _ := parseV2Date(*rule.EndsOn)
		if ruleEnd.Before(end) {
			end = ruleEnd
		}
	}
	if end.Before(start) || to.Before(from) {
		return []time.Time{}, nil
	}
	cursor := from
	if cursor.Before(start) {
		cursor = start
	}
	weekdays := make(map[int]struct{}, len(rule.Weekdays))
	for _, day := range rule.Weekdays {
		weekdays[day] = struct{}{}
	}
	if rule.Frequency == "weekly" && len(weekdays) == 0 {
		weekdays[isoWeekday(start)] = struct{}{}
	}
	result := make([]time.Time, 0, 31)
	for day := cursor; !day.After(end); day = day.AddDate(0, 0, 1) {
		if v2RecurrenceOccursWithWeekdays(rule, day, start, weekdays) {
			result = append(result, day)
		}
	}
	return result, nil
}

func v2RecurrenceOccurs(rule v2RecurrenceRule, date time.Time) bool {
	start, ok := parseV2Date(rule.StartsOn)
	if !ok {
		return false
	}
	weekdays := make(map[int]struct{}, len(rule.Weekdays))
	for _, day := range rule.Weekdays {
		weekdays[day] = struct{}{}
	}
	if rule.Frequency == "weekly" && len(weekdays) == 0 {
		weekdays[isoWeekday(start)] = struct{}{}
	}
	return v2RecurrenceOccursWithWeekdays(rule, date, start, weekdays)
}

func v2RecurrenceOccursWithWeekdays(rule v2RecurrenceRule, date, start time.Time, weekdays map[int]struct{}) bool {
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	if date.Before(start) {
		return false
	}
	if rule.EndsOn != nil {
		end, ok := parseV2Date(*rule.EndsOn)
		if !ok || date.After(end) {
			return false
		}
	}
	matches := false
	switch rule.Frequency {
	case "daily":
		matches = int(date.Sub(start).Hours()/24)%rule.Interval == 0
	case "weekly":
		startWeek := start.AddDate(0, 0, -(isoWeekday(start) - 1))
		dateWeek := date.AddDate(0, 0, -(isoWeekday(date) - 1))
		weeks := int(dateWeek.Sub(startWeek).Hours() / 24 / 7)
		_, matchesDay := weekdays[isoWeekday(date)]
		matches = weeks >= 0 && weeks%rule.Interval == 0 && matchesDay
	case "monthly":
		months := (date.Year()-start.Year())*12 + int(date.Month()-start.Month())
		day := start.Day()
		lastDay := time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day > lastDay {
			day = lastDay
		}
		matches = months >= 0 && months%rule.Interval == 0 && date.Day() == day
	case "yearly":
		years := date.Year() - start.Year()
		day := start.Day()
		lastDay := time.Date(date.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day > lastDay {
			day = lastDay
		}
		matches = years >= 0 && years%rule.Interval == 0 && date.Month() == start.Month() && date.Day() == day
	default:
		return false
	}
	if !matches || rule.Count == nil {
		return matches
	}
	ordinal := 0
	for candidate := start; !candidate.After(date); candidate = candidate.AddDate(0, 0, 1) {
		candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 0, 0, 0, 0, time.UTC)
		if v2RecurrenceOccursWithWeekdaysWithoutCount(rule, candidate, start, weekdays) {
			ordinal++
			if ordinal > *rule.Count {
				return false
			}
		}
	}
	return ordinal > 0
}

func v2RecurrenceOccursWithWeekdaysWithoutCount(rule v2RecurrenceRule, date, start time.Time, weekdays map[int]struct{}) bool {
	rule.Count = nil
	return v2RecurrenceOccursWithWeekdays(rule, date, start, weekdays)
}

func isoWeekday(date time.Time) int {
	weekday := int(date.Weekday())
	if weekday == 0 {
		return 7
	}
	return weekday
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
