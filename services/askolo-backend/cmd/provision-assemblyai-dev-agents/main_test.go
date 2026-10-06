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
		name       string
		deployment string
		askoloEnv  string
	}{
		{name: "published Replit deployment", deployment: "1", askoloEnv: "development"},
		{name: "unknown nonempty deployment marker", deployment: "unexpected", askoloEnv: "development"},
		{name: "production Askolo environment", deployment: "", askoloEnv: "production"},
		{name: "missing Askolo environment", deployment: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("REPLIT_DEPLOYMENT", test.deployment)
			t.Setenv("ASKOLO_ENVIRONMENT", test.askoloEnv)
			t.Setenv("ASSEMBLY_AI_API_KEY", "test-only-placeholder")
			err := run(context.Background())
			if err == nil || !strings.Contains(err.Error(), "REPLIT_DEPLOYMENT must be unset and ASKOLO_ENVIRONMENT must be development") {
				t.Fatalf("expected environment guard to stop provisioning, got %v", err)
			}
		})
	}
}

func TestProvisioningAcceptsProjectEditorDevelopmentRuntimeBeforeKeyValidation(t *testing.T) {
	t.Setenv("REPLIT_DEPLOYMENT", "")
	t.Setenv("ASKOLO_ENVIRONMENT", "development")
	t.Setenv("ASSEMBLY_AI_API_KEY", "")

	err := run(context.Background())
	if err == nil || err.Error() != "ASSEMBLY_AI_API_KEY is not configured" {
		t.Fatalf("expected Development runtime to pass the environment guard before key validation, got %v", err)
	}
}

func TestListAgentsAcceptsCurrentPaginatedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agents":[{"id":"agent_dev_test","name":"Askolo Development - Test","deleted_at":null}],"has_more":false,"response_metadata":{"next_cursor":""}}`))
	}))
	defer server.Close()

	agents, err := listAgentsFromEndpoint(context.Background(), server.Client(), "test-only-placeholder", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Name != "Askolo Development - Test" {
		t.Fatalf("unexpected agent list result: count=%d", len(agents))
	}
}

func TestListAgentsRejectsIncompletePaginatedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agents":[],"has_more":true,"response_metadata":{"next_cursor":"redacted"}}`))
	}))
	defer server.Close()

	_, err := listAgentsFromEndpoint(context.Background(), server.Client(), "test-only-placeholder", server.URL)
	if err == nil || !strings.Contains(err.Error(), "more pages") {
		t.Fatalf("expected incomplete list to stop provisioning, got %v", err)
	}
}

func TestListAgentsAcceptsDocumentedArrayResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"agent_dev_test","name":"Askolo Development - Test","deleted_at":null}]`))
	}))
	defer server.Close()

	agents, err := listAgentsFromEndpoint(context.Background(), server.Client(), "test-only-placeholder", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != "agent_dev_test" {
		t.Fatalf("unexpected agent list result: count=%d", len(agents))
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
