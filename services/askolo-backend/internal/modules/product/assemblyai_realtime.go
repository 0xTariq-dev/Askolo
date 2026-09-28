package product

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

const (
	// AssemblyAI v3 accepts 50–1000 ms mono PCM16 frames at 16 kHz.
	assemblyAIRealtimeSampleRateHz     = 16_000
	assemblyAIRealtimeMinPCMFrameBytes = assemblyAIRealtimeSampleRateHz * 2 * 50 / 1000
	assemblyAIRealtimeMaxPCMFrameBytes = assemblyAIRealtimeSampleRateHz * 2
	assemblyAIRealtimeMaxMessageBytes  = 1024 * 1024
)

var errAssemblyAIInvalidPCMFrame = errors.New("invalid assemblyai pcm frame")

type assemblyAIRealtimeEvent struct {
	Type    string
	Payload json.RawMessage
}

type assemblyAIRealtimeSession interface {
	SendPCMFrame(context.Context, []byte) error
	ReadProviderMessage(context.Context) (assemblyAIRealtimeEvent, error)
	SendTermination(context.Context) error
	Close()
}

type assemblyAIRealtimeConnection struct {
	conn *websocket.Conn
}

func buildAssemblyAIRealtimeURL(endpoint, token string) (string, error) {
	if token == "" || token != strings.TrimSpace(token) ||
		len(token) > maxAssemblyAIRealtimeTokenCharacters || strings.ContainsAny(token, "\r\n\t ") {
		return "", errAssemblyAIProviderFailure
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" ||
		parsed.Path != "/v3/ws" {
		return "", errAssemblyAIProviderFailure
	}
	if parsed.Scheme == "wss" {
		if parsed.Hostname() != "streaming.assemblyai.com" || parsed.Port() != "" {
			return "", errAssemblyAIProviderFailure
		}
	} else if parsed.Scheme == "ws" {
		if !isLoopbackWebsocketHost(parsed.Hostname()) {
			return "", errAssemblyAIProviderFailure
		}
	} else {
		return "", errAssemblyAIProviderFailure
	}

	query := url.Values{}
	query.Set("token", token)
	query.Set("sample_rate", "16000")
	query.Set("speech_model", assemblyAIRealtimeSpeechModel)
	query.Set("include_partial_turns", "true")
	query.Set("redact_pii", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func isLoopbackWebsocketHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func openAssemblyAIRealtimeSession(
	ctx context.Context,
	endpoint string,
	token string,
	httpClient *http.Client,
) (assemblyAIRealtimeSession, error) {
	assemblyAIURL, err := buildAssemblyAIRealtimeURL(endpoint, token)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, assemblyAIURL, &websocket.DialOptions{
		HTTPClient: httpClient,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errAssemblyAIProviderFailure
	}
	conn.SetReadLimit(assemblyAIRealtimeMaxMessageBytes)
	return &assemblyAIRealtimeConnection{conn: conn}, nil
}

func (s *assemblyAIRealtimeConnection) SendPCMFrame(ctx context.Context, frame []byte) error {
	if len(frame) < assemblyAIRealtimeMinPCMFrameBytes ||
		len(frame) > assemblyAIRealtimeMaxPCMFrameBytes || len(frame)%2 != 0 {
		return errAssemblyAIInvalidPCMFrame
	}
	if err := s.conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errAssemblyAIProviderFailure
	}
	return nil
}

func (s *assemblyAIRealtimeConnection) ReadProviderMessage(ctx context.Context) (assemblyAIRealtimeEvent, error) {
	messageType, payload, err := s.conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return assemblyAIRealtimeEvent{}, ctx.Err()
		}
		return assemblyAIRealtimeEvent{}, errAssemblyAIProviderFailure
	}
	if messageType != websocket.MessageText {
		return assemblyAIRealtimeEvent{}, errAssemblyAIProviderFailure
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if len(payload) == 0 || json.Unmarshal(payload, &envelope) != nil ||
		envelope.Type == "" || len(envelope.Type) > 32 {
		return assemblyAIRealtimeEvent{}, errAssemblyAIProviderFailure
	}
	if envelope.Type == "Error" {
		return assemblyAIRealtimeEvent{}, errAssemblyAIProviderFailure
	}
	return assemblyAIRealtimeEvent{
		Type:    envelope.Type,
		Payload: append(json.RawMessage(nil), payload...),
	}, nil
}

func (s *assemblyAIRealtimeConnection) SendTermination(ctx context.Context) error {
	if err := s.conn.Write(ctx, websocket.MessageText, []byte(`{"type":"Terminate"}`)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errAssemblyAIProviderFailure
	}
	return nil
}

func (s *assemblyAIRealtimeConnection) Close() {
	if s != nil && s.conn != nil {
		_ = s.conn.CloseNow()
	}
}
