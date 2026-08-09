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

	// reImageSetStart locates the opening `(` of an image-set()/-webkit-image-set()
	// call. image-set() accepts any number of comma-separated candidates
	// (each a quoted string or a url()), and is valid in any image-valued
	// property (list-style-image, cursor, border-image-source,
	// -webkit-mask-image, background(-image), ...). The property name is
	// irrelevant here since the fetch happens inside the function call.
	reImageSetStart = regexp.MustCompile(`(?i)(?:-webkit-)?image-set\s*\(`)

	// reImageSetCandidateExternal matches a quoted (or bare protocol-relative)
	// external candidate ANYWHERE inside an already paren-bounded image-set(...)
	// span - not just the first argument. A flat regex anchored at the open
	// paren only ever catches the first candidate; later comma-separated
	// candidates need no url() token, so they must be found by scanning the
	// whole bounded span instead.
	reImageSetCandidateExternal = regexp.MustCompile(`(?i)["'](?:https?://|//)`)

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

	// reNestableAtRule matches "grouping" at-rules whose block contains nested
	// rule blocks (@media, @supports, @layer, @container, @scope, the obsolete
	// @document), as opposed to: statement at-rules that end in `;` with no
	// block at all (@import, @charset, @namespace - these never reach the
	// brace-boundary scanners below), and declaration/percentage-selector
	// at-rules (@keyframes, @font-face, @page) which this pass deliberately
	// does not recurse into - their content is not a selector to re-check.
	reNestableAtRule = regexp.MustCompile(`(?i)^\s*@(?:-\w+-)?(?:media|supports|layer|container|scope|document)\b`)
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

	css = stripImageSetExternalCandidates(css, &warnings)

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
		switch {
		case reNestableAtRule.MatchString(decodeCSSEscapes(selector)):
			// A sensitive rule can be nested inside a grouping at-rule
			// (@media/@supports/@layer/...); recurse into the block's
			// content so it is still caught, while non-offending sibling
			// rules inside the same at-rule are preserved.
			inner := remaining[braceOpen+1 : closeIdx]
			recursed := stripRulesWithSensitiveSelectors(inner, warnings)
			result.WriteString(selector)
			result.WriteString("{")
			result.WriteString(recursed)
			result.WriteString("}")
		case reSensitiveSelector.MatchString(decodeCSSEscapes(selector)):
			*warnings = append(*warnings, "removed rule targeting sensitive selector: "+strings.TrimSpace(selector))
		default:
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

// findMatchingParen returns the index of the ')' that closes the '(' at openIdx,
// skipping over string literals and escape sequences so a quoted ')' (e.g. inside
// a candidate string) can never desync the depth count. Returns -1 if the call is
// truncated or never closes.
func findMatchingParen(s string, openIdx int) int {
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
		case '(':
			depth++
			i++
		case ')':
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

// stripImageSetExternalCandidates neutralizes an image-set()/-webkit-image-set()
// call as soon as ANY of its comma-separated candidates - first or later - is an
// external (http(s):// or protocol-relative) string. A flat regex anchored right
// after the opening paren only ever sees the first candidate; a second candidate
// like `image-set("a.png" 1x, "https://evil/x" 2x)` has no url() token and no
// fixed offset from `(` for a naive pattern to anchor on, so the call must be
// bounded with paren-matching (findMatchingParen) and its whole interior scanned.
func stripImageSetExternalCandidates(css string, warnings *[]string) string {
	locs := reImageSetStart.FindAllStringIndex(css, -1)
	if len(locs) == 0 {
		return css
	}
	var result strings.Builder
	last := 0
	for _, loc := range locs {
		start, openParen := loc[0], loc[1]-1
		if start < last {
			// Falls inside a span already neutralized by a previous iteration.
			continue
		}
		closeParen := findMatchingParen(css, openParen)
		if closeParen < 0 {
			// Unterminated call: fail closed rather than leave a partially
			// inspected image-set() in the output.
			result.WriteString(css[last:start])
			result.WriteString("none")
			*warnings = append(*warnings, "removed unterminated image-set() call")
			last = len(css)
			continue
		}
		inner := css[openParen+1 : closeParen]
		if reImageSetCandidateExternal.MatchString(inner) {
			result.WriteString(css[last:start])
			result.WriteString("none")
			*warnings = append(*warnings, "removed external image-set() candidate: "+strings.TrimSpace(css[start:closeParen+1]))
			last = closeParen + 1
		}
	}
	result.WriteString(css[last:])
	return result.String()
}

// stripFullScreenOverlays removes CSS rule blocks that combine position:fixed|sticky
// with full-viewport geometry (top:0, left:0, width:100vw or height:100vh).
// This prevents themes from rendering full-screen phishing overlays.
//
// Per SELECTOR, not per block: the browser cascades `.ov{position:fixed}` and
// `.ov{top:0;width:100vw;height:100vh}` into one overlay, so a per-block check
// passed both and splitting the declarations bypassed the control in one line.
// The reverse too - an at-rule was judged as ONE block, so `@media{.topbar
// {position:fixed}.hero{width:100vw}}` was deleted whole; at-rules now recurse.
//
// Keyed on normalized selector TEXT, so `.ov` + `div.ov` still passes - deciding
// that needs a real selector engine. "Per selector" is the guarantee, no more.
func stripFullScreenOverlays(css string, warnings *[]string) string {
	return stripFullScreenOverlaysScoped(css, nil, warnings)
}

// stripFullScreenOverlaysScoped is stripFullScreenOverlays for one scope.
// inherited carries the signals the ENCLOSING scope already gave each selector:
// a rule inside `@media` cascades on top of the same selector's root-level
// rule, so the pair must be judged together. Inheritance is one-directional on
// purpose - a root rule is NOT condemned by what an at-rule adds, because that
// overlay only exists while the query matches, and the inner rule is what
// creates it.
func stripFullScreenOverlaysScoped(css string, inherited map[string]*overlaySignals, warnings *[]string) string {
	rules, trailing, ok := splitScopeRules(css)

	// Pass 1: accumulate per selector.
	acc := make(map[string]*overlaySignals, len(rules)+len(inherited))
	for k, v := range inherited {
		cp := *v
		acc[k] = &cp
	}
	for _, ru := range rules {
		if ru.atRule {
			continue
		}
		for _, key := range selectorKeys(ru.selector) {
			sig := acc[key]
			if sig == nil {
				sig = &overlaySignals{}
				acc[key] = sig
			}
			sig.add(decodeCSSEscapes(ru.inner))
		}
	}

	// Pass 2: rewrite.
	var result strings.Builder
	for _, ru := range rules {
		if ru.atRule {
			result.WriteString(ru.selector)
			result.WriteString("{")
			result.WriteString(stripFullScreenOverlaysScoped(ru.inner, acc, warnings))
			result.WriteString("}")
			continue
		}
		overlay, combined := false, false
		for _, key := range selectorKeys(ru.selector) {
			if sig := acc[key]; sig != nil && sig.isOverlay() {
				overlay = true
				break
			}
		}
		if overlay {
			var own overlaySignals
			own.add(decodeCSSEscapes(ru.inner))
			combined = !own.isOverlay()
		}
		switch {
		case overlay && combined:
			*warnings = append(*warnings, "removed full-screen overlay rule: "+
				strings.TrimSpace(ru.selector)+" (combined with sibling rules for the same selector)")
		case overlay:
			*warnings = append(*warnings, "removed full-screen overlay rule: "+strings.TrimSpace(ru.selector))
		default:
			result.WriteString(ru.text)
		}
	}
	if !ok {
		// Malformed/unbalanced block: fail closed, drop the rest.
		*warnings = append(*warnings, "dropped malformed/unbalanced CSS block")
		return result.String()
	}
	result.WriteString(trailing)
	return result.String()
}

// cssRule is one selector+block pair of a scope. text is the rule exactly as it
// appeared (everything since the previous block's `}`, so leading whitespace and
// comments survive verbatim); inner is the block content without its braces.
type cssRule struct {
	text     string
	selector string
	inner    string
	atRule   bool
}

// splitScopeRules splits one scope into its rule blocks, in order. ok is false
// when a block never closes: the rules parsed so far are returned so the caller
// can keep them and fail closed on the rest, exactly as the single-pass
// scanners did.
func splitScopeRules(css string) (rules []cssRule, trailing string, ok bool) {
	remaining := css
	for {
		braceOpen := strings.Index(remaining, "{")
		if braceOpen < 0 {
			return rules, remaining, true
		}
		closeIdx := findMatchingBrace(remaining, braceOpen)
		if closeIdx < 0 {
			return rules, "", false
		}
		selector := remaining[:braceOpen]
		rules = append(rules, cssRule{
			text:     remaining[:closeIdx+1],
			selector: selector,
			inner:    remaining[braceOpen+1 : closeIdx],
			atRule:   reNestableAtRule.MatchString(decodeCSSEscapes(selector)),
		})
		remaining = remaining[closeIdx+1:]
	}
}

// selectorKeys normalizes a rule's selector text into one key per selector in
// the list, so that two rules the browser will cascade together accumulate
// together. Comments are stripped and whitespace runs collapsed (otherwise
// `.ov` and `/*x*/ .ov` would be different keys - a free bypass); case is
// preserved because CSS class and id selectors are case-sensitive. The split is
// on top-level commas only, so a comma inside :is(...) or [attr=","] does not
// fragment the key and make two unrelated selectors share one.
func selectorKeys(selector string) []string {
	s := stripCSSComments(decodeCSSEscapes(selector))
	var keys []string
	depth := 0
	start := 0
	for i := range len(s) {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				keys = appendSelectorKey(keys, s[start:i])
				start = i + 1
			}
		}
	}
	return appendSelectorKey(keys, s[start:])
}

func appendSelectorKey(keys []string, raw string) []string {
	if k := strings.Join(strings.Fields(raw), " "); k != "" {
		return append(keys, k)
	}
	return keys
}

// stripCSSComments removes /* ... */ spans. An unterminated comment swallows
// the rest, which is what a CSS parser does too.
func stripCSSComments(s string) string {
	if !strings.Contains(s, "/*") {
		return s
	}
	var b strings.Builder
	for {
		start := strings.Index(s, "/*")
		if start < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:start])
		end := strings.Index(s[start+2:], "*/")
		if end < 0 {
			return b.String()
		}
		s = s[start+2+end+2:]
	}
}

// overlaySignals is the union of full-screen-overlay signals a selector has
// received. Accumulating them (rather than testing one block) is what makes the
// control immune to splitting the declarations across rules.
type overlaySignals struct {
	posFixed bool
	inset    bool
	top      bool
	right    bool
	bottom   bool
	left     bool
	width    bool
	height   bool
}

// add folds one block's declarations into the set. The block must already be
// escape-decoded.
func (s *overlaySignals) add(block string) {
	s.posFixed = s.posFixed || rePosFixed.MatchString(block)
	s.inset = s.inset || reFullInset.MatchString(block)
	s.top = s.top || reFullTop.MatchString(block)
	s.right = s.right || reFullRight.MatchString(block)
	s.bottom = s.bottom || reFullBottom.MatchString(block)
	s.left = s.left || reFullLeft.MatchString(block)
	s.width = s.width || reFullWidth.MatchString(block)
	s.height = s.height || reFullHeight.MatchString(block)
}

// isOverlay is the policy: position:fixed|sticky plus at least two of
// top/right/bottom/left/width:100vw/height:100vh, one of which must be a
// viewport DIMENSION - a fixed header pinned with top:0;left:0 but sized by
// content is legitimate and must not be flagged. inset:0 and all four sides at
// zero are each equivalent to a full-bleed box on their own.
func (s *overlaySignals) isOverlay() bool {
	if !s.posFixed {
		return false
	}
	if s.inset {
		return true
	}
	if s.top && s.right && s.bottom && s.left {
		return true
	}
	count := 0
	for _, on := range []bool{s.top, s.right, s.bottom, s.left, s.width, s.height} {
		if on {
			count++
		}
	}
	return count >= 2 && (s.width || s.height)
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

		switch {
		case reNestableAtRule.MatchString(decodeCSSEscapes(selector)):
			// pointer-events:none on an interactive element can be nested
			// inside a grouping at-rule (@media/@supports/@layer/...);
			// recurse into the block's content so it is still caught, while
			// non-offending sibling rules inside the same at-rule are kept.
			inner := remaining[braceOpen+1 : closeIdx]
			recursed := stripPointerEventsOnInteractive(inner, warnings)
			result.WriteString(selector)
			result.WriteString("{")
			result.WriteString(recursed)
			result.WriteString("}")
		case reInteractiveSelector.MatchString(decodeCSSEscapes(selector)) && rePointerEventsNone.MatchString(decodeCSSEscapes(block)):
			// Remove only the pointer-events:none declaration, keep rest of block.
			cleaned := rePointerEventsNone.ReplaceAllString(block, "/* pointer-events removed */")
			*warnings = append(*warnings, "removed pointer-events:none from interactive selector: "+strings.TrimSpace(selector))
			result.WriteString(selector)
			result.WriteString(cleaned)
		default:
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
