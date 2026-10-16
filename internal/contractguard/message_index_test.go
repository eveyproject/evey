package contractguard

import (
	"go/ast"
	"go/parser"
	"go/token"
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
	got := deadFields(t, fields, oneGoSource(server), client)
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
	got := deadFields(t, fields, oneGoSource(server), client)
	want := []string{"MsgB.note"}
	if !slices.Equal(got, want) {
		t.Fatalf("verdict = %v, want %v", got, want)
	}
}

// The index once walked the bytes by hand and skipped quotes but not
// comments: an apostrophe in a `//` comment opened a rune that closed at the
// next apostrophe in the file, and every key in between went unseen (found
// 2026-10-16 in the LoadTestReport literals, whose nolint comments hid the
// fields written after them). The parser drops comments before we look.
func TestGuardReadsPastCommentsInLiterals(t *testing.T) {
	server := `package s
func a() *pb.Msg {
	m := &pb.Msg{
		A: 1, // the server's view
		B: 2, /* it's measured */
		C: 3,
	}
	// nobody's business
	m.D = 4
	return m
}
`
	idx := indexServerWrites(t, oneGoSource(server), map[string]bool{"msg": true})
	for _, k := range []string{"A", "B", "C", "D"} {
		if !idx.typed["msg|"+k] {
			t.Errorf("Msg.%s is written after a commented line, the index did not see it: %v", k, idx.typed)
		}
	}
}

// Element literals of a slice or map elide their type: []*pb.Msg{{A: 1}}
// writes Msg.A as surely as &pb.Msg{A: 1} does.
func TestGuardTypesElidedElementLiterals(t *testing.T) {
	server := `package s
var rows = []*pb.Row{{A: 1}, {B: 2}}
var byKey = map[string]pb.Cell{"k": {C: 3}}
`
	idx := indexServerWrites(t, oneGoSource(server), map[string]bool{"row": true, "cell": true})
	for _, k := range []string{"row|A", "row|B", "cell|C"} {
		if !idx.typed[k] {
			t.Errorf("%s not indexed: %v", k, idx.typed)
		}
	}
}

// Every way a function can name the message type of a variable it later
// sets: parameter, result, var declaration, new(), type assertion, closure
// parameter, type switch clause.
func TestGuardTypesVariablesFromDeclarations(t *testing.T) {
	server := `package s
func a(p *pb.MsgA) (r *pb.MsgB) {
	p.P = 1
	r.R = 2
	var d pb.MsgC
	d.D = 3
	n := new(pb.MsgD)
	n.N = 4
	v, _ := any(nil).(*pb.MsgE)
	v.V = 5
	fn := func(c *pb.MsgF) { c.C = 6 }
	switch s := any(nil).(type) {
	case *pb.MsgG:
		s.S = 7
	}
	fn(nil)
	out := &pb.MsgH{Inner: &pb.MsgI{}}
	out.Inner.I = 8
	out.Other = &pb.MsgJ{}
	out.Other.J = 9
}
`
	messages := map[string]bool{}
	for _, m := range []string{"msga", "msgb", "msgc", "msgd", "msge", "msgf", "msgg", "msgh", "msgi", "msgj"} {
		messages[m] = true
	}
	idx := indexServerWrites(t, oneGoSource(server), messages)
	for _, k := range []string{"msga|P", "msgb|R", "msgc|D", "msgd|N", "msge|V", "msgf|C", "msgg|S", "msgh|Inner", "msgh|Other", "msgi|I", "msgj|J"} {
		if !idx.typed[k] {
			t.Errorf("%s not indexed as a typed write: %v", k, idx.typed)
		}
	}
	if len(idx.untyped) != 0 {
		t.Errorf("every receiver is typed here, untyped = %v", idx.untyped)
	}
}

func oneGoSource(src string) []goSource {
	return []goSource{{path: "fixture.go", src: src}}
}

// goSource is one server file: the path names it in a parse failure, and the
// body is parsed alone, since a concatenation of files is not a Go file.
type goSource struct {
	path string
	src  string
}

// serverIndex is what the guard knows of the server: the per-message writes
// ("msgkey|GoField"), the untyped assignment targets (Go field names set
// through a receiver whose type the scan could not determine) and the
// word-only string literals (the JSON names of the manual HTTP handlers).
type serverIndex struct {
	typed      map[string]bool
	untyped    map[string]bool
	stringLits map[string]bool
}

var wordStringRe = regexp.MustCompile(`"(\w+)"`)

// msgKey normalizes a Go or proto type name to a lookup key: the generated
// nested type ThingResponse_Nested is the proto message Nested, and the
// lowercase form lets unexported fixture structs stand in for messages.
func msgKey(name string) string {
	if i := strings.LastIndexByte(name, '_'); i >= 0 {
		name = name[i+1:]
	}
	return strings.ToLower(name)
}

// indexServerWrites parses every file and reads the writes off the syntax
// tree, so comments and strings can hide nothing. A file that does not parse
// fails the guard: the build gate runs first, so it is a scan bug, not a
// reason to go quiet.
func indexServerWrites(t *testing.T, files []goSource, messages map[string]bool) serverIndex {
	t.Helper()
	idx := serverIndex{typed: map[string]bool{}, untyped: map[string]bool{}, stringLits: map[string]bool{}}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f.path, f.src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", f.path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if n.Type != nil {
					idx.indexLiteral(n, n.Type, messages)
				}
			case *ast.BasicLit:
				if n.Kind == token.STRING {
					for _, m := range wordStringRe.FindAllStringSubmatch(n.Value, -1) {
						idx.stringLits[m[1]] = true
					}
				}
			case *ast.FuncDecl:
				idx.indexAssignments(n, messages)
			}
			return true
		})
	}
	return idx
}

// indexLiteral records the keys of one composite literal of type typ, then
// follows the element literals that elide their type into typ's element type.
// Typed nested literals are reached by the file walk itself.
func (idx serverIndex) indexLiteral(lit *ast.CompositeLit, typ ast.Expr, messages map[string]bool) {
	key := msgKey(namedType(typ))
	for _, elt := range lit.Elts {
		value := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && messages[key] {
				idx.typed[key+"|"+id.Name] = true
			}
			if sub, ok := kv.Key.(*ast.CompositeLit); ok && sub.Type == nil {
				idx.indexLiteral(sub, elidedType(typ, true), messages)
			}
			value = kv.Value
		}
		if sub, ok := value.(*ast.CompositeLit); ok && sub.Type == nil {
			idx.indexLiteral(sub, elidedType(typ, false), messages)
		}
	}
}

// indexAssignments reads one function: the message type of every variable or
// field path it receives, declares or asserts (resp, out.LastReport), then
// each `x.Field = ...` on them. Names are scoped to the function, as the same
// name (resp, out) is reused for unrelated messages across a file. A receiver
// the function never typed (a range variable, a call result, items[i]) is an
// untyped write, matched by name only.
func (idx serverIndex) indexAssignments(fn *ast.FuncDecl, messages map[string]bool) {
	vars := map[string][]string{}
	declare := func(path string, typ ast.Expr) {
		if path == "" || typ == nil {
			return
		}
		if n := namedType(typ); n != "" {
			vars[path] = append(vars[path], msgKey(n))
		}
	}
	// An initializer types its variable, and a literal initializer types the
	// paths under it: out := &pb.S{Report: &pb.R{}} makes out.Report an R.
	var declareInit func(path string, init ast.Expr)
	declareInit = func(path string, init ast.Expr) {
		if path == "" || init == nil {
			return
		}
		declare(path, valueType(init))
		if ue, ok := init.(*ast.UnaryExpr); ok && ue.Op == token.AND {
			init = ue.X
		}
		lit, ok := init.(*ast.CompositeLit)
		if !ok {
			return
		}
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					declareInit(path+"."+key.Name, kv.Value)
				}
			}
		}
	}
	declareFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, name := range f.Names {
				declare(name.Name, f.Type)
			}
		}
	}
	declareFields(fn.Recv)
	declareFields(fn.Type.Params)
	declareFields(fn.Type.Results)
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if i < len(n.Rhs) {
					declareInit(identPath(lhs), n.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if n.Type != nil {
					declare(name.Name, n.Type)
				} else if i < len(n.Values) {
					declareInit(name.Name, n.Values[i])
				}
			}
		case *ast.FuncLit:
			declareFields(n.Type.Params)
			declareFields(n.Type.Results)
		case *ast.TypeSwitchStmt:
			as, ok := n.Assign.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 {
				return true
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			for _, stmt := range n.Body.List {
				if cc, ok := stmt.(*ast.CaseClause); ok {
					for _, typ := range cc.List {
						declare(id.Name, typ)
					}
				}
			}
		}
		return true
	})
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					idx.indexWrite(vars, sel, messages)
				}
			}
		case *ast.IncDecStmt:
			if sel, ok := n.X.(*ast.SelectorExpr); ok {
				idx.indexWrite(vars, sel, messages)
			}
		}
		return true
	})
}

func (idx serverIndex) indexWrite(vars map[string][]string, sel *ast.SelectorExpr, messages map[string]bool) {
	types, known := vars[identPath(sel.X)]
	if !known {
		idx.untyped[sel.Sel.Name] = true
		return
	}
	for _, ty := range types {
		if messages[ty] {
			idx.typed[ty+"|"+sel.Sel.Name] = true
		}
	}
}

// identPath renders a chain of identifiers (out, out.LastReport) and nothing
// for any other receiver (a call, an index, a dereference).
func identPath(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if base := identPath(e.X); base != "" {
			return base + "." + e.Sel.Name
		}
	}
	return ""
}

// namedType is the bare name behind a pointer, a generic instantiation or a
// package qualifier: &pb.Msg{}, pb.Msg{}, Msg{} and Gen[T]{} all name their
// type. A slice, map, array or function type has no name here.
func namedType(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.StarExpr:
		return namedType(e.X)
	case *ast.ParenExpr:
		return namedType(e.X)
	case *ast.IndexExpr:
		return namedType(e.X)
	case *ast.IndexListExpr:
		return namedType(e.X)
	}
	return ""
}

// elidedType is the type an element literal written without one takes from
// its container: []*pb.Msg{{A: 1}} and map[string]pb.Msg{"k": {A: 1}}.
func elidedType(container ast.Expr, isKey bool) ast.Expr {
	switch c := container.(type) {
	case *ast.ArrayType:
		return c.Elt
	case *ast.MapType:
		if isKey {
			return c.Key
		}
		return c.Value
	}
	return nil
}

// valueType is the message type an initializer gives its variable: &pb.Msg{},
// pb.Msg{}, new(pb.Msg), y.(*pb.Msg). A call result stays untyped.
func valueType(e ast.Expr) ast.Expr {
	switch e := e.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return valueType(e.X)
		}
	case *ast.CompositeLit:
		return e.Type
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "new" && len(e.Args) == 1 {
			return e.Args[0]
		}
	case *ast.TypeAssertExpr:
		return e.Type
	}
	return nil
}
