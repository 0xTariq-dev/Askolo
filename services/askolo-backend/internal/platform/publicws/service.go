package publicws

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return "public WebSocket operation failed"
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func Failure(status int, code, message string, cause error) *Error {
	return &Error{Status: status, Code: code, Message: message, Cause: cause}
}

type AssistantRequest struct {
	ConversationID string
	Transcript     string
	IdempotencyKey string
	PolicyVersion  int
}

type Services interface {
	RunAssistant(
		context.Context,
		string,
		string,
		AssistantRequest,
		func(runID string, payload json.RawMessage) error,
	) (json.RawMessage, error)
	StartVoice(
		context.Context,
		string,
		string,
		string,
		int,
	) (VoiceSession, json.RawMessage, error)
}

type VoiceAgentRequest struct {
	ConversationID         string
	Locale                 string
	IdempotencyKey         string
	PolicyVersion          int
	AssistantPolicyVersion int
}

// VoiceAgentServices is optional so deployments without the managed agent
// provider can continue to serve the existing transcription protocol.
type VoiceAgentServices interface {
	StartVoiceAgent(context.Context, string, string, VoiceAgentRequest) (VoiceAgentSession, json.RawMessage, error)
}

type VoiceSession interface {
	SendPCMFrame(context.Context, []byte) error
	ReadProviderMessage(context.Context) (string, json.RawMessage, error)
	SendTermination(context.Context) error
	MaxDuration() time.Duration
	Close()
}

type VoiceAgentSession interface {
	SendAudio(context.Context, []byte) error
	ReadProviderMessage(context.Context) (string, json.RawMessage, error)
	SendTermination(context.Context) error
	MaxDuration() time.Duration
	Close() VoiceAgentOutcome
}

type VoiceAgentOutcome struct {
	DeletionStatus string          `json:"deletionStatus"`
	CreditReceipt  json.RawMessage `json:"creditReceipt,omitempty"`
}

var ErrUnavailable = errors.New("public WebSocket service unavailable")
