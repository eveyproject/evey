package region

import "testing"

func strPtr(s string) *string { return &s }

func TestResolve_ProfileWins(t *testing.T) {
	got := Resolve(strPtr("be"), "FR", "en")
	if got != "BE" {
		t.Fatalf("want BE, got %s", got)
	}
}

func TestResolve_ServerWhenNoProfile(t *testing.T) {
	got := Resolve(nil, "fr", "en")
	if got != "FR" {
		t.Fatalf("want FR, got %s", got)
	}
}

func TestResolve_LocaleRegionWhenNoProfileNoServer(t *testing.T) {
	if got := Resolve(nil, "", "fr-BE"); got != "BE" {
		t.Fatalf("fr-BE: want BE, got %s", got)
	}
}

func TestResolve_LocaleDefaultWhenBareLang(t *testing.T) {
	if got := Resolve(nil, "", "fr"); got != "FR" {
		t.Fatalf("fr: want FR, got %s", got)
	}
	if got := Resolve(nil, "", "zh"); got != "CN" {
		t.Fatalf("zh: want CN, got %s", got)
	}
}

func TestResolve_GBFallback(t *testing.T) {
	if got := Resolve(nil, "", ""); got != "GB" {
		t.Fatalf("want GB, got %s", got)
	}
	if got := Resolve(nil, "", "xx"); got != "GB" {
		t.Fatalf("unknown locale: want GB, got %s", got)
	}
	if got := Resolve(nil, "", "en"); got != "GB" {
		t.Fatalf("en: want GB, got %s", got)
	}
}

func TestResolve_EmptyProfileSkipsToServer(t *testing.T) {
	empty := ""
	if got := Resolve(&empty, "GB", "en"); got != "GB" {
		t.Fatalf("empty profile should fall through to server GB, got %s", got)
	}
}

func TestResolve_UnderscoreLocale(t *testing.T) {
	if got := Resolve(nil, "", "fr_CA"); got != "CA" {
		t.Fatalf("fr_CA: want CA, got %s", got)
	}
}
