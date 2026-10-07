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
//   - the server side is matched per message, but two name-level fallbacks
//     remain (assignments on a receiver the scan cannot type, JSON-name string
//     literals) and can still mask a namesake; the client side is by name;
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

// neverWrittenByMessage is the per-message ticket list: fields a client reads
// that no server code sets on THAT message, revealed once the guard stopped
// letting a same-named field of another message mask them. Each entry is
// either a real gap (wire it or drop the client read) or a write the scan
// cannot see (set through an untyped path); triage in the backlog, then
// delete the line. Never grow this list to silence a new regression.
var neverWrittenByMessage = map[string]string{
	"ContainerInfo.date":                      "never written: tracked in backlog",
	"ContainerInfo.file_size":                 "never written: tracked in backlog",
	"ExtractDataChunk.total_size":             "never written: tracked in backlog",
	"LoadTestReport.cold_cache":               "never written: tracked in backlog",
	"LoadTestReport.disk_read_starved":        "never written: tracked in backlog",
	"LoadTestReport.harness_cpu_fraction":     "never written: tracked in backlog",
	"LoadTestReport.harness_cpu_measured":     "never written: tracked in backlog",
	"LoadTestReport.storage_fs_type":          "never written: tracked in backlog",
	"LoadTestReport.storage_opaque":           "never written: tracked in backlog",
	"LoadTestReport.tiers":                    "never written: tracked in backlog",
	"MetadataResult.backdrop_url":             "never written: tracked in backlog",
	"MetadataResult.poster_url":               "never written: tracked in backlog",
	"MetadataResult.provider_id":              "never written: tracked in backlog",
	"MkvTrack.bit_depth":                      "never written: tracked in backlog",
	"MkvTrack.frame_rate":                     "never written: tracked in backlog",
	"MkvTrack.is_forced":                      "never written: tracked in backlog",
	"ScheduledTask.last_error":                "never written: tracked in backlog",
	"SystemInfo.server_id":                    "never written: tracked in backlog (previously masked by a namesake)",
	"RollbackMediaSeekIndexResponse.restored": "never written: read by the TV browser app, surfaced when tv-web joined the scan; tracked in backlog",
	"TranscodeJob.container":                  "never written: tracked in backlog",
	"WebPushKeys.p256dh":                      "never written: tracked in backlog",
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

// deadFields is the verdict: "Message.snake_field" entries whose JSON name a
// client reads while no server code ever writes that field of THAT message.
// The corpora are tokenized ONCE into sets so the per-field check is a lookup
// (a thousand fields times a regex over megabytes was 14 s of gate).
//
// Server side, per message: a field is written when it is a key of a
// `pkg.Message{...}` literal (depth 1 of that literal only), or the target of
// `x.Field = ...` where x is typed to Message in the enclosing function. Two
// name-level fallbacks remain, because the guard has no type information
// there: an assignment whose receiver it cannot type (`a.b.Field = ...`,
// range variables, call results) and a JSON name appearing as a string literal
// (the manual-JSON HTTP handlers: struct tags, map keys). Both can still mask
// a never-written namesake. Client side stays by name: JSON carries no type.
func deadFields(fields []protoField, serverCorpus, clientCorpus string) []string {
	tokenize := func(corpus string, re *regexp.Regexp) map[string]bool {
		set := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(corpus, -1) {
			set[m[1]] = true
		}
		return set
	}
	messages := map[string]bool{}
	for _, f := range fields {
		messages[msgKey(f.Message)] = true
	}
	typed, untyped := indexServerWrites(serverCorpus, messages)
	stringLits := tokenize(serverCorpus, regexp.MustCompile(`"(\w+)"`))
	clientProps := tokenize(clientCorpus, regexp.MustCompile(`\.(\w+)\b`))

	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		id := f.Message + "." + f.Snake
		if seen[id] {
			continue
		}
		seen[id] = true
		g := goName(f.Snake)
		j := jsonName(f.Snake)
		if typed[msgKey(f.Message)+"|"+g] || untyped[g] || stringLits[j] {
			continue
		}
		if clientProps[j] {
			out = append(out, id)
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
	if len(got) != 1 || got[0] != "ThingResponse.dead_field" {
		t.Fatalf("guard verdict = %v, want exactly [ThingResponse.dead_field]", got)
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
		filepath.Join(repoRoot, "frontend/apps/tv-web/src"),
	}, isClientSource)
	for _, f := range deadFields(fields, server, client) {
		if _, ok := allowlist[f[strings.IndexByte(f, '.')+1:]]; ok {
			continue
		}
		if _, ok := neverWrittenByMessage[f]; ok {
			continue
		}
		t.Errorf("proto field %q is read by a client but written by no server code - "+
			"either wire it up (see applyMediaDuration for the last one) or add it to neverWrittenByMessage with a reason", f)
	}
}
