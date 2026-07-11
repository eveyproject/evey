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
	// \75\72\6c is "url" in CSS unicode escapes - regex-based sanitizer
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

// ---- Tests for enhanced rules (phase 5) ----

func TestSanitizeCSS_SensitiveSelector_PasswordInput(t *testing.T) {
	input := []byte(`input[type=password] { outline: 2px solid red; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "outline") {
		t.Errorf("password input rule not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for sensitive selector removal")
	}
}

func TestSanitizeCSS_SensitiveSelector_AdminClass(t *testing.T) {
	input := []byte(`.admin-panel { display: none; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "display: none") {
		t.Errorf(".admin-* rule not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for admin class removal")
	}
}

func TestSanitizeCSS_SensitiveSelector_TokenClass(t *testing.T) {
	input := []byte(`.token-display { visibility: hidden; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "visibility") {
		t.Errorf(".token-* rule not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for token class removal")
	}
}

func TestSanitizeCSS_SensitiveSelector_SecretClass(t *testing.T) {
	input := []byte(`.secret-key { color: transparent; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "transparent") {
		t.Errorf(".secret-* rule not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for secret class removal")
	}
}

func TestSanitizeCSS_SensitiveSelector_SafeRulesPreserved(t *testing.T) {
	input := []byte(`.card { color: red; } .admin-panel { display: none; } .footer { margin: 0; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "display: none") {
		t.Errorf(".admin-* rule should be removed")
	}
	if !containsString(out, "color: red") {
		t.Error(".card rule was incorrectly removed")
	}
	if !containsString(out, "margin: 0") {
		t.Error(".footer rule was incorrectly removed")
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
}

func TestSanitizeCSS_FullScreenOverlay_Blocked(t *testing.T) {
	input := []byte(`.overlay { position: fixed; top: 0; left: 0; width: 100vw; height: 100vh; background: rgba(0,0,0,0.9); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("full-screen overlay not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for full-screen overlay removal")
	}
}

func TestSanitizeCSS_FullScreenOverlay_StickyBlocked(t *testing.T) {
	input := []byte(`.phish { position: sticky; top: 0; width: 100vw; height: 100vh; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: sticky") {
		t.Errorf("full-screen sticky overlay not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
}

func TestSanitizeCSS_FullScreenOverlay_PartialNotBlocked(t *testing.T) {
	// position:fixed + only width:100vw, no offset side - only 1 signal, NOT a full overlay
	input := []byte(`.banner { position: fixed; width: 100vw; }`)
	out, warns := SanitizeCSS(input)
	if !containsString(out, "position: fixed") {
		t.Errorf("partial fixed rule incorrectly removed: %s", out)
	}
	_ = warns
}

func TestSanitizeCSS_PointerEventsNone_OnButton(t *testing.T) {
	input := []byte(`button { pointer-events: none; color: red; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "pointer-events: none") {
		t.Errorf("pointer-events:none on button not removed: %s", out)
	}
	// The rest of the rule should be preserved.
	if !containsString(out, "color: red") {
		t.Errorf("legitimate declaration removed from button rule: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
}

func TestSanitizeCSS_PointerEventsNone_OnNonInteractive(t *testing.T) {
	// pointer-events:none on a non-interactive element is fine (e.g. decorative overlay layer)
	input := []byte(`.decoration { pointer-events: none; width: 100px; }`)
	out, warns := SanitizeCSS(input)
	if !containsString(out, "pointer-events: none") {
		t.Errorf("pointer-events:none on decoration incorrectly removed: %s", out)
	}
	_ = warns
}

func TestSanitizeCSS_ZIndex_ClampedAbove999(t *testing.T) {
	input := []byte(`.modal { z-index: 9999; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "9999") {
		t.Errorf("z-index 9999 not clamped: %s", out)
	}
	if !containsString(out, "999") {
		t.Errorf("z-index not clamped to 999: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for z-index clamping")
	}
}

func TestSanitizeCSS_ZIndex_AllowedBelow1000(t *testing.T) {
	input := []byte(`.modal { z-index: 500; }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("z-index 500 should not be modified: %s", out)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings for z-index 500: %v", warns)
	}
}

func TestSanitizeCSS_ZIndex_ExactBoundary999(t *testing.T) {
	input := []byte(`.modal { z-index: 999; }`)
	out, warns := SanitizeCSS(input)
	if string(out) != string(input) {
		t.Errorf("z-index 999 should not be modified: %s", out)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings for z-index 999: %v", warns)
	}
}

func TestSanitizeCSS_AllNewRulesPass_SafeCSS(t *testing.T) {
	// Transitions, animations, custom props, flexbox, grid all pass through.
	input := []byte(`
:root { --accent: #ff0; }
.card { display: flex; flex-direction: column; gap: 16px; }
.grid { display: grid; grid-template-columns: repeat(3, 1fr); }
@keyframes slide { from { transform: translateX(0); } to { transform: translateX(100%); } }
.fade { transition: opacity 0.3s ease; animation: slide 1s; }
.logo { background: url(data:image/png;base64,iVBORz...); }
@font-face { src: url(data:font/woff2;base64,d09G...); }
.item { z-index: 500; }
.decoration { pointer-events: none; }
`)
	out, warns := SanitizeCSS(input)
	if len(warns) != 0 {
		t.Errorf("safe CSS produced unexpected warnings: %v\noutput: %s", warns, out)
	}
	if !containsString(out, "display: flex") {
		t.Error("flexbox removed")
	}
	if !containsString(out, "display: grid") {
		t.Error("grid removed")
	}
	if !containsString(out, "@keyframes") {
		t.Error("keyframes removed")
	}
	if !containsString(out, "data:image/png") {
		t.Error("data image URL removed")
	}
	if !containsString(out, "data:font/woff2") {
		t.Error("data font URL removed")
	}
	if !containsString(out, "z-index: 500") {
		t.Error("z-index 500 removed")
	}
}

// ---- M1: broadened overlay filter tests ----

func TestSanitizeCSS_OverlayM1_TopPxBlocked(t *testing.T) {
	// top:0px (with unit) + position:fixed + full viewport - must be detected
	input := []byte(`.phish { position: fixed; top: 0px; left: 0px; width: 100vw; height: 100vh; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("overlay with top:0px/left:0px not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_OverlayM1_RightBottomBlocked(t *testing.T) {
	// right:0 + bottom:0 + position:fixed + full-width - must be detected
	input := []byte(`.phish { position: fixed; right: 0; bottom: 0; width: 100vw; height: 100vh; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("overlay with right:0/bottom:0 not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_OverlayM1_InsetShorthandBlocked(t *testing.T) {
	// inset:0 shorthand + position:fixed - must be detected as overlay
	input := []byte(`.phish { position: fixed; inset: 0; width: 100vw; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("overlay with inset:0 not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_OverlayM1_InsetMultiValueBlocked(t *testing.T) {
	// inset: 0 0 0 0 + position:fixed - must be detected as overlay
	input := []byte(`.phish { position: fixed; inset: 0 0 0 0; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("overlay with inset:0 0 0 0 not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_OverlayM1_MarginTopNotMisclassified(t *testing.T) {
	// margin-top:0 must NOT be treated as a zero-offset side - no false positive
	input := []byte(`.header { position: fixed; width: 100vw; margin-top: 0; }`)
	out, warns := SanitizeCSS(input)
	// Only 1 real overlay signal (width:100vw), no zero-side offset → NOT removed
	if !containsString(out, "position: fixed") {
		t.Errorf("rule with margin-top:0 incorrectly classified as overlay: %s\nwarnings: %v", out, warns)
	}
}

func TestSanitizeCSS_OverlayM1_ZeroRemBlocked(t *testing.T) {
	// top:0rem is a zero offset - must be detected
	input := []byte(`.phish { position: fixed; top: 0rem; left: 0rem; width: 100vw; height: 100vh; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position: fixed") {
		t.Errorf("overlay with top:0rem not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
}

// ---- M2: CSS escape decode tests ----

func TestSanitizeCSS_EscapeBypass_ExternalURLColon(t *testing.T) {
	// \3a is the CSS hex escape for ':' - url(https\3a //evil.com) must be detected
	input := []byte(`div { background: url(https\3a //evil.com/beacon); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.com") {
		t.Errorf("escape-obfuscated external URL not stripped: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for escaped external URL")
	}
}

func TestSanitizeCSS_EscapeBypass_ExpressionKeyword(t *testing.T) {
	// \65 is 'e' - expr\65 ssion(...) must be detected and stripped
	input := []byte(`div { width: expr\65 ssion(document.body.clientWidth); }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "expr") {
		t.Errorf("escape-obfuscated expression() not stripped: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for escaped expression()")
	}
}

func TestSanitizeCSS_EscapeBypass_LocalURLPreserved(t *testing.T) {
	// A normal local url with no escapes must survive
	input := []byte(`div { background: url('/local.png'); }`)
	out, warns := SanitizeCSS(input)
	if !containsString(out, "/local.png") {
		t.Errorf("local url was incorrectly removed: %s", out)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings for local url: %v", warns)
	}
}

func TestSanitizeCSS_EscapeBypass_SimpleEscapeInIdentifier(t *testing.T) {
	// \61 = 'a', so \61 dmin-panel = admin-panel - sensitive selector bypass via escape
	input := []byte(`.\61 dmin-panel { display: none; }`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "display: none") {
		t.Errorf("escape-obfuscated .admin-* selector not removed: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for escaped sensitive selector")
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
