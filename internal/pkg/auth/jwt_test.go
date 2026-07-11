package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidate(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	claims := user.Claims{
		UserID:      "user-1",
		Username:    "admin",
		IsAdmin:     true,
		ProfileID:   "profile-1",
		Permissions: []string{"library.read", "media.read"},
	}

	token, err := gen.GenerateAccessToken(claims)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	parsed, err := gen.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", parsed.UserID)
	assert.Equal(t, "admin", parsed.Username)
	assert.True(t, parsed.IsAdmin)
	assert.Equal(t, "profile-1", parsed.ProfileID)
	assert.Contains(t, parsed.Permissions, "library.read")
}

func TestExpiredToken(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, -1*time.Hour)

	token, err := gen.GenerateAccessToken(user.Claims{UserID: "u1", Username: "test"})
	require.NoError(t, err)

	_, err = gen.ValidateAccessToken(token)
	require.Error(t, err)
}

func TestInvalidSignature(t *testing.T) {
	t.Parallel()
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := rsa.GenerateKey(rand.Reader, 2048)

	gen1 := auth.NewJWTGenerator(key1, 15*time.Minute)
	gen2 := auth.NewJWTGenerator(key2, 15*time.Minute)

	token, _ := gen1.GenerateAccessToken(user.Claims{UserID: "u1", Username: "test"})
	_, err := gen2.ValidateAccessToken(token)
	// Pin the rejection reason to the signature specifically — the previous
	// assertion ("token") also matched expiry/malformed errors, so it would not
	// catch a regression where a wrong-key token was rejected for the wrong
	// reason (or accepted and rejected later on an unrelated check).
	assert.ErrorContains(t, err, "signature")
}

func TestAlgorithmConfusionRejected(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	// Craft a token signed with HMAC using the public key bytes — algorithm confusion attack
	pubBytes := key.N.Bytes()
	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      "attacker",
		"username": "attacker",
		"is_admin": true,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := hmacToken.SignedString(pubBytes)
	require.NoError(t, err)

	_, err = gen.ValidateAccessToken(tokenStr)
	assert.Error(t, err, "HMAC-signed token must be rejected by RSA validator")
}

func TestGenerateRefreshToken(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	gen := auth.NewJWTGenerator(key, 15*time.Minute)
	token, err := gen.GenerateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	})
	require.NoError(t, err)
	claims := parsed.Claims.(jwt.MapClaims)
	assert.Equal(t, "refresh", claims["type"])
	assert.NotEmpty(t, claims["jti"])
}

func TestValidateAccessToken_NoProfileNoPermissions(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	token, err := gen.GenerateAccessToken(user.Claims{
		UserID:   "u1",
		Username: "bob",
		IsAdmin:  false,
	})
	require.NoError(t, err)

	parsed, err := gen.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "u1", parsed.UserID)
	assert.Empty(t, parsed.ProfileID)
	assert.Empty(t, parsed.Permissions)
}

func TestValidateAccessToken_MalformedToken(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	_, err := gen.ValidateAccessToken("not.a.token")
	require.Error(t, err)
}

func TestValidateAccessToken_RS512Rejected(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	token := jwt.NewWithClaims(jwt.SigningMethodRS512, jwt.MapClaims{
		"sub":      "u1",
		"username": "bob",
		"is_admin": false,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(key)
	require.NoError(t, err)

	_, err = gen.ValidateAccessToken(tokenStr)
	require.Error(t, err, "RS512 must be rejected — only RS256 is allowed")
}

func TestValidateAccessToken_MissingRequiredClaims(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	// craft a token missing "sub" and "username"
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(key)
	require.NoError(t, err)

	_, err = gen.ValidateAccessToken(tokenStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token claims")
}

func TestPINToken_RoundTrip(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	g := auth.NewJWTGenerator(key, 15*time.Minute)

	tok, err := g.GeneratePINToken("prf-123", time.Minute)
	require.NoError(t, err)
	pid, err := g.ParsePINToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "prf-123", pid)
}

func TestPINToken_Rejections(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	g := auth.NewJWTGenerator(key, 15*time.Minute)

	// empty profile / non-positive ttl
	_, err := g.GeneratePINToken("", time.Minute)
	require.Error(t, err)
	_, err = g.GeneratePINToken("prf-1", 0)
	require.Error(t, err)

	// expired (mint with a 1ns ttl, then let it lapse)
	expired, err := g.GeneratePINToken("prf-1", time.Nanosecond)
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = g.ParsePINToken(expired)
	require.Error(t, err, "expired pin token must be rejected")

	// signed by a different key (tampered/forged)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged, _ := auth.NewJWTGenerator(other, time.Minute).GeneratePINToken("prf-1", time.Minute)
	_, err = g.ParsePINToken(forged)
	require.Error(t, err, "pin token signed by another key must be rejected")

	// an access token is not a pin token (token_type guard)
	access, _ := g.GenerateAccessToken(user.Claims{UserID: "u1", ProfileID: "prf-1"})
	_, err = g.ParsePINToken(access)
	assert.Error(t, err, "access token must not pass as a pin token")
}

// The epoch claim must round-trip mint→validate, and legacy tokens without it
// must parse as epoch 0 (the users.token_epoch column default).
func TestAccessToken_EpochRoundTrip(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	gen := auth.NewJWTGenerator(key, 15*time.Minute)

	token, err := gen.GenerateAccessToken(user.Claims{UserID: "u1", Username: "u", TokenEpoch: 7})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := gen.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.TokenEpoch != 7 {
		t.Errorf("epoch: want 7, got %d", claims.TokenEpoch)
	}

	legacy, err := gen.GenerateAccessToken(user.Claims{UserID: "u1", Username: "u"})
	if err != nil {
		t.Fatalf("generate legacy: %v", err)
	}
	lc, err := gen.ValidateAccessToken(legacy)
	if err != nil {
		t.Fatalf("validate legacy: %v", err)
	}
	if lc.TokenEpoch != 0 {
		t.Errorf("zero-value epoch: want 0, got %d", lc.TokenEpoch)
	}
}
