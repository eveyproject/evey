package contractguard

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/i18n"
)

// A translated string interpolates named placeholders: "{{count}} entries".
// If one locale drops one, i18next has nothing to substitute and renders a
// HOLE - the sentence comes out with a missing number, in that language only,
// with no error, no warning and no failing test. The existing i18n guard
// checks that a key EXISTS in every locale; nothing checked that the locales
// agree on what the key interpolates.
//
// This is the same failure shape as the pagination bugs of 2026-08-08: silent,
// invisible to any assertion phrased as "the screen renders", and only ever
// noticed by someone reading that language.
//
// The catalogue was measured clean when this guard was written (6 locales, 10
// files, 0 mismatches), so it is a ratchet: it costs nothing today and refuses
// the drift tomorrow.
var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

func placeholdersOf(s string) []string {
	m := placeholderRe.FindAllStringSubmatch(s, -1)
	set := map[string]bool{}
	for _, g := range m {
		set[g[1]] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestI18nPlaceholdersAgreeAcrossLocales(t *testing.T) {
	// Reuse the server's own loader rather than re-parsing the YAML: it is the
	// exact flattening the runtime uses, so the guard can never disagree with
	// what actually ships.
	tr, err := i18n.NewFromDir(filepath.Join(repoRoot, "i18n"))
	if err != nil {
		t.Fatalf("load i18n catalogue: %v", err)
	}

	locales := i18n.SupportedLocales()
	if len(locales) < 2 {
		t.Fatalf("need at least two locales to compare, got %v", locales)
	}

	// key -> locale -> placeholder set, keyed by the sorted joined set so two
	// locales agreeing collapse to one entry.
	perKey := map[string]map[string]string{}
	for _, loc := range locales {
		for key, msg := range tr.AllMessages(loc) {
			if perKey[key] == nil {
				perKey[key] = map[string]string{}
			}
			perKey[key][loc] = strings.Join(placeholdersOf(msg), ",")
		}
	}

	keys := make([]string, 0, len(perKey))
	for k := range perKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		byLocale := perKey[key]
		distinct := map[string][]string{}
		for loc, set := range byLocale {
			distinct[set] = append(distinct[set], loc)
		}
		if len(distinct) <= 1 {
			continue
		}
		var parts []string
		for set, locs := range distinct {
			sort.Strings(locs)
			shown := set
			if shown == "" {
				shown = "(none)"
			}
			parts = append(parts, strings.Join(locs, "/")+" -> "+shown)
		}
		sort.Strings(parts)
		t.Errorf("%s: locales disagree on which placeholders the string interpolates (%s). "+
			"A locale missing one renders an empty hole in that language only, silently.",
			key, strings.Join(parts, " | "))
	}
}
