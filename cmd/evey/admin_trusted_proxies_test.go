package main

import (
	"testing"

	"github.com/evey-project/evey/internal/config"
)

func TestDoctorJudgesTheTrustedProxies(t *testing.T) {
	env, err := config.NormalizeTrustedProxies([]string{"172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		panel  []string
		env    []string
		status string
	}{
		{"private proxy network", nil, env, "ok"},
		{"panel list wins", []string{"10.0.0.2"}, env, "ok"},
		{"public range", []string{"173.245.48.0/20"}, env, "WARN"},
		{"catch-all from the environment", nil, append(env, "0.0.0.0/0"), "FAIL"},
	}
	for _, c := range cases {
		got := checkTrustedProxies(config.RemoteAccessConfig{TrustedProxies: c.panel}, c.env)
		if got.status != c.status {
			t.Errorf("%s: status %s, want %s (%s)", c.name, got.status, c.status, got.detail)
		}
	}
}
