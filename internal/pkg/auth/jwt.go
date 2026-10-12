package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/golang-jwt/jwt/v5"
)

type JWTGenerator struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	accessTTL  time.Duration
}

func NewJWTGenerator(privateKey *rsa.PrivateKey, accessTTL time.Duration) *JWTGenerator {
	return &JWTGenerator{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		accessTTL:  accessTTL,
	}
}

func (g *JWTGenerator) GenerateAccessToken(claims user.Claims) (string, error) {
	return g.GenerateAccessTokenWithTTL(claims, g.accessTTL)
}

// GenerateAccessTokenWithTTL mints an access token with an explicit lifetime
// instead of the configured one.
//
// This exists for server-side work that OUTLIVES the request that asked for it:
// the built-in load test runs for many minutes, and borrowing the caller's
// browser token means the run dies mid-flight with a 401 the moment that token
// expires - reporting a capacity verdict that is really just an expiry. Such a
// token carries the same claims (same user, same permissions, same epoch), so a
// revoked session still kills it; only the lifetime differs.
func (g *JWTGenerator) GenerateAccessTokenWithTTL(claims user.Claims, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = g.accessTTL
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":         claims.UserID,
		"username":    claims.Username,
		"is_admin":    claims.IsAdmin,
		"profile_id":  claims.ProfileID,
		"locale":      claims.Locale,
		"permissions": claims.Permissions,
		"epoch":       claims.TokenEpoch,
		"imp_by":      claims.ImpersonatedBy,
		"iat":         now.Unix(),
		"exp":         now.Add(ttl).Unix(),
	})
	return token.SignedString(g.privateKey)
}

func (g *JWTGenerator) GenerateRefreshToken() (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"type": "refresh",
		"jti":  id.New(""),
		"iat":  now.Unix(),
		"exp":  now.Add(7 * 24 * time.Hour).Unix(),
	})
	return token.SignedString(g.privateKey)
}

// DescribeRefreshToken decodes the token's own claims without verifying the
// signature - this is for a log line about a token the store does not know,
// so the signature is exactly what cannot vouch for anything here. The
// description says when the token claims to have been issued: a refresh token
// presented long after a sibling rotated it (and the sweep removed the row)
// carries an old iat, a forged one carries whatever it likes.
func (g *JWTGenerator) DescribeRefreshToken(raw string) string {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(raw, claims); err != nil {
		return ""
	}
	if t, _ := claims["type"].(string); t != "refresh" {
		return ""
	}
	jti, _ := claims["jti"].(string)
	desc := "jti=" + jti
	if iat, err := claims.GetIssuedAt(); err == nil && iat != nil {
		desc += " issued=" + iat.UTC().Format(time.RFC3339) + " age=" + time.Since(iat.Time).Truncate(time.Second).String()
	}
	if exp, err := claims.GetExpirationTime(); err == nil && exp != nil {
		desc += " expires=" + exp.UTC().Format(time.RFC3339)
	}
	return desc
}

// pinTokenType marks a parental-PIN proof token.
const pinTokenType = "pin"

// GeneratePINToken mints a short-lived RS256 proof that the caller entered the
// PIN of profileID. It is consumed by RefreshToken to authorise a switch out of
// a kids profile. Scoped to a single profile so a proof for one profile can't
// unlock another.
func (g *JWTGenerator) GeneratePINToken(profileID string, ttl time.Duration) (string, error) {
	if profileID == "" {
		return "", errors.New("profileID required")
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"token_type": pinTokenType,
		"profile_id": profileID,
		"iat":        now.Unix(),
		"exp":        now.Add(ttl).Unix(),
	})
	return token.SignedString(g.privateKey)
}

// ParsePINToken validates a PIN proof (signature, exp, token_type) and returns
// the profile id it was minted for. Any failure returns an error and an empty id.
func (g *JWTGenerator) ParsePINToken(raw string) (string, error) {
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return g.publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return "", err
	}
	if !parsed.Valid {
		return "", errors.New("invalid pin token")
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	if tt, _ := mc["token_type"].(string); tt != pinTokenType {
		return "", errors.New("not a pin token")
	}
	pid, _ := mc["profile_id"].(string)
	if pid == "" {
		return "", errors.New("pin token missing profile_id")
	}
	return pid, nil
}

func (g *JWTGenerator) ValidateAccessToken(tokenStr string) (*user.Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return g.publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	userID, ok1 := mapClaims["sub"].(string)
	username, ok2 := mapClaims["username"].(string)
	isAdmin, ok3 := mapClaims["is_admin"].(bool)
	if !ok1 || !ok2 || !ok3 {
		return nil, errors.New("invalid token claims")
	}
	claims := &user.Claims{
		UserID:   userID,
		Username: username,
		IsAdmin:  isAdmin,
	}
	if pid, ok := mapClaims["profile_id"].(string); ok {
		claims.ProfileID = pid
	}
	if impBy, ok := mapClaims["imp_by"].(string); ok {
		claims.ImpersonatedBy = impBy
	}
	if locale, ok := mapClaims["locale"].(string); ok {
		claims.Locale = locale
	}
	if perms, ok := mapClaims["permissions"].([]any); ok {
		for _, p := range perms {
			if s, ok := p.(string); ok {
				claims.Permissions = append(claims.Permissions, s)
			}
		}
	}
	// Tokens minted before the epoch claim existed default to 0, matching the
	// users.token_epoch column default - they stay valid until the first bump.
	if epoch, ok := mapClaims["epoch"].(float64); ok {
		claims.TokenEpoch = int64(epoch)
	}
	return claims, nil
}
