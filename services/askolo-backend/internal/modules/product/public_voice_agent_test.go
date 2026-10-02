package product

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestVoiceAgentSessionUpdateUsesClosedFlatToolSchema(t *testing.T) {
	update := voiceAgentSessionUpdate()
	session, ok := update["session"].(map[string]any)
	if !ok {
		t.Fatal("session.update has no session object")
	}
	tools, ok := session["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected exactly one provider tool, got %#v", session["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "function" || tool["name"] != voiceAgentToolName {
		t.Fatalf("provider tool is not flat or has an unexpected name: %#v", tools[0])
	}
	parameters, ok := tool["parameters"].(map[string]any)
	if !ok || parameters["type"] != "object" || parameters["additionalProperties"] != false {
		t.Fatalf("provider tool arguments are not closed-world: %#v", tool["parameters"])
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok || len(properties) != 0 {
		t.Fatalf("provider tool must not define executable arguments: %#v", parameters["properties"])
	}
}

func TestDecodeVoiceAgentAudioAcceptsOnlyBoundedEvenPCM(t *testing.T) {
	valid := []byte{0, 1, 2, 3}
	decoded, err := decodeVoiceAgentAudio(base64.StdEncoding.EncodeToString(valid))
	if err != nil || string(decoded) != string(valid) {
		t.Fatalf("valid PCM was rejected or changed: decoded=%v err=%v", decoded, err)
	}

	for name, encoded := range map[string]string{
		"empty":          "",
		"invalid-base64": "not base64",
		"odd-length":     base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
		"too-large":      base64.StdEncoding.EncodeToString(make([]byte, assemblyAIVoiceAgentMaxMessage+2)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeVoiceAgentAudio(encoded); err == nil {
				t.Fatal("invalid PCM frame was accepted")
			}
		})
	}
}

func TestRedactVoiceAgentAssistantTranscriptRemovesEmail(t *testing.T) {
	got := redactVoiceAgentAssistantTranscript("Please invite person@example.com to the project.")
	if strings.Contains(got, "person@example.com") || !strings.Contains(got, "[REDACTED_EMAIL]") {
		t.Fatalf("email was not redacted: %q", got)
	}
}

func TestValidAssemblyAIVoiceAgentSessionID(t *testing.T) {
	for _, value := range []string{"session_123", "Session-abc.9"} {
		if !validAssemblyAIVoiceAgentSessionID(value) {
			t.Errorf("valid session ID %q was rejected", value)
		}
	}
	for _, value := range []string{"", "../session", " session", "session id", strings.Repeat("a", 129)} {
		if validAssemblyAIVoiceAgentSessionID(value) {
			t.Errorf("invalid session ID %q was accepted", value)
		}
	}
}

func TestVoiceAgentCreditPricingKeyIsSeparate(t *testing.T) {
	key, ok := usdEstimateRateCardKey("voice.agent")
	if !ok || key != "assemblyai:voice_agent:managed-voice-agent" {
		t.Fatalf("Voice Agent must use its own configurable rate card; got key=%q ok=%t", key, ok)
	}
}
