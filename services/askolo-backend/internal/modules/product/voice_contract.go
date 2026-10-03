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
<<<<<<< HEAD
	assemblyAIRealtimeTokenBaseURL                = "https://streaming.assemblyai.com"
	assemblyAIRealtimeWebsocketURL                = "wss://streaming.assemblyai.com/v3/ws"
	assemblyAIRealtimeTokenExpiresInSeconds       = 60
	assemblyAIRealtimeMinProviderSessionSeconds   = 60
	assemblyAIRealtimeMaxProviderSessionSeconds   = 10_800
	assemblyAIRealtimeMaxSessionDurationSeconds   = 180
	assemblyAIRealtimeSpeechModel                 = "universal-3-5-pro"
	assemblyAIVoiceAgentModel                     = "managed-voice-agent"
	assemblyAIVoiceAgentMaxSessionDurationSeconds = 180
	maxAssemblyAIRealtimeTokenResponseBytes       = 64 * 1024
	maxAssemblyAIRealtimeTokenCharacters          = 16 * 1024
=======
	assemblyAIRealtimeTokenBaseURL              = "https://streaming.assemblyai.com"
	assemblyAIRealtimeWebsocketURL              = "wss://streaming.assemblyai.com/v3/ws"
	assemblyAIRealtimeTokenExpiresInSeconds     = 60
	assemblyAIRealtimeMinProviderSessionSeconds = 60
	assemblyAIRealtimeMaxProviderSessionSeconds = 10_800
	assemblyAIRealtimeMaxSessionDurationSeconds = 180
	assemblyAIRealtimeSpeechModel               = "universal-3-5-pro"
	maxAssemblyAIRealtimeTokenResponseBytes     = 64 * 1024
	maxAssemblyAIRealtimeTokenCharacters        = 16 * 1024
>>>>>>> c7ea45a5fdbf4bb422d76b6a13e0d4dca631b40a
)

type voiceCreditReceipt struct {
	ID                string `json:"id"`
	ReservationID     string `json:"reservationId"`
	OperationType     string `json:"operationType"`
	Provider          string `json:"provider"`
	Mode              string `json:"mode"`
	Status            string `json:"status"`
	ReservedUsdMicros int64  `json:"reservedUsdMicros,omitempty"`
	SettledUsdMicros  int64  `json:"settledUsdMicros,omitempty"`
	RefundedUsdMicros int64  `json:"refundedUsdMicros,omitempty"`
	BalanceUsdMicros  int64  `json:"balanceUsdMicros,omitempty"`
	UsageUnit         string `json:"usageUnit,omitempty"`
	UsageUnits        int64  `json:"usageUnits,omitempty"`
	PolicyVersion     int    `json:"policyVersion"`
	// Deprecated legacy fields retained only so older internal callers can
	// decode historical responses; USD fields are authoritative.
	ReservedCredits int `json:"reservedCredits,omitempty"`
	SettledCredits  int `json:"settledCredits,omitempty"`
	RefundedCredits int `json:"refundedCredits"`
	Balance         int `json:"balance,omitempty"`
}

type audioTranscriptionResponse struct {
	Transcript    string                      `json:"transcript"`
	Confidence    *float64                    `json:"confidence"`
	ReviewSignals []transcriptionReviewSignal `json:"reviewSignals"`
	Deletion      transcriptionDeletion       `json:"deletion"`
	CreditReceipt voiceCreditReceipt          `json:"creditReceipt"`
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
	return requestAssemblyAIRealtimeTokenWithSessionDuration(
		ctx, client, baseURL, apiKey, assemblyAIRealtimeMaxSessionDurationSeconds,
	)
}

func requestAssemblyAIRealtimeTokenWithSessionDuration(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	maxSessionDurationSeconds int,
) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errAssemblyAIProviderFailure
	}
	if maxSessionDurationSeconds < assemblyAIRealtimeMinProviderSessionSeconds ||
		maxSessionDurationSeconds > assemblyAIRealtimeMaxProviderSessionSeconds {
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
	query.Set("max_session_duration_seconds", strconv.Itoa(maxSessionDurationSeconds))
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
