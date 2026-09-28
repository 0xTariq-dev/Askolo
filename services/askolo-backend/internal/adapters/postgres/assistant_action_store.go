package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

type AssistantActionItemResult struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

func (s *Store) ConfirmAssistantActionItem(ctx context.Context, userID, runID, expectedIntentHash string) (AssistantRunRecord, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	defer tx.Rollback(ctx)

	run, err := assistantRunForUpdate(ctx, tx, userID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistantRunRecord{}, ErrAssistantNotFound
	}
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if subtle.ConstantTimeCompare([]byte(run.IntentSHA256), []byte(expectedIntentHash)) != 1 {
		return AssistantRunRecord{}, ErrAssistantConfirmation
	}
	if run.State == "completed" {
		return run, nil
	}
	if run.State != "needs_confirmation" || !run.RequiresConfirmation {
		return AssistantRunRecord{}, ErrAssistantConfirmation
	}
	var confirmationValid bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(confirmation_expires_at > clock_timestamp(),false)
		FROM assistant_runs WHERE id=$1 AND user_id=$2`, runID, userID).Scan(&confirmationValid); err != nil {
		return AssistantRunRecord{}, err
	}
	if !confirmationValid {
		return AssistantRunRecord{}, ErrAssistantConfirmation
	}
	var intent assistantActionIntent
	if err = json.Unmarshal(run.Intent, &intent); err != nil {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}
	intent.Tool = strings.TrimSpace(intent.Tool)
	intent.Title = strings.TrimSpace(intent.Title)
	if intent.Tool != "create_action_item" || intent.Title == "" || len([]byte(intent.Title)) > 120 {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}

	var workspaceID string
	if err = tx.QueryRow(ctx, `SELECT workspace_id FROM assistant_conversations
		WHERE id=$1 AND user_id=$2`, run.ConversationID, userID).Scan(&workspaceID); err != nil {
		return AssistantRunRecord{}, err
	}
	actionArgsHash, err := assistantActionArgsHash(intent)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "confirmation_accepted",
		run.TranscriptSHA256, run.IntentSHA256, "", "", ""); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "tool_started",
		run.TranscriptSHA256, run.IntentSHA256, intent.Tool, actionArgsHash, ""); err != nil {
		return AssistantRunRecord{}, err
	}
	var item AssistantActionItemResult
	if err = tx.QueryRow(ctx, `INSERT INTO action_items(user_id,title,source_type,completed)
		VALUES($1,$2,'assistant',false) RETURNING id,title,completed`, userID, intent.Title).
		Scan(&item.ID, &item.Title, &item.Completed); err != nil {
		return AssistantRunRecord{}, err
	}

	resourceID, err := id.New()
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO authorization_resources
		(id,workspace_id,resource_type,resource_id,owner_user_id,status)
		VALUES($1,$2,'actionItem',$3,$4,'active')
		ON CONFLICT (workspace_id,resource_type,resource_id) DO UPDATE SET
			owner_user_id=EXCLUDED.owner_user_id,status=EXCLUDED.status,updated_at=NOW()`,
		resourceID, workspaceID, fmt.Sprint(item.ID), userID); err != nil {
		return AssistantRunRecord{}, err
	}

	var verifiedUserID, verifiedTitle, sourceType string
	var verifiedCompleted bool
	if err = tx.QueryRow(ctx, `SELECT user_id,title,COALESCE(source_type,''),completed
		FROM action_items WHERE id=$1 AND user_id=$2`, item.ID, userID).
		Scan(&verifiedUserID, &verifiedTitle, &sourceType, &verifiedCompleted); err != nil {
		return AssistantRunRecord{}, err
	}
	if verifiedUserID != userID || verifiedTitle != intent.Title || sourceType != "assistant" || verifiedCompleted {
		return AssistantRunRecord{}, errors.New("assistant action result verification failed")
	}
	item.Title = verifiedTitle
	item.Completed = verifiedCompleted
	resultJSON, err := json.Marshal(item)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	message := assistantActionItemMessage(item.Title)
	tag, err := tx.Exec(ctx, `UPDATE assistant_runs SET
		state='completed',requires_confirmation=false,result=$3::jsonb,assistant_message=$4,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='needs_confirmation' AND confirmation_expires_at>clock_timestamp()`,
		runID, userID, resultJSON, message)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if tag.RowsAffected() != 1 {
		return AssistantRunRecord{}, ErrAssistantConfirmation
	}
	if err = insertAssistantMessageTx(ctx, tx, run.ConversationID, userID, runID, "assistant", message); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "tool_completed",
		run.TranscriptSHA256, run.IntentSHA256, intent.Tool, actionArgsHash, ""); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "result_verified",
		run.TranscriptSHA256, run.IntentSHA256, intent.Tool, actionArgsHash, fmt.Sprint(item.ID)); err != nil {
		return AssistantRunRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`,
		run.ConversationID, userID); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AssistantRunRecord{}, err
	}
	return s.GetAssistantRun(ctx, userID, runID)
}

func (s *Store) CancelAssistantConfirmation(ctx context.Context, userID, runID, expectedIntentHash string) (AssistantRunRecord, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	defer tx.Rollback(ctx)
	run, err := assistantRunForUpdate(ctx, tx, userID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistantRunRecord{}, ErrAssistantNotFound
	}
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if subtle.ConstantTimeCompare([]byte(run.IntentSHA256), []byte(expectedIntentHash)) != 1 {
		return AssistantRunRecord{}, ErrAssistantConfirmation
	}
	if run.State == "cancelled" {
		return run, nil
	}
	if run.State != "needs_confirmation" || !run.RequiresConfirmation {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}
	message := "I discarded that pending action. Nothing was changed."
	tag, err := tx.Exec(ctx, `UPDATE assistant_runs SET
		state='cancelled',requires_confirmation=false,assistant_message=$3,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='needs_confirmation'`,
		runID, userID, message)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if tag.RowsAffected() != 1 {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}
	if err = insertAssistantMessageTx(ctx, tx, run.ConversationID, userID, runID, "assistant", message); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "confirmation_cancelled",
		run.TranscriptSHA256, run.IntentSHA256, "", "", ""); err != nil {
		return AssistantRunRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`,
		run.ConversationID, userID); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AssistantRunRecord{}, err
	}
	return s.GetAssistantRun(ctx, userID, runID)
}
