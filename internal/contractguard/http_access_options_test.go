package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The access controls of the HTTP layer reach most routes as POSITIONAL
// arguments, where dropping one is a compile error and nothing needs guarding.
// Three do not: they arrive as FUNCTIONAL OPTIONS, and an option nobody passes
// is simply an option nobody passes.
//
//   - withParental(...) on the media serve handlers (byte range, decision,
//     stream token);
//   - withLibraryLister(...), which confines a streamed path to the library
//     roots;
//   - withSyncPlayParental(...) on the watch-together WebSocket.
//
// All three are read behind a nil test - `checkParental` no-ops on a nil
// checker and `h.libLister != nil` gates the confinement - by design, because
// a handler built in a unit test has neither a user service nor a library
// service to consult. That design is what makes the omission silent: delete
// the option from server.go and the build passes, every handler test passes
// (each one passes its own dependency explicitly, which is exactly why they
// cannot see this), and a kids profile can stream any title in the library
// while a poisoned path can be read off the disk.
//
// The syncplay one also guards `serveCastNext` and the master concat - the
// Chromecast surfaces - so it is the path most likely to be exercised by
// someone who is not looking for a parental bug.
//
// Static on purpose, like the boot wirings next door: this is a property of
// the source, and it must fail on a laptop in under a second.
var httpAccessOptions = []struct {
	// option is the functional-option constructor that carries the checker.
	option string
	// declaredIn is where it lives; a rename there must break this guard
	// loudly instead of turning the search into a no-op.
	declaredIn string
	// passedTo, when set, is the registration call that must receive it -
	// stronger than "mentioned somewhere in the file".
	passedTo string
	why      string
}{
	{
		option:     "withParental",
		declaredIn: filepath.Join("internal", "adapters", "driving", "http", "media_handler.go"),
		why: "the raw byte/decision/stream-token routes stop asking whether the viewer's profile " +
			"may see the title, so a kids profile plays anything by asking for its bytes directly",
	},
	{
		option:     "withLibraryLister",
		declaredIn: filepath.Join("internal", "adapters", "driving", "http", "media_handler.go"),
		why: "`h.libLister != nil` is the whole path-confinement check, so a poisoned DB path " +
			"turns the stream endpoint into an arbitrary-file read - the same defence " +
			"serveDownload applies, and the same silent omission",
	},
	{
		option:     "withSyncPlayParental",
		declaredIn: filepath.Join("internal", "adapters", "driving", "http", "syncplay_handler.go"),
		passedTo:   "registerSyncPlayRoutes",
		why: "a member joining a watch-together room is no longer gated, which also covers " +
			"serveCastNext and the master concat - the Chromecast surfaces",
	},
}

func TestAccessOptionsReachTheRoutesThatNeedThem(t *testing.T) {
	serverGo := filepath.Join(repoRoot, "internal", "adapters", "driving", "http", "server.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, serverGo, nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	// Every option constructor called anywhere in the file, and every option
	// handed to a given registration call.
	called := map[string]bool{}
	optionsOf := map[string]map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		called[name.Name] = true
		for _, arg := range call.Args {
			inner, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			innerName, ok := inner.Fun.(*ast.Ident)
			if !ok {
				continue
			}
			if optionsOf[name.Name] == nil {
				optionsOf[name.Name] = map[string]bool{}
			}
			optionsOf[name.Name][innerName.Name] = true
		}
		return true
	})

	for _, w := range httpAccessOptions {
		t.Run(w.option, func(t *testing.T) {
			assertFuncDeclared(t, w.declaredIn, w.option)

			if !called[w.option] {
				t.Fatalf("server.go never calls %s(...): %s", w.option, w.why)
			}
			if w.passedTo != "" && !optionsOf[w.passedTo][w.option] {
				t.Errorf("%s(...) is called but not handed to %s(...): %s", w.option, w.passedTo, w.why)
			}
		})
	}
}

// assertFuncDeclared fails when the option constructor no longer exists under
// that name, so a rename cannot quietly turn the guard above into a search for
// something that is not there any more.
func assertFuncDeclared(t *testing.T, relPath, name string) {
	t.Helper()
	path := filepath.Join(repoRoot, relPath)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return
		}
	}
	t.Fatalf("%s no longer declares %s - rename the guard's entry too", relPath, name)
}
