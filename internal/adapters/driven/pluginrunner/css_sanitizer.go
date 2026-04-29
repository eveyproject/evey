package pluginrunner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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
	rePosFixed  = regexp.MustCompile(`(?i)\bposition\s*:\s*(fixed|sticky)\b`)
	reFullWidth = regexp.MustCompile(`(?i)\bwidth\s*:\s*100vw\b`)
	reFullHeight = regexp.MustCompile(`(?i)\bheight\s*:\s*100vh\b`)
	reFullTop    = regexp.MustCompile(`(?i)\btop\s*:\s*0\b`)
	reFullLeft   = regexp.MustCompile(`(?i)\bleft\s*:\s*0\b`)

	// pointer-events:none on interactive elements.
	rePointerEventsNone      = regexp.MustCompile(`(?i)\bpointer-events\s*:\s*none\b`)
	reInteractiveSelector    = regexp.MustCompile(`(?i)\bbutton\b|\[role\s*=\s*["']?button["']?\]|a\[href[^]]*logout[^]]*\]|\[data-critical\]`)

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
// A regex-based rule block scanner is used; unicode-escape bypasses are a known limitation.
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
// combined with at least two of: top:0, left:0, width:100vw, height:100vh.
func isFullScreenOverlay(block string) bool {
	if !rePosFixed.MatchString(block) {
		return false
	}
	count := 0
	if reFullTop.MatchString(block) {
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
