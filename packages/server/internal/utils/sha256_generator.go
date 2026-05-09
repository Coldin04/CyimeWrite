// package utils is a utility package for the server, this is sha256_generator.go
package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateRandomSHA256 returns a hex-encoded random SHA256 hash string.
// It generates 32 random bytes and returns their SHA256 hex digest.
func GenerateRandomSHA256() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	sum := sha256.Sum256(randomBytes)
	return hex.EncodeToString(sum[:]), nil
}
