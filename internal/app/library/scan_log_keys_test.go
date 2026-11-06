package library

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanLogKeys_UseTheCanonicalCorrelationKeys: the admin log search and any
// log query join records on media_id, library_id and path. A scan line keyed
// mediaItemID or libraryID is invisible to a search on the canonical key, so a
// scan failure could not be tied to the item or library it hit.
func TestScanLogKeys_UseTheCanonicalCorrelationKeys(t *testing.T) {
	nonCanonical := map[string]string{
		"mediaItemID":   "media_id",
		"media_item_id": "media_id",
		"media_item.id": "media_id",
		"mediaID":       "media_id",
		"id":            "media_id",
		"libraryID":     "library_id",
		"filePath":      "path",
	}

	for _, file := range []string{"scan_service.go", "scan_service_tvshow.go"} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "slog" {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				if want, bad := nonCanonical[key]; bad {
					assert.Failf(t, "non-canonical slog key", "%s: key %q, want %q", fset.Position(lit.Pos()), key, want)
				}
				return true
			})
		}
	}
}
