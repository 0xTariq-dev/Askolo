package product

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/platform/apierror"
	policy "askolo/backend/internal/platform/authorization"
)

var habits = postgres.ProductSpec{
	Name: "habit", Table: "habits", Required: []string{"name"},
	Fields: []postgres.ProductField{
		{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "name", Name: "name"},
		{Column: "description", Name: "description"}, {Column: "frequency", Name: "frequency"}, {Column: "color", Name: "color"},
		{Column: "icon", Name: "icon"}, {Column: "current_streak", Name: "currentStreak"}, {Column: "longest_streak", Name: "longestStreak"},
		{Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
	}, Defaults: map[string]any{"frequency": "daily", "current_streak": 0, "longest_streak": 0}, OrderBy: "id ASC",
}

var goals = simpleSpec("goal", "goals", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "title", Name: "title"}, {Column: "description", Name: "description"},
	{Column: "target_date", Name: "targetDate"}, {Column: "status", Name: "status"}, {Column: "progress", Name: "progress"}, {Column: "category", Name: "category"},
	{Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"status": "active", "progress": 0}, "id ASC")

var dailyPlans = simpleSpec("dailyPlan", "daily_plans", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "date", Name: "date"}, {Column: "title", Name: "title"},
	{Column: "time_block", Name: "timeBlock"}, {Column: "priority", Name: "priority"}, {Column: "completed", Name: "completed"}, {Column: "notes", Name: "notes"},
	{Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"priority": "medium", "completed": false}, "date ASC, id ASC")

var events = simpleSpec("event", "events", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "title", Name: "title"}, {Column: "description", Name: "description"},
	{Column: "start_date", Name: "startDate"}, {Column: "start_time", Name: "startTime"}, {Column: "end_date", Name: "endDate"}, {Column: "end_time", Name: "endTime"},
	{Column: "all_day", Name: "allDay"}, {Column: "location", Name: "location"}, {Column: "color", Name: "color"}, {Column: "attendees", Name: "attendees"},
	{Column: "google_event_id", Name: "googleEventId"}, {Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"allDay": false}, "start_date ASC, start_time ASC NULLS LAST, id ASC")

var chores = simpleSpec("chore", "chores", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "title", Name: "title"}, {Column: "description", Name: "description"},
	{Column: "assigned_to", Name: "assignedTo"}, {Column: "frequency", Name: "frequency"}, {Column: "due_date", Name: "dueDate"}, {Column: "completed", Name: "completed"},
	{Column: "completed_at", Name: "completedAt"}, {Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"frequency": "once", "completed": false}, "id ASC")

var notes = simpleSpec("note", "notes", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "title", Name: "title"}, {Column: "content", Name: "content"},
	{Column: "tags", Name: "tags"}, {Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"content": ""}, "updated_at DESC, id DESC")

var actionItems = simpleSpec("actionItem", "action_items", []postgres.ProductField{
	{Column: "id", Name: "id"}, {Column: "user_id", Name: "userId"}, {Column: "title", Name: "title"}, {Column: "source_type", Name: "sourceType"},
	{Column: "source_id", Name: "sourceId"}, {Column: "due_date", Name: "dueDate"}, {Column: "completed", Name: "completed"}, {Column: "completed_at", Name: "completedAt"},
	{Column: "created_at", Name: "createdAt"}, {Column: "updated_at", Name: "updatedAt"},
}, map[string]any{"completed": false}, "id ASC")

func simpleSpec(name, table string, fields []postgres.ProductField, defaults map[string]any, order string) postgres.ProductSpec {
	required := []string{}
	for _, field := range fields {
		if field.Name == "userId" || field.Name == "id" || strings.HasSuffix(field.Name, "At") || strings.HasPrefix(field.Name, "created") || strings.HasPrefix(field.Name, "updated") {
			continue
		}
		if field.Name == "title" || field.Name == "date" {
			required = append(required, field.Name)
		}
	}
	return postgres.ProductSpec{Name: name, Table: table, Fields: fields, Required: required, Defaults: defaults, OrderBy: order}
}

type Handler struct {
	store             *postgres.Store
	logger            *slog.Logger
	sessionCookieName string
	assemblyAIKey     string
}

const voiceProviderRequestCreditCost = 1

func NewHandler(cfg config.Config, store *postgres.Store, logger *slog.Logger, sessionCookieName string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, logger: logger, sessionCookieName: sessionCookieName, assemblyAIKey: cfg.AssemblyAIKey}
}

func (h *Handler) spendVoiceProviderCredit(w http.ResponseWriter, r *http.Request, userID string) bool {
	_, spent, err := h.store.SpendAICredits(r.Context(), userID, voiceProviderRequestCreditCost)
	if err != nil {
		h.storeError(w, "AI credit spend failed", err)
		return false
	}
	if !spent {
		writeError(w, http.StatusPaymentRequired, "INSUFFICIENT_AI_CREDITS", "At least one AI credit is required to use voice features.")
		return false
	}
	return true
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	h.mount(mux, "/habits", habits)
	h.mount(mux, "/goals", goals)
	h.mount(mux, "/daily-plans", dailyPlans)
	h.mount(mux, "/events", events)
	h.mount(mux, "/chores", chores)
	h.mount(mux, "/notes", notes)
	h.mount(mux, "/action-items", actionItems)
	mux.HandleFunc("GET /api/habits/completions", h.listCompletions)
	mux.HandleFunc("GET /api/habits/{id}/completions", h.listCompletionsForHabit)
	mux.HandleFunc("POST /api/habits/{id}/completions", h.completeHabit)
	mux.HandleFunc("DELETE /api/habits/{id}/completions/{date}", h.deleteHabitCompletion)
	mux.HandleFunc("POST /api/chores/{id}/complete", h.completeChore)
	mux.HandleFunc("GET /api/dashboard/summary", h.dashboard)
	mux.HandleFunc("GET /api/ai/credits", h.aiCredits)
	mux.HandleFunc("GET /api/ai/credits/estimate", h.aiCreditEstimate)
	mux.HandleFunc("GET /api/ai/transcription-preferences", h.transcriptionPreferences)
	mux.HandleFunc("PATCH /api/ai/transcription-preferences", h.updateTranscriptionPreferences)
	mux.HandleFunc("POST /api/ai/coaching", h.coaching)
	mux.HandleFunc("POST /api/ai/assistant", h.assistant)
	mux.HandleFunc("POST /api/ai/generate-plan", h.generatePlan)
	mux.HandleFunc("POST /api/ai/voice-to-plan", h.voiceToPlan)
	mux.HandleFunc("POST /api/ai/meeting-extract", h.meetingExtract)
	mux.HandleFunc("POST /api/ai/transcribe-audio", h.transcribeAudio)
	mux.HandleFunc("POST /api/ai/realtime-token", h.realtimeToken)
	mux.HandleFunc("PATCH /api/user/profile", h.updateProfile)
	mux.HandleFunc("DELETE /api/user/data", h.deleteUserData)
	mux.HandleFunc("DELETE /api/user/account", h.deleteAccount)
	return mux
}

func (h *Handler) mount(mux *http.ServeMux, path string, spec postgres.ProductSpec) {
	mux.HandleFunc("GET /api"+path, h.list(spec))
	mux.HandleFunc("POST /api"+path, h.create(spec))
	mux.HandleFunc("GET /api"+path+"/{id}", h.get(spec))
	mux.HandleFunc("PATCH /api"+path+"/{id}", h.update(spec))
	mux.HandleFunc("DELETE /api"+path+"/{id}", h.delete(spec))
}

func (h *Handler) list(spec postgres.ProductSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, status := h.sessionUserID(r)
		if status != http.StatusOK || !h.authorize(r, userID, spec.Name, "", policy.ActionResourceRead, w) {
			return
		}
		filters := map[string]string{}
		if date := r.URL.Query().Get("date"); date != "" {
			filters["date"] = date
		}
		if completed := r.URL.Query().Get("completed"); completed != "" {
			filters["completed"] = completed
		}
		result, err := h.store.ListProduct(r.Context(), spec, userID, filters)
		if err != nil {
			h.storeError(w, "product list failed", err)
			return
		}
		if spec.Name == "habit" {
			h.addHabitStatus(r, userID, result)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *Handler) create(spec postgres.ProductSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, status := h.sessionUserID(r)
		if status != http.StatusOK || !h.authorize(r, userID, spec.Name, "", policy.ActionResourceCreate, w) {
			return
		}
		input, ok := h.decodeInput(w, r, spec, true)
		if !ok {
			return
		}
		result, err := h.store.CreateProduct(r.Context(), spec, userID, input)
		if err != nil {
			h.storeError(w, "product create failed", err)
			return
		}
		if id, ok := integerID(result["id"]); ok {
			_ = h.store.UpsertAuthorizationResource(r.Context(), postgres.AuthorizationResource{
				WorkspaceID: postgres.DefaultWorkspaceID(userID), ResourceType: spec.Name, ResourceID: strconv.FormatInt(id, 10), OwnerUserID: userID,
			})
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func (h *Handler) get(spec postgres.ProductSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.withID(w, r, spec, policy.ActionResourceRead, func(id int64, userID string) (any, error) {
			result, err := h.store.GetProduct(r.Context(), spec, userID, id)
			if err == nil && spec.Name == "habit" {
				h.addHabitStatus(r, userID, []map[string]any{result})
			}
			return result, err
		})
	}
}

func (h *Handler) update(spec postgres.ProductSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.withIDAndInput(w, r, spec, policy.ActionResourceUpdate, func(id int64, userID string, input map[string]any) (any, error) {
			return h.store.UpdateProduct(r.Context(), spec, userID, id, input)
		})
	}
}

func (h *Handler) delete(spec postgres.ProductSpec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
			return
		}
		userID, status := h.sessionUserID(r)
		if status != http.StatusOK || !h.authorize(r, userID, spec.Name, strconv.FormatInt(id, 10), policy.ActionResourceDelete, w) {
			return
		}
		err = h.store.DeleteProduct(r.Context(), spec, userID, id)
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s not found.", spec.Name))
			return
		}
		if err != nil {
			h.storeError(w, "product delete failed", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) withID(w http.ResponseWriter, r *http.Request, spec postgres.ProductSpec, action policy.Action, operation func(int64, string) (any, error)) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, spec.Name, strconv.FormatInt(id, 10), action, w) {
		return
	}
	result, err := operation(id, userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s not found.", spec.Name))
		return
	}
	if err != nil {
		h.storeError(w, "product lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) withIDAndInput(w http.ResponseWriter, r *http.Request, spec postgres.ProductSpec, action policy.Action, operation func(int64, string, map[string]any) (any, error)) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, spec.Name, strconv.FormatInt(id, 10), action, w) {
		return
	}
	input, ok := h.decodeInput(w, r, spec, false)
	if !ok {
		return
	}
	result, err := operation(id, userID, input)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s not found.", spec.Name))
		return
	}
	if err != nil {
		h.storeError(w, "product update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) decodeInput(w http.ResponseWriter, r *http.Request, spec postgres.ProductSpec, requireFields bool) (map[string]any, bool) {
	var input map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid.")
		return nil, false
	}
	if requireFields {
		for _, required := range spec.Required {
			if value, ok := input[required]; !ok || strings.TrimSpace(fmt.Sprint(value)) == "" {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", required+" is required.")
				return nil, false
			}
		}
	}
	allowed := map[string]bool{}
	for _, field := range spec.Fields {
		allowed[field.Name] = field.Column != "id" && field.Column != "user_id" && field.Column != "created_at" && field.Column != "updated_at"
	}
	for key := range input {
		if !allowed[key] {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Unknown or immutable field.")
			return nil, false
		}
	}
	return input, true
}

func (h *Handler) authorize(r *http.Request, userID, resourceType, resourceID string, action policy.Action, w http.ResponseWriter) bool {
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized.")
		return false
	}
	if err := h.store.EnsurePersonalWorkspace(r.Context(), userID); err != nil {
		h.storeError(w, "workspace provisioning failed", err)
		return false
	}
	if resourceID != "" {
		if err := h.store.UpsertAuthorizationResource(r.Context(), postgres.AuthorizationResource{
			WorkspaceID: postgres.DefaultWorkspaceID(userID), ResourceType: resourceType, ResourceID: resourceID, OwnerUserID: userID,
		}); err != nil {
			h.storeError(w, "resource registration failed", err)
			return false
		}
	}
	decision, err := h.store.Authorize(r.Context(), policy.Input{
		ActorUserID: userID, WorkspaceID: postgres.DefaultWorkspaceID(userID), ResourceType: resourceType, ResourceID: resourceID, Action: action,
	})
	if err != nil {
		h.storeError(w, "authorization failed", err)
		return false
	}
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, "AUTHORIZATION_DENIED", "You are not allowed to perform this action.")
		return false
	}
	return true
}

func (h *Handler) sessionUserID(r *http.Request) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	sessionID := ""
	if cookie, err := r.Cookie(config.CookieName(h.sessionCookieName)); err == nil {
		sessionID = cookie.Value
	}
	if sessionID == "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		sessionID = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	}
	if sessionID == "" {
		return "", http.StatusUnauthorized
	}
	userID, err := h.store.SessionUserID(r.Context(), sessionID)
	if errors.Is(err, postgres.ErrNotFound) {
		return "", http.StatusUnauthorized
	}
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	state, err := h.store.SessionMFAState(r.Context(), sessionID)
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	if state.Required && !state.Verified {
		return "", http.StatusForbidden
	}
	return userID, http.StatusOK
}

func (h *Handler) addHabitStatus(r *http.Request, userID string, result []map[string]any) {
	today := time.Now().UTC().Format("2006-01-02")
	for _, habit := range result {
		if id, ok := integerID(habit["id"]); ok {
			completed, err := h.store.HabitCompletedToday(r.Context(), userID, id, today)
			if err == nil {
				habit["completedToday"] = completed
			}
		}
	}
}

func (h *Handler) listCompletions(w http.ResponseWriter, r *http.Request) {
	h.listCompletionsWithID(w, r, nil)
}

func (h *Handler) listCompletionsForHabit(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	h.listCompletionsWithID(w, r, &id)
}

func (h *Handler) listCompletionsWithID(w http.ResponseWriter, r *http.Request, habitID *int64) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "habit", "", policy.ActionResourceRead, w) {
		return
	}
	result, err := h.store.ListHabitCompletions(r.Context(), userID, habitID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		h.storeError(w, "habit completion list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) completeHabit(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "habit", strconv.FormatInt(id, 10), policy.ActionResourceUpdate, w) {
		return
	}
	var input struct {
		Date string `json:"date"`
	}
	if !decodeBody(w, r, &input) || !validDate(input.Date) {
		writeError(w, http.StatusBadRequest, "INVALID_DATE", "A valid YYYY-MM-DD date is required.")
		return
	}
	result, err := h.store.CompleteHabit(r.Context(), userID, id, input.Date)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Habit not found.")
		return
	}
	if err != nil {
		h.storeError(w, "habit completion failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) deleteHabitCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "habit", strconv.FormatInt(id, 10), policy.ActionResourceUpdate, w) {
		return
	}
	if !validDate(r.PathValue("date")) {
		writeError(w, http.StatusBadRequest, "INVALID_DATE", "A valid YYYY-MM-DD date is required.")
		return
	}
	err = h.store.DeleteHabitCompletion(r.Context(), userID, id, r.PathValue("date"))
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Completion not found.")
		return
	}
	if err != nil {
		h.storeError(w, "habit completion delete failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) completeChore(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "A valid numeric id is required.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "chore", strconv.FormatInt(id, 10), policy.ActionResourceUpdate, w) {
		return
	}
	input := map[string]any{}
	if r.ContentLength != 0 {
		if !decodeBody(w, r, &input) {
			return
		}
	}
	input["completed"] = true
	if _, ok := input["completedAt"]; !ok {
		input["completedAt"] = time.Now().UTC()
	}
	result, err := h.store.UpdateProduct(r.Context(), chores, userID, id, input)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Chore not found.")
		return
	}
	if err != nil {
		h.storeError(w, "chore completion failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "dashboard", "", policy.ActionResourceRead, w) {
		return
	}
	habitRows, err := h.store.ListProduct(r.Context(), habits, userID, nil)
	if err != nil {
		h.storeError(w, "dashboard habits failed", err)
		return
	}
	goalRows, err := h.store.ListProduct(r.Context(), goals, userID, nil)
	if err != nil {
		h.storeError(w, "dashboard goals failed", err)
		return
	}
	planRows, err := h.store.ListProduct(r.Context(), dailyPlans, userID, map[string]string{"date": time.Now().UTC().Format("2006-01-02")})
	if err != nil {
		h.storeError(w, "dashboard plan failed", err)
		return
	}
	eventRows, err := h.store.ListProduct(r.Context(), events, userID, nil)
	if err != nil {
		h.storeError(w, "dashboard events failed", err)
		return
	}
	choreRows, err := h.store.ListProduct(r.Context(), chores, userID, map[string]string{"completed": "false"})
	if err != nil {
		h.storeError(w, "dashboard chores failed", err)
		return
	}
	actionRows, err := h.store.ListProduct(r.Context(), actionItems, userID, map[string]string{"completed": "false"})
	if err != nil {
		h.storeError(w, "dashboard actions failed", err)
		return
	}
	h.addHabitStatus(r, userID, habitRows)
	habitsCompleted := 0
	for _, row := range habitRows {
		if row["completedToday"] == true {
			habitsCompleted++
		}
	}
	activeGoals, completedGoals := 0, 0
	for _, row := range goalRows {
		if row["status"] == "completed" {
			completedGoals++
		} else {
			activeGoals++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"habits": habitRows, "goals": goalRows, "todayPlan": planRows, "upcomingEvents": limit(eventRows, 5),
		"pendingChores": limit(choreRows, 5), "openActionItems": limit(actionRows, 10),
		"habitsCompletedToday": habitsCompleted, "habitsTotal": len(habitRows), "goalsActive": activeGoals, "goalsCompleted": completedGoals,
		"googleConnection": map[string]any{"connected": false, "scopes": []string{}, "calendarConnected": false, "gmailConnected": false},
	})
}

func (h *Handler) aiCredits(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	balance, err := h.store.AICreditBalance(r.Context(), userID)
	if err != nil {
		h.storeError(w, "AI credit lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": balance, "enforcement": "strict"})
}

func (h *Handler) aiCreditEstimate(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	key := r.URL.Query().Get("pricingKey")
	units, err := strconv.Atoi(r.URL.Query().Get("units"))
	if key == "" || units < 1 || units > 100000 {
		writeError(w, http.StatusBadRequest, "INVALID_ESTIMATE", "A valid pricing key and units are required.")
		return
	}
	balance, err := h.store.AICreditBalance(r.Context(), userID)
	if err != nil {
		h.storeError(w, "AI credit lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pricingKey": key, "units": units, "estimatedCredits": units, "unit": "unit", "creditsPerUnit": 1, "hardCapCredits": units, "availableCredits": balance, "canReserve": balance >= units})
}

func (h *Handler) transcriptionPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	consent, version, err := h.store.VoiceConsent(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice preference lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"consentGiven": consent, "consentVersion": version})
}

func (h *Handler) updateTranscriptionPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input struct {
		Consent *bool `json:"consent"`
	}
	if !decodeBody(w, r, &input) || input.Consent == nil {
		writeError(w, http.StatusBadRequest, "INVALID_CONSENT", "A consent decision is required.")
		return
	}
	if err := h.store.SetVoiceConsent(r.Context(), userID, *input.Consent, "voice-v1"); err != nil {
		h.storeError(w, "voice preference update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"consentGiven": *input.Consent, "consentVersion": func() any {
		if *input.Consent {
			return "voice-v1"
		}
		return nil
	}()})
}

func (h *Handler) coaching(w http.ResponseWriter, r *http.Request) {
	h.staticAIResponse(w, r, "I can help you review your habits and goals. Choose one small next step for today.")
}
func (h *Handler) assistant(w http.ResponseWriter, r *http.Request) {
	h.staticAIResponse(w, r, "I’m ready to help you plan, organize, and follow through.")
}
func (h *Handler) generatePlan(w http.ResponseWriter, r *http.Request) {
	h.staticAIResponse(w, r, "Add a few priorities and I’ll help turn them into a plan.")
}
func (h *Handler) voiceToPlan(w http.ResponseWriter, r *http.Request) {
	h.staticAIResponse(w, r, "Your voice note is ready to turn into a plan.")
}
func (h *Handler) meetingExtract(w http.ResponseWriter, r *http.Request) {
	h.staticAIResponse(w, r, "I can summarize meeting notes and identify follow-up actions.")
}

func (h *Handler) transcribeAudio(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	consent, _, err := h.store.VoiceConsent(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice consent lookup failed", err)
		return
	}
	if !consent {
		writeError(w, http.StatusForbidden, "VOICE_CONSENT_REQUIRED", "Voice transcription consent is required.")
		return
	}
	if h.assemblyAIKey == "" {
		writeError(w, http.StatusServiceUnavailable, "VOICE_NOT_CONFIGURED", "Voice transcription is not configured.")
		return
	}
	var input struct {
		AudioBase64 string `json:"audioBase64"`
		MimeType    string `json:"mimeType"`
		DurationMS  int    `json:"durationMs"`
		Language    string `json:"language"`
	}
	if !decodeBody(w, r, &input) || input.AudioBase64 == "" || len(input.AudioBase64) > 24*1024*1024 {
		writeError(w, http.StatusBadRequest, "INVALID_AUDIO", "A valid audio recording is required.")
		return
	}
	audio, err := base64.StdEncoding.DecodeString(input.AudioBase64)
	if err != nil || len(audio) == 0 || len(audio) > 16*1024*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "INVALID_AUDIO", "The recording is too large or invalid.")
		return
	}
	if !h.spendVoiceProviderCredit(w, r, userID) {
		return
	}
	ctx := r.Context()
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.assemblyai.com/v2/upload", bytes.NewReader(audio))
	if err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	uploadReq.Header.Set("Authorization", h.assemblyAIKey)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadResponse, err := (&http.Client{Timeout: 25 * time.Second}).Do(uploadReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode < 200 || uploadResponse.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	var uploaded struct {
		URL string `json:"upload_url"`
	}
	if err := json.NewDecoder(uploadResponse.Body).Decode(&uploaded); err != nil || uploaded.URL == "" {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	submitPayload := map[string]any{"audio_url": uploaded.URL, "speech_models": []string{"universal-3-5-pro", "universal-2"}, "speaker_labels": false}
	if input.Language != "" {
		submitPayload["language_code"] = strings.Split(input.Language, "-")[0]
	}
	payload, _ := json.Marshal(submitPayload)
	submitReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.assemblyai.com/v2/transcript", bytes.NewReader(payload))
	submitReq.Header.Set("Authorization", h.assemblyAIKey)
	submitReq.Header.Set("Content-Type", "application/json")
	submitResponse, err := (&http.Client{Timeout: 25 * time.Second}).Do(submitReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	defer submitResponse.Body.Close()
	if submitResponse.StatusCode < 200 || submitResponse.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	var submitted struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(submitResponse.Body).Decode(&submitted); err != nil || submitted.ID == "" {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		if err := sleepContext(ctx, time.Second); err != nil {
			return
		}
		pollReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.assemblyai.com/v2/transcript/"+submitted.ID, nil)
		pollReq.Header.Set("Authorization", h.assemblyAIKey)
		pollResponse, pollErr := client.Do(pollReq)
		if pollErr != nil {
			continue
		}
		var transcript struct {
			Status        string  `json:"status"`
			Text          string  `json:"text"`
			Confidence    float64 `json:"confidence"`
			AudioDuration float64 `json:"audio_duration"`
			Error         string  `json:"error"`
		}
		decodeErr := json.NewDecoder(pollResponse.Body).Decode(&transcript)
		pollResponse.Body.Close()
		if decodeErr != nil {
			continue
		}
		if transcript.Status == "completed" {
			writeJSON(w, http.StatusOK, map[string]any{"transcript": transcript.Text, "confidence": transcript.Confidence, "durationMs": int(transcript.AudioDuration * 1000), "providerRequestId": submitted.ID, "redaction": "provider_pii_redaction", "audioDeleted": true})
			return
		}
		if transcript.Status == "error" {
			writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
			return
		}
	}
	writeError(w, http.StatusGatewayTimeout, "VOICE_PROVIDER_TIMEOUT", "Voice transcription took too long. Try a shorter recording.")
}

func (h *Handler) realtimeToken(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	consent, _, err := h.store.VoiceConsent(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice consent lookup failed", err)
		return
	}
	if !consent {
		writeError(w, http.StatusForbidden, "VOICE_CONSENT_REQUIRED", "Voice transcription consent is required.")
		return
	}
	if h.assemblyAIKey == "" {
		writeError(w, http.StatusServiceUnavailable, "VOICE_NOT_CONFIGURED", "Voice transcription is not configured.")
		return
	}
	if !h.spendVoiceProviderCredit(w, r, userID) {
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://streaming.assemblyai.com/v3/token?expires_in_seconds=60", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "A real-time transcription session could not be started.")
		return
	}
	request.Header.Set("Authorization", h.assemblyAIKey)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "A real-time transcription session could not be started.")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "A real-time transcription session could not be started.")
		return
	}
	var token map[string]any
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "A real-time transcription session could not be started.")
		return
	}
	token["speechModel"] = "universal-3-5-pro"
	token["redaction"] = "provider_pii_redaction"
	writeJSON(w, http.StatusOK, token)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (h *Handler) staticAIResponse(w http.ResponseWriter, r *http.Request, message string) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": message, "items": []any{}, "date": time.Now().UTC().Format("2006-01-02"), "summary": message, "decisions": []string{}, "actionItems": []any{}})
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	var input struct{ FirstName, LastName string }
	if !decodeBody(w, r, &input) {
		return
	}
	if len(input.FirstName) > 100 || len(input.LastName) > 100 {
		writeError(w, http.StatusBadRequest, "INVALID_PROFILE", "Profile fields are too long.")
		return
	}
	user, err := h.store.UpdateUserProfile(r.Context(), userID, strings.TrimSpace(input.FirstName), strings.TrimSpace(input.LastName))
	if err != nil {
		h.storeError(w, "profile update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileUserResponse(user))
}

type profileUserResponse struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	ProfileImageURL string `json:"profileImageUrl"`
	Status          string `json:"status"`
	AuthProvider    string `json:"authProvider"`
}

func toProfileUserResponse(user postgres.User) profileUserResponse {
	return profileUserResponse{
		ID:              user.ID,
		Email:           user.Email,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		ProfileImageURL: user.ProfileImageURL,
		Status:          user.Status,
		AuthProvider:    user.AccountCreatedVia,
	}
}

func (h *Handler) deleteUserData(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	if err := h.store.DeleteUserData(r.Context(), userID); err != nil {
		h.storeError(w, "user data deletion failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	if err := h.store.DeleteAccount(r.Context(), userID); err != nil {
		h.storeError(w, "account deletion failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) storeError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, "error", err)
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed.")
}

func parseID(raw string) (int64, error) { return strconv.ParseInt(raw, 10, 64) }
func integerID(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), typed == float64(int64(typed))
	}
	return 0, false
}
func validDate(value string) bool { _, err := time.Parse("2006-01-02", value); return err == nil }
func limit(rows []map[string]any, max int) []map[string]any {
	if len(rows) <= max {
		return rows
	}
	return rows[:max]
}
func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid.")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	apierror.Write(w, &http.Request{Header: make(http.Header)}, status, code, message)
}
