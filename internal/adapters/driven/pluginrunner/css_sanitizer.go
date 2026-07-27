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
	// @import\b (not @import\s+) so that a scheme-quote glued directly onto
	// the keyword - `@import"https://evil/x.css";` - is still matched; the
	// word boundary still rejects unrelated at-rules like @important.
	reImport        = regexp.MustCompile(`(?i)@import\b\s*[^;]+;?`)
	reURLExternal   = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*(https?://|//)[^)]*\)`)
	reURLJavascript = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*javascript\s*:[^)]*\)`)
	reExpression    = regexp.MustCompile(`(?i)expression\s*\([^)]*\)`)
	reMozBinding    = regexp.MustCompile(`(?i)-moz-binding\s*:[^;}"]+[;}]?`)
	reBehavior      = regexp.MustCompile(`(?i)\bbehavior\s*:[^;}"]+[;}]?`)

	// reURLExternalUnterminated catches a url(...) that is never closed with
	// a `)` before EOF (e.g. truncated by a proxy, or deliberately malformed
	// to slip past reURLExternal, which requires the closing paren).
	reURLExternalUnterminated = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*(https?://|//)[^)]*$`)

	// reImageSetExternal catches external references fetched WITHOUT ever
	// using the url() token: image-set()/-webkit-image-set() accept a bare
	// quoted string as their first argument, and are valid in any
	// image-valued property (list-style-image, cursor, border-image-source,
	// -webkit-mask-image, background(-image), ...). The property name is
	// irrelevant here since the fetch happens inside the function call.
	reImageSetExternal = regexp.MustCompile(`(?i)(?:-webkit-)?image-set\s*\(\s*["'](?:https?://|//)[^;{})]*\)`)

	// reCustomPropExternalURL catches an external URL stashed in a custom
	// property (--x: "https://...") and later referenced indirectly via
	// var(--x) - most commonly inside image-set(), which the regex above
	// cannot resolve. Neutralizing the property at its declaration defangs
	// every var(--x) use site without needing to track references.
	reCustomPropExternalURL = regexp.MustCompile(`(?i)(--[a-zA-Z0-9_-]+)\s*:\s*["']?(?:https?://|//)[^;}"']*["']?`)

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

	// Structural (brace-boundary) passes run FIRST, directly on the raw,
	// still-escaped CSS. findMatchingBrace is string/comment/url()-aware, so
	// a `}`/`{` embedded in a string literal, a comment, or a url() value
	// never desynchronizes the block boundaries these passes rely on.
	// Escape sequences (e.g. `\7d`) are skipped as opaque tokens during brace
	// counting - never decoded first - so an escaped brace can never act as
	// a structural delimiter. Selector/block text is decoded on the fly ONLY
	// for pattern matching inside these passes; the decoded copy never
	// replaces the text used for further boundary detection.
	css = stripRulesWithSensitiveSelectors(css, &warnings)
	css = stripFullScreenOverlays(css, &warnings)
	css = stripPointerEventsOnInteractive(css, &warnings)

	// Normalize CSS escape sequences now that block structure has already
	// been determined (M2). CSS escape sequences (e.g. `\3a ` → `:`, `\65 ` →
	// `e`) can be used to bypass literal-string regex matching. By decoding
	// escapes here, all remaining (flat, non-brace-aware) detection regexes
	// run against the semantically equivalent plain form. The sanitized
	// (decoded) string is what we return; it is functionally identical to
	// the original as interpreted by a CSS parser.
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

	if matches := reURLExternalUnterminated.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed unterminated external url: "+strings.TrimSpace(m))
		}
		css = reURLExternalUnterminated.ReplaceAllString(css, "none")
	}

	if matches := reImageSetExternal.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "removed external image-set() reference: "+strings.TrimSpace(m))
		}
		css = reImageSetExternal.ReplaceAllString(css, "none")
	}

	if matches := reCustomPropExternalURL.FindAllString(css, -1); len(matches) > 0 {
		for _, m := range matches {
			warnings = append(warnings, "neutralized external URL in custom property: "+strings.TrimSpace(m))
		}
		css = reCustomPropExternalURL.ReplaceAllString(css, "$1: initial")
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

	// Clamp z-index > 999 to 999.
	css = clampZIndex(css, &warnings)

	return []byte(css), warnings
}

// stripRulesWithSensitiveSelectors removes entire CSS rule blocks that target
// sensitive element selectors (password inputs, .admin-*, .token-*, .secret-*).
// Operates on raw, still-escaped CSS: the selector is decoded on the fly for
// matching only (see SanitizeCSS for why decoding happens here, not upstream).
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
		closeIdx := findMatchingBrace(remaining, braceOpen)
		if closeIdx < 0 {
			// Malformed/unbalanced block: we can no longer trust where this
			// or any later block ends, so fail closed and drop the rest
			// rather than recopy unchecked text that might hide a rule.
			*warnings = append(*warnings, "dropped malformed/unbalanced CSS block after: "+strings.TrimSpace(selector))
			break
		}
		if reSensitiveSelector.MatchString(decodeCSSEscapes(selector)) {
			*warnings = append(*warnings, "removed rule targeting sensitive selector: "+strings.TrimSpace(selector))
		} else {
			result.WriteString(remaining[:closeIdx+1])
		}
		remaining = remaining[closeIdx+1:]
	}
	return result.String()
}

// findMatchingBrace returns the index of the } that closes the { at openIdx,
// accounting for nested braces. It is aware of CSS string literals ('...'
// and "..."), comments (/* ... */), url(...) values, and escape sequences,
// none of which may contribute a structural { or } - a byte-for-byte count
// is fooled by any of these (e.g. content:"}" or content:"{" desyncs a naive
// counter, letting the attacker's real payload run past uninspected).
// Returns -1 if the block is truncated or never closes.
func findMatchingBrace(s string, openIdx int) int {
	depth := 0
	n := len(s)
	i := openIdx
	for i < n {
		switch c := s[i]; c {
		case '\\':
			i = skipCSSEscape(s, i)
		case '"', '\'':
			end := skipCSSString(s, i)
			if end < 0 {
				return -1
			}
			i = end
		case '/':
			if i+1 < n && s[i+1] == '*' {
				end := strings.Index(s[i+2:], "*/")
				if end < 0 {
					return -1
				}
				i = i + 2 + end + 2
			} else {
				i++
			}
		case 'u', 'U':
			if i+4 <= n && strings.EqualFold(s[i:i+4], "url(") {
				end, ok := skipBareURL(s, i)
				if !ok {
					return -1
				}
				i = end
			} else {
				i++
			}
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
			i++
		default:
			i++
		}
	}
	return -1
}

// skipCSSEscape returns the index immediately after the CSS escape sequence
// starting at s[i] (s[i] must be '\\'). Per CSS Syntax Level 3 §4.3.7, an
// escape is 1-6 hex digits with an optional single trailing whitespace, or
// any other single character. The escaped code point is consumed as part of
// the surrounding token; it is never re-interpreted as a structural
// character, so `\7d` can never act as a literal closing brace during brace
// counting - regardless of what character it decodes to.
func skipCSSEscape(s string, i int) int {
	n := len(s)
	if i+1 >= n {
		return i + 1
	}
	j := i + 1
	hexEnd := j
	for hexEnd < n && hexEnd-j < 6 && isHexDigit(s[hexEnd]) {
		hexEnd++
	}
	if hexEnd > j {
		if hexEnd < n && (s[hexEnd] == ' ' || s[hexEnd] == '\t' || s[hexEnd] == '\n' || s[hexEnd] == '\r' || s[hexEnd] == '\f') {
			hexEnd++
		}
		return hexEnd
	}
	// Simple escape: backslash + one literal character.
	return i + 2
}

// skipCSSString returns the index of the character immediately after the
// closing quote of a string starting at s[i] (a ' or "), or -1 if the string
// is never closed before EOF. An escaped quote inside the string does not
// terminate it.
func skipCSSString(s string, i int) int {
	quote := s[i]
	n := len(s)
	j := i + 1
	for j < n {
		switch s[j] {
		case '\\':
			j = skipCSSEscape(s, j)
		case quote:
			return j + 1
		default:
			j++
		}
	}
	return -1
}

// skipBareURL returns the index immediately after the closing ) of an
// unquoted url(...) value starting at the 'u'/'U' of "url(" at s[i], or
// false if it is never closed before EOF. Content inside is opaque: a brace
// found there is not structural CSS. A quoted url(...) is left for the
// normal string handling in findMatchingBrace to consume.
func skipBareURL(s string, i int) (int, bool) {
	n := len(s)
	j := i + 4 // len("url(")
	for j < n && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r' || s[j] == '\f') {
		j++
	}
	if j < n && (s[j] == '"' || s[j] == '\'') {
		return i + 4, true
	}
	for j < n {
		switch s[j] {
		case '\\':
			j = skipCSSEscape(s, j)
		case ')':
			return j + 1, true
		default:
			j++
		}
	}
	return -1, false
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
			// Malformed/unbalanced block: fail closed, drop the rest.
			*warnings = append(*warnings, "dropped malformed/unbalanced CSS block")
			break
		}
		block := remaining[braceOpen : closeIdx+1]
		selector := remaining[:braceOpen]

		if isFullScreenOverlay(decodeCSSEscapes(block)) {
			*warnings = append(*warnings, "removed full-screen overlay rule: "+strings.TrimSpace(selector))
		} else {
			result.WriteString(remaining[:closeIdx+1])
		}
		remaining = remaining[closeIdx+1:]
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
			// Malformed/unbalanced block: fail closed, drop the rest.
			*warnings = append(*warnings, "dropped malformed/unbalanced CSS block")
			break
		}
		selector := remaining[:braceOpen]
		block := remaining[braceOpen : closeIdx+1]

		if reInteractiveSelector.MatchString(decodeCSSEscapes(selector)) && rePointerEventsNone.MatchString(decodeCSSEscapes(block)) {
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
