// Domain isolation guard (ADR-0001): a bounded context under internal/domain
// may import internal/domain/shared and its own package tree, nothing else
// from internal/domain, and never internal/app or internal/adapters.
// "Cross-context collaboration goes through app-service interfaces or the
// shared event bus, never direct domain imports."
//
// The one leak this closed (transcoding -> library, for HLSProcessOpts.Streams)
// lived for months behind a comment explaining why it was cheaper. It was:
// the fix is a nine-method interface. The guard exists so the next one is a
// red test, not a comment.
package contractguard

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	domainRoot   = repoRoot + "/internal/domain"
	domainImport = "github.com/eveyproject/evey/internal/domain/"
	appImport    = "github.com/eveyproject/evey/internal/app/"
	adapterImprt = "github.com/eveyproject/evey/internal/adapters/"
)

func TestDomainContextsDoNotImportEachOther(t *testing.T) {
	t.Parallel()
	var violations []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(domainRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(domainRoot, path)
		if err != nil {
			return err
		}
		own := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			spec := strings.Trim(imp.Path.Value, `"`)
			switch {
			case strings.HasPrefix(spec, appImport), strings.HasPrefix(spec, adapterImprt):
				violations = append(violations, rel+" imports "+spec+" (domain must not depend on app/adapters)")
			case strings.HasPrefix(spec, domainImport):
				ctx := strings.SplitN(strings.TrimPrefix(spec, domainImport), "/", 2)[0]
				if ctx != "shared" && ctx != own {
					violations = append(violations, rel+" imports "+spec+" (cross-context domain import)")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", domainRoot, err)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Errorf("ADR-0001: %d domain import violation(s):\n  %s", len(violations), strings.Join(violations, "\n  "))
	}
}
