// Admin-gate guard for the native clients. The web guard next door matches
// generated symbol names, which mobile and TV do not use: they call eveyFetch
// with a URL and a method, so the only way to know an admin RPC is being
// reached is to read the http annotations out of the proto and match on them.
// That hole was documented when the web guard shipped; this closes it.
//
// The rule: a mobile or TV file that reaches an admin-only RPC must read
// isAdmin, or be classified below with the reason it does not need to.
//
// Honest limits:
//   - only calls whose url and method are literals in the file;
//   - "the file reads isAdmin" is not "the right control is gated" - that stays
//     the job of the per-surface tests.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// nativeGatedByCaller lists native files that reach an admin RPC without
// reading isAdmin, with the reason each is nonetheless out of a viewer's reach.
var nativeGatedByCaller = map[string]string{
	"mobile/app/(auth)/admin/":             "the whole admin area sits behind app/(auth)/admin/_layout.tsx, which redirects a non-admin",
	"mobile/src/components/admin/":         "helpers used only from the admin area, itself behind the layout redirect",
	"mobile/src/hooks/useAdminSegments.ts": "hook used only by app/(auth)/admin/media/[id].tsx, behind the layout redirect",
	"mobile/src/components/library/BulkAddSheets.tsx": "sheets with no entry point of their own - the buttons that open them are " +
		"gated in the library and favorites screens",
	"mobile/src/api/collections.ts": "hook wrappers, no UI of their own; the screens calling them are gated",
	"mobile/src/api/playlists.ts":   "playlist RPCs are RoleUser, not admin",
	"tv/app/(auth)/admin/":          "same layout redirect as mobile, if the TV ever grows an admin area",
}

var (
	httpVerbRe = regexp.MustCompile(`(?:post|put|delete|patch):\s*"([^"]+)"`)
	rpcNameRe  = regexp.MustCompile(`rpc\s+(\w+)\s*\(`)
	// eveyFetch calls on the native clients name the method explicitly; the
	// generic form (no type parameter) is the common one for writes.
	nativeCallRe = regexp.MustCompile(`(?s)eveyFetch(?:<[^>]*>)?\(\{(.{0,400}?)\}\)`)
)

// adminWriteRoutes maps a normalised write path to the RPC name, restricted to
// the RPCs the RBAC map marks RoleAdmin.
func adminWriteRoutes(t *testing.T) map[string]string {
	t.Helper()
	admin := map[string]bool{}
	b, err := os.ReadFile(filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/interceptors/rbac.go"))
	if err != nil {
		t.Fatalf("read rbac.go: %v", err)
	}
	for _, m := range adminRPCRe.FindAllStringSubmatch(string(b), -1) {
		admin[m[2]] = true
	}
	if len(admin) < 20 {
		t.Fatalf("parsed %d admin RPCs, want at least 20 - the parser has drifted", len(admin))
	}

	out := map[string]string{}
	dir := filepath.Join(repoRoot, "proto/evey/v1")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}
		src, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		var current string
		for line := range strings.SplitSeq(string(src), "\n") {
			if m := rpcNameRe.FindStringSubmatch(line); m != nil {
				current = m[1]
				continue
			}
			if m := httpVerbRe.FindStringSubmatch(line); m != nil && admin[current] {
				// Keyed by method too: PUT and DELETE on /collections/{id} are
				// different RPCs behind one path, and a message naming the wrong
				// one sends the reader to the wrong place.
				verb := strings.ToUpper(strings.TrimSpace(strings.SplitN(strings.TrimSpace(line), ":", 2)[0]))
				out[verb+" "+normalisePath(m[1])] = current
			}
		}
	}
	return out
}

func nativeClassified(rel string) bool {
	for prefix := range nativeGatedByCaller {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

// callReachesAdminRoute reports whether an eveyFetch call literal in src is a
// write against an admin-only route.
func callReachesAdminRoute(body string, routes map[string]string) (string, bool) {
	m := methodRe.FindStringSubmatch(body)
	if m == nil || m[1] == "GET" {
		return "", false
	}
	um := urlRe.FindStringSubmatch(body)
	if um == nil {
		return "", false
	}
	rpc, ok := routes[strings.ToUpper(m[1])+" "+normalisePath(um[1])]
	return rpc, ok
}

// nativeAdminHooks finds the exported hooks whose own body writes to an
// admin-only route. Screens call those, not eveyFetch: without following this
// one hop the guard would only ever look at the wrapper files themselves, which
// are all classified, and prove nothing.
func nativeAdminHooks(t *testing.T, files []string, routes map[string]string) map[string]string {
	t.Helper()
	exportRe := regexp.MustCompile(`export (?:function|const) (use\w+)`)
	out := map[string]string{}
	for _, f := range files {
		if !strings.Contains(f, "/src/api/") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		locs := exportRe.FindAllStringSubmatchIndex(src, -1)
		for i, loc := range locs {
			end := len(src)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			for _, call := range nativeCallRe.FindAllStringSubmatch(src[loc[0]:end], -1) {
				if rpc, ok := callReachesAdminRoute(call[1], routes); ok {
					out[src[loc[2]:loc[3]]] = rpc
				}
			}
		}
	}
	return out
}

func TestEveryNativeSurfaceReachingAnAdminRPCIsClassified(t *testing.T) {
	routes := adminWriteRoutes(t)
	if len(routes) < 15 {
		t.Fatalf("mapped %d admin write routes, want at least 15 - the annotation parser has drifted", len(routes))
	}
	files := handwrittenClientFiles(t)
	hooks := nativeAdminHooks(t, files, routes)
	if len(hooks) < 3 {
		t.Fatalf("found %d admin-writing hooks, want at least 3 - the indirection scan has drifted", len(hooks))
	}

	root := filepath.Join(repoRoot, "frontend/apps") + "/"
	var ungated []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		if strings.Contains(src, "isAdmin") {
			continue
		}
		rel := strings.TrimPrefix(f, root)
		if nativeClassified(rel) {
			continue
		}
		for _, call := range nativeCallRe.FindAllStringSubmatch(src, -1) {
			if rpc, ok := callReachesAdminRoute(call[1], routes); ok {
				ungated = append(ungated, rel+" -> "+rpc)
			}
		}
		for hook, rpc := range hooks {
			if strings.Contains(src, hook+"(") {
				ungated = append(ungated, rel+" -> "+hook+" -> "+rpc)
			}
		}
	}
	sort.Strings(ungated)
	if len(ungated) > 0 {
		t.Errorf("native surfaces reach an admin-only RPC without deciding who sees them:\n  %s\n"+
			"Gate the control on isAdmin, or classify the file in nativeGatedByCaller with the reason.",
			strings.Join(ungated, "\n  "))
	}
}

// The detector must be seen firing, or a green run proves nothing.
func TestNativeAdminGateGuardFiresOnAnUngatedFixture(t *testing.T) {
	routes := adminWriteRoutes(t)
	var adminPath string
	for p := range routes {
		if strings.HasPrefix(p, "DELETE ") && strings.Contains(p, "collections") {
			adminPath = p
			break
		}
	}
	if adminPath == "" {
		t.Fatal("no collection write route found - the annotation parser has drifted")
	}

	src := "eveyFetch({ url: `" + strings.ReplaceAll(strings.TrimPrefix(adminPath, "DELETE "), "{}", "${id}") + "`, method: \"DELETE\" })"
	var hits int
	for _, call := range nativeCallRe.FindAllStringSubmatch(src, -1) {
		if _, ok := callReachesAdminRoute(call[1], routes); ok {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("the fixture call was matched %d times, want exactly 1", hits)
	}
}
