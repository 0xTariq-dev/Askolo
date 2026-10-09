package product

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	v2AgendaPageSize             = 500
	v2AgendaTaskBatchSize        = 100
	v2AgendaCursorMaxOffset      = 1000000
	v2AgendaTaskBatchMaxItemSize = v2MaxAgendaDateRange * v2AgendaTaskBatchSize
)

type v2AgendaCursor struct {
	Version             int    `json:"v"`
	Scope               string `json:"s"`
	TaskOffset          int    `json:"to"`
	TaskItemOffset      int    `json:"ti"`
	PlanEntryOffset     int    `json:"po"`
	CalendarEventOffset int    `json:"eo"`
	TasksDone           bool   `json:"td"`
	PlanEntriesDone     bool   `json:"pd"`
	CalendarEventsDone  bool   `json:"ed"`
}

func v2AgendaScope(userID, workspaceID, role, from, to, timeZone string) string {
	sum := sha256.Sum256([]byte(strings.Join(
		[]string{userID, workspaceID, role, from, to, timeZone},
		"\x00",
	)))
	return hex.EncodeToString(sum[:])
}

func encodeV2AgendaCursor(cursor v2AgendaCursor, signingSecret string) (string, error) {
	if len(signingSecret) < 32 {
		return "", errors.New("agenda cursor signing secret is unavailable")
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(encoded)
	signature := v2AgendaCursorSignature(payload, signingSecret)
	return payload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeV2AgendaCursor(token, expectedScope, signingSecret string) (v2AgendaCursor, error) {
	if len(token) == 0 || len(token) > 4096 || len(signingSecret) < 32 {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, v2AgendaCursorSignature(parts[0], signingSecret)) {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor v2AgendaCursor
	if err := decoder.Decode(&cursor); err != nil {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	if cursor.Version != 1 || cursor.Scope != expectedScope ||
		cursor.TaskOffset < 0 || cursor.TaskOffset > v2AgendaCursorMaxOffset ||
		cursor.TaskItemOffset < 0 || cursor.TaskItemOffset > v2AgendaTaskBatchMaxItemSize ||
		cursor.PlanEntryOffset < 0 || cursor.PlanEntryOffset > v2AgendaCursorMaxOffset ||
		cursor.CalendarEventOffset < 0 || cursor.CalendarEventOffset > v2AgendaCursorMaxOffset {
		return v2AgendaCursor{}, errors.New("invalid agenda page token")
	}
	return cursor, nil
}

func v2AgendaCursorSignature(payload, signingSecret string) []byte {
	keyDerivation := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = keyDerivation.Write([]byte("askolo.v2.agenda.cursor.signing-key.v1"))
	signingKey := keyDerivation.Sum(nil)
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func validV2TimeZone(value string) bool {
	if value == "" || len(value) > 64 || strings.TrimSpace(value) != value || value == "Local" {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}
