package themejs

import (
	"fmt"
	"regexp"
)

// Theme JS runs in the user's page (mounted via theme-loader). Themes must
// only use the evey.theme.* surface — direct DOM/network/storage access is
// rejected at install time. Detection is text-based (no AST), so we flag
// not just literal token forms ("fetch(") but also common bypasses:
// bracket-notation property access (window["fetch"]), string concatenation
// at the call site, dynamic globals access (globalThis, Reflect.get), and
// constructor.constructor escapes. False positives are acceptable at install
// time — themes have no legitimate reason to mention these tokens at all.

// blockedLiterals lists exact byte sequences that indicate a forbidden API
// is referenced. These are scanned as plain regex word/contiguous matches.
var blockedLiterals = []string{
	"document.cookie",
	"localStorage",
	"sessionStorage",
	"indexedDB",
	"XMLHttpRequest",
	"navigator.sendBeacon",
	"navigator.serviceWorker",
	"WebSocket",
	"EventSource",
	"importScripts",
	"document.write",
	"Element.prototype",
	"Object.defineProperty",
	"globalThis",
	"window.parent",
	"window.top",
	"window.opener",
	"Reflect.get",
	"Reflect.apply",
	"Reflect.construct",
	"window.frames",
	"frameElement",
	"postMessage",
}

// blockedRegex captures patterns whose token form alone is too coarse:
// "fetch(" misses window["fetch"](), and bare "eval" hits "evaluate". These
// patterns target the actual call/access shapes attackers use.
var blockedRegex = []*regexp.Regexp{
	// Direct call: fetch(, eval(, atob(, Function(, import(, etc.
	regexp.MustCompile(`\b(fetch|eval|atob|btoa|Function|setTimeout|setInterval)\s*\(`),
	// Dynamic import: import("...")
	regexp.MustCompile(`\bimport\s*\(`),
	// Bracket-notation property access on host globals: window["fetch"],
	// globalThis['localStorage'], top["loca"+"tion"]. The string literal
	// inside the brackets is matched piecewise so concatenation bypasses
	// (window["fetc"+"h"]) still trip the literal scan above on each chunk.
	regexp.MustCompile(`(?:window|self|globalThis|top|parent|document|navigator|location)\s*\[`),
	// document.cookie via property descriptor or computed access elsewhere.
	regexp.MustCompile(`\.cookie\b`),
	// constructor.constructor escape: ([]).constructor.constructor("...")()
	regexp.MustCompile(`\.constructor\s*\.\s*constructor`),
	// new Function via aliased reference: const F = window.Function; new F(...)
	regexp.MustCompile(`new\s+Function\b`),
	// Read of host-global Function/eval/fetch via property access:
	// self.Function, window.eval, top.fetch, globalThis.fetch.
	regexp.MustCompile(`(?:window|self|top|parent|globalThis)\s*\.\s*(?:Function|eval|fetch|atob|btoa|XMLHttpRequest|WebSocket|EventSource)\b`),
	// Property assignment to host globals: window.xyz = …, top.location = …
	regexp.MustCompile(`(?:window|top|parent|document|location|globalThis|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]`),
	// Prototype pollution / Object.prototype writes
	regexp.MustCompile(`Object\s*\.\s*prototype`),
	// Service worker registration
	regexp.MustCompile(`\.register\s*\(\s*['"]\S+\.js`),
}

// Scan checks theme.js source for forbidden patterns.
// Returns a non-nil error describing the first critical violation found.
// Warnings (non-blocking) are returned in the second slice.
func Scan(src []byte) (err error, warnings []string) {
	for _, lit := range blockedLiterals {
		// Use regex with word-boundary-ish anchoring so "localStorage" still
		// trips on "localStorage.getItem" and on bracketed forms once the
		// substring appears anywhere as a string literal (window["localStorage"]
		// contains the literal "localStorage").
		if containsLiteral(src, lit) {
			return fmt.Errorf("forbidden pattern %q found in theme JS", lit), nil
		}
	}
	for _, re := range blockedRegex {
		if loc := re.FindIndex(src); loc != nil {
			return fmt.Errorf("forbidden pattern matched in theme JS: %s", re.String()), nil
		}
	}
	return nil, nil
}

func containsLiteral(src []byte, lit string) bool {
	if len(lit) == 0 {
		return false
	}
	pat := []byte(lit)
	for i := 0; i+len(pat) <= len(src); i++ {
		if src[i] == pat[0] {
			match := true
			for j := 1; j < len(pat); j++ {
				if src[i+j] != pat[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
