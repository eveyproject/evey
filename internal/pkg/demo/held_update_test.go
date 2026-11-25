package demo

import "testing"

// The demo takes a held release only for its own plugins, from the repository
// it names, installed under the project key.
func TestTakesHeldUpdate(t *testing.T) {
	const repo = "https://github.com/eveyproject/evey-plugin-weather"
	cases := []struct {
		name, plugin, source, key string
		want                      bool
	}{
		{"the demo's weather, project key", "weather", repo, ProjectKeyFingerprint, true},
		{"another key", "weather", repo, "sha256:00", false},
		{"unsigned", "weather", repo, "", false},
		{"another repository", "weather", "https://github.com/someone/weather", ProjectKeyFingerprint, false},
		{"a plugin the demo does not list", "librarian", "https://github.com/eveyproject/evey-plugin-librarian", ProjectKeyFingerprint, false},
	}
	for _, tc := range cases {
		if got := TakesHeldUpdate(tc.plugin, tc.source, tc.key); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
