package i18n

import (
	"sort"
	"testing"
)

func TestSupportedLocales_ContainsCoreSet(t *testing.T) {
	t.Parallel()
	locales := SupportedLocales()
	sort.Strings(locales)
	want := []string{"de", "en", "es", "fr", "hi", "zh"}
	if len(locales) != len(want) {
		t.Fatalf("len: want %d, got %d (%v)", len(want), len(locales), locales)
	}
	for i, l := range want {
		if locales[i] != l {
			t.Fatalf("locale[%d]: want %q, got %q", i, l, locales[i])
		}
	}
}

func TestNewEmpty_ReturnsKeysAsIs(t *testing.T) {
	t.Parallel()
	tr := NewEmpty()
	got := tr.T("en", "nonexistent.key")
	if got != "nonexistent.key" {
		t.Fatalf("want key as-is, got %q", got)
	}
}
