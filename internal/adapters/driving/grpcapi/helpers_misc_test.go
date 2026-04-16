package grpcapi

import "testing"

func TestContainsStr_FoundAndMissing(t *testing.T) {
	t.Parallel()
	if !containsStr([]string{"a", "b", "c"}, "b") {
		t.Fatal("want true for present element")
	}
	if containsStr([]string{"a", "b", "c"}, "z") {
		t.Fatal("want false for missing element")
	}
	if containsStr(nil, "x") {
		t.Fatal("nil slice contains nothing")
	}
	if containsStr([]string{}, "x") {
		t.Fatal("empty slice contains nothing")
	}
}

func TestCellularCapFromPreferences(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		prefs map[string]string
		want  int
	}{
		{"missing", map[string]string{}, 0},
		{"nil map", nil, 0},
		{"empty", map[string]string{"max_bitrate_cellular_kbps": ""}, 0},
		{"invalid", map[string]string{"max_bitrate_cellular_kbps": "abc"}, 0},
		{"zero", map[string]string{"max_bitrate_cellular_kbps": "0"}, 0},
		{"negative", map[string]string{"max_bitrate_cellular_kbps": "-100"}, 0},
		{"valid", map[string]string{"max_bitrate_cellular_kbps": "5000"}, 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cellularCapFromPreferences(c.prefs); got != c.want {
				t.Fatalf("want %d, got %d", c.want, got)
			}
		})
	}
}
