package product

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"askolo/backend/internal/platform/publicws"
)

func TestVoiceAgentSessionUpdateBindsOnlyStoredAgentID(t *testing.T) {
	update := voiceAgentSessionUpdate("agent_test_123")
	session, ok := update["session"].(map[string]string)
	if !ok {
		t.Fatal("session.update has no session object")
	}
	if len(session) != 1 || session["agent_id"] != "agent_test_123" {
		t.Fatalf("stored-agent handshake must include only agent_id, got %#v", session)
	}
}

func TestAssemblyAIAgentProvisioningDefinitionsUseRegisteredClientTools(t *testing.T) {
	definitions, err := AssemblyAIAgentProvisioningDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	expectedTools, err := newAssistantToolRegistry().assemblyAIAgentTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 9 {
		t.Fatalf("expected 9 stored voice agents, got %d", len(definitions))
	}
	for _, definition := range definitions {
		tools, ok := definition.Payload["tools"].([]map[string]any)
		if !ok || !reflect.DeepEqual(tools, expectedTools) {
			t.Fatalf("%s must expose the exact registered client tools: %#v", definition.VoiceKey, definition.Payload["tools"])
		}
		for _, tool := range tools {
			if _, hasType := tool["type"]; hasType {
				t.Fatalf("%s stored-agent tools must not contain inline function type: %#v", definition.VoiceKey, tool)
			}
		}
	}
}

func TestVoiceAgentToolArgumentsRejectMalformedObjects(t *testing.T) {
	for _, raw := range []json.RawMessage{
		nil,
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"title":"first","title":"second"}`),
	} {
		if isAssistantJSONObject(raw) {
			t.Errorf("invalid tool arguments were accepted: %s", raw)
		}
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
