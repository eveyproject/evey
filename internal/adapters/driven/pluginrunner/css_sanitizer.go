package pluginrunner

import (
	"regexp"
	"strings"
)

var (
	reImport        = regexp.MustCompile(`(?i)@import\s+[^;]+;?`)
	reURLExternal   = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*(https?://|//)[^)]*\)`)
	reURLJavascript = regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*javascript\s*:[^)]*\)`)
	reExpression    = regexp.MustCompile(`(?i)expression\s*\([^)]*\)`)
	reMozBinding    = regexp.MustCompile(`(?i)-moz-binding\s*:[^;}"]+[;}]?`)
	reBehavior      = regexp.MustCompile(`(?i)\bbehavior\s*:[^;}"]+[;}]?`)
)

// SanitizeCSS removes dangerous CSS constructs that could be used for
// data exfiltration or code execution. Returns sanitized CSS and warnings.
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

	return []byte(css), warnings
}
