package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"askolo/backend/internal/platform/id"
	"github.com/jackc/pgx/v5"
)

const (
	PublicWSMaxActiveConnections = 3
	PublicWSDefaultLease         = 45 * time.Second
	PublicWSMaxLease             = 5 * time.Minute
	PublicWSCleanupBatchSize     = 100
)

var (
	ErrPublicWSQuota            = errors.New("public WebSocket connection quota exceeded")
	ErrPublicWSSequenceConflict = errors.New("public WebSocket client sequence conflict")
	ErrPublicWSSequenceGap      = errors.New("public WebSocket client sequence gap")
	ErrPublicWSSessionNotFound  = errors.New("public WebSocket session not found")
	ErrPublicWSAlreadyTerminal  = errors.New("public WebSocket session is terminal")
	ErrPublicWSInvalidMetadata  = errors.New("invalid public WebSocket metadata")
)

type PublicWSSession struct {
	ID, UserID, WorkspaceID, Status  string
	LeaseUntil, CreatedAt, UpdatedAt time.Time
	ClosedAt                         *time.Time
	ClientSequence, ServerSequence   int64
	TerminalState                    string
}

type PublicWSClientSequence struct {
	Sequence                                int64
	MessageHash, CorrelationID, MessageType string
}

type PublicWSServerEvent struct {
	SessionID, CorrelationID, EventType, EventHash    string
	Sequence                                          int64
	AssistantRunID, VoiceReservationID, TerminalState string
	CreatedAt                                         time.Time
}

var publicWSHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var publicWSIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

func validatePublicWSString(value string, required bool) bool {
	return value == strings.TrimSpace(value) &&
		(!required || value != "") &&
		len(value) <= 200 &&
		publicWSIDPattern.MatchString(value)
}

func validatePublicWSLease(ttl time.Duration) error {
	if ttl <= 0 || ttl > PublicWSMaxLease {
		return fmt.Errorf("%w: lease duration", ErrPublicWSInvalidMetadata)
	}
	return nil
}

func (s *Store) CreatePublicWSSession(ctx context.Context, userID, workspaceID string, ttl time.Duration) (PublicWSSession, error) {
	var result PublicWSSession
	if s == nil || s.pool == nil {
		return result, errors.New("database is not configured")
	}
	if !validatePublicWSString(userID, true) || !validatePublicWSString(workspaceID, true) {
		return result, ErrPublicWSInvalidMetadata
	}
	if err := validatePublicWSLease(ttl); err != nil {
		return result, err
	}
	sessionID, err := id.New()
	if err != nil {
		return result, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7261736))`, userID); err != nil {
		return result, err
	}
	// Expiry is folded into creation, keeping quota bounded without an unbounded cleanup scan.
	_, err = tx.Exec(ctx, `UPDATE public_ws_sessions SET status='expired',terminal_state='expired',closed_at=COALESCE(closed_at,NOW()),updated_at=NOW()
		WHERE user_id=$1 AND status='active' AND lease_until<=NOW()
		AND id IN (SELECT id FROM public_ws_sessions WHERE user_id=$1 AND status='active' AND lease_until<=NOW() ORDER BY lease_until LIMIT $2)`, userID, PublicWSCleanupBatchSize)
	if err != nil {
		return result, err
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM public_ws_sessions WHERE user_id=$1 AND status='active' AND lease_until>NOW()`, userID).Scan(&active); err != nil {
		return result, err
	}
	if active >= PublicWSMaxActiveConnections {
		return result, ErrPublicWSQuota
	}
	lease := time.Now().UTC().Add(ttl)
	err = tx.QueryRow(ctx, `INSERT INTO public_ws_sessions(id,user_id,workspace_id,lease_until) VALUES($1,$2,$3,$4)
		RETURNING status,lease_until,created_at,updated_at,client_sequence,server_sequence`, sessionID, userID, workspaceID, lease).
		Scan(&result.Status, &result.LeaseUntil, &result.CreatedAt, &result.UpdatedAt, &result.ClientSequence, &result.ServerSequence)
	if err != nil {
		return result, err
	}
	result.ID, result.UserID, result.WorkspaceID = sessionID, userID, workspaceID
	if err = tx.Commit(ctx); err != nil {
		return PublicWSSession{}, err
	}
	return result, nil
}

func (s *Store) GetPublicWSSessionForOwner(ctx context.Context, userID, sessionID string) (PublicWSSession, error) {
	var r PublicWSSession
	if s == nil || s.pool == nil {
		return r, errors.New("database is not configured")
	}
	if !validatePublicWSString(userID, true) || !validatePublicWSString(sessionID, true) {
		return r, ErrPublicWSInvalidMetadata
	}
	var closed *time.Time
	err := s.pool.QueryRow(ctx, `SELECT id,user_id,workspace_id,status,lease_until,created_at,updated_at,closed_at,client_sequence,server_sequence,COALESCE(terminal_state,'')
		FROM public_ws_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID).Scan(&r.ID, &r.UserID, &r.WorkspaceID, &r.Status, &r.LeaseUntil, &r.CreatedAt, &r.UpdatedAt, &closed, &r.ClientSequence, &r.ServerSequence, &r.TerminalState)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrPublicWSSessionNotFound
	}
	if err != nil {
		return r, err
	}
	r.ClosedAt = closed
	return r, nil
}

func (s *Store) TouchPublicWSSession(ctx context.Context, userID, sessionID string, ttl time.Duration) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if err := validatePublicWSLease(ttl); err != nil {
		return err
	}
	if !validatePublicWSString(userID, true) || !validatePublicWSString(sessionID, true) {
		return ErrPublicWSInvalidMetadata
	}
	tag, err := s.pool.Exec(ctx, `UPDATE public_ws_sessions SET lease_until=NOW()+$3::interval,updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='active' AND lease_until>NOW()`, sessionID, userID, ttl.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPublicWSSessionNotFound
	}
	return nil
}

func (s *Store) ClosePublicWSSession(ctx context.Context, userID, sessionID, terminalState string) error {
	if s == nil || s.pool == nil {
		return errors.New("database is not configured")
	}
	if !validatePublicWSString(userID, true) || !validatePublicWSString(sessionID, true) || !validatePublicWSString(terminalState, true) {
		return ErrPublicWSInvalidMetadata
	}
	if terminalState != "completed" && terminalState != "cancelled" && terminalState != "failed" && terminalState != "expired" {
		return ErrPublicWSInvalidMetadata
	}
	tag, err := s.pool.Exec(ctx, `UPDATE public_ws_sessions SET status=CASE WHEN $3='expired' THEN 'expired' ELSE 'closed' END,terminal_state=$3,closed_at=COALESCE(closed_at,NOW()),updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='active'`, sessionID, userID, terminalState)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPublicWSSessionNotFound
	}
	return nil
}

// ExpirePublicWSSessions closes a bounded batch of leases that have elapsed.
// The UPDATE predicate makes concurrent cleanup workers harmless.
func (s *Store) ExpirePublicWSSessions(ctx context.Context, limit int) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, errors.New("database is not configured")
	}
	if limit < 1 || limit > PublicWSCleanupBatchSize {
		return 0, ErrPublicWSInvalidMetadata
	}
	tag, err := s.pool.Exec(ctx, `WITH expired AS (
		SELECT id FROM public_ws_sessions
		WHERE status='active' AND lease_until<=NOW()
		ORDER BY lease_until, id
		LIMIT $1
	)
	UPDATE public_ws_sessions AS s
	SET status='expired', terminal_state='expired',
	    closed_at=COALESCE(s.closed_at,NOW()), updated_at=NOW()
	FROM expired WHERE s.id=expired.id AND s.status='active'`, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) AcceptPublicWSClientSequence(ctx context.Context, userID, sessionID string, sequence int64, messageHash, correlationID, messageType string) (PublicWSClientSequence, error) {
	var out PublicWSClientSequence
	if s == nil || s.pool == nil {
		return out, errors.New("database is not configured")
	}
	if sequence <= 0 || !publicWSHashPattern.MatchString(messageHash) || !validatePublicWSString(userID, true) || !validatePublicWSString(sessionID, true) || !validatePublicWSString(messageType, true) || len(correlationID) > 200 {
		return out, ErrPublicWSInvalidMetadata
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var status string
	var current int64
	err = tx.QueryRow(ctx, `SELECT status,client_sequence FROM public_ws_sessions WHERE id=$1 AND user_id=$2 FOR UPDATE`, sessionID, userID).Scan(&status, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrPublicWSSessionNotFound
	}
	if err != nil {
		return out, err
	}
	if status != "active" {
		return out, ErrPublicWSAlreadyTerminal
	}
	if sequence <= current {
		var priorHash string
		err = tx.QueryRow(ctx, `SELECT message_hash FROM public_ws_client_sequences WHERE session_id=$1 AND sequence=$2`, sessionID, sequence).Scan(&priorHash)
		if err != nil {
			return out, ErrPublicWSSequenceConflict
		}
		if priorHash != messageHash {
			return out, ErrPublicWSSequenceConflict
		}
		return PublicWSClientSequence{Sequence: sequence, MessageHash: messageHash, CorrelationID: correlationID, MessageType: messageType}, nil
	}
	if sequence != current+1 {
		return out, ErrPublicWSSequenceGap
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public_ws_client_sequences(session_id,sequence,message_hash,correlation_id,message_type) VALUES($1,$2,$3,$4,$5)`, sessionID, sequence, messageHash, correlationID, messageType); err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE public_ws_sessions SET client_sequence=$2,updated_at=NOW() WHERE id=$1`, sessionID, sequence); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return PublicWSClientSequence{sequence, messageHash, correlationID, messageType}, nil
}

func (s *Store) AppendPublicWSServerEvent(ctx context.Context, userID, sessionID, correlationID, eventType, eventHash, assistantRunID, voiceReservationID, terminalState string) (PublicWSServerEvent, error) {
	var out PublicWSServerEvent
	if s == nil || s.pool == nil {
		return out, errors.New("database is not configured")
	}
	if !validatePublicWSString(userID, true) || !validatePublicWSString(sessionID, true) || !validatePublicWSString(eventType, true) || len(correlationID) > 200 || !publicWSHashPattern.MatchString(eventHash) || len(assistantRunID) > 200 || len(voiceReservationID) > 200 || len(terminalState) > 32 {
		return out, ErrPublicWSInvalidMetadata
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var seq int64
	var status string
	err = tx.QueryRow(ctx, `SELECT server_sequence,status FROM public_ws_sessions WHERE id=$1 AND user_id=$2 FOR UPDATE`, sessionID, userID).Scan(&seq, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrPublicWSSessionNotFound
	}
	if err != nil {
		return out, err
	}
	if status != "active" {
		return out, ErrPublicWSAlreadyTerminal
	}
	seq++
	err = tx.QueryRow(ctx, `INSERT INTO public_ws_server_events(session_id,sequence,correlation_id,event_type,event_hash,assistant_run_id,voice_reservation_id,terminal_state) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,'')) RETURNING created_at`, sessionID, seq, correlationID, eventType, eventHash, assistantRunID, voiceReservationID, terminalState).Scan(&out.CreatedAt)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `UPDATE public_ws_sessions SET server_sequence=$2,updated_at=NOW() WHERE id=$1`, sessionID, seq); err != nil {
		return out, err
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	out.SessionID, out.Sequence, out.CorrelationID, out.EventType, out.EventHash, out.AssistantRunID, out.VoiceReservationID, out.TerminalState = sessionID, seq, correlationID, eventType, eventHash, assistantRunID, voiceReservationID, terminalState
	return out, nil
}

func (s *Store) ListPublicWSServerEvents(ctx context.Context, userID, sessionID string, afterSequence int64, limit int) ([]PublicWSServerEvent, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("database is not configured")
	}
	if limit < 1 || limit > PublicWSCleanupBatchSize {
		return nil, ErrPublicWSInvalidMetadata
	}
	rows, err := s.pool.Query(ctx, `SELECT e.session_id,e.sequence,e.correlation_id,e.event_type,e.event_hash,COALESCE(e.assistant_run_id,''),COALESCE(e.voice_reservation_id,''),COALESCE(e.terminal_state,''),e.created_at FROM public_ws_server_events e JOIN public_ws_sessions s ON s.id=e.session_id WHERE s.id=$1 AND s.user_id=$2 AND e.sequence>$3 ORDER BY e.sequence LIMIT $4`, sessionID, userID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []PublicWSServerEvent{}
	for rows.Next() {
		var e PublicWSServerEvent
		if err := rows.Scan(&e.SessionID, &e.Sequence, &e.CorrelationID, &e.EventType, &e.EventHash, &e.AssistantRunID, &e.VoiceReservationID, &e.TerminalState, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
