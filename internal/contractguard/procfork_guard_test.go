package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every media tool evey forks goes through procfork.Command: that is where the
// decode threads, the memory cap, the admission and the exit log live. A bare
// exec.Command in these packages is an ffmpeg that can OOM the host again
// (2026-09-01: four kills, thread count = nproc, no cap, call site unknown).
// Discarding the Finish with `_` leaks the admission reservation and loses the
// exit log, so it is refused too.
var procforkGuardedDirs = []string{
	filepath.Join("internal", "adapters", "driven", "ffmpeg"),
	filepath.Join("internal", "adapters", "driving", "http"),
}

// The HLS producers keep their own start/exit logging and run the hardware
// (VAAPI) path, which was explicitly left alone in the 2026-09-03 hardening.
var procforkExemptFiles = map[string]bool{
	"hls_transcoder.go": true,
	"hls_audio_only.go": true,
}

func TestProcfork_EveryForkGoesThroughCommand(t *testing.T) {
	root := repoRoot
	fset := token.NewFileSet()

	// Anti-vacuity: the entry point must exist where announced, or a rename
	// would turn the search below into a green no-op.
	forkFile := filepath.Join(root, "internal", "pkg", "procfork", "fork.go")
	pf, err := parser.ParseFile(fset, forkFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", forkFile, err)
	}
	if !declaresFunc(pf, "Command") {
		t.Fatalf("procfork.Command is no longer declared in %s - update this guard", forkFile)
	}

	var violations []string
	parsed := 0
	for _, dir := range procforkGuardedDirs {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || procforkExemptFiles[name] {
				continue
			}
			path := filepath.Join(root, dir, name)
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			parsed++
			ast.Inspect(f, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CallExpr:
					if pkg, fn := selectorOf(v.Fun); pkg == "exec" && strings.HasPrefix(fn, "Command") {
						violations = append(violations, fset.Position(v.Pos()).String()+": exec."+fn+" bypasses procfork")
					}
				case *ast.AssignStmt:
					if len(v.Rhs) != 1 || len(v.Lhs) != 3 {
						return true
					}
					call, ok := v.Rhs[0].(*ast.CallExpr)
					if !ok {
						return true
					}
					if pkg, fn := selectorOf(call.Fun); pkg == "procfork" && fn == "Command" {
						if id, ok := v.Lhs[1].(*ast.Ident); ok && id.Name == "_" {
							violations = append(violations, fset.Position(v.Pos()).String()+": procfork.Command Finish discarded")
						}
					}
				}
				return true
			})
		}
	}
	if parsed == 0 {
		t.Fatal("no Go file parsed - the guarded directories moved")
	}
	if len(violations) > 0 {
		t.Fatalf("%d fork(s) outside procfork:\n  %s", len(violations), strings.Join(violations, "\n  "))
	}
}

func selectorOf(e ast.Expr) (pkg, name string) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return id.Name, sel.Sel.Name
}

func declaresFunc(f *ast.File, name string) bool {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name {
			return true
		}
	}
	return false
}
