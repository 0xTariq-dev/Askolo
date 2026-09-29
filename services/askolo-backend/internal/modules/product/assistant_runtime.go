package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"askolo/backend/internal/adapters/postgres"
	policy "askolo/backend/internal/platform/authorization"
)

const (
	assistantTranscriptLimit = 4096
	assistantConfirmationTTL = 5 * time.Minute
)

type createAssistantRunInput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Transcript     string `json:"transcript"`
}

type assistantConfirmationInput struct {
	ExpectedIntentSHA256 string `json:"expectedIntentSha256"`
}

type assistantIntent struct {
	Tool  string `json:"tool"`
	Title string `json:"title"`
}

func (h *Handler) createAssistantRun(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input createAssistantRunInput
	if !decodeAssistantJSON(w, r, &input, 8*1024) {
		return
	}
	transcript, ok := preflightAssistantTranscript(input.Transcript)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_TRANSCRIPT", "Enter a short, valid assistant message and try again.")
		return
	}
	if len(input.ConversationID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_CONVERSATION", "The conversation could not be loaded.")
		return
	}
	if h.assistantPlanner == nil || !h.assistantPlanner.Available() {
		writeError(w, http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "An Idempotency-Key header is required.")
		return
	}
	policyVersion, err := strconvHeader(r.Header.Get("X-AI-Credit-Policy-Version"))
	if err != nil || policyVersion < 1 {
		writeError(w, http.StatusBadRequest, "CREDIT_POLICY_VERSION_REQUIRED", "Refresh the credit estimate before sending.")
		return
	}

	run, _, created, err := h.store.StartAssistantRun(
		r.Context(), userID, postgres.DefaultWorkspaceID(userID),
		strings.TrimSpace(input.ConversationID), idempotencyKey, transcript, policyVersion,
		assistantPlannerModel, int64(4096), int64(256),
	)
	if err != nil {
		if h.handleAssistantStoreError(w, err) {
			return
		}
		h.storeError(w, "assistant run start failed", err)
		return
	}
	if !created {
		writeAssistantRun(w, run)
		return
	}
	started, err := h.store.MarkAssistantProviderStarted(r.Context(), userID, run.ID)
	if err != nil {
		if r.Context().Err() != nil {
			finishAssistantPlanning(h, userID, run.ID, postgres.AssistantPlanOutcome{
				State: "cancelled", Message: "I stopped this request. No action was taken.",
				AuditEvent: "run_cancelled",
			})
			return
		}
		h.storeError(w, "assistant provider claim failed", err)
		return
	}
	if !started {
		current, lookupErr := h.store.GetAssistantRun(r.Context(), userID, run.ID)
		if lookupErr != nil {
			h.storeError(w, "assistant run lookup failed", lookupErr)
			return
		}
		writeAssistantRun(w, current)
		return
	}
	run.ProviderStarted = true

	ctx, cancel := context.WithTimeout(r.Context(), assistantPlannerTimeout)
	defer cancel()
	plan, planErr := h.assistantPlanner.Plan(ctx, transcript)
	if r.Context().Err() != nil {
		outcome := postgres.AssistantPlanOutcome{
			State: "cancelled", Message: "I stopped this request. No action was taken.",
			AuditEvent: "run_cancelled",
		}
		if plan.UsageValid && plan.ProviderModel == assistantPlannerModel {
			outcome.Settle = true
			outcome.ProviderModel = plan.ProviderModel
			outcome.ProviderRequestID = plan.ProviderRequestID
			outcome.InputTokens = plan.InputTokens
			outcome.OutputTokens = plan.OutputTokens
		}
		finishAssistantPlanning(h, userID, run.ID, outcome)
		return
	}
	if planErr != nil {
		h.logger.Warn("assistant provider operation failed", "run_id", run.ID, "error_type", fmt.Sprintf("%T", planErr))
		outcome := postgres.AssistantPlanOutcome{
			State: "failed", Message: "I couldn't complete that request. Please try again.",
			AuditEvent: "provider_failed", Settle: plan.UsageValid && plan.ProviderModel == assistantPlannerModel,
			ProviderModel: plan.ProviderModel, ProviderRequestID: plan.ProviderRequestID,
			InputTokens: plan.InputTokens, OutputTokens: plan.OutputTokens,
		}
		if _, finishErr := h.store.FinishAssistantPlanning(context.Background(), userID, run.ID, outcome); finishErr != nil {
			h.storeError(w, "assistant provider failure recording failed", finishErr)
			return
		}
		writeError(w, http.StatusBadGateway, "ASSISTANT_PROVIDER_FAILED", "The assistant could not complete this request. Please try again.")
		return
	}

	outcome := postgres.AssistantPlanOutcome{
		State: "completed", Message: "I can prepare one action item at a time. Tell me the single item you want me to add.",
		AuditEvent: "plan_ready", Settle: plan.UsageValid,
		ProviderModel: plan.ProviderModel, ProviderRequestID: plan.ProviderRequestID,
		InputTokens: plan.InputTokens, OutputTokens: plan.OutputTokens,
	}
	switch plan.Intent {
	case "none":
		outcome.Message = "I can help prepare one action item for your list. Tell me the specific item you want to add."
	case "clarify":
		outcome.Message = "What single action item would you like me to prepare?"
	case "create_action_item":
		title := strings.TrimSpace(plan.Title)
		if !validAssistantActionTitle(title) {
			outcome.State = "rejected"
			outcome.Message = "I couldn't safely prepare that item. Please rephrase it as one specific action."
			outcome.AuditEvent = "intent_rejected"
			break
		}
		intentJSON, marshalErr := json.Marshal(assistantIntent{Tool: "create_action_item", Title: title})
		if marshalErr != nil {
			outcome.State = "rejected"
			outcome.Message = "I couldn't safely prepare that item. Please try again."
			outcome.AuditEvent = "intent_rejected"
			break
		}
		intentDigest := sha256.Sum256(intentJSON)
		outcome.State = "needs_confirmation"
		outcome.Intent = intentJSON
		outcome.IntentSHA256 = hex.EncodeToString(intentDigest[:])
		outcome.RiskLevel = "write"
		outcome.RequiresConfirmation = true
		expires := time.Now().UTC().Add(assistantConfirmationTTL)
		outcome.ConfirmationExpires = &expires
		outcome.ToolName = "create_action_item"
		outcome.ToolArgsSHA256 = outcome.IntentSHA256
		outcome.Message = fmt.Sprintf("I can add “%s” to your action items. Confirm to save it.", title)
	default:
		outcome.State = "rejected"
		outcome.Message = "I couldn't safely prepare an action from that request. Please try rephrasing it."
		outcome.AuditEvent = "intent_rejected"
	}
	finished, err := h.store.FinishAssistantPlanning(r.Context(), userID, run.ID, outcome)
	if err != nil {
		h.storeError(w, "assistant plan persistence failed", err)
		return
	}
	writeAssistantRun(w, finished)
}

func (h *Handler) getAssistantConversation(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	result, err := h.store.GetAssistantConversation(r.Context(), userID, postgres.DefaultWorkspaceID(userID))
	if err != nil {
		h.storeError(w, "assistant conversation lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getAssistantRun(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" || len(runID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_RUN_ID", "A valid assistant run id is required.")
		return
	}
	run, err := h.store.GetAssistantRun(r.Context(), userID, runID)
	if err != nil {
		if errors.Is(err, postgres.ErrAssistantNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Assistant run not found.")
			return
		}
		h.storeError(w, "assistant run lookup failed", err)
		return
	}
	writeAssistantRun(w, run)
}

func (h *Handler) confirmAssistantRun(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	if !h.authorize(r, userID, "actionItem", "", policy.ActionResourceCreate, w) {
		return
	}
	var input assistantConfirmationInput
	if !decodeAssistantJSON(w, r, &input, 2*1024) || !validIntentHash(input.ExpectedIntentSHA256) {
		writeError(w, http.StatusBadRequest, "INVALID_CONFIRMATION", "The action confirmation is invalid. Review the action and try again.")
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" || len(runID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_RUN_ID", "A valid assistant run id is required.")
		return
	}
	run, err := h.store.ConfirmAssistantActionItem(r.Context(), userID, runID, input.ExpectedIntentSHA256)
	if err != nil {
		if h.handleAssistantStoreError(w, err) {
			return
		}
		h.storeError(w, "assistant action confirmation failed", err)
		return
	}
	writeAssistantRun(w, run)
}

func (h *Handler) cancelAssistantRun(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK || !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input assistantConfirmationInput
	if !decodeAssistantJSON(w, r, &input, 2*1024) || !validIntentHash(input.ExpectedIntentSHA256) {
		writeError(w, http.StatusBadRequest, "INVALID_CONFIRMATION", "The pending action could not be dismissed.")
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" || len(runID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_RUN_ID", "A valid assistant run id is required.")
		return
	}
	run, err := h.store.CancelAssistantConfirmation(r.Context(), userID, runID, input.ExpectedIntentSHA256)
	if err != nil {
		if h.handleAssistantStoreError(w, err) {
			return
		}
		h.storeError(w, "assistant action cancellation failed", err)
		return
	}
	writeAssistantRun(w, run)
}

func (h *Handler) handleAssistantStoreError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, postgres.ErrAssistantNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Assistant run or conversation not found.")
	case errors.Is(err, postgres.ErrAssistantRateLimited):
		writeError(w, http.StatusTooManyRequests, "ASSISTANT_RATE_LIMITED", "Please wait a moment before sending another assistant request.")
	case errors.Is(err, postgres.ErrAssistantInsufficient):
		writeError(w, http.StatusPaymentRequired, "AI_CREDITS_EXHAUSTED", "There are not enough AI credits for this request.")
	case errors.Is(err, postgres.ErrAssistantPolicyChanged):
		writeError(w, http.StatusConflict, "AI_POLICY_CHANGED", "The AI credit policy changed. Refresh the estimate and try again.")
	case errors.Is(err, postgres.ErrAssistantPolicyInvalid):
		writeError(w, http.StatusServiceUnavailable, "AI_POLICY_INVALID", "The assistant credit policy is unavailable.")
	case errors.Is(err, postgres.ErrAssistantCreditConflict):
		writeError(w, http.StatusConflict, "CREDIT_RESERVATION_CONFLICT", "This assistant request is already being processed.")
	case errors.Is(err, postgres.ErrAssistantIdempotencyConflict):
		writeError(w, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "This request key was already used for different content.")
	case errors.Is(err, postgres.ErrAssistantConfirmation):
		writeError(w, http.StatusConflict, "CONFIRMATION_EXPIRED", "This confirmation is no longer valid. Review the action again.")
	case errors.Is(err, postgres.ErrAssistantRunConflict):
		writeError(w, http.StatusConflict, "ASSISTANT_RUN_CONFLICT", "This assistant run can no longer be changed.")
	default:
		return false
	}
	return true
}

func finishAssistantPlanning(h *Handler, userID, runID string, outcome postgres.AssistantPlanOutcome) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := h.store.FinishAssistantPlanning(ctx, userID, runID, outcome); err != nil {
		h.logger.Error("assistant cancellation outcome could not be persisted", "run_id", runID)
	}
}

func writeAssistantRun(w http.ResponseWriter, run postgres.AssistantRunRecord) {
	status := http.StatusOK
	if run.State == "planning" {
		status = http.StatusAccepted
	}
	writeJSON(w, status, run)
}

func preflightAssistantTranscript(value string) (string, bool) {
	if !utf8.ValidString(value) {
		return "", false
	}
	value = strings.TrimSpace(value)
	if len([]byte(value)) == 0 || len([]byte(value)) > assistantTranscriptLimit {
		return "", false
	}
	for _, character := range value {
		if (character < 0x20 && character != '\n' && character != '\r' && character != '\t') || character == 0x7f {
			return "", false
		}
	}
	return value, true
}

func validAssistantActionTitle(title string) bool {
	if title == "" || len([]byte(title)) > 120 || !utf8.ValidString(title) {
		return false
	}
	for _, character := range title {
		if (character < 0x20 && character != '\t') || character == 0x7f {
			return false
		}
	}
	return true
}

func validIntentHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func decodeAssistantJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid.")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid.")
		return false
	}
	return true
}

func strconvHeader(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 9 {
		return 0, errors.New("invalid integer header")
	}
	n := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid integer header")
		}
		n = n*10 + int(char-'0')
		if n > 100000000 {
			return 0, errors.New("integer header out of range")
		}
	}
	return n, nil
}
