package product

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const (
	assemblyAIVoiceAgentWebsocketURL = "wss://agents.assemblyai.com/v1/ws"
	assemblyAIVoiceAgentSessionsURL  = "https://agents.assemblyai.com/v1/sessions/"
	assemblyAIVoiceAgentMaxMessage   = 64 * 1024
	assemblyAIVoiceAgentMaxAudio     = 16 * 1024
)

type assemblyAIVoiceAgentProvider interface {
	Configured() bool
	OpenVoiceAgentSession(context.Context) (assemblyAIVoiceAgentConnection, error)
	DeleteVoiceAgentSession(context.Context, string) error
}

type assemblyAIVoiceAgentConnection interface {
	SendEvent(context.Context, any) error
	SendAudio(context.Context, []byte) error
	ReadProviderMessage(context.Context) (string, json.RawMessage, error)
	Close()
}

type assemblyAIVoiceAgentSocket struct {
	conn *websocket.Conn
}

func (c *assemblyAIClient) OpenVoiceAgentSession(ctx context.Context) (assemblyAIVoiceAgentConnection, error) {
	if !c.Configured() {
		return nil, errAssemblyAIProviderFailure
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+c.apiKey)
	conn, _, err := websocket.Dial(ctx, assemblyAIVoiceAgentWebsocketURL, &websocket.DialOptions{
		HTTPClient: c.websocketHTTPClient,
		HTTPHeader: header,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errAssemblyAIProviderFailure
	}
	conn.SetReadLimit(assemblyAIVoiceAgentMaxMessage)
	return &assemblyAIVoiceAgentSocket{conn: conn}, nil
}

func (c *assemblyAIClient) DeleteVoiceAgentSession(ctx context.Context, sessionID string) error {
	if !c.Configured() || !validAssemblyAIVoiceAgentSessionID(sessionID) {
		return errAssemblyAIProviderFailure
	}
	response, cancel, err := assemblyAIRequest(
		ctx,
		assemblyAINoRedirectClient(c.httpClient),
		http.MethodDelete,
		assemblyAIVoiceAgentSessionsURL+url.PathEscape(sessionID),
		nil,
		c.apiKey,
		"",
		8*time.Second,
	)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errAssemblyAIProviderFailure
	}
	defer cancel()
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errAssemblyAIProviderFailure
	}
	return nil
}

func validAssemblyAIVoiceAgentSessionID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && !strings.ContainsRune("._-", r) {
			return false
		}
	}
	return true
}

func (s *assemblyAIVoiceAgentSocket) SendEvent(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > assemblyAIVoiceAgentMaxMessage {
		return errAssemblyAIProviderFailure
	}
	if err := s.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errAssemblyAIProviderFailure
	}
	return nil
}

func (s *assemblyAIVoiceAgentSocket) SendAudio(ctx context.Context, pcm []byte) error {
	if len(pcm) < 2 || len(pcm) > assemblyAIVoiceAgentMaxAudio || len(pcm)%2 != 0 {
		return errAssemblyAIInvalidPCMFrame
	}
	return s.SendEvent(ctx, map[string]string{
		"type":  "input.audio",
		"audio": base64.StdEncoding.EncodeToString(pcm),
	})
}

func (s *assemblyAIVoiceAgentSocket) ReadProviderMessage(ctx context.Context) (string, json.RawMessage, error) {
	messageType, payload, err := s.conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, errAssemblyAIProviderFailure
	}
	if messageType != websocket.MessageText || len(payload) == 0 || len(payload) > assemblyAIVoiceAgentMaxMessage {
		return "", nil, errAssemblyAIProviderFailure
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &header); err != nil || header.Type == "" || len(header.Type) > 64 {
		return "", nil, errAssemblyAIProviderFailure
	}
	return header.Type, json.RawMessage(payload), nil
}

func (s *assemblyAIVoiceAgentSocket) Close() {
	if s != nil && s.conn != nil {
		_ = s.conn.Close(websocket.StatusNormalClosure, "session closed")
	}
}
