package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/id"
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
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing algorithm: %s", t.Method.Alg())
		}
		return g.publicKey, nil
	})
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

	claims := &user.Claims{
		UserID:   mapClaims["sub"].(string),
		Username: mapClaims["username"].(string),
		IsAdmin:  mapClaims["is_admin"].(bool),
	}
	if pid, ok := mapClaims["profile_id"].(string); ok {
		claims.ProfileID = pid
	}
	if perms, ok := mapClaims["permissions"].([]interface{}); ok {
		for _, p := range perms {
			if s, ok := p.(string); ok {
				claims.Permissions = append(claims.Permissions, s)
			}
		}
	}
	return claims, nil
}
