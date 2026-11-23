package astring

import (
	"errors"
	"math"
	"testing"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/js"
)

// The expected outputs in these tables were printed by astring 1.9.0 under Node 24.

func TestGenerate(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a + b * c", "a + b * c;\n"},
		{"(a + b) * c", "(a + b) * c;\n"},
		{"a - (b - c)", "a - (b - c);\n"},
		{"a ** b ** c", "a ** b ** c;\n"},
		{"(a ** b) ** c", "(a ** b) ** c;\n"},
		{"(-a) ** b", "(-a) ** b;\n"},
		{"a ?? (b || c)", "a ?? (b || c);\n"},
		{"(a && b) ?? c", "(a && b) ?? c;\n"},
		{"a in b", "(a in b);\n"},
		{"for (var x = (a in b);;);", "for (var x = (a in b); ; ) ;\n"},
		{"- -a", "- -a;\n"},
		{"-(-a)", "- -a;\n"},
		{"+ +a", "+ +a;\n"},
		{"- --a", "- --a;\n"},
		{"typeof (a + b)", "typeof (a + b);\n"},
		{"!(a && b)", "!(a && b);\n"},
		{"void 0", "void 0;\n"},
		{"new (a())()", "new (a())();\n"},
		{"new (a().b)()", "new (a().b)();\n"},
		{"new a.b()", "new a.b();\n"},
		// astring does not parenthesize an optional chain as a callee of new.
		{"new (a?.b)()", "new a?.b();\n"},
		{"(function () {})()", "(function () {})();\n"},
		{"(() => {})()", "(() => {})();\n"},
		{"({}).toString()", "({}).toString();\n"},
		{"({a} = b)", "({a} = b);\n"},
		{"x = {a, b: 1, [c]: 2, d() {}, get e() { return 1; }, set f(v) {}, ...g}", "x = {\n  a,\n  b: 1,\n  [c]: 2,\n  d() {},\n  get e() {\n    return 1;\n  },\n  set f(v) {},\n  ...g\n};\n"},
		{"x = [1, , 2, , ]", "x = [1, , 2, , ];\n"},
		{"a?.b?.[c]?.(d)", "a?.b?.[c]?.(d);\n"},
		{"a`x${b}y`", "a`x${b}y`;\n"},
		{"`a${b}c${d}`", "`a${b}c${d}`;\n"},
		{"async (a, b) => ({a})", "(async (a, b) => ({\n  a\n}));\n"},
		{"x = async a => await a", "x = async a => await a;\n"},
		{"function* g() { yield* a; yield; }", "function* g() {\n  yield* a;\n  yield;\n}\n"},
		{"class A extends (B, C) { static m() {} get g() {} constructor() { super(); } }", "class A extends (B, C) {\n  static m() {}\n  get g() {}\n  constructor() {\n    super();\n  }\n}\n"},
		{"if (a) b; else if (c) d; else { e; }", "if (a) b; else if (c) d; else {\n  e;\n}\n"},
		{"switch (a) {\n  case 1:\n    b();\n    break;\n  default:\n    c();\n}", "switch (a) {\n  case 1:\n    b();\n    break;\n  default:\n    c();\n}\n"},
		{"try { a(); } catch { b(); } finally { c(); }", "try {\n  a();\n} catch {\n  b();\n} finally {\n  c();\n}\n"},
		{"for (const [k, v] of m) { if (k) continue; }", "for (const [k, v] of m) {\n  if (k) continue;\n}\n"},
		{"label: for (;;) break label;", "label: for (; ; ) break label;\n"},
		{"do x++; while (x < 3)", "do x++; while (x < 3);\n"},
		{"(a, b), c", "((a, b), c);\n"},
		{"x = (a, b)", "x = (a, b);\n"},
		{"a ? b : c ? d : e", "a ? b : c ? d : e;\n"},
		{"(a ? b : c) ? d : e", "(a ? b : c) ? d : e;\n"},
		{"(a = b) ? c : d", "(a = b) ? c : d;\n"},
		{"x = a => b => c", "x = a => b => c;\n"},
		{`'A' + "\x41" + 0x10 + 1e21 + 1n`, `'A' + "\x41" + 0x10 + 1e21 + 1n;` + "\n"},
		{`/[/]\//g.test(a)`, `(/[/]\//g).test(a);` + "\n"},
		{`"use strict"; ({ "a": 1, 2: b, 0x3: c })`, "\"use strict\";\n({\n  \"a\": 1,\n  2: b,\n  0x3: c\n});\n"},
		{`x = { "a b": 1 }.a`, "x = ({\n  \"a b\": 1\n}).a;\n"},
		{"import(a).then(b)", "import(a).then(b);\n"},
	}
	for _, tt := range tests {
		program, err := acorn.Parse(tt.src, acorn.Options{EcmaVersion: 2020, Locations: true, AllowAwaitOutsideFunction: true})
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.src, err)
		}
		if got, err := Generate(program); err != nil || got != tt.want {
			t.Errorf("Generate(%q) = %q, %v, want %q", tt.src, got, err, tt.want)
		}
	}
}

func ident(name string) *acorn.Node { return &acorn.Node{Type: "Identifier", Name: name} }

func lit(value any) *acorn.Node { return &acorn.Node{Type: "Literal", LiteralValue: value} }

func member(object, property *acorn.Node, computed bool) *acorn.Node {
	return &acorn.Node{Type: "MemberExpression", Object: object, Property: property, Computed: computed}
}

func array(elements ...*acorn.Node) *acorn.Node {
	return &acorn.Node{Type: "ArrayExpression", Elements: elements}
}

// TestGenerateHandBuilt prints nodes built the way edge-parser builds them:
// without raw text or locations.
func TestGenerateHandBuilt(t *testing.T) {
	tests := []struct {
		name string
		node *acorn.Node
		want string
	}{
		{"call", &acorn.Node{Type: "CallExpression", Callee: member(ident("template"), ident("escape"), false), Arguments: []*acorn.Node{ident("x")}}, "template.escape(x)"},
		{"member", member(ident("state"), ident("user"), false), "state.user"},
		{"computed member", member(ident("state"), lit("a-b"), true), `state["a-b"]`},
		{"strings", array(lit(`a"b`), lit("c'd"), lit("\n \xed\xa0\x80"), lit("</script>")), `["a\"b", "c'd", "\n` + " " + `\ud800", "</script>"]`},
		{"numbers", array(lit(1.5), lit(math.Copysign(0, -1)), lit(1e21), lit(1.0/3), lit(math.Inf(1)), lit(math.NaN()), lit(nil), lit(true)), "[1.5, 0, 1e+21, 0.3333333333333333, null, null, null, true]"},
		{"regex and bigint", array(&acorn.Node{Type: "Literal", Regex: &acorn.RegexLiteral{Pattern: "a/b", Flags: "g"}}, &acorn.Node{Type: "Literal", Bigint: "10"}), "[/a/b/g, 10n]"},
		{"object", &acorn.Node{Type: "ObjectExpression", Properties: []*acorn.Node{
			{Type: "Property", Key: ident("a"), Value: member(ident("state"), ident("a"), false), Kind: "init"},
			{Type: "Property", Key: lit("b"), Value: lit(2.0), Kind: "init"},
		}}, "{\n  a: state.a,\n  \"b\": 2\n}"},
		{"literal without value", &acorn.Node{Type: "Literal", LiteralValue: js.Undefined}, "undefined"},
		{"postfix unary", &acorn.Node{Type: "UnaryExpression", Operator: "!", Argument: ident("a")}, "a!"},
		{"declarator", &acorn.Node{Type: "VariableDeclarator", Id: ident("a"), Init: lit(1.0)}, "a = 1"},
		{"template element", &acorn.Node{Type: "TemplateElement", TemplateValue: &acorn.TemplateValue{Raw: `x\n`}}, `x\n`},
		// An operator astring does not know has no precedence, so it is never
		// parenthesized.
		{"unknown operator", &acorn.Node{Type: "BinaryExpression", Operator: "+", Left: &acorn.Node{Type: "BinaryExpression", Operator: "@", Left: ident("a"), Right: ident("b")}, Right: ident("c")}, "a @ b + c"},
	}
	for _, tt := range tests {
		if got, err := Generate(tt.node); err != nil || got != tt.want {
			t.Errorf("%s: Generate = %q, %v, want %q", tt.name, got, err, tt.want)
		}
	}
}

// TestGenerateErrors checks the TypeErrors astring 1.9.0 throws under Node 24
// for trees it cannot print.
func TestGenerateErrors(t *testing.T) {
	tests := []struct {
		name string
		node *acorn.Node
		want string
	}{
		{"unknown root type", &acorn.Node{Type: "Foo"}, "state.generator[node.type] is not a function"},
		{"unknown operand type", &acorn.Node{Type: "BinaryExpression", Operator: "+", Left: ident("a"), Right: &acorn.Node{Type: "Bogus"}}, "generator[node.type] is not a function"},
		{"unknown statement expression", &acorn.Node{Type: "ExpressionStatement", Expression: &acorn.Node{Type: "Nope"}}, "this[node.expression.type] is not a function"},
		{"unknown argument", &acorn.Node{Type: "CallExpression", Callee: ident("f"), Arguments: []*acorn.Node{ident("a"), {Type: "X"}}}, "generator[param.type] is not a function"},
		{"unknown array element", array(ident("a"), &acorn.Node{Type: "X"}), "this[element.type] is not a function"},
		{"missing test", &acorn.Node{Type: "IfStatement", Consequent: &acorn.Node{Type: "EmptyStatement"}}, "Cannot read properties of undefined (reading 'type')"},
		{"bigint value", lit(js.BigIntOf(1)), "Do not know how to serialize a BigInt"},
		{"import declaration", &acorn.Node{Type: "ImportDeclaration", Source: lit("x")}, "Cannot destructure property 'length' of 'specifiers' as it is undefined."},
		{"export declaration", &acorn.Node{Type: "ExportNamedDeclaration"}, "Cannot destructure property 'length' of 'specifiers' as it is undefined."},
		{"export default", &acorn.Node{Type: "ExportDefaultDeclaration"}, "Cannot read properties of undefined (reading 'type')"},
		{"missing unary operand", &acorn.Node{Type: "UnaryExpression", Operator: "-", Prefix: true}, "Cannot read properties of undefined (reading 'type')"},
		{"missing callee", &acorn.Node{Type: "NewExpression"}, "Cannot read properties of undefined (reading 'type')"},
	}
	for _, tt := range tests {
		got, err := Generate(tt.node)
		var jsErr *js.Error
		if !errors.As(err, &jsErr) || jsErr.Name != "TypeError" || jsErr.Message != tt.want || got != "" {
			t.Errorf("%s: Generate = %q, %v, want TypeError %q", tt.name, got, err, tt.want)
		}
	}
}

// TestGenerateLaterNodes prints, built by hand, node types acorn produces from
// ECMAScript 2022 classes and from modules.
func TestGenerateLaterNodes(t *testing.T) {
	field := func(key *acorn.Node, value *acorn.Node, static bool) *acorn.Node {
		return &acorn.Node{Type: "PropertyDefinition", Key: key, Value: value, Static: static}
	}
	private := &acorn.Node{Type: "PrivateIdentifier", Name: "x"}
	body := &acorn.Node{Type: "ClassBody", BodyList: []*acorn.Node{
		field(private, lit(1.0), false),
		field(ident("y"), nil, true),
		{Type: "StaticBlock", BodyList: []*acorn.Node{{Type: "ExpressionStatement", Expression: member(&acorn.Node{Type: "ThisExpression"}, private, false)}}},
	}}
	tests := []struct {
		node *acorn.Node
		want string
	}{
		{&acorn.Node{Type: "ClassDeclaration", Id: ident("A"), Body: body}, "class A {\n  #x = 1;\n  static y;\n  static {\n    this.#x;\n  }\n}"},
		{&acorn.Node{Type: "ExportAllDeclaration", Source: lit("m")}, `export * from "m";`},
		{&acorn.Node{Type: "ImportAttribute", Key: ident("type"), Value: lit("json")}, `type: "json"`},
	}
	for _, tt := range tests {
		if got, err := Generate(tt.node); err != nil || got != tt.want {
			t.Errorf("Generate(%s) = %q, %v, want %q", tt.node.Type, got, err, tt.want)
		}
	}
}
