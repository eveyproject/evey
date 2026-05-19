package fakes

import (
	"slices"
	"sort"
)

// RuntimeChecker is an in-memory fake for plugin.RuntimeAvailabilityChecker.
// Build with the set of languages the test wants the host to advertise:
//
//	rc := fakes.NewRuntimeChecker("python", "go")
//	rc.IsAvailable("python") // true
//	rc.IsAvailable("nodejs") // false
type RuntimeChecker struct {
	langs []string
}

// NewRuntimeChecker returns a fake that reports the given languages as
// available. Pass no arguments for a checker that reports nothing — useful
// for asserting installer fails fast on missing runtime.
func NewRuntimeChecker(langs ...string) *RuntimeChecker {
	cp := append([]string(nil), langs...)
	sort.Strings(cp)
	return &RuntimeChecker{langs: cp}
}

func (r *RuntimeChecker) IsAvailable(language string) bool {
	return slices.Contains(r.langs, language)
}

func (r *RuntimeChecker) Available() []string {
	out := make([]string, len(r.langs))
	copy(out, r.langs)
	return out
}
