package i18n

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// repoI18nDir resolves ../../../i18n from this file, so the test finds the
// catalogue whatever the working directory is.
func repoI18nDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "i18n")
}

// Key parity across every shipped locale, enforced instead of audited.
//
// Drift here is invisible in development: a missing key falls back to English
// and looks like a translation nobody got round to, so it accumulates silently
// - the last audit found 20 orphans from a single admin card added weeks
// earlier. English is the reference because it is what the fallback serves.
//
// This checks KEYS, never values: "Chromecast", "Plugin" or "CFR/VFR" are
// legitimately identical across locales, and asserting otherwise would fail on
// correct translations.
func TestLocaleCatalogues_HaveIdenticalKeySets(t *testing.T) {
	t.Parallel()
	tr, err := NewFromDir(repoI18nDir(t))
	if err != nil {
		t.Fatalf("load catalogues: %v", err)
	}

	tr.mu.RLock()
	defer tr.mu.RUnlock()

	ref, ok := tr.messages[FallbackLocale]
	if !ok || len(ref) == 0 {
		t.Fatalf("reference locale %q is empty - the catalogue did not load", FallbackLocale)
	}

	for _, locale := range SupportedLocales() {
		if locale == FallbackLocale {
			continue
		}
		got := tr.messages[locale]
		var missing, extra []string
		for k := range ref {
			if _, present := got[k]; !present {
				missing = append(missing, k)
			}
		}
		for k := range got {
			if _, present := ref[k]; !present {
				extra = append(extra, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		if len(missing) > 0 {
			t.Errorf("locale %q is missing %d key(s) present in %q:\n  %s",
				locale, len(missing), FallbackLocale, strings.Join(head(missing, 15), "\n  "))
		}
		if len(extra) > 0 {
			t.Errorf("locale %q has %d key(s) absent from %q (dead or misspelled):\n  %s",
				locale, len(extra), FallbackLocale, strings.Join(head(extra, 15), "\n  "))
		}
	}
}

// A locale that loaded zero keys would trivially satisfy nothing above if the
// reference itself were empty; pin a floor so a broken loader cannot pass the
// parity check by making every catalogue equally empty.
func TestLocaleCatalogues_AreNotEmpty(t *testing.T) {
	t.Parallel()
	tr, err := NewFromDir(repoI18nDir(t))
	if err != nil {
		t.Fatalf("load catalogues: %v", err)
	}
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	for _, locale := range SupportedLocales() {
		if n := len(tr.messages[locale]); n < 500 {
			t.Errorf("locale %q loaded only %d keys - the catalogue is truncated or unreadable", locale, n)
		}
	}
}

func head(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(s[:n:n], "...")
}
