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
// Every case fails hard (t.Fatalf) on a miss - a security-detection regression
// must never be able to degrade into a skipped test.
func TestAnalyzerDetectsMaliciousFixtures(t *testing.T) {
	cases := []struct {
		dir         string
		expectedCat string
	}{
		// php-string-concat-rce: "sys" . "tem" reassembles system() via PHP dot
		// concat. Detected by alias pre-pass: $f = "system" → $f($cmd) → system($cmd).
		{
			dir:         "php-string-concat-rce",
			expectedCat: "shell_exec",
		},
		// python-fstring-eval: outer eval( is plaintext - detected today.
		{
			dir:         "python-fstring-eval",
			expectedCat: "obfuscation",
		},
		// js-constructor-escape: Function.constructor used to escape sandbox.
		{
			dir:         "js-constructor-escape",
			expectedCat: "sandbox_escape",
		},
		// python-os-alias: os aliased to _o then m = _o; m.system() called.
		// Detected by alias pre-pass: m.system() → os.system() after chain resolution.
		{
			dir:         "python-os-alias",
			expectedCat: "shell_exec",
		},
		// python-getattr-builtin: getattr(__builtins__, ...) - detected today.
		{
			dir:         "python-getattr-builtin",
			expectedCat: "dynamic_import",
		},
		// go-raw-syscall: syscall.Syscall(59,...) = raw execve, no stdlib wrapper.
		{
			dir:         "go-raw-syscall",
			expectedCat: "native_code",
		},
		// php-combo-realistic: multi-vector - same dot-concat fold as #1 needed.
		{
			dir:         "php-combo-realistic",
			expectedCat: "", // any critical finding is sufficient
		},
		// === adversarial variants (class-level coverage, not token-level) ===
		// php-string-concat-rce-v2: intermediate vars before concat. Detected by
		// fixed-point alias resolution: $x and $y resolve to literals, $f resolves
		// to "system", $f($cmd) → system($cmd).
		{
			dir:         "php-string-concat-rce-v2",
			expectedCat: "shell_exec",
		},
		// python-builtins-dict-access: __builtins__.__dict__[name] indexing.
		// Detected by literal dict-bracket pattern (after f-string fold inlines
		// the var, or directly if literal).
		{
			dir:         "python-builtins-dict-access",
			expectedCat: "dynamic_import",
		},
		// js-constructor-stored: const F = (fn).constructor; F(code). Detected
		// by stored-constructor tracking: F( is rewritten to .constructor(.
		{
			dir:         "js-constructor-stored",
			expectedCat: "sandbox_escape",
		},
		// python-exec-base64: the payload is a base64 blob decoded and handed to
		// exec() at runtime. Nothing dangerous is spelled out anywhere in the
		// source - the point of the fixture is that "we scan for system()/eval"
		// walks straight past an encoded body.
		// It IS caught, and better than expected: the analyzer decodes the blob
		// and finds the shell call inside it. Pinned so that decoding never
		// quietly becomes a warning-only path.
		{
			dir:         "python-exec-base64",
			expectedCat: "obfuscation_base64_decoded",
		},
		// js-datafile-function: the code lives in a .dat file the analyzer has
		// no reason to treat as source, and the loader turns it into a function
		// at runtime. Splitting payload from loader is the oldest trick there
		// is, and neither half looks like anything on its own.
		// Also caught: the .dat file is scanned like any other unknown file, so
		// the payload is read even though its extension says "data".
		{
			dir:         "js-datafile-function",
			expectedCat: "shell_exec",
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

			// A missed detection is a security regression, never a reason to
			// skip: t.Skipf here would let a sandbox-escape/RCE fixture go
			// undetected without failing CI. Always fail hard.
			if !hasCritical {
				t.Fatalf("no Critical finding for %q - analyzer regression", c.dir)
			}

			if c.expectedCat != "" && !categoryMatch {
				categories := make([]string, 0, len(report.Findings))
				for _, f := range report.Findings {
					if f.Severity == pluginrunner.SeverityCritical {
						categories = append(categories, f.Category)
					}
				}
				t.Fatalf("Critical finding present but wrong category for %q: got %v, want %q", c.dir, categories, c.expectedCat)
			}
		})
	}
}
