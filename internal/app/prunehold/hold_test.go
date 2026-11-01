package prunehold_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/app/prunehold"
)

func keys(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return out
}

func TestSnapshot_OrderFree(t *testing.T) {
	if prunehold.Snapshot([]string{"a", "b"}) != prunehold.Snapshot([]string{"b", "a"}) {
		t.Fatal("snapshot depends on order")
	}
	if prunehold.Snapshot([]string{"a", "b"}) == prunehold.Snapshot([]string{"a", "c"}) {
		t.Fatal("two sets share a snapshot")
	}
}

func TestJudge_ConfirmationExpires(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	h := prunehold.New(nil).WithClock(func() time.Time { return now }, time.Minute)
	ctx := context.Background()
	missing := keys("x", 40)
	if h.Judge(ctx, "lib", "/pool", 100, missing) {
		t.Fatal("40 % must be held")
	}
	if err := h.Confirm("lib", "/pool", h.Suspensions("lib")[0].Snapshot); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if h.Judge(ctx, "lib", "/pool", 100, missing) {
		t.Fatal("an expired confirmation must not allow the deletion")
	}
}

func TestJudge_ConfirmationUsedByOnePassOnly(t *testing.T) {
	h := prunehold.New(nil)
	ctx := context.Background()
	missing := keys("x", 40)
	h.Judge(ctx, "lib", "/pool", 100, missing)
	if err := h.Confirm("lib", "/pool", h.Suspensions("lib")[0].Snapshot); err != nil {
		t.Fatal(err)
	}
	// The pass sees another set: held, and the confirmation is gone.
	h.Judge(ctx, "lib", "/pool", 100, keys("y", 40))
	if h.Judge(ctx, "lib", "/pool", 100, missing) {
		t.Fatal("a confirmation must serve one pass at most")
	}
}

func TestAbandon_DropsPendingConfirmation(t *testing.T) {
	h := prunehold.New(nil)
	ctx := context.Background()
	missing := keys("x", 40)
	h.Judge(ctx, "lib", "/pool", 100, missing)
	if err := h.Confirm("lib", "/pool/", h.Suspensions("lib")[0].Snapshot); err != nil {
		t.Fatal(err)
	}
	h.Abandon("lib", "/pool")
	if h.Judge(ctx, "lib", "/pool", 100, missing) {
		t.Fatal("an abandoned pass must drop the confirmation")
	}
}

func TestConfirm_UnknownRoot(t *testing.T) {
	h := prunehold.New(nil)
	h.Judge(context.Background(), "lib", "/a", 100, keys("x", 40))
	if err := h.Confirm("lib", "/b", "whatever"); !errors.Is(err, prunehold.ErrNothingSuspended) {
		t.Fatalf("want ErrNothingSuspended, got %v", err)
	}
}
