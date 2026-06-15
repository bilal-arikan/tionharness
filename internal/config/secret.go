package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// secretFileName holds the persisted credential encryption key.
const secretFileName = "credential-secret"

// Secret manages AES-GCM encryption for stored credentials (API keys, tokens).
type Secret struct {
	gcm cipher.AEAD
}

// LoadSecret resolves the credential secret in this order:
//  1. CREDENTIAL_SECRET environment variable (base64 of 32 bytes)
//  2. DATA_DIR/credential-secret file
//  3. newly generated key, persisted to the file
func LoadSecret(dataDir string) (*Secret, error) {
	key, err := resolveKey(dataDir)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Secret{gcm: gcm}, nil
}

// Encrypt seals plaintext and returns a base64-encoded string (nonce prepended).
func (s *Secret) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (s *Secret) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	nonceSize := s.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(plaintext), nil
}

// resolveKey returns a 32-byte AES key from env, file, or generation.
func resolveKey(dataDir string) ([]byte, error) {
	if env := os.Getenv("CREDENTIAL_SECRET"); env != "" {
		key, err := base64.StdEncoding.DecodeString(env)
		if err != nil {
			return nil, fmt.Errorf("invalid CREDENTIAL_SECRET base64: %w", err)
		}
		if len(key) != 32 {
			return nil, errors.New("CREDENTIAL_SECRET must decode to 32 bytes")
		}
		return key, nil
	}

	path := filepath.Join(dataDir, secretFileName)
	if data, err := os.ReadFile(path); err == nil {
		key, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil || len(key) != 32 {
			return nil, errors.New("corrupt credential-secret file")
		}
		return key, nil
	}

	// Generate and persist a new key.
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("persist credential-secret: %w", err)
	}
	return key, nil
}
