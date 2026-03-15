package backupcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestKeyFromBase64_Valid(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	encoded := base64.StdEncoding.EncodeToString(raw)

	key, err := KeyFromBase64(encoded)
	require.NoError(t, err)
	assert.Equal(t, raw, key)
}

func TestKeyFromBase64_InvalidBase64(t *testing.T) {
	t.Parallel()
	_, err := KeyFromBase64("not-valid-base64!!!")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode backup key")
}

func TestKeyFromBase64_WrongSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		size int
	}{
		{"too short 16", 16},
		{"too long 64", 64},
		{"empty", 0},
		{"off by one short", 31},
		{"off by one long", 33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := make([]byte, tt.size)
			encoded := base64.StdEncoding.EncodeToString(raw)
			_, err := KeyFromBase64(encoded)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "32 bytes")
		})
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	plaintext := []byte("hello world backup data")

	ct, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	got, err := Decrypt(ct, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestEncryptDecrypt_EmptyInput(t *testing.T) {
	t.Parallel()
	key := validKey(t)

	ct, err := Encrypt([]byte{}, key)
	require.NoError(t, err)

	got, err := Decrypt(ct, key)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestEncryptDecrypt_LargeInput(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	plaintext := make([]byte, 10*1024*1024) // 10 MB
	_, _ = rand.Read(plaintext)

	ct, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	got, err := Decrypt(ct, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestEncrypt_ProducesDifferentCiphertexts(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	plaintext := []byte("same input")

	ct1, err := Encrypt(plaintext, key)
	require.NoError(t, err)
	ct2, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	assert.False(t, bytes.Equal(ct1, ct2), "two encryptions should produce different ciphertexts (random nonce)")
}

func TestEncrypt_HasMagicHeader(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	ct, err := Encrypt([]byte("data"), key)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(ct, magic))
}

func TestDecrypt_WrongKey(t *testing.T) {
	t.Parallel()
	key1 := validKey(t)
	key2 := validKey(t)
	plaintext := []byte("secret")

	ct, err := Encrypt(plaintext, key1)
	require.NoError(t, err)

	_, err = Decrypt(ct, key2)
	require.Error(t, err)
}

func TestDecrypt_CorruptedCiphertext(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	ct, err := Encrypt([]byte("data"), key)
	require.NoError(t, err)

	// flip a byte in the ciphertext portion (after magic + nonce)
	ct[len(magic)+15] ^= 0xFF

	_, err = Decrypt(ct, key)
	require.Error(t, err)
}

func TestDecrypt_MissingMagicHeader(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	_, err := Decrypt([]byte("no magic here"), key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "magic header missing")
}

func TestDecrypt_TruncatedAfterMagic(t *testing.T) {
	t.Parallel()
	key := validKey(t)
	// magic header only, no nonce or ciphertext
	data := append([]byte{}, magic...)
	_, err := Decrypt(data, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	t.Parallel()
	goodKey := validKey(t)
	ct, err := Encrypt([]byte("data"), goodKey)
	require.NoError(t, err)

	badKey := make([]byte, 15)
	_, err = Decrypt(ct, badKey)
	require.Error(t, err)
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	t.Parallel()
	_, err := Encrypt([]byte("data"), []byte("short"))
	require.Error(t, err)
}

func TestIsEncrypted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"encrypted data", append(append([]byte{}, magic...), []byte("rest")...), true},
		{"magic only", append([]byte{}, magic...), true},
		{"empty", nil, false},
		{"too short", []byte("EV"), false},
		{"wrong prefix", []byte("NOT-ENCRYPTED-DATA"), false},
		{"partial magic", magic[:len(magic)-1], false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsEncrypted(tt.data))
		})
	}
}
