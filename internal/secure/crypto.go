// Package secure provides AES-256-GCM encryption helpers used to protect
// provider API keys stored in PostgreSQL.
//
// Values encrypted at rest carry the "enc:" prefix so that enabling/disabling
// encryption (via PROVIDER_MASTER_KEY) never corrupts previously stored values:
// entries without the prefix are treated as plaintext and returned as-is.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const encPrefix = "enc:"

// DeriveKey hashes a passphrase into a 32-byte AES-256 key.
func DeriveKey(passphrase string) []byte {
	sum := sha256.Sum256([]byte(passphrase))
	return sum[:]
}

// Encrypt returns the plaintext unchanged when no key is set (dev mode),
// otherwise an "enc:"-prefixed base64 AES-GCM ciphertext.
func Encrypt(plaintext string, key []byte) (string, error) {
	if len(key) == 0 || plaintext == "" {
		return plaintext, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secure: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secure: gcm init: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secure: nonce: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.RawStdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. Plaintext entries (no prefix) are returned unchanged.
func Decrypt(ciphertext string, key []byte) (string, error) {
	if len(key) == 0 || ciphertext == "" {
		return ciphertext, nil
	}
	if !strings.HasPrefix(ciphertext, encPrefix) {
		return ciphertext, nil // legacy plaintext entry
	}
	raw := strings.TrimPrefix(ciphertext, encPrefix)
	data, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("secure: base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secure: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secure: gcm init: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("secure: ciphertext too short")
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secure: decrypt failed: %w", err)
	}
	return string(pt), nil
}

// Mask redacts a secret for admin listing (e.g. "sk-proj-****4fA2").
func Mask(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 10 {
		return "****"
	}
	return secret[:6] + "****" + secret[len(secret)-4:]
}
