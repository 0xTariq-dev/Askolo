package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type v2ActorHandler func(http.ResponseWriter, *http.Request, string, string)
type v2AllActorHandler func(http.ResponseWriter, *http.Request, string)

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
	TimeZone  string  `json:"timeZone"`
}

type v2TaskRequest struct {
	AssignedToUserID *string           `json:"assignedToUserId"`
	Kind             string            `json:"kind"`
	Title            string            `json:"title"`
	Details          *string           `json:"details"`
	Status           string            `json:"status"`
	Priority         string            `json:"priority"`
	DueOn            *string           `json:"dueOn"`
	PlannedStartAt   *time.Time        `json:"plannedStartAt"`
	PlannedOn        *string           `json:"plannedOn"`
	PlannedStartTime *string           `json:"plannedStartTime"`
	DurationMinutes  *int              `json:"durationMinutes"`
	DueTime          *string           `json:"dueTime"`
	TimeZone         string            `json:"timeZone"`
	Label            *string           `json:"label"`
	GoalID           *string           `json:"goalId"`
	ParentTaskID     *string           `json:"parentTaskId"`
	Recurrence       *v2RecurrenceRule `json:"recurrence"`
	SourceType       *string           `json:"sourceType"`
	SourceReference  *string           `json:"sourceReference"`
	Dependencies     []string          `json:"dependencies"`
}

type v2TaskPatchRequest struct {
	AssignedToUserID v2PatchField[string]           `json:"assignedToUserId"`
	Kind             v2PatchField[string]           `json:"kind"`
	Title            v2PatchField[string]           `json:"title"`
	Details          v2PatchField[string]           `json:"details"`
	Status           v2PatchField[string]           `json:"status"`
	Priority         v2PatchField[string]           `json:"priority"`
	DueOn            v2PatchField[string]           `json:"dueOn"`
	PlannedStartAt   v2PatchField[time.Time]        `json:"plannedStartAt"`
	PlannedOn        v2PatchField[string]           `json:"plannedOn"`
	PlannedStartTime v2PatchField[string]           `json:"plannedStartTime"`
	DurationMinutes  v2PatchField[int]              `json:"durationMinutes"`
	DueTime          v2PatchField[string]           `json:"dueTime"`
	TimeZone         v2PatchField[string]           `json:"timeZone"`
	Label            v2PatchField[string]           `json:"label"`
	GoalID           v2PatchField[string]           `json:"goalId"`
	ParentTaskID     v2PatchField[string]           `json:"parentTaskId"`
	Recurrence       v2PatchField[v2RecurrenceRule] `json:"recurrence"`
	SourceType       v2PatchField[string]           `json:"sourceType"`
	SourceReference  v2PatchField[string]           `json:"sourceReference"`
	Dependencies     v2PatchField[[]string]         `json:"dependencies"`
}

type v2OccurrencePatchRequest struct {
	Status        v2PatchField[string]    `json:"status"`
	DueAtOverride v2PatchField[time.Time] `json:"dueAtOverride"`
}

func (h *Handler) mountV2(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/healthz", h.v2Health)
	mux.HandleFunc("GET /api/v2/workspaces", h.v2ProtectAll(h.v2ListWorkspaces))
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
	writeJSON(w, http.StatusCreated, task)
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
	if current.CreatedByUserID != userID && role != "owner" && role != "admin" {
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
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) v2ArchiveTask(w http.ResponseWriter, r *http.Request, userID, role string) {
	taskID := r.PathValue("taskId")
	task, err := h.store.GetV2Task(r.Context(), r.PathValue("workspaceId"), taskID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "read task before archive", err)
		return
	}
	if task.CreatedByUserID != userID && role != "owner" && role != "admin" {
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
		AssignedToUserID: request.AssignedToUserID,
		Kind:             request.Kind,
		Title:            strings.TrimSpace(request.Title),
		Details:          request.Details,
		Status:           request.Status,
		Priority:         request.Priority,
		PlannedStartAt:   request.PlannedStartAt,
		PlannedStartTime: request.PlannedStartTime,
		DurationMinutes:  request.DurationMinutes,
		DueTime:          request.DueTime,
		TimeZone:         strings.TrimSpace(request.TimeZone),
		Label:            request.Label,
		GoalID:           request.GoalID,
		ParentTaskID:     request.ParentTaskID,
		SourceType:       request.SourceType,
		SourceReference:  request.SourceReference,
		Dependencies:     request.Dependencies,
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
		AssignedToUserID: current.AssignedToUserID,
		Kind:             current.Kind,
		Title:            current.Title,
		Details:          current.Details,
		Status:           current.Status,
		Priority:         current.Priority,
		DueOn:            current.DueOn,
		PlannedStartAt:   current.PlannedStartAt,
		PlannedOn:        current.PlannedOn,
		PlannedStartTime: current.PlannedStartTime,
		DurationMinutes:  current.DurationMinutes,
		DueTime:          current.DueTime,
		TimeZone:         current.TimeZone,
		Label:            current.Label,
		GoalID:           current.GoalID,
		ParentTaskID:     current.ParentTaskID,
		Recurrence:       current.Recurrence,
		SourceType:       current.SourceType,
		SourceReference:  current.SourceReference,
		Dependencies:     current.Dependencies,
	}
	if patch.AssignedToUserID.Set {
		if patch.AssignedToUserID.Null {
			input.AssignedToUserID = nil
		} else {
			value := strings.TrimSpace(patch.AssignedToUserID.Value)
			input.AssignedToUserID = &value
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
	for _, value := range []*string{input.AssignedToUserID, input.GoalID, input.ParentTaskID} {
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
	if !oneOf(rule.Frequency, "daily", "weekly", "monthly") {
		return errors.New("recurrence frequency must be daily, weekly, or monthly")
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
	if rule.Frequency != "weekly" && len(rule.Weekdays) > 0 {
		return errors.New("weekdays are only valid for weekly recurrence")
	}
	if len(rule.Weekdays) > 7 {
		return errors.New("weekly recurrence cannot contain more than seven weekdays")
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
	switch rule.Frequency {
	case "daily":
		return int(date.Sub(start).Hours()/24)%rule.Interval == 0
	case "weekly":
		startWeek := start.AddDate(0, 0, -(isoWeekday(start) - 1))
		dateWeek := date.AddDate(0, 0, -(isoWeekday(date) - 1))
		weeks := int(dateWeek.Sub(startWeek).Hours() / 24 / 7)
		_, matchesDay := weekdays[isoWeekday(date)]
		return weeks >= 0 && weeks%rule.Interval == 0 && matchesDay
	case "monthly":
		months := (date.Year()-start.Year())*12 + int(date.Month()-start.Month())
		return months >= 0 && months%rule.Interval == 0 && date.Day() == start.Day()
	default:
		return false
	}
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
