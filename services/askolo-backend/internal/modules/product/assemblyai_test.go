package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

	provider := newAssemblyAIClient(
		"synthetic-test-key",
		server.URL,
		"",
		"",
		server.Client(),
		nil,
	)
	result, deletionStatus, err := provider.Transcribe(context.Background(), []byte{1, 2, 3}, "en-US")
	if err != nil {
		t.Fatalf("transcribeAssemblyAI returned error: %v", err)
	}
	if string(uploadBody) != string([]byte{1, 2, 3}) {
		t.Fatalf("uploaded bytes = %v, want original transient audio bytes", uploadBody)
	}
	for _, key := range []string{"redact_pii", "redact_pii_sub", "redact_pii_policies"} {
		if _, exists := transcriptRequest[key]; exists {
			t.Fatalf("app-side mode must preserve the full provider transcript; unexpected %s request field: %#v", key, transcriptRequest[key])
		}
	}
	models, ok := transcriptRequest["speech_models"].([]any)
	if !ok || len(models) != 2 || models[0] != "universal-3-5-pro" || models[1] != "universal-2" {
		t.Fatalf("speech_models = %#v, want the documented Universal-3.5 Pro/Universal-2 fallback", transcriptRequest["speech_models"])
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
	if result.deletion.status != "unconfirmed" ||
		result.deletion.failureKind != "http_status" ||
		result.deletion.httpStatus != http.StatusServiceUnavailable ||
		result.deletion.attempts != assemblyAITranscriptDeleteAttempts {
		t.Fatalf("deletion diagnostics = %#v, want the last 503 after all retries", result.deletion)
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
	var diagnostic assemblyAIDiagnosticError
	if !errors.As(err, &diagnostic) ||
		diagnostic.stage != "poll" ||
		diagnostic.failureKind != "provider_transcription_error" ||
		diagnostic.deletion.status != "deleted" {
		t.Fatalf("diagnostic = %#v, want a sanitized provider transcription failure", diagnostic)
	}
}

func TestTranscribeAssemblyAIUploadFailureReportsSafeStageAndStatus(t *testing.T) {
	const privateProviderDetail = "synthetic private provider detail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"`+privateProviderDetail+`"}`)
	}))
	defer server.Close()

	_, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	if err == nil || deletionStatus != "deletion_failed" {
		t.Fatalf("transcription result = deletion %q, error %v; want upload failure", deletionStatus, err)
	}
	var diagnostic assemblyAIDiagnosticError
	if !errors.As(err, &diagnostic) ||
		diagnostic.stage != "upload" ||
		diagnostic.failureKind != "http_status" ||
		diagnostic.httpStatus != http.StatusUnauthorized ||
		diagnostic.deletion.status != "not_attempted" {
		t.Fatalf("diagnostic = %#v, want sanitized upload 401 with cleanup not attempted", diagnostic)
	}
	attrs := fmt.Sprint(assemblyAIDiagnosticLogAttrs(err))
	if strings.Contains(err.Error(), privateProviderDetail) ||
		strings.Contains(attrs, privateProviderDetail) ||
		strings.Contains(attrs, "synthetic-test-key") {
		t.Fatalf("provider details or credentials leaked into diagnostics: %q", attrs)
	}
	if !strings.Contains(attrs, "401") || !strings.Contains(attrs, "upload") {
		t.Fatalf("diagnostic log attributes = %q, want upload stage and HTTP 401", attrs)
	}
}

func TestTranscribeAssemblyAISubmissionClassifiesProviderErrorWithoutLoggingBody(t *testing.T) {
	const privateProviderDetail = "private synthetic media locator"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid audio_url: `+privateProviderDetail+`"}`)
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	var diagnostic assemblyAIDiagnosticError
	if err == nil || deletionStatus != "deletion_failed" ||
		!errors.As(err, &diagnostic) ||
		diagnostic.stage != "transcript_submission" ||
		diagnostic.failureKind != "http_status" ||
		diagnostic.httpStatus != http.StatusBadRequest ||
		diagnostic.providerErrorCategory != "audio_input" ||
		diagnostic.deletion.status != "not_attempted" {
		t.Fatalf("diagnostic=%#v deletion=%q error=%v; want a classified submission 400 without cleanup", diagnostic, deletionStatus, err)
	}
	attrs := fmt.Sprint(assemblyAIDiagnosticLogAttrs(err))
	if strings.Contains(err.Error(), privateProviderDetail) ||
		strings.Contains(attrs, privateProviderDetail) ||
		strings.Contains(attrs, "synthetic-test-key") {
		t.Fatalf("provider response body or credentials leaked into diagnostics: %q", attrs)
	}
	if !strings.Contains(attrs, "audio_input") {
		t.Fatalf("diagnostic log attrs = %q, want only the safe audio-input category", attrs)
	}
}

func TestClassifyAssemblyAIProviderErrorIdentifiesRedactionFieldSafely(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing policy list",
			body: `{"error":"You must explicitly define 'redact_pii_policies'"}`,
			want: "privacy_policy_configuration",
		},
		{
			name: "invalid substitution",
			body: `{"error":"redact_pii_sub must be a supported value"}`,
			want: "privacy_substitution_configuration",
		},
		{
			name: "other redaction error",
			body: `{"error":"redact_pii could not be enabled"}`,
			want: "privacy_configuration",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyAssemblyAIProviderError([]byte(test.body)); got != test.want {
				t.Fatalf("category = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTranscribeAssemblyAIProviderFailureReportsCleanupStatusSafely(t *testing.T) {
	const privateProviderDetail = "synthetic private transcript detail"
	const privateDeleteDetail = "synthetic private deletion detail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/upload":
			_, _ = io.WriteString(w, `{"upload_url":"https://cdn.assemblyai.com/upload/synthetic"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/transcript":
			_, _ = io.WriteString(w, `{"id":"synthetic-transcript-id"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			_, _ = io.WriteString(w, `{"status":"error","error":"`+privateProviderDetail+`"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/transcript/synthetic-transcript-id":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":"`+privateDeleteDetail+`"}`)
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, deletionStatus, err := transcribeAssemblyAI(
		context.Background(), server.Client(), server.URL, "synthetic-test-key", []byte{1}, "",
	)
	if err == nil || deletionStatus != "deletion_failed" {
		t.Fatalf("transcription result = deletion %q, error %v; want provider and cleanup failure", deletionStatus, err)
	}
	var diagnostic assemblyAIDiagnosticError
	if !errors.As(err, &diagnostic) ||
		diagnostic.stage != "poll" ||
		diagnostic.failureKind != "provider_transcription_error" ||
		diagnostic.deletion.status != "unconfirmed" ||
		diagnostic.deletion.failureKind != "http_status" ||
		diagnostic.deletion.httpStatus != http.StatusServiceUnavailable ||
		diagnostic.deletion.attempts != assemblyAITranscriptDeleteAttempts {
		t.Fatalf("diagnostic = %#v, want provider failure and final cleanup 503", diagnostic)
	}
	attrs := fmt.Sprint(assemblyAIDiagnosticLogAttrs(err))
	for _, privateValue := range []string{
		privateProviderDetail,
		privateDeleteDetail,
		"synthetic-test-key",
		"synthetic-transcript-id",
	} {
		if strings.Contains(attrs, privateValue) {
			t.Fatalf("sensitive provider value leaked into diagnostic attributes: %q", attrs)
		}
	}
	if !strings.Contains(attrs, "503") || !strings.Contains(attrs, "cleanup_attempts 3") {
		t.Fatalf("diagnostic log attributes = %q, want final HTTP 503 and retry count", attrs)
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

func TestTranscribeAssemblyAICancellationStopsUploadRequest(t *testing.T) {
	transport := &waitForRequestCancellationTransport{started: make(chan struct{})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := transcribeAssemblyAI(
			ctx,
			&http.Client{Transport: transport},
			"https://api.assemblyai.com",
			"synthetic-test-key",
			[]byte{1},
			"",
		)
		result <- err
	}()

	select {
	case <-transport.started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider upload request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("transcription error = %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transcription did not stop after cancellation")
	}
}

type waitForRequestCancellationTransport struct {
	started chan struct{}
}

func (t *waitForRequestCancellationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	close(t.started)
	<-request.Context().Done()
	return nil, request.Context().Err()
}
