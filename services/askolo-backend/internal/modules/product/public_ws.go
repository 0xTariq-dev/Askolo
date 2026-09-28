package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/publicws"
)

func (h *Handler) StartVoice(
	ctx context.Context,
	userID string,
	workspaceID string,
	idempotencyKey string,
	policyVersion int,
) (publicws.VoiceSession, json.RawMessage, error) {
	_ = workspaceID // Workspace authorization is performed by the shared transport.
	if h.store == nil {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", nil)
	}
	allowed, err := h.allowRealtimeConnectionStart(ctx, userID)
	if err != nil {
		h.logger.Error("realtime session start rate limit failed", "error", err)
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", err)
	}
	if !allowed {
		return nil, nil, publicws.Failure(http.StatusTooManyRequests, "VOICE_START_RATE_LIMIT", "Too many live transcription starts were requested.", nil)
	}
	consent, consentVersion, err := h.store.VoiceConsent(ctx, userID)
	if err != nil {
		h.logger.Error("voice consent lookup failed", "error", err)
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", err)
	}
	if !consent || consentVersion != postgres.VoiceConsentVersion {
		return nil, nil, publicws.Failure(http.StatusForbidden, "VOICE_CONSENT_REQUIRED", "Voice transcription consent is required.", nil)
	}
	if h.assemblyAI == nil || !h.assemblyAI.Configured() {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_NOT_CONFIGURED", "Voice transcription is not configured.", nil)
	}
	if h.realtimeLimiter == nil {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", nil)
	}
	releaseLimit, acquired := h.realtimeLimiter.acquire(userID)
	if !acquired {
		return nil, nil, publicws.Failure(http.StatusTooManyRequests, "VOICE_SESSION_LIMIT", "Too many live transcription sessions are active.", nil)
	}
	fail := func(err error) (publicws.VoiceSession, json.RawMessage, error) {
		releaseLimit()
		return nil, nil, err
	}

	reservation, failure := h.reserveVoiceProviderCreditRequest(ctx, userID, "realtime", strings.TrimSpace(idempotencyKey), policyVersion)
	if failure != nil {
		if failure.err != nil {
			h.logger.Error(failure.operation, "error", failure.err)
			return fail(publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "The live transcription session could not be started.", failure.err))
		}
		return fail(publicws.Failure(failure.status, failure.code, failure.message, nil))
	}
	providerStarted := false
	defer func() {
		if !providerStarted {
			if releaseErr := h.store.ReleaseAICreditReservation(context.Background(), reservation.ID, userID, reservation.ID+":release"); releaseErr != nil {
				h.logger.Error("voice credit reservation release failed", "reservation_id", reservation.ID, "error", releaseErr)
			}
		}
	}()

	setupCtx, cancel := context.WithTimeout(ctx, realtimeProviderSetupLimit)
	defer cancel()
	token, err := h.assemblyAI.RealtimeToken(setupCtx)
	if err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(publicws.Failure(http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Real-time transcription could not be started. Try recorded transcription instead.", err))
	}
	provider, err := h.assemblyAI.OpenRealtimeSession(setupCtx, token)
	if err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(publicws.Failure(http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "Real-time transcription could not be started. Try recorded transcription instead.", err))
	}
	providerStarted = true
	receipt, err := h.settleVoiceProviderCreditRecord(userID, reservation)
	if err != nil {
		h.logger.Error("voice credit settlement failed", "reservation_id", reservation.ID, "error", err)
		terminateCtx, cancelTerminate := context.WithTimeout(context.Background(), realtimeTerminationTimeout)
		_ = provider.SendTermination(terminateCtx)
		cancelTerminate()
		provider.Close()
		releaseLimit()
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "AI_CREDIT_SETTLEMENT_FAILED", "The provider session started, but its credit charge could not be confirmed. Check your credit history before retrying.", err)
	}
	payload, err := json.Marshal(map[string]any{
		"maxSessionDurationSeconds": assemblyAIRealtimeMaxSessionDurationSeconds,
		"creditReceipt":             receipt,
	})
	if err != nil {
		provider.Close()
		releaseLimit()
		return nil, nil, publicws.Failure(http.StatusInternalServerError, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", err)
	}
	return &publicVoiceSession{provider: provider, releaseLimit: releaseLimit}, payload, nil
}

type publicVoiceSession struct {
	provider     assemblyAIRealtimeSession
	releaseLimit func()
	closeOnce    sync.Once
}

func (s *publicVoiceSession) SendPCMFrame(ctx context.Context, frame []byte) error {
	return s.provider.SendPCMFrame(ctx, frame)
}

func (s *publicVoiceSession) ReadProviderMessage(ctx context.Context) (string, json.RawMessage, error) {
	event, err := s.provider.ReadProviderMessage(ctx)
	return event.Type, event.Payload, err
}

func (s *publicVoiceSession) SendTermination(ctx context.Context) error {
	return s.provider.SendTermination(ctx)
}

func (s *publicVoiceSession) MaxDuration() time.Duration {
	return time.Duration(assemblyAIRealtimeMaxSessionDurationSeconds) * time.Second
}

func (s *publicVoiceSession) Close() {
	if s == nil || s.provider == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.provider.Close()
		if s.releaseLimit != nil {
			s.releaseLimit()
		}
	})
}

func (h *Handler) RunAssistant(
	ctx context.Context,
	userID string,
	workspaceID string,
	input publicws.AssistantRequest,
	onStarted func(runID string, payload json.RawMessage) error,
) (json.RawMessage, error) {
	transcript, ok := preflightAssistantTranscript(input.Transcript)
	if !ok {
		return nil, publicws.Failure(http.StatusBadRequest, "INVALID_TRANSCRIPT", "Enter a short, valid assistant message and try again.", nil)
	}
	if len(input.ConversationID) > 128 {
		return nil, publicws.Failure(http.StatusBadRequest, "INVALID_CONVERSATION", "The conversation could not be loaded.", nil)
	}
	if input.IdempotencyKey == "" || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey || len(input.IdempotencyKey) > 200 {
		return nil, publicws.Failure(http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "An idempotency key is required.", nil)
	}
	if input.PolicyVersion < 1 {
		return nil, publicws.Failure(http.StatusBadRequest, "CREDIT_POLICY_VERSION_REQUIRED", "Refresh the credit estimate before sending.", nil)
	}
	if h.store == nil || h.assistantPlanner == nil || !h.assistantPlanner.Available() {
		return nil, publicws.Failure(http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", nil)
	}

	run, _, created, err := h.store.StartAssistantRun(
		ctx,
		userID,
		workspaceID,
		strings.TrimSpace(input.ConversationID),
		input.IdempotencyKey,
		transcript,
		input.PolicyVersion,
	)
	if err != nil {
		return nil, assistantPublicError(err)
	}
	startedPayload, err := json.Marshal(run)
	if err != nil {
		return nil, publicws.Failure(http.StatusInternalServerError, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", err)
	}
	if onStarted != nil {
		if err := onStarted(run.ID, startedPayload); err != nil {
			if created {
				finishAssistantPlanning(h, userID, run.ID, postgres.AssistantPlanOutcome{
					State: "cancelled", Message: "I stopped this request. No action was taken.",
					AuditEvent: "run_cancelled",
				})
			}
			return nil, err
		}
	}
	if !created {
		return startedPayload, nil
	}

	started, err := h.store.MarkAssistantProviderStarted(ctx, userID, run.ID)
	if err != nil {
		if ctx.Err() != nil {
			finishAssistantPlanning(h, userID, run.ID, postgres.AssistantPlanOutcome{
				State: "cancelled", Message: "I stopped this request. No action was taken.",
				AuditEvent: "run_cancelled",
			})
			return nil, ctx.Err()
		}
		return nil, assistantPublicError(err)
	}
	if !started {
		current, lookupErr := h.store.GetAssistantRun(ctx, userID, run.ID)
		if lookupErr != nil {
			return nil, assistantPublicError(lookupErr)
		}
		return json.Marshal(current)
	}

	planCtx, cancel := context.WithTimeout(ctx, assistantPlannerTimeout)
	plan, planErr := h.assistantPlanner.Plan(planCtx, transcript)
	cancel()
	if ctx.Err() != nil {
		finishAssistantPlanning(h, userID, run.ID, postgres.AssistantPlanOutcome{
			State: "cancelled", Message: "I stopped this request. No action was taken.",
			AuditEvent: "run_cancelled", Settle: true, SettledCredits: run.BaseCredits,
		})
		return nil, ctx.Err()
	}
	if planErr != nil {
		h.logger.Warn("assistant provider operation failed", "run_id", run.ID, "error_type", fmt.Sprintf("%T", planErr))
		outcome := postgres.AssistantPlanOutcome{
			State: "failed", Message: "I couldn't complete that request. Please try again.",
			AuditEvent: "provider_failed", Settle: true, SettledCredits: run.BaseCredits,
		}
		if _, finishErr := h.store.FinishAssistantPlanning(context.Background(), userID, run.ID, outcome); finishErr != nil {
			return nil, assistantPublicError(finishErr)
		}
		return nil, publicws.Failure(http.StatusBadGateway, "ASSISTANT_PROVIDER_FAILED", "The assistant could not complete this request. Please try again.", planErr)
	}

	outcome := postgres.AssistantPlanOutcome{
		State: "completed", Message: "I can prepare one action item at a time. Tell me the single item you want me to add.",
		AuditEvent: "plan_ready", Settle: true, SettledCredits: run.BaseCredits,
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
		digest := sha256.Sum256(intentJSON)
		outcome.State = "needs_confirmation"
		outcome.Intent = intentJSON
		outcome.IntentSHA256 = hex.EncodeToString(digest[:])
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
	finished, err := h.store.FinishAssistantPlanning(ctx, userID, run.ID, outcome)
	if err != nil {
		return nil, assistantPublicError(err)
	}
	return json.Marshal(finished)
}

func assistantPublicError(err error) error {
	switch {
	case errors.Is(err, postgres.ErrAssistantNotFound):
		return publicws.Failure(http.StatusNotFound, "NOT_FOUND", "Assistant run or conversation not found.", err)
	case errors.Is(err, postgres.ErrAssistantRateLimited):
		return publicws.Failure(http.StatusTooManyRequests, "ASSISTANT_RATE_LIMITED", "Please wait a moment before sending another assistant request.", err)
	case errors.Is(err, postgres.ErrAssistantInsufficient):
		return publicws.Failure(http.StatusPaymentRequired, "AI_CREDITS_EXHAUSTED", "There are not enough AI credits for this request.", err)
	case errors.Is(err, postgres.ErrAssistantPolicyChanged):
		return publicws.Failure(http.StatusConflict, "AI_POLICY_CHANGED", "The AI credit policy changed. Refresh the estimate and try again.", err)
	case errors.Is(err, postgres.ErrAssistantPolicyInvalid):
		return publicws.Failure(http.StatusServiceUnavailable, "AI_POLICY_INVALID", "AI credit policy is unavailable.", err)
	case errors.Is(err, postgres.ErrAssistantCreditConflict):
		return publicws.Failure(http.StatusConflict, "CREDIT_RESERVATION_CONFLICT", "This assistant request is already being processed.", err)
	case errors.Is(err, postgres.ErrAssistantIdempotencyConflict):
		return publicws.Failure(http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "This request key was already used for different content.", err)
	case errors.Is(err, postgres.ErrAssistantConfirmation):
		return publicws.Failure(http.StatusConflict, "CONFIRMATION_EXPIRED", "This confirmation is no longer valid. Review the action again.", err)
	case errors.Is(err, postgres.ErrAssistantRunConflict):
		return publicws.Failure(http.StatusConflict, "ASSISTANT_RUN_CONFLICT", "This assistant run can no longer be changed.", err)
	default:
		return publicws.Failure(http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", err)
	}
}
