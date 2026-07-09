package main

import (
	"context"
	"errors"
	"testing"
)

type fakeEpochRevoker struct {
	called []string
	err    error
}

func (f *fakeEpochRevoker) Revoke(_ context.Context, userID string) error {
	f.called = append(f.called, userID)
	return f.err
}

// A security event must bump the JWT epoch AND purge the user's stream tokens,
// epoch first (so the fast JWT revocation is never gated behind the stream
// purge). Closes the gap where a banned user's 12h cast token survived because
// only the epoch was bumped.
func TestCompositeTokenRevoker_RevokesBothLegs(t *testing.T) {
	epoch := &fakeEpochRevoker{}
	var purged []string
	order := []string{}
	rev := &compositeTokenRevoker{
		epoch: &orderingEpoch{epoch, &order},
		purgeStreams: func(userID string) int {
			order = append(order, "purge")
			purged = append(purged, userID)
			return 1
		},
	}

	if err := rev.Revoke(context.Background(), "usr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epoch.called) != 1 || epoch.called[0] != "usr-1" {
		t.Fatalf("epoch revoke not called with the userID: %v", epoch.called)
	}
	if len(purged) != 1 || purged[0] != "usr-1" {
		t.Fatalf("stream purge not called with the userID: %v", purged)
	}
	if len(order) != 2 || order[0] != "epoch" || order[1] != "purge" {
		t.Fatalf("epoch must run before the stream purge, got %v", order)
	}
}

// A nil purge (before the stream store is bound) preserves the epoch-only
// behaviour instead of panicking.
func TestCompositeTokenRevoker_NilPurgeIsSafe(t *testing.T) {
	epoch := &fakeEpochRevoker{}
	rev := &compositeTokenRevoker{epoch: epoch}
	if err := rev.Revoke(context.Background(), "usr-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epoch.called) != 1 {
		t.Fatal("epoch revoke must still fire when purge is unbound")
	}
}

// An epoch failure must abort before the stream purge (JWT kill is the
// authoritative revocation; a partial success that skipped it would be worse).
func TestCompositeTokenRevoker_EpochErrorAborts(t *testing.T) {
	epoch := &fakeEpochRevoker{err: errors.New("db down")}
	purged := false
	rev := &compositeTokenRevoker{
		epoch:        epoch,
		purgeStreams: func(string) int { purged = true; return 0 },
	}
	if err := rev.Revoke(context.Background(), "usr-1"); err == nil {
		t.Fatal("expected the epoch error to propagate")
	}
	if purged {
		t.Fatal("stream purge must not run when the epoch bump failed")
	}
}

type orderingEpoch struct {
	inner *fakeEpochRevoker
	order *[]string
}

func (o *orderingEpoch) Revoke(ctx context.Context, userID string) error {
	*o.order = append(*o.order, "epoch")
	return o.inner.Revoke(ctx, userID)
}
