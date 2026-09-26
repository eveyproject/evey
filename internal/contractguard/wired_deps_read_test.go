package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The composition root hands HTTPServer and GRPCServerDeps their dependencies
// field by field. A field that is assigned in cmd/evey but never read by the
// package that declares it compiles, passes every test and does nothing: an
// audit on 2026-09-23 found seven (HTTP ActivityQuerySvc, FavoriteSvc,
// StarRatingSvc, UserRatingRepo, PushSubRepo, VAPIDPublicKey; gRPC BioEnricher),
// one of which kept a whole feature reachable in name only. Worse is the field
// that looks like a control - a rate limiter wired and never consulted is a
// limit that is not applied.
//
// So every exported field must be READ in its own package (a selector that is
// not the target of an assignment). An exception says why, and one that no
// longer applies (the field is read, or gone) fails too, so the list cannot rot.
var wiredDepsExceptions = map[string]map[string]string{}

func TestWiredDepsAreRead(t *testing.T) {
	t.Parallel()
	targets := []struct{ dir, typ string }{
		{"internal/adapters/driving/http", "HTTPServer"},
		{"internal/adapters/driving/grpcapi", "GRPCServerDeps"},
	}
	for _, tg := range targets {
		fields, reads := wiredFieldsAndReads(t, filepath.Join(repoRoot, tg.dir), tg.typ)
		if len(fields) < 10 {
			t.Fatalf("%s: found %d exported fields on %s - has the struct moved?", tg.dir, len(fields), tg.typ)
		}
		exc := wiredDepsExceptions[tg.typ]
		var unread, stale []string
		for _, f := range fields {
			_, excepted := exc[f]
			switch {
			case reads[f] == 0 && !excepted:
				unread = append(unread, f)
			case reads[f] > 0 && excepted:
				stale = append(stale, f+" (now read)")
			}
		}
		declared := map[string]bool{}
		for _, f := range fields {
			declared[f] = true
		}
		for f := range exc {
			if !declared[f] {
				stale = append(stale, f+" (no longer declared)")
			}
		}
		sort.Strings(unread)
		sort.Strings(stale)
		if len(unread) > 0 {
			t.Errorf("%s.%s: wired but never read in %s - remove the field and its assignment, or list it in wiredDepsExceptions with a reason:\n  %s",
				filepath.Base(tg.dir), tg.typ, tg.dir, strings.Join(unread, "\n  "))
		}
		if len(stale) > 0 {
			t.Errorf("%s: wiredDepsExceptions entries that no longer apply - drop them:\n  %s", tg.typ, strings.Join(stale, "\n  "))
		}
	}
}

// wiredFieldsAndReads returns the exported fields of struct typ declared in dir,
// and how many times each name is read as a selector in dir's non-test files.
// Selectors are matched by name only: a same-named field on another type counts
// as a read, which errs towards letting a field through, never towards a false
// alarm.
func wiredFieldsAndReads(t *testing.T, dir, typ string) ([]string, map[string]int) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var fields []string
	reads := map[string]int{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		written := map[*ast.SelectorExpr]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.TypeSpec:
				if st, ok := x.Type.(*ast.StructType); ok && x.Name.Name == typ {
					for _, fl := range st.Fields.List {
						for _, name := range fl.Names {
							if name.IsExported() {
								fields = append(fields, name.Name)
							}
						}
					}
				}
			case *ast.AssignStmt:
				for _, lhs := range x.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok {
						written[sel] = true
					}
				}
			case *ast.SelectorExpr:
				if !written[x] {
					reads[x.Sel.Name]++
				}
			}
			return true
		})
	}
	return fields, reads
}
