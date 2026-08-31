package fakes

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
)

type FakeTokenGenerator struct {
	mu        sync.Mutex
	tokens    map[string]*user.Claims
	pinTokens map[string]string // token → profileID
	Err       error
}

func NewFakeTokenGenerator() *FakeTokenGenerator {
	return &FakeTokenGenerator{tokens: make(map[string]*user.Claims), pinTokens: make(map[string]string)}
}

func (g *FakeTokenGenerator) GeneratePINToken(profileID string, _ time.Duration) (string, error) {
	if g.Err != nil {
		return "", g.Err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	token := randomHex(16)
	g.pinTokens[token] = profileID
	return token, nil
}

func (g *FakeTokenGenerator) ParsePINToken(raw string) (string, error) {
	if g.Err != nil {
		return "", g.Err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	pid, ok := g.pinTokens[raw]
	if !ok {
		return "", errors.New("invalid pin token")
	}
	return pid, nil
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

// DescribeRefreshToken mirrors the real generator's log helper: the fake's
// tokens carry no claims, so there is nothing to describe.
func (g *FakeTokenGenerator) DescribeRefreshToken(string) string { return "" }
