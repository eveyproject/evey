package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOrGenerateRSAKey_GeneratesNewKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	key, err := auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)
	require.NotNil(t, key)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadOrGenerateRSAKey_LoadsExistingKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	// Generate and save a key
	original, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	require.NoError(t, err)
	err = pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(original),
	})
	require.NoError(t, err)
	f.Close()

	loaded, err := auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)
	assert.True(t, original.Equal(loaded))
}

func TestLoadOrGenerateRSAKey_FixesLoosePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	require.NoError(t, err)
	err = pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	require.NoError(t, err)
	f.Close()

	_, err = auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadOrGenerateRSAKey_CorruptedPEM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	err := os.WriteFile(path, []byte("not a pem file"), 0600)
	require.NoError(t, err)

	_, err = auth.LoadOrGenerateRSAKey(path)
	assert.ErrorContains(t, err, "corrupted")
}

func TestLoadOrGenerateRSAKey_InvalidPKCS1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	// Valid PEM block but invalid PKCS1 content
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("garbage")}
	err := os.WriteFile(path, pem.EncodeToMemory(block), 0600)
	require.NoError(t, err)

	_, err = auth.LoadOrGenerateRSAKey(path)
	assert.Error(t, err)
}

func TestLoadOrGenerateRSAKey_CreatesParentDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "jwt.key")

	key, err := auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)
	require.NotNil(t, key)

	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestLoadOrGenerateRSAKey_Deterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt.key")

	key1, err := auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)

	key2, err := auth.LoadOrGenerateRSAKey(path)
	require.NoError(t, err)

	assert.True(t, key1.Equal(key2), "loading same file twice should return same key")
}
