package grpcapi

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/user"
)

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
	pid, err := resolveProfileID(claims, "")
	if err != nil || pid != "p-1" {
		t.Errorf("expected p-1, got %q err=%v", pid, err)
	}
}

func TestResolveProfileID_SameProfile(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: false}
	pid, err := resolveProfileID(claims, "p-1")
	if err != nil || pid != "p-1" {
		t.Errorf("expected p-1, got %q err=%v", pid, err)
	}
}

func TestResolveProfileID_DifferentProfile_NonAdmin(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: false}
	_, err := resolveProfileID(claims, "p-2")
	if err == nil {
		t.Error("expected error for non-admin accessing different profile")
	}
}

func TestResolveProfileID_DifferentProfile_Admin(t *testing.T) {
	t.Parallel()
	claims := &user.Claims{ProfileID: "p-1", IsAdmin: true}
	pid, err := resolveProfileID(claims, "p-2")
	if err != nil || pid != "p-2" {
		t.Errorf("admin should access p-2, got %q err=%v", pid, err)
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
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeLogValue(c.in); got != c.want {
				t.Errorf("sanitizeLogValue(%q) = %q, want %q", c.in, got, c.want)
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
	// 256 'x' + ellipsis rune (3 bytes UTF-8) — assert length and suffix.
	if len(got) > 260 {
		t.Errorf("expected truncation near 256 bytes, got %d", len(got))
	}
	if got[len(got)-3:] != "…" {
		t.Errorf("expected ellipsis suffix, got %q", got[len(got)-3:])
	}
}
