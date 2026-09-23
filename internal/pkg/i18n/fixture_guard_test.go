package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The web unit tests fetch the English catalogue from the dev server and fall
// back to a committed JSON when no server answers - which is every CI run.
// That file drifted to 1166 missing and 236 stale keys before 2026-09-01: CI
// was testing different strings than a developer's machine. This guard keeps
// the two identical; `python3 scripts/i18n-fixture.py` regenerates the file.
func TestWebFixtureMatchesEnglishYAML(t *testing.T) {
	tr, err := NewFromDir(repoI18nDir(t))
	if err != nil {
		t.Fatalf("load catalogues: %v", err)
	}
	want := tr.AllMessages(FallbackLocale)

	fixturePath := filepath.Join(repoI18nDir(t), "..", "frontend", "apps", "web", "src", "test", "en-translations.json")
	raw, err := os.ReadFile(fixturePath) //nolint:gosec // repo-relative test asset
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("fixture is not a flat string map: %v", err)
	}

	var missing, stale, extra []string
	for k, v := range want {
		gv, ok := got[k]
		switch {
		case !ok:
			missing = append(missing, k)
		case gv != v:
			stale = append(stale, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	sort.Strings(extra)
	if len(missing)+len(stale)+len(extra) > 0 {
		t.Fatalf("frontend/apps/web/src/test/en-translations.json no longer matches i18n/en (%d missing, %d stale, %d extra; first: %v %v %v) - run: python3 scripts/i18n-fixture.py",
			len(missing), len(stale), len(extra), head(missing, 3), head(stale, 3), head(extra, 3))
	}
}

// The browser TV embeds, for every language, the strings of the screens it
// shows before a server is known (a packaged copy has no server to ask, and
// stayed in English there). Same source, same regeneration command. Keep
// bootPrefixes in step with BOOT_PREFIXES in scripts/i18n-fixture.py.
var bootPrefixes = []string{"mobile.serverConnect.", "mobile.servers.title", "common.clientChoice."}

func TestTVBootCatalogueMatchesYAML(t *testing.T) {
	tr, err := NewFromDir(repoI18nDir(t))
	if err != nil {
		t.Fatalf("load catalogues: %v", err)
	}
	path := filepath.Join(repoI18nDir(t), "..", "frontend", "apps", "tv-web", "src", "i18n", "boot.json")
	raw, err := os.ReadFile(path) //nolint:gosec // repo-relative test asset
	if err != nil {
		t.Fatalf("read boot catalogue: %v", err)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("boot catalogue is not {locale: {key: string}}: %v", err)
	}
	var drift []string
	for _, locale := range SupportedLocales() {
		want := map[string]string{}
		for k, v := range tr.AllMessages(locale) {
			for _, p := range bootPrefixes {
				if strings.HasPrefix(k, p) {
					want[k] = v
				}
			}
		}
		if len(want) == 0 {
			t.Fatalf("%s: no key under %v - have the prefixes moved?", locale, bootPrefixes)
		}
		have := got[locale]
		for k, v := range want {
			if have[k] != v {
				drift = append(drift, locale+" "+k)
			}
		}
		for k := range have {
			if _, ok := want[k]; !ok {
				drift = append(drift, locale+" "+k+" (extra)")
			}
		}
	}
	sort.Strings(drift)
	if len(drift) > 0 {
		t.Fatalf("frontend/apps/tv-web/src/i18n/boot.json no longer matches i18n/ (%d entries; first: %v) - run: python3 scripts/i18n-fixture.py", len(drift), head(drift, 3))
	}
}
