package acorn

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/js"
)

// recordedCase is one input of testdata/acorn.json.gz with what acorn 8.19.0 made
// of it under Node (see testdata/record.mjs): the onToken stream, and the
// SyntaxError or the AST. Values are in the JSON form of esTree.
type recordedCase struct {
	Src    []uint16 `json:"src"`
	Ecma   int      `json:"ecma"`
	Await  bool     `json:"await"`
	Tokens []any    `json:"tokens"`
	Error  *struct {
		Message  any `json:"message"`
		Pos      int `json:"pos"`
		Loc      struct{ Line, Column int }
		RaisedAt int `json:"raisedAt"`
	} `json:"error"`
	AST any `json:"ast"`
}

func TestRecorded(t *testing.T) {
	paths, err := filepath.Glob("testdata/acorn*.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no recordings in testdata")
	}
	total, failed := 0, 0
	for _, path := range paths {
		var cases []recordedCase
		if err := fixture.DecodeGzipJSON(path, &cases); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for i, c := range cases {
			total++
			if msg := c.check(); msg != "" {
				failed++
				if failed <= 30 {
					t.Errorf("%s #%d (ecmaVersion %d, await %v) %q:\n%s", filepath.Base(path), i, c.Ecma, c.Await, jsstr.FromUTF16(c.Src), msg)
				}
			}
		}
	}
	t.Logf("%d recorded cases, %d mismatches", total, failed)
}

// check parses the case and describes the first difference from the recording.
func (c *recordedCase) check() string {
	var tokens []*Token
	program, err := Parse(jsstr.FromUTF16(c.Src), Options{
		EcmaVersion:               c.Ecma,
		Locations:                 true,
		AllowAwaitOutsideFunction: c.Await,
		OnToken:                   func(t *Token) { tokens = append(tokens, t) },
	})
	enc := newESTreeEncoder(tokens, c.Ecma)
	if got := enc.tokens(); !jsonEqual(got, c.Tokens) {
		return diff("tokens", got, c.Tokens)
	}
	if c.Error != nil {
		syntaxErr, ok := err.(*SyntaxError)
		if !ok {
			return "no error, want " + mustJSON(c.Error)
		}
		got := map[string]any{
			"message": encodeString(syntaxErr.Message), "pos": syntaxErr.Pos,
			"line": syntaxErr.Loc.Line, "column": syntaxErr.Loc.Column, "raisedAt": syntaxErr.RaisedAt,
		}
		want := map[string]any{
			"message": c.Error.Message, "pos": c.Error.Pos,
			"line": c.Error.Loc.Line, "column": c.Error.Loc.Column, "raisedAt": c.Error.RaisedAt,
		}
		if !jsonEqual(got, want) {
			return diff("error", got, want)
		}
		return ""
	}
	if err != nil {
		return "error " + err.Error()
	}
	if got := enc.node(program); !jsonEqual(got, c.AST) {
		return diff("ast", got, c.AST)
	}
	return ""
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// jsonEqual compares two values by their JSON form, so that Go ints compare equal
// to the float64 numbers of decoded JSON.
func jsonEqual(got, want any) bool {
	return reflect.DeepEqual(fromJSON(got), fromJSON(want))
}

func fromJSON(v any) any {
	var out any
	if err := json.Unmarshal([]byte(mustJSON(v)), &out); err != nil {
		panic(err)
	}
	return out
}

// diff describes the first difference between got and want by its path.
func diff(what string, got, want any) string {
	return what + firstDifference(fromJSON(got), fromJSON(want))
}

func firstDifference(got, want any) string {
	switch w := want.(type) {
	case map[string]any:
		if g, ok := got.(map[string]any); ok {
			for k, wv := range w {
				if gv, ok := g[k]; !ok {
					return "." + k + ": missing, want " + mustJSON(wv)
				} else if !reflect.DeepEqual(gv, wv) {
					return "." + k + firstDifference(gv, wv)
				}
			}
			for k, gv := range g {
				if _, ok := w[k]; !ok {
					return "." + k + ": unexpected " + mustJSON(gv)
				}
			}
		}
	case []any:
		if g, ok := got.([]any); ok && len(g) == len(w) {
			for i := range w {
				if !reflect.DeepEqual(g[i], w[i]) {
					return "[" + strconv.Itoa(i) + "]" + firstDifference(g[i], w[i])
				}
			}
		}
	}
	return ":\n got  " + mustJSON(got) + "\n want " + mustJSON(want)
}

// encodeString is the recorder's encStr: a string, or {$wtf: code units} for a
// string holding an unpaired surrogate.
func encodeString(s string) any {
	units := jsstr.ToUTF16(s)
	if jsstr.FromUTF16WellFormed(units) == s {
		return s
	}
	return map[string]any{"$wtf": units}
}

// esTreeEncoder turns tokens and nodes into the JSON form the recorder writes for
// acorn's objects: the properties acorn sets on each node type, with positions
// tagged by the token whose start or end Position object they are.
type esTreeEncoder struct {
	ecmaVersion int
	refs        map[*Position]string
	toks        []*Token
}

func newESTreeEncoder(tokens []*Token, ecmaVersion int) *esTreeEncoder {
	if ecmaVersion >= 2015 {
		ecmaVersion -= 2009
	}
	e := &esTreeEncoder{ecmaVersion: ecmaVersion, refs: map[*Position]string{}, toks: tokens}
	for i, t := range tokens {
		if _, seen := e.refs[t.Loc.Start]; !seen {
			e.refs[t.Loc.Start] = "s" + strconv.Itoa(i)
		}
		if _, seen := e.refs[t.Loc.End]; !seen {
			e.refs[t.Loc.End] = "e" + strconv.Itoa(i)
		}
	}
	return e
}

func (e *esTreeEncoder) position(p *Position) any {
	ref, ok := e.refs[p]
	if !ok {
		ref = "-"
	}
	return map[string]any{"line": p.Line, "column": p.Column, "ref": ref}
}

func (e *esTreeEncoder) loc(l *SourceLocation) any {
	return map[string]any{"start": e.position(l.Start), "end": e.position(l.End)}
}

func encodeNumber(v float64) any { return map[string]any{"$num": js.NumberToString(v)} }

// value encodes a token value or a literal value.
func (e *esTreeEncoder) value(v any) any {
	switch v := v.(type) {
	case nil:
		return map[string]any{"$undef": 1}
	case string:
		return encodeString(v)
	case float64:
		return encodeNumber(v)
	case js.BigInt:
		return map[string]any{"$big": v.String()}
	case bool:
		return v
	}
	panic("unexpected value")
}

func (e *esTreeEncoder) tokens() []any {
	out := []any{}
	for _, t := range e.toks {
		var value any
		if re, ok := t.Value.(*RegexLiteral); ok {
			value = map[string]any{"pattern": encodeString(re.Pattern), "flags": re.Flags, "value": true}
		} else {
			value = e.value(t.Value)
		}
		out = append(out, map[string]any{"type": t.Type, "value": value, "start": t.Start, "end": t.End, "loc": e.loc(t.Loc)})
	}
	return out
}

func (e *esTreeEncoder) nodes(list []*Node) []any {
	out := []any{}
	for _, n := range list {
		out = append(out, e.node(n))
	}
	return out
}

// node encodes a node with the properties acorn gives its type.
func (e *esTreeEncoder) node(n *Node) any {
	if n == nil {
		return nil
	}
	m := map[string]any{"type": n.Type, "start": n.Start, "end": n.End, "loc": e.loc(n.Loc)}
	function := func() {
		m["id"], m["params"], m["body"] = e.node(n.Id), e.nodes(n.Params), e.node(n.Body)
		m["expression"] = n.ExpressionBody
		if e.ecmaVersion >= 6 {
			m["generator"] = n.Generator
		}
		if e.ecmaVersion >= 8 {
			m["async"] = n.Async
		}
	}
	switch n.Type {
	case "Program":
		m["body"], m["sourceType"] = e.nodes(n.BodyList), n.SourceType
	case "Identifier":
		m["name"] = encodeString(n.Name)
	case "Literal":
		m["raw"] = encodeString(n.Raw)
		if n.Regex != nil {
			m["value"] = map[string]any{"$re": true}
			m["regex"] = map[string]any{"pattern": encodeString(n.Regex.Pattern), "flags": n.Regex.Flags}
		} else if n.LiteralValue == nil {
			m["value"] = nil
		} else {
			m["value"] = e.value(n.LiteralValue)
		}
		if n.Bigint != "" {
			m["bigint"] = n.Bigint
		}
	case "ExpressionStatement":
		m["expression"] = e.node(n.Expression)
		if n.Directive != nil {
			m["directive"] = encodeString(*n.Directive)
		}
	case "BlockStatement", "ClassBody":
		m["body"] = e.nodes(n.BodyList)
	case "EmptyStatement", "DebuggerStatement", "ThisExpression", "Super":
	case "WithStatement":
		m["object"], m["body"] = e.node(n.Object), e.node(n.Body)
	case "ReturnStatement", "ThrowStatement", "SpreadElement", "RestElement", "AwaitExpression":
		m["argument"] = e.node(n.Argument)
	case "LabeledStatement":
		m["label"], m["body"] = e.node(n.Label), e.node(n.Body)
	case "BreakStatement", "ContinueStatement":
		m["label"] = e.node(n.Label)
	case "IfStatement", "ConditionalExpression":
		m["test"], m["consequent"], m["alternate"] = e.node(n.Test), e.node(n.Consequent), e.node(n.Alternate)
	case "SwitchStatement":
		m["discriminant"], m["cases"] = e.node(n.Discriminant), e.nodes(n.Cases)
	case "SwitchCase":
		m["test"], m["consequent"] = e.node(n.Test), e.nodes(n.ConsequentList)
	case "TryStatement":
		m["block"], m["handler"], m["finalizer"] = e.node(n.Block), e.node(n.Handler), e.node(n.Finalizer)
	case "CatchClause":
		m["param"], m["body"] = e.node(n.Param), e.node(n.Body)
	case "WhileStatement", "DoWhileStatement":
		m["test"], m["body"] = e.node(n.Test), e.node(n.Body)
	case "ForStatement":
		m["init"], m["test"], m["update"], m["body"] = e.node(n.Init), e.node(n.Test), e.node(n.Update), e.node(n.Body)
	case "ForInStatement", "ForOfStatement":
		m["left"], m["right"], m["body"] = e.node(n.Left), e.node(n.Right), e.node(n.Body)
		if n.Type == "ForOfStatement" && e.ecmaVersion >= 9 {
			m["await"] = n.Await
		}
	case "FunctionDeclaration", "FunctionExpression", "ArrowFunctionExpression":
		function()
	case "VariableDeclaration":
		m["declarations"], m["kind"] = e.nodes(n.Declarations), n.Kind
	case "VariableDeclarator":
		m["id"], m["init"] = e.node(n.Id), e.node(n.Init)
	case "ClassDeclaration", "ClassExpression":
		m["id"], m["superClass"], m["body"] = e.node(n.Id), e.node(n.SuperClass), e.node(n.Body)
	case "MethodDefinition":
		m["static"], m["computed"], m["key"], m["kind"], m["value"] = n.Static, n.Computed, e.node(n.Key), n.Kind, e.node(n.Value)
	case "ArrayExpression", "ArrayPattern":
		m["elements"] = e.nodes(n.Elements)
	case "ObjectExpression", "ObjectPattern":
		m["properties"] = e.nodes(n.Properties)
	case "Property":
		m["key"], m["value"], m["kind"] = e.node(n.Key), e.node(n.Value), n.Kind
		if e.ecmaVersion >= 6 {
			m["method"], m["shorthand"], m["computed"] = n.Method, n.Shorthand, n.Computed
		}
	case "UnaryExpression", "UpdateExpression":
		m["operator"], m["prefix"], m["argument"] = n.Operator, n.Prefix, e.node(n.Argument)
	case "BinaryExpression", "LogicalExpression", "AssignmentExpression":
		m["left"], m["operator"], m["right"] = e.node(n.Left), n.Operator, e.node(n.Right)
	case "AssignmentPattern":
		m["left"], m["right"] = e.node(n.Left), e.node(n.Right)
	case "CallExpression", "NewExpression":
		m["callee"], m["arguments"] = e.node(n.Callee), e.nodes(n.Arguments)
		if n.Type == "CallExpression" && e.ecmaVersion >= 11 {
			m["optional"] = n.Optional
		}
	case "SequenceExpression":
		m["expressions"] = e.nodes(n.Expressions)
	case "MemberExpression":
		m["object"], m["property"], m["computed"] = e.node(n.Object), e.node(n.Property), n.Computed
		if e.ecmaVersion >= 11 {
			m["optional"] = n.Optional
		}
	case "ChainExpression":
		m["expression"] = e.node(n.Expression)
	case "YieldExpression":
		m["delegate"], m["argument"] = n.Delegate, e.node(n.Argument)
	case "TemplateLiteral":
		m["expressions"], m["quasis"] = e.nodes(n.Expressions), e.nodes(n.Quasis)
	case "TemplateElement":
		var cooked any
		if n.TemplateValue.Cooked != nil {
			cooked = encodeString(*n.TemplateValue.Cooked)
		}
		m["value"], m["tail"] = map[string]any{"raw": encodeString(n.TemplateValue.Raw), "cooked": cooked}, n.Tail
	case "TaggedTemplateExpression":
		m["tag"], m["quasi"] = e.node(n.Tag), e.node(n.Quasi)
	case "MetaProperty":
		m["meta"], m["property"] = e.node(n.Meta), e.node(n.Property)
	case "ImportExpression":
		m["source"] = e.node(n.Source)
	default:
		panic("unexpected node type " + n.Type)
	}
	return m
}
