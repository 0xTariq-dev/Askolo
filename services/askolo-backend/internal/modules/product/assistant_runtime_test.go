package product

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreflightAssistantTranscript(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "trims whitespace", input: "  add milk  ", valid: true},
		{name: "accepts maximum UTF-8 byte length", input: strings.Repeat("é", assistantTranscriptLimit/2), valid: true},
		{name: "rejects empty", input: " \n\t "},
		{name: "rejects oversized UTF-8", input: strings.Repeat("é", assistantTranscriptLimit/2+1)},
		{name: "rejects invalid UTF-8", input: string([]byte{0xff})},
		{name: "rejects control characters", input: "add\x00milk"},
		{name: "accepts line breaks", input: "add milk\nand eggs", valid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, valid := preflightAssistantTranscript(test.input)
			if valid != test.valid {
				t.Fatalf("valid = %v, want %v", valid, test.valid)
			}
			if valid && normalized == "" {
				t.Fatal("valid transcript normalized to empty")
			}
		})
	}
}

func TestValidAssistantActionTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		valid bool
	}{
		{name: "ordinary title", title: "Buy milk", valid: true},
		{name: "maximum byte length", title: strings.Repeat("a", 120), valid: true},
		{name: "empty title"},
		{name: "too long", title: strings.Repeat("a", 121)},
		{name: "newline", title: "Buy\nmilk"},
		{name: "invalid UTF-8", title: string([]byte{0xff})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validAssistantActionTitle(test.title); got != test.valid {
				t.Fatalf("validAssistantActionTitle(%q) = %v, want %v", test.title, got, test.valid)
			}
		})
	}
}

func TestDecodeAssistantModelPlan(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantIntent string
		wantTitle  string
		wantErr   bool
	}{
		{name: "ordinary request", content: `{"intent":"none","title":""}`, wantIntent: "none"},
		{name: "clarification", content: `{"intent":"clarify","title":""}`, wantIntent: "clarify"},
		{name: "allow-listed action", content: `{"intent":"create_action_item","title":"Buy milk"}`, wantIntent: "create_action_item", wantTitle: "Buy milk"},
		{name: "unknown tool", content: `{"intent":"send_email","title":"Hello"}`, wantErr: true},
		{name: "extra field", content: `{"intent":"none","title":"","risk":"low"}`, wantErr: true},
		{name: "duplicate field", content: `{"intent":"none","intent":"create_action_item","title":"Buy milk"}`, wantErr: true},
		{name: "missing field", content: `{"intent":"none"}`, wantErr: true},
		{name: "wrong value type", content: `{"intent":[],"title":""}`, wantErr: true},
		{name: "title on non-action", content: `{"intent":"none","title":"Buy milk"}`, wantErr: true},
		{name: "empty action title", content: `{"intent":"create_action_item","title":""}`, wantErr: true},
		{name: "oversized action title", content: `{"intent":"create_action_item","title":"` + strings.Repeat("a", 121) + `"}`, wantErr: true},
		{name: "control character in action title", content: "{\"intent\":\"create_action_item\",\"title\":\"Buy\\nmilk\"}", wantErr: true},
		{name: "trailing JSON", content: `{"intent":"none","title":""} {}`, wantErr: true},
		{name: "markdown wrapper", content: "```json\n{\"intent\":\"none\",\"title\":\"\"}\n```", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := decodeAssistantModelPlan(test.content)
			if (err != nil) != test.wantErr {
				t.Fatalf("decodeAssistantModelPlan() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && (plan.Intent != test.wantIntent || plan.Title != test.wantTitle) {
				t.Fatalf("plan = %#v, want intent %q title %q", plan, test.wantIntent, test.wantTitle)
			}
		})
	}
}

func TestOpenAIAssistantPlannerRequiresSecureProviderURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantOK  bool
	}{
		{name: "secure provider URL", baseURL: "https://api.example.test/v1", wantOK: true},
		{name: "HTTP rejected", baseURL: "http://api.example.test/v1"},
		{name: "userinfo rejected", baseURL: "https://user:pass@api.example.test/v1"},
		{name: "query rejected", baseURL: "https://api.example.test/v1?redirect=other"},
		{name: "fragment rejected", baseURL: "https://api.example.test/v1#fragment"},
		{name: "missing key rejected", baseURL: "https://api.example.test/v1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key := "configured-test-key"
			if test.name == "missing key rejected" {
				key = ""
			}
			planner := newOpenAIAssistantPlanner(key, test.baseURL)
			if got := planner.Available(); got != test.wantOK {
				t.Fatalf("Available() = %v, want %v", got, test.wantOK)
			}
		})
	}
}

func TestAssistantRequestValidationHelpers(t *testing.T) {
	digest := sha256.Sum256([]byte("intent"))
	validHash := hex.EncodeToString(digest[:])

	if !validIntentHash(validHash) {
		t.Fatal("valid lowercase SHA-256 digest was rejected")
	}
	if validIntentHash(strings.ToUpper(validHash)) {
		t.Fatal("uppercase SHA-256 digest was accepted")
	}
	if validIntentHash("not-a-hash") {
		t.Fatal("invalid SHA-256 digest was accepted")
	}

	tests := []struct {
		name    string
		body    string
		limit   int64
		wantOK  bool
	}{
		{name: "valid strict request body", body: `{"transcript":"add milk"}`, limit: 1024, wantOK: true},
		{name: "unknown property rejected", body: `{"transcript":"add milk","intent":"send_email"}`, limit: 1024},
		{name: "trailing value rejected", body: `{"transcript":"add milk"} {}`, limit: 1024},
		{name: "oversized body rejected", body: `{"transcript":"add milk"}`, limit: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/api/ai/assistant/runs", strings.NewReader(test.body))
			var input createAssistantRunInput
			ok := decodeAssistantJSON(recorder, request, &input, test.limit)
			if ok != test.wantOK {
				t.Fatalf("decodeAssistantJSON() = %v, want %v", ok, test.wantOK)
			}
			if !test.wantOK && recorder.Code != 400 {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
		})
	}
}