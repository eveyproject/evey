// Package contractguard holds the static anti-regression gate for the
// producer/consumer contract of the API: a proto RESPONSE field that a client
// READS must be WRITTEN somewhere in the server. The bug class it catches
// shipped three times before it existed (found 2026-08-03): WatchProgress
// duration_ms/percent_complete were read by every client and filled by nobody,
// so history lines and progress bars rendered zero forever - no crash, no log,
// no failing test, because each side was locally consistent and the client
// mocks encoded the wished-for shape.
//
// Same posture as txguard: pure source scanning, no DB, sub-second, so it runs
// in pre-push (ci-arch) instead of surprising a human on a phone screen.
//
// Honest limits, so nobody over-trusts a green run:
//   - a field written for ONE message masks its never-written namesake on
//     another message (names are matched tree-wide, not per-message);
//   - a field written on one RPC path but left empty on another (the
//     streams-on-list case) is invisible here - that class is covered per path
//     in grpcapi (TestLibraryHandler_{Get,List,Filter}MediaItem(s)_HydratesStreams);
//   - a client property access that merely shares a proto field's JSON name
//     can flag a false positive: triage it into the allowlist with a reason.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"
)

const repoRoot = "../.."

// allowlist holds triaged findings that must not block the gate. Every entry
// needs the reason next to it, or the list rots into a mute button.
// Triage of the first real run (2026-08-03):
var allowlist = map[string]string{
	"available": "client reads are browser APIs (remotePlayback.available, docPiP.available), not the admin_plugin proto field",
	"settings":  "client reads are i18n keys and local nav objects, not the metadata provider settings map",
	"track_id":  "client read is the music player's local scrobble state, not the mkv diagnose proto field",
	"platform":  "PushService.ListDevices returns an empty list BY DESIGN (FCM era removed, see push_handler.go:102) - the field cannot be live",
}

type protoField struct {
	Message string
	Snake   string
}

var (
	msgRe   = regexp.MustCompile(`^\s*message\s+(\w+)\s*\{`)
	enumRe  = regexp.MustCompile(`^\s*enum\s+(\w+)\s*\{`)
	fieldRe = regexp.MustCompile(`^\s*(?:repeated\s+|optional\s+)?(?:map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*\d+`)
)

// parseProtoFields extracts every field of every non-Request message under
// dir. Request messages are excluded on purpose: their fields flow in the
// opposite direction (client writes, server reads), so "no server write" is
// their normal state, not a defect.
func parseProtoFields(t *testing.T, dir string) []protoField {
	t.Helper()
	var out []protoField
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".proto") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		type frame struct {
			kind string // "message" | "enum" | "other"
			name string
		}
		var stack []frame
		for raw := range strings.SplitSeq(string(b), "\n") {
			line := strings.TrimSpace(raw)
			if strings.HasPrefix(line, "//") {
				continue
			}
			switch {
			case msgRe.MatchString(line):
				stack = append(stack, frame{"message", msgRe.FindStringSubmatch(line)[1]})
				continue
			case enumRe.MatchString(line):
				stack = append(stack, frame{"enum", enumRe.FindStringSubmatch(line)[1]})
				continue
			case strings.HasSuffix(line, "{"):
				// oneof, service, rpc bodies... anything else that nests.
				stack = append(stack, frame{"other", ""})
				continue
			case line == "}" || line == "};":
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				continue
			}
			// Innermost enclosing message, ignoring oneof frames; skip enum bodies.
			inEnum := false
			msg := ""
			for _, fr := range slices.Backward(stack) {
				if fr.kind == "enum" {
					inEnum = true
					break
				}
				if fr.kind == "message" {
					msg = fr.name
					break
				}
			}
			if inEnum || msg == "" || strings.HasSuffix(msg, "Request") {
				continue
			}
			if m := fieldRe.FindStringSubmatch(line); m != nil {
				out = append(out, protoField{Message: msg, Snake: m[1]})
			}
		}
	}
	return out
}

func goName(snake string) string {
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		parts[i] = string(r)
	}
	return strings.Join(parts, "")
}

func jsonName(snake string) string {
	g := goName(snake)
	r := []rune(g)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// collectCorpus concatenates the content of every file under roots that the
// keep filter accepts. Plain bytes are enough: the guard only needs "does this
// pattern occur anywhere", never positions.
func collectCorpus(t *testing.T, roots []string, keep func(path string) bool) string {
	t.Helper()
	var sb strings.Builder
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "gen", "generated", "android", ".expo", "dist", "fakes":
					return filepath.SkipDir
				}
				return nil
			}
			if keep(path) {
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					return rerr
				}
				sb.Write(b)
				sb.WriteByte('\n')
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return sb.String()
}

func isServerSource(path string) bool {
	return strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go") &&
		!strings.HasSuffix(path, ".pb.go") &&
		!strings.HasSuffix(path, ".pb.gw.go")
}

func isClientSource(path string) bool {
	if strings.Contains(path, ".test.") || strings.Contains(path, "__tests__") {
		return false
	}
	return strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")
}

// deadFields is the verdict: fields whose JSON name a client reads while no
// server code ever assigns their Go name. The corpora are tokenized ONCE into
// sets (composite-literal keys, assignment targets, string literals, property
// accesses) so the per-field check is a lookup - a thousand fields times a
// regex over megabytes was 14 s of gate, this is under a second.
func deadFields(fields []protoField, serverCorpus, clientCorpus string) []string {
	tokenize := func(corpus string, re *regexp.Regexp) map[string]bool {
		set := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(corpus, -1) {
			set[m[1]] = true
		}
		return set
	}
	// A field counts as written when server code assigns its Go name (literal
	// key or assignment), or when its JSON name appears as a string literal -
	// that is how the manual-JSON HTTP handlers produce it (struct tags like
	// `json:"hasPin"`, map keys like out["pinToken"]).
	literalKeys := tokenize(serverCorpus, regexp.MustCompile(`\b(\w+)\s*:`))
	assignments := tokenize(serverCorpus, regexp.MustCompile(`\.(\w+)\s*=`))
	stringLits := tokenize(serverCorpus, regexp.MustCompile(`"(\w+)"`))
	clientProps := tokenize(clientCorpus, regexp.MustCompile(`\.(\w+)\b`))

	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		if seen[f.Snake] {
			continue
		}
		seen[f.Snake] = true
		g := goName(f.Snake)
		j := jsonName(f.Snake)
		if literalKeys[g] || assignments[g] || stringLits[j] {
			continue
		}
		if clientProps[j] {
			out = append(out, f.Snake)
		}
	}
	sort.Strings(out)
	return out
}

// The parser and the verdict are proven RED on fixtures first: a guard whose
// detector was never seen firing proves nothing (the 68-vs-42 lesson).
func TestGuardDetectsAFixtureDeadField(t *testing.T) {
	fields := parseProtoFields(t, "testdata/proto")
	// The fixture proto carries: 3 fields on ThingResponse, 1 on a nested
	// message, 1 enum value that must NOT parse as a field, and a Request
	// message that must be excluded entirely.
	if len(fields) != 4 {
		t.Fatalf("fixture proto parsed into %d fields, want 4: %v", len(fields), fields)
	}
	server := collectCorpus(t, []string{"testdata/server"}, isServerSource)
	client := collectCorpus(t, []string{"testdata/client"}, isClientSource)
	got := deadFields(fields, server, client)
	if len(got) != 1 || got[0] != "dead_field" {
		t.Fatalf("guard verdict = %v, want exactly [dead_field]", got)
	}
}

func TestNoClientReadOfAFieldTheServerNeverWrites(t *testing.T) {
	fields := parseProtoFields(t, filepath.Join(repoRoot, "proto/evey/v1"))
	if len(fields) < 100 {
		t.Fatalf("only %d proto fields parsed - the parser lost the tree, refusing to pass vacuously", len(fields))
	}
	server := collectCorpus(t, []string{filepath.Join(repoRoot, "internal")}, isServerSource)
	client := collectCorpus(t, []string{
		filepath.Join(repoRoot, "frontend/apps/web/src"),
		filepath.Join(repoRoot, "frontend/apps/mobile/app"),
		filepath.Join(repoRoot, "frontend/apps/mobile/src"),
		filepath.Join(repoRoot, "frontend/apps/tv/app"),
		filepath.Join(repoRoot, "frontend/apps/tv/src"),
	}, isClientSource)
	for _, f := range deadFields(fields, server, client) {
		if _, ok := allowlist[f]; ok {
			continue
		}
		t.Errorf("proto field %q is read by a client but written by no server code - "+
			"either wire it up (see applyMediaDuration for the last one) or allowlist it with a reason", f)
	}
}
