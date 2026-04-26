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

// stringConcatRe matches two adjacent string literals joined by + with optional
// whitespace, e.g. "foo" + "bar" or 'foo'+"bar".
// Both single- and double-quoted literals are handled; inner escapes are not
// interpreted here (folding is purely textual).
var stringConcatRe = regexp.MustCompile(`("(?:[^"\\]|\\.)*")\s*\+\s*("(?:[^"\\]|\\.)*")|('(?:[^'\\]|\\.)*')\s*\+\s*('(?:[^'\\]|\\.)*')`)

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

// normalizeLine applies all normalization steps to a single source line
// before it is fed to the regex rules:
//  1. Unicode confusable folding
//  2. Hex/octal/unicode escape decoding inside string literals
//  3. String concat folding ("a"+"b" → "ab")
func normalizeLine(line string) string {
	line = normalizeUnicode(line)
	line = decodeEscapesInStrings(line)
	line = foldStringConcat(line)
	return line
}
