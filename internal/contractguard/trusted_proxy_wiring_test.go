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

// internal/pkg/ratelimit carries a package-level trusted-proxy list that is
// deliberately WIDER than any operator would configure (the whole RFC1918
// space). It is the value in force during the boot window and in unit tests
// only: cmd/evey/http.go replaces it with cfg.Server.TrustedProxies, and the
// comment above the literal says so.
//
// Nothing enforced that replacement. Deleting the single call site leaves the
// build green and every trusted-proxy suite green (internal/config computes
// the list, internal/pkg/ratelimit is handed one explicitly - neither observes
// the boot), while production silently keeps the wide default: any host on the
// LAN can then rename itself with X-Forwarded-For, which collapses the rate
// limit into one shared bucket and makes the remote-access gate classify every
// visitor as local. That exact pair of symptoms has already been paid for once
// (EVEY_TRUSTED_PROXIES replacing instead of extending the default).
//
// Static on purpose: the invariant is "the boot installs the CONFIGURED list",
// which is a property of the source, not of a running server.
const (
	trustedProxySetter   = "SetTrustedProxies"
	trustedProxyPkg      = "ratelimit"
	trustedProxyConfiged = ".Server.TrustedProxies"
)

func TestBootInstallsTheConfiguredTrustedProxyList(t *testing.T) {
	assertTrustedProxySetterExists(t)

	files, err := filepath.Glob(filepath.Join(repoRoot, "cmd", "evey", "*.go"))
	if err != nil {
		t.Fatalf("glob cmd/evey: %v", err)
	}

	fset := token.NewFileSet()
	var parsed int
	var args []string

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
			if !ok || sel.Sel.Name != trustedProxySetter {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != trustedProxyPkg {
				return true
			}
			if len(call.Args) != 1 {
				t.Errorf("%s: %s.%s called with %d arguments, want 1",
					path, trustedProxyPkg, trustedProxySetter, len(call.Args))
				return true
			}
			args = append(args, renderExpr(t, fset, call.Args[0]))
			return true
		})
	}

	if parsed == 0 {
		t.Fatal("no non-test file found under cmd/evey - the guard would pass vacuously")
	}
	if len(args) == 0 {
		t.Fatalf("cmd/evey never calls %s.%s: the boot keeps the package-level default, which is "+
			"wider than any configured list and honors X-Forwarded-For from the whole LAN",
			trustedProxyPkg, trustedProxySetter)
	}

	for _, arg := range args {
		if !strings.HasSuffix(arg, trustedProxyConfiged) {
			t.Errorf("%s.%s(%s) does not install the configured list: the argument must end in %q, "+
				"otherwise the operator's EVEY_TRUSTED_PROXIES is not what takes effect",
				trustedProxyPkg, trustedProxySetter, arg, trustedProxyConfiged)
		}
	}
}

// A rename of the setter would leave the AST search matching nothing, which
// reads exactly like a deleted call. Anchor the guard on the real symbol so a
// rename fails here loudly instead of turning the check into a no-op.
func assertTrustedProxySetterExists(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "internal", "pkg", "ratelimit", "limiter.go"))
	if err != nil {
		t.Fatalf("read ratelimit limiter: %v", err)
	}
	if !strings.Contains(string(body), "func "+trustedProxySetter+"(") {
		t.Fatalf("internal/pkg/ratelimit no longer declares %s - update this guard to the new name "+
			"instead of letting it match nothing", trustedProxySetter)
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
