package id

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func New() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
