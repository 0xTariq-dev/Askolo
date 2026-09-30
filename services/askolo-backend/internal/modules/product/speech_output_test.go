package product

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

type speechTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip speechTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestAzureSpeechClientUsesFixedVoicesAndEscapesSSML(t *testing.T) {
	var requestBody string
	var request *http.Request
	client := &http.Client{Transport: speechTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		request = r
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		requestBody = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader("mp3-audio")),
		}, nil
	})}
	provider := newAzureSpeechProvider(
		"test-key",
		"westus",
		"https://westus.tts.speech.microsoft.com/cognitiveservices/v1",
		client,
	)

	audio, err := provider.Synthesize(context.Background(), "Hello <speak> & welcome", "ar")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if string(audio) != "mp3-audio" {
		t.Fatalf("Synthesize() audio = %q", audio)
	}
	if request == nil {
		t.Fatal("Azure request was not sent")
	}
	if request.Header.Get("Ocp-Apim-Subscription-Key") != "test-key" {
		t.Fatal("Azure subscription key header was not set")
	}
	if request.Header.Get("X-Microsoft-OutputFormat") != "audio-24khz-48kbitrate-mono-mp3" {
		t.Fatal("Azure output format was not set")
	}
	for _, expected := range []string{
		`xml:lang="ar-EG"`,
		`name="ar-EG-SalmaNeural"`,
		`Hello &lt;speak&gt; &amp; welcome`,
	} {
		if !strings.Contains(requestBody, expected) {
			t.Fatalf("SSML %q does not contain %q", requestBody, expected)
		}
	}
}

func TestAzureSpeechClientRejectsUnsafeEndpointsAndProviderResponses(t *testing.T) {
	for _, endpoint := range []string{
		"http://westus.tts.speech.microsoft.com/cognitiveservices/v1",
		"https://tts.speech.microsoft.com.evil.example/cognitiveservices/v1",
		"https://westus.tts.speech.microsoft.com/other",
	} {
		provider := newAzureSpeechProvider("test-key", "westus", endpoint, http.DefaultClient)
		if provider.Configured() {
			t.Errorf("provider accepted unsafe endpoint %q", endpoint)
		}
	}

	requests := 0
	provider := newAzureSpeechProvider("test-key", "westus", "", &http.Client{
		Transport: speechTestRoundTripper(func(*http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://attacker.example/collect"}},
				Body:       io.NopCloser(strings.NewReader("provider details must not escape")),
			}, nil
		}),
	})
	if _, err := provider.Synthesize(context.Background(), "Hello", "en"); err == nil {
		t.Fatal("Synthesize() accepted a redirect response")
	}
	if requests != 1 {
		t.Fatalf("Azure request count = %d; want one request without following redirects", requests)
	}
	if _, err := provider.Synthesize(context.Background(), "Hello", "xx"); err == nil {
		t.Fatal("Synthesize() accepted an unsupported locale")
	}
}

func TestVoiceOutputConsentIsSeparateAndRevocable(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	ctx := context.Background()
	userID := "voice-output-consent-test-user"

	consent, version, err := fixture.store.VoiceOutputConsent(ctx, userID)
	if err != nil || consent || version != "" {
		t.Fatalf("initial output consent = (%v, %q, %v), want (false, empty, nil)", consent, version, err)
	}
	if err := fixture.store.SetVoiceOutputConsent(ctx, userID, true); err != nil {
		t.Fatalf("enable output consent: %v", err)
	}
	consent, version, err = fixture.store.VoiceOutputConsent(ctx, userID)
	if err != nil || !consent || version != postgres.VoiceOutputConsentVersion {
		t.Fatalf("enabled output consent = (%v, %q, %v)", consent, version, err)
	}
	transcriptionConsent, _, err := fixture.store.VoiceConsent(ctx, userID)
	if err != nil || transcriptionConsent {
		t.Fatalf("output consent unexpectedly enabled transcription consent: (%v, %v)", transcriptionConsent, err)
	}
	if err := fixture.store.SetVoiceOutputConsent(ctx, userID, false); err != nil {
		t.Fatalf("revoke output consent: %v", err)
	}
	consent, version, err = fixture.store.VoiceOutputConsent(ctx, userID)
	if err != nil || consent || version != "" {
		t.Fatalf("revoked output consent = (%v, %q, %v)", consent, version, err)
	}
}
