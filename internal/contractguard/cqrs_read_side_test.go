// CQRS read-side guard (ADR-0009): the split into XCommandRepo / XQueryRepo
// only means something if the read side stays read-only - both the port a
// query service is handed and the query service itself. Until now that was
// checked by hand at each audit ("0 writes in a query service"); this makes
// it a red test.
//
// Two mechanical checks, both on method NAMES because that is the contract
// the split rests on (ADR-0009: "Get*, List*, Count*, anything that doesn't
// mutate" on the query side):
//   - no interface named *QueryRepo under internal/domain declares a method
//     whose name is a mutation verb;
//   - no internal/app/*/query_service.go calls a method whose name is a
//     mutation verb, or Publish (a query service emits no domain event).
package contractguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// mutationVerb matches a method name that starts with a write verb followed
// by the end of the name, a digit or an upper-case letter (so "Setup",
// "Settings" or "Addr" are not caught).
var mutationVerb = regexp.MustCompile(`^(Create|Update|Delete|Upsert|Insert|Save|Persist|Store|Mark|Touch|Revoke|Increment|Decrement|Reset|Purge|Publish|Remove|Rename|Set|Put|Write|Apply|Assign|Attach|Detach|Enable|Disable|Toggle|Ack|Acknowledge|Bump|Rotate|Replace|Move|Clear|Append|Record)([A-Z0-9].*)?$`)

// readSideCallExceptions are selector calls a query service legitimately
// makes that a verb match would catch: the OpenTelemetry span surface.
var readSideCallExceptions = map[string]bool{
	"SetAttributes": true, "SetStatus": true, "SetName": true, "RecordError": true, "AddEvent": true,
}

func TestQueryRepoPortsDeclareNoMutation(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var violations []string
	err := filepath.WalkDir(domainRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, path)
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(ts.Name.Name, "QueryRepo") {
				return true
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, m := range it.Methods.List {
				for _, name := range m.Names {
					if mutationVerb.MatchString(name.Name) {
						violations = append(violations, rel+": "+ts.Name.Name+"."+name.Name+" is a write on a query port")
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Errorf("a *QueryRepo port must stay read-only (ADR-0009) - move the method to the CommandRepo:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func TestQueryServicesDoNotWrite(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(repoRoot + "/internal/app/*/query_service*.go")
	if err != nil {
		t.Fatal(err)
	}
	var scanned int
	var violations []string
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(repoRoot, path)
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || readSideCallExceptions[sel.Sel.Name] {
				return true
			}
			if mutationVerb.MatchString(sel.Sel.Name) {
				violations = append(violations, fmt.Sprintf("%s:%d calls .%s(", rel, fset.Position(sel.Pos()).Line, sel.Sel.Name))
			}
			return true
		})
	}
	if scanned < 10 {
		t.Fatalf("expected the query services under internal/app/*/query_service.go, scanned %d files - has the layout moved?", scanned)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Errorf("a query service reads, it never writes nor publishes (ADR-0009) - move the call to the command service:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
