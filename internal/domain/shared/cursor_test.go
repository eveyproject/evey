package shared_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
)

func TestCursor_RoundTrip(t *testing.T) {
	t.Parallel()
	c := shared.Cursor{SortField: "title", SortValue: "Some Movie", LastID: "m-42"}
	enc := shared.EncodeCursor(c)
	if enc == "" {
		t.Fatal("non-empty cursor must encode to non-empty string")
	}
	got, err := shared.DecodeCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != c {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, c)
	}
}

func TestCursor_EmptyEncodesEmpty(t *testing.T) {
	t.Parallel()
	if got := shared.EncodeCursor(shared.Cursor{}); got != "" {
		t.Errorf("empty cursor must encode to empty string, got %q", got)
	}
}

func TestCursor_DecodeEmpty(t *testing.T) {
	t.Parallel()
	c, err := shared.DecodeCursor("")
	if err != nil {
		t.Fatalf("empty input must not error: %v", err)
	}
	if c != (shared.Cursor{}) {
		t.Errorf("empty input must decode to zero Cursor, got %+v", c)
	}
}

func TestCursor_DecodeInvalid(t *testing.T) {
	t.Parallel()
	if _, err := shared.DecodeCursor("not!base64!"); err == nil {
		t.Error("invalid base64 must return error")
	}
	// valid base64, invalid JSON
	if _, err := shared.DecodeCursor("bm90anNvbg"); err == nil {
		t.Error("invalid JSON must return error")
	}
}
