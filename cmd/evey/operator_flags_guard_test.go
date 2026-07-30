package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The operator flags read at boot through resolveFlag(dbKey, envVar) are set as
// free-text server_config rows. They were settable from the admin panel but
// undiscoverable there: you had to know the exact key to type it, and nothing
// listed them. The panel now suggests them from a catalogue.
//
// A catalogue is only useful while it is complete, and nothing about adding a
// resolveFlag call reminds anyone to update a TypeScript file. This guard is
// that reminder: it reads the keys straight out of the wire-up and fails when
// one is missing from the catalogue. Same shape as the repo's other static
// architecture guards - no DB, no build, just the source tree.
//
// If this test fails, add the key to
// frontend/apps/web/src/features/admin/operatorFlags.ts. Do NOT delete the
// assertion: an undiscoverable flag is a flag nobody can turn off when it
// misbehaves, which is the whole reason these are panel-settable.
const operatorFlagCatalogue = "../../frontend/apps/web/src/features/admin/operatorFlags.ts"

// resolveFlagCall captures the two string literals of resolveFlag(ctx, repo,
// "db.key", "ENV_VAR").
var resolveFlagCall = regexp.MustCompile(`resolveFlag\([^,]+,\s*[^,]+,\s*"([^"]+)",\s*"([^"]+)"\)`)

func TestOperatorFlagsAreDiscoverableInThePanel(t *testing.T) {
	catalogue, err := os.ReadFile(operatorFlagCatalogue)
	if err != nil {
		t.Fatalf("read the panel catalogue: %v", err)
	}
	cat := string(catalogue)

	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob cmd/evey: %v", err)
	}

	seen := map[string]string{} // db key -> env var
	for _, f := range entries {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range resolveFlagCall.FindAllStringSubmatch(string(src), -1) {
			seen[m[1]] = m[2]
		}
	}

	if len(seen) == 0 {
		t.Fatal("found no resolveFlag call in cmd/evey - the regex no longer matches the wire-up, so this guard is silently passing")
	}

	for key, env := range seen {
		if !strings.Contains(cat, `"`+key+`"`) {
			t.Errorf("operator flag %q (env %s) is read at boot but missing from the panel catalogue %s - it would be unsettable unless you already knew the key",
				key, env, operatorFlagCatalogue)
			continue
		}
		// The env var is shown next to the key in the panel, so a rename that
		// only lands on one side would display a variable that does nothing.
		if !strings.Contains(cat, `"`+env+`"`) {
			t.Errorf("operator flag %q lists env var %s in the wire-up, which is absent from %s - the panel would show a stale variable name",
				key, env, operatorFlagCatalogue)
		}
	}
}
