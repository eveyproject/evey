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

// ---- Red-team regression tests (2026-07-27 audit) ----
//
// These prove that the brace-confusion and external-URL-detection bypasses
// found in the pre-auth theme.css sanitizer are neutralized. Cases are
// adapted from the red-team harness at
// ~/evey-audit-2026-07-27/redteam-css-tests/ (css_brace_confusion_redteam_test.go,
// css_redteam_probe_test.go); that harness proves the bypass exists, these
// tests prove the fix holds.

func TestSanitizeCSS_Redteam_OverlayBehindContentBraceClose(t *testing.T) {
	// content:"}" desyncs a byte-level brace counter: it "closes" the block
	// before the overlay declarations are ever inspected.
	input := []byte(`.aurora-splash{content:"}";position:fixed;inset:0;width:100vw;height:100vh;background:#0b0f1a}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position:fixed") || containsString(out, "inset:0") {
		t.Errorf("overlay hidden behind content:\"}\" survived: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_Redteam_UnbalancedContentBraceDropsFollowingRules(t *testing.T) {
	// content:"{" desyncs the counter the other way: a naive scanner never
	// finds a matching close and (pre-fix) recopied the entire remainder,
	// including an overlay, a password-field rule and a clickjacking rule,
	// completely uninspected.
	input := []byte(`.decoy{content:"{"}` +
		`#ovl{position:fixed;inset:0;background:#000;z-index:999}` +
		` input[type=password]{opacity:0.01;background:#fff}` +
		` button{pointer-events:none}`)
	out, warns := SanitizeCSS(input)
	for _, marker := range []string{"position:fixed", "inset:0", "type=password", "pointer-events:none"} {
		if containsString(out, marker) {
			t.Errorf("marker %q survived unbalanced-brace bypass: %s", marker, out)
		}
	}
	if len(warns) == 0 {
		t.Error("expected warnings for dropped/stripped rules")
	}
}

func TestSanitizeCSS_Redteam_SensitiveSelectorBehindUnbalancedBrace(t *testing.T) {
	input := []byte(`.brand{content:"{"}` + "\n" + `.token-badge{position:absolute;left:-9999px}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, ".token-badge") {
		t.Errorf(".token-badge rule survived unbalanced-brace bypass: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for sensitive selector removal")
	}
}

func TestSanitizeCSS_Redteam_PasswordSelectorBehindContentBraceClose(t *testing.T) {
	input := []byte(`.decoy{color:#111;content:"{"}` + "\n" + `input[type=password]{opacity:0;position:fixed;top:40%;left:40%}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "type=password") {
		t.Errorf("password-field rule survived content-brace bypass: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for sensitive selector removal")
	}
}

func TestSanitizeCSS_Redteam_CommentContainingBraceDoesNotDesync(t *testing.T) {
	// A CSS comment containing } must not be counted as a real closing brace.
	input := []byte(`.ov{/* } */position:fixed;inset:0;width:100vw;height:100vh}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position:fixed") {
		t.Errorf("overlay hidden behind a comment brace survived: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_Redteam_HexEscapedBraceDoesNotDesync(t *testing.T) {
	// \7d decodes to '}' but must never be treated as a structural
	// delimiter during brace counting - only after the block boundaries
	// are already established.
	input := []byte(`.ov{content:"\7d";position:fixed;inset:0;width:100vw;height:100vh}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position:fixed") {
		t.Errorf("overlay hidden behind hex-escaped brace survived: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_Redteam_NestedAtRuleOverlayStripped(t *testing.T) {
	input := []byte(`@media screen{.ov{content:"}";position:fixed;inset:0;width:100vw;height:100vh}}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "position:fixed") {
		t.Errorf("overlay nested in @media survived: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for overlay removal")
	}
}

func TestSanitizeCSS_Redteam_ImageSetBareStringExternalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain image-set", `body{background-image:image-set("https://evil.example/a.png" 1x)}`},
		{"webkit image-set", `body{background-image:-webkit-image-set("https://evil.example/a.png" 1x)}`},
		{"list-style-image", `li{list-style-image:image-set("https://evil.example/b.png" 1x)}`},
		{"border-image-source", `div{border-image-source:image-set("https://evil.example/c.png" 1x)}`},
		{"cursor", `a{cursor:image-set("https://evil.example/d.png" 1x),pointer}`},
		{"mask-image", `div{-webkit-mask-image:image-set("https://evil.example/e.png" 1x)}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "evil.example") {
				t.Errorf("external URL via image-set() not neutralized: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warning for external image-set URL")
			}
		})
	}
}

func TestSanitizeCSS_Redteam_ImageSetViaCustomPropertyVar(t *testing.T) {
	// The external URL sits in a custom property, referenced indirectly via
	// var() inside image-set() - the regex can't resolve var() to fetch it
	// but neutralizing the custom property declaration itself defangs it.
	input := []byte(`:root{--u:"https://evil.example/a.png"}body{background-image:image-set(var(--u) 1x)}`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.example") {
		t.Errorf("external URL smuggled via custom property survived: %s", out)
	}
	if len(warns) == 0 {
		t.Error("expected warning for external URL in custom property")
	}
}

func TestSanitizeCSS_Redteam_UnterminatedURLAtEOF(t *testing.T) {
	input := []byte(`body{background-image:url(https://evil.example/beacon.png`)
	out, warns := SanitizeCSS(input)
	if containsString(out, "evil.example") {
		t.Errorf("unterminated url() at EOF survived: %s", out)
	}
	_ = warns
}

func TestSanitizeCSS_Redteam_ImportNoWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"double quote", `@import"https://evil.example/x.css";body{color:#111}`},
		{"single quote", `@import'https://evil.example/x.css';body{color:#111}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "@import") || containsString(out, "evil.example") {
				t.Errorf("@import without whitespace not stripped: %s", out)
			}
			if !containsString(out, "color:#111") {
				t.Error("valid trailing CSS was incorrectly removed")
			}
			if len(warns) == 0 {
				t.Error("expected warning for @import removal")
			}
		})
	}
}

// ---- Residual holes (2026-07-27 re-red-team) ----

func TestSanitizeCSS_ImageSetExternal_NonFirstCandidate(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain background", `body{background:image-set("a.png" 1x, "https://192.0.2.1/exfil" 2x)}`},
		{"mask-image with url() first candidate", `.x{mask-image:image-set(url(a.png) 1x, "https://192.0.2.1/exfil" 2x)}`},
		{"custom property", `:root{--i:image-set("a.png" 1x, "https://192.0.2.1/exfil" 2x)}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "192.0.2.1") {
				t.Errorf("non-first image-set() candidate not neutralized: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warning for image-set() external candidate")
			}
		})
	}
}

func TestSanitizeCSS_ImageSetExternal_AllLocalCandidatesPreserved(t *testing.T) {
	input := []byte(`body{background:image-set("a.png" 1x, "b.png" 2x)}`)
	out, warns := SanitizeCSS(input)
	if !containsString(out, "a.png") || !containsString(out, "b.png") {
		t.Errorf("all-local image-set() incorrectly modified: %s", out)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings for all-local image-set(): %v", warns)
	}
}

func TestSanitizeCSS_AtRuleNesting_SensitiveSelectorAndPointerEvents(t *testing.T) {
	tests := []struct {
		name  string
		input string
		bad   string
	}{
		{"media password", `@media screen{input[type=password]{opacity:0}}`, "opacity:0"},
		{"supports admin-panel", `@supports(display:grid){.admin-panel{display:none}}`, "display:none"},
		{"layer token-field", `@layer theme{.token-field{color:transparent}}`, "color:transparent"},
		{"media pointer-events button", `@media screen{button{pointer-events:none}}`, "pointer-events:none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, tt.bad) {
				t.Errorf("nested at-rule sensitive rule not stripped: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warning for nested at-rule strip")
			}
		})
	}
}

func TestSanitizeCSS_AtRuleNesting_SafeRulesPreserved(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"media normal", `@media screen{.normal{color:red}}`},
		{"keyframes", `@keyframes x{from{opacity:0}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if string(out) != tt.input {
				t.Errorf("safe nested at-rule modified:\ngot:  %s\nwant: %s", out, tt.input)
			}
			if len(warns) != 0 {
				t.Errorf("unexpected warnings: %v", warns)
			}
		})
	}
}

func TestSanitizeCSS_FixedHeader_NotOverlay(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no dimension", `.header{position:fixed;top:0;left:0;height:48px}`},
		{"height px, width percent (not viewport unit)", `.header{position:fixed;top:0;left:0;height:48px;width:100%}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if string(out) != tt.input {
				t.Errorf("legitimate fixed header incorrectly removed:\ngot:  %s\nwant: %s", out, tt.input)
			}
			if len(warns) != 0 {
				t.Errorf("unexpected warnings for fixed header: %v", warns)
			}
		})
	}
}

func TestSanitizeCSS_FixedHeader_RealOverlaysStillBlocked(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"inset:0", `.ov{position:fixed;inset:0}`},
		{"top/left + viewport dims", `.ov{position:fixed;top:0;left:0;width:100vw;height:100vh}`},
		{"four sides = inset equivalent", `.ov{position:fixed;top:0;left:0;right:0;bottom:0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "position:fixed") {
				t.Errorf("real overlay not blocked: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected warning for overlay removal")
			}
		})
	}
}

// The overlay check used to be handed a whole grouping at-rule as ONE block, so
// it summed the signals of unrelated rules inside it. A fixed header and a
// full-bleed hero in the same breakpoint - the most ordinary responsive CSS
// there is - were deleted together, with a warning blaming the @media itself,
// while the exact same two rules at root level passed untouched.
//
// The existing at-rule tests could not catch it: they each put a SINGLE rule
// inside the at-rule, which passes whatever the aggregation does.
func TestSanitizeCSS_AtRule_DoesNotSumSignalsAcrossSiblingRules(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"media", `@media (max-width:600px){.topbar{position:fixed;top:0}.hero{width:100vw}}`},
		{"supports", `@supports (display:grid){.bar{position:sticky;top:0}.full{height:100vh}}`},
		{"signals split three ways", `@layer t{.a{position:fixed}.b{top:0;left:0}.c{width:100vw}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if string(out) != tt.input {
				t.Errorf("legitimate rules inside an at-rule were removed:\ngot:  %s\nwant: %s", out, tt.input)
			}
			if len(warns) != 0 {
				t.Errorf("unexpected warnings: %v", warns)
			}
		})
	}
}

// The browser cascades two rules on the same selector into one overlay, so
// splitting the declarations was a one-line bypass of this control - no
// escaping, no var(), none of the obfuscation the rest of the file defends
// against. The signals must accumulate per selector, not per block.
func TestSanitizeCSS_OverlaySplitAcrossRules_IsStillRemoved(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"two blocks", `.ov{position:fixed}.ov{top:0;left:0;width:100vw;height:100vh}`},
		{"inset in the second", `.ov{position:fixed;z-index:99}.ov{inset:0}`},
		{"comment between", `.ov{position:fixed}/*x*/.ov{inset:0}`},
		{"selector list", `.a,.ov{position:fixed}.ov{inset:0}`},
		{"whitespace differs", `.ov   {position:fixed}   .ov{inset:0}`},
		{"descendant combinator spacing", `.a  >  .ov{position:fixed}.a > .ov{inset:0}`},
		{"reversed order", `.ov{inset:0}.ov{position:fixed}`},
		{"sticky variant", `.ov{position:sticky}.ov{width:100vw;height:100vh}`},
		{"both halves inside one at-rule", `@media screen{.ov{position:fixed}.ov{inset:0}}`},
		{"at-rules nested two deep", `@media screen{@supports(display:grid){.ov{position:fixed}.ov{inset:0}}}`},
		{"completed inside an at-rule", `.ov{position:fixed}@media screen{.ov{inset:0}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, warns := SanitizeCSS([]byte(tt.input))
			if containsString(out, "inset:0") || containsString(out, "100vw") {
				t.Errorf("the overlay survived by being split across rules: %s", out)
			}
			if len(warns) == 0 {
				t.Error("expected a warning for the split overlay")
			}
		})
	}
}

// Accumulating per selector must not become accumulating per scope: two
// DIFFERENT selectors each holding half the signals are two ordinary rules.
func TestSanitizeCSS_OverlaySignals_DoNotCrossSelectors(t *testing.T) {
	for _, in := range []string{
		`.ov{position:fixed}.other{inset:0}`,
		`.topbar{position:fixed;top:0}.hero{width:100vw;height:100vh}`,
	} {
		out, warns := SanitizeCSS([]byte(in))
		if string(out) != in {
			t.Errorf("signals leaked between selectors:\ngot:  %s\nwant: %s", out, in)
		}
		if len(warns) != 0 {
			t.Errorf("unexpected warnings: %v", warns)
		}
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
