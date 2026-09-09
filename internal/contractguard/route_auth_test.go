package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Route-auth gate: every route registered on the HTTP mux must have its access
// decision made in ONE of three places, and this file is the only list of which
// is which.
//
// The gRPC surface already has this (interceptors.TestAllRegisteredMethodsAreClassified
// walks the proto descriptors and refuses an unclassified RPC). The HTTP mux had
// nothing: 145 routes spread over 22 registerXxxRoutes functions plus a long
// RegisterRoutes, where the control is a POSITIONAL WRAPPER around the handler.
// Forgetting it is not a compile error and not a test failure - the handler just
// serves. And "this one is deliberately public" currently lives in a comment next
// to each route ("Artwork is public (non-sensitive)", "Avatars are public",
// "theme.css is public"), three files apart, so nobody can read the public
// surface in one place or notice a fourth entry joining it.
//
// The rule: a route is gated when its handler expression contains a call to one
// of this file's auth middlewares. Otherwise it must appear below, with a reason.
//
// Honest limits, in the spirit of the other guards here:
//   - only routes whose pattern is a string literal, registered on an identifier
//     named `mux` (all 145 are, today);
//   - alias resolution reads `x := httpAuthMiddleware(...)` only. A middleware
//     held in a `var` declaration, or received as a function PARAMETER, would
//     read as ungated and fail with the misleading "route unclassified" - none
//     exists today, and the floors below fire first if that changes wholesale;
//   - "the handler is wrapped in auth()" is not "the right identity is checked" -
//     per-route authorization (library access, parental, ownership) stays the job
//     of the per-handler tests;
//   - gatedInHandler only records that a route was reviewed and gates itself, not
//     that it gates itself correctly.

// authMiddlewareCtors are the constructors that produce an auth wrapper. Aliases
// are DERIVED from assignments of these (authMw := httpAuthMiddleware(tokenGen)),
// per file, rather than hardcoded: a new local name for the same middleware must
// not need an edit here. The eight in the tree today - auth, authMw, authMW,
// admin, adminMw, streamAuth, streamAuthMw, scrapeOrAdmin - all resolve this way.
var authMiddlewareCtors = map[string]bool{
	"httpAuthMiddleware":          true,
	"httpAdminMiddleware":         true,
	"httpStreamAuthMiddleware":    true,
	"httpScrapeOrAdminMiddleware": true,
}

// publicRoutes are served WITHOUT authentication on purpose. Adding a line here
// is the decision to widen the unauthenticated surface of the server - it should
// read like one.
var publicRoutes = map[string]string{
	"/healthz":                  "server discovery for mobile+TV; gating it breaks onboarding on both",
	"GET /api/v1/i18n/{locale}": "UI strings, needed before login",
	"GET /api/v1/media/{id}/artwork/{artType}":     "posters, deliberately public; ids are uuidv4 so not enumerable. NOTE: this is the one route parentalChk does not cover",
	"GET /api/v1/profiles/{profileId}/avatar":      "avatars, deliberately public; the POST on the same path IS gated",
	"GET /api/v1/themes/{id}/theme.css":            "CSS must apply to the login page, i.e. before there is a token (theme.js, which executes, IS gated)",
	"GET /api/v1/share/{token}/download":           "the share token in the path IS the credential",
	"POST /api/v1/auth/login":                      "issues the credential",
	"POST /api/v1/auth/refresh":                    "the refresh cookie/body token is the credential",
	"POST /api/v1/auth/logout":                     "clears the refresh cookie; must work on an expired session",
	"POST /api/v1/auth/forgot-password":            "pre-auth by definition",
	"POST /api/v1/auth/reset-password":             "the reset token is the credential",
	"GET /api/v1/auth/quick-connect/{code}":        "pre-auth TV pairing; the code is the credential",
	"GET /api/v1/auth/quick-connect/{code}/events": "SSE half of the same pairing flow",
	"GET /api/v1/auth/oidc/config":                 "discovery: tells the login page whether SSO exists",
	"GET /api/v1/auth/oidc/login":                  "starts the SSO redirect, pre-auth",
	"GET /api/v1/auth/oidc/callback":               "IdP redirect target, pre-auth",
	"GET /api/v1/auth/oidc/session":                "exchanges the post-callback one-shot for tokens",
	"POST /api/v1/syncplay/join-code":              "the room code is the credential (guest join)",
	"/":                                            "SPA files and the grpc-gateway mux; the gateway enforces its own RBAC interceptor",
}

// gatedInHandler are routes that DO authenticate, but inside the handler rather
// than through a mux wrapper - so this gate cannot see it. Each entry says what
// does the checking, and that name is what a reviewer greps for.
var gatedInHandler = map[string]string{
	"POST /api/v1/admin/media/{id}/detect-segments":    "introDetectionHandler builds httpAdminMiddleware itself",
	"POST /api/v1/admin/libraries/{id}/detect-intros":  "libraryIntroBackfillHandler builds httpAdminMiddleware itself",
	"PUT /api/v1/admin/media/{id}/segments":            "setSegmentHandler builds httpAdminMiddleware itself",
	"DELETE /api/v1/admin/media/{id}/segments":         "deleteSegmentHandler builds httpAdminMiddleware itself",
	"POST /api/v1/admin/media/{id}/segments/propagate": "propagateSegmentHandler builds httpAdminMiddleware itself",
	"GET /api/v1/plugins/{id}/assets/{path...}":        "pluginAssetHandler.authorizeFrontJS: accepts a user token OR a plugin-scoped token pinned to {id}",
	"/api/v1/devices/ws":                               "WebSocket: the token rides Sec-WebSocket-Protocol, which no HandlerFunc wrapper can read",
	"/api/v1/syncplay/rooms/{id}/ws":                   "same, plus guest room tokens",
	"GET /api/v1/media/{id}/subtitles/search":          "subtitleRoutes.requireAuth (hand-rolled; 403 instead of 401, no auth metrics)",
	"POST /api/v1/media/{id}/subtitles/download":       "subtitleRoutes.requireAuth (hand-rolled; 403 instead of 401, no auth metrics)",
	"GET /api/v1/media/{id}/subtitles":                 "subtitleRoutes.requireAuth (hand-rolled; 403 instead of 401, no auth metrics)",
	"DELETE /api/v1/media/{id}/subtitles/{subtitleId}": "subtitleRoutes.requireAuth (hand-rolled; 403 instead of 401, no auth metrics)",
}

// Registering the same pattern twice PANICS http.ServeMux at boot - a server
// that does not come up, not a 4xx. These three are exclusive if/else branches.
var allowedDuplicateRoutes = map[string]string{
	"/":                            "spaHandler vs the bare gateway handler, `if webDirOK {} else {}` in RegisterRoutes",
	"GET /api/v1/auth/oidc/config": "registerOIDCRoutes vs the {\"enabled\":false} stub, `if s.OIDCSvc != nil {} else {}`",
	"GET /api/v1/auth/oidc/login":  "rate-limited vs bare, `if len(oidcLoginRL) > 0 && oidcLoginRL[0] != nil {} else {}`",
}

// Floors that guard the parser itself: if a refactor changes how routes are
// registered, this test must fail loudly rather than pass on zero findings.
// Today: 145 routes, 111 of them resolving to an auth wrapper.
const (
	routeAuthMinRoutes = 100
	routeAuthMinGated  = 80
)

type registeredRoute struct {
	pattern string
	file    string
	line    int
	gated   bool
}

func collectRoutes(t *testing.T) []registeredRoute {
	t.Helper()
	dir := filepath.Join(repoRoot, "internal/adapters/driving/http")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read http adapter dir: %v", err)
	}

	var out []registeredRoute
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}

		aliases := authAliases(f)

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle" {
				return true
			}
			if id, isIdent := sel.X.(*ast.Ident); !isIdent || id.Name != "mux" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true // non-literal pattern: out of scope, see "Honest limits"
			}
			out = append(out, registeredRoute{
				pattern: strings.Trim(lit.Value, `"`),
				file:    e.Name(),
				line:    fset.Position(lit.Pos()).Line,
				gated:   handlerIsGated(call.Args[1], aliases),
			})
			return true
		})
	}
	return out
}

// authAliases returns the local identifiers in f that hold an auth middleware,
// i.e. anything assigned from a call to one of authMiddlewareCtors.
func authAliases(f *ast.File) map[string]bool {
	aliases := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || !authMiddlewareCtors[id.Name] {
				continue
			}
			if i < len(assign.Lhs) {
				if lhs, isIdent := assign.Lhs[i].(*ast.Ident); isIdent {
					aliases[lhs.Name] = true
				}
			}
		}
		return true
	})
	return aliases
}

// handlerIsGated reports whether expr contains a call to an auth wrapper - either
// a file-local alias or one of the constructors used inline.
func handlerIsGated(expr ast.Expr, aliases map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, isIdent := call.Fun.(*ast.Ident); isIdent {
			if aliases[id.Name] || authMiddlewareCtors[id.Name] {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// TestEveryHTTPRouteIsClassified fails when a route is neither wrapped in an auth
// middleware nor listed above. A new route ships gated, public-by-decision, or
// not at all.
func TestEveryHTTPRouteIsClassified(t *testing.T) {
	routes := collectRoutes(t)

	if len(routes) < routeAuthMinRoutes {
		t.Fatalf("found %d routes, want at least %d - the parser has drifted from how routes are registered",
			len(routes), routeAuthMinRoutes)
	}
	var gated int
	for _, r := range routes {
		if r.gated {
			gated++
		}
	}
	if gated < routeAuthMinGated {
		t.Fatalf("only %d/%d routes resolved to an auth wrapper, want at least %d - alias resolution has drifted",
			gated, len(routes), routeAuthMinGated)
	}

	for _, r := range routes {
		if r.gated {
			if _, isPublic := publicRoutes[r.pattern]; isPublic {
				t.Errorf("%s:%d: route %q is wrapped in an auth middleware but is listed in publicRoutes - "+
					"remove the entry, or the list stops describing the unauthenticated surface",
					r.file, r.line, r.pattern)
			}
			continue
		}
		_, isPublic := publicRoutes[r.pattern]
		_, inHandler := gatedInHandler[r.pattern]
		switch {
		case isPublic && inHandler:
			t.Errorf("%s:%d: route %q is in BOTH publicRoutes and gatedInHandler - it cannot be both",
				r.file, r.line, r.pattern)
		case !isPublic && !inHandler:
			t.Errorf("%s:%d: route %q has no auth wrapper and is unclassified.\n"+
				"  Wrap it (auth/streamAuth/admin), or add it to publicRoutes (with the reason it is safe "+
				"to serve unauthenticated) or gatedInHandler (naming what checks the token inside).",
				r.file, r.line, r.pattern)
		}
	}

	// Reverse check: a stale entry is a claim about a route that no longer
	// exists, and it will quietly cover a future route that reuses the pattern.
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r.pattern] = true
	}
	for _, m := range []struct {
		name string
		set  map[string]string
	}{{"publicRoutes", publicRoutes}, {"gatedInHandler", gatedInHandler}, {"allowedDuplicateRoutes", allowedDuplicateRoutes}} {
		var stale []string
		for pattern := range m.set {
			if !seen[pattern] {
				stale = append(stale, pattern)
			}
		}
		sort.Strings(stale)
		for _, p := range stale {
			t.Errorf("%s has a stale entry %q - no such route is registered any more", m.name, p)
		}
	}
}

// The test above judges each route alone; a duplicate is a property of the pair.
func TestNoUnexpectedDuplicateRoutes(t *testing.T) {
	routes := collectRoutes(t)

	occurrences := map[string][]registeredRoute{}
	for _, r := range routes {
		occurrences[r.pattern] = append(occurrences[r.pattern], r)
	}

	var dupes []string
	for pattern := range occurrences {
		if len(occurrences[pattern]) > 1 {
			dupes = append(dupes, pattern)
		}
	}
	sort.Strings(dupes)

	for _, pattern := range dupes {
		where := make([]string, 0, len(occurrences[pattern]))
		for _, r := range occurrences[pattern] {
			where = append(where, r.file+":"+strconv.Itoa(r.line))
		}
		if _, ok := allowedDuplicateRoutes[pattern]; !ok {
			t.Errorf("route %q is registered %d times (%s) - http.ServeMux PANICS on a duplicate "+
				"pattern, so this is a boot failure, not a 4xx. Put the two registrations in exclusive "+
				"if/else branches and add the pattern to allowedDuplicateRoutes with the condition, or "+
				"remove one.", pattern, len(occurrences[pattern]), strings.Join(where, ", "))
		}
	}

	// Same reverse check as above, one level deeper: an allowance for a pattern
	// that is no longer registered twice would silently permit a future one.
	for pattern := range allowedDuplicateRoutes {
		if len(occurrences[pattern]) < 2 {
			t.Errorf("allowedDuplicateRoutes has a stale entry %q - it is registered %d time(s) now, "+
				"so the allowance covers nothing and would hide the next real duplicate",
				pattern, len(occurrences[pattern]))
		}
	}
}

// TestPublicRoutesAreJustified keeps the lists from rotting into mute buttons:
// an entry with no reason is an entry nobody has to defend.
func TestPublicRoutesAreJustified(t *testing.T) {
	for _, m := range []struct {
		name string
		set  map[string]string
	}{{"publicRoutes", publicRoutes}, {"gatedInHandler", gatedInHandler}, {"allowedDuplicateRoutes", allowedDuplicateRoutes}} {
		for pattern, reason := range m.set {
			if len(strings.TrimSpace(reason)) < 15 {
				t.Errorf("%s[%q] has no usable reason (%q) - say why, or wrap the route",
					m.name, pattern, reason)
			}
		}
	}
}
