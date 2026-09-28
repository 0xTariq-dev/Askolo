package product

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDecodeRealtimeStart(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantOK  bool
	}{
		{name: "valid single-use grant", payload: `{"type":"Start","connectionGrant":"` + strings.Repeat("A", 43) + `"}`, wantOK: true},
		{name: "wrong message type", payload: `{"type":"Terminate","connectionGrant":"` + strings.Repeat("A", 43) + `"}`},
		{name: "missing grant", payload: `{"type":"Start"}`},
		{name: "invalid grant encoding", payload: `{"type":"Start","connectionGrant":"not-a-grant"}`},
		{name: "legacy client-supplied credit fields", payload: `{"type":"Start","idempotencyKey":"voice-key","policyVersion":3}`},
		{name: "unknown field", payload: `{"type":"Start","connectionGrant":"` + strings.Repeat("A", 43) + `","token":"not-accepted"}`},
		{name: "trailing value", payload: `{"type":"Start","connectionGrant":"` + strings.Repeat("A", 43) + `"} {}`},
		{name: "malformed JSON", payload: `not-json`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, ok := decodeRealtimeStart([]byte(test.payload))
			if ok != test.wantOK {
				t.Fatalf("decodeRealtimeStart() accepted = %t, want %t", ok, test.wantOK)
			}
		})
	}
}

func TestRealtimeConnectionGrantHashRequiresCanonicalToken(t *testing.T) {
	valid := strings.Repeat("A", 43)
	if hash, ok := realtimeConnectionGrantHash(valid); !ok || len(hash) != 64 {
		t.Fatalf("realtimeConnectionGrantHash(valid) = (%q, %t), want a SHA-256 hex digest", hash, ok)
	}
	for _, token := range []string{"", "short", valid + "=", strings.Repeat("A", 44), strings.Repeat("A", 42) + "!"} {
		if _, ok := realtimeConnectionGrantHash(token); ok {
			t.Fatalf("realtimeConnectionGrantHash(%q) accepted an invalid token", token)
		}
	}
}

func TestAllowedRealtimeOrigin(t *testing.T) {
	handler := &Handler{canonicalOrigin: "https://assistant.example"}
	tests := []struct {
		name   string
		origin string
		wantOK bool
	}{
		{name: "configured origin", origin: "https://assistant.example", wantOK: true},
		{name: "foreign origin", origin: "https://evil.example"},
		{name: "wrong scheme", origin: "http://assistant.example"},
		{name: "origin path", origin: "https://assistant.example/path"},
		{name: "origin query", origin: "https://assistant.example?next=evil"},
		{name: "invalid origin", origin: "not a URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://assistant.example/api/ai/realtime", nil)
			request.Header.Set("Origin", test.origin)
			_, ok := handler.allowedRealtimeOrigin(request)
			if ok != test.wantOK {
				t.Fatalf("allowedRealtimeOrigin(%q) = %t, want %t", test.origin, ok, test.wantOK)
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "https://assistant.example/api/ai/realtime", nil)
	request.Header.Add("Origin", "https://assistant.example")
	request.Header.Add("Origin", "https://assistant.example")
	if _, ok := handler.allowedRealtimeOrigin(request); ok {
		t.Fatal("multiple Origin headers were accepted")
	}

	development := &Handler{}
	request = httptest.NewRequest(http.MethodGet, "http://localhost:5173/api/ai/realtime", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	if _, ok := development.allowedRealtimeOrigin(request); !ok {
		t.Fatal("same-host local development origin was rejected")
	}
	localCanonical := &Handler{canonicalOrigin: "http://localhost:5173"}
	if _, ok := localCanonical.allowedRealtimeOrigin(request); !ok {
		t.Fatal("configured loopback development origin was rejected")
	}
	request.Header.Set("Origin", "http://attacker.example")
	if _, ok := development.allowedRealtimeOrigin(request); ok {
		t.Fatal("foreign local-development origin was accepted")
	}
	if _, ok := localCanonical.allowedRealtimeOrigin(request); ok {
		t.Fatal("configured loopback origin accepted a foreign host")
	}
}

func TestRealtimeSessionLimiter(t *testing.T) {
	limiter := newRealtimeSessionLimiter()
	releaseOne, ok := limiter.acquire("user-one")
	if !ok {
		t.Fatal("first session was rejected")
	}
	releaseTwo, ok := limiter.acquire("user-one")
	if !ok {
		t.Fatal("second per-user session was rejected")
	}
	if _, ok := limiter.acquire("user-one"); ok {
		t.Fatal("third per-user session was accepted")
	}
	releaseOther, ok := limiter.acquire("user-two")
	if !ok {
		t.Fatal("another user's session was rejected")
	}
	releaseOne()
	releaseOne()
	releaseTwo()
	releaseOther()
	releaseAfterCapacity, ok := limiter.acquire("user-one")
	if !ok {
		t.Fatal("released session capacity was not returned")
	}
	releaseAfterCapacity()
}

type testRealtimeSession struct {
	frames      chan []byte
	events      chan assemblyAIRealtimeEvent
	terminate   sync.Once
	termination assemblyAIRealtimeEvent
}

func (s *testRealtimeSession) SendPCMFrame(ctx context.Context, frame []byte) error {
	select {
	case s.frames <- append([]byte(nil), frame...):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *testRealtimeSession) ReadProviderMessage(ctx context.Context) (assemblyAIRealtimeEvent, error) {
	select {
	case event := <-s.events:
		return event, nil
	case <-ctx.Done():
		return assemblyAIRealtimeEvent{}, ctx.Err()
	}
}

func (s *testRealtimeSession) SendTermination(ctx context.Context) error {
	var sendErr error
	s.terminate.Do(func() {
		select {
		case s.events <- s.termination:
		case <-ctx.Done():
			sendErr = ctx.Err()
		}
	})
	return sendErr
}

func (s *testRealtimeSession) Close() {}

func TestRelayRealtimeSessionForwardsAudioAndProviderEvents(t *testing.T) {
	provider := &testRealtimeSession{
		frames: make(chan []byte, 1),
		events: make(chan assemblyAIRealtimeEvent, 2),
		termination: assemblyAIRealtimeEvent{
			Type: "Termination", Payload: []byte(`{"type":"Termination"}`),
		},
	}
	handler := &Handler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept test websocket: %v", err)
			return
		}
		defer client.CloseNow()
		handler.relayRealtimeSession(r.Context(), client, provider, time.Now())
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial test websocket: %v", err)
	}
	defer client.CloseNow()

	frame := make([]byte, assemblyAIRealtimeMinPCMFrameBytes)
	if err := client.Write(ctx, websocket.MessageText, []byte(`{"type":"Heartbeat"}`)); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	if err := client.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write PCM frame: %v", err)
	}
	select {
	case received := <-provider.frames:
		if len(received) != len(frame) {
			t.Fatalf("forwarded frame length = %d, want %d", len(received), len(frame))
		}
	case <-ctx.Done():
		t.Fatal("provider did not receive the PCM frame")
	}

	turn := assemblyAIRealtimeEvent{
		Type: "Turn", Payload: []byte(`{"type":"Turn","turn_order":1,"transcript":"hello","end_of_turn":true}`),
	}
	provider.events <- turn
	messageType, payload, err := client.Read(ctx)
	if err != nil {
		t.Fatalf("read provider event: %v", err)
	}
	if messageType != websocket.MessageText || string(payload) != string(turn.Payload) {
		t.Fatalf("provider event = %q, want %q", payload, turn.Payload)
	}

	if err := client.Write(ctx, websocket.MessageText, []byte(`{"type":"Terminate"}`)); err != nil {
		t.Fatalf("write client termination: %v", err)
	}
	messageType, payload, err = client.Read(ctx)
	if err != nil {
		t.Fatalf("read provider termination: %v", err)
	}
	if messageType != websocket.MessageText || string(payload) != `{"type":"Termination"}` {
		t.Fatalf("termination event = %q, want provider acknowledgement", payload)
	}
}

func TestRelayRealtimeSessionLimitStopsForwardingAudio(t *testing.T) {
	provider := &testRealtimeSession{
		frames: make(chan []byte, 1),
		events: make(chan assemblyAIRealtimeEvent, 2),
		termination: assemblyAIRealtimeEvent{
			Type: "Termination", Payload: []byte(`{"type":"Termination"}`),
		},
	}
	handler := &Handler{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept test websocket: %v", err)
			return
		}
		defer client.CloseNow()
		startedAt := time.Now().Add(-2 * time.Duration(assemblyAIRealtimeMaxSessionDurationSeconds) * time.Second)
		handler.relayRealtimeSession(r.Context(), client, provider, startedAt)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial test websocket: %v", err)
	}
	defer client.CloseNow()

	messageType, payload, err := client.Read(ctx)
	if err != nil {
		t.Fatalf("read session-limit event: %v", err)
	}
	if messageType != websocket.MessageText || !strings.Contains(string(payload), `"type":"AskoloSessionLimit"`) {
		t.Fatalf("session-limit event = %q, want AskoloSessionLimit", payload)
	}

	frame := make([]byte, assemblyAIRealtimeMinPCMFrameBytes)
	_ = client.Write(ctx, websocket.MessageBinary, frame)
	select {
	case forwarded := <-provider.frames:
		t.Fatalf("audio was forwarded after the session limit: %d bytes", len(forwarded))
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRelayRealtimeSessionEndsIdleClient(t *testing.T) {
	provider := &testRealtimeSession{
		frames: make(chan []byte, 1),
		events: make(chan assemblyAIRealtimeEvent, 2),
		termination: assemblyAIRealtimeEvent{
			Type: "Termination", Payload: []byte(`{"type":"Termination"}`),
		},
	}
	handler := &Handler{realtimeIdleTimeout: 60 * time.Millisecond}
	outcomes := make(chan realtimeSessionOutcome, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept test websocket: %v", err)
			return
		}
		defer client.CloseNow()
		outcomes <- handler.relayRealtimeSession(r.Context(), client, provider, time.Now())
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial test websocket: %v", err)
	}
	defer client.CloseNow()

	messageType, payload, err := client.Read(ctx)
	if err != nil {
		t.Fatalf("read idle timeout event: %v", err)
	}
	if messageType != websocket.MessageText || !strings.Contains(string(payload), `"code":"CLIENT_IDLE_TIMEOUT"`) {
		t.Fatalf("idle timeout event = %q, want CLIENT_IDLE_TIMEOUT", payload)
	}
	select {
	case outcome := <-outcomes:
		if outcome != realtimeOutcomeClientIdle {
			t.Fatalf("session outcome = %q, want %q", outcome, realtimeOutcomeClientIdle)
		}
	case <-ctx.Done():
		t.Fatal("idle session did not terminate")
	}
}
