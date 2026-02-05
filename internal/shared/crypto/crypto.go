package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
)

const (
	ivLength      = 12 // 96 bits for GCM
	authTagLength = 16 // 128 bits
	keyLength     = 32 // 256 bits for AES-256
)

var encryptionKey []byte

func init() {
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		return // Key will be checked at runtime when encryption/decryption is called
	}
	var err error
	encryptionKey, err = parseKey(key)
	if err != nil {
		panic(fmt.Sprintf("failed to parse ENCRYPTION_KEY: %v", err))
	}
}

func parseKey(key string) ([]byte, error) {
	// Support both hex (64 chars) and base64 (44 chars) encoded keys
	if len(key) == 64 && isHex(key) {
		return hex.DecodeString(key)
	}
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	if len(keyBytes) != keyLength {
		return nil, fmt.Errorf("key must be %d bytes (256 bits), got %d bytes", keyLength, len(keyBytes))
	}
	return keyBytes, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func getKey() ([]byte, error) {
	if encryptionKey != nil {
		return encryptionKey, nil
	}
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY environment variable is not set")
	}
	var err error
	encryptionKey, err = parseKey(key)
	if err != nil {
		return nil, err
	}
	return encryptionKey, nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM.
// Returns a base64-encoded string containing: iv + authTag + ciphertext
// Compatible with the TypeScript crypto.ts implementation.
func Encrypt(plaintext string) (string, error) {
	key, err := getKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// GCM Seal appends the auth tag to the ciphertext
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), nil)

	// Extract auth tag (last 16 bytes) and actual ciphertext
	authTag := ciphertext[len(ciphertext)-authTagLength:]
	encryptedData := ciphertext[:len(ciphertext)-authTagLength]

	// Combine: iv (12 bytes) + authTag (16 bytes) + ciphertext
	combined := make([]byte, 0, ivLength+authTagLength+len(encryptedData))
	combined = append(combined, iv...)
	combined = append(combined, authTag...)
	combined = append(combined, encryptedData...)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts a base64-encoded ciphertext that was encrypted with Encrypt().
// Expects format: base64(iv + authTag + ciphertext)
// Compatible with the TypeScript crypto.ts implementation.
func Decrypt(ciphertext string) (string, error) {
	key, err := getKey()
	if err != nil {
		return "", err
	}

	combined, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(combined) < ivLength+authTagLength {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract components
	iv := combined[:ivLength]
	authTag := combined[ivLength : ivLength+authTagLength]
	encryptedData := combined[ivLength+authTagLength:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Reconstruct the ciphertext with auth tag appended (as GCM expects)
	ciphertextWithTag := append(encryptedData, authTag...)

	plaintext, err := gcm.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}
