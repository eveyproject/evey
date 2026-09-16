package main

import "testing"

// The QoE row keeps NULL for "not reported": an empty id, a zero counter and
// a zero rate must not be stored as 0 (which the dashboard would average),
// and the segment index only exists for a media segment - never for an init
// or a manifest, whose 0 would read as "segment 0".
func TestQoEStorageAdapter_NullMapping(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Error("empty id must be NULL")
	}
	if got := nilIfEmpty("u1"); got == nil || *got != "u1" {
		t.Error("a set id round-trips")
	}
	if intPtrIfNonZero(0) != nil || floatPtrIfNonZero(0) != nil {
		t.Error("a zero measurement is not reported")
	}
	if got := intPtrIfNonZero(-3); got == nil || *got != -3 {
		t.Error("a negative value is still a value")
	}
	if got := floatPtrIfNonZero(1.5); got == nil || *got != 1.5 {
		t.Error("a rate round-trips")
	}
	if intPtrIf(0, false) != nil {
		t.Error("no segment index for an init or a manifest")
	}
	if got := intPtrIf(0, true); got == nil || *got != 0 {
		t.Error("segment 0 of a media request is a real index")
	}
}
