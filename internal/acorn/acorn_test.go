package acorn

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/js"
)

// edgeOptions are the options edge-parser passes to acorn.
func edgeOptions() Options {
	return Options{EcmaVersion: 2020, Locations: true, AllowAwaitOutsideFunction: true}
}

// The expected values in these tables were printed by acorn 8.19.0 under Node 24.

func TestParseErrors(t *testing.T) {
	tests := []struct {
		src                         string
		message                     string
		pos, line, column, raisedAt int
	}{
		{"a +", "Unexpected token (1:3)", 3, 1, 3, 3},
		{"a b", "Unexpected token (1:2)", 2, 1, 2, 3},
		{"let a; let a", "Identifier 'a' has already been declared (1:11)", 11, 1, 11, 12},
		{"class A { a = 1 }", "Unexpected token (1:12)", 12, 1, 12, 13},
		{"a ??= b", "Unexpected token (1:4)", 4, 1, 4, 5},
		{"1_000", "Identifier directly after number (1:1)", 1, 1, 1, 1},
		{"a?.b = 1", "Optional chaining cannot appear in left-hand side (1:0)", 0, 1, 0, 6},
		{"\"use strict\"; with (a) b", "'with' in strict mode (1:14)", 14, 1, 14, 18},
		{"return a", "'return' outside of function (1:0)", 0, 1, 0, 6},
		{"a => { \"use strict\"; 010 }", "Invalid number (1:21)", 21, 1, 21, 24},
		{"/a/gg", "Duplicate regular expression flag (1:1)", 1, 1, 1, 5},
		{"/(?<a>x)(?<a>y)/", "Invalid regular expression: /(?<a>x)(?<a>y)/: Duplicate capture group name (1:1)", 1, 1, 1, 16},
		{"/\\p{Foo}/u", "Invalid regular expression: /\\p{Foo}/: Invalid property name (1:1)", 1, 1, 1, 10},
		{"/a{2,1}/", "Invalid regular expression: /a{2,1}/: numbers out of order in {} quantifier (1:1)", 1, 1, 1, 8},
		{"`${a`", "Unterminated template literal (1:5)", 5, 1, 5, 5},
		{"\"abc", "Unterminated string constant (1:0)", 0, 1, 0, 4},
		{"/* x", "Unterminated comment (1:0)", 0, 1, 0, 2},
		{"a\n++", "Unexpected token (2:2)", 4, 2, 2, 4},
		{"new.target", "'new.target' can only be used in functions and class static block (1:0)", 0, 1, 0, 10},
		{"import.meta", "Cannot use 'import.meta' outside a module (1:0)", 0, 1, 0, 11},
		{"async () => await", "Unexpected token (1:17)", 17, 1, 17, 17},
		{"yield => 1;function* g() { yield => 1 }", "Unexpected token (1:33)", 33, 1, 33, 35},
		{"x = { a = 1 }", "Shorthand property assignments are valid only in destructuring patterns (1:8)", 8, 1, 8, 13},
		{"({a: 1} = b)", "Assigning to rvalue (1:5)", 5, 1, 5, 9},
		{"[...a, b] = c", "Comma is not permitted after the rest element (1:5)", 5, 1, 5, 11},
		{"a ?? b || c", "Logical expressions and coalesce expressions cannot be mixed. Wrap either by parentheses (1:7)", 7, 1, 7, 9},
		{"-a ** b", "Unexpected token (1:3)", 3, 1, 3, 7},
		{"class A extends B { constructor() { super.x; } } super()", "'super' keyword outside a method (1:49)", 49, 1, 49, 54},
		{"#x", "Unexpected character '#' (1:0)", 0, 1, 0, 0},
		{"@a", "Unexpected character '@' (1:0)", 0, 1, 0, 0},
		{"a\u2028b c", "Unexpected token (2:2)", 4, 2, 2, 5},
		{"var \\u0030", "Invalid Unicode escape (1:4)", 4, 1, 4, 10},
		{"f(a,,b)", "Unexpected token (1:4)", 4, 1, 4, 5},
		{"(a, b) => { let a }", "Identifier 'a' has already been declared (1:16)", 16, 1, 16, 19},
		{"function f(a = 1) { \"use strict\" }", "Illegal 'use strict' directive in function with non-simple parameter list (1:0)", 0, 1, 0, 19},
		{"x = 08n", "Identifier directly after number (1:6)", 6, 1, 6, 6},
		{"0b2", "Expected number in radix 2 (1:2)", 2, 1, 2, 2},
		{"1e", "Invalid number (1:0)", 0, 1, 0, 2},
		{"\"\\u{110000}\"", "Code point out of bounds (1:4)", 4, 1, 4, 11},
		{"`\\u`", "Bad escape sequence in untagged template literal (1:1)", 1, 1, 1, 3},
		{"try {}", "Missing catch or finally clause (1:0)", 0, 1, 0, 6},
		{"switch (a) { default: default: }", "Multiple default clauses (1:22)", 22, 1, 22, 30},
		{"a: a: b", "Label 'a' is already declared (1:3)", 3, 1, 3, 7},
		{"break", "Unsyntactic break (1:0)", 0, 1, 0, 5},
		{"function f() { new.foo }", "The only valid meta property for new is 'new.target' (1:19)", 19, 1, 19, 24},
		{"[a += 1] = b", "Only '=' operator can be used for specifying default value. (1:2)", 2, 1, 2, 10},
		{"async function", "Unexpected token (1:14)", 14, 1, 14, 14},
		{"{ import x from \"y\" }", "'import' and 'export' may only appear at the top level (1:2)", 2, 1, 2, 8},
		{"a ? function : b", "Unexpected token (1:13)", 13, 1, 13, 14},
	}
	for _, tt := range tests {
		_, err := Parse(tt.src, edgeOptions())
		var syntaxErr *SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Errorf("Parse(%q) = %v, want a SyntaxError", tt.src, err)
			continue
		}
		got := fmt.Sprintf("%s %d %d:%d %d", syntaxErr.Message, syntaxErr.Pos, syntaxErr.Loc.Line, syntaxErr.Loc.Column, syntaxErr.RaisedAt)
		want := fmt.Sprintf("%s %d %d:%d %d", tt.message, tt.pos, tt.line, tt.column, tt.raisedAt)
		if got != want {
			t.Errorf("Parse(%q) error = %s, want %s", tt.src, got, want)
		}
	}
}

func TestUnpairedSurrogateCharacter(t *testing.T) {
	// A lone lead surrogate at the end of the input reads as NaN, which acorn
	// prints through String.fromCharCode as U+10000.
	_, err := Parse("\xed\xa0\x80", edgeOptions())
	if err == nil || err.Error() != "Unexpected character '\U00010000' (1:0)" {
		t.Errorf("Parse(lone surrogate) error = %v", err)
	}
}

type tokenRow struct {
	typ                                  string
	value                                any
	start, end                           int
	startLine, startCol, endLine, endCol int
}

func tokensOf(t *testing.T, src string) []tokenRow {
	t.Helper()
	var rows []tokenRow
	opts := edgeOptions()
	opts.OnToken = func(tok *Token) {
		rows = append(rows, tokenRow{tok.Type, tok.Value, tok.Start, tok.End, tok.Loc.Start.Line, tok.Loc.Start.Column, tok.Loc.End.Line, tok.Loc.End.Column})
	}
	if _, err := Parse(src, opts); err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return rows
}

func TestTokens(t *testing.T) {
	tests := []struct {
		src  string
		want []tokenRow
	}{
		{"a?.b(`x${y}`) / 2", []tokenRow{
			{"name", "a", 0, 1, 1, 0, 1, 1},
			{"?.", "?.", 1, 3, 1, 1, 1, 3},
			{"name", "b", 3, 4, 1, 3, 1, 4},
			{"(", nil, 4, 5, 1, 4, 1, 5},
			{"`", nil, 5, 6, 1, 5, 1, 6},
			{"template", "x", 6, 7, 1, 6, 1, 7},
			{"${", nil, 7, 9, 1, 7, 1, 9},
			{"name", "y", 9, 10, 1, 9, 1, 10},
			{"}", nil, 10, 11, 1, 10, 1, 11},
			{"template", "", 11, 11, 1, 11, 1, 11},
			{"`", nil, 11, 12, 1, 11, 1, 12},
			{")", nil, 12, 13, 1, 12, 1, 13},
			{"/", "/", 14, 15, 1, 14, 1, 15},
			{"num", 2.0, 16, 17, 1, 16, 1, 17},
			{"eof", nil, 17, 17, 1, 17, 1, 17},
		}},
		{"x = /[/]/g + 1n + 'é' + 0.5", []tokenRow{
			{"name", "x", 0, 1, 1, 0, 1, 1},
			{"=", "=", 2, 3, 1, 2, 1, 3},
			{"regexp", &RegexLiteral{Pattern: "[/]", Flags: "g"}, 4, 10, 1, 4, 1, 10},
			{"+/-", "+", 11, 12, 1, 11, 1, 12},
			{"num", js.BigIntOf(1), 13, 15, 1, 13, 1, 15},
			{"+/-", "+", 16, 17, 1, 16, 1, 17},
			{"string", "é", 18, 21, 1, 18, 1, 21},
			{"+/-", "+", 22, 23, 1, 22, 1, 23},
			{"num", 0.5, 24, 27, 1, 24, 1, 27},
			{"eof", nil, 27, 27, 1, 27, 1, 27},
		}},
		{"await\n--x", []tokenRow{
			{"name", "await", 0, 5, 1, 0, 1, 5},
			{"++/--", "--", 6, 8, 2, 0, 2, 2},
			{"name", "x", 8, 9, 2, 2, 2, 3},
			{"eof", nil, 9, 9, 2, 3, 2, 3},
		}},
	}
	for _, tt := range tests {
		if got := tokensOf(t, tt.src); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("tokens of %q:\n got  %v\n want %v", tt.src, got, tt.want)
		}
	}
}

// TestRegexpOrDivision checks the token context acorn tracks to tell a regular
// expression from a division after a closing brace or parenthesis.
func TestRegexpOrDivision(t *testing.T) {
	tests := []struct{ src, types string }{
		{"function f() { return\n{}/1/g }", "function name ( ) { return { } regexp } eof"},
		{"function f() { return {}/1/g }", "function name ( ) { return { } / num / name } eof"},
		{"x = {}/1/g", "name = { } / num / name eof"},
		{"{}/1/g", "{ } regexp eof"},
		{"a\n++/b/.c", "name ++/-- regexp . name eof"},
		{"function* g() { yield\n{}/1/g }", "function * name ( ) { name { } regexp } eof"},
		{"function* g() { yield {}/1/g }", "function * name ( ) { name { } / num / name } eof"},
	}
	for _, tt := range tests {
		var types []string
		for _, row := range tokensOf(t, tt.src) {
			types = append(types, row.typ)
		}
		if got := strings.Join(types, " "); got != tt.types {
			t.Errorf("tokens of %q = %q, want %q", tt.src, got, tt.types)
		}
	}
}

func TestTokensBeforeError(t *testing.T) {
	var types []string
	opts := edgeOptions()
	opts.OnToken = func(tok *Token) { types = append(types, tok.Type) }
	if _, err := Parse("a + (b c", opts); err == nil {
		t.Fatal("Parse succeeded")
	}
	// The token at the error is not reported: acorn reports a token when it
	// moves past it.
	if want := []string{"name", "+/-", "(", "name"}; !reflect.DeepEqual(types, want) {
		t.Errorf("tokens = %v, want %v", types, want)
	}
}

// TestSharedPositions applies edge-parser's patchLoc in OnToken, which moves the
// positions of the tokens to the place of the expression in the template, and
// checks that the nodes see the change through the Position objects they share
// with the tokens.
func TestSharedPositions(t *testing.T) {
	const line, col = 7, 12
	opts := edgeOptions()
	opts.OnToken = func(tok *Token) {
		if tok.Loc.Start.Line == 1 {
			tok.Loc.Start.Column += col
		}
		tok.Loc.Start.Line += line - 1
		tok.Loc.End.Line += line - 1
	}
	program, err := Parse("user.name +\n  greeting(a)", opts)
	if err != nil {
		t.Fatal(err)
	}
	e := program.BodyList[0].Expression
	var got []string
	for _, n := range []*Node{program, e, e.Left, e.Left.Object, e.Left.Property, e.Right, e.Right.Callee, e.Right.Arguments[0]} {
		got = append(got, fmt.Sprintf("%s %d:%d-%d:%d", n.Type, n.Loc.Start.Line, n.Loc.Start.Column, n.Loc.End.Line, n.Loc.End.Column))
	}
	want := []string{
		"Program 1:0-8:13",
		"BinaryExpression 7:12-8:13",
		"MemberExpression 7:12-7:9",
		"Identifier 7:12-7:4",
		"Identifier 7:17-7:9",
		"CallExpression 8:2-8:13",
		"Identifier 8:2-8:10",
		"Identifier 8:11-8:12",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("locations:\n got  %q\n want %q", got, want)
	}
}

func TestLiterals(t *testing.T) {
	tests := []struct {
		src    string
		value  any
		bigint string
		regex  *RegexLiteral
	}{
		{"0x1F", 31.0, "", nil},
		{"017", 15.0, "", nil},
		{"019", 19.0, "", nil},
		{"1e21", 1e21, "", nil},
		{".5e-3", 0.0005, "", nil},
		{"9007199254740993", 9007199254740992.0, "", nil},
		{"1e999", math.Inf(1), "", nil},
		{`"\u{1F600}\x41"`, "😀A", "", nil},
		{"'a\\\nb'", "ab", "", nil},
		{"'a\\\u2028b'", "ab", "", nil},
		{"10n", js.BigIntOf(10), "10", nil},
		{"0b101n", js.BigIntOf(5), "5", nil},
		{`/a\/b/gi`, nil, "", &RegexLiteral{Pattern: `a\/b`, Flags: "gi"}},
		{"null", nil, "", nil},
		{"true", true, "", nil},
	}
	for _, tt := range tests {
		program, err := Parse(tt.src, edgeOptions())
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.src, err)
		}
		lit := program.BodyList[0].Expression
		if lit.Type != "Literal" || lit.Raw != tt.src || !reflect.DeepEqual(lit.LiteralValue, tt.value) || lit.Bigint != tt.bigint || !reflect.DeepEqual(lit.Regex, tt.regex) {
			t.Errorf("Parse(%q) = %s raw %q value %v bigint %q regex %v", tt.src, lit.Type, lit.Raw, lit.LiteralValue, lit.Bigint, lit.Regex)
		}
	}
}

func TestTemplateElements(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		src  string
		want []TemplateValue
	}{
		{"`a\\x41${b}\r\nc`", []TemplateValue{{Raw: `a\x41`, Cooked: str("aA")}, {Raw: "\nc", Cooked: str("\nc")}}},
		// A tagged template may hold invalid escapes; their cooked value is null.
		{"f`\\u${b}\\xZ`", []TemplateValue{{Raw: `\u`}, {Raw: `\xZ`}}},
		{"f`\\u$x`", []TemplateValue{{Raw: `\u$x`}}},
	}
	for _, tt := range tests {
		program, err := Parse(tt.src, edgeOptions())
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.src, err)
		}
		template := program.BodyList[0].Expression
		if template.Type == "TaggedTemplateExpression" {
			template = template.Quasi
		}
		var got []TemplateValue
		for _, quasi := range template.Quasis {
			got = append(got, *quasi.TemplateValue)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q) quasis = %+v, want %+v", tt.src, got, tt.want)
		}
	}
}

func TestEcmaVersions(t *testing.T) {
	tests := []struct {
		src         string
		ecmaVersion int
		await       bool
		err         string
	}{
		{"a?.b", 2019, false, "Unexpected token (1:2)"},
		{"a ?? b", 10, false, "Unexpected token (1:3)"},
		{"a ?? b", 0, false, ""},
		{"await x", 2020, false, "Unexpected token (1:6)"},
		{"await x", 2020, true, ""},
		{"async () => { for await (x of y); }", 2017, false, "Unexpected token (1:18)"},
		{"let x", 5, false, "Unexpected token (1:4)"},
		{"x = class {}", 3, false, "Unexpected token (1:10)"},
		{"x = { get a() {}, get a() {} }", 5, false, "Redefinition of property (1:22)"},
		{"do a; while (b) c", 5, false, "Unexpected token (1:16)"},
		{"do a; while (b) c", 6, false, ""},
		// In ECMAScript 2015 the rest element of a parameter must be an identifier.
		{"([...[a]]) => 1", 6, false, "Unexpected token (1:5)"},
		{"([...[a]]) => 1", 7, false, ""},
	}
	for _, tt := range tests {
		_, err := Parse(tt.src, Options{EcmaVersion: tt.ecmaVersion, AllowAwaitOutsideFunction: tt.await})
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != tt.err {
			t.Errorf("Parse(%q, ecmaVersion %d) error = %q, want %q", tt.src, tt.ecmaVersion, got, tt.err)
		}
	}
	if _, err := Parse("a", Options{EcmaVersion: 2021}); err == nil || !strings.Contains(err.Error(), "ecmaVersion 2021") {
		t.Errorf("Parse with ecmaVersion 2021: error = %v", err)
	}
}

func TestWithoutLocations(t *testing.T) {
	var tokens []*Token
	program, err := Parse("a + 1", Options{EcmaVersion: 2020, OnToken: func(tok *Token) { tokens = append(tokens, tok) }})
	if err != nil {
		t.Fatal(err)
	}
	right := program.BodyList[0].Expression.Right
	if program.Loc != nil || right.Loc != nil || program.Start != 0 || program.End != 5 || right.Start != 4 {
		t.Errorf("program %d-%d loc %v, right %d loc %v", program.Start, program.End, program.Loc, right.Start, right.Loc)
	}
	for _, tok := range tokens {
		if tok.Loc != nil {
			t.Errorf("token %s has a location", tok.Type)
		}
	}
	program, err = Parse("function f() { return new.target }", Options{EcmaVersion: 2020})
	if err != nil {
		t.Fatal(err)
	}
	meta := program.BodyList[0].Body.BodyList[0].Argument
	if meta.Type != "MetaProperty" || meta.Meta.Loc != nil || meta.Meta.Start != 22 || meta.Meta.End != 25 {
		t.Errorf("new.target = %s, meta %d-%d loc %v", meta.Type, meta.Meta.Start, meta.Meta.End, meta.Meta.Loc)
	}
}
