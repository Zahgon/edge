package parser

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/jsstringify"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/js"
)

// The expected values are edge-parser 9.1.1's on Node 24.

func TestEdgeBuffer(t *testing.T) {
	b := NewEdgeBuffer("a.edge", BufferOptions{OutputVar: "out", RethrowCallPath: []string{"template", "reThrow"}})
	b.OutputRaw("Hello \"x\"\t\n")
	b.OutputExpression("state.a", "a.edge", 1, true)
	b.OutputExpression("`${x}`", "a.edge", 2, false)
	b.WriteExpression("let a = 1", "a.edge", 2)
	b.WriteStatement("if (a) {", "b.edge", 3)
	b.WriteStatement("}", "b.edge", -1)
	b.WriteStatement("// zero", "b.edge", 0)
	if b.Size() != 10 {
		t.Errorf("size %d, want 10", b.Size())
	}
	b.Wrap("function () {\nconst x = 1;", "}\n// end")
	want := strings.Join([]string{
		"function () {",
		"const x = 1;",
		`let out = "";`,
		"let $lineNumber = 1;",
		`let $filename = "a.edge";`,
		"try {",
		`out += "Hello \"x\"\t\n";`,
		"out += `${state.a}`;",
		"$lineNumber = 2;",
		"out += `${x}`;",
		"let a = 1;",
		`$filename = "b.edge";`,
		"$lineNumber = 3;",
		"if (a) {",
		"}",
		"// zero",
		"} catch (error) {",
		"template.reThrow(error, $filename, $lineNumber);",
		"}",
		"return out;",
		"}",
		"// end",
	}, "\n")
	if got := b.Flush(); got != want {
		t.Errorf("flush\n%s\nwant\n%s", got, want)
	}
	// The first flush fixes the output.
	b.OutputRaw("ignored")
	if got := b.Flush(); got != want {
		t.Errorf("second flush\n%s", got)
	}

	child := b.Create("c.edge", "slot_1")
	child.OutputRaw("x")
	wantChild := "let slot_1 = \"\";\ntry {\nslot_1 += \"x\";\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn slot_1;"
	if got := child.DisableFileAndLineVariables().Flush(); got != wantChild {
		t.Errorf("child buffer\n%s", got)
	}

	bare := b.Create("d.edge", "stack_1")
	bare.OutputRaw("y")
	got := bare.DisableFileAndLineVariables().DisableReturnStatement().DisableTryCatchBlock().DisableOutVariable().Flush()
	if got != `stack_1 += "y";` || bare.OutputVariableName() != "stack_1" {
		t.Errorf("bare buffer %q (%s)", got, bare.OutputVariableName())
	}
}

func TestStack(t *testing.T) {
	s := NewStack()
	s.DefineVariable("top")
	s.DefineScope()
	s.DefineVariable("a")
	s.DefineScope()
	s.DefineVariable("b")
	if got := s.List(); !reflect.DeepEqual(got, []string{"a", "b", "top"}) {
		t.Errorf("list %q", got)
	}
	if !s.Has("a") || !s.Has("b") || !s.Has("top") || s.Has("c") {
		t.Error("has")
	}
	s.ClearScope()
	if got := s.List(); !reflect.DeepEqual(got, []string{"a", "top"}) || s.Has("b") {
		t.Errorf("list after clearScope %q", got)
	}
	s.ClearScope()
	s.ClearScope()
	if got := s.List(); !reflect.DeepEqual(got, []string{"top"}) {
		t.Errorf("list after clearing every scope %q", got)
	}
}

func TestStackValues(t *testing.T) {
	// @let and @set define whatever value they find: undefined for a nested
	// pattern, numbers for a literal key. Lookups use ===, and a falsy value is
	// never found, as list.find returns it.
	s := NewStack()
	s.DefineValue(js.Undefined)
	s.DefineValue(float64(1))
	s.DefineValue(nil)
	s.DefineValue(float64(0))
	if s.HasValue(js.Undefined) || !s.HasValue(float64(1)) || s.HasValue("1") || s.HasValue(nil) || s.HasValue(float64(0)) {
		t.Error("has")
	}
	if got := s.List(); !reflect.DeepEqual(got, []string{"undefined", "1", "null", "0"}) {
		t.Errorf("list %q", got)
	}
	if got := JoinValues(s.Values(), ","); got != ",1,,0" {
		t.Errorf("join %q", got)
	}
}

func newTestParser() *Parser {
	return New(map[string]*Tag{}, NewStack(), Options{StatePropertyName: "state", EscapeCallPath: []string{"escape"}})
}

func TestGenerateASTLocations(t *testing.T) {
	p := newTestParser()
	type loc struct{ startLine, startCol, endLine, endCol int }
	locOf := func(n *acorn.Node) loc {
		return loc{n.Loc.Start.Line, n.Loc.Start.Column, n.Loc.End.Line, n.Loc.End.Column}
	}
	// Positions on the expression's first line are moved by the token's column;
	// the end columns are left alone.
	statement, err := p.GenerateAST("a + b", lexer.LexerLoc{Start: lexer.LexerPosition{Line: 3, Col: 7}, End: lexer.LexerPosition{Line: 3, Col: 12}}, "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	if got := []loc{locOf(statement), locOf(statement.Expression), locOf(statement.Expression.Right)}; !reflect.DeepEqual(got, []loc{{3, 7, 3, 5}, {3, 7, 3, 5}, {3, 11, 3, 5}}) {
		t.Errorf("locations %v", got)
	}
	statement, err = p.GenerateAST("a +\n  b", lexer.LexerLoc{Start: lexer.LexerPosition{Line: 3, Col: 7}, End: lexer.LexerPosition{Line: 4, Col: 4}}, "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	if got := []loc{locOf(statement.Expression), locOf(statement.Expression.Right)}; !reflect.DeepEqual(got, []loc{{3, 7, 4, 3}, {4, 2, 4, 3}}) {
		t.Errorf("multi-line locations %v", got)
	}
	empty, err := p.GenerateAST("", lexer.LexerLoc{}, "f.edge")
	if empty != nil || err != nil {
		t.Errorf("empty program: %v %v", empty, err)
	}
}

func TestGenerateASTErrors(t *testing.T) {
	tests := []struct {
		src       string
		message   string
		line, col float64
	}{
		{"a b", "Unexpected token ", 3, 9},
		{"a +\n  b c", "Unexpected token ", 4, 4},
		{"\n\n  )", "Unexpected token ", 5, 2},
		{`x = "abc`, "Unterminated string constant ", 3, 11},
	}
	p := newTestParser()
	for _, test := range tests {
		_, err := p.GenerateAST(test.src, lexer.LexerLoc{Start: lexer.LexerPosition{Line: 3, Col: 7}, End: lexer.LexerPosition{Line: 3, Col: 12}}, "f.edge")
		var edgeError *edgeerror.EdgeError
		if !errors.As(err, &edgeError) {
			t.Fatalf("%q: error %v", test.src, err)
		}
		want := edgeerror.EdgeError{Message: test.message, Code: "E_ACORN_ERROR", Filename: "f.edge", Line: test.line, Col: test.col}
		if *edgeError != want {
			t.Errorf("%q: %+v, want %+v", test.src, *edgeError, want)
		}
	}
}

func TestUtils(t *testing.T) {
	p := newTestParser()
	statement, err := p.GenerateAST("a + b", lexer.LexerLoc{Start: lexer.LexerPosition{Line: 3, Col: 7}}, "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Stringify(MakeCallable([]string{"escape"}, []*acorn.Node{statement.Expression})); got != "escape(a + b)" {
		t.Errorf("makeCallable %q", got)
	}
	if got := p.Stringify(MakeMemberAccessor("ctx", &acorn.Node{Type: "Identifier", Name: "x"})); got != "ctx.x" {
		t.Errorf("makeMemberAccessor %q", got)
	}
	// A member accessor has no location of its own: its property's is used.
	if loc, err := GetExpressionLoc(MakeMemberAccessor("ctx", statement.Expression.Left)); err != nil || loc != (lexer.LexerPosition{Line: 3, Col: 7}) {
		t.Errorf("getExpressionLoc %v %v", loc, err)
	}
	arrow, err := p.GenerateAST("({ a, b: c }) => [d, e]", lexer.LexerLoc{}, "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	if names, err := CollectObjectExpressionProperties(arrow.Expression.Params[0]); err != nil || !reflect.DeepEqual(names, []string{"a", "c"}) {
		t.Errorf("collectObjectExpressionProperties %q %v", names, err)
	}
	if names, err := CollectArrayExpressionProperties(arrow.Expression.Body); err != nil || !reflect.DeepEqual(names, []string{"d", "e"}) {
		t.Errorf("collectArrayExpressionProperties %q %v", names, err)
	}
}

func TestProcessTokens(t *testing.T) {
	// A single escape call path is a plain function call; the rethrow call path is
	// joined with dots.
	p := newTestParser()
	buffer := NewEdgeBuffer("f.edge", BufferOptions{OutputVar: "out", RethrowCallPath: []string{"reThrow"}})
	tokens, err := p.Tokenize("{{ a }} {{{ b }}}\n@{{ c }}", "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if err := p.ProcessToken(token, buffer); err != nil {
			t.Fatal(err)
		}
	}
	want := "let out = \"\";\nlet $lineNumber = 1;\nlet $filename = \"f.edge\";\ntry {\nout += `${escape(state.a)}`;\nout += \" \";\nout += `${state.b}`;\nout += \"\\n\";\nout += \"{{ c }}\";\n} catch (error) {\nreThrow(error, $filename, $lineNumber);\n}\nreturn out;"
	if got := buffer.Flush(); got != want {
		t.Errorf("compiled\n%s\nwant\n%s", got, want)
	}
}

func TestOnTagAndOnMustache(t *testing.T) {
	// The hooks see every tag and mustache before it is compiled; an error stops
	// the compilation.
	var seen []string
	stop := errors.New("stop")
	tags := map[string]*Tag{"if": {TagName: "if", Block: true, Seekable: true, Compile: func(p *Parser, b *EdgeBuffer, token *lexer.TagToken) error {
		seen = append(seen, "compile "+token.Properties.Name)
		return nil
	}}}
	p := New(tags, nil, Options{
		StatePropertyName: "state", EscapeCallPath: []string{"escape"},
		OnTag: func(token *lexer.TagToken) error { seen = append(seen, "tag "+token.Properties.JsArg); return nil },
		OnMustache: func(token *lexer.MustacheToken) error {
			seen = append(seen, "mustache"+token.Properties.JsArg)
			return stop
		},
	})
	tokens, err := p.Tokenize("@if(a)\n@end\n{{ b }}", "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	buffer := NewEdgeBuffer("f.edge", BufferOptions{OutputVar: "out"})
	for _, token := range tokens {
		if err = p.ProcessToken(token, buffer); err != nil {
			break
		}
	}
	if err != stop || !reflect.DeepEqual(seen, []string{"tag a", "compile if", "mustache b "}) {
		t.Errorf("%v %q", err, seen)
	}
}

// TestStringLiterals checks the string literals the buffer writes, which
// js-stringify quotes.
func TestStringLiterals(t *testing.T) {
	for _, c := range stringifyCases {
		if got := jsstringify.String(c.in); got != c.want {
			t.Errorf("jsstringify.String(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

// TestStringifyThrowsFromCompile checks that a TypeError astring throws inside a
// tag's compile function comes out of ProcessToken, as the exception does in
// edge-parser.
func TestStringifyThrowsFromCompile(t *testing.T) {
	tags := map[string]*Tag{"show": {TagName: "show", Seekable: true, Compile: func(p *Parser, b *EdgeBuffer, token *lexer.TagToken) error {
		b.OutputExpression(p.Stringify(&acorn.Node{Type: "Foo"}), token.Filename, token.Loc.Start.Line, true)
		return nil
	}}}
	p := New(tags, nil, Options{StatePropertyName: "state", EscapeCallPath: []string{"escape"}})
	tokens, err := p.Tokenize("@show(a)", "f.edge")
	if err != nil {
		t.Fatal(err)
	}
	err = p.ProcessToken(tokens[0], NewEdgeBuffer("f.edge", BufferOptions{OutputVar: "out"}))
	var jsErr *js.Error
	if !errors.As(err, &jsErr) || jsErr.Name != "TypeError" || jsErr.Message != "state.generator[node.type] is not a function" {
		t.Errorf("ProcessToken = %v, want the TypeError of astring", err)
	}
}
