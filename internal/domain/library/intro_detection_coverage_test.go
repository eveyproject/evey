package library

import "testing"

// One answer to "does this episode still need its intro detected?", shared by
// the in-process sweep and the SQL work list: operator marks settle whatever
// the span, file-derived sources settle once they cover enough, community
// databases and heuristics never do.
func TestIntroDetectionSettled(t *testing.T) {
	cases := []struct {
		source string
		spanMs int64
		want   bool
	}{
		{"manual", 0, true},
		{"manual_propagated", 1, true},
		{"mkv_chapter", MinCoveringIntroMs, true},
		{"audio_cluster", MinCoveringIntroMs + 1, true},
		{"phash", MinCoveringIntroMs - 1, false},
		{"tidb", 90_000, false},
		{"introdb", 90_000, false},
		{"audio", 90_000, false},
		{"blackdetect", 90_000, false},
		{"", 90_000, false},
	}
	for _, tc := range cases {
		if got := IntroDetectionSettled(tc.source, tc.spanMs); got != tc.want {
			t.Errorf("%q/%dms: got %v want %v", tc.source, tc.spanMs, got, tc.want)
		}
	}
	for _, s := range []string{"tidb", "introdb"} {
		if !IsCommunityIntroSource(s) {
			t.Errorf("%s is a community source", s)
		}
	}
	for _, s := range []string{"phash", "manual", "audio", ""} {
		if IsCommunityIntroSource(s) {
			t.Errorf("%s is this server's own analysis", s)
		}
	}
	// The lists the SQL is handed must agree with the predicate.
	for _, s := range IntroLockedSources() {
		if !IntroDetectionSettled(s, 0) {
			t.Errorf("locked source %s must settle at any span", s)
		}
	}
	for _, s := range IntroCoveringSources() {
		if IsCommunityIntroSource(s) || IntroDetectionSettled(s, MinCoveringIntroMs-1) {
			t.Errorf("covering source %s: file-derived, and needs the span", s)
		}
	}
}
