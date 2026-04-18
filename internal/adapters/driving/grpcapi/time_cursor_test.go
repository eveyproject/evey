package grpcapi

import (
	"testing"
	"time"
)

func TestTimeCursor_RoundTrip(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 4, 18, 12, 34, 56, 123_456_789, time.UTC)
	enc := encodeTimeCursor(when, "row-42")
	if enc == "" {
		t.Fatal("expected non-empty cursor")
	}
	got, err := decodeTimeCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.t.Equal(when) {
		t.Errorf("timestamp mismatch: got %v want %v", got.t, when)
	}
	if got.id != "row-42" {
		t.Errorf("id mismatch: got %q want %q", got.id, "row-42")
	}
}

func TestTimeCursor_DecodeInvalid(t *testing.T) {
	t.Parallel()
	if _, err := decodeTimeCursor("!not!valid!"); err == nil {
		t.Error("expected error for invalid base64")
	}
}
