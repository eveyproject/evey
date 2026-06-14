package postgres

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNoDirectDBExecInRepos is the hard anti-regression gate for TxUnit.
//
// Every repo statement must go through txFromCtx(ctx, r.db) so that an ambient
// transaction opened by TxUnit.Do (uow.Do) is honored. A direct call on the raw
// *sql.DB handle — r.db.ExecContext / QueryContext / QueryRowContext — silently
// runs OUTSIDE the caller's transaction, so a handler wrapping several writes in
// uow.Do loses atomicity without any error.
//
// The forbidigo lint rule flags this too, but CI runs golangci-lint in
// only-new-issues mode, which grandfathers the pre-migration baseline. This test
// runs on the full tree every `make test`, so the whole package stays clean.
//
// Repos that manage their own transaction keep using r.db.BeginTx followed by
// tx.ExecContext (receiver "tx"), which this pattern intentionally does not match.
func TestNoDirectDBExecInRepos(t *testing.T) {
	pat := regexp.MustCompile(`\.db\.(ExecContext|QueryContext|QueryRowContext)\(`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if pat.MatchString(line) {
				t.Errorf("%s:%d bypasses TxUnit — use txFromCtx(ctx, <recv>.db).XContext instead:\n\t%s",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
}
