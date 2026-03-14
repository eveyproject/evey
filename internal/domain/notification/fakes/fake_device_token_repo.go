package fakes

import (
	"context"
	"sync"
)

type tokenEntry struct {
	userID    string
	profileID string
	token     string
	platform  string
}

type FakeDeviceTokenRepository struct {
	mu  sync.Mutex
	entries []tokenEntry
	Err error
}

func NewFakeDeviceTokenRepository() *FakeDeviceTokenRepository {
	return &FakeDeviceTokenRepository{}
}

func (r *FakeDeviceTokenRepository) Upsert(_ context.Context, userID, profileID, token, platform string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.token == token {
			r.entries[i] = tokenEntry{userID: userID, profileID: profileID, token: token, platform: platform}
			return nil
		}
	}
	r.entries = append(r.entries, tokenEntry{userID: userID, profileID: profileID, token: token, platform: platform})
	return nil
}

func (r *FakeDeviceTokenRepository) Delete(_ context.Context, userID, token string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.entries[:0]
	for _, e := range r.entries {
		if e.token != token || e.userID != userID {
			out = append(out, e)
		}
	}
	r.entries = out
	return nil
}

func (r *FakeDeviceTokenRepository) GetByProfile(_ context.Context, profileID string) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var tokens []string
	for _, e := range r.entries {
		if e.profileID == profileID {
			tokens = append(tokens, e.token)
		}
	}
	return tokens, nil
}

func (r *FakeDeviceTokenRepository) GetByUser(_ context.Context, userID string) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var tokens []string
	for _, e := range r.entries {
		if e.userID == userID {
			tokens = append(tokens, e.token)
		}
	}
	return tokens, nil
}
