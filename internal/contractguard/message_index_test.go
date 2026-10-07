package contractguard

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Two messages share the field name updated_at; the server fills it for A
// only. A name-level match would call B.updated_at written (the masking that
// let WatchProgress.updated_at and SystemInfo.server_id ship empty).
func TestGuardIndexesWritesPerMessage(t *testing.T) {
	fields := []protoField{
		{Message: "MsgA", Snake: "updated_at"},
		{Message: "MsgB", Snake: "updated_at"},
		{Message: "MsgB", Snake: "title"},
	}
	server := `package s
func a() *pb.MsgA { return &pb.MsgA{UpdatedAt: 1} }
func b() *pb.MsgB { return &pb.MsgB{Title: "x", Inner: pb.Other{UpdatedAt: 2}} }
`
	client := `const t = row.updatedAt + row.title`
	got := deadFields(fields, server, client)
	want := []string{"MsgB.updated_at"}
	if !slices.Equal(got, want) {
		t.Fatalf("verdict = %v, want %v", got, want)
	}
}

// A setter on a variable the guard can type is exact; one it cannot type
// falls back to the name-level match.
func TestGuardTypedAndUntypedAssignments(t *testing.T) {
	fields := []protoField{
		{Message: "MsgA", Snake: "note"},
		{Message: "MsgB", Snake: "note"},
		{Message: "MsgC", Snake: "tag"},
		{Message: "MsgD", Snake: "tag"},
	}
	server := `package s
func a() {
	x := &pb.MsgA{}
	x.Note = "n"
	other.Tag = "t"
}
`
	client := `a.note; a.tag`
	got := deadFields(fields, server, client)
	want := []string{"MsgB.note"}
	if !slices.Equal(got, want) {
		t.Fatalf("verdict = %v, want %v", got, want)
	}
}

var (
	typedLiteralRe = regexp.MustCompile(`(\w+)\{`)
	varInitRe      = regexp.MustCompile(`(\w+)\s*:?=\s*&?(?:\w+\.)?(\w+)\{`)
	varDeclRe      = regexp.MustCompile(`\b(\w+)\s+\*?\w+\.(\w+)\b`)
	fieldAssignRe  = regexp.MustCompile(`\b(\w+)\.(\w+)\s*=[^=]`)
)

// msgKey normalizes a Go or proto type name to a lookup key: the generated
// nested type ThingResponse_Nested is the proto message Nested, and the
// lowercase form lets unexported fixture structs stand in for messages.
func msgKey(name string) string {
	if i := strings.LastIndexByte(name, '_'); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(name)
}

// indexServerWrites returns the per-message writes ("msgkey|GoField") and the
// untyped assignment targets (Go field names set through a receiver whose
// type the scan could not determine).
func indexServerWrites(corpus string, messages map[string]bool) (typed, untyped map[string]bool) {
	typed = map[string]bool{}
	untyped = map[string]bool{}
	for _, m := range typedLiteralRe.FindAllStringSubmatchIndex(corpus, -1) {
		key := msgKey(corpus[m[2]:m[3]])
		if !messages[key] {
			continue
		}
		for _, k := range literalKeys(corpus, m[1]-1) {
			typed[key+"|"+k] = true
		}
	}
	// Variable types are scoped to the enclosing function: the same name
	// (resp, out) is reused for unrelated messages across a file.
	for fn := range strings.SplitSeq(corpus, "\nfunc ") {
		vars := map[string][]string{}
		for _, re := range []*regexp.Regexp{varInitRe, varDeclRe} {
			for _, m := range re.FindAllStringSubmatch(fn, -1) {
				vars[m[1]] = append(vars[m[1]], msgKey(m[2]))
			}
		}
		for _, m := range fieldAssignRe.FindAllStringSubmatch(fn, -1) {
			types, known := vars[m[1]]
			if !known {
				untyped[m[2]] = true
				continue
			}
			for _, ty := range types {
				if messages[ty] {
					typed[ty+"|"+m[2]] = true
				}
			}
		}
	}
	return typed, untyped
}

// literalKeys returns the keys set directly in the composite literal whose
// opening brace is at s[open]; keys of nested literals, calls and closures
// are skipped by depth tracking, strings and runes are skipped whole.
func literalKeys(s string, open int) []string {
	var keys []string
	depth := 0
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '`' || c == '\'':
			i = skipQuoted(s, i)
		case c == '{' || c == '(' || c == '[':
			depth++
		case c == '}' || c == ')' || c == ']':
			depth--
			if depth == 0 {
				return keys
			}
		case depth == 1 && isWordByte(c) && (i == 0 || !isWordByte(s[i-1])):
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			k := j
			for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
				k++
			}
			if k < len(s) && s[k] == ':' && (k+1 >= len(s) || s[k+1] != '=') {
				keys = append(keys, s[i:j])
			}
			i = j - 1
		}
	}
	return keys
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func skipQuoted(s string, i int) int {
	q := s[i]
	for i++; i < len(s); i++ {
		if s[i] == '\\' && q != '`' {
			i++
			continue
		}
		if s[i] == q {
			return i
		}
	}
	return i
}
