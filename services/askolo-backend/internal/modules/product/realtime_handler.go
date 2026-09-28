package product

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	policy "askolo/backend/internal/platform/authorization"
	"github.com/coder/websocket"
)

const (
	realtimeStartTimeout       = 10 * time.Second
	realtimeProviderSetupLimit = 20 * time.Second
	realtimeTerminationTimeout = 3 * time.Second
	realtimeWriteDeadlineExtra = 60 * time.Second
	realtimeClientIdleTimeout  = 45 * time.Second
	realtimeStartRateLimit     = 5
	realtimeHeartbeatInterval  = 15 * time.Second
	realtimeMaxActiveSessions  = 128
	realtimeMaxSessionsPerUser = 2
	maxRealtimeStartMessage    = 4096
)

var errRealtimeClientProtocol = errors.New("invalid realtime client protocol")

type realtimeSessionLimiter struct {
	mu     sync.Mutex
	active int
	users  map[string]int
}

func newRealtimeSessionLimiter() *realtimeSessionLimiter {
	return &realtimeSessionLimiter{users: make(map[string]int)}
}

func (l *realtimeSessionLimiter) acquire(userID string) (func(), bool) {
	l.mu.Lock()
	if l.active >= realtimeMaxActiveSessions || l.users[userID] >= realtimeMaxSessionsPerUser {
		l.mu.Unlock()
		return nil, false
	}
	l.active++
	l.users[userID]++
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			l.active--
			l.users[userID]--
			if l.users[userID] == 0 {
				delete(l.users, userID)
			}
			l.mu.Unlock()
		})
	}, true
}

type realtimeStartMessage struct {
	Type           string `json:"type"`
	IdempotencyKey string `json:"idempotencyKey"`
	PolicyVersion  int    `json:"policyVersion"`
}

type realtimeClientEvent struct {
	Type                   string              `json:"type"`
	Code                   string              `json:"code,omitempty"`
	Message                string              `json:"message,omitempty"`
	MaxSessionDurationSecs int                 `json:"maxSessionDurationSeconds,omitempty"`
	CreditReceipt          *voiceCreditReceipt `json:"creditReceipt,omitempty"`
}

type realtimeClientResult struct {
	terminationSent bool
	err             error
}

type realtimeProviderResult struct {
	event assemblyAIRealtimeEvent
	err   error
}

type realtimeSessionOutcome string

const (
	realtimeOutcomeClientTerminated       realtimeSessionOutcome = "client_terminated"
	realtimeOutcomeClientTerminateTimeout realtimeSessionOutcome = "client_termination_timeout"
	realtimeOutcomeProviderTerminated     realtimeSessionOutcome = "provider_terminated"
	realtimeOutcomeClientIdle             realtimeSessionOutcome = "client_idle_timeout"
	realtimeOutcomeClientDisconnected     realtimeSessionOutcome = "client_disconnected"
	realtimeOutcomeProviderFailed         realtimeSessionOutcome = "provider_failed"
	realtimeOutcomeInvalidAudio           realtimeSessionOutcome = "invalid_audio"
	realtimeOutcomeInvalidMessage         realtimeSessionOutcome = "invalid_client_message"
	realtimeOutcomeDurationLimit          realtimeSessionOutcome = "duration_limit"
)

func (h *Handler) allowRealtimeConnectionStart(ctx context.Context, userID string) (bool, error) {
	secret := strings.TrimSpace(h.authRateLimitSecret)
	if len([]byte(secret)) < 32 {
		return false, errors.New("realtime start rate-limit secret is not configured")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("askolo:realtime-start:v1\x00" + userID))
	return h.store.AllowAuthRateLimitBucket(
		ctx,
		hex.EncodeToString(mac.Sum(nil)),
		realtimeStartRateLimit,
		time.Minute,
	)
}

func (h *Handler) realtimeTranscription(w http.ResponseWriter, r *http.Request) {
	origin, allowed := h.allowedRealtimeOrigin(r)
	if !allowed {
		writeError(w, http.StatusForbidden, "INVALID_ORIGIN", "This voice session origin is not allowed.")
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
	allowed, err := h.allowRealtimeConnectionStart(r.Context(), userID)
	if err != nil {
		h.storeError(w, "realtime connection rate limit failed", err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "VOICE_START_RATE_LIMIT", "Too many live transcription starts were requested.")
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

	if h.realtimeLimiter == nil {
		if h.logger != nil {
			h.logger.Error("realtime session limiter is not initialized")
		}
		writeError(w, http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.")
		return
	}
	releaseLimit, acquired := h.realtimeLimiter.acquire(userID)
	if !acquired {
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, "VOICE_SESSION_LIMIT", "Too many live transcription sessions are active.")
		return
	}
	defer releaseLimit()

	// The HTTP server has a short default write deadline. Extend it only for
	// this upgrade so a valid 180-second WebSocket session can finish.
	writeDeadline := time.Now().Add(time.Duration(assemblyAIRealtimeMaxSessionDurationSeconds)*time.Second + realtimeWriteDeadlineExtra)
	if err := http.NewResponseController(w).SetWriteDeadline(writeDeadline); err != nil {
		writeError(w, http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  []string{origin.Host},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(assemblyAIRealtimeMaxMessageBytes)

	startCtx, cancelStart := context.WithTimeout(r.Context(), realtimeStartTimeout)
	messageType, payload, err := conn.Read(startCtx)
	cancelStart()
	if err != nil || messageType != websocket.MessageText || len(payload) > maxRealtimeStartMessage {
		h.writeRealtimeError(conn, "INVALID_SESSION_START", "The live transcription session could not be started.")
		return
	}
	start, ok := decodeRealtimeStart(payload)
	if !ok {
		h.writeRealtimeError(conn, "INVALID_SESSION_START", "The live transcription session could not be started.")
		return
	}

	reservation, failure := h.reserveVoiceProviderCreditRequest(
		r.Context(),
		userID,
		"realtime",
		strings.TrimSpace(start.IdempotencyKey),
		start.PolicyVersion,
	)
	if failure != nil {
		if failure.err != nil {
			h.logger.Error(failure.operation, "error", failure.err)
			h.writeRealtimeError(conn, "INTERNAL_ERROR", "The live transcription session could not be started.")
			return
		}
		h.writeRealtimeError(conn, failure.code, failure.message)
		return
	}
	providerStarted := false
	defer func() {
		if providerStarted {
			return
		}
		if releaseErr := h.store.ReleaseAICreditReservation(context.Background(), reservation.ID, userID, reservation.ID+":release"); releaseErr != nil {
			h.logger.Error("voice credit reservation release failed", "reservation_id", reservation.ID, "error", releaseErr)
		}
	}()

	setupCtx, cancelSetup := context.WithTimeout(r.Context(), realtimeProviderSetupLimit)
	token, err := h.assemblyAI.RealtimeToken(setupCtx)
	if err == nil {
		var provider assemblyAIRealtimeSession
		provider, err = h.assemblyAI.OpenRealtimeSession(setupCtx, token)
		if err == nil {
			providerStarted = true
			providerStartedAt := time.Now()
			defer provider.Close()
			cancelSetup()

			receipt, settleErr := h.settleVoiceProviderCreditRecord(userID, reservation)
			if settleErr != nil {
				h.logger.Error("voice credit settlement failed", "reservation_id", reservation.ID, "error", settleErr)
				h.writeRealtimeError(conn, "AI_CREDIT_SETTLEMENT_FAILED", "The provider session started, but its credit charge could not be confirmed. Check your credit history before retrying.")
				h.terminateRealtimeProvider(provider)
				return
			}
			ready := realtimeClientEvent{
				Type: "AskoloReady", MaxSessionDurationSecs: assemblyAIRealtimeMaxSessionDurationSeconds,
				CreditReceipt: &receipt,
			}
			readyCtx, cancelReady := context.WithDeadline(
				r.Context(),
				providerStartedAt.Add(time.Duration(assemblyAIRealtimeMaxSessionDurationSeconds)*time.Second),
			)
			writeErr := writeRealtimeClientEvent(readyCtx, conn, ready)
			cancelReady()
			if writeErr != nil {
				h.terminateRealtimeProvider(provider)
				return
			}
			outcome := h.relayRealtimeSession(r.Context(), conn, provider, providerStartedAt)
			h.logger.Info(
				"realtime voice session ended",
				"user_id", userID,
				"reservation_id", reservation.ID,
				"outcome", string(outcome),
				"duration_ms", time.Since(providerStartedAt).Milliseconds(),
			)
			return
		}
	}
	cancelSetup()
	if r.Context().Err() != nil {
		return
	}
	h.writeRealtimeError(conn, "VOICE_PROVIDER_FAILED", "Real-time transcription could not be started. Try recorded transcription instead.")
}

func (h *Handler) allowedRealtimeOrigin(r *http.Request) (*url.URL, bool) {
	values := r.Header.Values("Origin")
	if len(values) != 1 {
		return nil, false
	}
	origin, err := url.Parse(values[0])
	if err != nil || origin.User != nil || origin.Host == "" || origin.Path != "" ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return nil, false
	}
	if canonical := strings.TrimSpace(h.canonicalOrigin); canonical != "" {
		expected, parseErr := url.Parse(canonical)
		expectedIsLoopbackHTTP := parseErr == nil && expected.Scheme == "http" &&
			isLoopbackWebsocketHost(expected.Hostname())
		if parseErr != nil || expected.User != nil || expected.Host == "" ||
			expected.Path != "" || expected.RawQuery != "" || expected.Fragment != "" ||
			(expected.Scheme != "https" && !expectedIsLoopbackHTTP) ||
			!strings.EqualFold(origin.Scheme, expected.Scheme) || !strings.EqualFold(origin.Host, expected.Host) {
			return nil, false
		}
		return origin, true
	}

	if origin.Scheme != "http" && origin.Scheme != "https" {
		return nil, false
	}
	if origin.Scheme == "http" && !isLoopbackWebsocketHost(origin.Hostname()) {
		return nil, false
	}
	expectedHost := strings.TrimSpace(r.Host)
	if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
		expectedHost = forwardedHost
	}
	return origin, expectedHost != "" && strings.EqualFold(origin.Host, expectedHost)
}

func decodeRealtimeStart(payload []byte) (realtimeStartMessage, bool) {
	var start realtimeStartMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&start); err != nil {
		return realtimeStartMessage{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return realtimeStartMessage{}, false
	}
	if start.Type != "Start" || strings.TrimSpace(start.IdempotencyKey) == "" ||
		len(start.IdempotencyKey) > 200 || strings.TrimSpace(start.IdempotencyKey) != start.IdempotencyKey ||
		start.PolicyVersion < 1 {
		return realtimeStartMessage{}, false
	}
	return start, true
}

func (h *Handler) writeRealtimeError(conn *websocket.Conn, code, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = writeRealtimeClientEvent(ctx, conn, realtimeClientEvent{Type: "AskoloError", Code: code, Message: message})
}

func writeRealtimeClientEvent(ctx context.Context, conn *websocket.Conn, event realtimeClientEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, payload)
}

func (h *Handler) terminateRealtimeProvider(provider assemblyAIRealtimeSession) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := provider.SendTermination(ctx); err != nil && !errors.Is(err, context.Canceled) {
		h.logger.Warn("realtime provider termination could not be confirmed", "provider", "assemblyai")
	}
}

func (h *Handler) relayRealtimeSession(
	ctx context.Context,
	client *websocket.Conn,
	provider assemblyAIRealtimeSession,
	sessionStartedAt time.Time,
) realtimeSessionOutcome {
	relayCtx, cancelRelay := context.WithCancel(ctx)
	defer cancelRelay()
	pcmCtx, cancelPCM := context.WithCancel(relayCtx)
	defer cancelPCM()
	stopSendingPCM := make(chan struct{})
	var stopSendingPCMOnce sync.Once
	stopPCM := func() {
		stopSendingPCMOnce.Do(func() {
			close(stopSendingPCM)
			cancelPCM()
		})
	}

	clientResults := make(chan realtimeClientResult, 1)
	clientActivity := make(chan struct{}, 1)
	go func() {
		terminationSent, err := relayRealtimeClient(relayCtx, client, provider, pcmCtx, stopSendingPCM, clientActivity)
		clientResults <- realtimeClientResult{terminationSent: terminationSent, err: err}
	}()

	providerResults := make(chan realtimeProviderResult, 8)
	go func() {
		for {
			event, err := provider.ReadProviderMessage(relayCtx)
			result := realtimeProviderResult{event: event, err: err}
			select {
			case providerResults <- result:
			case <-relayCtx.Done():
				return
			}
			if err != nil || event.Type == "Termination" {
				return
			}
		}
	}()

	sessionDeadline := sessionStartedAt.Add(time.Duration(assemblyAIRealtimeMaxSessionDurationSeconds) * time.Second)
	remainingSessionTime := time.Until(sessionDeadline)
	if remainingSessionTime < 0 {
		remainingSessionTime = 0
	}
	sessionTimer := time.NewTimer(remainingSessionTime)
	defer sessionTimer.Stop()
	idleTimeout := h.realtimeIdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = realtimeClientIdleTimeout
	}
	idleTimer := time.NewTimer(idleTimeout)
	defer idleTimer.Stop()
	var terminationTimer *time.Timer
	defer func() {
		if terminationTimer != nil {
			terminationTimer.Stop()
		}
	}()
	var terminationTimerC <-chan time.Time
	sessionTimerC := sessionTimer.C
	terminationSent := false
	sessionLimitTriggered := false
	clientResultsC := (<-chan realtimeClientResult)(clientResults)

	sendTermination := func() {
		if terminationSent {
			return
		}
		terminationSent = true
		h.terminateRealtimeProvider(provider)
	}
	resetIdleTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleTimeout)
	}

	for {
		select {
		case result := <-clientResultsC:
			clientResultsC = nil
			if result.terminationSent {
				sendTermination()
				sessionTimer.Stop()
				sessionTimerC = nil
				if terminationTimer == nil {
					terminationTimer = time.NewTimer(realtimeTerminationTimeout)
					terminationTimerC = terminationTimer.C
				}
				continue
			}
			if sessionLimitTriggered && errors.Is(result.err, context.Canceled) {
				clientResultsC = nil
				continue
			}
			if errors.Is(result.err, errAssemblyAIInvalidPCMFrame) {
				h.writeRealtimeError(client, "INVALID_AUDIO_FRAME", "The live audio frame is invalid.")
				sendTermination()
				return realtimeOutcomeInvalidAudio
			}
			if errors.Is(result.err, errRealtimeClientProtocol) {
				h.writeRealtimeError(client, "INVALID_CLIENT_MESSAGE", "The live transcription session received an unsupported message.")
				sendTermination()
				return realtimeOutcomeInvalidMessage
			}
			sendTermination()
			return realtimeOutcomeClientDisconnected

		case result := <-providerResults:
			if result.err != nil {
				if ctx.Err() == nil {
					h.writeRealtimeError(client, "VOICE_PROVIDER_FAILED", "Real-time transcription stopped unexpectedly. Try recorded transcription instead.")
					return realtimeOutcomeProviderFailed
				}
				return realtimeOutcomeClientDisconnected
			}
			if err := writeRealtimeProviderEvent(ctx, client, result.event); err != nil {
				sendTermination()
				return realtimeOutcomeClientDisconnected
			}
			if result.event.Type == "Termination" {
				if sessionLimitTriggered {
					return realtimeOutcomeDurationLimit
				}
				if terminationSent {
					return realtimeOutcomeClientTerminated
				}
				return realtimeOutcomeProviderTerminated
			}

		case <-clientActivity:
			resetIdleTimer()

		case <-idleTimer.C:
			h.writeRealtimeError(client, "CLIENT_IDLE_TIMEOUT", "The live transcription session ended after an idle period.")
			sendTermination()
			return realtimeOutcomeClientIdle

		case <-sessionTimerC:
			sessionTimerC = nil
			sessionLimitTriggered = true
			stopPCM()
			sendTermination()
			terminationTimer = time.NewTimer(realtimeTerminationTimeout)
			terminationTimerC = terminationTimer.C

			limitCtx, cancelLimit := context.WithTimeout(ctx, realtimeTerminationTimeout)
			err := writeRealtimeClientEvent(limitCtx, client, realtimeClientEvent{
				Type: "AskoloSessionLimit", Message: "The 180-second live transcription limit was reached.",
			})
			cancelLimit()
			if err != nil {
				return realtimeOutcomeDurationLimit
			}

		case <-terminationTimerC:
			if sessionLimitTriggered {
				return realtimeOutcomeDurationLimit
			}
			return realtimeOutcomeClientTerminateTimeout

		case <-ctx.Done():
			sendTermination()
			return realtimeOutcomeClientDisconnected
		}
	}
}

func relayRealtimeClient(
	ctx context.Context,
	client *websocket.Conn,
	provider assemblyAIRealtimeSession,
	pcmCtx context.Context,
	stopSendingPCM <-chan struct{},
	clientActivity chan<- struct{},
) (bool, error) {
	for {
		messageType, payload, err := client.Read(ctx)
		if err != nil {
			return false, err
		}
		switch messageType {
		case websocket.MessageBinary:
			select {
			case clientActivity <- struct{}{}:
			default:
			}
			select {
			case <-stopSendingPCM:
				continue
			default:
			}
			if err := provider.SendPCMFrame(pcmCtx, payload); err != nil {
				return false, err
			}
		case websocket.MessageText:
			var message struct {
				Type string `json:"type"`
			}
			if len(payload) > maxRealtimeStartMessage {
				return false, errRealtimeClientProtocol
			}
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&message) != nil || decoder.Decode(&struct{}{}) != io.EOF {
				return false, errRealtimeClientProtocol
			}
			switch message.Type {
			case "Heartbeat":
				select {
				case clientActivity <- struct{}{}:
				default:
				}
			case "Terminate":
				select {
				case clientActivity <- struct{}{}:
				default:
				}
				return true, nil
			default:
				return false, errRealtimeClientProtocol
			}
		default:
			return false, errRealtimeClientProtocol
		}
	}
}

func writeRealtimeProviderEvent(ctx context.Context, client *websocket.Conn, event assemblyAIRealtimeEvent) error {
	return client.Write(ctx, websocket.MessageText, event.Payload)
}
