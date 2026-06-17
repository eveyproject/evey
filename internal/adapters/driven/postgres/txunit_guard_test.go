package postgres_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every repo query/exec must go through txFromCtx(ctx, r.db) so an ambient
// uow.Do(...) transaction is honored — a direct r.db.ExecContext /
// QueryContext / QueryRowContext escapes that transaction and silently
// auto-commits, breaking the all-or-nothing guarantee of a multi-write
// operation. This guard fails if any repo reintroduces such a direct call.
// (BeginTx in txunit.go and the txFromCtx fallback arg `r.db` are not matched:
// the pattern requires `.db.` immediately followed by an Exec/Query call.)
func TestNoDirectDBCallsBypassingTx(t *testing.T) {
	forbidden := regexp.MustCompile(`\.db\.(Exec|Query|QueryRow)Context\b`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if forbidden.MatchString(line) {
				t.Errorf("%s:%d bypasses txFromCtx with a direct DB call — route it through "+
					"txFromCtx(ctx, r.db) so uow.Do transactions are honored:\n\t%s",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
}
