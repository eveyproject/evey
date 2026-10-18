package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/user"
)

func TestIsClientCanceled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"context canceled", context.Canceled, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped context canceled", fmt.Errorf("query failed: %w", context.Canceled), true},
		{"pg 57014 query canceled", errors.New("pq: canceling statement due to user request (57014)"), true},
		{"genuine server error", errors.New("connection refused"), false},
		{"unrelated constraint error", errors.New("constraint violation"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isClientCanceled(tc.err); got != tc.want {
				t.Errorf("isClientCanceled(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSecureToken(t *testing.T) {
	t.Parallel()
	tok := secureToken("stk_", 16)
	if len(tok) != 4+32 { // prefix + 16 bytes hex
		t.Errorf("expected length 36, got %d: %s", len(tok), tok)
	}
	if tok[:4] != "stk_" {
		t.Errorf("expected prefix stk_, got %s", tok[:4])
	}
	tok2 := secureToken("stk_", 16)
	if tok == tok2 {
		t.Error("tokens should be different")
	}
}

func TestRequireClaims_NoClaims(t *testing.T) {
	t.Parallel()
	ctx := ctxWithClaims("u-1", "p-1", false)
	claims, err := requireClaims(ctx)
	if err != nil {
		t.Fatalf("requireClaims: %v", err)
	}
	if claims.UserID != "u-1" {
		t.Errorf("UserID = %q, want u-1", claims.UserID)
	}
}

func TestResolveProfileID_Empty(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: false}
	pid, err := resolveProfileID(context.Background(), claims, "")
	if err != nil || pid != "p-1" {
		t.Errorf("expected p-1, got %q err=%v", pid, err)
	}
}

func TestResolveProfileID_SameProfile(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: false}
	pid, err := resolveProfileID(context.Background(), claims, "p-1")
	if err != nil || pid != "p-1" {
		t.Errorf("expected p-1, got %q err=%v", pid, err)
	}
}

func TestResolveProfileID_DifferentProfile_NonAdmin(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: false}
	_, err := resolveProfileID(context.Background(), claims, "p-2")
	if err == nil {
		t.Error("expected error for non-admin accessing different profile")
	}
}

func TestResolveProfileID_DifferentProfile_Admin(t *testing.T) {
	t.Parallel()
	// Admins are NOT special-cased: a foreign profile_id on a normal request is
	// denied even for an admin. Cross-profile admin access goes through the
	// explicit ImpersonateUser RPC (which mints a token whose ProfileID is the
	// target, so resolveProfileID then sees reqProfileID == claims.ProfileID).
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: true}
	if _, err := resolveProfileID(context.Background(), claims, "p-2"); err == nil {
		t.Error("expected admin to be denied a foreign profile_id (must impersonate instead)")
	}
}

func TestSanitizeLogValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "alice", "alice"},
		{"crlf", "alice\r\nINFO fake log line", `alice\r\nINFO fake log line`},
		{"lone_lf", "a\nb", `a\nb`},
		{"lone_cr", "a\rb", `a\rb`},
		{"tab", "a\tb", `a\tb`},
		{"ansi_reset", "a\x1b[0mb", `a\x1b[0mb`},
		{"nul", "a\x00b", `a\x00b`},
		{"unicode_preserved", "élève_中文", "élève_中文"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeLogValue(c.in); got != c.want {
				t.Errorf("sanitizeLogValue(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestClampLimitOffset(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		limit        int32
		offset       int32
		defaultLimit int
		wantLimit    int
		wantOffset   int
	}{
		{"valid values unchanged", 20, 5, 50, 20, 5},
		{"zero limit → default", 0, 0, 50, 50, 0},
		{"negative limit → default", -10, 0, 50, 50, 0},
		{"limit at boundary", maxPageSize, 0, 50, maxPageSize, 0},
		{"limit above maxPageSize → default", maxPageSize + 1, 0, 50, 50, 0},
		{"huge limit → default", 1_000_000, 0, 50, 50, 0},
		{"negative offset → 0", 10, -1, 50, 10, 0},
		{"very negative offset → 0", 10, -9999, 50, 10, 0},
		{"zero offset unchanged", 10, 0, 50, 10, 0},
		{"positive offset unchanged", 10, 42, 50, 10, 42},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			gotLimit, gotOffset := clampLimitOffset(c.limit, c.offset, c.defaultLimit)
			if gotLimit != c.wantLimit {
				t.Errorf("limit: got %d, want %d", gotLimit, c.wantLimit)
			}
			if gotOffset != c.wantOffset {
				t.Errorf("offset: got %d, want %d", gotOffset, c.wantOffset)
			}
		})
	}
}

func TestSanitizeLogValue_TruncatesLongInputs(t *testing.T) {
	t.Parallel()
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	got := sanitizeLogValue(string(long))
	// 256 'x' + ellipsis rune (3 bytes UTF-8) - assert length and suffix.
	if len(got) > 260 {
		t.Errorf("expected truncation near 256 bytes, got %d", len(got))
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("expected ellipsis suffix, got %q", got[len(got)-3:])
	}
}

// An audit entry's profile must be the one that ACTED. When an admin acts on
// another account (impersonate, disable, delete), the entry is attributed to
// the admin's user id, and stamping the admin's profile there would be right -
// but stamping it on an entry attributed to the TARGET would name a profile
// that did nothing. auditProfileID answers only for the caller's own account.
func TestAuditProfileID(t *testing.T) {
	callerCtx := interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID: "usr-1", ProfileID: "prf-1",
	})

	if got := auditProfileID(callerCtx, "usr-1"); got != "prf-1" {
		t.Fatalf("own account: got %q, want prf-1", got)
	}
	if got := auditProfileID(callerCtx, "usr-2"); got != "" {
		t.Fatalf("other account: got %q, want empty", got)
	}
	if got := auditProfileID(context.Background(), "usr-1"); got != "" {
		t.Fatalf("no claims: got %q, want empty", got)
	}
}
