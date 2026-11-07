package user

import (
	"context"
	"errors"
	"testing"
)

// A refresh the store cannot answer is not a refusal. Every failure of the
// hash lookup used to read as "token not found", so a database away or slow
// for longer than the access token's grace logged every device of the
// household out at once - and, the refusal having cleared their refresh
// tokens, kept them out once the database was back.
func TestRefreshToken_StoreUnavailable_IsNotARefusal(t *testing.T) {
	d := newAuthTestDeps()
	storeErr := errors.New("dial tcp: connection refused")
	d.refreshTkns.Err = storeErr

	_, _, err := d.svc.RefreshToken(context.Background(), "whatever-the-device-holds", "", "")
	if err == nil {
		t.Fatal("a refresh against a store that cannot answer must fail")
	}
	if errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a store failure must not read as a refusal: %v", err)
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("the store's own error must be kept for the logs: %v", err)
	}
}

// The symmetric case keeps its verdict: a token the store does know nothing
// about is refused, and only that.
func TestRefreshToken_UnknownToken_IsRefused(t *testing.T) {
	d := newAuthTestDeps()
	_, _, err := d.svc.RefreshToken(context.Background(), "never-issued", "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("an unknown token is a refusal, got %v", err)
	}
}

// A browser whose refresh cookie is gone presents nothing at all. That is a
// refusal decided without the store: with the store away, the verdict is
// still the refusal, which proves no lookup was made for an empty token.
func TestRefreshToken_NoToken_IsRefusedWithoutALookup(t *testing.T) {
	d := newAuthTestDeps()
	d.refreshTkns.Err = errors.New("dial tcp: connection refused")

	_, _, err := d.svc.RefreshToken(context.Background(), "", "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("an absent token is a refusal before any lookup, got %v", err)
	}
}
