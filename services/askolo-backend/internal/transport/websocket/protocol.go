package websocket

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	policy "askolo/backend/internal/platform/authorization"
	"askolo/backend/internal/platform/publicws"
	"github.com/coder/websocket"
)

const (
	protocolVersion         = 1
	maxProtocolMessageBytes = 64 * 1024
	maxAudioFrameBytes      = 32_000
	minAudioFrameBytes      = 1_600
	protocolStartTimeout    = 10 * time.Second
	protocolIdleTimeout     = 45 * time.Second
	protocolHeartbeat       = 15 * time.Second
	protocolWriteTimeout    = 5 * time.Second
	voiceStopTimeout        = 3 * time.Second
)

type clientEnvelope struct {
	Version       int             `json:"version"`
	Type          string          `json:"type"`
	Sequence      int64           `json:"sequence"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type serverEnvelope struct {
	Version       int             `json:"version"`
	SessionID     string          `json:"sessionId"`
	Sequence      int64           `json:"sequence"`
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

type sessionStartPayload struct {
	ResumeSessionID string `json:"resumeSessionId,omitempty"`
	AfterSequence   int64  `json:"afterSequence,omitempty"`
}

type assistantRunPayload struct {
	ConversationID string `json:"conversationId,omitempty"`
	Transcript     string `json:"transcript"`
	IdempotencyKey string `json:"idempotencyKey"`
	PolicyVersion  int    `json:"policyVersion"`
}

type voiceStartPayload struct {
	IdempotencyKey string `json:"idempotencyKey"`
	PolicyVersion  int    `json:"policyVersion"`
}

type decodedFrame struct {
	messageType websocket.MessageType
	payload     []byte
	err         error
}

type queuedEvent struct {
	eventType          string
	correlationID      string
	payload            json.RawMessage
	assistantRunID     string
	voiceReservationID string
	terminalState      string
	sent               chan error
}

type assistantResult struct {
	correlationID string
	runID         string
	payload       json.RawMessage
	err           error
}

type providerResult struct {
	eventType          string
	payload            json.RawMessage
	voiceReservationID string
	err                error
}

type protocolState struct {
	userID      string
	workspaceID string
	store       *postgres.Store
	session     postgres.PublicWSSession
	conn        *websocket.Conn
	ctx         context.Context
	lastClient  int64
	lastActive  time.Time
	voice       publicws.VoiceSession
	voiceCancel context.CancelFunc
	voiceTimer  *time.Timer
	voiceStop   *time.Timer
	voiceID     string
	assistant   bool
	terminal    string
}

func (h *Handler) allowedOrigin(r *http.Request) (*url.URL, bool) {
	values := r.Header.Values("Origin")
	if len(values) != 1 {
		return nil, false
	}
	origin, err := url.Parse(values[0])
	if err != nil || origin.User != nil || origin.Host == "" ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, false
	}
	if canonical := strings.TrimSpace(h.canonicalOrigin); canonical != "" {
		expected, parseErr := url.Parse(canonical)
		expectedLoopbackHTTP := parseErr == nil && expected.Scheme == "http" && isLoopbackHost(expected.Hostname())
		if parseErr != nil || expected.User != nil || expected.Host == "" ||
			expected.Path != "" || expected.RawQuery != "" || expected.Fragment != "" ||
			(expected.Scheme != "https" && !expectedLoopbackHTTP) ||
			!strings.EqualFold(origin.Scheme, expected.Scheme) ||
			!strings.EqualFold(origin.Host, expected.Host) {
			return nil, false
		}
		return origin, true
	}
	if origin.Scheme != "http" && origin.Scheme != "https" {
		return nil, false
	}
	if origin.Scheme == "http" && !isLoopbackHost(origin.Hostname()) {
		return nil, false
	}
	expectedHost := strings.TrimSpace(r.Host)
	return origin, expectedHost != "" && strings.EqualFold(origin.Host, expectedHost)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *Handler) serveProtocol(w http.ResponseWriter, r *http.Request, userID, workspaceID string, origin *url.URL) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  []string{origin.Host},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxProtocolMessageBytes + 8)

	startCtx, cancelStart := context.WithTimeout(r.Context(), protocolStartTimeout)
	messageType, payload, err := conn.Read(startCtx)
	cancelStart()
	if err != nil || messageType != websocket.MessageText || len(payload) > maxProtocolMessageBytes {
		h.writeHandshakeError(conn, "INVALID_SESSION_START", "The connection could not be started.")
		return
	}
	start, ok := decodeClientEnvelope(payload)
	if !ok || start.Type != "session.start" || start.Sequence != 1 {
		h.writeHandshakeError(conn, "INVALID_SESSION_START", "The connection could not be started.")
		return
	}
	var resume sessionStartPayload
	if !decodePayload(start.Payload, &resume) || resume.AfterSequence < 0 ||
		(resume.ResumeSessionID == "" && resume.AfterSequence != 0) ||
		(resume.ResumeSessionID != "" && !validProtocolID(resume.ResumeSessionID)) {
		h.writeHandshakeError(conn, "INVALID_SESSION_START", "The connection could not be started.")
		return
	}

	session, err := h.store.CreatePublicWSSession(r.Context(), userID, workspaceID, postgres.PublicWSDefaultLease)
	if err != nil {
		switch {
		case errors.Is(err, postgres.ErrPublicWSQuota):
			h.writeHandshakeError(conn, "SESSION_LIMIT", "Too many connections are active. Close another connection and try again.")
		default:
			h.logger.Error("public WebSocket session creation failed", "user_id", userID, "error", err)
			h.writeHandshakeError(conn, "WEBSOCKET_UNAVAILABLE", "The connection is temporarily unavailable.")
		}
		return
	}
	state := &protocolState{
		userID: userID, workspaceID: workspaceID, session: session, conn: conn,
		store: h.store, ctx: r.Context(), lastClient: 0, lastActive: time.Now(), terminal: "cancelled",
	}
	defer state.finish()
	if _, err := h.acceptClient(state, start, payload); err != nil {
		h.writeProtocolError(conn, session.ID, "INVALID_SEQUENCE", "The connection sequence is invalid.")
		return
	}
	if err := h.sendEvent(state, queuedEvent{
		eventType: "session.ready",
		payload:   mustJSON(map[string]any{"protocolVersion": protocolVersion, "leaseSeconds": int(postgres.PublicWSDefaultLease.Seconds()), "resumeMode": "metadata_only"}),
	}); err != nil {
		h.logger.Error("public WebSocket ready event could not be persisted", "session_id", session.ID, "error", err)
		return
	}
	if resume.ResumeSessionID != "" {
		if err := h.sendResumeState(state, resume); err != nil {
			h.writeProtocolError(conn, session.ID, "RESUME_UNAVAILABLE", "The previous session state could not be checked.")
			return
		}
	}

	readCtx, stopRead := context.WithCancel(r.Context())
	defer stopRead()
	frames := make(chan decodedFrame, 4)
	go readFrames(readCtx, conn, frames)
	moduleEvents := make(chan queuedEvent, 16)
	assistantResults := make(chan assistantResult, 1)
	providerEvents := make(chan providerResult, 8)
	idleTimer := time.NewTimer(protocolIdleTimeout)
	defer idleTimer.Stop()
	sessionTimer := time.NewTimer(time.Duration(maxSessionDurationSeconds) * time.Second)
	defer sessionTimer.Stop()
	heartbeat := time.NewTicker(protocolHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case frame := <-frames:
			if frame.err != nil {
				return
			}
			state.lastActive = time.Now()
			resetTimer(idleTimer, protocolIdleTimeout)
			if frame.messageType == websocket.MessageBinary {
				if err := h.handleAudioFrame(state, frame.payload); err != nil {
					h.sendPublicError(state, err)
					return
				}
				continue
			}
			if frame.messageType != websocket.MessageText || len(frame.payload) > maxProtocolMessageBytes {
				h.sendEvent(state, queuedEvent{eventType: "protocol.error", payload: mustJSON(map[string]string{"code": "INVALID_MESSAGE", "message": "The connection received an unsupported message."})})
				return
			}
			message, valid := decodeClientEnvelope(frame.payload)
			if !valid || message.Version != protocolVersion || !validProtocolIDOptional(message.CorrelationID) {
				h.sendEvent(state, queuedEvent{eventType: "protocol.error", payload: mustJSON(map[string]string{"code": "INVALID_MESSAGE", "message": "The connection received an invalid message."})})
				return
			}
			accepted, acceptErr := h.acceptClient(state, message, frame.payload)
			if acceptErr != nil {
				code, publicMessage := sequenceError(acceptErr)
				h.sendEvent(state, queuedEvent{eventType: "protocol.error", correlationID: message.CorrelationID, payload: mustJSON(map[string]string{"code": code, "message": publicMessage})})
				return
			}
			if !accepted {
				if err := h.sendEvent(state, queuedEvent{eventType: "protocol.ack", correlationID: message.CorrelationID, payload: mustJSON(map[string]any{"sequence": message.Sequence, "duplicate": true})}); err != nil {
					return
				}
				continue
			}
			if err := h.dispatchMessage(state, message, moduleEvents, assistantResults, providerEvents); err != nil {
				if errors.Is(err, errClientClosed) {
					return
				}
				h.sendPublicError(state, err)
				var operation *publicws.Error
				if errors.As(err, &operation) && operation.Status == http.StatusBadRequest {
					return
				}
				continue
			}

		case event := <-moduleEvents:
			if event.voiceReservationID != "" && event.voiceReservationID != state.voiceID {
				if event.sent != nil {
					event.sent <- nil
				}
				continue
			}
			if event.eventType == "voice.session_limit" {
				state.stopVoice(true)
			} else if event.eventType == "voice.ended" {
				state.stopVoice(false)
			}
			sendErr := h.sendEvent(state, event)
			if event.sent != nil {
				event.sent <- sendErr
			}
			if sendErr != nil {
				h.logger.Error("public WebSocket module event could not be sent", "session_id", state.session.ID, "error", sendErr)
				return
			}
			if event.eventType == "voice.session_limit" {
				if err := h.sendEvent(state, queuedEvent{
					eventType: "voice.ended", voiceReservationID: event.voiceReservationID,
					terminalState: "completed", payload: mustJSON(map[string]string{"reason": "duration_limit"}),
				}); err != nil {
					return
				}
			}

		case result := <-assistantResults:
			state.assistant = false
			event := queuedEvent{correlationID: result.correlationID, assistantRunID: result.runID}
			if result.err != nil {
				event.eventType = "assistant.run.failed"
				event.terminalState = "failed"
				if operation, ok := result.err.(*publicws.Error); ok {
					event.payload = mustJSON(map[string]string{"code": operation.Code, "message": operation.Message})
				} else if !errors.Is(result.err, context.Canceled) {
					event.payload = mustJSON(map[string]string{"code": "ASSISTANT_UNAVAILABLE", "message": "The assistant could not complete this request."})
				} else {
					event.eventType = "assistant.run.cancelled"
					event.terminalState = "cancelled"
					event.payload = mustJSON(map[string]string{"message": "The assistant request was interrupted. Check its status before retrying."})
				}
			} else {
				event.eventType, event.terminalState = assistantResultType(result.payload)
				event.payload = result.payload
			}
			if err := h.sendEvent(state, event); err != nil {
				return
			}

		case result := <-providerEvents:
			if result.err != nil {
				if result.voiceReservationID != state.voiceID {
					continue
				}
				state.stopVoice(false)
				if r.Context().Err() == nil {
					if err := h.sendEvent(state, queuedEvent{
						eventType: "voice.failed", voiceReservationID: result.voiceReservationID,
						terminalState: "failed",
						payload:       mustJSON(map[string]string{"code": "VOICE_PROVIDER_FAILED", "message": "Real-time transcription stopped unexpectedly. Start a new voice session to continue."}),
					}); err != nil {
						return
					}
				}
				continue
			}
			if result.voiceReservationID != state.voiceID {
				continue
			}
			if result.eventType == "Termination" {
				_ = h.sendEvent(state, queuedEvent{
					eventType: "voice.provider", voiceReservationID: result.voiceReservationID,
					payload: result.payload,
				})
				state.stopVoice(false)
				if err := h.sendEvent(state, queuedEvent{
					eventType: "voice.ended", voiceReservationID: result.voiceReservationID,
					terminalState: "completed", payload: mustJSON(map[string]string{"reason": "provider_terminated"}),
				}); err != nil {
					return
				}
				continue
			}
			if result.eventType == "AskoloSettlement" {
				if err := h.sendEvent(state, queuedEvent{
					eventType: "voice.settlement", voiceReservationID: result.voiceReservationID,
					payload: result.payload,
				}); err != nil {
					return
				}
				continue
			}
			if err := h.sendEvent(state, queuedEvent{
				eventType: "voice.provider", voiceReservationID: result.voiceReservationID,
				payload: result.payload,
			}); err != nil {
				return
			}

		case <-heartbeat.C:
			if err := h.store.TouchPublicWSSession(r.Context(), userID, state.session.ID, postgres.PublicWSDefaultLease); err != nil {
				h.logger.Error("public WebSocket lease renewal failed", "session_id", state.session.ID, "error", err)
				return
			}
			pingCtx, cancel := context.WithTimeout(r.Context(), protocolWriteTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}

		case <-idleTimer.C:
			_ = h.sendEvent(state, queuedEvent{eventType: "session.draining", terminalState: "expired", payload: mustJSON(map[string]string{"reason": "idle_timeout", "resumeMode": "metadata_only"})})
			state.terminal = "expired"
			return

		case <-sessionTimer.C:
			_ = h.sendEvent(state, queuedEvent{eventType: "session.draining", terminalState: "expired", payload: mustJSON(map[string]string{"reason": "duration_limit", "resumeMode": "metadata_only"})})
			state.terminal = "expired"
			return

		case <-r.Context().Done():
			return
		}
	}
}

func readFrames(ctx context.Context, conn *websocket.Conn, frames chan<- decodedFrame) {
	for {
		messageType, payload, err := conn.Read(ctx)
		frame := decodedFrame{messageType: messageType, payload: payload, err: err}
		select {
		case frames <- frame:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func decodeClientEnvelope(payload []byte) (clientEnvelope, bool) {
	var message clientEnvelope
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return clientEnvelope{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return clientEnvelope{}, false
	}
	if message.Version != protocolVersion || message.Sequence <= 0 ||
		message.Type == "" || len(message.Type) > 64 ||
		!validProtocolIDOptional(message.CorrelationID) {
		return clientEnvelope{}, false
	}
	return message, true
}

func decodePayload(payload json.RawMessage, target any) bool {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}

func validProtocolID(value string) bool {
	if value == "" || len(value) > 200 || strings.TrimSpace(value) != value {
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

func validProtocolIDOptional(value string) bool {
	return value == "" || validProtocolID(value)
}

func (h *Handler) acceptClient(state *protocolState, message clientEnvelope, raw []byte) (bool, error) {
	digest := h.messageDigest(raw)
	accepted, err := h.store.AcceptPublicWSClientSequence(
		state.ctx, state.userID, state.session.ID, message.Sequence, digest,
		message.CorrelationID, message.Type,
	)
	if err != nil {
		return false, err
	}
	if accepted.Sequence <= state.lastClient {
		return false, nil
	}
	state.lastClient = accepted.Sequence
	if err := h.store.TouchPublicWSSession(state.ctx, state.userID, state.session.ID, postgres.PublicWSDefaultLease); err != nil {
		return false, err
	}
	return true, nil
}

func (h *Handler) handleAudioFrame(state *protocolState, frame []byte) error {
	if len(frame) < 8+minAudioFrameBytes || len(frame) > 8+maxAudioFrameBytes || (len(frame)-8)%2 != 0 {
		return publicws.Failure(http.StatusBadRequest, "INVALID_AUDIO_FRAME", "The live audio frame is invalid.", nil)
	}
	if state.voice == nil || state.voiceStop != nil {
		return publicws.Failure(http.StatusConflict, "VOICE_NOT_ACTIVE", "Start a live voice session before sending audio.", nil)
	}
	sequence := int64(binary.BigEndian.Uint64(frame[:8]))
	if sequence <= 0 {
		return publicws.Failure(http.StatusBadRequest, "INVALID_SEQUENCE", "The connection sequence is invalid.", nil)
	}
	digest := h.messageDigest(frame)
	accepted, err := h.store.AcceptPublicWSClientSequence(
		state.ctx, state.userID, state.session.ID, sequence, digest, "", "voice.audio",
	)
	if err != nil {
		return err
	}
	if accepted.Sequence <= state.lastClient {
		return nil
	}
	state.lastClient = accepted.Sequence
	if err := h.store.TouchPublicWSSession(state.ctx, state.userID, state.session.ID, postgres.PublicWSDefaultLease); err != nil {
		return err
	}
	sendCtx, cancel := context.WithTimeout(state.ctx, protocolWriteTimeout)
	err = state.voice.SendPCMFrame(sendCtx, frame[8:])
	cancel()
	if err != nil {
		return publicws.Failure(http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "The live audio connection stopped. Start a new voice session to continue.", err)
	}
	return nil
}

func (h *Handler) dispatchMessage(
	state *protocolState,
	message clientEnvelope,
	moduleEvents chan<- queuedEvent,
	assistantResults chan<- assistantResult,
	providerEvents chan<- providerResult,
) error {
	switch message.Type {
	case "session.heartbeat":
		if !decodePayload(message.Payload, &struct{}{}) {
			return invalidClientMessage()
		}
		return h.sendEvent(state, queuedEvent{eventType: "session.pong", correlationID: message.CorrelationID, payload: mustJSON(map[string]any{"receivedSequence": message.Sequence})})

	case "assistant.run.create":
		var input assistantRunPayload
		if !decodePayload(message.Payload, &input) || len(input.IdempotencyKey) > 200 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey {
			return invalidClientMessage()
		}
		if state.assistant {
			return publicws.Failure(http.StatusConflict, "ASSISTANT_RUN_ACTIVE", "An assistant request is already running on this connection.", nil)
		}
		if err := h.authorizeCapability(state, policy.ActionAIExecute); err != nil {
			return err
		}
		if h.services == nil {
			return publicws.Failure(http.StatusServiceUnavailable, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.", nil)
		}
		state.assistant = true
		request := publicws.AssistantRequest{
			ConversationID: input.ConversationID, Transcript: input.Transcript,
			IdempotencyKey: input.IdempotencyKey, PolicyVersion: input.PolicyVersion,
		}
		go func(correlationID string) {
			payload, err := h.services.RunAssistant(state.ctx, state.userID, state.workspaceID, request, func(runID string, startedPayload json.RawMessage) error {
				sent := make(chan error, 1)
				event := queuedEvent{
					eventType: "assistant.run.started", correlationID: correlationID,
					payload: startedPayload, assistantRunID: runID, sent: sent,
				}
				select {
				case moduleEvents <- event:
				case <-state.ctx.Done():
					return state.ctx.Err()
				}
				select {
				case err := <-sent:
					return err
				case <-state.ctx.Done():
					return state.ctx.Err()
				}
			})
			runID, _ := assistantRunMetadata(payload)
			select {
			case assistantResults <- assistantResult{correlationID: correlationID, runID: runID, payload: payload, err: err}:
			case <-state.ctx.Done():
			}
		}(message.CorrelationID)
		return nil

	case "voice.start":
		var input voiceStartPayload
		if !decodePayload(message.Payload, &input) || len(input.IdempotencyKey) > 200 ||
			input.IdempotencyKey == "" || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey ||
			input.PolicyVersion < 1 {
			return invalidClientMessage()
		}
		if state.voice != nil {
			return publicws.Failure(http.StatusConflict, "VOICE_SESSION_ACTIVE", "A live voice session is already active.", nil)
		}
		if err := h.authorizeCapability(state, policy.ActionAIExecute); err != nil {
			return err
		}
		if h.services == nil {
			return publicws.Failure(http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Real-time transcription is temporarily unavailable.", nil)
		}
		voice, readyPayload, err := h.services.StartVoice(state.ctx, state.userID, state.workspaceID, input.IdempotencyKey, input.PolicyVersion)
		if err != nil {
			return err
		}
		state.voice = voice
		state.voiceID = voiceReservationID(readyPayload)
		reservationID := state.voiceID
		voiceCtx, cancel := context.WithCancel(state.ctx)
		state.voiceCancel = cancel
		state.voiceTimer = time.AfterFunc(voice.MaxDuration(), func() {
			select {
			case moduleEvents <- queuedEvent{
				eventType: "voice.session_limit", voiceReservationID: reservationID,
				payload: mustJSON(map[string]string{"message": "The 180-second live transcription limit was reached."}),
			}:
			case <-state.ctx.Done():
			}
		})
		go func(session publicws.VoiceSession, reservationID string) {
			for {
				eventType, payload, err := session.ReadProviderMessage(voiceCtx)
				result := providerResult{eventType: eventType, payload: payload, voiceReservationID: reservationID, err: err}
				select {
				case providerEvents <- result:
				case <-voiceCtx.Done():
					return
				}
				if err != nil || eventType == "Termination" {
					return
				}
			}
		}(voice, state.voiceID)
		return h.sendEvent(state, queuedEvent{
			eventType: "voice.ready", correlationID: message.CorrelationID,
			payload: readyPayload, voiceReservationID: state.voiceID,
		})

	case "voice.stop":
		if !decodePayload(message.Payload, &struct{}{}) {
			return invalidClientMessage()
		}
		if state.voice == nil {
			return publicws.Failure(http.StatusConflict, "VOICE_NOT_ACTIVE", "There is no active voice session to stop.", nil)
		}
		if state.voiceStop != nil {
			return nil
		}
		if err := h.sendEvent(state, queuedEvent{
			eventType: "voice.stopping", correlationID: message.CorrelationID,
			voiceReservationID: state.voiceID,
		}); err != nil {
			return err
		}
		terminateCtx, cancel := context.WithTimeout(state.ctx, protocolWriteTimeout)
		err := state.voice.SendTermination(terminateCtx)
		cancel()
		if err != nil {
			return publicws.Failure(http.StatusBadGateway, "VOICE_PROVIDER_FAILED", "The live transcription session could not be stopped.", err)
		}
		reservationID := state.voiceID
		state.voiceStop = time.AfterFunc(voiceStopTimeout, func() {
			select {
			case moduleEvents <- queuedEvent{
				eventType: "voice.ended", voiceReservationID: reservationID,
				terminalState: "completed", payload: mustJSON(map[string]string{"reason": "termination_timeout"}),
			}:
			case <-state.ctx.Done():
			}
		})
		return nil

	case "session.close":
		var input struct {
			Reason string `json:"reason,omitempty"`
		}
		if !decodePayload(message.Payload, &input) || (input.Reason != "" && input.Reason != "completed" && input.Reason != "cancelled") {
			return invalidClientMessage()
		}
		terminal := input.Reason
		if terminal == "" {
			terminal = "cancelled"
		}
		if err := h.sendEvent(state, queuedEvent{eventType: "session.closed", terminalState: terminal, payload: mustJSON(map[string]string{"reason": terminal})}); err != nil {
			return err
		}
		if err := h.store.ClosePublicWSSession(state.ctx, state.userID, state.session.ID, terminal); err != nil {
			return publicws.Failure(http.StatusServiceUnavailable, "WEBSOCKET_UNAVAILABLE", "The connection is temporarily unavailable.", err)
		}
		state.session.Status = "closed"
		state.terminal = terminal
		return errClientClosed
	default:
		return invalidClientMessage()
	}
}

var errClientClosed = errors.New("client closed public WebSocket session")

func (h *Handler) sendPublicError(state *protocolState, err error) {
	var operation *publicws.Error
	if errors.As(err, &operation) {
		_ = h.sendEvent(state, queuedEvent{eventType: "protocol.error", payload: mustJSON(map[string]string{"code": operation.Code, "message": operation.Message})})
		return
	}
	_ = h.sendEvent(state, queuedEvent{eventType: "protocol.error", payload: mustJSON(map[string]string{"code": "WEBSOCKET_UNAVAILABLE", "message": "The connection is temporarily unavailable."})})
}

func (h *Handler) authorizeCapability(state *protocolState, action policy.Action) error {
	decision, err := h.store.Authorize(state.ctx, policy.Input{
		ActorUserID: state.userID, WorkspaceID: state.workspaceID, Action: action,
	})
	if err != nil {
		return publicws.Failure(http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.", err)
	}
	if !decision.Allowed {
		_ = h.store.CreateSecurityEvent(state.ctx, state.userID, "authorization_denied", "", map[string]any{
			"workspace_id": state.workspaceID, "action": action, "reason": decision.Reason,
		})
		return publicws.Failure(http.StatusForbidden, "AUTHORIZATION_DENIED", "You are not allowed to use this operation.", nil)
	}
	return nil
}

func (h *Handler) sendEvent(state *protocolState, event queuedEvent) error {
	if len(event.payload) == 0 {
		event.payload = []byte("{}")
	}
	digest := h.messageDigest(append([]byte(event.eventType+"\x00"+event.correlationID+"\x00"), event.payload...))
	record, err := h.store.AppendPublicWSServerEvent(
		state.ctx, state.userID, state.session.ID, event.correlationID,
		event.eventType, digest, event.assistantRunID, event.voiceReservationID, event.terminalState,
	)
	if err != nil {
		return err
	}
	wire, err := json.Marshal(serverEnvelope{
		Version: protocolVersion, SessionID: state.session.ID, Sequence: record.Sequence,
		Type: event.eventType, CorrelationID: event.correlationID, Payload: event.payload,
	})
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(state.ctx, protocolWriteTimeout)
	defer cancel()
	return state.conn.Write(writeCtx, websocket.MessageText, wire)
}

func (h *Handler) sendResumeState(state *protocolState, input sessionStartPayload) error {
	previous, err := h.store.GetPublicWSSessionForOwner(state.ctx, state.userID, input.ResumeSessionID)
	if err != nil {
		return err
	}
	if previous.WorkspaceID != state.workspaceID || input.AfterSequence > previous.ServerSequence {
		return postgres.ErrPublicWSSessionNotFound
	}
	events, err := h.store.ListPublicWSServerEvents(state.ctx, state.userID, previous.ID, input.AfterSequence, postgres.PublicWSCleanupBatchSize)
	if err != nil {
		return err
	}
	runIDs := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	voiceRestart := false
	for _, event := range events {
		if event.AssistantRunID != "" {
			if _, ok := seen[event.AssistantRunID]; !ok {
				seen[event.AssistantRunID] = struct{}{}
				runIDs = append(runIDs, event.AssistantRunID)
			}
		}
		if event.VoiceReservationID != "" {
			voiceRestart = true
		}
	}
	return h.sendEvent(state, queuedEvent{
		eventType: "session.resume_state",
		payload: mustJSON(map[string]any{
			"previousSessionId":       previous.ID,
			"previousStatus":          previous.Status,
			"previousTerminalState":   previous.TerminalState,
			"previousServerSequence":  previous.ServerSequence,
			"assistantRunIds":         runIDs,
			"voiceRestartRequired":    voiceRestart,
			"replayAvailable":         false,
			"audioOrTranscriptStored": false,
			"resumeAfterSequence":     input.AfterSequence,
			"metadataOnlyResume":      true,
		}),
	})
}

func (h *Handler) writeHandshakeError(conn *websocket.Conn, code, message string) {
	payload, _ := json.Marshal(map[string]any{
		"version": protocolVersion, "sessionId": "", "sequence": 0,
		"type": "protocol.error", "payload": map[string]string{"code": code, "message": message},
	})
	ctx, cancel := context.WithTimeout(context.Background(), protocolWriteTimeout)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, payload)
}

func (h *Handler) writeProtocolError(conn *websocket.Conn, sessionID, code, message string) {
	payload, _ := json.Marshal(serverEnvelope{
		Version: protocolVersion, SessionID: sessionID, Sequence: 0, Type: "protocol.error",
		Payload: mustJSON(map[string]string{"code": code, "message": message}),
	})
	ctx, cancel := context.WithTimeout(context.Background(), protocolWriteTimeout)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, payload)
}

func (h *Handler) messageDigest(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(h.messageHMACSecret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func sequenceError(err error) (string, string) {
	switch {
	case errors.Is(err, postgres.ErrPublicWSSequenceGap):
		return "SEQUENCE_GAP", "A connection message was missed. Reconnect and resume using the session metadata."
	case errors.Is(err, postgres.ErrPublicWSSequenceConflict):
		return "SEQUENCE_CONFLICT", "A sequence number was reused for different content."
	case errors.Is(err, postgres.ErrPublicWSAlreadyTerminal), errors.Is(err, postgres.ErrPublicWSSessionNotFound):
		return "SESSION_CLOSED", "This connection session is no longer active."
	default:
		return "WEBSOCKET_UNAVAILABLE", "The connection is temporarily unavailable."
	}
}

func invalidClientMessage() error {
	return publicws.Failure(http.StatusBadRequest, "INVALID_MESSAGE", "The connection received an invalid message.", nil)
}

func assistantResultType(payload json.RawMessage) (string, string) {
	var result struct {
		State string `json:"state"`
	}
	if json.Unmarshal(payload, &result) != nil {
		return "assistant.run.failed", "failed"
	}
	switch result.State {
	case "planning":
		return "assistant.run.pending", ""
	case "cancelled":
		return "assistant.run.cancelled", "cancelled"
	case "failed":
		return "assistant.run.failed", "failed"
	default:
		return "assistant.run.completed", "completed"
	}
}

func assistantRunMetadata(payload json.RawMessage) (string, string) {
	var result struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	_ = json.Unmarshal(payload, &result)
	return result.ID, result.State
}

func voiceReservationID(payload json.RawMessage) string {
	var result struct {
		CreditReceipt struct {
			ReservationID string `json:"reservationId"`
		} `json:"creditReceipt"`
	}
	_ = json.Unmarshal(payload, &result)
	return result.CreditReceipt.ReservationID
}

func mustJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return payload
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func (state *protocolState) stopVoice(sendTermination bool) {
	if state.voiceCancel != nil {
		state.voiceCancel()
		state.voiceCancel = nil
	}
	if state.voiceTimer != nil {
		state.voiceTimer.Stop()
		state.voiceTimer = nil
	}
	if state.voiceStop != nil {
		state.voiceStop.Stop()
		state.voiceStop = nil
	}
	if state.voice != nil {
		if sendTermination {
			ctx, cancel := context.WithTimeout(context.Background(), protocolWriteTimeout)
			_ = state.voice.SendTermination(ctx)
			cancel()
		}
		state.voice.Close()
		state.voice = nil
	}
	state.voiceID = ""
}

func (state *protocolState) finish() {
	state.stopVoice(true)
	if state.session.ID == "" || state.session.Status != "active" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := state.store.ClosePublicWSSession(ctx, state.userID, state.session.ID, state.terminal); err != nil && !errors.Is(err, postgres.ErrPublicWSSessionNotFound) {
		// The lease will expire if the database cannot record a disconnect.
	}
	state.session.Status = "closed"
}
