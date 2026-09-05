package contractguard

// Every sort field a client can ask for must exist in the SQL column map.
//
// The failure this guards is silent, which is why it needs a test rather than
// care: postgres.resolveSortKey falls back to "title" on an unknown key. A sort
// field the gRPC layer converts but the map does not know therefore does not
// error - it returns the library in title order, on every client at once, and
// looks like the feature was never wired. That is what "Order by rating" did
// until 2026-09-04: the converter emitted "rating", the map only held
// "community_rating", and web, mobile and TV all silently sorted by title.
//
// Parsed with go/ast rather than grepped: these are Go map literals, and a
// regex over them would break on a reformat or a comment and pass for the wrong
// reason.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// mapLiteralEntries returns the keys and values of a top-level
// `var <name> = map[string]string{...}` (or map[SomeType]string) declaration.
// Keys that are not plain string literals (e.g. proto enum constants) come back
// as "", which is fine: this guard only cares about the string VALUES on the
// converter side and the string KEYS on the SQL side.
func mapLiteralEntries(t *testing.T, path, varName string) (keys, values []string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != varName || len(vs.Values) != 1 {
			return true
		}
		lit, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		found = true
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keys = append(keys, stringLit(kv.Key))
			values = append(values, stringLit(kv.Value))
		}
		return false
	})
	if !found {
		t.Fatalf("%s: variable %q not found - was it renamed? this guard must be updated with it", path, varName)
	}
	return keys, values
}

// stringLit unquotes a basic string literal, or returns "" for anything else.
func stringLit(e ast.Expr) string {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return ""
	}
	return s
}

func TestEverySortFieldTheAPIAcceptsHasASQLColumn(t *testing.T) {
	_, emitted := mapLiteralEntries(t,
		filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/library_converters.go"),
		"mediaSortFieldFromProto")
	sqlKeys, _ := mapLiteralEntries(t,
		filepath.Join(repoRoot, "internal/adapters/driven/postgres/sort_columns.go"),
		"SortColumnMap")

	known := make(map[string]bool, len(sqlKeys))
	for _, k := range sqlKeys {
		known[k] = true
	}

	if len(emitted) == 0 {
		t.Fatal("no sort fields parsed from the gRPC converter - the guard would pass vacuously")
	}
	for _, field := range emitted {
		if field == "" {
			continue
		}
		if !known[field] {
			t.Errorf("the API accepts sort field %q but postgres.SortColumnMap has no such key: "+
				"resolveSortKey will silently fall back to title, so this sort does nothing on every client. "+
				"Add %q to internal/adapters/driven/postgres/sort_columns.go", field, field)
		}
	}
}

// switchCaseStrings returns every string literal used as a `case` in the named
// top-level function.
func switchCaseStrings(t *testing.T, path, funcName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			return true
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if s := stringLit(e); s != "" {
					out = append(out, s)
				}
			}
			return true
		})
		return false
	})
	if !found {
		t.Fatalf("%s: function %q not found - was it renamed? this guard must be updated with it", path, funcName)
	}
	return out
}

// A sort field needs TWO things, and having only one is silent both ways.
//
// The test above pins the SQL column. This pins the other half: the value the
// keyset cursor encodes for the LAST row of a page. mediaItemSortValue answers
// by sort key, and its default returns the item's UUID - fine as a fallback for
// a key that never paginates, fatal for one that does.
//
// That is exactly what shipped on 2026-09-05. "rating" was added to
// SortColumnMap so the ORDER BY finally worked, the guard above went green, and
// nothing looked at this switch - which only knew "community_rating". Sorting a
// library by rating therefore built a cursor holding a UUID, and page 2 bound
// that UUID against a numeric column: HTTP 500. From the viewer's seat, "échec
// du chargement" the moment they scrolled.
func TestEverySortFieldTheAPIAcceptsCanBuildACursorValue(t *testing.T) {
	_, emitted := mapLiteralEntries(t,
		filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/library_converters.go"),
		"mediaSortFieldFromProto")
	cases := switchCaseStrings(t,
		filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/library_handler_mediaitems.go"),
		"mediaItemSortValue")

	known := make(map[string]bool, len(cases))
	for _, c := range cases {
		known[c] = true
	}

	if len(emitted) == 0 || len(cases) == 0 {
		t.Fatalf("parsed %d sort fields and %d cursor cases - the guard would pass vacuously", len(emitted), len(cases))
	}
	for _, field := range emitted {
		if field == "" {
			continue
		}
		if !known[field] {
			t.Errorf("the API accepts sort field %q but mediaItemSortValue has no case for it: "+
				"the cursor would carry the item's UUID as its sort value, and the next page binds that "+
				"UUID against the sort column - a 500 the moment a viewer scrolls. "+
				"Add a %q case in internal/adapters/driving/grpcapi/library_handler_mediaitems.go", field, field)
		}
	}
}
