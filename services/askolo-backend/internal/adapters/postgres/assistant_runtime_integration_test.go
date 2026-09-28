package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func prepareAssistantConfirmation(
	t *testing.T,
	ctx context.Context,
	store *Store,
	userID, idempotencyKey, transcript, title string,
	expiresAt time.Time,
) (AssistantRunRecord, string) {
	t.Helper()

	policy, err := store.AICreditPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.OperationWeights["assistant"] < 1 {
		t.Fatal("assistant credit weight is not configured")
	}

	run, _, created, err := store.StartAssistantRun(
		ctx, userID, DefaultWorkspaceID(userID), "", idempotencyKey, transcript, policy.Version,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a new assistant run")
	}
	started, err := store.MarkAssistantProviderStarted(ctx, userID, run.ID)
	if err != nil || !started {
		t.Fatalf("provider claim started=%v err=%v", started, err)
	}

	intent, err := json.Marshal(assistantActionIntent{Tool: "create_action_item", Title: title})
	if err != nil {
		t.Fatal(err)
	}
	intentDigest := sha256.Sum256(intent)
	intentHash := hex.EncodeToString(intentDigest[:])
	outcome := AssistantPlanOutcome{
		State: "needs_confirmation", Intent: intent, IntentSHA256: intentHash,
		RiskLevel: "write", RequiresConfirmation: true, ConfirmationExpires: &expiresAt,
		Message: "Please confirm this action item.", AuditEvent: "plan_ready",
		ToolName: "create_action_item", ToolArgsSHA256: intentHash,
		Settle: true, SettledCredits: run.BaseCredits,
	}
	finished, err := store.FinishAssistantPlanning(ctx, userID, run.ID, outcome)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != "needs_confirmation" || finished.IntentSHA256 != intentHash {
		t.Fatalf("unexpected pending run: %#v", finished)
	}
	return finished, intentHash
}

func TestAssistantRunConfirmationAndCancellation(t *testing.T) {
	ctx, pool, store := openTAR10(t)
	userID := fmt.Sprintf("assistant-user-%d", time.Now().UnixNano())
	tar10User(t, ctx, pool, userID)
	tar10Account(t, ctx, pool, userID, 20)
	if err := store.EnsurePersonalWorkspace(ctx, userID); err != nil {
		t.Fatal(err)
	}

	run, intentHash := prepareAssistantConfirmation(
		t, ctx, store, userID, "assistant-confirm-key",
		"Add buy milk to my action items", "Buy milk", time.Now().UTC().Add(5*time.Minute),
	)
	if _, _, created, err := store.StartAssistantRun(
		ctx, userID, DefaultWorkspaceID(userID), "", "assistant-confirm-key",
		"Add buy milk to my action items", run.PolicyVersion,
	); err != nil || created {
		t.Fatalf("same-key replay created=%v err=%v", created, err)
	}
	if _, _, _, err := store.StartAssistantRun(
		ctx, userID, DefaultWorkspaceID(userID), "", "assistant-confirm-key",
		"Add a different item", run.PolicyVersion,
	); !errors.Is(err, ErrAssistantIdempotencyConflict) {
		t.Fatalf("changed-content replay error = %v, want idempotency conflict", err)
	}
	if _, err := store.ConfirmAssistantActionItem(ctx, userID, run.ID, fmt.Sprintf("%064x", 1)); !errors.Is(err, ErrAssistantConfirmation) {
		t.Fatalf("incorrect intent hash error = %v, want confirmation error", err)
	}

	completed, err := store.ConfirmAssistantActionItem(ctx, userID, run.ID, intentHash)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != "completed" || completed.Result == nil {
		t.Fatalf("confirmed run = %#v", completed)
	}
	if _, err := store.ConfirmAssistantActionItem(ctx, userID, run.ID, intentHash); err != nil {
		t.Fatalf("same confirmation replay: %v", err)
	}

	var itemCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM action_items
		WHERE user_id=$1 AND title='Buy milk' AND source_type='assistant'`, userID).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 1 {
		t.Fatalf("action item count = %d, want 1", itemCount)
	}

	conversation, err := store.GetAssistantConversation(ctx, userID, DefaultWorkspaceID(userID))
	if err != nil {
		t.Fatal(err)
	}
	if len(conversation.Messages) != 3 {
		t.Fatalf("message count = %d, want user, planned action, and confirmed result", len(conversation.Messages))
	}
	if conversation.Messages[0].Role != "user" || conversation.Messages[1].Role != "assistant" || conversation.Messages[2].Role != "assistant" {
		t.Fatalf("conversation messages are not chronological: %#v", conversation.Messages)
	}
	if conversation.Messages[1].ConfirmationExpiresAt == nil {
		t.Fatal("pending confirmation expiry was not persisted to conversation history")
	}

	expired, expiredHash := prepareAssistantConfirmation(
		t, ctx, store, userID, "assistant-expired-key",
		"Add expired item", "Expired item", time.Now().UTC().Add(-time.Minute),
	)
	if _, err := store.ConfirmAssistantActionItem(ctx, userID, expired.ID, expiredHash); !errors.Is(err, ErrAssistantConfirmation) {
		t.Fatalf("expired confirmation error = %v, want confirmation error", err)
	}
	cancelled, cancelHash := prepareAssistantConfirmation(
		t, ctx, store, userID, "assistant-cancel-key",
		"Add later item", "Later item", time.Now().UTC().Add(5*time.Minute),
	)
	cancelled, err = store.CancelAssistantConfirmation(ctx, userID, cancelled.ID, cancelHash)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "cancelled" {
		t.Fatalf("cancelled run state = %q, want cancelled", cancelled.State)
	}
	if _, err := store.CancelAssistantConfirmation(ctx, userID, cancelled.ID, cancelHash); err != nil {
		t.Fatalf("same cancellation replay: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM action_items
		WHERE user_id=$1 AND title IN ('Expired item','Later item') AND source_type='assistant'`, userID).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 0 {
		t.Fatalf("expired/cancelled action item count = %d, want 0", itemCount)
	}
}
