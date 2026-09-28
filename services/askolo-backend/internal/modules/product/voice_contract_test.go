package product

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestVoiceResponseContractsMatchFixtures(t *testing.T) {
	confidence := 0.91
	start, end := 240, 680
	receipt := voiceCreditReceipt{
		ID: "voice-synthetic", ReservationID: "voice-synthetic",
		OperationType: "voice", Provider: "assemblyai", Mode: "recorded",
		Status: "settled", ReservedCredits: 3, SettledCredits: 2,
		RefundedCredits: 0, Balance: 18, PolicyVersion: 1,
	}

	recorded := audioTranscriptionResponse{
		Transcript: "Synthetic transcript",
		Confidence: &confidence,
		ReviewSignals: []transcriptionReviewSignal{{
			Kind: "low_confidence_entity", Text: "Thursday", Confidence: 0.61,
			StartMS: &start, EndMS: &end,
		}},
		Deletion: transcriptionDeletion{
			RawAudio: "not_stored", ProviderTranscript: "deleted", Marker: "provider_transcript_deleted",
		},
		CreditReceipt: receipt,
	}
	assertVoiceResponseFixture(t, "recorded-transcription-response.json", recorded)

	receipt.Mode = "realtime"
	realtime := realtimeTranscriptionTokenResponse{
		Token:                     "synthetic-temporary-token",
		ExpiresInSeconds:          assemblyAIRealtimeTokenExpiresInSeconds,
		MaxSessionDurationSeconds: assemblyAIRealtimeMaxSessionDurationSeconds,
		Region:                    assemblyAIRealtimeRegion,
		WebsocketURL:              assemblyAIRealtimeWebsocketURL,
		SpeechModel:               assemblyAIRealtimeSpeechModel,
		Redaction:                 assemblyAIRealtimeRedaction,
		CreditReceipt:             receipt,
	}
	assertVoiceResponseFixture(t, "realtime-token-response.json", realtime)
}

func assertVoiceResponseFixture(t *testing.T, fixtureName string, response any) {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	gotJSON, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response for fixture %s: %v", fixtureName, err)
	}
	var got any
	var want any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("decode marshaled response for fixture %s: %v", fixtureName, err)
	}
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatalf("decode fixture %s: %v", fixtureName, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s response = %#v, want fixture %#v", fixtureName, got, want)
	}
}

func TestRequestAssemblyAIRealtimeTokenUsesEdgeTokenEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v3/token" ||
			query.Get("expires_in_seconds") != "60" ||
			query.Get("max_session_duration_seconds") != "10800" ||
			len(query) != 2 {
			t.Errorf("request = %s %s?%s, want GET /v3/token with a 60s token and 10800s session cap", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "synthetic-test-key" {
			t.Errorf("authorization header = %q, want the raw synthetic key", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token":"synthetic-temporary-token","expires_in_seconds":60}`)
	}))
	defer server.Close()

	token, err := requestAssemblyAIRealtimeToken(
		context.Background(), server.Client(), server.URL, "synthetic-test-key",
	)
	if err != nil {
		t.Fatalf("requestAssemblyAIRealtimeToken returned error: %v", err)
	}
	if token != "synthetic-temporary-token" {
		t.Fatalf("token = %q, want the provider's temporary token", token)
	}
}

func TestRequestAssemblyAIRealtimeTokenRejectsInvalidProviderResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "missing token", statusCode: http.StatusOK, body: `{}`},
		{name: "missing token expiry", statusCode: http.StatusOK, body: `{"token":"synthetic-token"}`},
		{name: "wrong token expiry", statusCode: http.StatusOK, body: `{"token":"synthetic-token","expires_in_seconds":600}`},
		{name: "invalid JSON", statusCode: http.StatusOK, body: `not-json`},
		{name: "trailing JSON", statusCode: http.StatusOK, body: `{"token":"one","expires_in_seconds":60}{"token":"two"}`},
		{name: "oversized body", statusCode: http.StatusOK, body: `{"token":"` + strings.Repeat("x", maxAssemblyAIRealtimeTokenResponseBytes) + `","expires_in_seconds":60}`},
		{name: "provider failure", statusCode: http.StatusBadGateway, body: `{"error":"synthetic failure"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			_, err := requestAssemblyAIRealtimeToken(
				context.Background(), server.Client(), server.URL, "synthetic-test-key",
			)
			if !errors.Is(err, errAssemblyAIProviderFailure) {
				t.Fatalf("error = %v, want a generic provider failure", err)
			}
		})
	}
}

func TestAssemblyAIRealtimeSettingsKeepTheEdgeContract(t *testing.T) {
	settings := newAssemblyAIClient(
		"synthetic-test-key",
		"",
		"",
		"",
		nil,
		nil,
	).RealtimeSettings()
	if settings.Region != "edge" || settings.WebsocketURL != "wss://streaming.assemblyai.com/v3/ws" {
		t.Fatalf("realtime region/URL = %q/%q, want global edge", settings.Region, settings.WebsocketURL)
	}
	if settings.ExpiresInSeconds != 60 || settings.MaxSessionDurationSeconds != 10_800 {
		t.Fatalf("token/session durations = %d/%d, want 60/10800 seconds",
			settings.ExpiresInSeconds, settings.MaxSessionDurationSeconds)
	}
	if settings.SpeechModel != "universal-3-5-pro" || settings.Redaction != "provider_pii_redaction" {
		t.Fatalf("realtime model/redaction = %q/%q", settings.SpeechModel, settings.Redaction)
	}
}

func BenchmarkTranscribeAssemblyAIMockProvider(b *testing.B) {
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			_, _ = io.WriteString(w, `{"status":"completed","text":"Synthetic transcript","confidence":0.9,"audio_duration":1,"words":[]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			w.WriteHeader(http.StatusOK)
		default:
			b.Errorf("unexpected mock provider request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	audio := make([]byte, 1024)
	client := server.Client()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, deletionStatus, err := transcribeAssemblyAI(
			context.Background(), client, server.URL, "synthetic-test-key", audio, "",
		)
		if err != nil || deletionStatus != "deleted" {
			b.Fatalf("transcribeAssemblyAI = deletion %q, error %v", deletionStatus, err)
		}
	}
	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(requestCount.Load())/float64(b.N), "provider_requests/op")
	}
}
