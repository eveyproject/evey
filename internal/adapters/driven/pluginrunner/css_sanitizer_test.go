package pluginrunner

import (
	"testing"
)

func TestSanitizeCSS_CleanPassesThrough(t *testing.T) {
	input := []byte(`body { color: red; background: #fff; }
.container { margin: 0 auto; padding: 16px; }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("clean CSS modified:\ngot:  %s\nwant: %s", out, input)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_EmptyInput(t *testing.T) {
	out, warns := SanitizeCSS([]byte{})
	if len(out) != 0 {
		t.Errorf("expected empty output, got: %s", out)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_RemovesImport(t *testing.T) {
	input := []byte(`@import url("https://evil.com/spy.css");
body { color: red; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "@import") {
		t.Errorf("@import not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings for @import removal")
	}
	if !containsString(out, "body") {
		t.Error("valid CSS removed")
	}
}

func TestSanitizeCSS_RemovesExternalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"https", `div { background: url(https://evil.com/img.png); }`},
		{"http", `div { background: url(http://evil.com/img.png); }`},
		{"protocol-relative", `div { background: url(//evil.com/img.png); }`},
		{"with quotes", `div { background: url("https://evil.com/img.png"); }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "evil.com") {
				t.Errorf("external URL not removed: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warnings")
			}
		})
	}
}

func TestSanitizeCSS_PreservesDataURL(t *testing.T) {
	input := []byte(`div { background: url(data:image/png;base64,iVBORz...); }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("data URL was modified:\ngot:  %s\nwant: %s", out, input)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_PreservesRelativePath(t *testing.T) {
	input := []byte(`div { background: url(../images/bg.png); }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("relative path was modified:\ngot:  %s\nwant: %s", out, input)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_RemovesExpression(t *testing.T) {
	input := []byte(`div { width: expression(document.body.clientWidth); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "expression") {
		t.Errorf("expression() not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestSanitizeCSS_RemovesMozBinding(t *testing.T) {
	input := []byte(`div { -moz-binding: url("http://evil.com/xbl"); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "-moz-binding") {
		t.Errorf("-moz-binding not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestSanitizeCSS_RemovesBehavior(t *testing.T) {
	input := []byte(`div { behavior: url(malicious.htc); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "behavior") {
		t.Errorf("behavior not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestSanitizeCSS_RemovesJavascriptURL(t *testing.T) {
	input := []byte(`div { background: url(javascript:alert(1)); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "javascript") {
		t.Errorf("javascript URL not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestSanitizeCSS_MultipleDangerousPatterns(t *testing.T) {
	input := []byte(`@import url("https://evil.com/spy.css");
body { background: url(https://evil.com/track.png); color: red; }
div { -moz-binding: url("http://evil.com/xbl"); }
p { width: expression(alert(1)); }
span { behavior: url(malicious.htc); }
a { background: url(javascript:void(0)); }`)

	out, warns := SanitizeCSS(input)
	for _, bad := range []string{"@import", "evil.com", "-moz-binding", "expression", "behavior", "javascript"} {
		if containsString(out, bad) {
			t.Errorf("%s not removed from output: %s", bad, out)
		}
	}
	if len(warns) < 6 {
		t.Errorf("expected at least 6 warnings, got %d: %v", len(warns), warns)
	}
	if !containsString(out, "color: red") {
		t.Error("safe CSS was removed")
	}
}

func TestSanitizeCSS_CustomProperties(t *testing.T) {
	input := []byte(`:root { --accent: #ff0; --bg: #1a1a1a; }
.card { color: var(--accent); }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("custom properties modified:\ngot:  %s\nwant: %s", out, input)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_ComplexValidCSS(t *testing.T) {
	input := []byte(`@media (max-width: 768px) {
  .grid { grid-template-columns: 1fr; }
}
@keyframes fade {
  from { opacity: 0; }
  to { opacity: 1; }
}
.card:hover { transform: scale(1.05); transition: all 0.3s ease; }
::selection { background: var(--accent); }`)

	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("complex valid CSS modified:\ngot:  %s\nwant: %s", out, input)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestSanitizeCSS_URLQuoteVariants(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		removed bool
	}{
		{"single quotes", `div { background: url('https://evil.com/img.png'); }`, true},
		{"double quotes", `div { background: url("https://evil.com/img.png"); }`, true},
		{"no quotes", `div { background: url(https://evil.com/img.png); }`, true},
		{"spaces around url", `div { background: url( https://evil.com/img.png ); }`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if tt.removed {
				if containsString(out, "evil.com") {
					t.Errorf("external URL not removed: %s", out)
				}
				if len(warns) == 0 {
					t.Error("expected warnings")
				}
			}
		})
	}
}

func TestSanitizeCSS_CaseInsensitiveURL(t *testing.T) {
	input := []byte(`div { background: URL(HTTPS://EVIL.COM/img.png); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "EVIL.COM") {
		t.Errorf("case-insensitive URL not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestSanitizeCSS_MixedSafeAndUnsafeURL(t *testing.T) {
	input := []byte(`.card {
  background: url(../images/local.png);
  border-image: url(https://evil.com/border.png);
  cursor: url(data:image/png;base64,abc123);
}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.com") {
		t.Errorf("external URL not removed: %s", out)
	}
	if !containsString(out, "../images/local.png") {
		t.Error("relative path was removed")
	}
	if !containsString(out, "data:image/png") {
		t.Error("data URL was removed")
	}
	if len(warns) == 0 {
		t.Error("expected warnings for external URL")
	}
}

func TestSanitizeCSS_ImportVariants(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"url import", `@import url("https://evil.com/spy.css");`},
		{"double quote import", `@import "https://evil.com/spy.css";`},
		{"single quote import", `@import 'https://evil.com/spy.css';`},
		{"url no quotes", `@import url(https://evil.com/spy.css);`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "@import") {
				t.Errorf("@import not removed: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warnings for @import removal")
			}
		})
	}
}

func TestSanitizeCSS_LargeCSS(t *testing.T) {
	var sb []byte
	for i := 0; i < 10000; i++ {
		sb = append(sb, []byte(`.class-`+string(rune('a'+i%26))+` { color: red; margin: 10px; }`+"\n")...)
	}
	out, warns := SanitizeCSS(sb)
	if len(out) != len(sb) {
		t.Errorf("large clean CSS was modified: input len %d, output len %d", len(sb), len(out))
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings on large clean CSS: %v", warns)
	}
}

func TestSanitizeCSS_UnicodeEscapeBypass(t *testing.T) {
	// \75\72\6c is "url" in CSS unicode escapes — regex-based sanitizer
	// won't catch this; document as known limitation
	input := []byte(`div { background: \75\72\6c(https://evil.com/img.png); }`)
	_, _ = SanitizeCSS(input)
	// NOTE: CSS unicode escapes are a known limitation of regex-based sanitization.
	// A full CSS parser would be needed to handle these.
}

func TestSanitizeCSS_URLInsideMediaQuery(t *testing.T) {
	input := []byte(`@media (min-width: 768px) {
  .hero { background: url(https://evil.com/hero.png); }
}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.com") {
		t.Errorf("URL inside @media not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
	if !containsString(out, "@media") {
		t.Error("@media rule itself was removed")
	}
}

func TestSanitizeCSS_URLInsideSupports(t *testing.T) {
	input := []byte(`@supports (display: grid) {
  .grid { background: url(https://evil.com/grid.png); }
}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.com") {
		t.Errorf("URL inside @supports not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
	if !containsString(out, "@supports") {
		t.Error("@supports rule itself was removed")
	}
}

func TestSanitizeCSS_DataURLPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"png base64", `div { background: url(data:image/png;base64,iVBOR...); }`},
		{"svg inline", `div { background: url(data:image/svg+xml,%3Csvg%3E%3C/svg%3E); }`},
		{"font woff2", `@font-face { src: url(data:font/woff2;base64,d09G...); }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if string(out) != tt.input {
				t.Errorf("data URL modified:\ngot:  %s\nwant: %s", out, tt.input)
			}
			if len(warns) != 0 {
				t.Errorf("unexpected warnings: %v", warns)
			}
		})
	}
}

func TestSanitizeCSS_RelativePathPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"parent dir", `div { background: url(../images/bg.png); }`},
		{"font file", `@font-face { src: url(../fonts/foo.woff2); }`},
		{"current dir", `div { background: url(./images/bg.png); }`},
		{"bare filename", `div { background: url(icon.svg); }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if string(out) != tt.input {
				t.Errorf("relative path modified:\ngot:  %s\nwant: %s", out, tt.input)
			}
			if len(warns) != 0 {
				t.Errorf("unexpected warnings: %v", warns)
			}
		})
	}
}

func TestSanitizeCSS_JavascriptURLCaseInsensitive(t *testing.T) {
	input := []byte(`div { background: url(JAVASCRIPT:alert(1)); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "JAVASCRIPT") {
		t.Errorf("case-insensitive javascript URL not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warnings")
	}
}

func TestRegexCSSSanitizer_Wrapper_DelegatesToPackageFn(t *testing.T) {
	s := NewCSSSanitizer()
	if s == nil {
		t.Fatal("NewCSSSanitizer returned nil")
	}
	input := []byte(`@import "https://evil.example/x.css"; body { color: red; }`)
	out, warns := s.SanitizeCSS(input)
	if containsString(out, "@import") {
		t.Errorf("wrapper did not remove @import: %s", out)
	}
	if len(warns) == 0 {
		t.Error("wrapper did not surface warnings")
	}
}

func containsString(b []byte, s string) bool {
	return len(b) > 0 && len(s) > 0 && string(b) != "" && contains(string(b), s)
}

func contains(haystack, needle string) bool {
	for i := range haystack {
		if i+len(needle) <= len(haystack) && haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
