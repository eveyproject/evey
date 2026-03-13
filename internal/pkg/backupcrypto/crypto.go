// Package backupcrypto provides AES-256-GCM encryption for backup files.
// Generate a key: openssl rand -base64 32
package backupcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// magic prefix written before encrypted payloads
var magic = []byte("EVEY-ENC-V1\n")

// KeyFromBase64 decodes a base64-encoded 32-byte key.
func KeyFromBase64(s string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode backup key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("backup key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM and prepends the magic header.
// Output layout: magic | 12-byte nonce | ciphertext+tag
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	out := make([]byte, 0, len(magic)+len(ct))
	out = append(out, magic...)
	out = append(out, ct...)
	return out, nil
}

// Decrypt decrypts data produced by Encrypt. Returns an error if the magic
// header is missing (i.e. the data is not encrypted).
func Decrypt(data, key []byte) ([]byte, error) {
	if !IsEncrypted(data) {
		return nil, errors.New("not an encrypted backup (magic header missing)")
	}
	data = data[len(magic):]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// IsEncrypted reports whether data starts with the magic header.
func IsEncrypted(data []byte) bool {
	if len(data) < len(magic) {
		return false
	}
	for i, b := range magic {
		if data[i] != b {
			return false
		}
	}
	return true
}
