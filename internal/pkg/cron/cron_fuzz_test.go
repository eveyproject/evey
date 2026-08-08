package cron

import (
	"testing"
	"time"
)

// Cron expressions reach this parser from the admin panel and from plugin
// manifests, so the string is not the server's own. NextTime walks five fields
// with hand-rolled range/step/list splitting, and it is called from the
// scheduler: an unhandled panic there is a scheduler that stops firing, which
// looks like "backups quietly stopped running" rather than like a crash.
//
// The only claim under test is that a malformed expression comes back as an
// error rather than a panic, and that a well-formed one never yields a time in
// the past - a stop condition getting that wrong is how a scheduler ends up in
// a tight loop.
func FuzzNextTime(f *testing.F) {
	f.Add("0 3 * * *")
	f.Add("*/15 * * * *")
	f.Add("0 0 1 1 *")
	f.Add("5,10,15 * * * *")
	f.Add("0 0-6 * * 1-5")
	// The awkward ones: too few fields, empty fields, inverted and out-of-range
	// bounds, a zero step, and separators with nothing around them.
	f.Add("")
	f.Add("* * * *")
	f.Add("* * * * * *")
	f.Add("*/0 * * * *")
	f.Add("60 * * * *")
	f.Add("10-5 * * * *")
	f.Add("- - - - -")
	f.Add(",,,, ,,,, ,,,, ,,,, ,,,,")

	from := time.Date(2026, 8, 8, 5, 0, 0, 0, time.UTC)

	f.Fuzz(func(t *testing.T, expr string) {
		next, err := NextTime(expr, from)
		if err != nil {
			return
		}
		// An accepted expression must move forward. A "next" time at or before
		// the reference point would have the scheduler fire the same job over
		// and over instead of waiting.
		if !next.After(from) {
			t.Fatalf("NextTime(%q) = %v, which is not after %v", expr, next, from)
		}
	})
}
