package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/evey-project/evey/gen/evey/v1" // register proto service descriptors
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/pkg/demo"
)

// Demo-policy guard: on the public demo every visitor is an administrator, so
// every admin operation is refused unless internal/pkg/demo names it. That
// allow-list is a snapshot of the server on the day it was written, and it
// rots the moment an endpoint is added without a decision - with a deny-list
// the new endpoint ships open, with an allow-list it ships closed, and either
// way nobody decided. This guard makes the decision mandatory:
//
//   - every RPC the RBAC map classifies RoleAdmin, and every user-callable RPC
//     the policy locks, appears in exactly one of the policy's RPC lists;
//   - every HTTP route that is admin-gated, state-changing, or method-less
//     appears in exactly one of the policy's route lists;
//   - no entry names an RPC or a route that no longer exists;
//   - every entry carries a reason a guest can weigh.
//
// Same posture as the RBAC guard (TestAllRegisteredMethodsAreClassified) and
// the route-auth guard next door, and it reuses the latter's route parser, so
// the two see the same 145 routes.
//
// Honest limit: this proves each operation was classified, not that it was
// classified correctly - the reason next to each entry is what a guest
// reads for that.

// adminMiddlewareCtors are the wrappers that make a route admin-only. A route
// wrapped in one is an admin operation for the demo, whatever its verb.
var adminMiddlewareCtors = map[string]bool{
	"httpAdminMiddleware":         true,
	"httpScrapeOrAdminMiddleware": true,
}

func TestEveryAdminRPCIsClassifiedForTheDemo(t *testing.T) {
	rbac := interceptors.DefaultRBACConfig()

	for method, role := range rbac.MethodRoles {
		if role != interceptors.RoleAdmin {
			continue
		}
		_, open := demo.OpenAdminRPCs[method]
		_, locked := demo.LockedAdminRPCs[method]
		switch {
		case open && locked:
			t.Errorf("admin RPC %s is in BOTH demo.OpenAdminRPCs and demo.LockedAdminRPCs", method)
		case !open && !locked:
			t.Errorf("admin RPC %s is not classified for the demo - add it to demo.OpenAdminRPCs "+
				"(a read that discloses nothing about the host or other visitors) or demo.LockedAdminRPCs, with the reason", method)
		}
	}

	// Reverse: a stale entry is a claim about an RPC that is gone, and it will
	// quietly cover a future RPC that reuses the name.
	for name, set := range map[string]map[string]string{
		"OpenAdminRPCs":   demo.OpenAdminRPCs,
		"LockedAdminRPCs": demo.LockedAdminRPCs,
	} {
		for method := range set {
			if role, ok := rbac.MethodRoles[method]; !ok || role != interceptors.RoleAdmin {
				t.Errorf("demo.%s lists %s, which is not an admin RPC in the RBAC map - stale entry, or the RPC "+
					"belongs in demo.LockedUserRPCs", name, method)
			}
		}
	}
	// A user-callable RPC the demo locks must exist and must NOT be admin
	// (an admin RPC is locked by default and belongs in LockedAdminRPCs).
	for method := range demo.LockedUserRPCs {
		if role, ok := rbac.MethodRoles[method]; ok && role == interceptors.RoleAdmin {
			t.Errorf("demo.LockedUserRPCs lists %s, which is an admin RPC - move it to demo.LockedAdminRPCs", method)
		}
		if _, open := demo.OpenAdminRPCs[method]; open {
			t.Errorf("%s is both locked (LockedUserRPCs) and open (OpenAdminRPCs)", method)
		}
	}
}

// TestLockedUserRPCsExist walks the proto descriptors the way the RBAC guard
// does, so a renamed RPC cannot leave a dead entry behind.
func TestLockedUserRPCsExist(t *testing.T) {
	real := registeredRPCNames(t)
	for method := range demo.LockedUserRPCs {
		if !real[method] {
			t.Errorf("demo.LockedUserRPCs lists %s but no such evey.v1 RPC is registered", method)
		}
	}
}

func TestEveryAdminOrWritingHTTPRouteIsClassifiedForTheDemo(t *testing.T) {
	routes := collectDemoRoutes(t)
	if len(routes) < routeAuthMinRoutes {
		t.Fatalf("found %d routes, want at least %d - the parser has drifted", len(routes), routeAuthMinRoutes)
	}

	seen := map[string]bool{}
	var mustClassify int
	for _, r := range routes {
		seen[r.pattern] = true
		if !r.needsDemoDecision() {
			continue
		}
		mustClassify++
		_, open := demo.OpenRoutes[r.pattern]
		_, locked := demo.LockedRoutes[r.pattern]
		switch {
		case open && locked:
			t.Errorf("%s:%d: route %q is in BOTH demo.OpenRoutes and demo.LockedRoutes", r.file, r.line, r.pattern)
		case !open && !locked:
			t.Errorf("%s:%d: route %q is admin-gated or state-changing and not classified for the demo.\n"+
				"  Add it to demo.OpenRoutes (own data or a harmless read) or demo.LockedRoutes, with the reason.",
				r.file, r.line, r.pattern)
		}
	}
	// The guard must be looking at something: 58 admin routes plus the writes.
	if mustClassify < 60 {
		t.Fatalf("only %d routes needed a demo decision, want at least 60 - admin/write detection has drifted", mustClassify)
	}

	for name, set := range map[string]map[string]string{"OpenRoutes": demo.OpenRoutes, "LockedRoutes": demo.LockedRoutes} {
		var stale []string
		for pattern := range set {
			if !seen[pattern] {
				stale = append(stale, pattern)
			}
		}
		sort.Strings(stale)
		for _, p := range stale {
			t.Errorf("demo.%s has a stale entry %q - no such route is registered any more", name, p)
		}
	}
}

// TestDemoPolicyEntriesAreJustified keeps the lists from rotting into mute
// buttons: an entry with no reason is an entry nobody has to defend.
func TestDemoPolicyEntriesAreJustified(t *testing.T) {
	for name, set := range map[string]map[string]string{
		"OpenAdminRPCs":   demo.OpenAdminRPCs,
		"LockedAdminRPCs": demo.LockedAdminRPCs,
		"LockedUserRPCs":  demo.LockedUserRPCs,
		"OpenRoutes":      demo.OpenRoutes,
		"LockedRoutes":    demo.LockedRoutes,
	} {
		for key, reason := range set {
			if len(strings.TrimSpace(reason)) < 4 {
				t.Errorf("demo.%s[%q] has no usable reason (%q)", name, key, reason)
			}
		}
	}
}

// TestDemoDefaultsDenyTheAdminSurface pins the runtime default the guard above
// makes unreachable for registered routes: an unclassified admin route or write
// is locked, an unclassified read is not. If the default ever flips to open,
// a skipped build ships the next admin endpoint open on the demo.
func TestDemoDefaultsDenyTheAdminSurface(t *testing.T) {
	cases := []struct {
		method, pattern string
		locked          bool
	}{
		{"GET", "GET /api/v1/admin/brand-new-thing", true},
		{"POST", "POST /api/v1/brand-new-thing", true},
		{"DELETE", "DELETE /api/v1/brand-new-thing/{id}", true},
		{"GET", "GET /api/v1/brand-new-thing", false},
		{"GET", "", false},
	}
	for _, c := range cases {
		if got := demo.RouteLocked(c.method, c.pattern); got != c.locked {
			t.Errorf("RouteLocked(%q, %q) = %v, want %v", c.method, c.pattern, got, c.locked)
		}
	}
	if !demo.RPCLocked("/evey.v1.AdminSystemService/BrandNew", true) {
		t.Error("an unclassified admin RPC must be locked by default")
	}
	if demo.RPCLocked("/evey.v1.LibraryService/BrandNewRead", false) {
		t.Error("an unclassified user RPC must stay open")
	}
}

// demoRoute is a registered route with what the demo needs to know about it.
type demoRoute struct {
	registeredRoute
	adminGated bool
}

// needsDemoDecision: admin-gated, or not a plain GET (writes and method-less
// patterns, which serve every verb), or an admin path however it is gated.
func (r demoRoute) needsDemoDecision() bool {
	if r.adminGated || strings.Contains(r.pattern, "/admin/") {
		return true
	}
	return !strings.HasPrefix(r.pattern, "GET ")
}

// collectDemoRoutes is collectRoutes with the admin-wrapper detection the
// route-auth guard does not need: it only asks "is there SOME auth wrapper".
func collectDemoRoutes(t *testing.T) []demoRoute {
	t.Helper()
	routes := collectRoutes(t)
	adminByPattern := adminGatedPatterns(t)
	out := make([]demoRoute, 0, len(routes))
	for _, r := range routes {
		out = append(out, demoRoute{registeredRoute: r, adminGated: adminByPattern[r.pattern]})
	}
	return out
}

// adminGatedPatterns returns the mux patterns whose handler expression calls
// an admin wrapper - inline, or through a file-local alias assigned from one
// (`admin := httpAdminMiddleware(tokenGen)`), the same resolution the
// route-auth guard applies to the wider set of auth wrappers.
func adminGatedPatterns(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(repoRoot, "internal/adapters/driving/http")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read http adapter dir: %v", err)
	}
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", e.Name(), perr)
		}
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
				if id, ok := call.Fun.(*ast.Ident); ok && adminMiddlewareCtors[id.Name] && i < len(assign.Lhs) {
					if lhs, ok := assign.Lhs[i].(*ast.Ident); ok {
						aliases[lhs.Name] = true
					}
				}
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
				return true
			}
			if id, isIdent := sel.X.(*ast.Ident); !isIdent || id.Name != "mux" {
				return true
			}
			lit, isLit := call.Args[0].(*ast.BasicLit)
			if !isLit || lit.Kind != token.STRING {
				return true
			}
			found := false
			ast.Inspect(call.Args[1], func(n ast.Node) bool {
				if found {
					return false
				}
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && (aliases[id.Name] || adminMiddlewareCtors[id.Name]) {
						found = true
						return false
					}
				}
				return true
			})
			if found {
				out[strings.Trim(lit.Value, `"`)] = true
			}
			return true
		})
	}
	if len(out) < 40 {
		t.Fatalf("only %d admin-gated routes resolved, want at least 40 - admin alias resolution has drifted", len(out))
	}
	return out
}

// registeredRPCNames walks the proto descriptors for every evey.v1 RPC.
func registeredRPCNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if !strings.HasPrefix(string(svc.FullName()), "evey.v1.") {
				continue
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				out["/"+string(svc.FullName())+"/"+string(methods.Get(j).Name())] = true
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatal("no evey.v1 RPC discovered - proto descriptors not registered?")
	}
	return out
}

// Extended level: it opens what the public demo locks, so its only decisions
// are the exceptions. The guard keeps those exceptions honest: each one must
// be something the public demo locks (otherwise extended would be locking what
// public leaves open), must carry a reason, and must name a real operation.
func TestExtendedStaysLockedEntriesAreHonest(t *testing.T) {
	rpcs := registeredRPCNames(t)
	for method, reason := range demo.ExtendedStaysLockedRPCs {
		_, a := demo.LockedAdminRPCs[method]
		_, u := demo.LockedUserRPCs[method]
		if !a && !u {
			t.Errorf("demo.ExtendedStaysLockedRPCs[%q] is not locked on the public demo - extended only closes what public closes", method)
		}
		if !rpcs[method] {
			t.Errorf("demo.ExtendedStaysLockedRPCs lists %s, which is not a registered RPC", method)
		}
		if len(strings.TrimSpace(reason)) < 8 {
			t.Errorf("demo.ExtendedStaysLockedRPCs[%q] has no usable reason (%q)", method, reason)
		}
	}
	registered := map[string]bool{}
	for _, r := range collectDemoRoutes(t) {
		registered[r.pattern] = true
	}
	for pattern, reason := range demo.ExtendedStaysLockedRoutes {
		if _, ok := demo.LockedRoutes[pattern]; !ok {
			t.Errorf("demo.ExtendedStaysLockedRoutes[%q] is not locked on the public demo", pattern)
		}
		if !registered[pattern] {
			t.Errorf("demo.ExtendedStaysLockedRoutes lists %q, which is not a registered route", pattern)
		}
		if len(strings.TrimSpace(reason)) < 8 {
			t.Errorf("demo.ExtendedStaysLockedRoutes[%q] has no usable reason (%q)", pattern, reason)
		}
	}
	for method, reason := range demo.ExtendedProtectedUserRPCs {
		if !rpcs[method] {
			t.Errorf("demo.ExtendedProtectedUserRPCs lists %s, which is not a registered RPC", method)
		}
		if _, stays := demo.ExtendedStaysLockedRPCs[method]; stays {
			t.Errorf("%s is protected per account AND locked outright on extended - pick one", method)
		}
		if len(strings.TrimSpace(reason)) < 8 {
			t.Errorf("demo.ExtendedProtectedUserRPCs[%q] has no usable reason (%q)", method, reason)
		}
	}
}

// Extended is a strict relaxation of Public, minus the named exceptions: no
// operation public leaves open becomes locked, and the unclassified default
// still denies the admin surface.
func TestExtendedLevelRelaxesOnlyWhatPublicLocks(t *testing.T) {
	isAdmin := map[string]bool{}
	for method, role := range interceptors.DefaultRBACConfig().MethodRoles {
		isAdmin[method] = role == interceptors.RoleAdmin
	}
	for method := range registeredRPCNames(t) {
		pub := demo.RPCLockedAt(demo.Public, method, isAdmin[method])
		ext := demo.RPCLockedAt(demo.Extended, method, isAdmin[method])
		_, stays := demo.ExtendedStaysLockedRPCs[method]
		switch {
		case !pub && ext:
			t.Errorf("%s is open on the public demo and locked on extended", method)
		case pub && !ext && stays:
			t.Errorf("%s is listed as staying locked but extended opens it", method)
		case pub && ext && !stays:
			t.Errorf("%s is locked on extended without being in ExtendedStaysLockedRPCs (a default deny leaked in)", method)
		}
	}
	if !demo.RPCLockedAt(demo.Extended, "/evey.v1.AdminSystemService/BrandNew", true) {
		t.Error("an unclassified admin RPC must be locked on extended too")
	}
	if !demo.RouteLockedAt(demo.Extended, "GET", "GET /api/v1/admin/brand-new-thing") {
		t.Error("an unclassified admin route must be locked on extended too")
	}
	for pattern := range demo.LockedRoutes {
		_, stays := demo.ExtendedStaysLockedRoutes[pattern]
		if got := demo.RouteLockedAt(demo.Extended, "POST", pattern); got != stays {
			t.Errorf("route %q: locked on extended = %v, want %v", pattern, got, stays)
		}
	}
}
