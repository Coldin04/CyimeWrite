package securevalue

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	encryptedValuePrefix = "enc:v1:"
	minSecretLength      = 32
)

// EncryptedValuePrefix is the prefix used for encrypted values stored in the database.
const EncryptedValuePrefix = encryptedValuePrefix

// ErrInvalidFormat is returned by DecryptString when the input is not a valid
// encrypted value (e.g. plaintext stored before encryption was introduced).
var ErrInvalidFormat = errors.New("invalid encrypted value format")

var insecureSecretBlocklist = map[string]struct{}{
	"insecure-default-secret-for-dev-only": {},
	"replace-with-a-strong-secret":         {},
	"change-me":                            {},
	"changeme":                             {},
	"secret":                               {},
	"f619a2942a188928414afd7e97fc6072c1c21905a723301749f13150bdd57612": {},
	"5619a079a803a895e1ced94f5a759dd12dd3df7c06a9355c11c12ed9805b6da9": {},
	"45d80dcef35bf1009602b9baa57c091daa5a307f3f275b7f510f6df18c2475bb": {},
}

func EncryptString(plaintext string) (string, error) {
	key, err := loadKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return encryptedValuePrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func DecryptString(encrypted string) (string, error) {
	key, err := loadKey()
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(encrypted, encryptedValuePrefix) {
		return "", ErrInvalidFormat
	}

	encoded := strings.TrimPrefix(encrypted, encryptedValuePrefix)
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", errors.New("invalid encrypted value payload")
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func ValidateEncryptionKey() error {
	_, err := loadKey()
	return err
}

func loadKey() ([]byte, error) {
	secretName := "APP_ENCRYPTION_KEY"
	secret := strings.TrimSpace(os.Getenv(secretName))
	if secret == "" {
		secretName = "JWT_SECRET_KEY"
		secret = strings.TrimSpace(os.Getenv(secretName))
	}
	if secret == "" {
		return nil, errors.New("missing APP_ENCRYPTION_KEY (or JWT_SECRET_KEY fallback)")
	}
	if err := validateSecret(secretName, secret); err != nil {
		return nil, err
	}

	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}

func validateSecret(name, secret string) error {
	if _, blocked := insecureSecretBlocklist[strings.ToLower(secret)]; blocked {
		return fmt.Errorf("%s is set to a known insecure default; generate a strong random secret (for example: openssl rand -hex 32)", name)
	}
	if len(secret) < minSecretLength {
		return fmt.Errorf("%s must be at least %d characters long (got %d); generate one with a cryptographically secure random generator (for example: openssl rand -hex 32)", name, minSecretLength, len(secret))
	}
	return nil
}
