package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 3
	argonThreads   = 2
	argonKeyLen    = 32
	argonSaltLen   = 16
)

var ErrInvalidPassword = errors.New("invalid password")

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrInvalidPassword
	}
	if len(password) < 12 || len(password) > 128 {
		return ErrInvalidPassword
	}
	if strings.TrimSpace(password) != password {
		return ErrInvalidPassword
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoryKiB,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	params := map[string]uint32{}
	for _, item := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(item, "=", 2)
		if len(keyValue) != 2 {
			return false
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil {
			return false
		}
		params[keyValue[0]] = uint32(value)
	}
	if params["m"] == 0 || params["t"] == 0 || params["p"] == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, params["t"], params["m"], uint8(params["p"]), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
