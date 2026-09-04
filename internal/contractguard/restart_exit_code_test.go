package contractguard

// The admin "restart server" button is a contract between two languages: the Go
// process exits with a number, and docker/entrypoint.sh decides from that number
// whether to relaunch it or let the container die.
//
// Both halves fail silently if they drift. A Go-side code the script does not
// recognise makes "restart" a permanent SHUTDOWN of the user's server; a script
// that relaunches on a code Go never emits turns a real crash into a boot loop.
// Neither shows up in any test that runs only one side.
//
// It also pins the guard the RPC keys on. Availability used to be inferred from
// `getpid() == 1`, which was exactly backwards: the entrypoint exec'd evey into
// PID 1 only when there was NO embedded database, so the single-file install the
// project ships answered "server restart is not available in this deployment"
// (prod, 2026-09-04). The entrypoint now says so explicitly with
// EVEY_SUPERVISED, and this makes sure the two spellings stay one word.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// goConstInt returns the value of a top-level `const <name> = <int>`.
func goConstInt(t *testing.T, path, name string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out int
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != name || len(vs.Values) != 1 {
			return true
		}
		lit, ok := vs.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return true
		}
		v, cErr := strconv.Atoi(lit.Value)
		if cErr != nil {
			t.Fatalf("%s: %s = %q is not an int", path, name, lit.Value)
		}
		out, found = v, true
		return false
	})
	if !found {
		t.Fatalf("%s: const %s not found - was it renamed? this guard must be updated with it", path, name)
	}
	return out
}

var shAssign = func(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + name + `=([0-9]+)\s*$`)
}

func TestRestartExitCodeAgreesWithTheEntrypoint(t *testing.T) {
	goCode := goConstInt(t, filepath.Join(repoRoot, "cmd/evey/main.go"), "restartExitCode")

	raw, err := os.ReadFile(filepath.Join(repoRoot, "docker/entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}
	script := string(raw)

	m := shAssign("EVEY_RESTART_EXIT_CODE").FindStringSubmatch(script)
	if m == nil {
		t.Fatal("docker/entrypoint.sh no longer assigns EVEY_RESTART_EXIT_CODE=<n>: " +
			"the admin restart button has nothing to land on, and would shut the server down for good")
	}
	shCode, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("EVEY_RESTART_EXIT_CODE = %q is not a number", m[1])
	}

	if goCode != shCode {
		t.Errorf("restart exit code disagrees: Go exits with %d, docker/entrypoint.sh relaunches on %d. "+
			"The admin restart button would drain the server and never bring it back", goCode, shCode)
	}

	// The conventional ranges a real failure lands in. Sharing one would make a
	// crash look like a restart request and put the container in a boot loop.
	if goCode <= 2 || (goCode >= 126 && goCode <= 165) || goCode > 255 {
		t.Errorf("restart exit code %d collides with a conventional range "+
			"(0-2 shell, 126-127 exec failure, 128+n signal): a genuine crash would be relaunched forever", goCode)
	}
}

// The supervisor marker is the whole availability check. If the entrypoint stops
// exporting it, every deployment silently loses the restart button - which is
// the shape of the bug this replaced, just with a different cause.
func TestEntrypointDeclaresItSupervisesTheServer(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docker/entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}
	script := string(raw)

	if shAssign("EVEY_SUPERVISED").FindStringSubmatch(script) == nil {
		t.Error("docker/entrypoint.sh no longer assigns EVEY_SUPERVISED=1: selfRestartFn returns nil " +
			"and the admin panel reports the restart unavailable on every deployment")
	}
	if !strings.Contains(script, "export EVEY_SUPERVISED") {
		t.Error("EVEY_SUPERVISED is assigned but not exported: the child evey process never sees it")
	}

	// And the Go side must still be reading that exact name.
	goSrc, err := os.ReadFile(filepath.Join(repoRoot, "cmd/evey/grpc.go"))
	if err != nil {
		t.Fatalf("read grpc.go: %v", err)
	}
	if !strings.Contains(string(goSrc), `os.Getenv("EVEY_SUPERVISED")`) {
		t.Error("cmd/evey/grpc.go no longer reads EVEY_SUPERVISED - the two halves of the restart " +
			"availability check have drifted apart")
	}
}

// The supervisor must actually be a loop. Restoring the `exec /evey` handoff
// would put evey back in PID 1 with nothing above it, so the restart code would
// stop the container and leave the outcome to a Docker `restart:` policy that
// nothing inside the container can read.
func TestEntrypointSupervisesRatherThanExecsTheServer(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docker/entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint: %v", err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") {
			continue
		}
		if code == `exec /evey "$@"` {
			t.Error(`docker/entrypoint.sh exec's the server again: evey becomes PID 1, ` +
				`nothing can relaunch it, and the admin restart turns into a shutdown`)
		}
	}
	if !strings.Contains(string(raw), `/evey "$@" &`) {
		t.Error("docker/entrypoint.sh no longer starts the server as a supervised child")
	}
}
