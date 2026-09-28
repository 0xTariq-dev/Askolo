package product

import (
	"context"
	"net/http"
	"strings"
)

type assemblyAIRealtimeSettings struct {
	ExpiresInSeconds          int
	MaxSessionDurationSeconds int
	Region                    string
	WebsocketURL              string
	SpeechModel               string
	Redaction                 string
}

type assemblyAIProvider interface {
	Configured() bool
	Transcribe(context.Context, []byte, string) (assemblyAITranscriptionResult, string, error)
	RealtimeToken(context.Context) (string, error)
	RealtimeSettings() assemblyAIRealtimeSettings
	OpenRealtimeSession(context.Context, string) (assemblyAIRealtimeSession, error)
}

type assemblyAIClient struct {
	apiKey               string
	restBaseURL          string
	realtimeTokenBaseURL string
	realtimeWebsocketURL string
	httpClient           *http.Client
	websocketHTTPClient  *http.Client
}

func newAssemblyAIClient(
	apiKey string,
	restBaseURL string,
	realtimeTokenBaseURL string,
	realtimeWebsocketURL string,
	httpClient *http.Client,
	websocketHTTPClient *http.Client,
) *assemblyAIClient {
	if strings.TrimSpace(restBaseURL) == "" {
		restBaseURL = assemblyAIRESTBaseURL
	}
	if strings.TrimSpace(realtimeTokenBaseURL) == "" {
		realtimeTokenBaseURL = assemblyAIRealtimeTokenBaseURL
	}
	if strings.TrimSpace(realtimeWebsocketURL) == "" {
		realtimeWebsocketURL = assemblyAIRealtimeWebsocketURL
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &assemblyAIClient{
		apiKey:               strings.TrimSpace(apiKey),
		restBaseURL:          strings.TrimRight(restBaseURL, "/"),
		realtimeTokenBaseURL: strings.TrimRight(realtimeTokenBaseURL, "/"),
		realtimeWebsocketURL: strings.TrimRight(realtimeWebsocketURL, "/"),
		httpClient:           httpClient,
		websocketHTTPClient:  websocketHTTPClient,
	}
}

func (c *assemblyAIClient) Configured() bool {
	return c != nil && c.apiKey != "" && strings.TrimSpace(c.apiKey) == c.apiKey &&
		!strings.ContainsAny(c.apiKey, "\r\n")
}

func (c *assemblyAIClient) Transcribe(ctx context.Context, audio []byte, language string) (assemblyAITranscriptionResult, string, error) {
	if !c.Configured() {
		return assemblyAITranscriptionResult{}, "deletion_failed", errAssemblyAIProviderFailure
	}
	return transcribeAssemblyAI(ctx, c.httpClient, c.restBaseURL, c.apiKey, audio, language)
}

func (c *assemblyAIClient) RealtimeToken(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", errAssemblyAIProviderFailure
	}
	return requestAssemblyAIRealtimeToken(ctx, c.httpClient, c.realtimeTokenBaseURL, c.apiKey)
}

func (c *assemblyAIClient) RealtimeSettings() assemblyAIRealtimeSettings {
	return assemblyAIRealtimeSettings{
		ExpiresInSeconds:          assemblyAIRealtimeTokenExpiresInSeconds,
		MaxSessionDurationSeconds: assemblyAIRealtimeMaxSessionDurationSeconds,
		Region:                    assemblyAIRealtimeRegion,
		WebsocketURL:              c.realtimeWebsocketURL,
		SpeechModel:               assemblyAIRealtimeSpeechModel,
		Redaction:                 assemblyAIRealtimeRedaction,
	}
}

func (c *assemblyAIClient) OpenRealtimeSession(ctx context.Context, token string) (assemblyAIRealtimeSession, error) {
	if !c.Configured() {
		return nil, errAssemblyAIProviderFailure
	}
	return openAssemblyAIRealtimeSession(ctx, c.realtimeWebsocketURL, token, c.websocketHTTPClient)
}
