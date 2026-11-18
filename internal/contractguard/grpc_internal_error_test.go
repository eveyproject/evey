// A gRPC handler that fails turns the error into a status through
// internalError (grpcapi/helpers.go), which tells a client that gave up
// (context canceled, pq 57014) from a server fault: the first answers Canceled
// and logs at DEBUG, the second answers Internal and logs an ERROR. A handler
// that writes the Internal status by hand skips that test, so every navigation
// away from its screen becomes an ERROR line and a 5xx (prod: the resume rail,
// 27 such lines in three weeks). This guard keeps the hand-written form out.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// handWrittenInternalRe is the status the helper builds, written in place.
var handWrittenInternalRe = regexp.MustCompile(`status\.Errorf?\(\s*codes\.Internal,\s*"internal error"\s*\)`)

// grpcInternalErrorOwners may build that status themselves: the helper, and
// the interceptors (panic recovery, error sanitizer), which run after any
// handler code and have no client cancel to tell apart.
var grpcInternalErrorOwners = map[string]bool{
	"helpers.go": true,
}

func TestGRPCHandlers_InternalErrorGoesThroughTheHelper(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal", "adapters", "driving", "grpcapi")
	helper, err := os.ReadFile(filepath.Join(dir, "helpers.go"))
	if err != nil {
		t.Fatalf("read helpers.go: %v", err)
	}
	// Anti-vacuity: the helper must still exist and still build the status,
	// or a rename would leave this guard green on nothing.
	if !strings.Contains(string(helper), "func internalError(") || !handWrittenInternalRe.Match(helper) {
		t.Fatal("grpcapi/helpers.go no longer declares internalError building the Internal status - update this guard")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var violations []string
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || grpcInternalErrorOwners[name] {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			if handWrittenInternalRe.MatchString(line) {
				violations = append(violations, name+":"+strconv.Itoa(i+1))
			}
		}
	}
	if scanned < 20 {
		t.Fatalf("only %d handler files scanned - the grpcapi layout moved, update this guard", scanned)
	}
	if len(violations) > 0 {
		t.Errorf("Internal status written by hand instead of internalError(ctx, op, err): %s", strings.Join(violations, ", "))
	}
}
