package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"askolo/backend/internal/modules/product"
)

func TestProvisioningRefusesNonDevelopmentEnvironmentBeforeNetworkAccess(t *testing.T) {
	tests := []struct {
		name      string
		replitEnv string
		askoloEnv string
	}{
		{name: "production Replit environment", replitEnv: "production", askoloEnv: "development"},
		{name: "production Askolo environment", replitEnv: "development", askoloEnv: "production"},
		{name: "missing Replit environment", askoloEnv: "development"},
		{name: "missing Askolo environment", replitEnv: "development"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("REPLIT_ENVIRONMENT", test.replitEnv)
			t.Setenv("ASKOLO_ENVIRONMENT", test.askoloEnv)
			t.Setenv("ASSEMBLY_AI_API_KEY", "test-only-placeholder")
			err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "both REPLIT_ENVIRONMENT and ASKOLO_ENVIRONMENT must be development") {
				t.Fatalf("expected environment guard to stop provisioning, got %v", err)
			}
		})
	}
}

func TestProvisioningVerificationRejectsProviderHostedTools(t *testing.T) {
	definitions, err := product.AssemblyAIAgentProvisioningDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) == 0 {
		t.Fatal("expected a stored-agent definition")
	}
	definition := definitions[0]
	expectedTools, ok := definition.Payload["tools"].([]map[string]any)
	if !ok || len(expectedTools) != 3 {
		t.Fatalf("unexpected tool configuration: %#v", definition.Payload["tools"])
	}
	greeting, _ := definition.Payload["greeting"].(string)
	voice, _ := definition.Payload["voice"].(map[string]string)
	record := agentRecord{
		ID:           "agent_dev_123",
		Name:         definition.Name,
		SystemPrompt: definition.Payload["system_prompt"].(string),
		Greeting:     &greeting,
	}
	for _, expectedTool := range expectedTools {
		parameters, err := json.Marshal(expectedTool["parameters"])
		if err != nil {
			t.Fatal(err)
		}
		record.Tools = append(record.Tools, struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
			HTTP        json.RawMessage `json:"http"`
		}{
			Name:        expectedTool["name"].(string),
			Description: expectedTool["description"].(string),
			Parameters:  parameters,
		})
	}
	record.Voice.VoiceID = voice["voice_id"]
	record.Input.Format.Encoding = "audio/pcm"
	record.Input.Format.SampleRate = 24000
	record.Output.Voice = voice["voice_id"]
	record.Output.Format.Encoding = "audio/pcm"
	record.Output.Format.SampleRate = 24000
	if !matchesAgentDefinition(record, definition) {
		t.Fatal("matching client-handled agent configuration was rejected")
	}
	record.Tools[0].HTTP = json.RawMessage(`{"url":"https://example.invalid"}`)
	if matchesAgentDefinition(record, definition) {
		t.Fatal("agent with a provider-hosted tool was accepted")
	}
}

func TestAssemblyAIRequestUsesVoiceAgentAuthorizationScheme(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := request(context.Background(), server.Client(), "test-only-placeholder", http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if authorization != "Bearer test-only-placeholder" {
		t.Fatalf("Voice Agent request authorization = %q, want Bearer scheme", authorization)
	}
}
