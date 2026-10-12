package main

import (
	"context"
	"errors"
	"testing"
	"time"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
)

type countingSettings struct {
	calls int
	value string
	err   error
}

func (c *countingSettings) Get(_ context.Context, key string) (*domainshared.ServerConfigEntry, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return &domainshared.ServerConfigEntry{Key: key, Value: c.value}, nil
}

func TestCachedBoolSetting_AsksTheStoreOncePerTTL(t *testing.T) {
	repo := &countingSettings{value: "true"}
	get, stop := cachedBoolSetting(repo, "transcode.audio_normalize", 50*time.Millisecond)
	defer stop()
	ctx := context.Background()

	for range 10 {
		if !get(ctx) {
			t.Fatal("expected true")
		}
	}
	if repo.calls != 1 {
		t.Fatalf("ten reads inside the TTL hit the store %d times, want 1", repo.calls)
	}

	repo.value = "false"
	time.Sleep(70 * time.Millisecond)
	if get(ctx) {
		t.Fatal("after the TTL the toggle flipped off must be seen")
	}
	if repo.calls != 2 {
		t.Fatalf("one read past the TTL hit the store %d times, want 2", repo.calls)
	}
}

func TestCachedBoolSetting_DoesNotCacheAnError(t *testing.T) {
	repo := &countingSettings{err: errors.New("db down")}
	get, stop := cachedBoolSetting(repo, "transcode.audio_normalize", time.Minute)
	defer stop()
	ctx := context.Background()

	first, second := get(ctx), get(ctx)
	if first || second {
		t.Fatal("an unreadable knob reads as off")
	}
	if repo.calls != 2 {
		t.Fatalf("an error must not be cached: store asked %d times, want 2", repo.calls)
	}
	repo.err = nil
	repo.value = "true"
	if !get(ctx) {
		t.Fatal("the first successful read is served")
	}
}
