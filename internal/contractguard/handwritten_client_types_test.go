// Hand-written client types guard: the mobile and TV apps do not use the
// generated client. They declare their own response interfaces and hand them to
// eveyFetch, which trusts them blindly - so an interface can promise a field the
// endpoint never sends, and nothing complains. TypeScript is happy, the tests
// mock the hook, and the screen renders nothing.
//
// It shipped exactly that way: GetCollectionResponse was declared with `items`
// and `total` on both clients while the endpoint returned the collection alone,
// so every collection detail screen on mobile and TV was empty (found
// 2026-08-04). The web was fine because orval generates its types from the same
// proto the server compiles.
//
// The rule: for a call whose VERB+URL matches an rpc's http annotation, every field
// the client's response interface declares must exist on that rpc's response
// message.
//
// Honest limits:
//   - only calls whose URL is a literal in the same file;
//   - only the top-level interface handed to eveyFetch, not nested ones;
//   - a field the client declares and never reads is still flagged, which is
//     the right call: it is a promise nobody kept.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// clientTypeAllowlist holds triaged findings, each with its reason.
var clientTypeAllowlist = map[string]string{}

var (
	rpcRe       = regexp.MustCompile(`rpc\s+\w+\s*\(\s*[\w.]+\s*\)\s*returns\s*\(\s*([\w.]+)\s*\)`)
	// Every verb, not just get: the guard used to stop at GET while the mobile
	// and TV apps hand eveyFetch a declared response type on 18 non-GET calls
	// too (3 POST, 6 PUT, 9 DELETE), all of them trusted just as blindly.
	httpRouteRe = regexp.MustCompile(`(get|post|put|patch|delete):\s*"([^"]+)"`)
	fetchCallRe = regexp.MustCompile(`(?s)eveyFetch<(\w+)>\(\{(.{0,400}?)\}\)`)
	urlRe       = regexp.MustCompile("url:\\s*[`\"']([^`\"']+)[`\"']")
	methodRe    = regexp.MustCompile(`method:\s*"(\w+)"`)
	tsFieldRe   = regexp.MustCompile(`(?m)^\s*(\w+)\??\s*:`)
	pathParamRe = regexp.MustCompile(`\{[^}]*\}|\$\{[^}]*\}`)
)

// routeKey pairs a verb with a normalised path. Keying on the path alone would
// match a client DELETE against the GET annotation of the same URL - two
// different rpcs, two different messages.
func routeKey(verb, path string) string {
	return strings.ToUpper(verb) + " " + normalisePath(path)
}

func normalisePath(p string) string {
	if i := strings.Index(p, "?"); i >= 0 {
		p = p[:i]
	}
	return pathParamRe.ReplaceAllString(p, "{}")
}

// parseHTTPRoutes pairs each http annotation with the response message of the rpc
// it belongs to. The annotation always follows its rpc line, so a scan that
// remembers the last rpc seen is enough.
func parseHTTPRoutes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		var current string
		for line := range strings.SplitSeq(string(b), "\n") {
			if m := rpcRe.FindStringSubmatch(line); m != nil {
				current = m[1]
				continue
			}
			if m := httpRouteRe.FindStringSubmatch(line); m != nil && current != "" {
				out[routeKey(m[1], m[2])] = current
			}
		}
	}
	return out
}

// protoResponseFields maps a message name to the json names of its fields.
func protoResponseFields(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		var current string
		for line := range strings.SplitSeq(string(b), "\n") {
			if m := msgRe.FindStringSubmatch(line); m != nil {
				current = m[1]
				out[current] = map[string]bool{}
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "}") {
				current = ""
				continue
			}
			if current == "" {
				continue
			}
			if m := fieldRe.FindStringSubmatch(line); m != nil {
				out[current][jsonName(m[1])] = true
			}
		}
	}
	return out
}

// tsInterfaceFields reads the field names of a TS interface or type alias
// declared in src.
func tsInterfaceFields(src, name string) ([]string, bool) {
	re := regexp.MustCompile(`(?s)(?:interface|type)\s+` + regexp.QuoteMeta(name) + `\s*=?\s*\{(.*?)\n\}`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil, false
	}
	var out []string
	for _, f := range tsFieldRe.FindAllStringSubmatch(m[1], -1) {
		out = append(out, f[1])
	}
	return out, true
}

func handwrittenClientFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, app := range []string{"frontend/apps/mobile", "frontend/apps/tv"} {
		root := filepath.Join(repoRoot, app)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "android", "ios", ".expo", "dist":
					return filepath.SkipDir
				}
				return nil
			}
			if isClientSource(path) {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", app, err)
		}
	}
	sort.Strings(out)
	return out
}

// queryParamAllowlist holds parameters carried outside the proto request.
var queryParamAllowlist = map[string]string{
	"token": "stream and artwork URLs carry a mint-on-use token read by the HTTP layer, not the rpc",
}

// rpcRequestRe pairs an rpc with its REQUEST message, the mirror of
// parseHTTPRoutes.
var rpcRequestRe = regexp.MustCompile(`rpc\s+\w+\s*\(\s*([\w.]+)\s*\)\s*returns`)

func parseHTTPRequestMessages(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		var current string
		for line := range strings.SplitSeq(string(b), "\n") {
			if m := rpcRequestRe.FindStringSubmatch(line); m != nil {
				current = m[1]
				continue
			}
			if m := httpRouteRe.FindStringSubmatch(line); m != nil && current != "" {
				out[routeKey(m[1], m[2])] = current
			}
		}
	}
	return out
}

// A query parameter the request message does not declare is dropped in silence:
// the client believes it asked for something and the server never hears it.
// Mobile and TV sent limit and offset to GetCollection for months against a
// request that had neither, which is how a truncated collection looked
// deliberate (found 2026-08-04).
func TestClientQueryParamsExistOnTheRequestTheyTarget(t *testing.T) {
	protoDir := filepath.Join(repoRoot, "proto/evey/v1")
	requests := parseHTTPRequestMessages(t, protoDir)
	fields := protoResponseFields(t, protoDir)

	var findings []string
	checked := 0
	for _, f := range handwrittenClientFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		for _, call := range fetchCallRe.FindAllStringSubmatch(src, -1) {
			body := call[2]
			mm := methodRe.FindStringSubmatch(body)
			if mm == nil {
				continue
			}
			verb := mm[1]
			um := urlRe.FindStringSubmatch(body)
			if um == nil {
				continue
			}
			raw := um[1]
			_, query, hasQuery := strings.Cut(raw, "?")
			if !hasQuery {
				continue
			}
			msg, known := requests[routeKey(verb, raw)]
			if !known {
				continue
			}
			want := fields[msg]
			if len(want) == 0 {
				continue
			}
			checked++
			rel := strings.TrimPrefix(f, filepath.Join(repoRoot, "frontend/apps")+"/")
			for pair := range strings.SplitSeq(query, "&") {
				name, _, _ := strings.Cut(pair, "=")
				if name == "" || want[name] || queryParamAllowlist[name] != "" {
					continue
				}
				findings = append(findings, rel+": ?"+name+" is not a field of "+msg)
			}
		}
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("clients send query parameters their endpoint never reads:\n  %s\n"+
			"Either declare the field on the request, or stop sending it.",
			strings.Join(findings, "\n  "))
	}
	t.Logf("checked %d hand-written GET calls carrying query parameters", checked)
}

// The detector must be seen firing on the exact shape that shipped: a wrapper
// field the flat endpoint never sends.
func TestHandWrittenClientTypeGuardFiresOnAWrapperThatDoesNotExist(t *testing.T) {
	protoDir := filepath.Join(repoRoot, "proto/evey/v1")
	fields := protoResponseFields(t, protoDir)
	want := fields["PlaylistProto"]
	if len(want) == 0 {
		t.Fatal("PlaylistProto has no parsed fields - the parser has drifted")
	}
	src := "interface Detail {\n  playlist: { id: string };\n  name: string;\n}\n"
	declared, ok := tsInterfaceFields(src, "Detail")
	if !ok {
		t.Fatal("the fixture interface did not parse")
	}
	var flagged []string
	for _, d := range declared {
		if !want[d] {
			flagged = append(flagged, d)
		}
	}
	if len(flagged) != 1 || flagged[0] != "playlist" {
		t.Fatalf("verdict = %v, want exactly [playlist] - name is a real field and must not be flagged", flagged)
	}
}

func TestHandWrittenClientTypesMatchTheEndpointTheyCall(t *testing.T) {
	protoDir := filepath.Join(repoRoot, "proto/evey/v1")
	routes := parseHTTPRoutes(t, protoDir)
	if len(routes) < 50 {
		t.Fatalf("parsed %d GET routes, want at least 50 - the parser has drifted", len(routes))
	}
	fields := protoResponseFields(t, protoDir)

	var findings []string
	checked := 0
	for _, f := range handwrittenClientFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		for _, call := range fetchCallRe.FindAllStringSubmatch(src, -1) {
			typeName, body := call[1], call[2]
			mm := methodRe.FindStringSubmatch(body)
			if mm == nil {
				continue
			}
			verb := mm[1]
			um := urlRe.FindStringSubmatch(body)
			if um == nil {
				continue
			}
			msg, known := routes[routeKey(verb, um[1])]
			if !known {
				continue
			}
			declared, found := tsInterfaceFields(src, typeName)
			if !found {
				continue
			}
			want := fields[msg]
			if len(want) == 0 {
				continue
			}
			checked++
			rel := strings.TrimPrefix(f, filepath.Join(repoRoot, "frontend/apps")+"/")
			for _, d := range declared {
				if want[d] || clientTypeAllowlist[rel+":"+typeName+"."+d] != "" {
					continue
				}
				findings = append(findings, rel+": "+typeName+"."+d+" is not a field of "+msg)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no hand-written GET response type could be matched to an rpc - the matcher has drifted")
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("hand-written client types promise fields their endpoint never sends:\n  %s\n"+
			"Either the server must send them, or the client must stop claiming them.",
			strings.Join(findings, "\n  "))
	}
	t.Logf("checked %d hand-written response types", checked)
}
