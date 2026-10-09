package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// HashAPIKey mengembalikan representasi SHA-256 hex dari raw API key.
func HashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// GenerateAPIKey menghasilkan API key acak (32 byte / 64 hex char dengan prefix tnk_)
// dan mengembalikan (rawKey, sha256HexHash, error).
func GenerateAPIKey() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes for api key: %w", err)
	}
	rawKey := "tnk_" + hex.EncodeToString(b)
	hash := HashAPIKey(rawKey)
	return rawKey, hash, nil
}
