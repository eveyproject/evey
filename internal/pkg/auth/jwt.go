package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/id"
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
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":         claims.UserID,
		"username":    claims.Username,
		"is_admin":    claims.IsAdmin,
		"profile_id":  claims.ProfileID,
		"locale":      claims.Locale,
		"permissions": claims.Permissions,
		"iat":         now.Unix(),
		"exp":         now.Add(g.accessTTL).Unix(),
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
	return claims, nil
}
