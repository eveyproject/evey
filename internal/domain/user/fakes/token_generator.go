package fakes

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/evey-project/evey/internal/domain/user"
)

type FakeTokenGenerator struct {
	mu     sync.Mutex
	tokens map[string]*user.Claims
	Err    error
}

func NewFakeTokenGenerator() *FakeTokenGenerator {
	return &FakeTokenGenerator{tokens: make(map[string]*user.Claims)}
}

func (g *FakeTokenGenerator) GenerateAccessToken(claims user.Claims) (string, error) {
	if g.Err != nil {
		return "", g.Err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	token := randomHex(32)
	g.tokens[token] = &claims
	return token, nil
}

func (g *FakeTokenGenerator) GenerateRefreshToken() (string, error) {
	if g.Err != nil {
		return "", g.Err
	}
	return randomHex(32), nil
}

func (g *FakeTokenGenerator) ValidateAccessToken(token string) (*user.Claims, error) {
	if g.Err != nil {
		return nil, g.Err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	claims, ok := g.tokens[token]
	if !ok {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
