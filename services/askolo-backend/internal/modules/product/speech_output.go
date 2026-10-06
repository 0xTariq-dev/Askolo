package product

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"askolo/backend/internal/adapters/postgres"
	policy "askolo/backend/internal/platform/authorization"
)

const (
	maxAssistantSpeechCharacters = 12_000
	maxAssistantSpeechBytes      = 4 << 20
	assistantSpeechTimeout       = 22 * time.Second
)

var errAzureSpeechUnavailable = errors.New("azure speech synthesis unavailable")

type azureSpeechProvider interface {
	Configured() bool
	Synthesize(context.Context, string, string) ([]byte, error)
}

type azureSpeechClient struct {
	key      string
	endpoint string
	client   *http.Client
}

func newAzureSpeechProvider(key, region, rawURL string, client *http.Client) azureSpeechProvider {
	if client == nil {
		client = &http.Client{Timeout: assistantSpeechTimeout}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	endpoint, _ := azureSpeechEndpoint(region, rawURL)
	return &azureSpeechClient{
		key:      strings.TrimSpace(key),
		endpoint: endpoint,
		client:   &clientCopy,
	}
}

func azureSpeechEndpoint(region, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		region = strings.TrimSpace(region)
		if region == "" || strings.ContainsAny(region, "/ .:\\") {
			return "", errors.New("invalid Azure Speech region")
		}
		rawURL = "https://" + region + ".tts.speech.microsoft.com/cognitiveservices/v1"
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid Azure Speech endpoint")
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".tts.speech.microsoft.com") &&
		!strings.HasSuffix(host, ".cognitiveservices.azure.com") {
		return "", errors.New("Azure Speech endpoint host is not allowed")
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return "", errors.New("Azure Speech endpoint port is not allowed")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/cognitiveservices/v1"
	} else if parsed.Path != "/cognitiveservices/v1" {
		return "", errors.New("invalid Azure Speech endpoint path")
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (c *azureSpeechClient) Configured() bool {
	return c != nil && c.client != nil && c.endpoint != "" &&
		strings.TrimSpace(c.key) != "" && !strings.ContainsAny(c.key, "\r\n")
}

func (c *azureSpeechClient) Synthesize(ctx context.Context, text, locale string) ([]byte, error) {
	if !c.Configured() {
		return nil, errAzureSpeechUnavailable
	}
	voice, speechLocale := azureSpeechVoice(locale)
	if voice == "" {
		return nil, errors.New("unsupported speech locale")
	}
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxAssistantSpeechCharacters {
		return nil, errors.New("invalid speech text")
	}

	var escaped strings.Builder
	if err := xml.EscapeText(&escaped, []byte(text)); err != nil {
		return nil, errors.New("speech text could not be encoded")
	}
	ssml := `<speak version="1.0" xml:lang="` + speechLocale + `"><voice name="` + voice + `">` + escaped.String() + `</voice></speak>`

	requestCtx, cancel := context.WithTimeout(ctx, assistantSpeechTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, strings.NewReader(ssml))
	if err != nil {
		return nil, errAzureSpeechUnavailable
	}
	request.Header.Set("Content-Type", "application/ssml+xml")
	request.Header.Set("Accept", "audio/mpeg")
	request.Header.Set("X-Microsoft-OutputFormat", "audio-24khz-48kbitrate-mono-mp3")
	request.Header.Set("Ocp-Apim-Subscription-Key", c.key)

	response, err := c.client.Do(request)
	if err != nil {
		return nil, errAzureSpeechUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errAzureSpeechUnavailable
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "audio/mpeg" {
		return nil, errAzureSpeechUnavailable
	}
	audio, err := io.ReadAll(io.LimitReader(response.Body, maxAssistantSpeechBytes+1))
	if err != nil || len(audio) == 0 || len(audio) > maxAssistantSpeechBytes {
		return nil, errAzureSpeechUnavailable
	}
	return audio, nil
}

func azureSpeechVoice(locale string) (voice, speechLocale string) {
	switch locale {
	case "ar":
		return "ar-EG-SalmaNeural", "ar-EG"
	case "en":
		return "en-US-AndrewMultilingualNeural", "en-US"
	default:
		return "", ""
	}
}

func (h *Handler) allowAssistantSpeech(ctx context.Context, userID string) (bool, string, error) {
	secret := strings.TrimSpace(h.authRateLimitSecret)
	if len([]byte(secret)) < 32 {
		return false, "", errors.New("assistant speech rate-limit secret is not configured")
	}
	for _, bucket := range []struct {
		name   string
		max    int
		window time.Duration
		retry  string
	}{
		{name: "minute", max: 10, window: time.Minute, retry: "60"},
		{name: "day", max: 100, window: 24 * time.Hour, retry: "86400"},
	} {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte("askolo:assistant-speech:v1\x00" + bucket.name + "\x00" + userID))
		allowed, err := h.store.AllowAuthRateLimitBucket(
			ctx,
			hex.EncodeToString(mac.Sum(nil)),
			bucket.max,
			bucket.window,
		)
		if err != nil {
			return false, "", err
		}
		if !allowed {
			return false, bucket.retry, nil
		}
	}
	return true, "", nil
}

func (h *Handler) voiceOutputPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	preferences, err := h.store.VoiceOutputPreferences(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice output preference lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) updateVoiceOutputPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input struct {
		Consent          *bool `json:"consent"`
		AutoSpeakEnabled *bool `json:"autoSpeakEnabled"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Consent == nil && input.AutoSpeakEnabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_VOICE_OUTPUT_PREFERENCES", "Choose at least one Azure speech setting to update.")
		return
	}
	if err := h.store.UpdateVoiceOutputPreferences(r.Context(), userID, input.Consent, input.AutoSpeakEnabled); err != nil {
		if errors.Is(err, postgres.ErrVoiceOutputConsentRequired) {
			writeError(w, http.StatusForbidden, "VOICE_OUTPUT_CONSENT_REQUIRED", "Accept the Azure speech notice before enabling automatic spoken replies.")
			return
		}
		h.storeError(w, "voice output preference update failed", err)
		return
	}
	preferences, err := h.store.VoiceOutputPreferences(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice output preference lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) assistantRunSpeech(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	consent, version, err := h.store.VoiceOutputConsent(r.Context(), userID)
	if err != nil {
		h.storeError(w, "voice output consent lookup failed", err)
		return
	}
	if !consent || version != postgres.VoiceOutputConsentVersion {
		writeError(w, http.StatusForbidden, "VOICE_OUTPUT_CONSENT_REQUIRED", "Consent is required for Azure speech output.")
		return
	}

	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" || len(runID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_RUN_ID", "A valid assistant run id is required.")
		return
	}
	run, err := h.store.GetAssistantRun(r.Context(), userID, runID)
	if err != nil {
		if errors.Is(err, postgres.ErrAssistantNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Assistant run not found.")
			return
		}
		h.storeError(w, "assistant speech run lookup failed", err)
		return
	}
	if run.State != "needs_confirmation" && run.State != "completed" {
		writeError(w, http.StatusConflict, "SPEECH_NOT_READY", "This assistant response is not ready for speech.")
		return
	}
	text := strings.TrimSpace(run.AssistantMessage)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxAssistantSpeechCharacters {
		writeError(w, http.StatusBadRequest, "SPEECH_TEXT_UNAVAILABLE", "This response cannot be read aloud.")
		return
	}
	if h.speechOutput == nil || !h.speechOutput.Configured() {
		writeError(w, http.StatusServiceUnavailable, "VOICE_SYNTHESIS_UNAVAILABLE", "Speech output is temporarily unavailable.")
		return
	}

	allowed, retryAfter, err := h.allowAssistantSpeech(r.Context(), userID)
	if err != nil {
		h.storeError(w, "assistant speech rate limit failed", err)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", retryAfter)
		writeError(w, http.StatusTooManyRequests, "VOICE_RATE_LIMITED", "Speech requests are temporarily limited.")
		return
	}

	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.storeError(w, "assistant speech locale lookup failed", err)
		return
	}
	locale := "en"
	if user.PreferredLocale != nil && *user.PreferredLocale == "ar" {
		locale = "ar"
	}
	ctx, cancel := context.WithTimeout(r.Context(), assistantSpeechTimeout)
	defer cancel()
	started := time.Now()
	audio, err := h.speechOutput.Synthesize(ctx, text, locale)
	if err != nil {
		h.logger.Warn("assistant speech synthesis failed",
			"run_id", runID,
			"error_type", fmt.Sprintf("%T", err),
			"duration_ms", time.Since(started).Milliseconds(),
		)
		writeError(w, http.StatusBadGateway, "VOICE_SYNTHESIS_FAILED", "Speech output could not be generated.")
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audio); err != nil {
		h.logger.Warn("assistant speech response write failed",
			"run_id", runID,
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}
