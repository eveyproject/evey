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
	// Must not hang - maxFoldPasses = 10 limits iterations.
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
	// base64("os.system(\"echo hacked\")") - we use a known encoding
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
	// base64("hello world from a safe plugin") - benign
	// "aGVsbG8gd29ybGQgZnJvbSBhIHNhZmUgcGx1Z2lu"
	encoded := "aGVsbG8gd29ybGQgZnJvbSBhIHNhZmUgcGx1Z2lu"
	findings := scanBase64Strings(encoded, "test.py", 1)
	if len(findings) != 0 {
		t.Errorf("expected no findings for benign base64, got %d", len(findings))
	}
}

func TestScanBase64Strings_TooShort_Ignored(t *testing.T) {
	// shorter than 16 chars - should not be considered.
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
		t.Fatal("expected unsafe - concat folding should expose os.system")
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
		t.Fatal("expected unsafe - hex-decoded os.system should be detected")
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
	// b3MuZWNobygiaGFja2VkIikK decodes to: os.echo("hacked")\n - contains os.
	// Use a payload that definitely contains os.system:
	// base64("os.system(\"id\")") = "b3Muc3lzdGVtKCJpZCIp"
	writeFile(t, dir, "exploit.py", `eval(b64decode("b3Muc3lzdGVtKCJpZCIp"))`)

	report, err := AnalyzePlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Safe {
		t.Fatal("expected unsafe - base64 payload contains os.system")
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
		t.Fatal("expected unsafe - getattr(__builtins__,...) is a sandbox bypass")
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
		t.Fatal("expected unsafe - __import__ with variable argument")
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
		t.Fatal("expected unsafe - unicode lookalike eval should be detected")
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
	// Greek αβγ - three lowercase greek letters that map to 'a','b' (β), but
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

// ─── foldFStrings ─────────────────────────────────────────────────────────────

func TestFoldFStrings_EvalObfuscated(t *testing.T) {
	got := foldFStrings(`f"{'ev'}{'al'}"`)
	want := `"eval"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldFStrings_SingleLiteral(t *testing.T) {
	got := foldFStrings(`f"{'system'}"`)
	want := `"system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldFStrings_NoChange_ComplexInterpolation(t *testing.T) {
	// f-string with non-literal expression - must be left as-is
	src := `f"{x+1}"`
	got := foldFStrings(src)
	if got != src {
		t.Errorf("expected no change for complex interpolation, got %q", got)
	}
}

func TestFoldFStrings_NoChange_PlainString(t *testing.T) {
	src := `"hello"`
	got := foldFStrings(src)
	if got != src {
		t.Errorf("expected no change for plain string, got %q", got)
	}
}

// ─── foldJoin ────────────────────────────────────────────────────────────────

func TestFoldJoin_ExecFromList(t *testing.T) {
	got := foldJoin(`"".join(["e","x","e","c"])`)
	want := `"exec"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldJoin_SingleQuoteJoiner(t *testing.T) {
	got := foldJoin(`''.join(['e','v','a','l'])`)
	want := `"eval"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldJoin_NoChange_WithVariables(t *testing.T) {
	// list contains a variable - must not fold
	src := `"".join([x, "b"])`
	got := foldJoin(src)
	if got != src {
		t.Errorf("expected no change for list with variable, got %q", got)
	}
}

func TestFoldJoin_NoChange_EmptyList(t *testing.T) {
	src := `"".join([])`
	// An empty list is fine to fold to empty string - or leave as-is, both acceptable.
	// We just verify no panic.
	_ = foldJoin(src)
}

// ─── stringConcatRe PHP dot operator ─────────────────────────────────────────

func TestFoldStringConcat_PHPDot(t *testing.T) {
	got := foldStringConcat(`"sys"."tem"`)
	want := `"system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldStringConcat_PHPDot_Recursive(t *testing.T) {
	got := foldStringConcat(`"s"."y"."s"."t"."e"."m"`)
	want := `"system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFoldStringConcat_JSPlus_StillWorks(t *testing.T) {
	got := foldStringConcat(`"sys"+"tem"`)
	want := `"system"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ─── resolveSimpleAliases ─────────────────────────────────────────────────────

func TestResolveSimpleAliases_PHPVarCallable(t *testing.T) {
	// $f = "system"; $f($cmd) should become system($cmd)
	src := `<?php
$f = "system";
$cmd = $_GET['c'];
$f($cmd);
`
	got := resolveSimpleAliases(src)
	if !strings.Contains(got, "system($cmd)") {
		t.Errorf("expected system($cmd) after alias resolution, got:\n%s", got)
	}
}

func TestResolveSimpleAliases_PHPDotConcatVar(t *testing.T) {
	// $f = "sys"."tem" → after foldStringConcat + resolveSimpleAliases → system($cmd)
	// resolveSimpleAliases itself doesn't fold - but scanFile runs foldStringConcat
	// per-line after the pre-pass. The fixture index.php uses this pattern.
	// This test verifies the raw string assignment tracking works.
	src := "$f = \"system\";\n$f($x);\n"
	got := resolveSimpleAliases(src)
	if !strings.Contains(got, "system($x)") {
		t.Errorf("expected system($x) after alias resolution, got:\n%s", got)
	}
}

func TestResolveSimpleAliases_PythonModuleChain(t *testing.T) {
	// import os as _o; m = _o; m.system("id") → os.system("id")
	src := "import os as _o\nm = _o\nm.system(\"id\")\n"
	got := resolveSimpleAliases(src)
	if !strings.Contains(got, "os.system") {
		t.Errorf("expected os.system after alias chain resolution, got:\n%s", got)
	}
}

func TestResolveSimpleAliases_NoSubstitution_BenignValue(t *testing.T) {
	// f = "hello" is not dangerous - must not be substituted
	src := "f = \"hello\"\nf(\"world\")\n"
	got := resolveSimpleAliases(src)
	if strings.Contains(got, "hello(") {
		t.Errorf("unexpected substitution for benign value, got:\n%s", got)
	}
	// The original call site must remain unchanged
	if !strings.Contains(got, "f(\"world\")") {
		t.Errorf("expected f(\"world\") to remain unchanged, got:\n%s", got)
	}
}

func TestResolveSimpleAliases_Reassigned_NoSub(t *testing.T) {
	// f = "system"; f = lambda x: x  → f is re-assigned, must not sub
	src := "f = \"system\"\nf = lambda x: x\nf(\"safe\")\n"
	got := resolveSimpleAliases(src)
	// f("safe") must NOT become system("safe")
	if strings.Contains(got, "system(\"safe\")") {
		t.Errorf("must not substitute after re-assignment, got:\n%s", got)
	}
}

func TestResolveSimpleAliases_NoContent_NoAlloc(t *testing.T) {
	// No aliases → content returned unchanged (fast path)
	src := "print(\"hello\")\n"
	got := resolveSimpleAliases(src)
	if got != src {
		t.Errorf("expected identical content for no-alias file, got:\n%s", got)
	}
}

// ─── stripLineComments ───────────────────────────────────────────────────────

func TestStripLineComments_PythonHashComment(t *testing.T) {
	got := stripLineComments(`# pip install grpcio`)
	if got != "" {
		t.Errorf("hash comment should be fully stripped, got %q", got)
	}
}

func TestStripLineComments_PythonTrailingHash(t *testing.T) {
	got := stripLineComments(`x = 1  # pip install foo`)
	want := `x = 1  `
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripLineComments_HashInsideDoubleQuote_Preserved(t *testing.T) {
	// `#` inside a string literal is NOT a comment.
	got := stripLineComments(`x = "# pip install"`)
	want := `x = "# pip install"`
	if got != want {
		t.Errorf("hash inside string must survive, got %q", got)
	}
}

func TestStripLineComments_HashInsideSingleQuote_Preserved(t *testing.T) {
	got := stripLineComments(`x = '# pip install'`)
	want := `x = '# pip install'`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripLineComments_DoubleSlashJSComment(t *testing.T) {
	got := stripLineComments(`var x = 1; // npm install foo`)
	want := `var x = 1; `
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripLineComments_DoubleSlashInURL_Preserved(t *testing.T) {
	// `//` inside a quoted URL must survive.
	got := stripLineComments(`fetch("https://api.example.com/path")`)
	want := `fetch("https://api.example.com/path")`
	if got != want {
		t.Errorf("URL slashes must survive, got %q", got)
	}
}

func TestStripLineComments_EscapedQuoteInside(t *testing.T) {
	// Backslash-escaped quote must not close the string.
	got := stripLineComments(`x = "say \"hi\" # not a comment"`)
	want := `x = "say \"hi\" # not a comment"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripLineComments_NoCommentMarker_Unchanged(t *testing.T) {
	got := stripLineComments(`os.system("rm -rf /")`)
	want := `os.system("rm -rf /")`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripLineComments_FullLineComment_Empty(t *testing.T) {
	// Full-line comment → return empty (truncate at marker).
	got := stripLineComments(`// some comment about os.system`)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestStripLineComments_BacktickPreserved(t *testing.T) {
	// Template literal in JS - backtick string.
	got := stripLineComments("const cmd = `# not a comment` + x")
	want := "const cmd = `# not a comment` + x"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ─── stripBlockComments ──────────────────────────────────────────────────────

// TestStripBlockComments_BasicStrip verifies that a simple /* … */ block is
// replaced with spaces (preserving length) so downstream line numbers stay
// accurate.
func TestStripBlockComments_BasicStrip(t *testing.T) {
	src := `before /* evil */ after`
	got := stripBlockComments(src)
	if strings.Contains(got, "evil") {
		t.Errorf("block comment content must be stripped, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("surrounding code must be preserved, got %q", got)
	}
}

// TestStripBlockComments_EvalStripped verifies that eval() inside a block
// comment is not visible after stripping.
func TestStripBlockComments_EvalStripped(t *testing.T) {
	src := "/* eval(dangerousPayload) */\nconsole.log('ok')"
	got := stripBlockComments(src)
	if strings.Contains(got, "eval") {
		t.Errorf("eval inside block comment must be stripped, got %q", got)
	}
	if !strings.Contains(got, "console.log") {
		t.Errorf("code after block comment must be preserved, got %q", got)
	}
}

// TestStripBlockComments_InsideStringPreserved verifies that /* inside a string
// literal is not treated as an opening block comment.
func TestStripBlockComments_InsideStringPreserved(t *testing.T) {
	src := `url = "http://example.com/path/* not a comment */"`
	got := stripBlockComments(src)
	if got != src {
		t.Errorf("/* inside string literal must not be stripped; got %q", got)
	}
}

// TestStripBlockComments_SingleQuoteStringPreserved verifies single-quoted
// strings with /* are not treated as comment openers.
func TestStripBlockComments_SingleQuoteStringPreserved(t *testing.T) {
	src := `msg = '/* this is a string */'`
	got := stripBlockComments(src)
	if got != src {
		t.Errorf("/* inside single-quoted string must not be stripped; got %q", got)
	}
}

// TestStripBlockComments_MultilinePreservesNewlines verifies that newlines
// inside a block comment are kept so line numbers remain accurate.
func TestStripBlockComments_MultilinePreservesNewlines(t *testing.T) {
	src := "line1\n/* comment\nspanning\nthree lines */\nline5"
	got := stripBlockComments(src)
	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Errorf("newlines inside block comment must be preserved; got %d lines: %q", len(lines), got)
	}
	if strings.Contains(got, "comment") || strings.Contains(got, "spanning") {
		t.Errorf("comment content must be blanked, got %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line5") {
		t.Errorf("surrounding lines must survive, got %q", got)
	}
}

// TestStripBlockComments_LineNumbersAccurate verifies that a finding on the
// line after a multi-line block comment still reports the correct line number.
// This is a unit-level check of the stripping output - the integration-level
// check lives in analyzer_test.go.
func TestStripBlockComments_LineNumbersAccurate(t *testing.T) {
	// Line 1: comment opens
	// Lines 2-3: inside comment (newlines preserved → still lines 2 and 3)
	// Line 4: code that should be on line 4
	src := "/* block\ncomment\n*/\nactual_code"
	got := stripBlockComments(src)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines after strip, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[3], "actual_code") {
		t.Errorf("actual_code must be on line 4 (index 3), got %q", lines[3])
	}
}

// TestStripBlockComments_OnlyFirstStarSlashCloses verifies that only the first
// */ closes the comment - code on the next line is still active.
func TestStripBlockComments_OnlyFirstStarSlashCloses(t *testing.T) {
	// This looks like nested comments but Go/JS/C don't nest them.
	// The first */ closes the comment opened by /*.
	src := "/* outer /* inner */ code_after"
	got := stripBlockComments(src)
	if !strings.Contains(got, "code_after") {
		t.Errorf("code after first closing */ must be preserved, got %q", got)
	}
	if strings.Contains(got, "outer") || strings.Contains(got, "inner") {
		t.Errorf("comment body must be stripped, got %q", got)
	}
}

// ─── stripTypedArrayConstructor ──────────────────────────────────────────────

func TestStripTypedArrayConstructor_NewArrayConstructorRemoved(t *testing.T) {
	got := stripTypedArrayConstructor(`new this.array.constructor(t)`)
	if got != "" {
		t.Errorf("expected empty after strip, got %q", got)
	}
}

func TestStripTypedArrayConstructor_NewIdentConstructorRemoved(t *testing.T) {
	got := stripTypedArrayConstructor(`r=new t.constructor(i)`)
	want := `r=`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripTypedArrayConstructor_PreservesNonNewConstructorCall(t *testing.T) {
	// Without `new`, `.constructor(arg)` is the dangerous Function-constructor
	// idiom and must survive the strip so the rule can flag it.
	src := `f.constructor(t)`
	got := stripTypedArrayConstructor(src)
	if got != src {
		t.Errorf("non-new constructor call must survive strip, got %q", got)
	}
}

func TestStripTypedArrayConstructor_E2E_NoFalsePositiveOnThreejsBundle(t *testing.T) {
	// Realistic line shape from minified Three.js bundle (weather plugin).
	line := `init(e,t,i){let r=new Et(new this.array.constructor(t),3,!1)}`
	got := normalizeLine(line)
	// The `.constructor(t)` payload must NOT survive normalization.
	if strings.Contains(got, ".constructor(t)") {
		t.Errorf("expected typed-array constructor stripped, got %q", got)
	}
}
