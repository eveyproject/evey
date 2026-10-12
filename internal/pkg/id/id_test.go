package id_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/pkg/id"
)

func TestNew_HasPrefix(t *testing.T) {
	got := id.New("plg-")
	if got[:4] != "plg-" {
		t.Fatalf("expected prefix plg-, got %s", got)
	}
}

func TestNew_UniqueIDs(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		v := id.New("x-")
		if _, dup := seen[v]; dup {
			t.Fatalf("duplicate ID: %s", v)
		}
		seen[v] = struct{}{}
	}
}

func TestNew_EmptyPrefix(t *testing.T) {
	got := id.New("")
	if got == "" {
		t.Fatal("expected non-empty ID")
	}
	if len(got) != 24 { // 12 bytes = 24 hex chars
		t.Fatalf("expected 24 chars, got %d", len(got))
	}
}

func TestNew_Length(t *testing.T) {
	got := id.New("usr-")
	// 4 prefix + 24 hex = 28
	if len(got) != 28 {
		t.Fatalf("expected length 28, got %d (%s)", len(got), got)
	}
}
