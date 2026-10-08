package product

import (
	"context"
	"encoding/json"
	"testing"
)

type voiceAgentStartupEvent struct {
	eventType string
	payload   json.RawMessage
}

type voiceAgentStartupConnection struct {
	events []voiceAgentStartupEvent
}

func (c *voiceAgentStartupConnection) SendEvent(context.Context, any) error {
	return nil
}

func (c *voiceAgentStartupConnection) SendAudio(context.Context, []byte) error {
	return nil
}

func (c *voiceAgentStartupConnection) ReadProviderMessage(context.Context) (string, json.RawMessage, error) {
	if len(c.events) == 0 {
		return "", nil, errAssemblyAIProviderFailure
	}
	event := c.events[0]
	c.events = c.events[1:]
	return event.eventType, event.payload, nil
}

func (c *voiceAgentStartupConnection) Close() {}

func TestVoiceAgentStartupWaitsForReadyAfterInitialUpdateAck(t *testing.T) {
	connection := &voiceAgentStartupConnection{
		events: []voiceAgentStartupEvent{
			{
				eventType: "session.updated",
				payload:   json.RawMessage(`{"type":"session.updated","config":{}}`),
			},
			{
				eventType: "session.ready",
				payload:   json.RawMessage(`{"type":"session.ready","session_id":"session_123"}`),
			},
		},
	}

	sessionID, err := waitForVoiceAgentSessionID(context.Background(), connection)
	if err != nil {
		t.Fatalf("initial session.updated should be ignored until session.ready: %v", err)
	}
	if sessionID != "session_123" {
		t.Fatalf("unexpected session ID: %q", sessionID)
	}
	if len(connection.events) != 0 {
		t.Fatalf("startup returned before consuming session.ready: %d events remain", len(connection.events))
	}
}

func TestVoiceAgentStartupRejectsSessionError(t *testing.T) {
	connection := &voiceAgentStartupConnection{
		events: []voiceAgentStartupEvent{
			{
				eventType: "session.error",
				payload:   json.RawMessage(`{"type":"session.error","code":"agent_init_failed"}`),
			},
		},
	}

	if sessionID, err := waitForVoiceAgentSessionID(context.Background(), connection); err == nil || sessionID != "" {
		t.Fatalf("session.error should not establish a session: sessionID=%q err=%v", sessionID, err)
	}
}

func TestVoiceAgentStartupRejectsReadyWithoutValidSessionID(t *testing.T) {
	connection := &voiceAgentStartupConnection{
		events: []voiceAgentStartupEvent{
			{
				eventType: "session.ready",
				payload:   json.RawMessage(`{"type":"session.ready","session_id":""}`),
			},
		},
	}

	if sessionID, err := waitForVoiceAgentSessionID(context.Background(), connection); err == nil || sessionID != "" {
		t.Fatalf("invalid session.ready should not establish a session: sessionID=%q err=%v", sessionID, err)
	}
}
