package google

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const homeEmailThreadStateKeyVersion = 1

// homeEmailThreadToken derives a purpose-specific key from the configured
// auth HMAC secret, then tokenizes a Gmail thread ID without persisting it.
// Domain separation keeps these tokens unrelated to auth rate-limit digests.
func homeEmailThreadToken(secret, connectionID, threadID string) string {
	key := homeEmailThreadStateKey(secret)
	token := hmac.New(sha256.New, key)
	_, _ = token.Write([]byte(connectionID))
	_, _ = token.Write([]byte{0})
	_, _ = token.Write([]byte(threadID))
	return hex.EncodeToString(token.Sum(nil))
}

func homeEmailThreadKeyFingerprint(secret string) string {
	key := homeEmailThreadStateKey(secret)
	fingerprint := hmac.New(sha256.New, key)
	_, _ = fingerprint.Write([]byte("fingerprint"))
	return hex.EncodeToString(fingerprint.Sum(nil))
}

func homeEmailThreadStateKey(secret string) []byte {
	derive := hmac.New(sha256.New, []byte(secret))
	_, _ = derive.Write([]byte("askolo/home-email/thread-state/v1"))
	return derive.Sum(nil)
}
