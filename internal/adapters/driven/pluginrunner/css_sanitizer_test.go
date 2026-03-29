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
