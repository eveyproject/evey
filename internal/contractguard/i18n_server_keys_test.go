package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/i18n"
)

// Every "errors.*" literal the SERVER emits must exist in the catalogue.
//
// The server has two ways of naming a failure and both end at the viewer:
//   - gRPC: TranslatedError(ctx, code, "errors.x") - the server translates,
//     and an unknown key comes back as the KEY (i18n.go lookup returns the key
//     it could not find), so the client shows "errors.x".
//   - raw HTTP: http.Error(w, `{"message":"errors.x"}`) - the client resolves
//     it, and an unknown key falls through to a generic fallback, hiding the
//     real cause.
//
// Either way a typo or a renamed catalogue entry degrades silently: no compile
// error, no failing test, no log. It shipped once - beta.38 rendered raw keys
// on a tester's phone and was found from a screenshot. This guard is the
// ratchet: it costs nothing while the tree is clean and refuses the drift.
//
// Deliberately broad: ANY "errors."-prefixed string literal under internal/,
// not just the two call shapes, because the point is that the namespace means
// "a catalogue entry" everywhere it appears.
var serverErrorKeyRe = regexp.MustCompile(`"(errors\.[a-z0-9_.]+)"`)

func TestServerEmittedI18nKeysExistInCatalogue(t *testing.T) {
	tr, err := i18n.NewFromDir(filepath.Join(repoRoot, "i18n"))
	if err != nil {
		t.Fatalf("load i18n catalogue: %v", err)
	}

	type site struct{ file, key string }
	var missing []site
	var scanned int
	seen := map[string]bool{}

	root := filepath.Join(repoRoot, "internal")
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		// Test files may name a deliberately-unknown key to prove a fallback.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range serverErrorKeyRe.FindAllStringSubmatch(string(b), -1) {
			key := m[1]
			scanned++
			// The catalogue answers an unknown key with the key itself.
			if tr.T(i18n.FallbackLocale, key) == key && !seen[key] {
				seen[key] = true
				missing = append(missing, site{file: path, key: key})
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/: %v", walkErr)
	}
	// A guard that matches nothing passes forever. Renaming the namespace,
	// moving the walk root or narrowing the regex must break the check loudly
	// rather than turn it into a green no-op.
	if scanned == 0 {
		t.Fatal("no \"errors.*\" literal found under internal/ - the guard would pass vacuously")
	}

	if len(missing) > 0 {
		sort.Slice(missing, func(i, j int) bool { return missing[i].key < missing[j].key })
		var b strings.Builder
		for _, m := range missing {
			b.WriteString("\n  " + m.key + "  (" + m.file + ")")
		}
		t.Fatalf("%d i18n key(s) emitted by the server but absent from the catalogue - each one renders RAW at the viewer:%s", len(missing), b.String())
	}
}
