package product

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscribeAssemblyAIRedactsReturnsReviewSignalsAndDeletes(t *testing.T) {
	var uploadBody []byte
	var transcriptRequest map[string]any
	var deleteCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "synthetic-test-key" {
			t.Errorf("authorization header = %q, want the synthetic test key", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			uploadBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			if err := json.NewDecoder(r.Body).Decode(&transcriptRequest); err != nil {
				t.Errorf("decode transcript request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"status":"completed",
				"text":"Call #### at 3pm",
				"confidence":0.91,
				"audio_duration":10,
				"words":[
					{"text":"Call","start":0,"end":300,"confidence":0.99},
					{"text":"####","start":301,"end":700,"confidence":0.6},
					{"text":"3pm","start":800,"end":950,"confidence":0.55}
				]
			}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			deleteCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected AssemblyAI request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, deletionStatus, err := transcribeAssemblyAI(
		context.Background(),
		server.Client(),
		server.URL,
		"synthetic-test-key",
		[]byte{1, 2, 3},
		"en-US",
	)
	if err != nil {
		t.Fatalf("transcribeAssemblyAI returned error: %v", err)
	}
	if string(uploadBody) != string([]byte{1, 2, 3}) {
		t.Fatalf("uploaded bytes = %v, want original transient audio bytes", uploadBody)
	}
	if transcriptRequest["redact_pii"] != true || transcriptRequest["redact_pii_sub"] != "hash" {
		t.Fatalf("provider request did not enable PII redaction: %#v", transcriptRequest)
	}
	models, ok := transcriptRequest["speech_models"].([]any)
	if !ok || len(models) != 1 || models[0] != "universal-3-5-pro" {
		t.Fatalf("speech_models = %#v, want only Universal-3.5 Pro", transcriptRequest["speech_models"])
	}
	if transcriptRequest["language_code"] != "en" {
		t.Fatalf("language_code = %v, want normalized primary language", transcriptRequest["language_code"])
	}
	if result.Transcript != "Call #### at 3pm" || result.Confidence == nil || *result.Confidence != 0.91 {
		t.Fatalf("transcription result = %#v", result)
	}
	if deletionStatus != "deleted" || deleteCalls != 1 {
		t.Fatalf("deletion status/calls = %q/%d, want deleted/1", deletionStatus, deleteCalls)
	}
	signals := buildTranscriptionReviewSignals(result.Words)
	if len(signals) != 2 || signals[0].Text != "####" || signals[1].Text != "3pm" {
		t.Fatalf("review signals = %#v, want low-confidence critical details only", signals)
	}
}

func TestTranscribeAssemblyAIDeleteFailureIsVisibleAndRetried(t *testing.T) {
	var deleteCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			_, _ = io.WriteString(w, `{"status":"completed","text":"Safe transcript","confidence":0.9,"audio_duration":10,"words":[]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			deleteCalls++
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected AssemblyAI request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	if err != nil {
		t.Fatalf("completed transcription should remain available when deletion fails: %v", err)
	}
	if result.Transcript != "Safe transcript" {
		t.Fatalf("transcript = %q", result.Transcript)
	}
	if deletionStatus != "deletion_failed" || deleteCalls != assemblyAITranscriptDeleteAttempts {
		t.Fatalf("deletion status/calls = %q/%d, want deletion_failed/%d", deletionStatus, deleteCalls, assemblyAITranscriptDeleteAttempts)
	}
}

func TestTranscribeAssemblyAIDeletesProviderTranscriptAfterProviderError(t *testing.T) {
	var deleteCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			_, _ = io.WriteString(w, `{"status":"error","error":"untrusted provider detail"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			deleteCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected AssemblyAI request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	if err == nil || !strings.Contains(err.Error(), "provider request failed") {
		t.Fatalf("transcription error = %v, want generic provider failure", err)
	}
	if deletionStatus != "deleted" || deleteCalls != 1 {
		t.Fatalf("deletion status/calls = %q/%d, want deleted/1", deletionStatus, deleteCalls)
	}
}

func TestTranscribeAssemblyAIRejectsProviderDurationOverLimitAndDeletes(t *testing.T) {
	var deleteCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			_, _ = io.WriteString(w, `{"status":"completed","text":"Overlong recording","audio_duration":120.1,"words":[]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			deleteCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected AssemblyAI request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	if !errors.Is(err, errAssemblyAIAudioTooLong) {
		t.Fatalf("transcription error = %v, want actual provider duration rejection", err)
	}
	if deletionStatus != "deleted" || deleteCalls != 1 {
		t.Fatalf("deletion status/calls = %q/%d, want deleted/1", deletionStatus, deleteCalls)
	}
}

func TestVoiceInputValidation(t *testing.T) {
	for _, mimeType := range []string{"audio/webm", "audio/mp4", "audio/m4a", "audio/wav", "audio/ogg", "audio/mpeg"} {
		if !supportedVoiceMimeType(mimeType) {
			t.Errorf("supported MIME type %q was rejected", mimeType)
		}
	}
	if supportedVoiceMimeType("text/plain") {
		t.Error("unsupported MIME type was accepted")
	}
	if language, ok := normalizeVoiceLanguage("en-US"); !ok || language != "en" {
		t.Errorf("normalizeVoiceLanguage(en-US) = %q, %v", language, ok)
	}
	if _, ok := normalizeVoiceLanguage("en/../../metadata"); ok {
		t.Error("invalid language tag was accepted")
	}
	for _, uploadURL := range []string{
		"https://cdn.assemblyai.com/upload/synthetic",
	} {
		if !validAssemblyAIUploadURL(uploadURL) {
			t.Errorf("valid AssemblyAI upload URL %q was rejected", uploadURL)
		}
	}
	for _, uploadURL := range []string{
		"https://example.com/upload/synthetic",
		"https://cdn.assemblyai.com.evil.test/upload/synthetic",
		"http://cdn.assemblyai.com/upload/synthetic",
	} {
		if validAssemblyAIUploadURL(uploadURL) {
			t.Errorf("untrusted upload URL %q was accepted", uploadURL)
		}
	}

	body := `{"audioBase64":"` + strings.Repeat("A", 200*1024) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	var input struct {
		AudioBase64 string `json:"audioBase64"`
	}
	if !decodeBodyLimit(recorder, request, &input, 25*1024*1024) {
		t.Fatalf("audio request larger than the default 128 KiB limit failed: %s", recorder.Body.String())
	}
	if len(input.AudioBase64) != 200*1024 {
		t.Fatalf("decoded audioBase64 length = %d, want %d", len(input.AudioBase64), 200*1024)
	}
}
