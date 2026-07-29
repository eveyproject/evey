package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The translation catalogue must NEVER be rate-limited.
//
// It is the one endpoint whose failure makes the whole UI unreadable: every
// screen falls back to raw key names ("auth.login.signIn"), which is exactly
// what a beta tester saw on 2026-07-29. A user who trips a limiter elsewhere
// (retrying a login, say) must still get an app they can READ - the refusal
// has to be a legible sentence, not a broken interface.
//
// Static guard (no dependency graph to build): it reads the registration line
// and fails if a limiter is ever wrapped around it.
func TestI18nRouteIsNeverRateLimited(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	re := regexp.MustCompile(`(?m)^.*HandleFunc\("GET /api/v1/i18n/\{locale\}".*$`)
	line := re.FindString(string(src))
	if line == "" {
		t.Fatal("the i18n route registration was not found - if it moved, move this guard with it")
	}
	for _, forbidden := range []string{"ratelimit.", "RL,", "RL)", "Middleware"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("the i18n route is wrapped in a limiter (%q in %q): a throttled catalogue means an unreadable app", forbidden, strings.TrimSpace(line))
		}
	}
}

// /healthz is how every client decides whether the server is reachable at all
// (server-connect, the boot redirect, the saved-server switch). Throttling it
// would make a healthy instance look dead and strand users on the connect
// screen - the opposite of the guard those probes exist for.
func TestHealthzIsNeverRateLimited(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	re := regexp.MustCompile(`(?m)^.*HandleFunc\("/healthz".*$`)
	line := re.FindString(string(src))
	if line == "" {
		t.Fatal("the healthz route registration was not found - if it moved, move this guard with it")
	}
	for _, forbidden := range []string{"ratelimit.", "RL,", "RL)", "Middleware"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("healthz is wrapped in a limiter (%q): reachability probes would report a healthy server as dead", forbidden)
		}
	}
}
