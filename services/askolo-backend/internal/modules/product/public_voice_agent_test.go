package product

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"askolo/backend/internal/platform/publicws"
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
	got := redactAssistantSensitiveValues("Please invite person@example.com to the project.")
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

func TestNormalizeVoiceAgentTranscriptEventShapes(t *testing.T) {
	tests := []struct {
		name          string
		eventType     string
		payload       string
		wantFinalUser string
		wantText      string
		wantDelta     string
		wantItemID    string
		wantReplyID   string
		wantValid     bool
	}{
		{
			name: "user partial", eventType: "transcript.user.delta",
			payload:  `{"type":"transcript.user.delta","item_id":"item_abc123","text":"What's the weather"}`,
			wantText: "What's the weather", wantItemID: "item_abc123", wantValid: true,
		},
		{
			name: "user final", eventType: "transcript.user",
			payload:       `{"type":"transcript.user","item_id":"item_abc123","text":"What's the weather in Tokyo?"}`,
			wantFinalUser: "What's the weather in Tokyo?", wantText: "What's the weather in Tokyo?",
			wantItemID: "item_abc123", wantValid: true,
		},
		{
			name: "agent delta", eventType: "transcript.agent.delta",
			payload:   `{"type":"transcript.agent.delta","item_id":"item_abc123","reply_id":"reply_abc123","delta":"sunny"}`,
			wantDelta: "sunny", wantItemID: "item_abc123", wantReplyID: "reply_abc123", wantValid: true,
		},
		{
			name: "interrupted agent final", eventType: "transcript.agent",
			payload:  `{"type":"transcript.agent","item_id":"item_abc123","reply_id":"reply_abc123","text":"It's sunny","interrupted":true}`,
			wantText: "It's sunny", wantItemID: "item_abc123", wantReplyID: "reply_abc123", wantValid: true,
		},
		{
			name: "missing agent reply ID", eventType: "transcript.agent.delta",
			payload: `{"type":"transcript.agent.delta","item_id":"item_abc123","delta":"sunny"}`,
		},
		{
			name: "unexpected event", eventType: "session.ready",
			payload: `{"type":"session.ready"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, finalUserText, ok := normalizeVoiceAgentTranscriptEvent(test.eventType, []byte(test.payload))
			if ok != test.wantValid {
				t.Fatalf("valid = %v, want %v", ok, test.wantValid)
			}
			if !ok {
				return
			}
			if finalUserText != test.wantFinalUser {
				t.Fatalf("final user text = %q, want %q", finalUserText, test.wantFinalUser)
			}
			var event map[string]json.RawMessage
			if err := json.Unmarshal(normalized, &event); err != nil {
				t.Fatalf("decode normalized event: %v", err)
			}
			for key, want := range map[string]string{
				"type": test.eventType, "text": test.wantText, "delta": test.wantDelta,
				"item_id": test.wantItemID, "reply_id": test.wantReplyID,
			} {
				if want == "" {
					continue
				}
				var got string
				if err := json.Unmarshal(event[key], &got); err != nil || got != want {
					t.Errorf("%s = %q, want %q (err %v)", key, got, want, err)
				}
			}
			if test.eventType == "transcript.agent" {
				var interrupted bool
				if err := json.Unmarshal(event["interrupted"], &interrupted); err != nil || !interrupted {
					t.Errorf("interrupted = %v, err %v; want true", interrupted, err)
				}
			}
		})
	}
}

func TestVoiceAgentCannotStartWhileFeatureGateIsDisabled(t *testing.T) {
	handler := &Handler{}
	if _, _, err := handler.StartVoiceAgent(
		context.Background(), "user", "", publicws.VoiceAgentRequest{},
	); err == nil {
		t.Fatal("expected disabled Voice Agent to reject the session before setup")
	}
}
