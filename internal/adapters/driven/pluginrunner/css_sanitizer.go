package pluginrunner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// reZeroVal is the RE2-compatible suffix for a CSS zero value with optional unit.
// Matches the tail of: 0, 0px, 0%, 0rem, 0em, 0vh, 0vw (exact zero, not 0.5px or 01px).
// The trailing (?:$|[^\d.]) ensures the zero is not the start of a larger number.
// The prefix pattern (?:^|[\s;{])<prop>\s*:\s* anchors to the property name so that
// compound properties like margin-top or padding-bottom are not matched.
const reZeroVal = `0(?:px|%|r?em|v[hw])?(?:$|[^\d.])`

var (
	reImport        = regexp.MustCompile(`(?i)@import\s+[^;]+;?`)
	reURLExternal   = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*(https?://|//)[^)]*\)`)
	reURLJavascript = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*javascript\s*:[^)]*\)`)
	reExpression    = regexp.MustCompile(`(?i)expression\s*\([^)]*\)`)
	reMozBinding    = regexp.MustCompile(`(?i)-moz-binding\s*:[^;}"]+[;}]?`)
	reBehavior      = regexp.MustCompile(`(?i)\bbehavior\s*:[^;}"]+[;}]?`)

	// Sensitive selector patterns that should not be styled by themes.
	reSensitiveSelector = regexp.MustCompile(`(?i)\[type\s*=\s*["']?(password|hidden)["']?\]|\.admin-[a-z0-9_-]*|\.token-[a-z0-9_-]*|\.secret-[a-z0-9_-]*`)

	// Full-screen overlay pattern: position fixed/sticky + full-viewport dimensions.
	// Matches rules containing position:fixed|sticky together with width:100vw or height:100vh.
	rePosFixed   = regexp.MustCompile(`(?i)\bposition\s*:\s*(fixed|sticky)\b`)
	reFullWidth  = regexp.MustCompile(`(?i)\bwidth\s*:\s*100vw\b`)
	reFullHeight = regexp.MustCompile(`(?i)\bheight\s*:\s*100vh\b`)
	// Zero-offset side properties: the (?:^|[\s;{]) prefix prevents matching compound
	// properties like margin-top or padding-bottom (which end in -top/-bottom).
	reFullTop    = regexp.MustCompile(`(?i)(?:^|[\s;{])top\s*:\s*` + reZeroVal)
	reFullRight  = regexp.MustCompile(`(?i)(?:^|[\s;{])right\s*:\s*` + reZeroVal)
	reFullBottom = regexp.MustCompile(`(?i)(?:^|[\s;{])bottom\s*:\s*` + reZeroVal)
	reFullLeft   = regexp.MustCompile(`(?i)(?:^|[\s;{])left\s*:\s*` + reZeroVal)
	// inset shorthand covers all four sides at once; a zero-leading value combined with
	// rePosFixed is an unambiguous full-screen overlay signal.
	reFullInset = regexp.MustCompile(`(?i)\binset\s*:\s*0`)

	// pointer-events:none on interactive elements.
	rePointerEventsNone   = regexp.MustCompile(`(?i)\bpointer-events\s*:\s*none\b`)
	reInteractiveSelector = regexp.MustCompile(`(?i)\bbutton\b|\[role\s*=\s*["']?button["']?\]|a\[href[^]]*logout[^]]*\]|\[data-critical\]`)

	// z-index clamping: any z-index value > 999.
	reZIndex = regexp.MustCompile(`(?i)\bz-index\s*:\s*(\d+)\b`)
)

// RegexCSSSanitizer implements plugin.CSSSanitizer using regex-based rules.
type RegexCSSSanitizer struct{}

func NewCSSSanitizer() *RegexCSSSanitizer { return &RegexCSSSanitizer{} }

func (r *RegexCSSSanitizer) SanitizeCSS(input []byte) ([]byte, []string) {
	return SanitizeCSS(input)
}

// SanitizeCSS removes dangerous CSS constructs that could be used for
// data exfiltration, code execution, or UI-redressing attacks.
// Returns sanitized CSS and warnings.
func SanitizeCSS(input []byte) ([]byte, []string) {
	if len(input) == 0 {
		return input, nil
	}

	css := string(input)
	var warnings []string

	// Normalize CSS escape sequences before applying detection/removal (M2).
	// CSS escape sequences (e.g. `\3a ` → `:`, `\65 ` → `e`) can be used to bypass
	// literal-string regex matching. By decoding escapes first, all subsequent
	// detection regexes run against the semantically equivalent plain form.
	// The sanitized (decoded) string is what we return; it is functionally identical
	// to the original as interpreted by a CSS parser.
	css = decodeCSSEscapes(css)

	if matches := reImport.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed @import: "+strings.TrimSpace(m))
		}
		css = reImport.ReplaceAllString(css, "")
	}

	if matches := reURLJavascript.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed javascript url: "+strings.TrimSpace(m))
		}
		css = reURLJavascript.ReplaceAllString(css, "none")
	}

	if matches := reURLExternal.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed external url: "+strings.TrimSpace(m))
		}
		css = reURLExternal.ReplaceAllString(css, "none")
	}

	if matches := reExpression.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed expression(): "+strings.TrimSpace(m))
		}
		css = reExpression.ReplaceAllString(css, "none")
	}

	if matches := reMozBinding.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed -moz-binding: "+strings.TrimSpace(m))
		}
		css = reMozBinding.ReplaceAllString(css, "")
	}

	if matches := reBehavior.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed behavior: "+strings.TrimSpace(m))
		}
		css = reBehavior.ReplaceAllString(css, "")
	}

	// Strip rules containing sensitive selectors (password inputs, admin/token/secret classes).
	css = stripRulesWithSensitiveSelectors(css, &warnings)

	// Block full-screen overlay rules: position:fixed|sticky + top:0 + left:0 + full viewport size.
	css = stripFullScreenOverlays(css, &warnings)

	// Strip pointer-events:none on interactive elements (anti-clickjacking).
	css = stripPointerEventsOnInteractive(css, &warnings)

	// Clamp z-index > 999 to 999.
	css = clampZIndex(css, &warnings)

	return []byte(css), warnings
}

// stripRulesWithSensitiveSelectors removes entire CSS rule blocks that target
// sensitive element selectors (password inputs, .admin-*, .token-*, .secret-*).
// CSS escape sequences are decoded upstream by SanitizeCSS before this runs.
func stripRulesWithSensitiveSelectors(css string, warnings *[]string) string {
	// We process the CSS looking for selector+block pairs where the selector matches.
	// Strategy: scan for { } blocks and check the preceding selector text.
	var result strings.Builder
	remaining := css
	for {
		braceOpen := strings.Index(remaining, "{")
		if braceOpen < 0 {
			result.WriteString(remaining)
			break
		}
		selector := remaining[:braceOpen]
		if reSensitiveSelector.MatchString(selector) {
			// Find the matching closing brace (handle nesting).
			closeIdx := findMatchingBrace(remaining, braceOpen)
			if closeIdx < 0 {
				// Malformed: drop the rest.
				*warnings = append(*warnings, "removed rule with sensitive selector (malformed block): "+strings.TrimSpace(selector))
				break
			}
			*warnings = append(*warnings, "removed rule targeting sensitive selector: "+strings.TrimSpace(selector))
			remaining = remaining[closeIdx+1:]
		} else {
			// Safe selector: emit up to and including the closing brace.
			closeIdx := findMatchingBrace(remaining, braceOpen)
			if closeIdx < 0 {
				result.WriteString(remaining)
				break
			}
			result.WriteString(remaining[:closeIdx+1])
			remaining = remaining[closeIdx+1:]
		}
	}
	return result.String()
}

// findMatchingBrace returns the index of the } that closes the { at openIdx,
// accounting for nested braces. Returns -1 if not found.
func findMatchingBrace(s string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// stripFullScreenOverlays removes CSS rule blocks that combine position:fixed|sticky
// with full-viewport geometry (top:0, left:0, width:100vw or height:100vh).
// This prevents themes from rendering full-screen phishing overlays.
func stripFullScreenOverlays(css string, warnings *[]string) string {
	var result strings.Builder
	remaining := css
	for {
		braceOpen := strings.Index(remaining, "{")
		if braceOpen < 0 {
			result.WriteString(remaining)
			break
		}
		closeIdx := findMatchingBrace(remaining, braceOpen)
		if closeIdx < 0 {
			result.WriteString(remaining)
			break
		}
		block := remaining[braceOpen : closeIdx+1]
		selector := remaining[:braceOpen]

		if isFullScreenOverlay(block) {
			*warnings = append(*warnings, "removed full-screen overlay rule: "+strings.TrimSpace(selector))
			remaining = remaining[closeIdx+1:]
		} else {
			result.WriteString(remaining[:closeIdx+1])
			remaining = remaining[closeIdx+1:]
		}
	}
	return result.String()
}

// isFullScreenOverlay returns true when a CSS block contains position:fixed|sticky
// combined with at least two of: top/right/bottom/left (any zero-unit), inset:0, width:100vw, height:100vh.
// inset:0 (the shorthand that sets all four sides) counts as two signals on its own
// because it is semantically equivalent to top:0 + right:0 + bottom:0 + left:0.
func isFullScreenOverlay(block string) bool {
	if !rePosFixed.MatchString(block) {
		return false
	}
	// inset:0 is equivalent to all four sides pinned to zero; treat it as two signals.
	if reFullInset.MatchString(block) {
		return true
	}
	count := 0
	if reFullTop.MatchString(block) {
		count++
	}
	if reFullRight.MatchString(block) {
		count++
	}
	if reFullBottom.MatchString(block) {
		count++
	}
	if reFullLeft.MatchString(block) {
		count++
	}
	if reFullWidth.MatchString(block) {
		count++
	}
	if reFullHeight.MatchString(block) {
		count++
	}
	return count >= 2
}

// stripPointerEventsOnInteractive removes pointer-events:none declarations from
// rules whose selector targets interactive elements (buttons, logout links, critical).
func stripPointerEventsOnInteractive(css string, warnings *[]string) string {
	var result strings.Builder
	remaining := css
	for {
		braceOpen := strings.Index(remaining, "{")
		if braceOpen < 0 {
			result.WriteString(remaining)
			break
		}
		closeIdx := findMatchingBrace(remaining, braceOpen)
		if closeIdx < 0 {
			result.WriteString(remaining)
			break
		}
		selector := remaining[:braceOpen]
		block := remaining[braceOpen : closeIdx+1]

		if reInteractiveSelector.MatchString(selector) && rePointerEventsNone.MatchString(block) {
			// Remove only the pointer-events:none declaration, keep rest of block.
			cleaned := rePointerEventsNone.ReplaceAllString(block, "/* pointer-events removed */")
			*warnings = append(*warnings, "removed pointer-events:none from interactive selector: "+strings.TrimSpace(selector))
			result.WriteString(selector)
			result.WriteString(cleaned)
		} else {
			result.WriteString(remaining[:closeIdx+1])
		}
		remaining = remaining[closeIdx+1:]
	}
	return result.String()
}

// decodeCSSEscapes returns a copy of s with CSS escape sequences resolved so
// that detection regexes cannot be bypassed by hex-encoding (e.g. `\3a ` → `:`).
// It handles:
//   - `\` followed by 1-6 hex digits and an optional single trailing whitespace
//     (per CSS Syntax Level 3 §4.3.7)
//   - simple backslash-escaped non-hex characters (`\x` → `x`)
//
// The result is semantically equivalent to the input as interpreted by a CSS parser.
func decodeCSSEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// s[i] == '\\'
		if i+1 >= len(s) {
			// trailing backslash - emit as-is
			b.WriteByte('\\')
			i++
			continue
		}
		// Collect up to 6 hex digits.
		j := i + 1
		hexEnd := j
		for hexEnd < len(s) && hexEnd-j < 6 && isHexDigit(s[hexEnd]) {
			hexEnd++
		}
		if hexEnd > j {
			// Hex escape: parse the code point.
			cp, err := strconv.ParseInt(s[j:hexEnd], 16, 32)
			if err != nil || cp == 0 || cp > 0x10FFFF {
				// Invalid code point; emit a replacement character so the
				// decoded form never accidentally forms a valid keyword.
				b.WriteRune(unicode.ReplacementChar)
			} else {
				b.WriteRune(rune(cp))
			}
			// Consume an optional single trailing whitespace (CSS spec rule).
			if hexEnd < len(s) && (s[hexEnd] == ' ' || s[hexEnd] == '\t' ||
				s[hexEnd] == '\n' || s[hexEnd] == '\r' || s[hexEnd] == '\f') {
				hexEnd++
			}
			i = hexEnd
			continue
		}
		// Simple escape `\x` → `x` (non-hex character after backslash).
		b.WriteByte(s[i+1])
		i += 2
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// clampZIndex replaces any z-index value above 999 with 999.
func clampZIndex(css string, warnings *[]string) string {
	return reZIndex.ReplaceAllStringFunc(css, func(match string) string {
		sub := reZIndex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		val, err := strconv.Atoi(sub[1])
		if err != nil || val <= 999 {
			return match
		}
		*warnings = append(*warnings, fmt.Sprintf("clamped z-index %d to 999", val))
		return reZIndex.ReplaceAllString(match, "z-index: 999")
	})
}
