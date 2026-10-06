package product

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/publicws"
)

const (
	voiceAgentSetupTimeout       = 12 * time.Second
	voiceAgentStopTimeout        = 4 * time.Second
	voiceAgentSendTimeout        = 5 * time.Second
	voiceAgentDeleteTimeout      = 5 * time.Second
	maxVoiceAgentToolCalls       = 3
	maxVoiceAgentTranscriptBytes = 4096
	maxVoiceAgentAudioBytes      = 16 * 1024
)

func normalizeVoiceAgentTranscriptEvent(eventType string, payload []byte) (json.RawMessage, string, bool) {
	switch eventType {
	case "transcript.user":
		var event struct {
			Text   string `json:"text"`
			ItemID string `json:"item_id"`
		}
		if json.Unmarshal(payload, &event) != nil || !validAssemblyAIVoiceAgentSessionID(event.ItemID) ||
			!utf8.ValidString(event.Text) || len(event.Text) > maxVoiceAgentTranscriptBytes {
			return nil, "", false
		}
		normalized, err := json.Marshal(map[string]any{"type": eventType, "text": event.Text, "item_id": event.ItemID})
		return normalized, event.Text, err == nil
	case "transcript.user.delta":
		var event struct {
			Text   string `json:"text"`
			ItemID string `json:"item_id"`
		}
		if json.Unmarshal(payload, &event) != nil || !validAssemblyAIVoiceAgentSessionID(event.ItemID) ||
			!utf8.ValidString(event.Text) || len(event.Text) > maxVoiceAgentTranscriptBytes {
			return nil, "", false
		}
		normalized, err := json.Marshal(map[string]any{"type": eventType, "text": event.Text, "item_id": event.ItemID})
		return normalized, "", err == nil
	case "transcript.agent.delta":
		var event struct {
			Delta   string `json:"delta"`
			ItemID  string `json:"item_id"`
			ReplyID string `json:"reply_id"`
		}
		if json.Unmarshal(payload, &event) != nil || !validAssemblyAIVoiceAgentSessionID(event.ItemID) ||
			!validAssemblyAIVoiceAgentSessionID(event.ReplyID) || !utf8.ValidString(event.Delta) ||
			len(event.Delta) > maxVoiceAgentTranscriptBytes {
			return nil, "", false
		}
		normalized, err := json.Marshal(map[string]any{
			"type": eventType, "delta": event.Delta, "item_id": event.ItemID, "reply_id": event.ReplyID,
		})
		return normalized, "", err == nil
	case "transcript.agent":
		var event struct {
			Text        string `json:"text"`
			ItemID      string `json:"item_id"`
			ReplyID     string `json:"reply_id"`
			Interrupted bool   `json:"interrupted"`
		}
		if json.Unmarshal(payload, &event) != nil || !validAssemblyAIVoiceAgentSessionID(event.ItemID) ||
			!validAssemblyAIVoiceAgentSessionID(event.ReplyID) || !utf8.ValidString(event.Text) ||
			len(event.Text) > maxVoiceAgentTranscriptBytes {
			return nil, "", false
		}
		normalized, err := json.Marshal(map[string]any{
			"type": eventType, "text": event.Text, "item_id": event.ItemID,
			"reply_id": event.ReplyID, "interrupted": event.Interrupted,
		})
		return normalized, "", err == nil
	default:
		return nil, "", false
	}
}

func (h *Handler) StartVoiceAgent(
	ctx context.Context,
	userID string,
	workspaceID string,
	request publicws.VoiceAgentRequest,
) (publicws.VoiceAgentSession, json.RawMessage, error) {
	if !h.voiceAgentEnabled {
		return nil, nil, publicws.Failure(
			http.StatusServiceUnavailable, "VOICE_AGENT_NOT_ENABLED",
			"Live Mode is not enabled in this environment.", nil,
		)
	}
	if h.store == nil {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", nil)
	}
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 200 ||
		strings.TrimSpace(request.IdempotencyKey) != request.IdempotencyKey ||
		request.PolicyVersion < 1 || request.AssistantPolicyVersion < 1 ||
		len(request.ConversationID) > 128 ||
		(request.ConversationID != "" && strings.TrimSpace(request.ConversationID) != request.ConversationID) {
		return nil, nil, publicws.Failure(http.StatusBadRequest, "INVALID_VOICE_AGENT_REQUEST", "Refresh the estimates and try starting Live Mode again.", nil)
	}
	if !h.hasAllVoiceAgentIDs() {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_NOT_CONFIGURED", "Live Mode is not configured.", nil)
	}

	allowed, err := h.allowRealtimeConnectionStart(ctx, userID)
	if err != nil {
		h.logger.Error("Voice Agent start rate limit failed", "error_type", fmt.Sprintf("%T", err))
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", err)
	}
	if !allowed {
		return nil, nil, publicws.Failure(http.StatusTooManyRequests, "VOICE_START_RATE_LIMIT", "Too many live voice sessions were requested.", nil)
	}

	consent, consentVersion, err := h.store.VoiceAgentConsent(ctx, userID)
	if err != nil {
		h.logger.Error("Voice Agent consent lookup failed", "error_type", fmt.Sprintf("%T", err))
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", err)
	}
	if !consent || consentVersion != postgres.VoiceAgentConsentVersion {
		return nil, nil, publicws.Failure(http.StatusForbidden, "VOICE_AGENT_CONSENT_REQUIRED", "Review and accept the AssemblyAI Live Mode privacy notice before starting.", nil)
	}
	preferences, err := h.store.AIPrivacyPreferences(ctx, userID)
	if err != nil {
		h.logger.Error("Voice Agent preference lookup failed", "error_type", fmt.Sprintf("%T", err))
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", err)
	}
	voice, supported := assemblyAILiveVoiceChoices[preferences.AssemblyAILiveVoice]
	if !supported || request.Locale != voice.Locale {
		return nil, nil, publicws.Failure(http.StatusBadRequest, "VOICE_AGENT_LANGUAGE_UNSUPPORTED", "The selected Live Mode voice and language do not match.", nil)
	}
	agentID, configured := h.voiceAgentID(preferences.AssemblyAILiveVoice)
	if !configured {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_NOT_CONFIGURED", "Live Mode is not configured.", nil)
	}

	provider, ok := h.assemblyAI.(assemblyAIVoiceAgentProvider)
	if !ok || !provider.Configured() {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_NOT_CONFIGURED", "Live Mode is not configured.", nil)
	}
	if h.realtimeLimiter == nil {
		return nil, nil, publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", nil)
	}
	releaseLimit, acquired := h.realtimeLimiter.acquire(userID)
	if !acquired {
		return nil, nil, publicws.Failure(http.StatusTooManyRequests, "VOICE_SESSION_LIMIT", "Too many live voice sessions are active.", nil)
	}
	release := func() {
		if releaseLimit != nil {
			releaseLimit()
			releaseLimit = nil
		}
	}
	fail := func(err error) (publicws.VoiceAgentSession, json.RawMessage, error) {
		release()
		return nil, nil, err
	}

	reservation, failure := h.reserveVoiceProviderCreditRequest(
		ctx, userID, "voice_agent", request.IdempotencyKey, request.PolicyVersion,
	)
	if failure != nil {
		if failure.err != nil {
			h.logger.Error(failure.operation, "error_type", fmt.Sprintf("%T", failure.err))
			return fail(publicws.Failure(http.StatusServiceUnavailable, "VOICE_AGENT_UNAVAILABLE", "The Live Mode session could not be started.", failure.err))
		}
		return fail(publicws.Failure(failure.status, failure.code, failure.message, nil))
	}

	setupCtx, cancel := context.WithTimeout(ctx, voiceAgentSetupTimeout)
	defer cancel()
	connection, err := provider.OpenVoiceAgentSession(setupCtx)
	if err != nil {
		h.releaseVoiceAgentReservation(userID, reservation)
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not connect. Try again later.", err))
	}
	if err := connection.SendEvent(setupCtx, voiceAgentSessionUpdate(agentID)); err != nil {
		connection.Close()
		h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
		h.releaseVoiceAgentLimit(release)
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not be initialized. Try again later.", err)
	}

	var providerSessionID string
	for providerSessionID == "" {
		eventType, payload, readErr := connection.ReadProviderMessage(setupCtx)
		if readErr != nil {
			connection.Close()
			h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
			h.releaseVoiceAgentLimit(release)
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			return nil, nil, publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not be initialized. Try again later.", readErr)
		}
		switch eventType {
		case "session.ready":
			var ready struct {
				SessionID string `json:"session_id"`
			}
			if json.Unmarshal(payload, &ready) != nil || !validAssemblyAIVoiceAgentSessionID(ready.SessionID) {
				connection.Close()
				h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
				h.releaseVoiceAgentLimit(release)
				return nil, nil, publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not be initialized. Try again later.", nil)
			}
			providerSessionID = ready.SessionID
		case "session.error":
			connection.Close()
			h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
			h.releaseVoiceAgentLimit(release)
			return nil, nil, publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not be initialized. Try again later.", nil)
		default:
			// The provider must acknowledge session.update before audio is sent.
			connection.Close()
			h.settleVoiceProviderCreditRecordWithUsage(userID, reservation, 1, "server_elapsed")
			h.releaseVoiceAgentLimit(release)
			return nil, nil, publicws.Failure(http.StatusBadGateway, "VOICE_AGENT_PROVIDER_FAILED", "Live Mode could not be initialized. Try again later.", nil)
		}
	}

	session := &publicVoiceAgentSession{
		provider: connection, providerService: provider, handler: h, userID: userID,
		workspaceID: workspaceID, request: request, reservation: reservation,
		providerSessionID: providerSessionID, startedAt: time.Now(), releaseLimit: release,
		deletionStatus: "unconfirmed", voice: voice.Name,
	}
	readyPayload, err := json.Marshal(map[string]any{
		"maxSessionDurationSeconds": assemblyAIVoiceAgentMaxSessionDurationSeconds,
		"locale":                    voice.Locale,
		"voice":                     voice.Name,
		"creditReceipt": voiceCreditReceipt{
			ID: reservation.ID, ReservationID: reservation.ID, OperationType: "voice",
			Provider: "assemblyai", Mode: "voice_agent", Status: "claimed",
			ReservedUsdMicros: reservation.ReservedUsdMicros, PolicyVersion: reservation.PolicyVersion,
		},
	})
	if err != nil {
		outcome := session.Close()
		return nil, nil, publicws.Failure(http.StatusInternalServerError, "VOICE_AGENT_UNAVAILABLE", "Live Mode is temporarily unavailable.", errors.New(outcome.DeletionStatus))
	}
	session.unregister = h.registerRealtimeSession(session, func() { session.Close() })
	return session, readyPayload, nil
}

func voiceAgentSessionUpdate(agentID string) map[string]any {
	return map[string]any{
		"type":    "session.update",
		"session": map[string]string{"agent_id": agentID},
	}
}

func (h *Handler) releaseVoiceAgentReservation(userID string, reservation voiceCreditReservation) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.store.ReleaseUSDReservation(ctx, reservation.ID, userID, reservation.ID+":release"); err != nil {
		h.logger.Error("Voice Agent reservation release failed", "reservation_id", reservation.ID, "error_type", fmt.Sprintf("%T", err))
	}
}

func (h *Handler) releaseVoiceAgentLimit(release func()) {
	if release != nil {
		release()
	}
}

type voiceAgentPendingTool struct {
	callID    string
	name      string
	arguments json.RawMessage
}

type publicVoiceAgentSession struct {
	provider          assemblyAIVoiceAgentConnection
	providerService   assemblyAIVoiceAgentProvider
	handler           *Handler
	userID            string
	workspaceID       string
	request           publicws.VoiceAgentRequest
	reservation       voiceCreditReservation
	providerSessionID string
	startedAt         time.Time
	voice             string

	mu                    sync.Mutex
	deleteMu              sync.Mutex
	settled               bool
	receipt               json.RawMessage
	deletionAttempted     bool
	deletionStatus        string
	providerEnded         bool
	terminationSent       bool
	terminated            bool
	lastUserTranscript    string
	pendingToolCalls      []voiceAgentPendingTool
	toolCallCount         int
	pendingProtocolEvents []agentProtocolEvent
	releaseLimit          func()
	closeOnce             sync.Once
	unregister            func()
}

type agentProtocolEvent struct {
	eventType string
	payload   json.RawMessage
}

func (s *publicVoiceAgentSession) SendAudio(ctx context.Context, frame []byte) error {
	if len(frame) < 2 || len(frame) > maxVoiceAgentAudioBytes || len(frame)%2 != 0 {
		return errAssemblyAIInvalidPCMFrame
	}
	s.mu.Lock()
	closed := s.terminated
	s.mu.Unlock()
	if closed {
		return errAssemblyAIProviderFailure
	}
	return s.provider.SendAudio(ctx, frame)
}

func (s *publicVoiceAgentSession) ReadProviderMessage(ctx context.Context) (string, json.RawMessage, error) {
	for {
		s.mu.Lock()
		if len(s.pendingProtocolEvents) > 0 {
			event := s.pendingProtocolEvents[0]
			s.pendingProtocolEvents = s.pendingProtocolEvents[1:]
			s.mu.Unlock()
			return event.eventType, event.payload, nil
		}
		if s.terminated {
			s.mu.Unlock()
			return "Termination", nil, nil
		}
		s.mu.Unlock()

		eventType, payload, err := s.provider.ReadProviderMessage(ctx)
		if err != nil {
			return "", nil, err
		}
		switch eventType {
		case "session.ready", "session.updated":
			// These echo session IDs, resume tokens, and the full prompt/config.
			continue
		case "session.error":
			receipt, _ := s.settle(s.elapsedDurationMillis(), "server_elapsed")
			deletionStatus := s.deleteProviderSession()
			s.markTerminated()
			failure, _ := json.Marshal(map[string]any{
				"message":        "The live voice session stopped unexpectedly.",
				"deletionStatus": deletionStatus,
				"creditReceipt":  receipt,
			})
			s.enqueue(agentProtocolEvent{eventType: "Termination"})
			return "AskoloVoiceAgentFailed", failure, nil
		case "session.ended":
			durationMS, source := s.providerDuration(payload)
			receipt, settleErr := s.settle(durationMS, source)
			if settleErr != nil {
				return "", nil, settleErr
			}
			deletionStatus := s.deleteProviderSession()
			s.markProviderEnded()
			ended, _ := json.Marshal(map[string]any{
				"reason": "provider_ended", "deletionStatus": deletionStatus,
				"creditReceipt": receipt,
			})
			s.enqueue(agentProtocolEvent{eventType: "Termination"})
			return "AskoloVoiceAgentEnded", ended, nil
		case "tool.call":
			var call struct {
				CallID    string          `json:"call_id"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(payload, &call) != nil || !validVoiceAgentCallID(call.CallID) ||
				len(call.Arguments) > 8192 || !isAssistantJSONObject(call.Arguments) {
				return "", nil, errAssemblyAIProviderFailure
			}
			s.mu.Lock()
			if len(s.pendingToolCalls) >= maxVoiceAgentToolCalls {
				s.mu.Unlock()
				return "", nil, errAssemblyAIProviderFailure
			}
			s.pendingToolCalls = append(s.pendingToolCalls, voiceAgentPendingTool{
				callID: call.CallID, name: call.Name,
				arguments: append(json.RawMessage(nil), call.Arguments...),
			})
			s.mu.Unlock()
			continue
		case "reply.done":
			var reply struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(payload, &reply) != nil || reply.Status != "completed" {
				s.mu.Lock()
				s.pendingToolCalls = nil
				s.mu.Unlock()
				continue
			}
			s.mu.Lock()
			calls := append([]voiceAgentPendingTool(nil), s.pendingToolCalls...)
			s.pendingToolCalls = nil
			s.mu.Unlock()
			for _, call := range calls {
				eventType, eventPayload, callErr := s.executeAssistantRequest(ctx, call)
				if callErr != nil {
					return "", nil, callErr
				}
				s.enqueue(agentProtocolEvent{eventType: eventType, payload: eventPayload})
			}
			continue
		case "transcript.user", "transcript.user.delta", "transcript.agent.delta", "transcript.agent":
			normalized, finalUserText, ok := normalizeVoiceAgentTranscriptEvent(eventType, payload)
			if !ok {
				continue
			}
			if eventType == "transcript.user" {
				s.mu.Lock()
				s.lastUserTranscript = finalUserText
				s.mu.Unlock()
			}
			return eventType, normalized, nil
		case "reply.audio":
			var event struct {
				Data string `json:"data"`
			}
			if json.Unmarshal(payload, &event) != nil || len(event.Data) > assemblyAIVoiceAgentMaxMessage {
				continue
			}
			if _, err := decodeVoiceAgentAudio(event.Data); err != nil {
				continue
			}
			return eventType, payload, nil
		case "input.speech.started", "input.speech.stopped", "reply.started", "reply.interrupted":
			return eventType, payload, nil
		default:
			// Do not forward unknown provider events that might include session
			// configuration, identifiers, or recovery material.
			continue
		}
	}
}

func (s *publicVoiceAgentSession) executeAssistantRequest(ctx context.Context, call voiceAgentPendingTool) (string, json.RawMessage, error) {
	if _, ok := s.handler.toolRegistry().lookup(call.name); !ok {
		s.sendToolResult(ctx, call.callID, map[string]any{"status": "not_allowed"}, true)
		return "AskoloVoiceAgentActionRefused", voiceAgentJSON(map[string]string{"message": "That action is not available."}), nil
	}

	s.mu.Lock()
	s.toolCallCount++
	callCount := s.toolCallCount
	transcript := s.lastUserTranscript
	s.lastUserTranscript = ""
	s.mu.Unlock()
	if callCount > maxVoiceAgentToolCalls || transcript == "" {
		s.sendToolResult(ctx, call.callID, map[string]any{"status": "not_run"}, true)
		return "AskoloVoiceAgentActionRefused", voiceAgentJSON(map[string]string{
			"message": "I could not prepare an action from the latest spoken request. Please try again.",
		}), nil
	}

	if strings.TrimSpace(transcript) == "" {
		s.sendToolResult(ctx, call.callID, map[string]any{"status": "not_run"}, true)
		return "AskoloVoiceAgentActionRefused", voiceAgentJSON(map[string]string{"message": "I could not prepare an action from that request."}), nil
	}
	keyDigest := sha256.Sum256(append(
		append([]byte(s.request.IdempotencyKey+"\x00"+call.callID+"\x00"+call.name+"\x00"), call.arguments...),
		0,
	))
	request := publicws.AssistantRequest{
		ConversationID: s.request.ConversationID,
		Transcript:     transcript,
		IdempotencyKey: "voice-agent-" + hex.EncodeToString(keyDigest[:]),
		PolicyVersion:  s.request.AssistantPolicyVersion,
		ToolName:       call.name,
		ToolArguments:  call.arguments,
	}
	run, err := s.handler.RunAssistant(ctx, s.userID, s.workspaceID, request, nil)
	if err != nil {
		s.sendToolResult(ctx, call.callID, map[string]any{"status": "unavailable"}, true)
		message := "Askolo could not prepare an action. You can continue speaking or try again."
		var operation *publicws.Error
		if errors.As(err, &operation) && operation.Code == "AI_CREDITS_EXHAUSTED" {
			message = operation.Message
		}
		return "AskoloVoiceAgentActionFailed", voiceAgentJSON(map[string]string{"message": message}), nil
	}
	var runState struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(run, &runState)
	resultStatus := "prepared"
	if runState.State == "needs_confirmation" {
		resultStatus = "confirmation_required"
	} else if runState.State == "completed" {
		resultStatus = "complete"
	}
	s.sendToolResult(ctx, call.callID, map[string]any{
		"status":  resultStatus,
		"message": "Askolo prepared the result. The user must review and confirm any change in the app.",
	}, false)
	return "AskoloAssistantRun", run, nil
}

func (s *publicVoiceAgentSession) sendToolResult(ctx context.Context, callID string, result any, isError bool) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, voiceAgentSendTimeout)
	defer cancel()
	_ = s.provider.SendEvent(sendCtx, map[string]any{
		"type": "tool.result", "call_id": callID, "result": string(encoded), "is_error": isError,
	})
}

func (s *publicVoiceAgentSession) SendTermination(ctx context.Context) error {
	if err := s.provider.SendEvent(ctx, map[string]string{"type": "session.end"}); err != nil {
		return err
	}
	s.mu.Lock()
	s.terminationSent = true
	s.mu.Unlock()
	return nil
}

func (s *publicVoiceAgentSession) MaxDuration() time.Duration {
	return time.Duration(assemblyAIVoiceAgentMaxSessionDurationSeconds) * time.Second
}

func (s *publicVoiceAgentSession) Close() publicws.VoiceAgentOutcome {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		shouldTerminate := !s.providerEnded && !s.terminationSent
		s.mu.Unlock()
		if shouldTerminate {
			ctx, cancel := context.WithTimeout(context.Background(), voiceAgentStopTimeout)
			_ = s.SendTermination(ctx)
			cancel()
		}
		s.provider.Close()
		s.mu.Lock()
		isSettled := s.settled
		s.mu.Unlock()
		if !isSettled {
			if _, err := s.settle(s.elapsedDurationMillis(), "server_elapsed"); err != nil && s.handler != nil {
				s.handler.logger.Error("Voice Agent settlement failed", "reservation_id", s.reservation.ID, "error_type", fmt.Sprintf("%T", err))
			}
		}
		s.deleteProviderSession()
		s.markTerminated()
		if s.releaseLimit != nil {
			s.releaseLimit()
			s.releaseLimit = nil
		}
		if s.unregister != nil {
			s.unregister()
			s.unregister = nil
		}
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return publicws.VoiceAgentOutcome{
		DeletionStatus: s.deletionStatus,
		CreditReceipt:  append(json.RawMessage(nil), s.receipt...),
	}
}

func (s *publicVoiceAgentSession) settle(durationMS int64, source string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return append(json.RawMessage(nil), s.receipt...), nil
	}
	if durationMS < 1 {
		durationMS = 1
	}
	maxMS := int64(assemblyAIVoiceAgentMaxSessionDurationSeconds) * 1000
	if durationMS > maxMS {
		durationMS = maxMS
	}
	receipt, err := s.handler.settleVoiceProviderCreditRecordWithUsage(
		s.userID, s.reservation, durationMS, source,
	)
	if err != nil {
		return nil, err
	}
	s.receipt, _ = json.Marshal(receipt)
	s.settled = true
	return append(json.RawMessage(nil), s.receipt...), nil
}

func (s *publicVoiceAgentSession) providerDuration(payload json.RawMessage) (int64, string) {
	var event struct {
		SessionDurationSeconds float64 `json:"session_duration_seconds"`
	}
	if json.Unmarshal(payload, &event) == nil {
		if durationMS, ok := durationSecondsMillis(event.SessionDurationSeconds); ok {
			return durationMS, "assemblyai_session_duration"
		}
	}
	return s.elapsedDurationMillis(), "server_elapsed"
}

func (s *publicVoiceAgentSession) elapsedDurationMillis() int64 {
	if s == nil || s.startedAt.IsZero() {
		return 1
	}
	return time.Since(s.startedAt).Milliseconds()
}

func (s *publicVoiceAgentSession) deleteProviderSession() string {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()
	s.mu.Lock()
	if s.deletionAttempted {
		status := s.deletionStatus
		s.mu.Unlock()
		return status
	}
	s.deletionAttempted = true
	sessionID := s.providerSessionID
	s.mu.Unlock()

	status := "unconfirmed"
	if validAssemblyAIVoiceAgentSessionID(sessionID) {
		for attempt := 0; attempt < 2; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), voiceAgentDeleteTimeout)
			err := s.providerService.DeleteVoiceAgentSession(ctx, sessionID)
			cancel()
			if err == nil {
				status = "soft_deleted"
				break
			}
			if attempt == 0 {
				time.Sleep(150 * time.Millisecond)
			}
		}
	}
	s.mu.Lock()
	s.deletionStatus = status
	s.mu.Unlock()
	if status != "soft_deleted" && s.handler != nil && s.handler.logger != nil {
		s.handler.logger.Warn("AssemblyAI Voice Agent session deletion unconfirmed", "reservation_id", s.reservation.ID)
	}
	return status
}

func (s *publicVoiceAgentSession) enqueue(event agentProtocolEvent) {
	s.mu.Lock()
	s.pendingProtocolEvents = append(s.pendingProtocolEvents, event)
	s.mu.Unlock()
}

func (s *publicVoiceAgentSession) markProviderEnded() {
	s.mu.Lock()
	s.providerEnded = true
	s.mu.Unlock()
}

func (s *publicVoiceAgentSession) markTerminated() {
	s.mu.Lock()
	s.terminated = true
	s.lastUserTranscript = ""
	s.pendingToolCalls = nil
	s.mu.Unlock()
}

func validVoiceAgentCallID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && !strings.ContainsRune("._:-", r) {
			return false
		}
	}
	return true
}

func decodeVoiceAgentAudio(encoded string) ([]byte, error) {
	if encoded == "" || len(encoded) > assemblyAIVoiceAgentMaxMessage {
		return nil, errAssemblyAIInvalidPCMFrame
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < 2 || len(data) > assemblyAIVoiceAgentMaxMessage || len(data)%2 != 0 {
		return nil, errAssemblyAIInvalidPCMFrame
	}
	return data, nil
}

func voiceAgentJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return payload
}
