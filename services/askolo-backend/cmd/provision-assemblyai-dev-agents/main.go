package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"askolo/backend/internal/modules/product"
)

const agentsEndpoint = "https://agents.assemblyai.com/v1/agents"

type listedAgent struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	DeletedAt *string `json:"deleted_at"`
}

type agentRecord struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	SystemPrompt       string            `json:"system_prompt"`
	Greeting           *string           `json:"greeting"`
	LLM                []json.RawMessage `json:"llm"`
	PreConnectRequests []json.RawMessage `json:"pre_connect_requests"`
	Voice              struct {
		VoiceID string `json:"voice_id"`
	} `json:"voice"`
	Input struct {
		Format struct {
			Encoding   string `json:"encoding"`
			SampleRate int    `json:"sample_rate"`
		} `json:"format"`
	} `json:"input"`
	Output struct {
		Voice  string `json:"voice"`
		Format struct {
			Encoding   string `json:"encoding"`
			SampleRate int    `json:"sample_rate"`
		} `json:"format"`
	} `json:"output"`
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
		HTTP        json.RawMessage `json:"http"`
	} `json:"tools"`
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	if strings.TrimSpace(os.Getenv("REPLIT_ENVIRONMENT")) != "development" ||
		!strings.EqualFold(strings.TrimSpace(os.Getenv("ASKOLO_ENVIRONMENT")), "development") {
		return errors.New("refusing to contact AssemblyAI: both REPLIT_ENVIRONMENT and ASKOLO_ENVIRONMENT must be development")
	}
	apiKey := strings.TrimSpace(os.Getenv("ASSEMBLY_AI_API_KEY"))
	if apiKey == "" {
		return errors.New("ASSEMBLY_AI_API_KEY is not configured")
	}
	definitions, err := product.AssemblyAIAgentProvisioningDefinitions()
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	existing, err := listAgents(ctx, client, apiKey)
	if err != nil {
		return err
	}

	ids := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		var matches []listedAgent
		for _, candidate := range existing {
			if candidate.Name == definition.Name && candidate.DeletedAt == nil {
				matches = append(matches, candidate)
			}
		}
		if len(matches) > 1 {
			return fmt.Errorf("multiple AssemblyAI agents use the expected name for voice %s; no agents were modified", definition.VoiceKey)
		}
		if len(matches) == 1 {
			record, err := getAgent(ctx, client, apiKey, matches[0].ID)
			if err != nil {
				return err
			}
			if !matchesAgentDefinition(record, definition) {
				return fmt.Errorf("an agent named %q exists with different settings; it was left unchanged", definition.Name)
			}
			ids[definition.VoiceKey] = record.ID
		}
	}

	for _, definition := range definitions {
		if ids[definition.VoiceKey] != "" {
			continue
		}
		record, err := createAgent(ctx, client, apiKey, definition)
		if err != nil {
			return err
		}
		if !matchesAgentDefinition(record, definition) {
			return fmt.Errorf("AssemblyAI returned an unexpected configuration for voice %s; inspect the Development provider account before retrying", definition.VoiceKey)
		}
		ids[definition.VoiceKey] = record.ID
	}

	encoded, err := json.Marshal(ids)
	if err != nil {
		return errors.New("agent id map could not be encoded")
	}
	fmt.Printf("Development agents verified. Set ASKOLO_ASSEMBLYAI_AGENT_IDS to this JSON value in the Development environment:\n%s\n", encoded)
	return nil
}

func listAgents(ctx context.Context, client *http.Client, apiKey string) ([]listedAgent, error) {
	response, err := request(ctx, client, apiKey, http.MethodGet, agentsEndpoint, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AssemblyAI agent list failed with HTTP %d", response.StatusCode)
	}
	var agents []listedAgent
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&agents); err != nil {
		return nil, errors.New("AssemblyAI agent list response was invalid")
	}
	return agents, nil
}

func getAgent(ctx context.Context, client *http.Client, apiKey, id string) (agentRecord, error) {
	response, err := request(ctx, client, apiKey, http.MethodGet, agentsEndpoint+"/"+url.PathEscape(id), nil)
	if err != nil {
		return agentRecord{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return agentRecord{}, fmt.Errorf("AssemblyAI agent lookup failed with HTTP %d", response.StatusCode)
	}
	var record agentRecord
	if err := json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(&record); err != nil {
		return agentRecord{}, errors.New("AssemblyAI agent response was invalid")
	}
	return record, nil
}

func createAgent(ctx context.Context, client *http.Client, apiKey string, definition product.AssemblyAIAgentDefinition) (agentRecord, error) {
	body, err := json.Marshal(definition.Payload)
	if err != nil {
		return agentRecord{}, errors.New("AssemblyAI agent configuration could not be encoded")
	}
	response, err := request(ctx, client, apiKey, http.MethodPost, agentsEndpoint, body)
	if err != nil {
		return agentRecord{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return agentRecord{}, fmt.Errorf("AssemblyAI agent creation failed with HTTP %d", response.StatusCode)
	}
	var record agentRecord
	if err := json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(&record); err != nil {
		return agentRecord{}, errors.New("AssemblyAI agent creation response was invalid")
	}
	return record, nil
}

func request(ctx context.Context, client *http.Client, apiKey, method, endpoint string, body []byte) (*http.Response, error) {
	var input io.Reader
	if len(body) > 0 {
		input = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, input)
	if err != nil {
		return nil, errors.New("AssemblyAI request could not be constructed")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("AssemblyAI could not be reached")
	}
	return response, nil
}

func matchesAgentDefinition(record agentRecord, definition product.AssemblyAIAgentDefinition) bool {
	if record.ID == "" || record.Name != definition.Name || record.SystemPrompt != definition.Payload["system_prompt"] {
		return false
	}
	if record.Greeting == nil || *record.Greeting != definition.Payload["greeting"] {
		return false
	}
	expectedVoice, ok := definition.Payload["voice"].(map[string]string)
	if !ok || record.Voice.VoiceID != expectedVoice["voice_id"] {
		return false
	}
	if record.Input.Format.Encoding != "audio/pcm" || record.Input.Format.SampleRate != 24000 ||
		record.Output.Voice != expectedVoice["voice_id"] ||
		record.Output.Format.Encoding != "audio/pcm" || record.Output.Format.SampleRate != 24000 ||
		len(record.LLM) != 0 || len(record.PreConnectRequests) != 0 {
		return false
	}
	expectedTools, ok := definition.Payload["tools"].([]map[string]any)
	if !ok || len(record.Tools) != len(expectedTools) {
		return false
	}
	for index, expected := range expectedTools {
		if record.Tools[index].Name != expected["name"] || record.Tools[index].Description != expected["description"] {
			return false
		}
		if len(record.Tools[index].HTTP) > 0 && string(record.Tools[index].HTTP) != "null" {
			return false
		}
		var actualParameters any
		if json.Unmarshal(record.Tools[index].Parameters, &actualParameters) != nil {
			return false
		}
		if !reflect.DeepEqual(actualParameters, expected["parameters"]) {
			return false
		}
	}
	return true
}
