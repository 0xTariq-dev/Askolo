package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

var (
	ErrAssistantNotFound            = errors.New("assistant run not found")
	ErrAssistantRunConflict         = errors.New("assistant run cannot be changed")
	ErrAssistantRateLimited         = errors.New("assistant rate limit exceeded")
	ErrAssistantInsufficient        = errors.New("insufficient AI credits")
	ErrAssistantPolicyChanged       = errors.New("AI credit policy changed")
	ErrAssistantPolicyInvalid       = errors.New("AI credit policy is invalid")
	ErrAssistantConfirmation        = errors.New("assistant confirmation is invalid or expired")
	ErrAssistantCreditConflict      = errors.New("assistant credit reservation conflict")
	ErrAssistantIdempotencyConflict = errors.New("assistant idempotency key does not match the original request")
)

type AssistantRunRecord struct {
	ID                    string          `json:"id"`
	ConversationID        string          `json:"conversationId"`
	UserID                string          `json:"-"`
	State                 string          `json:"state"`
	TranscriptSHA256      string          `json:"transcriptSha256"`
	ReservationID         string          `json:"reservationId"`
	BaseCredits           int             `json:"baseCredits"`
	ReservedCredits       int             `json:"reservedCredits"`
	SettledCredits        int             `json:"settledCredits"`
	PolicyVersion         int             `json:"policyVersion"`
	ProviderStarted       bool            `json:"-"`
	Intent                json.RawMessage `json:"intent,omitempty"`
	IntentSHA256          string          `json:"intentSha256,omitempty"`
	RiskLevel             string          `json:"riskLevel,omitempty"`
	RequiresConfirmation  bool            `json:"requiresConfirmation"`
	ConfirmationExpiresAt *time.Time      `json:"confirmationExpiresAt,omitempty"`
	Result                json.RawMessage `json:"result,omitempty"`
	AssistantMessage      string          `json:"message,omitempty"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type AssistantMessageRecord struct {
	ID                    string          `json:"id"`
	Role                  string          `json:"role"`
	Content               string          `json:"content"`
	RunID                 string          `json:"runId,omitempty"`
	State                 string          `json:"state,omitempty"`
	Intent                json.RawMessage `json:"intent,omitempty"`
	IntentSHA256          string          `json:"intentSha256,omitempty"`
	RequiresConfirmation  bool            `json:"requiresConfirmation,omitempty"`
	ConfirmationExpiresAt *time.Time      `json:"confirmationExpiresAt,omitempty"`
	Result                json.RawMessage `json:"result,omitempty"`
}

type AssistantConversationRecord struct {
	ID       string                   `json:"conversationId,omitempty"`
	Messages []AssistantMessageRecord `json:"messages"`
}

type AssistantPlanOutcome struct {
	State                string
	Intent               json.RawMessage
	IntentSHA256         string
	RiskLevel            string
	RequiresConfirmation bool
	ConfirmationExpires  *time.Time
	Message              string
	AuditEvent           string
	ToolName             string
	ToolArgsSHA256       string
	Settle               bool
	SettledCredits       int
}

type assistantActionIntent struct {
	Tool  string `json:"tool"`
	Title string `json:"title"`
}

type assistantRow interface {
	Scan(dest ...any) error
}

func (s *Store) StartAssistantRun(
	ctx context.Context,
	userID, workspaceID, conversationID, idempotencyKey, transcript string,
	expectedPolicyVersion int,
) (AssistantRunRecord, AICreditReservation, bool, error) {
	var emptyRun AssistantRunRecord
	var emptyReservation AICreditReservation
	if s == nil || s.pool == nil {
		return emptyRun, emptyReservation, false, errors.New("database is not configured")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 200 ||
		len([]byte(transcript)) < 1 || len([]byte(transcript)) > 4096 ||
		expectedPolicyVersion < 1 {
		return emptyRun, emptyReservation, false, errors.New("invalid assistant run")
	}

	keyDigest := sha256.Sum256([]byte(idempotencyKey))
	keyHash := hex.EncodeToString(keyDigest[:])
	creditKey := "assistant:" + keyHash
	transcriptDigest := sha256.Sum256([]byte(transcript))
	transcriptHash := hex.EncodeToString(transcriptDigest[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	defer tx.Rollback(ctx)

	// Serialize starts for one account. This makes the per-user rate limit and
	// idempotency check deterministic across API processes.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	existing, err := assistantRunByKeyTx(ctx, tx, userID, keyHash)
	if err == nil {
		if existing.TranscriptSHA256 != transcriptHash ||
			(conversationID != "" && existing.ConversationID != conversationID) {
			return emptyRun, emptyReservation, false, ErrAssistantIdempotencyConflict
		}
		return existing, emptyReservation, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return emptyRun, emptyReservation, false, err
	}

	var recentRuns int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM assistant_runs WHERE user_id=$1 AND created_at > NOW()-INTERVAL '1 minute'`, userID).Scan(&recentRuns); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if recentRuns >= 5 {
		return emptyRun, emptyReservation, false, ErrAssistantRateLimited
	}

	var policyVersion, operationWeight, margin int
	var rawWeights []byte
	err = tx.QueryRow(ctx, `SELECT version,operation_weights,overrun_margin_percent
		FROM ai_credit_policy_versions ORDER BY version DESC LIMIT 1`).
		Scan(&policyVersion, &rawWeights, &margin)
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if policyVersion != expectedPolicyVersion {
		return emptyRun, emptyReservation, false, ErrAssistantPolicyChanged
	}
	var weights map[string]int
	if err = json.Unmarshal(rawWeights, &weights); err != nil {
		return emptyRun, emptyReservation, false, ErrAssistantPolicyInvalid
	}
	operationWeight = weights["assistant"]
	if operationWeight < 1 || operationWeight > 100000 || margin < 0 || margin > 100 {
		return emptyRun, emptyReservation, false, ErrAssistantPolicyInvalid
	}
	reservedCredits := operationWeight * (100 + margin) / 100
	if operationWeight*(100+margin)%100 != 0 {
		reservedCredits++
	}
	if reservedCredits < 1 || reservedCredits > 100000 {
		return emptyRun, emptyReservation, false, ErrAssistantPolicyInvalid
	}

	if conversationID == "" {
		conversationID, err = id.New()
		if err != nil {
			return emptyRun, emptyReservation, false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO assistant_conversations(id,user_id,workspace_id,title)
			VALUES($1,$2,$3,'Assistant')`, conversationID, userID, workspaceID); err != nil {
			return emptyRun, emptyReservation, false, err
		}
	} else {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM assistant_conversations
			WHERE id=$1 AND user_id=$2 AND workspace_id=$3
		)`, conversationID, userID, workspaceID).Scan(&exists); err != nil {
			return emptyRun, emptyReservation, false, err
		}
		if !exists {
			return emptyRun, emptyReservation, false, ErrAssistantNotFound
		}
	}

	runID, err := id.New()
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	reservationID, err := id.New()
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	reservation, created, err := reserveAssistantCreditTx(
		ctx, tx, reservationID, userID, creditKey, reservedCredits, policyVersion,
	)
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if !created {
		if reservation.ID == "" {
			return emptyRun, emptyReservation, false, ErrAssistantInsufficient
		}
		return emptyRun, emptyReservation, false, ErrAssistantCreditConflict
	}

	claimed, err := tx.Exec(ctx, `UPDATE ai_credit_reservations
		SET status='claimed',started_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND status='reserved' AND expires_at>NOW()`,
		reservation.ID, userID)
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if claimed.RowsAffected() != 1 {
		return emptyRun, emptyReservation, false, ErrAssistantCreditConflict
	}
	reservation.Status = "claimed"

	if _, err = tx.Exec(ctx, `INSERT INTO assistant_runs(
		id,conversation_id,user_id,idempotency_key_hash,transcript,transcript_sha256,state,
		reservation_id,base_credits,reserved_credits,policy_version
	) VALUES($1,$2,$3,$4,$5,$6,'planning',$7,$8,$9,$10)`,
		runID, conversationID, userID, keyHash, transcript, transcriptHash,
		reservation.ID, operationWeight, reservation.ReservedCredits, policyVersion,
	); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if err = insertAssistantMessageTx(ctx, tx, conversationID, userID, runID, "user", transcript); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, "run_started", transcriptHash, "", "", "", ""); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`, conversationID, userID); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return emptyRun, emptyReservation, false, err
	}
	run, err := s.GetAssistantRun(ctx, userID, runID)
	if err != nil {
		return emptyRun, emptyReservation, false, err
	}
	return run, reservation, true, nil
}

func reserveAssistantCreditTx(
	ctx context.Context,
	tx pgx.Tx,
	id, userID, key string,
	credits, policyVersion int,
) (AICreditReservation, bool, error) {
	var result AICreditReservation
	if id == "" || userID == "" || key == "" || credits < 1 || credits > 100000 || policyVersion < 1 {
		return result, false, errors.New("invalid assistant credit reservation")
	}
	var prior AICreditReservation
	err := tx.QueryRow(ctx, `SELECT id,status,reserved_credits,settled_credits,refunded_credits,policy_version,expires_at
		FROM ai_credit_reservations WHERE user_id=$1 AND idempotency_key=$2`, userID, key).
		Scan(&prior.ID, &prior.Status, &prior.ReservedCredits, &prior.SettledCredits, &prior.RefundedCredits, &prior.PolicyVersion, &prior.ExpiresAt)
	if err == nil {
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM ai_credit_reservations
		WHERE user_id=$1 AND status='reserved' AND expires_at<=NOW() FOR UPDATE`, userID); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `WITH expired AS (
		UPDATE ai_credit_reservations SET status='expired',closed_at=NOW(),updated_at=NOW()
		WHERE user_id=$1 AND status='reserved' AND expires_at<=NOW()
		RETURNING id,user_id,reserved_credits
	), total AS (
		SELECT COALESCE(SUM(reserved_credits),0) credits FROM expired
	), account AS (
		UPDATE ai_credit_accounts SET reserved_credits=reserved_credits-(SELECT credits FROM total),updated_at=NOW()
		WHERE user_id=$1 RETURNING user_id
	)
	INSERT INTO ai_credit_reservation_events(reservation_id,user_id,event_type,credits,idempotency_key)
	SELECT id,user_id,'expired',reserved_credits,id||':expired' FROM expired
	ON CONFLICT (reservation_id,idempotency_key) DO NOTHING`, userID); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		return result, false, err
	}
	var balance int
	if err = tx.QueryRow(ctx, `SELECT granted_credits+adjustment_credits-reserved_credits-spent_credits+refunded_credits
		FROM ai_credit_accounts WHERE user_id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return result, false, err
	}
	// Keep this second idempotency lookup after the account lock. Another run
	// with the same key may have passed the first lookup before serialization.
	err = tx.QueryRow(ctx, `SELECT id,status,reserved_credits,settled_credits,refunded_credits,policy_version,expires_at
		FROM ai_credit_reservations WHERE user_id=$1 AND idempotency_key=$2`, userID, key).
		Scan(&prior.ID, &prior.Status, &prior.ReservedCredits, &prior.SettledCredits, &prior.RefundedCredits, &prior.PolicyVersion, &prior.ExpiresAt)
	if err == nil {
		return prior, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, false, err
	}
	if balance < credits {
		return result, false, nil
	}
	expires := time.Now().UTC().Add(5 * time.Minute)
	accountUpdate, err := tx.Exec(ctx, `UPDATE ai_credit_accounts SET reserved_credits=reserved_credits+$2,updated_at=NOW()
		WHERE user_id=$1`, userID, credits)
	if err != nil {
		return result, false, err
	}
	if accountUpdate.RowsAffected() != 1 {
		return result, false, ErrAssistantCreditConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservations(
		id,user_id,operation_type,provider,mode,status,idempotency_key,reserved_credits,max_credits,expires_at,policy_version
	) VALUES($1,$2,'assistant','openai','agent','reserved',$3,$4,$4,$5,$6)`,
		id, userID, key, credits, expires, policyVersion); err != nil {
		return result, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(
		reservation_id,user_id,event_type,credits,idempotency_key
	) VALUES($1,$2,'reserved',$3,$4)`, id, userID, credits, key+":reserved"); err != nil {
		return result, false, err
	}
	return AICreditReservation{
		ID: id, Status: "reserved", ReservedCredits: credits, PolicyVersion: policyVersion, ExpiresAt: &expires,
	}, true, nil
}

func (s *Store) MarkAssistantProviderStarted(ctx context.Context, userID, runID string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database is not configured")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE assistant_runs
		SET provider_started=true,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='planning' AND provider_started=false`,
		runID, userID)
	return tag.RowsAffected() == 1, err
}

func (s *Store) FinishAssistantPlanning(ctx context.Context, userID, runID string, outcome AssistantPlanOutcome) (AssistantRunRecord, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, errors.New("database is not configured")
	}
	switch outcome.State {
	case "needs_confirmation", "completed", "cancelled", "failed", "rejected":
	default:
		return AssistantRunRecord{}, errors.New("invalid assistant run outcome")
	}
	if strings.TrimSpace(outcome.Message) == "" || len([]byte(outcome.Message)) > 4096 {
		return AssistantRunRecord{}, errors.New("invalid assistant response")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	defer tx.Rollback(ctx)
	run, err := assistantRunForUpdate(ctx, tx, userID, runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssistantRunRecord{}, ErrAssistantNotFound
		}
		return AssistantRunRecord{}, err
	}
	if run.State != "planning" {
		return run, nil
	}
	settled := 0
	if outcome.Settle {
		if outcome.SettledCredits < 0 || outcome.SettledCredits > run.ReservedCredits {
			return AssistantRunRecord{}, errors.New("invalid assistant settlement")
		}
		if err = settleAssistantCreditTx(ctx, tx, run.ReservationID, userID, run.ReservationID+":settle", outcome.SettledCredits); err != nil {
			return AssistantRunRecord{}, err
		}
		settled = outcome.SettledCredits
	} else if err = releaseAssistantCreditTx(ctx, tx, run.ReservationID, userID, run.ReservationID+":release"); err != nil {
		return AssistantRunRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE assistant_runs SET
		state=$3,intent=$4::jsonb,intent_sha256=NULLIF($5,''),risk_level=NULLIF($6,''),
		requires_confirmation=$7,confirmation_expires_at=$8,settled_credits=$9,
		assistant_message=$10,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND state='planning'`,
		runID, userID, outcome.State, nullableJSON(outcome.Intent), outcome.IntentSHA256,
		outcome.RiskLevel, outcome.RequiresConfirmation, outcome.ConfirmationExpires, settled, outcome.Message,
	); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = insertAssistantMessageTx(ctx, tx, run.ConversationID, userID, runID, "assistant", outcome.Message); err != nil {
		return AssistantRunRecord{}, err
	}
	eventName := outcome.AuditEvent
	if eventName == "" {
		eventName = "plan_ready"
	}
	if err = insertAssistantAuditTx(ctx, tx, userID, runID, eventName,
		run.TranscriptSHA256, outcome.IntentSHA256, outcome.ToolName, outcome.ToolArgsSHA256, ""); err != nil {
		return AssistantRunRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE assistant_conversations SET updated_at=NOW() WHERE id=$1 AND user_id=$2`, run.ConversationID, userID); err != nil {
		return AssistantRunRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AssistantRunRecord{}, err
	}
	return s.GetAssistantRun(ctx, userID, runID)
}

func settleAssistantCreditTx(ctx context.Context, tx pgx.Tx, id, userID, key string, actual int) error {
	if actual < 0 {
		return errors.New("invalid assistant settlement")
	}
	var reserved int
	var state string
	if err := tx.QueryRow(ctx, `SELECT reserved_credits,status FROM ai_credit_reservations
		WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "settled" {
		var credits int
		err := tx.QueryRow(ctx, `SELECT credits FROM ai_credit_reservation_events
			WHERE reservation_id=$1 AND event_type='settled' AND idempotency_key=$2`, id, key).Scan(&credits)
		if err == nil && credits == actual {
			return nil
		}
		return errors.New("assistant settlement idempotency conflict")
	}
	if state != "claimed" || actual > reserved {
		return errors.New("invalid assistant reservation state")
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM ai_credit_reservations
		WHERE user_id=$1 AND status='reserved' AND expires_at<=NOW() FOR UPDATE`, userID); err != nil {
		return err
	}
	accountUpdate, err := tx.Exec(ctx, `UPDATE ai_credit_accounts
		SET reserved_credits=reserved_credits-$2,spent_credits=spent_credits+$3,updated_at=NOW()
		WHERE user_id=$1 AND reserved_credits >= $2`, userID, reserved, actual)
	if err != nil {
		return err
	}
	if accountUpdate.RowsAffected() != 1 {
		return ErrAssistantCreditConflict
	}
	reservationUpdate, err := tx.Exec(ctx, `UPDATE ai_credit_reservations
		SET status='settled',settled_credits=$2,refunded_credits=reserved_credits-$2,closed_at=NOW(),updated_at=NOW()
		WHERE id=$1`, id, actual)
	if err != nil {
		return err
	}
	if reservationUpdate.RowsAffected() != 1 {
		return ErrAssistantCreditConflict
	}
	eventInsert, err := tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(
		reservation_id,user_id,event_type,credits,idempotency_key
	) VALUES($1,$2,'settled',$3,$4) ON CONFLICT DO NOTHING`, id, userID, actual, key)
	if err != nil {
		return err
	}
	if eventInsert.RowsAffected() != 1 {
		return errors.New("assistant settlement event conflict")
	}
	return nil
}

func releaseAssistantCreditTx(ctx context.Context, tx pgx.Tx, id, userID, key string) error {
	var reserved int
	var state string
	if err := tx.QueryRow(ctx, `SELECT reserved_credits,status FROM ai_credit_reservations
		WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&reserved, &state); err != nil {
		return err
	}
	if state == "released" {
		var credits int
		err := tx.QueryRow(ctx, `SELECT credits FROM ai_credit_reservation_events
			WHERE reservation_id=$1 AND event_type='released' AND idempotency_key=$2`, id, key).Scan(&credits)
		if err == nil && credits == reserved {
			return nil
		}
		return errors.New("assistant release idempotency conflict")
	}
	if state != "claimed" && state != "reserved" {
		return errors.New("invalid assistant reservation state")
	}
	accountUpdate, err := tx.Exec(ctx, `UPDATE ai_credit_accounts
		SET reserved_credits=reserved_credits-$2,updated_at=NOW()
		WHERE user_id=$1 AND reserved_credits >= $2`, userID, reserved)
	if err != nil {
		return err
	}
	if accountUpdate.RowsAffected() != 1 {
		return ErrAssistantCreditConflict
	}
	reservationUpdate, err := tx.Exec(ctx, `UPDATE ai_credit_reservations
		SET status='released',refunded_credits=reserved_credits,closed_at=NOW(),updated_at=NOW()
		WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if reservationUpdate.RowsAffected() != 1 {
		return ErrAssistantCreditConflict
	}
	eventInsert, err := tx.Exec(ctx, `INSERT INTO ai_credit_reservation_events(
		reservation_id,user_id,event_type,credits,idempotency_key
	) VALUES($1,$2,'released',$3,$4) ON CONFLICT DO NOTHING`, id, userID, reserved, key)
	if err != nil {
		return err
	}
	if eventInsert.RowsAffected() != 1 {
		return errors.New("assistant release event conflict")
	}
	return nil
}

func (s *Store) GetAssistantRun(ctx context.Context, userID, runID string) (AssistantRunRecord, error) {
	if s == nil || s.pool == nil {
		return AssistantRunRecord{}, errors.New("database is not configured")
	}
	run, err := scanAssistantRun(s.pool.QueryRow(ctx, assistantRunSelect+` WHERE user_id=$1 AND id=$2`, userID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AssistantRunRecord{}, ErrAssistantNotFound
	}
	return run, err
}

func (s *Store) GetAssistantConversation(ctx context.Context, userID, workspaceID string) (AssistantConversationRecord, error) {
	result := AssistantConversationRecord{Messages: []AssistantMessageRecord{}}
	if s == nil || s.pool == nil {
		return result, errors.New("database is not configured")
	}
	err := s.pool.QueryRow(ctx, `SELECT id FROM assistant_conversations
		WHERE user_id=$1 AND workspace_id=$2 ORDER BY updated_at DESC,id DESC LIMIT 1`,
		userID, workspaceID).Scan(&result.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	rows, err := s.pool.Query(ctx, `SELECT
		m.id,m.role,m.content,COALESCE(m.run_id,''),COALESCE(r.state,''),
		COALESCE(r.intent::text,''),COALESCE(r.intent_sha256,''),
		COALESCE(r.requires_confirmation,false),r.confirmation_expires_at,COALESCE(r.result::text,'')
		FROM assistant_messages m
		LEFT JOIN assistant_runs r ON r.id=m.run_id AND r.user_id=m.user_id
		WHERE m.conversation_id=$1 AND m.user_id=$2
		ORDER BY m.created_at DESC,m.id DESC LIMIT 50`, result.ID, userID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var message AssistantMessageRecord
		var intent, actionResult string
		var expires sql.NullTime
		if err := rows.Scan(&message.ID, &message.Role, &message.Content, &message.RunID, &message.State,
			&intent, &message.IntentSHA256, &message.RequiresConfirmation, &expires, &actionResult); err != nil {
			return result, err
		}
		if intent != "" {
			message.Intent = json.RawMessage(intent)
		}
		if actionResult != "" {
			message.Result = json.RawMessage(actionResult)
		}
		if expires.Valid {
			message.ConfirmationExpiresAt = &expires.Time
		}
		result.Messages = append(result.Messages, message)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	for left, right := 0, len(result.Messages)-1; left < right; left, right = left+1, right-1 {
		result.Messages[left], result.Messages[right] = result.Messages[right], result.Messages[left]
	}
	return result, nil
}

const assistantRunSelect = `SELECT
	id,conversation_id,user_id,transcript_sha256,state,reservation_id,base_credits,
	reserved_credits,settled_credits,policy_version,provider_started,
	COALESCE(intent::text,''),COALESCE(intent_sha256,''),COALESCE(risk_level,''),
	requires_confirmation,confirmation_expires_at,COALESCE(result::text,''),
	COALESCE(assistant_message,''),created_at,updated_at
	FROM assistant_runs`

func assistantRunByKeyTx(ctx context.Context, tx pgx.Tx, userID, keyHash string) (AssistantRunRecord, error) {
	return scanAssistantRun(tx.QueryRow(ctx, assistantRunSelect+` WHERE user_id=$1 AND idempotency_key_hash=$2`, userID, keyHash))
}

func assistantRunForUpdate(ctx context.Context, tx pgx.Tx, userID, runID string) (AssistantRunRecord, error) {
	return scanAssistantRun(tx.QueryRow(ctx, assistantRunSelect+` WHERE user_id=$1 AND id=$2 FOR UPDATE`, userID, runID))
}

func scanAssistantRun(row assistantRow) (AssistantRunRecord, error) {
	var run AssistantRunRecord
	var intent, result, intentHash, riskLevel, message string
	var expires sql.NullTime
	err := row.Scan(&run.ID, &run.ConversationID, &run.UserID, &run.TranscriptSHA256, &run.State,
		&run.ReservationID, &run.BaseCredits, &run.ReservedCredits, &run.SettledCredits,
		&run.PolicyVersion, &run.ProviderStarted, &intent, &intentHash, &riskLevel,
		&run.RequiresConfirmation, &expires, &result, &message, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return AssistantRunRecord{}, err
	}
	if intent != "" {
		run.Intent = json.RawMessage(intent)
	}
	if result != "" {
		run.Result = json.RawMessage(result)
	}
	run.IntentSHA256 = intentHash
	run.RiskLevel = riskLevel
	run.AssistantMessage = message
	if expires.Valid {
		run.ConfirmationExpiresAt = &expires.Time
	}
	return run, nil
}

func insertAssistantMessageTx(ctx context.Context, tx pgx.Tx, conversationID, userID, runID, role, content string) error {
	if (role != "user" && role != "assistant") || strings.TrimSpace(content) == "" || len([]byte(content)) > 4096 {
		return errors.New("invalid assistant message")
	}
	messageID, err := id.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO assistant_messages(id,conversation_id,user_id,run_id,role,content,created_at)
		VALUES($1,$2,$3,$4,$5,$6,clock_timestamp())`, messageID, conversationID, userID, runID, role, content)
	return err
}

func insertAssistantAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	userID, runID, eventType, transcriptHash, intentHash, toolName, argsHash, resultResourceID string,
) error {
	eventID, err := id.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO assistant_audit_events(
		id,run_id,user_id,event_type,transcript_sha256,intent_sha256,tool_name,tool_args_sha256,result_resource_id,created_at
	) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),clock_timestamp())`,
		eventID, runID, userID, eventType, transcriptHash, intentHash, toolName, argsHash, resultResourceID)
	return err
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

func assistantIntentHash(value json.RawMessage) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func assistantActionArgsHash(value assistantActionIntent) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return assistantIntentHash(encoded), nil
}

func assistantActionItemMessage(title string) string {
	return fmt.Sprintf("Done — I added “%s” to your action items.", title)
}
