package themejs

import (
	"bytes"
	"fmt"
)

// blockedPatterns lists byte sequences whose presence in theme JS source is
// considered a critical security violation. The scanner is intentionally
// conservative: simple string matching catches obvious data-exfiltration and
// host-escape patterns without requiring a full AST parse. False positives
// (e.g. a comment mentioning "fetch(") are acceptable at install time — themes
// must use only the evey.theme.* API surface.
var blockedPatterns = [][]byte{
	[]byte("document.cookie"),
	[]byte("localStorage"),
	[]byte("sessionStorage"),
	[]byte("XMLHttpRequest"),
	[]byte("fetch("),
	[]byte("navigator.sendBeacon"),
	[]byte("eval("),
	[]byte("Function("),
	[]byte("import("),
	[]byte("atob("),
	[]byte("top."),
	[]byte("parent."),
	[]byte("window.parent"),
	[]byte("document.write"),
	[]byte("Element.prototype"),
}

// Scan checks theme.js source for forbidden patterns.
// Returns a non-nil error describing the first critical violation found.
// Warnings (non-blocking) are returned in the second slice.
func Scan(src []byte) (err error, warnings []string) {
	for _, pat := range blockedPatterns {
		if bytes.Contains(src, pat) {
			return fmt.Errorf("forbidden pattern %q found in theme JS", string(pat)), nil
		}
	}
	return nil, nil
}
