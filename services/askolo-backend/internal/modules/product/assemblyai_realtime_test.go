package product

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBuildAssemblyAIRealtimeURLUsesCurrentEdgeContract(t *testing.T) {
	token := "temporary-token/+?="
	rawURL, err := buildAssemblyAIRealtimeURL(assemblyAIRealtimeWebsocketURL, token)
	if err != nil {
		t.Fatalf("buildAssemblyAIRealtimeURL returned error: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse generated WebSocket URL: %v", err)
	}
	query := parsed.Query()
	if parsed.Scheme != "wss" || parsed.Host != "streaming.assemblyai.com" || parsed.Path != "/v3/ws" {
		t.Fatalf("WebSocket endpoint = %s://%s%s, want the global edge endpoint", parsed.Scheme, parsed.Host, parsed.Path)
	}
	for key, expected := range map[string]string{
		"token":                 token,
		"sample_rate":           "16000",
		"speech_model":          "universal-3-5-pro",
		"include_partial_turns": "true",
	} {
		if query.Get(key) != expected {
			t.Errorf("query %q = %q, want %q", key, query.Get(key), expected)
		}
	}
	if query.Has("redact_pii") || len(query) != 4 || query.Has("api_key") || query.Has("authorization") {
		t.Fatalf("provider query contains unexpected credentials or parameters: %#v", query)
	}
}

func TestBuildAssemblyAIRealtimeURLRejectsUntrustedRegionAndToken(t *testing.T) {
	for _, endpoint := range []string{
		"https://streaming.assemblyai.com/v3/ws",
		"wss://streaming.us.assemblyai.com/v3/ws",
		"wss://streaming.eu.assemblyai.com/v3/ws",
		"wss://streaming.assemblyai.com.evil.test/v3/ws",
		"wss://streaming.assemblyai.com/v3/ws?sample_rate=8000",
		"wss://streaming.assemblyai.com/other",
		"ws://example.test/v3/ws",
	} {
		if _, err := buildAssemblyAIRealtimeURL(endpoint, "synthetic-token"); !errors.Is(err, errAssemblyAIProviderFailure) {
			t.Errorf("endpoint %q error = %v, want generic provider failure", endpoint, err)
		}
	}
	for _, token := range []string{"", " token ", "token\r\nheader", strings.Repeat("x", maxAssemblyAIRealtimeTokenCharacters+1)} {
		if _, err := buildAssemblyAIRealtimeURL(assemblyAIRealtimeWebsocketURL, token); !errors.Is(err, errAssemblyAIProviderFailure) {
			t.Errorf("invalid token %q error = %v, want generic provider failure", token, err)
		}
	}
}

func TestAssemblyAIRealtimeSessionSendsBinaryPCMAndTermination(t *testing.T) {
	serverResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/v3/ws" || query.Get("token") != "synthetic-temporary-token" ||
			query.Get("sample_rate") != "16000" || query.Get("speech_model") != assemblyAIRealtimeSpeechModel ||
			query.Get("include_partial_turns") != "true" || query.Has("redact_pii") ||
			query.Has("api_key") {
			serverResult <- errors.New("unexpected provider URL contract")
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			serverResult <- err
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"Begin","id":"synthetic-session"}`)); err != nil {
			serverResult <- err
			return
		}
		messageType, pcm, err := conn.Read(ctx)
		if err != nil {
			serverResult <- err
			return
		}
		if messageType != websocket.MessageBinary ||
			len(pcm) != assemblyAIRealtimeMinPCMFrameBytes ||
			pcm[0] != 1 {
			serverResult <- errors.New("audio was not sent as its original binary PCM16 frame")
			return
		}
		messageType, termination, err := conn.Read(ctx)
		if err != nil {
			serverResult <- err
			return
		}
		if messageType != websocket.MessageText || string(termination) != `{"type":"Terminate"}` {
			serverResult <- errors.New("expected the raw Terminate protocol message")
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"Termination"}`)); err != nil {
			serverResult <- err
			return
		}
		serverResult <- nil
	}))
	defer server.Close()

	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/v3/ws"
	session, err := openAssemblyAIRealtimeSession(
		context.Background(),
		endpoint,
		"synthetic-temporary-token",
		server.Client(),
	)
	if err != nil {
		t.Fatalf("openAssemblyAIRealtimeSession returned error: %v", err)
	}
	defer session.Close()

	begin, err := session.ReadProviderMessage(context.Background())
	if err != nil || begin.Type != "Begin" {
		t.Fatalf("begin event = %#v, error = %v", begin, err)
	}
	if !json.Valid(begin.Payload) {
		t.Fatalf("begin payload is not valid JSON: %q", begin.Payload)
	}
	pcmFrame := make([]byte, assemblyAIRealtimeMinPCMFrameBytes)
	pcmFrame[0] = 1
	if err := session.SendPCMFrame(context.Background(), pcmFrame); err != nil {
		t.Fatalf("SendPCMFrame returned error: %v", err)
	}
	for _, invalidFrame := range [][]byte{
		nil,
		make([]byte, assemblyAIRealtimeMinPCMFrameBytes-2),
		{1},
		make([]byte, assemblyAIRealtimeMaxPCMFrameBytes+2),
	} {
		if err := session.SendPCMFrame(context.Background(), invalidFrame); !errors.Is(err, errAssemblyAIInvalidPCMFrame) {
			t.Errorf("invalid PCM frame length %d error = %v", len(invalidFrame), err)
		}
	}
	if err := session.SendTermination(context.Background()); err != nil {
		t.Fatalf("SendTermination returned error: %v", err)
	}
	termination, err := session.ReadProviderMessage(context.Background())
	if err != nil || termination.Type != "Termination" {
		t.Fatalf("termination event = %#v, error = %v", termination, err)
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatalf("provider protocol check failed: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("provider protocol check did not finish")
	}
}

func TestAssemblyAIRealtimeProviderErrorsAreRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept WebSocket: %v", err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"Error","error":"private provider detail"}`)); err != nil {
			t.Errorf("write provider error: %v", err)
		}
	}))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/v3/ws"
	session, err := openAssemblyAIRealtimeSession(context.Background(), endpoint, "synthetic-temporary-token", server.Client())
	if err != nil {
		t.Fatalf("openAssemblyAIRealtimeSession returned error: %v", err)
	}
	defer session.Close()

	_, err = session.ReadProviderMessage(context.Background())
	if !errors.Is(err, errAssemblyAIProviderFailure) || strings.Contains(err.Error(), "private provider detail") {
		t.Fatalf("provider error = %v, want a generic safe provider failure", err)
	}
}
