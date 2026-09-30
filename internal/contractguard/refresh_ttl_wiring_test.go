package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// EVEY_JWT_REFRESH_TTL was read into cfg.JWT.RefreshTokenTTL and never used:
// the composition root handed the user and OIDC services a literal 7 days, so
// the documented variable changed nothing. The TTL is a constructor argument,
// which the setter guard of boot_wiring_test.go cannot see.
var literalWeekRe = regexp.MustCompile(`7\s*\*\s*24\s*\*\s*time\.Hour`)

func TestRefreshTokenTTLComesFromTheConfig(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoRoot, "cmd", "evey", "wire*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no wire*.go found: %v", err)
	}
	uses := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if literalWeekRe.MatchString(line) {
				t.Errorf("%s:%d hardcodes a 7-day duration: a refresh token lifetime comes from cfg.JWT.RefreshTokenTTL (EVEY_JWT_REFRESH_TTL)", filepath.Base(f), i+1)
			}
		}
		uses += strings.Count(string(body), "cfg.JWT.RefreshTokenTTL")
	}
	// The password login and the OIDC login both issue refresh tokens.
	if uses < 2 {
		t.Errorf("cfg.JWT.RefreshTokenTTL is used %d time(s) in the composition root, want the user and the OIDC services (2)", uses)
	}
}
