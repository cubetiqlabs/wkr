package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const prefix = "enc::"

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// Returns a base64-encoded string prefixed with "enc::".
func Encrypt(plaintext, key string) (string, error) {
	block, err := aes.NewCipher(deriveKey(key))
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
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts an "enc::"-prefixed ciphertext using AES-256-GCM.
func Decrypt(ciphertext, key string) (string, error) {
	if !IsEncrypted(ciphertext) {
		return ciphertext, nil // plaintext passthrough
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// IsEncrypted checks if a value is encrypted.
func IsEncrypted(s string) bool {
	return len(s) > len(prefix) && s[:len(prefix)] == prefix
}

// EncryptMap encrypts all values in a map.
func EncryptMap(m map[string]string, key string) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		enc, err := Encrypt(v, key)
		if err != nil {
			return nil, err
		}
		out[k] = enc
	}
	return out, nil
}

// DecryptMap decrypts all values in a map (skips non-encrypted values).
func DecryptMap(m map[string]string, key string) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		dec, err := Decrypt(v, key)
		if err != nil {
			return nil, err
		}
		out[k] = dec
	}
	return out, nil
}

// deriveKey produces a 32-byte key from an arbitrary string via SHA-256.
func deriveKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
