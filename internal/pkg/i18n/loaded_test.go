package i18n_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/pkg/i18n"
)

// Why this exists: when the catalogue fails to load, NewEmpty() is installed
// and lookup() returns the KEY for every message (i18n.go:154). The whole UI
// then shows "errors.auth.invalid_pin" instead of a sentence - the beta.38
// raw-key regression, catalogue-wide, behind a single boot WARN that says only
// "i18n load failed, using defaults" and never names that consequence.
//
// The coupling that makes it reachable is invisible: the server loads the
// RELATIVE path "i18n", the published image copies the catalogue to /i18n, and
// that only resolves because the runtime stage leaves WorkingDir empty. Every
// other asset in that image lives under /app - so adding a WORKDIR /app to the
// runtime stage, the most natural edit in the world, silently breaks every
// message in the product.
//
// Loaded() is the cheap guard: a caller can assert the catalogue is real
// instead of discovering it from a screenshot.
func TestTranslator_LoadedDistinguishesARealCatalogueFromAnEmptyOne(t *testing.T) {
	empty := i18n.NewEmpty()
	if empty.Loaded() {
		t.Fatal("an empty translator must not claim to be loaded: it answers every key with the key")
	}

	real, err := i18n.NewFromDir("../../../i18n")
	if err != nil {
		t.Fatalf("load the repo catalogue: %v", err)
	}
	if !real.Loaded() {
		t.Fatal("the repo catalogue is real and must report loaded")
	}

	var nilT *i18n.Translator
	if nilT.Loaded() {
		t.Fatal("a nil translator is not loaded")
	}
}
