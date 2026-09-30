package product

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	store               *postgres.Store
	logger              *slog.Logger
	sessionCookieName   string
	canonicalOrigin     string
	assemblyAI          assemblyAIProvider
	adminEmails         map[string]struct{}
	realtimeLimiter     *realtimeSessionLimiter
	authRateLimitSecret string
	realtimeIdleTimeout time.Duration
	assistantPlanner    assistantPlanner
	realtimeMeters      sync.Map
}

type voiceCreditReservation struct {
	ID                string
	Mode              string
	Model             string
	ReservedUsdMicros int64
	PolicyVersion     int
	RateCard          postgres.USDRateCard
}

func NewHandler(cfg config.Config, store *postgres.Store, logger *slog.Logger, sessionCookieName string) *Handler {
	return newHandler(
		cfg,
		store,
		logger,
		sessionCookieName,
		newAssemblyAIClient(
			cfg.AssemblyAIKey,
			assemblyAIRESTBaseURL,
			assemblyAIRealtimeTokenBaseURL,
			assemblyAIRealtimeWebsocketURL,
			&http.Client{Timeout: 25 * time.Second},
			nil,
		),
		cfg.CanonicalOrigin,
	)
}

func newHandler(
	cfg config.Config,
	store *postgres.Store,
	logger *slog.Logger,
	sessionCookieName string,
	assemblyAI assemblyAIProvider,
	canonicalOrigin string,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		store:               store,
		logger:              logger,
		sessionCookieName:   sessionCookieName,
		assemblyAI:          assemblyAI,
		adminEmails:         cfg.AdminEmails,
		canonicalOrigin:     canonicalOrigin,
		realtimeLimiter:     newRealtimeSessionLimiter(),
		authRateLimitSecret: cfg.AuthRateLimitHMACSecret,
		realtimeIdleTimeout: realtimeClientIdleTimeout,
		assistantPlanner:    newOpenAIAssistantPlanner(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL),
	}
}

func (h *Handler) reserveVoiceProviderCredit(w http.ResponseWriter, r *http.Request, userID, mode string) (voiceCreditReservation, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	expectedVersion, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-AI-Credit-Policy-Version")))
	if err != nil {
		expectedVersion = 0
	}
	result, failure := h.reserveVoiceProviderCreditRequest(r.Context(), userID, mode, key, expectedVersion)
	if failure == nil {
		return result, true
	}
	if failure.err != nil {
		h.storeError(w, failure.operation, failure.err)
		return voiceCreditReservation{}, false
	}
	writeError(w, failure.status, failure.code, failure.message)
	return voiceCreditReservation{}, false
}

type voiceCreditReservationFailure struct {
	status    int
	code      string
	message   string
	operation string
	err       error
}

func (h *Handler) reserveVoiceProviderCreditRequest(
	ctx context.Context,
	userID string,
	mode string,
	key string,
	expectedVersion int,
) (voiceCreditReservation, *voiceCreditReservationFailure) {
	var result voiceCreditReservation
	if key == "" || len(key) > 200 {
		return result, &voiceCreditReservationFailure{
			status: http.StatusBadRequest, code: "IDEMPOTENCY_KEY_REQUIRED",
			message: "An Idempotency-Key header is required.",
		}
	}
	if expectedVersion < 1 {
		return result, &voiceCreditReservationFailure{
			status: http.StatusBadRequest, code: "CREDIT_POLICY_VERSION_REQUIRED",
			message: "Refresh the credit estimate before starting this voice operation.",
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return result, &voiceCreditReservationFailure{
			operation: "AI reservation id generation failed", err: err,
		}
	}
	id := "voice-" + fmt.Sprintf("%x", raw)
	p, err := h.store.USDPolicy(ctx)
	if err != nil {
		return result, &voiceCreditReservationFailure{operation: "AI policy lookup failed", err: err}
	}
	if expectedVersion != p.Version {
		return result, &voiceCreditReservationFailure{
			status: http.StatusConflict, code: "AI_POLICY_CHANGED",
			message: "The AI credit policy changed. Refresh the estimate and try again.",
		}
	}
	model := assemblyAIRealtimeSpeechModel
	card, known := p.RateCards["assemblyai:"+mode+":"+model]
	if !known || card.Meter != "hour" || card.UsdMicrosPerHour <= 0 || mode == "" {
		return result, &voiceCreditReservationFailure{
			status: http.StatusServiceUnavailable, code: "AI_POLICY_INVALID",
			message: "AI credit policy is unavailable.",
		}
	}
	maxMS := int64(maxVoiceRecordingDurationMS)
	if mode == "realtime" {
		maxMS = int64(assemblyAIRealtimeMaxSessionDurationSeconds) * 1000
	}
	base, ok := ceilMulDiv(card.UsdMicrosPerHour, maxMS, 3600000)
	if !ok {
		return result, &voiceCreditReservationFailure{
			status: http.StatusServiceUnavailable, code: "AI_POLICY_INVALID",
			message: "AI credit policy is unavailable.",
		}
	}
	cap, ok := applyMargin(base, p.OverrunMarginPercent)
	if !ok {
		return result, &voiceCreditReservationFailure{status: http.StatusServiceUnavailable, code: "AI_POLICY_INVALID", message: "AI credit policy is unavailable."}
	}
	snapshot, _ := json.Marshal(card)
	reservation, created, err := h.store.ReserveUSD(ctx, id, userID, "voice", "assemblyai", mode, model, key, cap, 300, p.Version, snapshot)
	if err != nil {
		return result, &voiceCreditReservationFailure{operation: "AI credit reservation failed", err: err}
	}
	if !created {
		return result, &voiceCreditReservationFailure{
			status: http.StatusConflict, code: "CREDIT_RESERVATION_CONFLICT",
			message: "This operation is already being processed.",
		}
	}
	claimed, claimErr := h.store.ClaimUSDReservation(ctx, reservation.ID, userID)
	if claimErr != nil || !claimed {
		if releaseErr := h.store.ReleaseUSDReservation(context.Background(), reservation.ID, userID, reservation.ID+":claim-failed"); releaseErr != nil {
			h.logger.Error("voice credit reservation release failed", "reservation_id", reservation.ID, "error", releaseErr)
		}
		return result, &voiceCreditReservationFailure{
			status: http.StatusConflict, code: "CREDIT_RESERVATION_CONFLICT",
			message: "This operation is already being processed.",
		}
	}
	return voiceCreditReservation{
		ID: reservation.ID, Mode: mode, Model: model, ReservedUsdMicros: reservation.ReservedUsdMicros,
		PolicyVersion: reservation.PolicyVersion, RateCard: card,
	}, nil
}

func ceilMulDiv(a, b, divisor int64) (int64, bool) {
	if a < 0 || b < 0 || divisor <= 0 || (a != 0 && b > math.MaxInt64/a) {
		return 0, false
	}
	product := a * b
	if product > math.MaxInt64-(divisor-1) {
		return 0, false
	}
	return (product + divisor - 1) / divisor, true
}

func applyMargin(base int64, margin int) (int64, bool) {
	if base < 0 || margin < 0 || margin > 100 {
		return 0, false
	}
	return ceilMulDiv(base, int64(100+margin), 100)
}

func (h *Handler) settleVoiceProviderCredit(w http.ResponseWriter, userID string, reservation voiceCreditReservation, durationMS int64, source string) (voiceCreditReceipt, bool) {
	actual, ok := ceilMulDiv(reservation.RateCard.UsdMicrosPerHour, durationMS, 3600000)
	if !ok || actual < 1 || actual > reservation.ReservedUsdMicros {
		writeError(w, http.StatusBadGateway, "VOICE_USAGE_UNAVAILABLE", "The provider did not return valid usage for this operation.")
		return voiceCreditReceipt{}, false
	}
	payload, _ := json.Marshal(map[string]string{"meterSource": source})
	evidence := postgres.USDEvidence{
		UsageUnit: "milliseconds", UsageUnits: durationMS, DurationMs: durationMS,
		ProviderRequestId: source, Payload: payload,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.store.RecordUSDEvidence(ctx, reservation.ID, userID, "assemblyai", reservation.Mode, reservation.Model, reservation.ID+":usage", evidence); err != nil {
		h.storeError(w, "provider usage recording failed", err)
		return voiceCreditReceipt{}, false
	}
	if err := h.store.SettleUSDReservation(ctx, reservation.ID, userID, reservation.ID+":settle", actual); err != nil {
		h.storeError(w, "USD settlement failed", err)
		return voiceCreditReceipt{}, false
	}
	usage, err := h.store.USDUsage(ctx, userID)
	if err != nil {
		h.storeError(w, "USD usage lookup failed", err)
		return voiceCreditReceipt{}, false
	}
	return voiceCreditReceipt{ID: reservation.ID, ReservationID: reservation.ID, OperationType: "voice", Provider: "assemblyai", Mode: reservation.Mode, Status: "settled", ReservedUsdMicros: reservation.ReservedUsdMicros, SettledUsdMicros: actual, RefundedUsdMicros: reservation.ReservedUsdMicros - actual, BalanceUsdMicros: usage.BalanceUsdMicros, UsageUnit: "milliseconds", UsageUnits: durationMS, PolicyVersion: reservation.PolicyVersion}, true
}

// settleVoiceProviderCreditRecord is kept for the legacy public websocket
// adapter. New realtime sessions settle only after provider metering arrives.
func (h *Handler) settleVoiceProviderCreditRecord(userID string, reservation voiceCreditReservation) (voiceCreditReceipt, error) {
	return voiceCreditReceipt{}, errors.New("provider usage is required before USD settlement")
}

func (h *Handler) settleVoiceProviderCreditRecordWithUsage(userID string, reservation voiceCreditReservation, durationMS int64, source string) (voiceCreditReceipt, error) {
	actual, ok := ceilMulDiv(reservation.RateCard.UsdMicrosPerHour, durationMS, 3600000)
	if !ok || actual < 1 || actual > reservation.ReservedUsdMicros {
		return voiceCreditReceipt{}, errors.New("invalid provider usage")
	}
	payload, _ := json.Marshal(map[string]string{"meterSource": source})
	evidence := postgres.USDEvidence{UsageUnit: "milliseconds", UsageUnits: durationMS, DurationMs: durationMS, Payload: payload}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.store.RecordUSDEvidence(ctx, reservation.ID, userID, "assemblyai", reservation.Mode, reservation.Model, reservation.ID+":usage", evidence); err != nil {
		return voiceCreditReceipt{}, err
	}
	if err := h.store.SettleUSDReservation(ctx, reservation.ID, userID, reservation.ID+":settle", actual); err != nil {
		return voiceCreditReceipt{}, err
	}
	usage, err := h.store.USDUsage(ctx, userID)
	if err != nil {
		return voiceCreditReceipt{}, err
	}
	return voiceCreditReceipt{ID: reservation.ID, ReservationID: reservation.ID, OperationType: "voice", Provider: "assemblyai", Mode: reservation.Mode, Status: "settled", ReservedUsdMicros: reservation.ReservedUsdMicros, SettledUsdMicros: actual, RefundedUsdMicros: reservation.ReservedUsdMicros - actual, BalanceUsdMicros: usage.BalanceUsdMicros, UsageUnit: "milliseconds", UsageUnits: durationMS, PolicyVersion: reservation.PolicyVersion}, nil
}

func durationSecondsMillis(seconds float64) (int64, bool) {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, false
	}
	text := strconv.FormatFloat(seconds, 'f', 6, 64)
	parts := strings.SplitN(text, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	fraction := "000"
	roundUp := false
	if len(parts) == 2 {
		raw := parts[1]
		fraction = (raw + "000")[:3]
		roundUp = len(raw) > 3 && strings.Trim(raw[3:], "0") != ""
	}
	msFraction, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, false
	}
	if roundUp {
		msFraction++
		if msFraction == 1000 {
			whole++
			msFraction = 0
		}
	}
	ms := whole*1000 + msFraction
	return ms, ms > 0 && ms <= int64(assemblyAIRealtimeMaxSessionDurationSeconds)*1000
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
	mux.HandleFunc("GET /api/ai/credits/usage", h.aiCreditUsage)
	mux.HandleFunc("GET /api/ai/credits/estimate", h.aiCreditEstimate)
	mux.HandleFunc("GET /api/admin/ai-credit-policy", h.adminPolicy)
	mux.HandleFunc("PATCH /api/admin/ai-credit-policy", h.updateAdminPolicy)
	mux.HandleFunc("POST /api/admin/ai-credit-adjustments", h.adminAdjustment)
	mux.HandleFunc("POST /api/admin/ai-credit-adjustments/{id}/reverse", h.adminReverseAdjustment)
	mux.HandleFunc("POST /api/admin/ai-credit-grants/{id}/reverse", h.adminReverseGrant)
	mux.HandleFunc("POST /api/admin/ai-credit-reservations/{id}/refund", h.adminRefundReservation)
	mux.HandleFunc("GET /api/admin/ai-credit-usage/{userId}", h.adminCreditUsage)
	mux.HandleFunc("GET /api/ai/transcription-preferences", h.transcriptionPreferences)
	mux.HandleFunc("PATCH /api/ai/transcription-preferences", h.updateTranscriptionPreferences)
	mux.HandleFunc("POST /api/ai/coaching", h.coaching)
	mux.HandleFunc("POST /api/ai/assistant", h.assistant)
	mux.HandleFunc("GET /api/ai/assistant/conversations/current", h.getAssistantConversation)
	mux.HandleFunc("POST /api/ai/assistant/runs", h.createAssistantRun)
	mux.HandleFunc("GET /api/ai/assistant/runs/{id}", h.getAssistantRun)
	mux.HandleFunc("POST /api/ai/assistant/runs/{id}/confirm", h.confirmAssistantRun)
	mux.HandleFunc("POST /api/ai/assistant/runs/{id}/cancel", h.cancelAssistantRun)
	mux.HandleFunc("POST /api/ai/generate-plan", h.generatePlan)
	mux.HandleFunc("POST /api/ai/voice-to-plan", h.voiceToPlan)
	mux.HandleFunc("POST /api/ai/meeting-extract", h.meetingExtract)
	mux.HandleFunc("POST /api/ai/transcribe-audio", h.transcribeAudio)
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
	scopeType := resourceType
	if resourceID == "" {
		scopeType = ""
	}
	decision, err := h.store.Authorize(r.Context(), policy.Input{
		ActorUserID: userID, WorkspaceID: postgres.DefaultWorkspaceID(userID), ResourceType: scopeType, ResourceID: resourceID, Action: action,
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

func writeCreditSessionError(w http.ResponseWriter, status int) {
	switch status {
	case http.StatusUnauthorized:
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized.")
	case http.StatusForbidden:
		writeError(w, http.StatusForbidden, "MFA_REQUIRED", "Complete multi-factor verification to continue.")
	default:
		writeError(w, http.StatusServiceUnavailable, "AUTHENTICATION_UNAVAILABLE", "Authentication is temporarily unavailable.")
	}
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
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	usage, err := h.store.USDUsage(r.Context(), userID)
	if err != nil {
		h.storeError(w, "AI credit lookup failed", err)
		return
	}
	p, err := h.store.USDPolicy(r.Context())
	if err != nil {
		h.storeError(w, "AI policy lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"currency": "USD", "balanceUsdMicros": usage.BalanceUsdMicros, "grantedUsdMicros": usage.GrantedUsdMicros, "adjustmentsUsdMicros": usage.AdjustmentsUsdMicros, "reservedUsdMicros": usage.ReservedUsdMicros, "spentUsdMicros": usage.SpentUsdMicros, "refundedUsdMicros": usage.RefundedUsdMicros, "policyVersion": p.Version, "canManage": h.isCreditAdmin(r, userID), "enforcement": "strict"})
}

func (h *Handler) aiCreditUsage(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	usage, err := h.store.USDUsage(r.Context(), userID)
	if err != nil {
		h.storeError(w, "AI credit usage lookup failed", err)
		return
	}
	recent, err := h.store.USDRecent(r.Context(), userID, 25)
	if err != nil {
		h.storeError(w, "AI credit history lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"currency": "USD", "usage": usage, "reservations": recent["reservations"], "events": recent["events"], "adjustments": recent["adjustments"], "grants": recent["grants"],
	})
}

func (h *Handler) adminCreditUsage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireCreditAdmin(w, r); !ok {
		return
	}
	target := strings.TrimSpace(r.PathValue("userId"))
	if target == "" {
		writeError(w, http.StatusBadRequest, "INVALID_USER", "A valid user is required.")
		return
	}
	usage, err := h.store.USDUsage(r.Context(), target)
	if err != nil {
		h.storeError(w, "AI credit usage lookup failed", err)
		return
	}
	recent, err := h.store.USDRecent(r.Context(), target, 100)
	if err != nil {
		h.storeError(w, "AI credit history lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"currency": "USD", "usage": usage, "reservations": recent["reservations"], "events": recent["events"], "adjustments": recent["adjustments"], "grants": recent["grants"],
	})
}

func (h *Handler) isCreditAdmin(r *http.Request, userID string) bool {
	if userID == "" || len(h.adminEmails) == 0 {
		return false
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil || user.Status != "active" || strings.TrimSpace(user.Email) == "" || user.EmailVerifiedAt == nil {
		return false
	}
	_, ok := h.adminEmails[strings.ToLower(strings.TrimSpace(user.Email))]
	return ok
}

func (h *Handler) requireCreditAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return "", false
	}
	if !h.isCreditAdmin(r, userID) {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You are not allowed to perform this action.")
		return "", false
	}
	return userID, true
}

func (h *Handler) adminPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireCreditAdmin(w, r); !ok {
		return
	}
	result, err := h.store.USDPolicy(r.Context())
	if err != nil {
		h.storeError(w, "AI policy lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) updateAdminPolicy(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireCreditAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		postgres.USDPolicy
		ExpectedVersion int    `json:"expectedVersion"`
		ChangeReason    string `json:"changeReason"`
	}
	if !decodeBody(w, r, &input) || input.ExpectedVersion < 1 || len(strings.TrimSpace(input.ChangeReason)) == 0 || len(input.ChangeReason) > 160 {
		writeError(w, http.StatusBadRequest, "INVALID_POLICY", "A valid policy version is required.")
		return
	}
	input.Version = input.ExpectedVersion + 1
	input.ChangeReason = strings.TrimSpace(input.ChangeReason)
	input.USDPolicy.ChangeReason = input.ChangeReason
	result, err := h.store.UpdateUSDPolicy(r.Context(), input.ExpectedVersion, input.USDPolicy, actor)
	if err != nil && strings.Contains(err.Error(), "policy version conflict") {
		writeError(w, http.StatusConflict, "POLICY_VERSION_CONFLICT", "The policy changed; reload and try again.")
		return
	}
	if err != nil {
		h.storeError(w, "AI policy update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) adminAdjustment(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireCreditAdmin(w, r)
	if !ok {
		return
	}
	var input struct {
		UserID         string `json:"userId"`
		Amount         int64  `json:"amountUsdMicros"`
		Reason         string `json:"reason"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !decodeBody(w, r, &input) || strings.TrimSpace(input.UserID) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ADJUSTMENT", "A valid user and adjustment are required.")
		return
	}
	if err := h.store.AddUSDAdjustment(r.Context(), input.UserID, actor, input.Amount, input.Reason, input.IdempotencyKey); err != nil {
		if strings.Contains(err.Error(), "idempotency") {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "This request key was already used.")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			writeError(w, http.StatusBadRequest, "INVALID_ADJUSTMENT", "The adjustment is invalid.")
			return
		}
		h.storeError(w, "AI adjustment failed", err)
		return
	}
	usage, err := h.store.USDUsage(r.Context(), input.UserID)
	if err != nil {
		h.storeError(w, "AI usage lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *Handler) adminReverseAdjustment(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireCreditAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ENTRY", "A valid entry is required.")
		return
	}
	var input struct {
		UserID         string `json:"userId"`
		IdempotencyKey string `json:"idempotencyKey"`
		Reason         string `json:"reason"`
	}
	if !decodeBody(w, r, &input) || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REVERSAL", "A valid reversal is required.")
		return
	}
	if err := h.store.ReverseUSDAdjustment(r.Context(), input.UserID, actor, id, input.IdempotencyKey, input.Reason); err != nil {
		if strings.Contains(err.Error(), "already reversed") {
			writeError(w, http.StatusConflict, "ALREADY_REVERSED", "This entry was already reversed.")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			writeError(w, http.StatusBadRequest, "INVALID_REVERSAL", "The reversal is invalid.")
			return
		}
		h.storeError(w, "AI reversal failed", err)
		return
	}
	usage, err := h.store.USDUsage(r.Context(), input.UserID)
	if err != nil {
		h.storeError(w, "AI usage lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *Handler) adminReverseGrant(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireCreditAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ENTRY", "A valid grant is required.")
		return
	}
	var input struct {
		UserID         string `json:"userId"`
		IdempotencyKey string `json:"idempotencyKey"`
		Reason         string `json:"reason"`
	}
	if !decodeBody(w, r, &input) || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Reason) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REVERSAL", "A valid reversal is required.")
		return
	}
	if err := h.store.ReverseUSDGrant(r.Context(), input.UserID, actor, id, input.IdempotencyKey, input.Reason); err != nil {
		if strings.Contains(err.Error(), "already reversed") {
			writeError(w, http.StatusConflict, "ALREADY_REVERSED", "This grant was already reversed.")
			return
		}
		if strings.Contains(err.Error(), "idempotency") {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "This request key was already used.")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			writeError(w, http.StatusBadRequest, "INVALID_REVERSAL", "The reversal is invalid.")
			return
		}
		h.storeError(w, "AI grant reversal failed", err)
		return
	}
	usage, err := h.store.USDUsage(r.Context(), input.UserID)
	if err != nil {
		h.storeError(w, "AI usage lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *Handler) adminRefundReservation(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireCreditAdmin(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_RESERVATION", "A valid reservation is required.")
		return
	}
	var input struct {
		UserID         string `json:"userId"`
		Amount         int64  `json:"amountUsdMicros"`
		Reason         string `json:"reason"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if !decodeBody(w, r, &input) || strings.TrimSpace(input.UserID) == "" || input.Amount < 1 || len(input.Reason) > 160 || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REFUND", "A valid refund is required.")
		return
	}
	if err := h.store.RefundUSDReservation(r.Context(), id, input.UserID, actor, input.IdempotencyKey, input.Reason, input.Amount); err != nil {
		if strings.Contains(err.Error(), "idempotency") {
			writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "This request key was already used.")
			return
		}
		if strings.Contains(err.Error(), "exceeds") {
			writeError(w, http.StatusBadRequest, "INVALID_REFUND", "The refund exceeds settled credits.")
			return
		}
		h.storeError(w, "AI refund failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refunded", "reason": input.Reason})
}

func usdEstimateRateCardKey(pricingKey string) (string, bool) {
	switch pricingKey {
	case "assistant":
		return "openai:assistant:" + assistantPlannerModel, true
	case "voice.recorded":
		return "assemblyai:recorded:" + assemblyAIRealtimeSpeechModel, true
	case "voice.realtime":
		return "assemblyai:realtime:" + assemblyAIRealtimeSpeechModel, true
	default:
		return pricingKey, false
	}
}

func estimateAssistantRequestUsdMicros(card postgres.USDRateCard) (int64, bool) {
	if card.Meter != "tokens" || card.InputUsdMicrosPerMillion <= 0 ||
		card.OutputUsdMicrosPerMillion <= 0 {
		return 0, false
	}
	inputMicros, ok := ceilMulDiv(
		card.InputUsdMicrosPerMillion,
		assistantPlannerInputTokenReservationCap,
		postgres.USDMicroUnitsPerDollar,
	)
	if !ok {
		return 0, false
	}
	outputMicros, ok := ceilMulDiv(
		card.OutputUsdMicrosPerMillion,
		assistantPlannerOutputTokenReservationCap,
		postgres.USDMicroUnitsPerDollar,
	)
	if !ok || inputMicros > math.MaxInt64-outputMicros {
		return 0, false
	}
	return inputMicros + outputMicros, true
}

func (h *Handler) aiCreditEstimate(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	key := r.URL.Query().Get("pricingKey")
	units, unitsErr := strconv.Atoi(r.URL.Query().Get("units"))
	if key == "" || unitsErr != nil || units < 1 || units > 10000000 ||
		(key == "assistant" && units != 1) {
		writeError(w, http.StatusBadRequest, "INVALID_ESTIMATE", "A valid pricing key and units are required.")
		return
	}
	p, policyErr := h.store.USDPolicy(r.Context())
	if policyErr != nil {
		h.storeError(w, "AI policy lookup failed", policyErr)
		return
	}
	rateCardKey, publicPricingKey := usdEstimateRateCardKey(key)
	card, known := p.RateCards[rateCardKey]
	if !known && publicPricingKey {
		writeError(w, http.StatusServiceUnavailable, "AI_POLICY_INVALID", "AI credit policy is unavailable.")
		return
	}
	if !known {
		writeError(w, http.StatusBadRequest, "INVALID_ESTIMATE", "A valid pricing key and units are required.")
		return
	}
	var estimated int64
	var ok bool
	unit := card.Meter
	switch {
	case key == "assistant":
		estimated, ok = estimateAssistantRequestUsdMicros(card)
		unit = "request"
	case card.Meter == "hour":
		estimated, ok = ceilMulDiv(card.UsdMicrosPerHour, int64(units), 3600)
		unit = "seconds"
	case card.Meter == "input_tokens":
		estimated, ok = ceilMulDiv(card.InputUsdMicrosPerMillion, int64(units), postgres.USDMicroUnitsPerDollar)
	case card.Meter == "output_tokens":
		estimated, ok = ceilMulDiv(card.OutputUsdMicrosPerMillion, int64(units), postgres.USDMicroUnitsPerDollar)
	case card.Meter == "tokens":
		estimated, ok = ceilMulDiv(card.InputUsdMicrosPerMillion+card.OutputUsdMicrosPerMillion, int64(units), 1000000)
		unit = "tokens"
	}
	if !ok || estimated < 1 {
		writeError(w, http.StatusServiceUnavailable, "AI_POLICY_INVALID", "AI credit policy is unavailable.")
		return
	}
	hardCap, ok := applyMargin(estimated, p.OverrunMarginPercent)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "AI_POLICY_INVALID", "AI credit policy is unavailable.")
		return
	}
	usage, err := h.store.USDUsage(r.Context(), userID)
	if err != nil {
		h.storeError(w, "USD usage lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"currency": "USD", "pricingKey": key, "units": units, "estimatedUsdMicros": estimated, "hardCapUsdMicros": hardCap, "availableUsdMicros": usage.BalanceUsdMicros, "policyVersion": p.Version, "canReserve": usage.BalanceUsdMicros >= hardCap, "overrunMarginPercent": p.OverrunMarginPercent, "unit": unit})
}

func (h *Handler) transcriptionPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
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
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input struct {
		Consent *bool `json:"consent"`
	}
	if !decodeBody(w, r, &input) || input.Consent == nil {
		writeError(w, http.StatusBadRequest, "INVALID_CONSENT", "A consent decision is required.")
		return
	}
	if err := h.store.SetVoiceConsent(r.Context(), userID, *input.Consent, postgres.VoiceConsentVersion); err != nil {
		h.storeError(w, "voice preference update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"consentGiven": *input.Consent, "consentVersion": func() any {
		if *input.Consent {
			return postgres.VoiceConsentVersion
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
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(160 * time.Second)); err != nil {
		writeError(w, http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Voice transcription is temporarily unavailable.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	consent, consentVersion, err := h.store.VoiceConsent(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice consent lookup failed", err)
		return
	}
	if !consent || consentVersion != postgres.VoiceConsentVersion {
		writeError(w, http.StatusForbidden, "VOICE_CONSENT_REQUIRED", "Voice transcription consent is required.")
		return
	}
	if h.assemblyAI == nil || !h.assemblyAI.Configured() {
		writeError(w, http.StatusServiceUnavailable, "VOICE_NOT_CONFIGURED", "Voice transcription is not configured.")
		return
	}
	var input struct {
		AudioBase64 string `json:"audioBase64"`
		MimeType    string `json:"mimeType"`
		DurationMS  int    `json:"durationMs"`
		Language    string `json:"language"`
	}
	if !decodeBodyLimit(w, r, &input, 25*1024*1024) {
		return
	}
	if input.AudioBase64 == "" {
		writeError(w, http.StatusBadRequest, "INVALID_AUDIO", "A valid audio recording is required.")
		return
	}
	if len(input.AudioBase64) > maxVoiceAudioBase64Characters {
		writeError(w, http.StatusRequestEntityTooLarge, "INVALID_AUDIO", "The recording is too large or invalid.")
		return
	}
	if !supportedVoiceMimeType(input.MimeType) {
		writeError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_AUDIO_TYPE", "This audio format is not supported.")
		return
	}
	if input.DurationMS < 1 || input.DurationMS > maxVoiceRecordingDurationMS {
		writeError(w, http.StatusBadRequest, "INVALID_AUDIO_DURATION", "The recording must be no longer than 120 seconds.")
		return
	}
	if _, ok := normalizeVoiceLanguage(input.Language); !ok {
		writeError(w, http.StatusBadRequest, "INVALID_LANGUAGE", "The requested transcription language is invalid.")
		return
	}
	audio, err := base64.StdEncoding.DecodeString(input.AudioBase64)
	if err != nil || len(audio) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_AUDIO", "A valid audio recording is required.")
		return
	}
	if len(audio) > maxVoiceAudioBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "INVALID_AUDIO", "The recording is too large or invalid.")
		return
	}
	reservation, ok := h.reserveVoiceProviderCredit(w, r, userID, "recorded")
	if !ok {
		return
	}
	providerSucceeded := false
	defer func() {
		if providerSucceeded {
			return
		}
		if releaseErr := h.store.ReleaseUSDReservation(context.Background(), reservation.ID, userID, reservation.ID+":release"); releaseErr != nil {
			h.logger.Error("voice credit reservation release failed", "reservation_id", reservation.ID, "error", releaseErr)
		}
	}()
	result, deletionStatus, err := h.assemblyAI.Transcribe(r.Context(), audio, input.Language)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		diagnosticAttrs := []any{
			"request_id", r.Header.Get("X-Request-ID"),
			"provider", "assemblyai",
			"operation", "transcribe",
		}
		diagnosticAttrs = append(diagnosticAttrs, assemblyAIDiagnosticLogAttrs(err)...)
		h.logger.Error("voice transcription provider failed", diagnosticAttrs...)
		if errors.Is(err, errAssemblyAITranscriptionTimeout) {
			if deletionStatus != "deleted" {
				writeError(w, http.StatusGatewayTimeout, "VOICE_PROVIDER_TIMEOUT_CLEANUP_FAILED", "Transcription timed out and provider data deletion could not be confirmed.")
				return
			}
			writeError(w, http.StatusGatewayTimeout, "VOICE_PROVIDER_TIMEOUT", "Voice transcription took too long. Try a shorter recording.")
			return
		}
		if errors.Is(err, errAssemblyAIAudioTooLong) && deletionStatus == "deleted" {
			writeError(w, http.StatusBadRequest, "INVALID_AUDIO_DURATION", "The recording must be no longer than 120 seconds.")
			return
		}
		if deletionStatus != "deleted" {
			writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_DELETION_UNCONFIRMED", "Transcription failed and provider data deletion could not be confirmed.")
			return
		}
		writeError(w, http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Voice transcription is temporarily unavailable.")
		return
	}
	providerSucceeded = true
	creditReceipt, settled := h.settleVoiceProviderCredit(w, userID, reservation, result.AudioDurationMs, result.ProviderRequestID)
	if !settled {
		return
	}
	providerTranscriptStatus := "deleted"
	deletionMarker := "provider_transcript_deleted"
	if deletionStatus != "deleted" {
		providerTranscriptStatus = "deletion_failed"
		deletionMarker = "provider_transcript_deletion_failed"
		diagnosticAttrs := []any{
			"request_id", r.Header.Get("X-Request-ID"),
			"provider", "assemblyai",
			"operation", "transcribe",
			"stage", "delete_transcript",
			"failure_kind", result.deletion.failureKind,
			"cleanup_attempts", result.deletion.attempts,
		}
		if result.deletion.httpStatus > 0 {
			diagnosticAttrs = append(diagnosticAttrs, "cleanup_http_status", result.deletion.httpStatus)
		}
		h.logger.Error("voice provider deletion could not be confirmed", diagnosticAttrs...)
	}
	writeJSON(w, http.StatusOK, audioTranscriptionResponse{
		Transcript:    result.Transcript,
		Confidence:    result.Confidence,
		ReviewSignals: buildTranscriptionReviewSignals(result.Words),
		Deletion: transcriptionDeletion{
			RawAudio:           "not_stored",
			ProviderTranscript: providerTranscriptStatus,
			Marker:             deletionMarker,
		},
		CreditReceipt: creditReceipt,
	})
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
	var input struct {
		FirstName       *string `json:"firstName"`
		LastName        *string `json:"lastName"`
		PreferredLocale *string `json:"preferredLocale"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.FirstName == nil && input.LastName == nil && input.PreferredLocale == nil {
		writeError(w, http.StatusBadRequest, "INVALID_PROFILE", "At least one profile field is required.")
		return
	}
	if (input.FirstName != nil && len(*input.FirstName) > 100) ||
		(input.LastName != nil && len(*input.LastName) > 100) {
		writeError(w, http.StatusBadRequest, "INVALID_PROFILE", "Profile fields are too long.")
		return
	}
	if input.PreferredLocale != nil && !isSupportedPreferredLocale(*input.PreferredLocale) {
		writeError(w, http.StatusBadRequest, "INVALID_PROFILE", "The preferred locale is not supported.")
		return
	}
	firstName := trimOptionalString(input.FirstName)
	lastName := trimOptionalString(input.LastName)
	user, err := h.store.UpdateUserProfile(r.Context(), userID, firstName, lastName, input.PreferredLocale)
	if err != nil {
		h.storeError(w, "profile update failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileUserResponse(user))
}

type profileUserResponse struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	FirstName       string  `json:"firstName"`
	LastName        string  `json:"lastName"`
	ProfileImageURL string  `json:"profileImageUrl"`
	PreferredLocale *string `json:"preferredLocale"`
	Status          string  `json:"status"`
	AuthProvider    string  `json:"authProvider"`
}

func toProfileUserResponse(user postgres.User) profileUserResponse {
	return profileUserResponse{
		ID:              user.ID,
		Email:           user.Email,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		ProfileImageURL: user.ProfileImageURL,
		PreferredLocale: user.PreferredLocale,
		Status:          user.Status,
		AuthProvider:    user.AccountCreatedVia,
	}
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func isSupportedPreferredLocale(value string) bool {
	return value == "en" || value == "ar"
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
	return decodeBodyLimit(w, r, target, 128*1024)
}
func decodeBodyLimit(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil && !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body is too large.")
			return false
		}
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
