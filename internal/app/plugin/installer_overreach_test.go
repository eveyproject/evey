package plugin_test

// Plugins that are not malicious, only reckless: a whole app packaged as a
// plugin (spins up Docker), one that reaches for its own remote database, one
// that installs its dependencies on the host at startup. None of it can work in
// the sandbox, so the admin must be WARNED at preview - a hard block would be
// wrong (the author meant well), and silence would leave them debugging a
// confusing runtime failure.
//
// The operator asked for these on 2026-09-02: reckless plugins that shell out
// to Docker, stand up their own database, or dial a remote one.

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
)

func overreachRoot() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(root, "tests", "fixtures", "plugins", "overreach")
}

func TestAnalyzerWarnsOnOverreachFixtures(t *testing.T) {
	cases := []struct{ dir string }{
		{"docker-orchestrator"},
		{"remote-db-client"},
		{"runtime-pkg-install"},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			report, err := pluginrunner.AnalyzePlugin(filepath.Join(overreachRoot(), c.dir))
			if err != nil {
				t.Fatalf("analyze: %v", err)
			}
			var overreach bool
			for _, f := range report.Findings {
				if f.Category == "overreach" {
					overreach = true
				}
			}
			if !overreach {
				t.Fatalf("%s: expected an 'overreach' warning so the admin sees it at preview", c.dir)
			}
		})
	}
}
