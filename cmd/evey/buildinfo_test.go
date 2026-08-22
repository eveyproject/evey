package main

import (
	"testing"
	"time"
)

// A blank stamp must never reach an API surface: "" reads as "field not
// implemented" to a client, which is the exact ambiguity this mechanism
// removes. It has to collapse to the explicit unknownStamp instead.
func TestNormalizeStampRejectsBlank(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if got := normalizeStamp(in); got != unknownStamp {
			t.Fatalf("normalizeStamp(%q) = %q, want %q", in, got, unknownStamp)
		}
	}
	if got := normalizeStamp(" abc123 "); got != "abc123" {
		t.Fatalf("normalizeStamp trimmed value = %q, want %q", got, "abc123")
	}
}

// resolveBuildIdentity must never invent a version: an empty ldflags value
// falls back to "dev", not to "".
func TestResolveBuildIdentityNeverBlank(t *testing.T) {
	origV, origC, origD := version, commit, buildDate
	t.Cleanup(func() { version, commit, buildDate = origV, origC, origD })

	version, commit, buildDate = "", "", ""
	id := resolveBuildIdentity()
	if id.Version == "" || id.Commit == "" || id.BuildDate == "" {
		t.Fatalf("resolveBuildIdentity returned a blank field: %+v", id)
	}
	if id.Version != "dev" {
		t.Fatalf("Version = %q, want %q", id.Version, "dev")
	}
	// Commit/BuildDate may legitimately come from the toolchain's VCS stamp
	// when the test binary is built inside a checkout; the contract is only
	// that they are never blank.
}

func TestResolveBuildIdentityPrefersLdflags(t *testing.T) {
	origV, origC, origD := version, commit, buildDate
	t.Cleanup(func() { version, commit, buildDate = origV, origC, origD })

	version, commit, buildDate = "beta-0.0.1", "abcdef1234567890", "2026-08-22T09:00:00Z"
	id := resolveBuildIdentity()
	if id.Commit != "abcdef1234567890" || id.BuildDate != "2026-08-22T09:00:00Z" {
		t.Fatalf("ldflags stamps not honoured: %+v", id)
	}
}

func TestShortCommit(t *testing.T) {
	if got := shortCommit("abcdef1234567890"); got != "abcdef1" {
		t.Fatalf("shortCommit = %q, want %q", got, "abcdef1")
	}
	if got := shortCommit(unknownStamp); got != unknownStamp {
		t.Fatalf("shortCommit(%q) = %q, want it untouched", unknownStamp, got)
	}
}

func TestBuildTimeAge(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	if got := buildTimeAge("2026-08-22T09:00:00Z", now); got != 3*time.Hour {
		t.Fatalf("buildTimeAge = %v, want 3h", got)
	}
	if got := buildTimeAge(unknownStamp, now); got != 0 {
		t.Fatalf("buildTimeAge(unknown) = %v, want 0", got)
	}
}
