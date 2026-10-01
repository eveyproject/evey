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

// gRPC handlers are built as STRUCT LITERALS in grpcapi/server.go, and most of
// their ports are read behind a nil guard. Leaving a key out of the literal is
// therefore silent in the strongest sense: the build passes, every handler test
// passes (each one builds its own literal with exactly the ports it wants to
// exercise), and the port is simply never consulted in production.
//
// What that costs is not hypothetical. `userQuery` is what
// resolveParentalFilterFromUserQuery reads; a nil one returns the no-op filter
// (maxAge -1), so mediaItemBlockedByParental answers false for everything and
// parental control is off on that surface. `libQuery` is what hydrates the
// items the access and parental checks are written against - an empty map makes
// both filters skip every row (fixed on favorites and playlists on 2026-08-24).
//
// So the rule is the blunt one: a handler registered in server.go must be
// handed EVERY field it declares. Exceptions are listed below with a reason,
// which is what separates a decision from an omission.
var handlerFieldExceptions = map[string]map[string]string{
	"adminSystemHandler": {
		"cpuBudgetFn":    "optional probe, explicitly nil-checked with a documented fallback",
		"meterMu":        "internal state",
		"meterLastTotal": "internal state",
		"meterLastAt":    "internal state",
	},
	"adminIntroHandler": {
		"backfillMu":      "internal state",
		"backfillRunning": "internal state",
	},
	"photoHandler": {
		"serverCtx": "late-bound; the code documents the nil case",
	},
	"playbackHandler": {
		// The HTTP pipeline owns the resolver; main binds it with
		// SetPlaybackDecider once buildHTTPHandler returns, before Start
		// (TestMainBindsThePlaybackDeciderBeforeServing keeps that order).
		"decider":      "late-bound; nil decides on the unwired resolver, the pre-binding behavior",
		"closingWarns": "internal state: a zero-value map of warned sessions",
	},
}

const minRegisteredHandlers = 20

func TestRegisteredHandlersGetEveryPortTheyDeclare(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal", "adapters", "driving", "grpcapi")
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob grpcapi: %v", err)
	}

	declared := map[string][]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(ts.Name.Name, "Handler") {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			var fields []string
			for _, f := range st.Fields.List {
				// Embedded fields (the generated Unimplemented*Server) have no
				// names and nothing to wire.
				for _, name := range f.Names {
					fields = append(fields, name.Name)
				}
			}
			declared[ts.Name.Name] = fields
			return true
		})
	}

	serverFile, err := parser.ParseFile(fset, filepath.Join(dir, "server.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	usedException := map[string]bool{}
	var literals int

	ast.Inspect(serverFile, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		ident, ok := lit.Type.(*ast.Ident)
		if !ok || !strings.HasSuffix(ident.Name, "Handler") {
			return true
		}
		fields, known := declared[ident.Name]
		if !known {
			t.Errorf("server.go builds %s but no struct of that name was found in the package", ident.Name)
			return true
		}
		literals++

		keyed := map[string]bool{}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok {
				keyed[key.Name] = true
			}
		}

		var missing []string
		for _, f := range fields {
			if keyed[f] {
				continue
			}
			// The exception's value is its reason: it is there to be read in
			// the source, not consumed here.
			if _, excused := handlerFieldExceptions[ident.Name][f]; excused {
				usedException[ident.Name+"."+f] = true
				continue
			}
			missing = append(missing, f)
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("server.go builds %s without %v: the field stays nil and every `if h.<field> != nil` "+
				"guard reading it turns into a silent pass. Wire it, or add it to handlerFieldExceptions "+
				"with the reason it is meant to be nil.", ident.Name, missing)
		}
		return true
	})

	if literals < minRegisteredHandlers {
		t.Fatalf("found %d handler literals in server.go, want at least %d - the parser has drifted "+
			"and the guard would pass vacuously", literals, minRegisteredHandlers)
	}

	// A stale exception is how an allowlist rots into a blanket waiver: the
	// field gets wired (or deleted) and the excuse survives, ready to excuse
	// the next omission that happens to share its name.
	for handler, fields := range handlerFieldExceptions {
		for field := range fields {
			if !usedException[handler+"."+field] {
				t.Errorf("handlerFieldExceptions has %s.%s but that field is now wired (or gone) - "+
					"remove the exception instead of leaving it to cover a future omission", handler, field)
			}
		}
	}
}
