package postgres

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ClaimAssistantExternalAction(
	ctx context.Context,
	userID string,
	runID string,
	expectedIntentHash string,
) (AssistantRunRecord, bool, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, false, errors.New("database is not configured")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssistantRunRecord{}, false, err
	}
	defer tx.Rollback(ctx)

	run, err := assistantRunForUpdate(ctx, tx, userID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistantRunRecord{}, false, ErrAssistantNotFound
	}
	if err != nil {
		return AssistantRunRecord{}, false, err
	}
	if subtle.ConstantTimeCompare([]byte(run.IntentSHA256), []byte(expectedIntentHash)) != 1 {
		return AssistantRunRecord{}, false, ErrAssistantConfirmation
	}
	if run.State == "executing" || run.State == "completed" || run.State == "failed" || run.State == "uncertain" {
		return run, false, nil
	}
	if run.State != "needs_confirmation" || !run.RequiresConfirmation {
		return AssistantRunRecord{}, false, ErrAssistantConfirmation
	}
	var confirmationValid bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(confirmation_expires_at > clock_timestamp(),false)
		FROM assistant_runs WHERE id=$1 AND user_id=$2`, runID, userID).Scan(&confirmationValid); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if !confirmationValid {
		return AssistantRunRecord{}, false, ErrAssistantConfirmation
	}
	var header struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(run.Intent, &header); err != nil ||
		(header.Tool != "send_gmail" && header.Tool != "create_calendar_event") {
		return AssistantRunRecord{}, false, ErrAssistantRunConflict
	}
	message := "Confirmation accepted. I’m sending the approved request to Google."
	if _, err := tx.Exec(ctx, `UPDATE assistant_runs SET
		state='executing',requires_confirmation=false,confirmation_expires_at=NULL,
		assistant_message=$3,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='needs_confirmation'`,
		runID, userID, message); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if err := insertAssistantAuditTx(ctx, tx, userID, runID, "confirmation_accepted",
		run.TranscriptSHA256, run.IntentSHA256, "", "", ""); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if err := insertAssistantAuditTx(ctx, tx, userID, runID, "tool_started",
		run.TranscriptSHA256, run.IntentSHA256, header.Tool, run.IntentSHA256, ""); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if err := insertAssistantMessageTx(ctx, tx, run.ConversationID, userID, runID, "assistant", message); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`,
		run.ConversationID, userID); err != nil {
		return AssistantRunRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AssistantRunRecord{}, false, err
	}
	run.State = "executing"
	run.RequiresConfirmation = false
	run.ConfirmationExpiresAt = nil
	run.AssistantMessage = message
	run.UpdatedAt = time.Now().UTC()
	return run, true, nil
}

func (s *Store) FinishAssistantExternalAction(
	ctx context.Context,
	userID string,
	runID string,
	state string,
	result json.RawMessage,
	message string,
) (AssistantRunRecord, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, errors.New("database is not configured")
	}
	if state != "completed" && state != "failed" && state != "uncertain" {
		return AssistantRunRecord{}, errors.New("invalid assistant external action outcome")
	}
	if strings.TrimSpace(message) == "" || len([]byte(message)) > 4096 ||
		(len(result) > 0 && (len(result) > 4096 || !json.Valid(result))) {
		return AssistantRunRecord{}, errors.New("invalid assistant external action result")
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
	if run.State != "executing" {
		switch run.State {
		case "completed", "failed", "uncertain":
			return run, nil
		default:
			return AssistantRunRecord{}, ErrAssistantRunConflict
		}
	}
	var header struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(run.Intent, &header); err != nil ||
		(header.Tool != "send_gmail" && header.Tool != "create_calendar_event") {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE assistant_runs SET
		state=$3,requires_confirmation=false,result=$4::jsonb,assistant_message=$5,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='executing'`,
		runID, userID, state, nullableJSON(result), message)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if tag.RowsAffected() != 1 {
		return AssistantRunRecord{}, ErrAssistantRunConflict
	}
	if err := insertAssistantMessageTx(ctx, tx, run.ConversationID, userID, runID, "assistant", message); err != nil {
		return AssistantRunRecord{}, err
	}
	resourceID := ""
	if len(result) > 0 {
		var summary struct {
			ResourceID string `json:"resourceId"`
		}
		if err := json.Unmarshal(result, &summary); err == nil && len(summary.ResourceID) <= 256 {
			resourceID = summary.ResourceID
		}
	}
	eventType := "provider_failed"
	if state == "completed" {
		eventType = "tool_completed"
	}
	if err := insertAssistantAuditTx(ctx, tx, userID, runID, eventType,
		run.TranscriptSHA256, run.IntentSHA256, header.Tool, run.IntentSHA256, resourceID); err != nil {
		return AssistantRunRecord{}, err
	}
	if state == "completed" {
		if err := insertAssistantAuditTx(ctx, tx, userID, runID, "result_verified",
			run.TranscriptSHA256, run.IntentSHA256, header.Tool, run.IntentSHA256, resourceID); err != nil {
			return AssistantRunRecord{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`,
		run.ConversationID, userID); err != nil {
		return AssistantRunRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AssistantRunRecord{}, err
	}
	return s.GetAssistantRun(ctx, userID, runID)
}
