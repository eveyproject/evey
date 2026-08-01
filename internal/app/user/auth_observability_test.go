package user

import (
	"context"
	"strings"
	"testing"
)

// The failed-login log is the only place a credential-stuffing run becomes
// searchable line by line - the limiter keeps an aggregate per username, not a
// timeline. Mutation testing (2026-08-01) showed nothing pinned it: both log
// lines could be deleted with the suite still green.
//
// `reason` is not decoration: `unknown_user` in bulk means a stolen list is
// being replayed (block the source), `bad_password` on few accounts means
// targeted brute force (warn those users). Same log, opposite responses.
func TestLoginFailure_EmitsSearchableWarn(t *testing.T) {
	cases := []struct {
		name       string
		seed       bool
		username   string
		password   string
		wantReason string
	}{
		{
			name:       "unknown username",
			seed:       false,
			username:   "ghost",
			password:   "hunter2",
			wantReason: "reason=unknown_user",
		},
		{
			name:       "wrong password on a real account",
			seed:       true,
			username:   "alice",
			password:   "wrong-one",
			wantReason: "reason=bad_password",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No t.Parallel: captureSlogBuffer swaps the global logger.
			buf, restore := captureSlogBuffer(t)
			defer restore()

			d := newAuthTestDeps()
			if tc.seed {
				seedAuthUser(t, d, "u1", tc.username, "correct-horse", false, true)
			}

			if _, _, err := d.svc.Login(context.Background(), tc.username, tc.password); err == nil {
				t.Fatal("this login was supposed to fail")
			}

			out := buf.String()
			if !strings.Contains(out, "login failed") {
				t.Fatalf("no 'login failed' log: a stuffing campaign leaves no trace\nlogs:\n%s", out)
			}
			if !strings.Contains(out, "username="+tc.username) {
				t.Fatalf("the log does not name the targeted account, so a broad sweep "+
					"cannot be told from a targeted attack\nlogs:\n%s", out)
			}
			if !strings.Contains(out, tc.wantReason) {
				t.Fatalf("missing %q: without it, 'no such account' and 'wrong password' "+
					"look alike while calling for opposite responses\nlogs:\n%s", tc.wantReason, out)
			}
			// Failure traces must never become a password dictionary - these are
			// often real passwords from other services, and logs are read by more
			// people than the database is.
			if strings.Contains(out, tc.password) {
				t.Fatalf("the submitted password leaked into the logs\nlogs:\n%s", out)
			}
		})
	}
}
