package fakes

import (
	"errors"
	"slices"
	"sort"
)

// RuntimeChecker is an in-memory fake for plugin.RuntimeAvailabilityChecker.
// Build with the set of languages the test wants the host to advertise: an
// entrypoint passes when its language is one of them.
//
//	rc := fakes.NewRuntimeChecker("python", "go")
//	rc.CheckEntrypoint(zip, "python", "python3 main.py") // nil
//	rc.CheckEntrypoint(zip, "nodejs", "node main.js")    // error
type RuntimeChecker struct {
	langs []string
}

// NewRuntimeChecker returns a fake that reports the given languages as
// available. Pass no arguments for a checker that reports nothing - useful
// for asserting installer fails fast on missing runtime.
func NewRuntimeChecker(langs ...string) *RuntimeChecker {
	cp := append([]string(nil), langs...)
	sort.Strings(cp)
	return &RuntimeChecker{langs: cp}
}

func (r *RuntimeChecker) CheckEntrypoint(_, language, _ string) error {
	if slices.Contains(r.langs, language) {
		return nil
	}
	return errors.New("runtime not installed in this image")
}

func (r *RuntimeChecker) Available() []string {
	out := make([]string, len(r.langs))
	copy(out, r.langs)
	return out
}
