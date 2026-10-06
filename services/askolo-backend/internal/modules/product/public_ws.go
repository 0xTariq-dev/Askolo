package product

import (
	"context"
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
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer releaseCancel()
			if releaseErr := h.store.ReleaseUSDReservation(releaseCtx, reservation.ID, userID, reservation.ID+":release"); releaseErr != nil {
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
	payload, err := json.Marshal(map[string]any{
		"maxSessionDurationSeconds": assemblyAIRealtimeMaxSessionDurationSeconds,
		"creditReceipt":             voiceCreditReceipt{ID: reservation.ID, ReservationID: reservation.ID, OperationType: "voice", Provider: "assemblyai", Mode: "realtime", Status: "claimed", ReservedUsdMicros: reservation.ReservedUsdMicros, PolicyVersion: reservation.PolicyVersion},
	})
	if err != nil {
		_, settleErr := h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
		if settleErr != nil {
			h.logger.Error("public voice setup settlement failed", "reservation_id", reservation.ID, "error", settleErr)
		}
		provider.Close()
		releaseLimit()
		return nil, nil, publicws.Failure(http.StatusInternalServerError, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", err)
	}
	return &publicVoiceSession{provider: provider, handler: h, userID: userID, reservation: reservation, startedAt: time.Now(), releaseLimit: releaseLimit}, payload, nil
}

type publicVoiceSession struct {
	provider           assemblyAIRealtimeSession
	handler            *Handler
	userID             string
	reservation        voiceCreditReservation
	startedAt          time.Time
	mu                 sync.Mutex
	settled            bool
	pendingTermination bool
	meteredDurationMS  int64
	meterSource        string
	releaseLimit       func()
	closeOnce          sync.Once
}

func (s *publicVoiceSession) SendPCMFrame(ctx context.Context, frame []byte) error {
	return s.provider.SendPCMFrame(ctx, frame)
}

func (s *publicVoiceSession) ReadProviderMessage(ctx context.Context) (string, json.RawMessage, error) {
	s.mu.Lock()
	if s.pendingTermination {
		s.pendingTermination = false
		s.mu.Unlock()
		return "Termination", nil, nil
	}
	s.mu.Unlock()

	event, err := s.provider.ReadProviderMessage(ctx)
	if err != nil || event.Type != "Termination" {
		return event.Type, event.Payload, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return event.Type, event.Payload, nil
	}
	if s.meteredDurationMS == 0 {
		durationMS := time.Since(s.startedAt).Milliseconds()
		var payload struct {
			SessionDurationSeconds float64 `json:"session_duration_seconds"`
		}
		source := "server_elapsed"
		if json.Unmarshal(event.Payload, &payload) == nil {
			if measured, ok := durationSecondsMillis(payload.SessionDurationSeconds); ok {
				durationMS, source = measured, "assemblyai_session_duration"
			}
		}
		maxMS := int64(assemblyAIRealtimeMaxSessionDurationSeconds) * 1000
		if durationMS < 1 {
			durationMS = 1
		}
		if durationMS > maxMS {
			durationMS = maxMS
		}
		s.meteredDurationMS = durationMS
		s.meterSource = source
	}
	receipt, settleErr := s.handler.settleVoiceProviderCreditRecordWithUsage(
		s.userID, s.reservation, s.meteredDurationMS, s.meterSource,
	)
	if settleErr != nil {
		return "", nil, settleErr
	}
	s.settled = true
	s.pendingTermination = true
	settlement, _ := json.Marshal(map[string]any{"creditReceipt": receipt})
	return "AskoloSettlement", settlement, nil
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
		closedAt := time.Now()
		s.provider.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.settled && s.handler != nil {
			durationMS := s.meteredDurationMS
			source := s.meterSource
			if durationMS < 1 {
				durationMS = closedAt.Sub(s.startedAt).Milliseconds()
				source = "server_elapsed"
			}
			maxMS := int64(assemblyAIRealtimeMaxSessionDurationSeconds) * 1000
			if durationMS < 1 {
				durationMS = 1
			}
			if durationMS > maxMS {
				durationMS = maxMS
			}
			if _, err := s.handler.settleVoiceProviderCreditRecordWithUsage(s.userID, s.reservation, durationMS, source); err != nil {
				s.handler.logger.Error("public voice settlement failed", "reservation_id", s.reservation.ID, "error", err)
			} else {
				s.settled = true
			}
		}
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
	if h.store == nil {
		h.logAssistantUnavailable("preflight", "store_unavailable", nil)
		return nil, publicws.Failure(http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", nil)
	}
	transcript, err := h.assistantFacingTranscript(ctx, userID, transcript)
	if errors.Is(err, errAssistantProcessingConsentRequired) {
		return nil, publicws.Failure(
			http.StatusForbidden,
			"ASSISTANT_PROCESSING_CONSENT_REQUIRED",
			"Review the Assistant processing and app-side redaction settings before sending.",
			nil,
		)
	}
	if err != nil {
		h.logAssistantUnavailable("privacy_preflight", "privacy_preferences_unavailable", err)
		return nil, publicws.Failure(http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", nil)
	}
	if reason := assistantPlannerUnavailableReason(h.assistantPlanner); reason != "" {
		h.logAssistantUnavailable("preflight", reason, nil)
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
		return nil, h.assistantPublicErrorAt("create_run", err)
	}
	startedPayload, err := json.Marshal(run)
	if err != nil {
		h.logAssistantUnavailable("serialize_run", "serialization_failure", err)
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
		return nil, h.assistantPublicErrorAt("claim_provider", err)
	}
	if !started {
		current, lookupErr := h.store.GetAssistantRun(ctx, userID, run.ID)
		if lookupErr != nil {
			return nil, h.assistantPublicErrorAt("load_run", lookupErr)
		}
		return json.Marshal(current)
	}

	planCtx, cancel := context.WithTimeout(ctx, assistantPlannerTimeout)
	plan, planErr := h.assistantPlanner.Plan(planCtx, transcript)
	cancel()
	if ctx.Err() != nil {
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
		return nil, ctx.Err()
	}
	if planErr != nil {
		h.logger.Warn("assistant provider operation failed", "run_id", run.ID, "error_type", fmt.Sprintf("%T", planErr))
		outcome := postgres.AssistantPlanOutcome{
			State: "failed", Message: "I couldn't complete that request. Please try again.",
			AuditEvent:    "provider_failed",
			Settle:        plan.UsageValid && plan.ProviderModel == assistantPlannerModel,
			ProviderModel: plan.ProviderModel, ProviderRequestID: plan.ProviderRequestID,
			InputTokens: plan.InputTokens, OutputTokens: plan.OutputTokens,
		}
		if _, finishErr := h.store.FinishAssistantPlanning(context.Background(), userID, run.ID, outcome); finishErr != nil {
			return nil, h.assistantPublicErrorAt("finish_failed_run", finishErr)
		}
		return nil, publicws.Failure(http.StatusBadGateway, "ASSISTANT_PROVIDER_FAILED", "The assistant could not complete this request. Please try again.", planErr)
	}

	outcome := prepareAssistantPlanOutcome(plan, h.toolRegistry())
	finished, err := h.store.FinishAssistantPlanning(ctx, userID, run.ID, outcome)
	if err != nil {
		return nil, h.assistantPublicErrorAt("finish_run", err)
	}
	return json.Marshal(finished)
}

func (h *Handler) logAssistantUnavailable(stage, reason string, err error) {
	if h == nil || h.logger == nil {
		return
	}
	attributes := []any{"stage", stage, "reason", reason}
	if err != nil {
		attributes = append(attributes, "error_type", fmt.Sprintf("%T", err))
		var sqlStateError interface{ SQLState() string }
		if errors.As(err, &sqlStateError) {
			attributes = append(attributes, "sql_state", sqlStateError.SQLState())
		}
	}
	h.logger.Error("assistant operation unavailable", attributes...)
}

func (h *Handler) assistantPublicErrorAt(stage string, err error) error {
	failure := assistantPublicError(err)
	var publicError *publicws.Error
	if errors.As(failure, &publicError) && publicError.Code == "ASSISTANT_UNAVAILABLE" {
		h.logAssistantUnavailable(stage, "persistence_failure", err)
	}
	return failure
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
