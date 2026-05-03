package pluginrunner

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// maxFoldPasses limits the number of string-concat folding iterations to
// prevent pathological inputs from hanging the analyzer.
const maxFoldPasses = 10

// stringConcatRe matches two adjacent string literals joined by + or . (PHP)
// with optional whitespace, e.g. "foo" + "bar", 'foo'+"bar", or "sys"."tem".
// Both single- and double-quoted literals are handled; inner escapes are not
// interpreted here (folding is purely textual).
var stringConcatRe = regexp.MustCompile(`("(?:[^"\\]|\\.)*")\s*[+.]\s*("(?:[^"\\]|\\.)*")|('(?:[^'\\]|\\.)*')\s*[+.]\s*('(?:[^'\\]|\\.)*')`)

// foldStringConcat repeatedly folds adjacent string literals joined by +
// until the string stops changing or maxFoldPasses is reached.
// It handles both single- and double-quoted strings.
func foldStringConcat(src string) string {
	for i := 0; i < maxFoldPasses; i++ {
		next := stringConcatRe.ReplaceAllStringFunc(src, func(m string) string {
			// m is e.g. "foo" + "bar" or 'foo'+'bar'
			// Strip outer quotes of both operands and merge.
			parts := stringConcatRe.FindStringSubmatch(m)
			// parts[1]+parts[2] for double-quoted, parts[3]+parts[4] for single-quoted.
			if parts[1] != "" && parts[2] != "" {
				inner1 := parts[1][1 : len(parts[1])-1]
				inner2 := parts[2][1 : len(parts[2])-1]
				return `"` + inner1 + inner2 + `"`
			}
			if parts[3] != "" && parts[4] != "" {
				inner1 := parts[3][1 : len(parts[3])-1]
				inner2 := parts[4][1 : len(parts[4])-1]
				return `'` + inner1 + inner2 + `'`
			}
			return m
		})
		if next == src {
			break
		}
		src = next
	}
	return src
}

// hexEscapeRe matches hex escapes like \xNN inside a string context.
var hexEscapeRe = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

// octEscapeRe matches octal escapes like \NNN (up to 3 digits) inside a string context.
var octEscapeRe = regexp.MustCompile(`\\([0-7]{1,3})`)

// unicodeEscapeRe matches unicode escapes like \uNNNN or \UNNNNNNNN.
var unicodeEscapeRe = regexp.MustCompile(`\\u([0-9a-fA-F]{4})|\\U([0-9a-fA-F]{8})`)

// decodeEscapesInStrings decodes hex, octal, and unicode escapes within
// string literals (single or double quoted) so that subsequent regex
// matching sees the plain-text equivalents.
func decodeEscapesInStrings(src string) string {
	return mapStringLiterals(src, func(inner string) string {
		// Decode \xNN hex escapes.
		inner = hexEscapeRe.ReplaceAllStringFunc(inner, func(m string) string {
			sub := hexEscapeRe.FindStringSubmatch(m)
			b, err := strconv.ParseUint(sub[1], 16, 8)
			if err != nil {
				return m
			}
			return string([]byte{byte(b)})
		})
		// Decode \NNN octal escapes.
		inner = octEscapeRe.ReplaceAllStringFunc(inner, func(m string) string {
			sub := octEscapeRe.FindStringSubmatch(m)
			b, err := strconv.ParseUint(sub[1], 8, 9)
			if err != nil || b > 0xff {
				return m
			}
			return string([]byte{byte(b)})
		})
		// Decode \uNNNN / \UNNNNNNNN unicode escapes.
		inner = unicodeEscapeRe.ReplaceAllStringFunc(inner, func(m string) string {
			sub := unicodeEscapeRe.FindStringSubmatch(m)
			hex := sub[1]
			if hex == "" {
				hex = sub[2]
			}
			cp, err := strconv.ParseInt(hex, 16, 32)
			if err != nil {
				return m
			}
			return string(rune(cp))
		})
		return inner
	})
}

// mapStringLiterals applies fn to the inner content of every string literal
// (single- or double-quoted) in src, reconstructing the surrounding quotes.
func mapStringLiterals(src string, fn func(string) string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		q := src[i]
		if q != '"' && q != '\'' {
			b.WriteByte(src[i])
			i++
			continue
		}
		// Collect the quoted string.
		j := i + 1
		for j < len(src) {
			if src[j] == '\\' {
				j += 2
				continue
			}
			if src[j] == q {
				break
			}
			j++
		}
		if j >= len(src) {
			// Unterminated string — emit as-is.
			b.WriteString(src[i:])
			break
		}
		inner := src[i+1 : j]
		b.WriteByte(q)
		b.WriteString(fn(inner))
		b.WriteByte(q)
		i = j + 1
	}
	return b.String()
}

// base64StringRe matches base64-looking payloads that are long enough to be
// meaningful (≥ 16 chars) and correctly padded.
var base64StringRe = regexp.MustCompile(`[A-Za-z0-9+/]{16,}={0,2}`)

// dangerousPayloadRe matches dangerous patterns inside a decoded base64 string.
var dangerousPayloadRe = regexp.MustCompile(`(?i)(os\.system|exec\(|eval\(|subprocess|pip\s+install|child_process)`)

// scanBase64Strings scans src for base64-looking literals, attempts to decode
// them, and returns a slice of AnalysisFindings for any that decode to
// dangerous content.
func scanBase64Strings(src, relPath string, lineNum int) []AnalysisFinding {
	var findings []AnalysisFinding
	for _, candidate := range base64StringRe.FindAllString(src, -1) {
		decoded, err := base64.StdEncoding.DecodeString(candidate)
		if err != nil {
			// Also try without padding in case trailing = was stripped.
			decoded, err = base64.RawStdEncoding.DecodeString(candidate)
			if err != nil {
				continue
			}
		}
		decodedStr := string(decoded)
		if dangerousPayloadRe.MatchString(decodedStr) {
			findings = append(findings, AnalysisFinding{
				Severity:    SeverityCritical,
				Category:    "obfuscation_base64_decoded",
				File:        relPath,
				Line:        lineNum,
				Pattern:     candidate,
				Description: "base64-decoded payload contains dangerous code pattern",
			})
		}
	}
	return findings
}

// unicodeConfusables maps common lookalike code-point runes to their ASCII
// equivalents. Limited to the ~20 most-commonly abused characters from
// Cyrillic and Greek that overlap visually with Latin letters.
var unicodeConfusables = map[rune]rune{
	// Cyrillic
	'а': 'a', // U+0430
	'е': 'e', // U+0435
	'о': 'o', // U+043E
	'р': 'p', // U+0440
	'с': 'c', // U+0441
	'х': 'x', // U+0445
	'у': 'y', // U+0443
	'і': 'i', // U+0456
	'ѕ': 's', // U+0455
	'ј': 'j', // U+0458
	'ℬ': 'B', // U+212C
	// Greek
	'α': 'a',  // U+03B1
	'β': 'b',  // U+03B2 (close enough)
	'ε': 'e',  // U+03B5
	'ο': 'o',  // U+03BF
	'ρ': 'p',  // U+03C1
	'υ': 'u',  // U+03C5
	'ν': 'v',  // U+03BD
	'κ': 'k',  // U+03BA
	'Α': 'A',  // U+0391
	'Β': 'B',  // U+0392
	'Ε': 'E',  // U+0395
	'Η': 'H',  // U+0397
	'Ι': 'I',  // U+0399
	'Κ': 'K',  // U+039A
	'Μ': 'M',  // U+039C
	'Ν': 'N',  // U+039D
	'Ο': 'O',  // U+039F
	'Ρ': 'R',  // U+03A1
	'Τ': 'T',  // U+03A4
	'Υ': 'Y',  // U+03A5
	'Χ': 'X',  // U+03A7
	// Fullwidth Latin
	'Ａ': 'A', 'Ｂ': 'B', 'Ｃ': 'C', 'Ｄ': 'D', 'Ｅ': 'E',
	'Ｆ': 'F', 'Ｇ': 'G', 'Ｈ': 'H', 'Ｉ': 'I', 'Ｊ': 'J',
	'Ｋ': 'K', 'Ｌ': 'L', 'Ｍ': 'M', 'Ｎ': 'N', 'Ｏ': 'O',
	'Ｐ': 'P', 'Ｑ': 'Q', 'Ｒ': 'R', 'Ｓ': 'S', 'Ｔ': 'T',
	'Ｕ': 'U', 'Ｖ': 'V', 'Ｗ': 'W', 'Ｘ': 'X', 'Ｙ': 'Y', 'Ｚ': 'Z',
	'ａ': 'a', 'ｂ': 'b', 'ｃ': 'c', 'ｄ': 'd', 'ｅ': 'e',
	'ｆ': 'f', 'ｇ': 'g', 'ｈ': 'h', 'ｉ': 'i', 'ｊ': 'j',
	'ｋ': 'k', 'ｌ': 'l', 'ｍ': 'm', 'ｎ': 'n', 'ｏ': 'o',
	'ｐ': 'p', 'ｑ': 'q', 'ｒ': 'r', 'ｓ': 's', 'ｔ': 't',
	'ｕ': 'u', 'ｖ': 'v', 'ｗ': 'w', 'ｘ': 'x', 'ｙ': 'y', 'ｚ': 'z',
}

// normalizeUnicode returns a version of src where Cyrillic/Greek/fullwidth
// confusable characters are replaced by their ASCII lookalike. The result
// is used only for pattern matching — the original source is preserved for
// display purposes.
func normalizeUnicode(src string) string {
	// Fast path: bail early if no non-ASCII rune is present.
	if isAllASCII(src) {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for _, r := range src {
		if r > unicode.MaxASCII {
			if ascii, ok := unicodeConfusables[r]; ok {
				b.WriteRune(ascii)
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isAllASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// fstringLiteralRe matches Python f-string segments where each interpolation
// is a single-quoted string literal: f"{'ev'}{'al'}" → "eval".
// Only handles the pattern f"({literal})+" — more complex expressions are left as-is.
var fstringLiteralRe = regexp.MustCompile(`f"((?:\{'[^']*'\})+)"`)

// foldFStrings folds Python f-string expressions containing only single-quoted
// string literal interpolations into a plain string.
// e.g. f"{'ev'}{'al'}" becomes "eval".
func foldFStrings(src string) string {
	return fstringLiteralRe.ReplaceAllStringFunc(src, func(m string) string {
		subs := fstringLiteralRe.FindStringSubmatch(m)
		if len(subs) < 2 {
			return m
		}
		inner := subs[1] // e.g. {'ev'}{'al'}
		var result strings.Builder
		for len(inner) > 0 {
			start := strings.Index(inner, "{'")
			if start == -1 {
				break
			}
			inner = inner[start+2:]
			end := strings.Index(inner, "'}")
			if end == -1 {
				break
			}
			result.WriteString(inner[:end])
			inner = inner[end+2:]
		}
		return `"` + result.String() + `"`
	})
}

// joinListRe matches Python "".join([...]) or ''.join([...]) where the list
// contains only string literals.
var joinListRe = regexp.MustCompile(`["']{2}\.join\s*\(\s*\[([^\]]+)\]\s*\)`)

// foldJoin folds Python "".join(["a","b","c"]) into "abc".
func foldJoin(src string) string {
	return joinListRe.ReplaceAllStringFunc(src, func(m string) string {
		subs := joinListRe.FindStringSubmatch(m)
		if len(subs) < 2 {
			return m
		}
		items := subs[1]
		var result strings.Builder
		i := 0
		for i < len(items) {
			if items[i] == ' ' || items[i] == '\t' || items[i] == ',' || items[i] == '\n' || items[i] == '\r' {
				i++
				continue
			}
			q := items[i]
			if q != '"' && q != '\'' {
				// Non-literal element — can't fold safely.
				return m
			}
			i++
			j := i
			for j < len(items) {
				if items[j] == '\\' {
					j += 2
					continue
				}
				if items[j] == q {
					break
				}
				j++
			}
			if j >= len(items) {
				return m // unterminated
			}
			result.WriteString(items[i:j])
			i = j + 1
		}
		return `"` + result.String() + `"`
	})
}

// dangerousStringValues is the set of string values that, when assigned to a
// variable, warrant tracking so variable-callable patterns can be detected.
var dangerousStringValues = map[string]bool{
	"system": true, "exec": true, "passthru": true,
	"shell_exec": true, "popen": true, "proc_open": true,
	"eval": true,
}

// dangerousModuleAliases is the set of module names whose aliases should be
// tracked for method-call detection after chained assignment.
var dangerousModuleAliases = map[string]bool{
	"os": true, "subprocess": true, "builtins": true,
}

// importAliasRe matches Python import-as declarations:
//
//	import os as _o
//	import subprocess as sp
//	from os import system as s
var importAliasRe = regexp.MustCompile(`(?:from\s+\S+\s+)?import\s+(\S+)\s+as\s+([a-zA-Z_]\w*)`)

// varStringAssignRe matches simple PHP/Python string assignments after
// concat folding: $f = "system"  or  f = 'exec'
var varStringAssignRe = regexp.MustCompile(`^[\$]?([a-zA-Z_]\w*)\s*=\s*["']([a-zA-Z_][\w]*)["']\s*(?:;|$|#)`)

// varIdentAssignRe matches simple identifier = identifier assignments:
//
//	m = _o   (where _o was previously bound to os)
var varIdentAssignRe = regexp.MustCompile(`^([a-zA-Z_]\w*)\s*=\s*([a-zA-Z_]\w*)\s*$`)

// anyAssignRe detects any assignment to a bare variable name (PHP or Python).
// Used to detect reassignments that would invalidate a previous binding.
var anyAssignRe = regexp.MustCompile(`^[\$]?([a-zA-Z_]\w*)\s*=`)

// varConcatAssignRe matches multi-operand variable concatenation, both PHP (.)
// and JS/Python (+):
//
//	$f = $x . $y
//	$g = $a . $b . $c . $d
//	f = a + b
//
// The whole RHS (capture group 2) is split on the operator at substitution
// time. Both single and double quotes for already-literal fragments are kept
// so a mix like  $f = $a . "tem"  still works after $a is resolved.
var varConcatAssignRe = regexp.MustCompile(`^[\$]?([a-zA-Z_]\w*)\s*=\s*([\$]?[a-zA-Z_"'][^;]*?(?:\s*[.+]\s*[\$]?[a-zA-Z_"'][^;]*?)+)\s*(?:;|$|#)`)

// concatOperandRe matches a single operand inside a concat expression: either
// a quoted literal or a bare variable name (with optional $).
var concatOperandRe = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'|[\$]?([a-zA-Z_]\w*)`)

// constructorAssignRe matches stored .constructor references:
//
//	const F = (function(){}).constructor
//	let F = obj.constructor
//	var F = X.constructor;
//
// Captures the lhs name in group 1.
var constructorAssignRe = regexp.MustCompile(`(?:const|let|var)\s+([a-zA-Z_]\w*)\s*=\s*[^;]*\.constructor(?:\s*;|\s*$)`)

// resolveConcatOperands takes the RHS of a concat assignment and returns the
// concatenated literal value if every operand resolves to a string literal
// (either inline or via stringAlias). Returns the joined string and true on
// success; otherwise returns "", false.
func resolveConcatOperands(rhs string, stringAlias map[string]string) (string, bool) {
	// Split on . and + at the top level (no nesting expected for the patterns
	// we target — this is a regex-driven analyzer, not a full AST).
	// Replace . and + outside of quotes with a sentinel, then split.
	var b strings.Builder
	inS, inD := false, false
	for i := 0; i < len(rhs); i++ {
		c := rhs[i]
		switch {
		case c == '\\' && i+1 < len(rhs):
			b.WriteByte(c)
			b.WriteByte(rhs[i+1])
			i++
			continue
		case c == '"' && !inS:
			inD = !inD
		case c == '\'' && !inD:
			inS = !inS
		case (c == '.' || c == '+') && !inS && !inD:
			b.WriteByte(0x1F)
			continue
		}
		b.WriteByte(c)
	}
	parts := strings.Split(b.String(), "\x1f")

	var out strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return "", false
		}
		if (strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`)) ||
			(strings.HasPrefix(p, `'`) && strings.HasSuffix(p, `'`)) {
			if len(p) < 2 {
				return "", false
			}
			out.WriteString(p[1 : len(p)-1])
			continue
		}
		// Strip leading $ for PHP variables.
		name := strings.TrimPrefix(p, "$")
		if !isValidIdentifier(name) {
			return "", false
		}
		v, ok := stringAlias[name]
		if !ok {
			return "", false
		}
		out.WriteString(v)
	}
	return out.String(), true
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if i == 0 && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
			return false
		}
		if i > 0 && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// maxAliasIterations caps the fixed-point loop for binding resolution. Cycles
// like  $a = $b; $b = $a  cannot extend the binding map past iteration 1, so
// 5 is a generous budget that still bounds CPU.
//
// LIMITATION: AST-based dynamic transforms (strrev("metsys"), base64.b64decode,
// eval-of-encoded-bytes, regex.replace, etc.) remain out of scope for the regex
// analyzer. The bwrap sandbox + cgroups (defense layers 6-7) are the actual
// enforcement boundary; the analyzer surfaces obvious cases at preview time.
const maxAliasIterations = 5

// resolveSimpleAliases performs a file-level pre-pass that:
//  1. Detects `import X as alias` for dangerous modules (os, subprocess).
//  2. Detects `var = alias` chained assignments (e.g. m = _o where _o → os).
//  3. Detects `$var = "dangerousName"` (PHP/Python) constant string assignments
//     after string-concat folding.
//  4. Detects multi-operand concatenation `$f = $x . $y . "literal"` and
//     resolves it to a literal via fixed-point iteration over the binding map.
//  5. Detects stored `.constructor` references (`const F = X.constructor`)
//     and rewrites later calls `F(...)` as `(X).constructor(...)` so the
//     existing constructor-call rule fires.
//  6. Substitutes alias usages so subsequent per-line rules can match them.
//
// Safety limits:
//   - A variable that is re-assigned to a different value is dropped from
//     the binding map.
//   - String concat folding is applied to each line before binding extraction
//     so `$f = "sys"."tem"` is seen as `$f = "system"`.
//   - Multi-step concat resolution iterates with maxAliasIterations cap.
//   - Object method chains beyond the documented patterns are not followed.
func resolveSimpleAliases(content string) string {
	lines := strings.Split(content, "\n")

	// moduleAlias maps alias → canonical module name ("os", "subprocess", …).
	moduleAlias := map[string]string{}
	// stringAlias maps varname → dangerous string value (after fixed-point).
	// Used for pass 2 substitution: $f → "system".
	stringAlias := map[string]string{}
	// allStringAlias maps varname → ANY string value (including innocent
	// fragments like "sys", "tem"). Used by resolveConcatOperands so that
	// a multi-step concat $f = $x . $y can be resolved when $x and $y are
	// individually innocent fragments. The dangerousness check happens on
	// the final concatenated value, not on the fragments.
	allStringAlias := map[string]string{}
	// constructorAlias maps varname → true when bound to an `X.constructor`
	// reference (the actual prefix is irrelevant; we only need to know the
	// lhs is a Function constructor handle).
	constructorAlias := map[string]bool{}
	// firstSeen tracks whether we have already recorded a binding for a var.
	firstSeen := map[string]bool{}
	// reassigned tracks vars that were assigned to a different RHS later in
	// the source — invalidates any prior binding.
	reassigned := map[string]bool{}
	// pendingConcat holds RHS of multi-operand concats we couldn't resolve
	// in the first pass (operands not yet bound). We retry with fixed-point.
	pendingConcat := map[string]string{}

	// Pass 1: collect bindings (line by line, with per-line normalization).
	for _, rawLine := range lines {
		// Apply f-string fold + join fold + concat fold so that
		//   v = f"{'ev'}{'al'}"   becomes  v = "eval"
		//   k = "".join(["e","x","e","c"])   becomes  k = "exec"
		//   $f = "sys"."tem"   becomes  $f = "system"
		// before binding extraction. The order matters: f-strings and joins
		// produce quoted literals that the concat folder then merges.
		line := strings.TrimSpace(foldStringConcat(foldJoin(foldFStrings(rawLine))))

		// import X as alias  /  from pkg import X as alias
		if m := importAliasRe.FindStringSubmatch(line); m != nil {
			modName := m[1]
			alias := m[2]
			if dangerousModuleAliases[modName] {
				if firstSeen[alias] {
					reassigned[alias] = true
					delete(moduleAlias, alias)
				} else {
					firstSeen[alias] = true
					moduleAlias[alias] = modName
				}
			}
			continue
		}

		// Stored .constructor reference: const F = (...).constructor
		if cm := constructorAssignRe.FindStringSubmatch(line); cm != nil {
			alias := cm[1]
			if firstSeen[alias] {
				reassigned[alias] = true
				delete(constructorAlias, alias)
			} else {
				firstSeen[alias] = true
				constructorAlias[alias] = true
			}
			continue
		}

		// Detect any assignment to track reassignments.
		if m := anyAssignRe.FindStringSubmatch(line); m != nil {
			varName := m[1]

			// Check for string assignment: $f = "system" or f = 'exec' or f = "sys"
			if sm := varStringAssignRe.FindStringSubmatch(line); sm != nil {
				val := sm[2]
				// Track every string assignment in allStringAlias so concat
				// resolution can compose innocent fragments into a dangerous
				// final value (e.g. $x="sys"; $y="tem"; $f=$x.$y → "system").
				if firstSeen[varName] {
					if existing, ok := allStringAlias[varName]; ok && existing != val {
						reassigned[varName] = true
						delete(allStringAlias, varName)
						delete(stringAlias, varName)
						delete(moduleAlias, varName)
					}
				} else {
					firstSeen[varName] = true
					allStringAlias[varName] = val
					if dangerousStringValues[val] {
						stringAlias[varName] = val
					}
				}
				continue
			}

			// Check for ident = ident: m = _o (module alias chain)
			if im := varIdentAssignRe.FindStringSubmatch(line); im != nil {
				lhs := im[1]
				rhs := im[2]
				if mod, ok := moduleAlias[rhs]; ok {
					if firstSeen[lhs] {
						reassigned[lhs] = true
						delete(moduleAlias, lhs)
					} else {
						firstSeen[lhs] = true
						moduleAlias[lhs] = mod
					}
					continue
				}
			}

			// Multi-operand concat: $f = $x . $y . "literal"
			if cm := varConcatAssignRe.FindStringSubmatch(line); cm != nil {
				lhs := cm[1]
				rhs := cm[2]
				// Try immediate resolution against allStringAlias (fragments
				// allowed). If some operand is not yet bound, queue for
				// fixed-point retry.
				if val, ok := resolveConcatOperands(rhs, allStringAlias); ok {
					firstSeen[lhs] = true
					allStringAlias[lhs] = val
					if dangerousStringValues[val] {
						stringAlias[lhs] = val
					}
				} else {
					pendingConcat[lhs] = rhs
				}
				continue
			}

			// Any other assignment to a tracked var → invalidate it.
			if firstSeen[varName] {
				reassigned[varName] = true
				delete(stringAlias, varName)
				delete(moduleAlias, varName)
			}
		}
	}

	// Pass 1.5: fixed-point retry for pending concats. Each iteration may
	// resolve more operands as earlier-pending bindings become known.
	for iter := 0; iter < maxAliasIterations && len(pendingConcat) > 0; iter++ {
		progress := false
		for lhs, rhs := range pendingConcat {
			if reassigned[lhs] {
				delete(pendingConcat, lhs)
				continue
			}
			if val, ok := resolveConcatOperands(rhs, allStringAlias); ok {
				firstSeen[lhs] = true
				allStringAlias[lhs] = val
				if dangerousStringValues[val] {
					stringAlias[lhs] = val
				}
				delete(pendingConcat, lhs)
				progress = true
			}
		}
		if !progress {
			break
		}
	}

	// Nothing to substitute → return early without allocating.
	if len(moduleAlias) == 0 && len(stringAlias) == 0 && len(constructorAlias) == 0 {
		return content
	}

	// Pass 2: substitute usages line by line.
	result := make([]string, len(lines))
	for i, line := range lines {
		// Module alias substitutions: alias. → module.
		for alias, mod := range moduleAlias {
			line = strings.ReplaceAll(line, alias+".", mod+".")
		}

		// String alias substitutions: $varName( → dangerous(  and  varName( → dangerous(
		for varName, dangerous := range stringAlias {
			phpPat := "$" + varName + "("
			if strings.Contains(line, phpPat) {
				line = strings.ReplaceAll(line, phpPat, dangerous+"(")
				continue
			}
			pyPat := varName + "("
			if idx := strings.Index(line, pyPat); idx >= 0 {
				if idx == 0 || (line[idx-1] != '.' && !isWordChar(line[idx-1])) {
					line = strings.Replace(line, pyPat, dangerous+"(", 1)
				}
			}
		}

		// Bracket indexing substitution: __dict__[var] → __dict__["literal"]
		// when var resolves to a known string. Iterates allStringAlias because
		// even fragments like "exec" may legitimately appear as an indexer.
		for varName, val := range allStringAlias {
			bracketPat := "[" + varName + "]"
			if strings.Contains(line, bracketPat) {
				line = strings.ReplaceAll(line, bracketPat, `["`+val+`"]`)
			}
		}

		// Stored constructor: rewrite F( to .constructor( so the existing
		// pattern catches the call. Skip the binding line itself.
		for alias := range constructorAlias {
			pat := alias + "("
			if idx := strings.Index(line, pat); idx >= 0 {
				if idx == 0 || (line[idx-1] != '.' && !isWordChar(line[idx-1])) {
					// Don't rewrite the assignment line.
					if !strings.Contains(line, alias+" =") && !strings.Contains(line, alias+"=") {
						line = strings.Replace(line, pat, ".constructor(", 1)
					}
				}
			}
		}

		result[i] = line
	}

	return strings.Join(result, "\n")
}

// isWordChar returns true for characters that can appear in an identifier.
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// normalizeLine applies all normalization steps to a single source line
// before it is fed to the regex rules:
//  1. Unicode confusable folding
//  2. Hex/octal/unicode escape decoding inside string literals
//  3. Python f-string literal-only interpolation folding
//  4. Python "".join([...]) folding
//  5. String concat folding ("a"+"b" → "ab", "a"."b" → "ab" for PHP)
func normalizeLine(line string) string {
	line = normalizeUnicode(line)
	line = decodeEscapesInStrings(line)
	line = foldFStrings(line)
	line = foldJoin(line)
	line = foldStringConcat(line)
	return line
}
