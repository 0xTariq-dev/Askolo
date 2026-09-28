package product

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	assemblyAIRealtimeTokenBaseURL              = "https://streaming.assemblyai.com"
	assemblyAIRealtimeWebsocketURL              = "wss://streaming.assemblyai.com/v3/ws"
	assemblyAIRealtimeRegion                    = "edge"
	assemblyAIRealtimeTokenExpiresInSeconds     = 60
	assemblyAIRealtimeMaxSessionDurationSeconds = 10_800
	assemblyAIRealtimeSpeechModel               = "universal-3-5-pro"
	assemblyAIRealtimeRedaction                 = "provider_pii_redaction"
	maxAssemblyAIRealtimeTokenResponseBytes     = 64 * 1024
	maxAssemblyAIRealtimeTokenCharacters        = 16 * 1024
)

type voiceCreditReceipt struct {
	ID              string `json:"id"`
	ReservationID   string `json:"reservationId"`
	OperationType   string `json:"operationType"`
	Provider        string `json:"provider"`
	Mode            string `json:"mode"`
	Status          string `json:"status"`
	ReservedCredits int    `json:"reservedCredits"`
	SettledCredits  int    `json:"settledCredits"`
	RefundedCredits int    `json:"refundedCredits"`
	Balance         int    `json:"balance"`
	PolicyVersion   int    `json:"policyVersion"`
}

type audioTranscriptionResponse struct {
	Transcript    string                      `json:"transcript"`
	Confidence    *float64                    `json:"confidence"`
	ReviewSignals []transcriptionReviewSignal `json:"reviewSignals"`
	Deletion      transcriptionDeletion       `json:"deletion"`
	CreditReceipt voiceCreditReceipt          `json:"creditReceipt"`
}

type realtimeTranscriptionTokenResponse struct {
	Token                     string             `json:"token"`
	ExpiresInSeconds          int                `json:"expiresInSeconds"`
	MaxSessionDurationSeconds int                `json:"maxSessionDurationSeconds"`
	Region                    string             `json:"region"`
	WebsocketURL              string             `json:"websocketUrl"`
	SpeechModel               string             `json:"speechModel"`
	Redaction                 string             `json:"redaction"`
	CreditReceipt             voiceCreditReceipt `json:"creditReceipt"`
}

type assemblyAIRealtimeTokenPayload struct {
	Token            string `json:"token"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

func requestAssemblyAIRealtimeToken(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errAssemblyAIProviderFailure
	}
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = assemblyAIRealtimeTokenBaseURL
	}
	query := url.Values{}
	query.Set("expires_in_seconds", strconv.Itoa(assemblyAIRealtimeTokenExpiresInSeconds))
	query.Set("max_session_duration_seconds", strconv.Itoa(assemblyAIRealtimeMaxSessionDurationSeconds))
	endpoint := strings.TrimRight(baseURL, "/") + "/v3/token?" + query.Encode()
	response, cancel, err := assemblyAIRequest(
		ctx,
		assemblyAINoRedirectClient(client),
		http.MethodGet,
		endpoint,
		nil,
		apiKey,
		"",
		10*time.Second,
	)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errAssemblyAIProviderFailure
	}
	defer cancel()
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAssemblyAIRealtimeTokenResponseBytes+1))
	if err != nil || len(body) > maxAssemblyAIRealtimeTokenResponseBytes {
		return "", errAssemblyAIProviderFailure
	}
	var payload assemblyAIRealtimeTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errAssemblyAIProviderFailure
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" || token != payload.Token || len(token) > maxAssemblyAIRealtimeTokenCharacters ||
		payload.ExpiresInSeconds != assemblyAIRealtimeTokenExpiresInSeconds {
		return "", errAssemblyAIProviderFailure
	}
	return token, nil
}
