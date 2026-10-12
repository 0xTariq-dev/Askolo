package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	assistantPlannerModel                     = "gpt-5.6-luna"
	assistantPlannerTimeout                   = 20 * time.Second
	assistantPlannerInputTokenReservationCap  = int64(4096)
	assistantPlannerOutputTokenReservationCap = int64(256)
	assistantMaxOutputBytes                   = 16 * 1024
)

type assistantPlanner interface {
	Available() bool
	Plan(context.Context, string) (assistantModelPlan, error)
}

type assistantConversationMessage struct {
	Role    string
	Content string
}

type assistantHistoryPlanner interface {
	PlanWithHistory(context.Context, []assistantConversationMessage, string) (assistantModelPlan, error)
}

type assistantStreamingPlanner interface {
	PlanStream(
		context.Context,
		[]assistantConversationMessage,
		string,
		func(string) error,
	) (assistantModelPlan, error)
}

type assistantModelPlan struct {
	Intent            string              `json:"intent"`
	Arguments         json.RawMessage     `json:"arguments"`
	Message           string              `json:"message"`
	Questions         []assistantQuestion `json:"questions"`
	ProviderModel     string              `json:"-"`
	ProviderRequestID string              `json:"-"`
	InputTokens       int64               `json:"-"`
	OutputTokens      int64               `json:"-"`
	UsageValid        bool                `json:"-"`
}

type assistantQuestion struct {
	ID           string   `json:"id"`
	Prompt       string   `json:"prompt"`
	Type         string   `json:"type"`
	Options      []string `json:"options"`
	AllowWriteIn bool     `json:"allowWriteIn"`
	Required     bool     `json:"required"`
}

type openAIAssistantPlanner struct {
	apiKey  string
	baseURL string
	client  *http.Client
	tools   *assistantToolRegistry
}

func newOpenAIAssistantPlanner(apiKey, baseURL string, registries ...*assistantToolRegistry) assistantPlanner {
	tools := newAssistantToolRegistry()
	if len(registries) > 0 && registries[0] != nil {
		tools = registries[0]
	}
	return &openAIAssistantPlanner{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimSpace(baseURL),
		client:  &http.Client{Timeout: assistantPlannerTimeout},
		tools:   tools,
	}
}

func (p *openAIAssistantPlanner) Available() bool {
	return p.availabilityReason() == ""
}

func (p *openAIAssistantPlanner) availabilityReason() string {
	if p == nil || p.apiKey == "" {
		return "provider_key_missing"
	}
	parsed, err := url.Parse(strings.TrimSpace(p.baseURL))
	if err != nil || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "provider_base_url_invalid"
	}
	if parsed.Scheme == "https" {
		return ""
	}
	if parsed.Scheme == "http" && parsed.Port() == "1106" && isLoopbackHost(parsed.Hostname()) {
		return ""
	}
	return "provider_base_url_invalid"
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func assistantPlannerUnavailableReason(planner assistantPlanner) string {
	if planner == nil {
		return "planner_missing"
	}
	if configured, ok := planner.(*openAIAssistantPlanner); ok {
		return configured.availabilityReason()
	}
	if !planner.Available() {
		return "planner_not_ready"
	}
	return ""
}

func (p *openAIAssistantPlanner) Plan(ctx context.Context, transcript string) (assistantModelPlan, error) {
	return p.PlanWithHistory(ctx, nil, transcript)
}

func (p *openAIAssistantPlanner) PlanWithHistory(
	ctx context.Context,
	history []assistantConversationMessage,
	transcript string,
) (assistantModelPlan, error) {
	return p.plan(ctx, history, transcript, nil)
}

func (p *openAIAssistantPlanner) PlanStream(
	ctx context.Context,
	history []assistantConversationMessage,
	transcript string,
	onDelta func(string) error,
) (assistantModelPlan, error) {
	return p.plan(ctx, history, transcript, onDelta)
}

func (p *openAIAssistantPlanner) plan(
	ctx context.Context,
	history []assistantConversationMessage,
	transcript string,
	onDelta func(string) error,
) (assistantModelPlan, error) {
	if !p.Available() {
		return assistantModelPlan{}, errors.New("assistant provider is not configured")
	}
	tools := p.tools
	if tools == nil {
		tools = newAssistantToolRegistry()
	}
	systemPrompt, err := buildAssistantPlannerSystemPrompt(tools)
	if err != nil {
		return assistantModelPlan{}, err
	}
	base, err := url.Parse(p.baseURL)
	if err != nil {
		return assistantModelPlan{}, errors.New("assistant provider URL is invalid")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/chat/completions"
	base.RawPath = ""

	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	requestMessages := make([]chatMessage, 0, len(history)+2)
	requestMessages = append(requestMessages, chatMessage{Role: "system", Content: systemPrompt})
	if len(history) > 12 {
		history = history[len(history)-12:]
	}
	for _, message := range history {
		if (message.Role != "user" && message.Role != "assistant") ||
			!utf8.ValidString(message.Content) || len([]byte(message.Content)) > 4096 {
			continue
		}
		requestMessages = append(requestMessages, chatMessage{Role: message.Role, Content: message.Content})
	}
	requestMessages = append(requestMessages, chatMessage{Role: "user", Content: transcript})
	requestBody := struct {
		Model               string            `json:"model"`
		Messages            []chatMessage     `json:"messages"`
		ResponseFormat      map[string]string `json:"response_format"`
		MaxCompletionTokens int               `json:"max_completion_tokens"`
		Stream              bool              `json:"stream,omitempty"`
		StreamOptions       map[string]bool   `json:"stream_options,omitempty"`
	}{
		Model:               assistantPlannerModel,
		Messages:            requestMessages,
		ResponseFormat:      map[string]string{"type": "json_object"},
		MaxCompletionTokens: int(assistantPlannerOutputTokenReservationCap),
		Stream:              onDelta != nil,
	}
	if onDelta != nil {
		requestBody.StreamOptions = map[string]bool{"include_usage": true}
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return assistantModelPlan{}, errors.New("assistant request could not be prepared")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(encoded))
	if err != nil {
		return assistantModelPlan{}, errors.New("assistant request could not be prepared")
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if onDelta != nil {
		request.Header.Set("Accept", "text/event-stream")
	}

	response, err := p.client.Do(request)
	if err != nil {
		return assistantModelPlan{}, fmt.Errorf("assistant provider request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return assistantModelPlan{}, fmt.Errorf("assistant provider returned status %d", response.StatusCode)
	}
	if onDelta != nil {
		return readAssistantPlannerStream(response.Body, tools, onDelta)
	}
	responseBytes, err := io.ReadAll(io.LimitReader(response.Body, assistantMaxOutputBytes+1))
	if err != nil {
		return assistantModelPlan{}, errors.New("assistant provider response could not be read")
	}
	if len(responseBytes) > assistantMaxOutputBytes {
		return assistantModelPlan{}, errors.New("assistant provider response exceeded the size limit")
	}
	var envelope struct {
		Model string `json:"model"`
		ID    string `json:"id"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBytes, &envelope); err != nil {
		return assistantModelPlan{}, errors.New("assistant provider response was invalid")
	}
	plan := assistantModelPlan{}
	decodeErr := errors.New("assistant provider response did not contain a usable plan")
	if len(envelope.Choices) == 1 {
		plan, decodeErr = decodeAssistantModelPlan(envelope.Choices[0].Message.Content, tools)
	}
	plan.ProviderModel = envelope.Model
	plan.ProviderRequestID = envelope.ID
	plan.InputTokens = envelope.Usage.PromptTokens
	plan.OutputTokens = envelope.Usage.CompletionTokens
	plan.UsageValid = envelope.Usage.PromptTokens > 0 && envelope.Usage.CompletionTokens > 0
	if plan.ProviderModel != assistantPlannerModel {
		return plan, errors.New("assistant provider model mismatch")
	}
	if decodeErr != nil {
		return plan, decodeErr
	}
	if !plan.UsageValid {
		return plan, errors.New("assistant provider usage was missing")
	}
	return plan, nil
}

func decodeAssistantModelPlan(content string, registries ...*assistantToolRegistry) (assistantModelPlan, error) {
	registry := newAssistantToolRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	var plan assistantModelPlan
	if len([]byte(content)) == 0 || len([]byte(content)) > 8192 {
		return plan, errors.New("assistant intent response is invalid")
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return assistantModelPlan{}, errors.New("assistant intent response is invalid")
	}
	seen := make(map[string]bool, 4)
	for decoder.More() {
		keyToken, keyErr := decoder.Token()
		key, ok := keyToken.(string)
		if keyErr != nil || !ok || seen[key] {
			return assistantModelPlan{}, errors.New("assistant intent response is invalid")
		}
		seen[key] = true
		switch key {
		case "intent":
			err = decoder.Decode(&plan.Intent)
		case "arguments":
			err = decoder.Decode(&plan.Arguments)
		case "message":
			err = decoder.Decode(&plan.Message)
		case "questions":
			err = decoder.Decode(&plan.Questions)
		default:
			return assistantModelPlan{}, errors.New("assistant intent response has an unknown field")
		}
		if err != nil {
			return assistantModelPlan{}, errors.New("assistant intent response is invalid")
		}
	}
	if _, err := decoder.Token(); err != nil ||
		len(seen) != 4 || !seen["intent"] || !seen["arguments"] || !seen["message"] || !seen["questions"] {
		return assistantModelPlan{}, errors.New("assistant intent response is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return assistantModelPlan{}, errors.New("assistant intent response has trailing data")
	}
	plan.Intent = strings.TrimSpace(plan.Intent)
	if !validAssistantReply(plan.Message, true) {
		return assistantModelPlan{}, errors.New("assistant reply message is invalid")
	}
	switch plan.Intent {
	case assistantIntentNone:
		if !assistantArgumentsEmpty(plan.Arguments) {
			return assistantModelPlan{}, errors.New("assistant intent contains unexpected data")
		}
		if len(plan.Questions) != 0 {
			return assistantModelPlan{}, errors.New("assistant intent contains unexpected questions")
		}
	case assistantIntentClarify:
		if !assistantArgumentsEmpty(plan.Arguments) || !validAssistantQuestions(plan.Questions) {
			return assistantModelPlan{}, errors.New("assistant clarification questions are invalid")
		}
	default:
		if len(plan.Questions) != 0 {
			return assistantModelPlan{}, errors.New("assistant action contains unexpected questions")
		}
		if _, _, err := registry.prepare(plan.Intent, plan.Arguments); err != nil {
			return assistantModelPlan{}, errors.New("assistant tool selection is invalid")
		}
	}
	return plan, nil
}

func validAssistantQuestions(questions []assistantQuestion) bool {
	if len(questions) < 1 || len(questions) > 3 {
		return false
	}
	seen := make(map[string]struct{}, len(questions))
	for _, question := range questions {
		if !validAssistantToolName(question.ID) || len(question.ID) > 32 ||
			!validAssistantReply(question.Prompt, false) || len([]byte(question.Prompt)) > 240 {
			return false
		}
		if _, exists := seen[question.ID]; exists {
			return false
		}
		seen[question.ID] = struct{}{}
		switch question.Type {
		case "radio", "checkbox":
			if len(question.Options) < 2 || len(question.Options) > 6 {
				return false
			}
			if question.Type == "checkbox" && question.AllowWriteIn {
				return false
			}
			optionSet := make(map[string]struct{}, len(question.Options))
			for _, option := range question.Options {
				option = strings.TrimSpace(option)
				if option == "" || len([]byte(option)) > 100 || !utf8.ValidString(option) {
					return false
				}
				if _, exists := optionSet[option]; exists {
					return false
				}
				optionSet[option] = struct{}{}
			}
		case "text":
			if len(question.Options) != 0 || question.AllowWriteIn {
				return false
			}
		default:
			return false
		}
	}
	return true
}
