package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type assistantActionExecutorStub struct {
	connectionID string
	accountEmail string
	resolveErr   error
}

func (s assistantActionExecutorStub) ResolveAssistantActionTarget(context.Context, string, string) (string, string, error) {
	return s.connectionID, s.accountEmail, s.resolveErr
}

func (assistantActionExecutorStub) ExecuteAssistantAction(context.Context, string, string, string, json.RawMessage) (json.RawMessage, bool, error) {
	return nil, false, errors.New("unexpected provider call in unit test")
}

func TestPrepareAssistantGmailNormalizesOneRecipientAndPreservesBody(t *testing.T) {
	prepared, err := prepareAssistantGmail(json.RawMessage(
		`{"to":"Alice Example <alice@example.com>","subject":"  Planning  ","body":"First line\nSecond line"}`,
	))
	if err != nil {
		t.Fatalf("prepare email: %v", err)
	}
	var intent assistantIntent
	if err := json.Unmarshal(prepared.Intent, &intent); err != nil {
		t.Fatalf("decode intent: %v", err)
	}
	if intent.Tool != assistantToolSendGmail ||
		intent.To != "alice@example.com" ||
		intent.Subject != "Planning" ||
		intent.Body != "First line\nSecond line" {
		t.Fatalf("prepared intent = %#v", intent)
	}
}

func TestPrepareAssistantGmailRejectsUnsafeOrAmbiguousFields(t *testing.T) {
	cases := map[string]string{
		"multiple recipients": `{"to":"one@example.com, two@example.com","subject":"Hello","body":"Text"}`,
		"header injection":    `{"to":"one@example.com","subject":"Hello\r\nBcc: other@example.com","body":"Text"}`,
		"empty body":          `{"to":"one@example.com","subject":"Hello","body":" \n "}`,
		"unknown field":       `{"to":"one@example.com","subject":"Hello","body":"Text","cc":"other@example.com"}`,
	}
	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := prepareAssistantGmail(json.RawMessage(arguments)); err == nil {
				t.Fatal("unsafe email arguments were accepted")
			}
		})
	}
}

func TestPrepareAssistantCalendarRequiresBoundedRFC3339Interval(t *testing.T) {
	start := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	end := start.Add(time.Hour)
	arguments, err := json.Marshal(map[string]string{
		"title": "Planning",
		"start": start.Format(time.RFC3339),
		"end":   end.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	prepared, err := prepareAssistantCalendarEvent(arguments)
	if err != nil {
		t.Fatalf("prepare event: %v", err)
	}
	var intent assistantIntent
	if err := json.Unmarshal(prepared.Intent, &intent); err != nil {
		t.Fatalf("decode intent: %v", err)
	}
	if intent.Tool != assistantToolCreateCalendar || intent.Title != "Planning" ||
		intent.Start != start.Format(time.RFC3339) || intent.End != end.Format(time.RFC3339) {
		t.Fatalf("prepared intent = %#v", intent)
	}

	for name, invalid := range map[string]string{
		"missing timezone": `{"title":"Planning","start":"2030-01-02T10:00:00","end":"2030-01-02T11:00:00"}`,
		"past start":       `{"title":"Planning","start":"2020-01-02T10:00:00Z","end":"2020-01-02T11:00:00Z"}`,
		"too long":         `{"title":"Planning","start":"2030-01-02T10:00:00Z","end":"2030-01-06T11:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := prepareAssistantCalendarEvent(json.RawMessage(invalid)); err == nil {
				t.Fatal("invalid calendar interval was accepted")
			}
		})
	}
}

func TestBindAssistantActionTargetHashesImmutableGoogleIntent(t *testing.T) {
	outcome := prepareAssistantPlanOutcome(assistantModelPlan{
		Intent:    assistantToolSendGmail,
		Arguments: json.RawMessage(`{"to":"person@example.com","subject":"Hello","body":"Message"}`),
	}, nil)
	executor := assistantActionExecutorStub{
		connectionID: "google-connection-1",
		accountEmail: "owner@example.com",
	}
	handler := &Handler{assistantActionExecutor: executor}
	handler.bindAssistantActionTarget(context.Background(), "user-1", &outcome)
	if outcome.State != "needs_confirmation" || !outcome.RequiresConfirmation {
		t.Fatalf("bound outcome state = %q, confirmation = %v", outcome.State, outcome.RequiresConfirmation)
	}
	var intent assistantIntent
	if err := json.Unmarshal(outcome.Intent, &intent); err != nil {
		t.Fatalf("decode bound intent: %v", err)
	}
	if intent.ConnectionID != executor.connectionID ||
		intent.AccountEmail != executor.accountEmail ||
		intent.CalendarID != "" {
		t.Fatalf("bound intent = %#v", intent)
	}
	digest := sha256.Sum256(outcome.Intent)
	if outcome.IntentSHA256 != hex.EncodeToString(digest[:]) ||
		outcome.ToolArgsSHA256 != outcome.IntentSHA256 {
		t.Fatal("bound intent hashes do not match the persisted intent")
	}
}

func TestBindAssistantActionTargetRejectsMissingConnection(t *testing.T) {
	outcome := prepareAssistantPlanOutcome(assistantModelPlan{
		Intent:    assistantToolCreateCalendar,
		Arguments: json.RawMessage(`{"title":"Planning","start":"2030-01-02T10:00:00Z","end":"2030-01-02T11:00:00Z"}`),
	}, nil)
	handler := &Handler{assistantActionExecutor: assistantActionExecutorStub{
		resolveErr: errors.New("connection missing"),
	}}
	handler.bindAssistantActionTarget(context.Background(), "user-1", &outcome)
	if outcome.State != "rejected" || outcome.RequiresConfirmation || len(outcome.Intent) != 0 {
		t.Fatalf("missing connection outcome = %#v", outcome)
	}
}

func TestPlannerPromptRequiresExplicitGoogleWriteDetails(t *testing.T) {
	prompt, err := buildAssistantPlannerSystemPrompt(newAssistantToolRegistry())
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	for _, required := range []string{"never invent", "explicit subject", "UTC offsets"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("planner prompt does not include %q", required)
		}
	}
}
