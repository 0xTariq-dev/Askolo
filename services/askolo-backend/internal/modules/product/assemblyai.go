package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	assemblyAIRESTBaseURL              = "https://api.assemblyai.com"
	maxVoiceAudioBytes                 = 16 * 1024 * 1024
	maxVoiceAudioBase64Characters      = 24 * 1024 * 1024
	maxVoiceRecordingDurationMS        = 120_000
	assemblyAIPollTimeout              = 75 * time.Second
	assemblyAIPollInterval             = time.Second
	assemblyAITranscriptDeleteAttempts = 3
)

var (
	errAssemblyAIProviderFailure      = errors.New("assemblyai provider request failed")
	errAssemblyAITranscriptionTimeout = errors.New("assemblyai transcription timed out")
	errAssemblyAIAudioTooLong         = errors.New("recording exceeds the allowed duration")
)

type assemblyAITranscriptionResult struct {
	Transcript        string
	Confidence        *float64
	Words             []assemblyAIWord
	AudioDurationMs   int64
	ProviderRequestID string
	deletion          assemblyAIDeletionDiagnostics
}

type assemblyAIWord struct {
	Text       string   `json:"text"`
	StartMS    *int     `json:"start"`
	EndMS      *int     `json:"end"`
	Confidence *float64 `json:"confidence"`
}

type transcriptionReviewSignal struct {
	Kind       string  `json:"kind"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	StartMS    *int    `json:"startMs"`
	EndMS      *int    `json:"endMs"`
}

type transcriptionDeletion struct {
	RawAudio           string `json:"rawAudio"`
	ProviderTranscript string `json:"providerTranscript"`
	Marker             string `json:"marker"`
}

type assemblyAITranscriptResponse struct {
	ID            string           `json:"id"`
	Status        string           `json:"status"`
	Text          string           `json:"text"`
	Confidence    *float64         `json:"confidence"`
	AudioDuration float64          `json:"audio_duration"`
	Words         []assemblyAIWord `json:"words"`
}

type assemblyAIHTTPStatusError struct {
	statusCode int
}

type assemblyAIDeletionDiagnostics struct {
	status      string
	attempts    int
	httpStatus  int
	failureKind string
}

type assemblyAIDiagnosticError struct {
	stage               string
	failureKind         string
	httpStatus          int
	lastPollFailureKind string
	lastPollHTTPStatus  int
	deletion            assemblyAIDeletionDiagnostics
	cause               error
}

func (e assemblyAIDiagnosticError) Error() string {
	if e.cause == nil {
		return errAssemblyAIProviderFailure.Error()
	}
	return e.cause.Error()
}

func (e assemblyAIDiagnosticError) Unwrap() error {
	return e.cause
}

func newAssemblyAIDiagnosticError(
	stage string,
	failureKind string,
	httpStatus int,
	deletion assemblyAIDeletionDiagnostics,
	cause error,
) error {
	if cause == nil {
		cause = errAssemblyAIProviderFailure
	}
	return assemblyAIDiagnosticError{
		stage:       stage,
		failureKind: failureKind,
		httpStatus:  httpStatus,
		deletion:    deletion,
		cause:       cause,
	}
}

func assemblyAIRequestFailure(err error) (string, int) {
	var statusError assemblyAIHTTPStatusError
	if errors.As(err, &statusError) {
		return "http_status", statusError.statusCode
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request_timeout", 0
	}
	return "network_error", 0
}

func assemblyAIDiagnosticLogAttrs(err error) []any {
	var diagnostic assemblyAIDiagnosticError
	if !errors.As(err, &diagnostic) {
		return []any{"stage", "unknown", "failure_kind", "unclassified"}
	}

	deletionStatus := diagnostic.deletion.status
	if deletionStatus == "" {
		deletionStatus = "not_attempted"
	}
	attrs := []any{
		"stage", diagnostic.stage,
		"failure_kind", diagnostic.failureKind,
		"cleanup_status", deletionStatus,
		"cleanup_attempts", diagnostic.deletion.attempts,
	}
	if diagnostic.httpStatus > 0 {
		attrs = append(attrs, "http_status", diagnostic.httpStatus)
	}
	if diagnostic.lastPollFailureKind != "" {
		attrs = append(attrs, "last_poll_failure_kind", diagnostic.lastPollFailureKind)
	}
	if diagnostic.lastPollHTTPStatus > 0 {
		attrs = append(attrs, "last_poll_http_status", diagnostic.lastPollHTTPStatus)
	}
	if diagnostic.deletion.failureKind != "" {
		attrs = append(attrs, "cleanup_failure_kind", diagnostic.deletion.failureKind)
	}
	if diagnostic.deletion.httpStatus > 0 {
		attrs = append(attrs, "cleanup_http_status", diagnostic.deletion.httpStatus)
	}
	return attrs
}

func assemblyAINotAttemptedDeletion() assemblyAIDeletionDiagnostics {
	return assemblyAIDeletionDiagnostics{
		status:      "not_attempted",
		failureKind: "not_attempted",
	}
}

func (d assemblyAIDeletionDiagnostics) handlerStatus() string {
	if d.status == "deleted" {
		return "deleted"
	}
	return "deletion_failed"
}

func audioDurationMillis(seconds float64) int64 {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > float64(maxVoiceRecordingDurationMS)/1000 {
		return 0
	}
	// Convert the provider's decimal representation without using the result
	// as a money amount; billing receives only this bounded integer meter.
	text := strconv.FormatFloat(seconds, 'f', 6, 64)
	parts := strings.SplitN(text, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > maxVoiceRecordingDurationMS/1000 {
		return 0
	}
	fraction := int64(0)
	if len(parts) == 2 {
		f := parts[1]
		for len(f) < 3 {
			f += "0"
		}
		roundUp := len(f) > 3 && strings.Trim(f[3:], "0") != ""
		if len(f) > 3 {
			f = f[:3]
		}
		fraction, err = strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0
		}
		if roundUp {
			fraction++
			if fraction == 1000 {
				whole++
				fraction = 0
			}
		}
	}
	ms := whole*1000 + fraction
	if ms < 1 || ms > maxVoiceRecordingDurationMS {
		return 0
	}
	return ms
}

func (e assemblyAIHTTPStatusError) Error() string {
	return errAssemblyAIProviderFailure.Error()
}

func (e assemblyAIHTTPStatusError) Unwrap() error {
	return errAssemblyAIProviderFailure
}

func normalizeVoiceLanguage(language string) (string, bool) {
	language = strings.TrimSpace(language)
	if language == "" {
		return "", true
	}
	if len(language) > 20 {
		return "", false
	}
	parts := strings.Split(language, "-")
	if len(parts) > 4 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return "", false
	}
	for _, char := range parts[0] {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z') {
			return "", false
		}
	}
	for _, part := range parts[1:] {
		if len(part) < 2 || len(part) > 8 {
			return "", false
		}
		for _, char := range part {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
				return "", false
			}
		}
	}
	return strings.ToLower(parts[0]), true
}

func supportedVoiceMimeType(mimeType string) bool {
	switch strings.TrimSpace(mimeType) {
	case "audio/webm", "audio/mp4", "audio/m4a", "audio/wav", "audio/ogg", "audio/mpeg":
		return true
	default:
		return false
	}
}

func transcribeAssemblyAI(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	audio []byte,
	language string,
) (assemblyAITranscriptionResult, string, error) {
	var empty assemblyAITranscriptionResult
	notAttempted := assemblyAINotAttemptedDeletion()
	if len(audio) == 0 || len(audio) > maxVoiceAudioBytes || strings.TrimSpace(apiKey) == "" {
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("input", "invalid_input", 0, notAttempted, errAssemblyAIProviderFailure)
	}
	normalizedLanguage, validLanguage := normalizeVoiceLanguage(language)
	if !validLanguage {
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("input", "invalid_language", 0, notAttempted, errAssemblyAIProviderFailure)
	}
	if client == nil {
		client = http.DefaultClient
	}
	client = assemblyAINoRedirectClient(client)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = assemblyAIRESTBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	uploadResponse, cancel, err := assemblyAIRequest(
		ctx, client, http.MethodPost, baseURL+"/v2/upload", audio, apiKey, "application/octet-stream", 25*time.Second,
	)
	if err != nil {
		failureKind, statusCode := assemblyAIRequestFailure(err)
		if ctx.Err() != nil {
			return empty, notAttempted.handlerStatus(),
				newAssemblyAIDiagnosticError("upload", failureKind, statusCode, notAttempted, ctx.Err())
		}
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("upload", failureKind, statusCode, notAttempted, errAssemblyAIProviderFailure)
	}
	var uploaded struct {
		URL string `json:"upload_url"`
	}
	decodeErr := decodeAssemblyAIResponse(uploadResponse, &uploaded, 64*1024)
	cancel()
	if decodeErr != nil || !validAssemblyAIUploadURL(uploaded.URL) {
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("upload", "invalid_response", 0, notAttempted, errAssemblyAIProviderFailure)
	}

	submitPayload := map[string]any{
		"audio_url":      uploaded.URL,
		"speech_models":  []string{"universal-3-5-pro"},
		"speaker_labels": false,
		"redact_pii":     true,
		"redact_pii_sub": "hash",
	}
	if normalizedLanguage != "" {
		submitPayload["language_code"] = normalizedLanguage
	}
	payload, err := json.Marshal(submitPayload)
	if err != nil {
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("transcript_submission", "request_encoding", 0, notAttempted, errAssemblyAIProviderFailure)
	}
	submitResponse, cancel, err := assemblyAIRequest(
		ctx, client, http.MethodPost, baseURL+"/v2/transcript", payload, apiKey, "application/json", 25*time.Second,
	)
	if err != nil {
		failureKind, statusCode := assemblyAIRequestFailure(err)
		if ctx.Err() != nil {
			return empty, notAttempted.handlerStatus(),
				newAssemblyAIDiagnosticError("transcript_submission", failureKind, statusCode, notAttempted, ctx.Err())
		}
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("transcript_submission", failureKind, statusCode, notAttempted, errAssemblyAIProviderFailure)
	}
	var submitted struct {
		ID string `json:"id"`
	}
	decodeErr = decodeAssemblyAIResponse(submitResponse, &submitted, 64*1024)
	cancel()
	if decodeErr != nil || !validAssemblyAITranscriptID(submitted.ID) {
		return empty, notAttempted.handlerStatus(),
			newAssemblyAIDiagnosticError("transcript_submission", "invalid_response", 0, notAttempted, errAssemblyAIProviderFailure)
	}

	pollCtx, pollCancel := context.WithTimeout(ctx, assemblyAIPollTimeout)
	defer pollCancel()
	var lastPollFailureKind string
	var lastPollHTTPStatus int
	for {
		pollResponse, requestCancel, requestErr := assemblyAIRequest(
			pollCtx, client, http.MethodGet, baseURL+"/v2/transcript/"+url.PathEscape(submitted.ID), nil, apiKey, "", 10*time.Second,
		)
		if requestErr == nil {
			var transcript assemblyAITranscriptResponse
			decodeErr = decodeAssemblyAIResponse(pollResponse, &transcript, 2*1024*1024)
			requestCancel()
			if decodeErr == nil {
				lastPollFailureKind = ""
				lastPollHTTPStatus = 0
				switch transcript.Status {
				case "completed":
					deletion := deleteAssemblyAITranscript(ctx, client, baseURL, apiKey, submitted.ID)
					if strings.TrimSpace(transcript.Text) == "" || len(transcript.Text) > 2*1024*1024 {
						return empty, deletion.handlerStatus(),
							newAssemblyAIDiagnosticError("transcript_result", "invalid_transcript", 0, deletion, errAssemblyAIProviderFailure)
					}
					if math.IsNaN(transcript.AudioDuration) || math.IsInf(transcript.AudioDuration, 0) ||
						transcript.AudioDuration <= 0 || transcript.AudioDuration > float64(maxVoiceRecordingDurationMS)/1000 {
						return empty, deletion.handlerStatus(),
							newAssemblyAIDiagnosticError("transcript_result", "audio_duration_out_of_range", 0, deletion, errAssemblyAIAudioTooLong)
					}
					if transcript.Confidence != nil && (*transcript.Confidence < 0 || *transcript.Confidence > 1) {
						transcript.Confidence = nil
					}
					return assemblyAITranscriptionResult{
						Transcript:        transcript.Text,
						Confidence:        transcript.Confidence,
						Words:             transcript.Words,
						AudioDurationMs:   audioDurationMillis(transcript.AudioDuration),
						ProviderRequestID: submitted.ID,
						deletion:          deletion,
					}, deletion.handlerStatus(), nil
				case "error":
					deletion := deleteAssemblyAITranscript(ctx, client, baseURL, apiKey, submitted.ID)
					return empty, deletion.handlerStatus(),
						newAssemblyAIDiagnosticError("poll", "provider_transcription_error", 0, deletion, errAssemblyAIProviderFailure)
				}
			} else {
				lastPollFailureKind = "invalid_response"
				lastPollHTTPStatus = 0
			}
		} else {
			lastPollFailureKind, lastPollHTTPStatus = assemblyAIRequestFailure(requestErr)
			var statusError assemblyAIHTTPStatusError
			if errors.As(requestErr, &statusError) &&
				statusError.statusCode < http.StatusInternalServerError &&
				statusError.statusCode != http.StatusRequestTimeout &&
				statusError.statusCode != http.StatusTooManyRequests {
				deletion := deleteAssemblyAITranscript(ctx, client, baseURL, apiKey, submitted.ID)
				return empty, deletion.handlerStatus(),
					newAssemblyAIDiagnosticError("poll", "http_status", statusError.statusCode, deletion, errAssemblyAIProviderFailure)
			}
		}

		timer := time.NewTimer(assemblyAIPollInterval)
		select {
		case <-pollCtx.Done():
			timer.Stop()
			deletion := deleteAssemblyAITranscript(ctx, client, baseURL, apiKey, submitted.ID)
			if ctx.Err() != nil {
				return empty, deletion.handlerStatus(),
					newAssemblyAIDiagnosticError("poll", "request_cancelled", 0, deletion, ctx.Err())
			}
			return empty, deletion.handlerStatus(), assemblyAIDiagnosticError{
				stage:               "poll",
				failureKind:         "poll_timeout",
				lastPollFailureKind: lastPollFailureKind,
				lastPollHTTPStatus:  lastPollHTTPStatus,
				deletion:            deletion,
				cause:               errAssemblyAITranscriptionTimeout,
			}
		case <-timer.C:
		}
	}
}

func assemblyAIRequest(
	parent context.Context,
	client *http.Client,
	method string,
	endpoint string,
	body []byte,
	apiKey string,
	contentType string,
	timeout time.Duration,
) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	request.Header.Set("Authorization", apiKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(request)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		statusCode := response.StatusCode
		response.Body.Close()
		cancel()
		return nil, nil, assemblyAIHTTPStatusError{statusCode: statusCode}
	}
	return response, cancel, nil
}

func decodeAssemblyAIResponse(response *http.Response, target any, maxBytes int64) error {
	defer response.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxBytes))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func validAssemblyAIUploadURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil &&
		parsed.Scheme == "https" &&
		parsed.Hostname() == "cdn.assemblyai.com" &&
		parsed.User == nil &&
		parsed.Port() == "" &&
		strings.HasPrefix(parsed.Path, "/upload/")
}

func validAssemblyAITranscriptID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func deleteAssemblyAITranscript(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	apiKey string,
	transcriptID string,
) assemblyAIDeletionDiagnostics {
	if client == nil {
		client = http.DefaultClient
	}
	client = assemblyAINoRedirectClient(client)
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer cancelCleanup()
	endpoint := strings.TrimRight(baseURL, "/") + "/v2/transcript/" + url.PathEscape(transcriptID)
	outcome := assemblyAIDeletionDiagnostics{status: "unconfirmed", failureKind: "request_error"}
	for attempt := 1; attempt <= assemblyAITranscriptDeleteAttempts; attempt++ {
		outcome.attempts = attempt
		requestCtx, cancel := context.WithTimeout(cleanupCtx, 2*time.Second)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodDelete, endpoint, nil)
		if err == nil {
			request.Header.Set("Authorization", apiKey)
			response, doErr := client.Do(request)
			if doErr == nil {
				status := response.StatusCode
				outcome.httpStatus = status
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 16*1024))
				response.Body.Close()
				cancel()
				if status >= http.StatusOK && status < http.StatusMultipleChoices || status == http.StatusNotFound {
					return assemblyAIDeletionDiagnostics{
						status:     "deleted",
						attempts:   attempt,
						httpStatus: status,
					}
				}
				outcome.failureKind = "http_status"
			} else {
				if errors.Is(doErr, context.DeadlineExceeded) {
					outcome.failureKind = "request_timeout"
				} else {
					outcome.failureKind = "network_error"
				}
				cancel()
			}
		} else {
			outcome.failureKind = "request_error"
			cancel()
		}
		if attempt < assemblyAITranscriptDeleteAttempts {
			timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
			select {
			case <-cleanupCtx.Done():
				timer.Stop()
				outcome.failureKind = "cleanup_timeout"
				return outcome
			case <-timer.C:
			}
		}
	}
	return outcome
}

func assemblyAINoRedirectClient(client *http.Client) *http.Client {
	copyOfClient := *client
	copyOfClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copyOfClient
}

func buildTranscriptionReviewSignals(words []assemblyAIWord) []transcriptionReviewSignal {
	signals := make([]transcriptionReviewSignal, 0)
	seen := make(map[string]struct{})
	for _, word := range words {
		text := strings.Join(strings.Fields(word.Text), " ")
		if text == "" || len(text) > 200 || word.Confidence == nil ||
			*word.Confidence < 0 || *word.Confidence >= 0.78 || !isCriticalVoiceEntity(text) {
			continue
		}
		start := word.StartMS
		end := word.EndMS
		if start != nil && *start < 0 {
			start = nil
		}
		if end != nil && (*end < 0 || start != nil && *end < *start) {
			end = nil
		}
		key := "unknown"
		if start != nil {
			key = fmt.Sprintf("%d", *start)
		}
		key += ":" + text
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		signals = append(signals, transcriptionReviewSignal{
			Kind:       "low_confidence_entity",
			Text:       text,
			Confidence: *word.Confidence,
			StartMS:    start,
			EndMS:      end,
		})
		if len(signals) == 20 {
			break
		}
	}
	return signals
}

func isCriticalVoiceEntity(text string) bool {
	for _, char := range text {
		if char >= '0' && char <= '9' || strings.ContainsRune("@#$%", char) {
			return true
		}
	}
	lower := strings.ToLower(text)
	return strings.HasSuffix(lower, "am") || strings.HasSuffix(lower, "pm")
}
