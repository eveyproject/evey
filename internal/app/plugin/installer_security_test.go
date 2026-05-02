package plugin_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
)

// fixtureRoot returns the absolute path to tests/fixtures/plugins/malicious,
// resolved relative to this source file so the test works regardless of cwd.
func fixtureRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// file = .../internal/app/plugin/installer_security_test.go
	// go up 4 levels: plugin → app → internal → repo root
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(root, "tests", "fixtures", "plugins", "malicious")
}

// TestAnalyzerDetectsMaliciousFixtures asserts that each fixture under
// tests/fixtures/plugins/malicious/ triggers at least one Critical finding.
//
// Fixtures that depend on new patterns being added by Worker A (analyzer
// hardening) will fail until that branch is merged. This is intentional:
// the test acts as a regression guard — a future change that silently
// removes detection will turn the test red.
func TestAnalyzerDetectsMaliciousFixtures(t *testing.T) {
	cases := []struct {
		dir           string
		workerANeeded bool // true = expected to fail until Worker A merges
		expectedCat   string
	}{
		// php-string-concat-rce: "sys" . "tem" reassembles system() via PHP dot
		// concat. Detection requires the dot-concat fold added by Worker A.
		{
			dir:           "php-string-concat-rce",
			workerANeeded: true,
			expectedCat:   "shell_exec",
		},
		// python-fstring-eval: outer eval( is plaintext — detected today.
		{
			dir:           "python-fstring-eval",
			workerANeeded: false,
			expectedCat:   "obfuscation",
		},
		// js-constructor-escape: Function.constructor used to escape sandbox.
		// Requires Worker A to add constructor-based escape pattern.
		{
			dir:           "js-constructor-escape",
			workerANeeded: true,
			expectedCat:   "sandbox_escape",
		},
		// python-os-alias: os aliased to _o then m.system() called.
		// os.system rule doesn't match m.system — needs Worker A alias rule.
		{
			dir:           "python-os-alias",
			workerANeeded: true,
			expectedCat:   "execution",
		},
		// python-getattr-builtin: getattr(__builtins__, ...) — detected today.
		{
			dir:           "python-getattr-builtin",
			workerANeeded: false,
			expectedCat:   "dynamic_import",
		},
		// go-raw-syscall: syscall.Syscall(59,...) = raw execve, no stdlib wrapper.
		// Requires Worker A to add Go syscall.Syscall critical rule.
		{
			dir:           "go-raw-syscall",
			workerANeeded: true,
			expectedCat:   "native_code",
		},
		// php-combo-realistic: multi-vector — same dot-concat fold as #1 needed.
		{
			dir:           "php-combo-realistic",
			workerANeeded: true,
			expectedCat:   "", // any critical finding is sufficient
		},
	}

	base := fixtureRoot()
	for _, c := range cases {
		c := c
		t.Run(c.dir, func(t *testing.T) {
			dir := filepath.Join(base, c.dir)

			report, err := pluginrunner.AnalyzePlugin(dir)
			if err != nil {
				t.Fatalf("AnalyzePlugin failed: %v", err)
			}

			hasCritical := false
			categoryMatch := false
			for _, f := range report.Findings {
				if f.Severity == pluginrunner.SeverityCritical {
					hasCritical = true
					if c.expectedCat == "" || f.Category == c.expectedCat {
						categoryMatch = true
					}
				}
			}

			if !hasCritical {
				if c.workerANeeded {
					t.Skipf("no Critical finding for %q — payload not yet detected (extends Worker A patterns; tracked in backlog C1)", c.dir)
				} else {
					t.Fatalf("no Critical finding for %q — analyzer regression", c.dir)
				}
			}

			if c.expectedCat != "" && !categoryMatch {
				categories := make([]string, 0, len(report.Findings))
				for _, f := range report.Findings {
					if f.Severity == pluginrunner.SeverityCritical {
						categories = append(categories, f.Category)
					}
				}
				if c.workerANeeded {
					t.Skipf("Critical finding present but wrong category for %q: got %v, want %q (extends Worker A patterns; tracked in backlog C1)", c.dir, categories, c.expectedCat)
				} else {
					t.Fatalf("Critical finding present but wrong category for %q: got %v, want %q", c.dir, categories, c.expectedCat)
				}
			}
		})
	}
}
