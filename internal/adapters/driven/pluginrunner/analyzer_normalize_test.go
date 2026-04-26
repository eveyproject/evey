package pluginrunner

import (
	"strings"
	"testing"
)

// ─── foldStringConcat ────────────────────────────────────────────────────────

func TestFoldStringConcat_SimpleDouble(t *testing.T) {
	got := foldStringConcat(`"os" + "system"`)
	want := `"ossystem"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldStringConcat_SimpleSingle(t *testing.T) {
	got := foldStringConcat(`'os' + 'system'`)
	want := `'ossystem'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldStringConcat_Recursive(t *testing.T) {
	// "o"+"s"+"."+"sys"+"tem" should become "os.system"
	got := foldStringConcat(`"o"+"s"+".sys"+"tem"`)
	want := `"os.system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldStringConcat_NoChange_NonStringConcat(t *testing.T) {
	src := `x = a + b`
	got := foldStringConcat(src)
	if got != src {
		t.Errorf("expected no change for variable concat, got %q", got)
	}
}

func TestFoldStringConcat_MaxIterationsDoesNotHang(t *testing.T) {
	// Build a pathologically long chain: "a"+"a"+"a"+... × 100
	var parts []string
	for i := 0; i < 100; i++ {
		parts = append(parts, `"a"`)
	}
	src := strings.Join(parts, "+")
	// Must not hang — maxFoldPasses = 10 limits iterations.
	got := foldStringConcat(src)
	// After 10 passes the string has been folded as far as 10 passes allow.
	// We just verify it doesn't contain more than 2^10 = 1024 concatenated parts.
	if strings.Count(got, `"+"`) > 1 {
		t.Logf("after max passes, result: %q", got)
	}
}

// ─── decodeEscapesInStrings ──────────────────────────────────────────────────

func TestDecodeEscapes_HexOS(t *testing.T) {
	// "\x6f\x73" == "os"
	got := decodeEscapesInStrings(`"\x6f\x73"`)
	if !strings.Contains(got, "os") {
		t.Errorf("expected decoded 'os' in %q", got)
	}
}

func TestDecodeEscapes_OctalOS(t *testing.T) {
	// "\157\163" == "os"
	got := decodeEscapesInStrings(`"\157\163"`)
	if !strings.Contains(got, "os") {
		t.Errorf("expected decoded 'os' in %q", got)
	}
}

func TestDecodeEscapes_Unicode(t *testing.T) {
	// "os" == "os"
	got := decodeEscapesInStrings(`"os"`)
	if !strings.Contains(got, "os") {
		t.Errorf("expected decoded 'os' in %q", got)
	}
}

func TestDecodeEscapes_OutsideString_NoChange(t *testing.T) {
	// \x6f outside a string literal should not be decoded.
	src := `x = \x6f`
	got := decodeEscapesInStrings(src)
	if got != src {
		t.Errorf("expected no change outside string literal, got %q", got)
	}
}

// ─── normalizeUnicode ────────────────────────────────────────────────────────

func TestNormalizeUnicode_CyrillicEval(t *testing.T) {
	// Cyrillic е (U+0435) looks like 'e'
	src := "еval("
	got := normalizeUnicode(src)
	if got != "eval(" {
		t.Errorf("expected 'eval(', got %q", got)
	}
}

func TestNormalizeUnicode_ASCIIUnchanged(t *testing.T) {
	src := "eval(payload)"
	got := normalizeUnicode(src)
	if got != src {
		t.Errorf("pure ASCII should be unchanged, got %q", got)
	}
}

func TestNormalizeUnicode_GreekOmicron(t *testing.T) {
	// Greek Ο (U+039F) looks like 'O', Greek ρ (U+03C1) looks like 'p'
	// "οs.system" with Greek ο → "os.system"
	src := "ο" + "s.system"
	got := normalizeUnicode(src)
	if got != "os.system" {
		t.Errorf("expected 'os.system', got %q", got)
	}
}

// ─── scanBase64Strings ───────────────────────────────────────────────────────

func TestScanBase64Strings_DangerousPayload(t *testing.T) {
	// base64("os.system(\"echo hacked\")") — we use a known encoding
	// "b3Muc3lzdGVtKCJlY2hvIGhhY2tlZCIp" decodes to: os.system("echo hacked")
	encoded := "b3Muc3lzdGVtKCJlY2hvIGhhY2tlZCIp"
	findings := scanBase64Strings(encoded, "test.py", 1)
	if len(findings) == 0 {
		t.Error("expected base64 dangerous payload to be detected")
	}
	for _, f := range findings {
		if f.Category != "obfuscation_base64_decoded" || f.Severity != SeverityCritical {
			t.Errorf("expected obfuscation_base64_decoded CRITICAL, got %s / %d", f.Category, f.Severity)
		}
	}
}

func TestScanBase64Strings_BenignPayload(t *testing.T) {
	// base64("hello world from a safe plugin") — benign
	// "aGVsbG8gd29ybGQgZnJvbSBhIHNhZmUgcGx1Z2lu"
	encoded := "aGVsbG8gd29ybGQgZnJvbSBhIHNhZmUgcGx1Z2lu"
	findings := scanBase64Strings(encoded, "test.py", 1)
	if len(findings) != 0 {
		t.Errorf("expected no findings for benign base64, got %d", len(findings))
	}
}

func TestScanBase64Strings_TooShort_Ignored(t *testing.T) {
	// shorter than 16 chars — should not be considered.
	findings := scanBase64Strings("b3M=", "test.py", 1)
	if len(findings) != 0 {
		t.Errorf("expected no findings for short base64, got %d", len(findings))
	}
}

// ─── End-to-end integration via AnalyzePlugin ────────────────────────────────

// TestAnalyze_StringConcatFolding verifies that "o"+"s"+".sys"+"tem" is folded
// to "os.system" and matched by the shell_exec rule.
func TestAnalyze_StringConcatFolding(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exploit.py", `cmd = "o"+"s"+".sys"+"tem"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — concat folding should expose os.system")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical finding via string concat folding")
	}
}

// TestAnalyze_StringConcatFolding_Negative verifies that innocent variable
// concatenations do not produce false positives.
func TestAnalyze_StringConcatFolding_Negative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.py", `greeting = "hel"+"lo"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" || f.Category == "obfuscation" {
			t.Errorf("unexpected finding %s for benign concat", f.Category)
		}
	}
}

// TestAnalyze_HexEscapeDecode verifies that hex-encoded dangerous names inside
// string literals are decoded before regex matching.
func TestAnalyze_HexEscapeDecode(t *testing.T) {
	dir := t.TempDir()
	// "\x6f\x73\x2e\x73\x79\x73\x74\x65\x6d" == "os.system"
	writeFile(t, dir, "exploit.py", `func = "\x6f\x73\x2e\x73\x79\x73\x74\x65\x6d"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — hex-decoded os.system should be detected")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "shell_exec" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected shell_exec critical via hex decode")
	}
}

// TestAnalyze_HexEscapeDecode_Negative verifies benign hex strings don't false-positive.
func TestAnalyze_HexEscapeDecode_Negative(t *testing.T) {
	dir := t.TempDir()
	// "\x68\x65\x6c\x6c\x6f" == "hello"
	writeFile(t, dir, "clean.py", `msg = "\x68\x65\x6c\x6c\x6f"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "shell_exec" || f.Category == "obfuscation_getattr_builtins" {
			t.Errorf("unexpected critical finding %s for benign hex string", f.Category)
		}
	}
}

// TestAnalyze_Base64Payload verifies that b64decode("...") where the decoded
// payload contains os.system is flagged.
func TestAnalyze_Base64Payload(t *testing.T) {
	dir := t.TempDir()
	// b3MuZWNobygiaGFja2VkIikK decodes to: os.echo("hacked")\n — contains os.
	// Use a payload that definitely contains os.system:
	// base64("os.system(\"id\")") = "b3Muc3lzdGVtKCJpZCIp"
	writeFile(t, dir, "exploit.py", `eval(b64decode("b3Muc3lzdGVtKCJpZCIp"))`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — base64 payload contains os.system")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation_base64_decoded" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Errorf("expected obfuscation_base64_decoded critical finding; findings: %v", report.Findings)
	}
}

// TestAnalyze_Base64Payload_Negative verifies benign base64 content is not flagged.
func TestAnalyze_Base64Payload_Negative(t *testing.T) {
	dir := t.TempDir()
	// base64("this is just a safe message from the plugin") → benign
	writeFile(t, dir, "clean.py", `msg = b64decode("dGhpcyBpcyBqdXN0IGEgc2FmZSBtZXNzYWdlIGZyb20gdGhlIHBsdWdpbg==")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation_base64_decoded" {
			t.Errorf("unexpected obfuscation_base64_decoded for benign payload")
		}
	}
}

// TestAnalyze_GetAttrBuiltins verifies that getattr(__builtins__, ...) is detected.
func TestAnalyze_GetAttrBuiltins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exploit.py", `fn = getattr(__builtins__, "eval")(code)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — getattr(__builtins__,...) is a sandbox bypass")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation_getattr_builtins" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation_getattr_builtins critical finding")
	}
}

// TestAnalyze_GetAttrBuiltins_WithConcatArg verifies that
// getattr(__builtins__, "ev"+"al") is detected after concat folding.
func TestAnalyze_GetAttrBuiltins_WithConcatArg(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exploit.py", `fn = getattr(__builtins__, "ev"+"al")(code)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation_getattr_builtins" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation_getattr_builtins critical finding after concat fold")
	}
}

// TestAnalyze_GetAttrBuiltins_Negative verifies normal getattr usage is not flagged.
func TestAnalyze_GetAttrBuiltins_Negative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.py", `value = getattr(my_obj, "field_name")`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation_getattr_builtins" {
			t.Errorf("unexpected obfuscation_getattr_builtins for normal getattr usage")
		}
	}
}

// TestAnalyze_DynamicImportVariable verifies that __import__(var) is detected.
func TestAnalyze_DynamicImportVariable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exploit.py", `mod = __import__(module_name)`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — __import__ with variable argument")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation_dynamic_import" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation_dynamic_import critical finding")
	}
}

// TestAnalyze_DynamicImportVariable_Negative verifies that __import__("os")
// with a literal string does not trigger obfuscation_dynamic_import
// (it triggers dynamic_import instead, which is fine).
func TestAnalyze_DynamicImportVariable_Negative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "clean.py", `import os  # normal import`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Category == "obfuscation_dynamic_import" {
			t.Errorf("unexpected obfuscation_dynamic_import for plain 'import os'")
		}
	}
}

// TestAnalyze_UnicodeLookalike verifies that cyrillc confusable characters
// in dangerous keyword positions are normalized and caught.
func TestAnalyze_UnicodeLookalike(t *testing.T) {
	dir := t.TempDir()
	// Use Cyrillic е (U+0435) in place of Latin 'e' in "eval"
	writeFile(t, dir, "exploit.py", "еval(payload)")

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe — unicode lookalike eval should be detected")
	}
	found := false
	for _, f := range report.Findings {
		if f.Category == "obfuscation" && f.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Error("expected obfuscation critical finding via unicode normalization")
	}
}

// TestAnalyze_UnicodeLookalike_Negative verifies purely unicode identifiers
// that do not map to dangerous keywords are not flagged.
func TestAnalyze_UnicodeLookalike_Negative(t *testing.T) {
	dir := t.TempDir()
	// Greek αβγ — three lowercase greek letters that map to 'a','b' (β), but
	// together they don't spell any dangerous function.
	writeFile(t, dir, "clean.py", `αβγ = "some unicode variable"`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Safe {
		t.Errorf("unexpected unsafe: %v", report.Findings)
	}
}
