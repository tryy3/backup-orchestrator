package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const encPrefix = "enc:"

// Encrypt encrypts plaintext using AES-256-GCM and returns an "enc:"-prefixed
// base64-encoded ciphertext. The key must be exactly 32 bytes.
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value produced by Encrypt. Values without the "enc:"
// prefix are returned as-is (plaintext legacy values).
func Decrypt(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}

	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	plaintext, err := aead.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted reports whether a value carries the "enc:" prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encPrefix)
}
