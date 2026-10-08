package ratings

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The clients carry a copy of byCountry to ask for the parental PIN before a
// title plays. It used to be a hand-kept 12-country subset of this table
// ("mirrors the backend") that nothing checked, so the same title asked for a
// PIN on the server's word but not on the client's. The copy is now a file
// this test RENDERS from byCountry and refuses to let drift: a rating added
// here without re-rendering fails the gate, with the command that fixes it.

var updateClientTable = flag.Bool("update", false, "rewrite the client ratings table from byCountry")

func clientTablePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "frontend", "packages", "client", "src", "hooks", "ratingsTable.ts")
}

// renderClientTable writes byCountry as a TypeScript module, countries and
// ratings in sorted order so the output is stable across runs.
func renderClientTable() string {
	var b strings.Builder
	b.WriteString("// Generated from internal/domain/ratings/ratings.go (byCountry) by\n")
	b.WriteString("// TestClientRatingsTableMirrorsTheServer - do not edit by hand. To refresh:\n")
	b.WriteString("//   go test ./internal/domain/ratings -run TestClientRatingsTableMirrorsTheServer -update\n")
	b.WriteString("// A -1 is a rating the board uses for \"not rated\": unknown, never an age.\n")
	b.WriteString("export const RATINGS_BY_COUNTRY: Record<string, Record<string, number>> = {\n")
	countries := make([]string, 0, len(byCountry))
	for c := range byCountry {
		countries = append(countries, c)
	}
	sort.Strings(countries)
	for _, c := range countries {
		table := byCountry[c]
		ratings := make([]string, 0, len(table))
		for r := range table {
			ratings = append(ratings, r)
		}
		sort.Strings(ratings)
		parts := make([]string, 0, len(ratings))
		for _, r := range ratings {
			parts = append(parts, fmt.Sprintf("%q: %d", r, table[r]))
		}
		fmt.Fprintf(&b, "  %q: { %s },\n", c, strings.Join(parts, ", "))
	}
	b.WriteString("};\n")
	return b.String()
}

func TestClientRatingsTableMirrorsTheServer(t *testing.T) {
	want := renderClientTable()
	path := clientTablePath()
	if *updateClientTable {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("rewrote %s", path)
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the client ratings table is missing (%v); render it with: go test ./internal/domain/ratings -run TestClientRatingsTableMirrorsTheServer -update", err)
	}
	if string(got) != want {
		t.Fatalf("frontend/packages/client/src/hooks/ratingsTable.ts drifted from byCountry: the clients would gate a title on a different age than the server. Re-render it with:\n  go test ./internal/domain/ratings -run TestClientRatingsTableMirrorsTheServer -update")
	}
}

// The rendering itself: every country and every rating of byCountry is in the
// output, in a form the TypeScript side reads back as the same numbers.
func TestRenderClientTable_CoversEveryEntry(t *testing.T) {
	out := renderClientTable()
	for c, table := range byCountry {
		if !strings.Contains(out, fmt.Sprintf("  %q: {", c)) {
			t.Errorf("country %s missing from the rendered table", c)
		}
		for r, age := range table {
			if !strings.Contains(out, fmt.Sprintf("%q: %d", r, age)) {
				t.Errorf("%s/%s = %d missing from the rendered table", c, r, age)
			}
		}
	}
}
