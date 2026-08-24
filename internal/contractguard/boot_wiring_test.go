package contractguard

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Wirings installed once at boot, whose absence is SILENT.
//
// internal/pkg/ratelimit carries a package-level trusted-proxy list that is
// deliberately WIDER than any operator would configure (the whole RFC1918
// space). It is the value in force during the boot window and in unit tests
// only: cmd/evey/http.go replaces it with cfg.Server.TrustedProxies, and the
// comment above the literal says so. Nothing enforced that replacement.
// Deleting the single call site leaves the build green and every trusted-proxy
// suite green (internal/config computes the list, internal/pkg/ratelimit is
// handed one explicitly - neither observes the boot), while production keeps
// the wide default: any host on the LAN can then rename itself with
// X-Forwarded-For, which collapses the rate limit into one shared bucket and
// makes the remote-access gate classify every visitor as local. That exact
// pair of symptoms has already been paid for once (EVEY_TRUSTED_PROXIES
// replacing instead of extending the default).
//
// Enricher.SetArtworkPostSaveHook is the same shape one layer up: a
// post-construction back-reference (the CommandService that owns the hook is
// built WITH the enricher, so it cannot go through NewEnricher). EnrichItem
// guards it with `if e.artworkPostSaveHook != nil` - correct for every test
// that never sets it, and indistinguishable from a boot that forgot to. Drop
// the wire.go line and artwork auto-download stops firing: no error, no log,
// enrichment still reports success, posters just never arrive.
//
// Static on purpose: the invariant is a property of the source, not of a
// running server, so it must fail on a laptop in under a second.
var bootWirings = []struct {
	name string
	// receiver is the identifier the call hangs off in cmd/evey: a package
	// name for a package-level setter, a variable for a method.
	receiver string
	setter   string
	// declaredIn is where the setter lives; a rename there must break this
	// guard loudly instead of turning the AST search into a no-op.
	declaredIn string
	// argSuffix, when set, is what the single argument must end with. Without
	// it a caller could satisfy the guard while passing the wrong source.
	argSuffix string
	// why is appended to the failure so the reader gets the consequence, not
	// just the missing line.
	why string
}{
	{
		name:       "trusted proxy list",
		receiver:   "ratelimit",
		setter:     "SetTrustedProxies",
		declaredIn: filepath.Join("internal", "pkg", "ratelimit", "limiter.go"),
		argSuffix:  ".Server.TrustedProxies",
		why: "the boot keeps the package-level default, which is wider than any configured list " +
			"and honors X-Forwarded-For from the whole LAN",
	},
	{
		name:       "persisted log level",
		receiver:   "eveylogger",
		setter:     "SetLevel",
		declaredIn: filepath.Join("internal", "pkg", "logger", "logger.go"),
		argSuffix:  ".Log.Level",
		why: "a level chosen in the admin panel applies live but is re-read only here: without " +
			"this line every restart silently reverts to the file default while the panel keeps " +
			"showing the stored value",
	},
	{
		name:       "intro detection chapter repo",
		receiver:   "introDetectionSvc",
		setter:     "WithChapterRepo",
		declaredIn: filepath.Join("internal", "app", "library", "intro_detection_service.go"),
		why: "chapterPass returns early on a nil repo, so the ONE authoritative intro source " +
			"(chapters labelled by the uploader) stops winning and every heuristic below it " +
			"silently takes over",
	},
	{
		name:       "intro fingerprint cache",
		receiver:   "introDetectionSvc",
		setter:     "WithFingerprintCache",
		declaredIn: filepath.Join("internal", "app", "library", "intro_detection_service.go"),
		why: "pHash fingerprints are recomputed on every pass instead of read from the cache - " +
			"correct, but it multiplies the catalogue sweep, which is already measured in days",
	},
	{
		name:       "intro audio fingerprint cache",
		receiver:   "introDetectionSvc",
		setter:     "WithAudioFingerprintCache",
		declaredIn: filepath.Join("internal", "app", "library", "intro_detection_service.go"),
		why: "Chromaprint fingerprints are recomputed by fpcalc on every pass instead of read " +
			"from the cache - same cost, on the slowest signal of the pipeline",
	},
	{
		name:       "artwork post-save hook",
		receiver:   "enricher",
		setter:     "SetArtworkPostSaveHook",
		declaredIn: filepath.Join("internal", "app", "metadata", "enricher.go"),
		why: "EnrichItem keeps a nil hook and skips artwork auto-download silently - enrichment " +
			"still reports success",
	},
}

func TestBootInstallsItsSilentWirings(t *testing.T) {
	calls := bootCallsByReceiverSetter(t)

	for _, w := range bootWirings {
		t.Run(w.name, func(t *testing.T) {
			assertSetterDeclared(t, w.declaredIn, w.setter)

			args, ok := calls[w.receiver+"."+w.setter]
			if !ok {
				t.Fatalf("cmd/evey never calls %s.%s: %s", w.receiver, w.setter, w.why)
			}
			for _, arg := range args {
				if arg == "nil" {
					t.Errorf("%s.%s(nil) wires nothing: %s", w.receiver, w.setter, w.why)
					continue
				}
				if w.argSuffix != "" && !strings.HasSuffix(arg, w.argSuffix) {
					t.Errorf("%s.%s(%s) must be handed a value ending in %q, otherwise what the "+
						"operator configured is not what takes effect",
						w.receiver, w.setter, arg, w.argSuffix)
				}
			}
		})
	}
}

// bootCallsByReceiverSetter maps "receiver.Setter" to the rendered arguments of
// every single-argument call found in cmd/evey.
func bootCallsByReceiverSetter(t *testing.T) map[string][]string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(repoRoot, "cmd", "evey", "*.go"))
	if err != nil {
		t.Fatalf("glob cmd/evey: %v", err)
	}

	fset := token.NewFileSet()
	out := map[string][]string{}
	var parsed int

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		parsed++
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recv, ok := sel.X.(*ast.Ident)
			if !ok || len(call.Args) != 1 {
				return true
			}
			key := recv.Name + "." + sel.Sel.Name
			out[key] = append(out[key], renderExpr(t, fset, call.Args[0]))
			return true
		})
	}

	if parsed == 0 {
		t.Fatal("no non-test file found under cmd/evey - the guard would pass vacuously")
	}
	return out
}

func assertSetterDeclared(t *testing.T, relPath, setter string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if !strings.Contains(string(body), setter+"(") {
		t.Fatalf("%s no longer declares %s - update this guard to the new name instead of letting "+
			"it match nothing", relPath, setter)
	}
}

func renderExpr(t *testing.T, fset *token.FileSet, expr ast.Expr) string {
	t.Helper()
	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, expr); err != nil {
		t.Fatalf("render argument: %v", err)
	}
	return sb.String()
}
