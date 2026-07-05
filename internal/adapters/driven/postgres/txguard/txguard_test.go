// Package txguard holds the static anti-regression gates for the postgres
// repositories' TxUnit contract. They live in their OWN package -- deliberately
// separate from postgres_test -- because that package's TestMain boots a
// testcontainers Postgres (~130s, needs Docker), which keeps the whole suite out
// of `make pre-push`. These guards only parse .go source with a regex: no DB, no
// Docker, sub-second. Running them here lets pre-push catch a TxUnit bypass
// locally instead of surprising you on CI.
package txguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// postgresDir is the repo package these guards scan, relative to this test's
// working directory (go test runs with the package dir as cwd).
const postgresDir = ".."

// scanRepoSources runs fn(filename, lineNo, line) over every non-test .go file
// in the postgres package, reporting via t.Errorf on the caller's behalf.
func scanRepoSources(t *testing.T, pat *regexp.Regexp, msg string) {
	t.Helper()
	entries, err := os.ReadDir(postgresDir)
	if err != nil {
		t.Fatalf("read postgres package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(postgresDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if pat.MatchString(line) {
				t.Errorf("%s:%d %s:\n\t%s", name, i+1, msg, strings.TrimSpace(line))
			}
		}
	}
}

// TestNoDirectDBExecInRepos is the hard anti-regression gate for TxUnit.
//
// Every repo statement must go through txFromCtx(ctx, r.db) so that an ambient
// transaction opened by TxUnit.Do (uow.Do) is honored. A direct call on the raw
// *sql.DB handle -- r.db.ExecContext / QueryContext / QueryRowContext -- silently
// runs OUTSIDE the caller's transaction, so a handler wrapping several writes in
// uow.Do loses atomicity without any error.
//
// The forbidigo lint rule flags this too, but CI runs golangci-lint in
// only-new-issues mode, which grandfathers the pre-migration baseline. This test
// runs on the full tree, so the whole package stays clean.
//
// Repos that manage their own transaction keep using r.db.BeginTx followed by
// tx.ExecContext (receiver "tx"), which this pattern intentionally does not match.
func TestNoDirectDBExecInRepos(t *testing.T) {
	pat := regexp.MustCompile(`\.db\.(ExecContext|QueryContext|QueryRowContext)\(`)
	scanRepoSources(t, pat, "bypasses TxUnit -- use txFromCtx(ctx, <recv>.db).XContext instead")
}

// TestNoDirectDBCallsBypassingTx is the companion guard: same intent, matching
// the .db.X call site rather than the full argument list. Kept as a distinct
// pattern so either spelling drift is caught.
// (BeginTx in txunit.go and the txFromCtx fallback arg `r.db` are not matched:
// the pattern requires `.db.` immediately followed by an Exec/Query call.)
func TestNoDirectDBCallsBypassingTx(t *testing.T) {
	pat := regexp.MustCompile(`\.db\.(Exec|Query|QueryRow)Context\b`)
	scanRepoSources(t, pat, "bypasses txFromCtx with a direct DB call -- route it through "+
		"txFromCtx(ctx, r.db) so uow.Do transactions are honored")
}
