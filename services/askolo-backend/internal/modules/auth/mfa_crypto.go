package auth

import (
	"context"
	"errors"

	authcrypto "askolo/backend/internal/platform/crypto"
)

func (h *Handler) decryptTOTPSecret(ctx context.Context, userID string) ([]byte, error) {
	encrypted, err := h.store.EncryptedTOTPSecret(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(h.cfg.TOTPEncryptionKey) != 32 {
		return nil, errors.New("TOTP encryption key is unavailable")
	}
	secret, activeErr := authcrypto.Open(h.cfg.TOTPEncryptionKey, encrypted)
	if activeErr == nil {
		return secret, nil
	}
	if len(h.cfg.TOTPEncryptionKeyFallback) != 32 {
		return nil, activeErr
	}
	secret, err = authcrypto.Open(h.cfg.TOTPEncryptionKeyFallback, encrypted)
	if err != nil {
		return nil, activeErr
	}
	replacement, err := authcrypto.Seal(h.cfg.TOTPEncryptionKey, secret)
	if err != nil {
		return nil, err
	}
	if err := h.store.ReplaceEncryptedTOTPSecret(ctx, userID, encrypted, replacement); err != nil {
		for i := range secret {
			secret[i] = 0
		}
		return nil, err
	}
	return secret, nil
}
