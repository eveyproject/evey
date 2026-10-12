package auth_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBcryptHash_RoundTrip(t *testing.T) {
	h := auth.BcryptHasher{}
	password := "s3cur3P@ssw0rd!"

	hash, err := h.Hash(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	ok, err := h.Verify(password, hash)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestBcryptHash_WrongPassword(t *testing.T) {
	h := auth.BcryptHasher{}

	hash, err := h.Hash("correct-password")
	require.NoError(t, err)

	ok, err := h.Verify("wrong-password", hash)
	require.NoError(t, err)
	assert.False(t, ok)
}
