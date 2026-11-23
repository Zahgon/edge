package edgert_test

// The expected values were recorded with edge.js 6.5.1 on Node 24: each
// global was called as edgeGlobals.<fn>(...args), and the ComponentProps
// expressions ran against an instance of the class $props is in a component.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// globalFunctions maps the paths of Edge's globals to the functions edgert
// builds for them.
func globalFunctions() map[string]*js.Function {
	fns := map[string]*js.Function{
		"html.escape":     edgert.EscapeFunction(),
		"html.safe":       edgert.HTMLSafeFunction(),
		"html.classNames": edgert.ClassNames(),
		"html.attrs":      edgert.Attrs(),
		"js.stringify":    edgert.JSStringify(),
		"nl2br":           edgert.NewNl2br("globals"),
		"inspect":         edgert.NewInspect("globals"),
		"truncate":        edgert.NewTruncate("globals"),
		"excerpt":         edgert.NewExcerpt("globals"),
	}
	for _, g := range edgert.StringHelpers() {
		fns[g.Name] = g.Value.(*js.Function)
	}
	return fns
}

// eval evaluates the JavaScript expression src in the process-wide realm.
func eval(t *testing.T, src string) any {
	t.Helper()
	v, err := jsvaltest.Eval(edgert.Realm(), src)
	if err != nil {
		t.Fatalf("evaluating %s: %v", src, err)
	}
	return v
}

// unwrapSafe returns the value of a SafeValue, and whether v was one.
func unwrapSafe(t *testing.T, v any) (any, bool) {
	t.Helper()
	if !js.IsObject(v) {
		return v, false
	}
	ctor, err := edgert.Ops().Get(v, "constructor")
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := ctor.(*js.Function); !ok || f.Name != "SafeValue" {
		return v, false
	}
	value, err := edgert.Ops().Get(v, "value")
	if err != nil {
		t.Fatal(err)
	}
	return value, true
}

func TestGlobals(t *testing.T) {
	tests := []struct {
		fn   string
		args string // the JavaScript arguments
		want any
		safe bool // the result is a SafeValue holding want
	}{
		{fn: "html.escape", args: `'<a href="x">Tom & \'Jerry\'</a>'`, want: "&lt;a href=&quot;x&quot;&gt;Tom &amp; &#x27;Jerry&#x27;&lt;/a&gt;"},
		{fn: "html.escape", args: "42", want: "42"},
		{fn: "html.escape", args: "null", want: "null"},
		{fn: "html.escape", args: "undefined", want: "undefined"},
		{fn: "html.escape", args: "{ toString() { return '<i>' } }", want: "&lt;i&gt;"},
		{fn: "html.escape", args: "html.safe('<b>')", want: "<b>"},
		{fn: "html.safe", args: "'<b>bold</b>'", want: "<b>bold</b>", safe: true},
		{fn: "nl2br", args: `'a\nb'`, want: "a<br>b"},
		{fn: "nl2br", args: `'<p>x</p>\r\ny\n\rz\r'`, want: "<p>x</p><br>y<br>z<br>"},
		{fn: "nl2br", args: "''", want: js.Undefined},
		{fn: "nl2br", args: "0", want: js.Undefined},
		{fn: "nl2br", args: "12", want: "12"},
		{fn: "truncate", args: "'This is a very long sentence', 10", want: "This is a ..."},
		{fn: "truncate", args: "'This is a very long sentence', 10, { strict: true }", want: "This is a ..."},
		{fn: "truncate", args: "'hello world', 5, { suffix: '!' }", want: "hello!"},
		{fn: "truncate", args: "'short'", want: "short"},
		{fn: "excerpt", args: "'<p>Hello <b>world</b> and everyone else</p>', 12", want: "Hello world ..."},
		{fn: "excerpt", args: "'<p>Hello world</p>', 7, { strict: true }", want: "Hello w..."},
		{fn: "html.classNames", args: "'a', { b: true, c: false }, ['d', ['e', { f: 1 }]], null, 0, 'g'", want: "a b d e f g"},
		{fn: "html.classNames", args: "", want: ""},
		{fn: "html.attrs", args: "{ class: ['a', { b: true }], id: 'x', disabled: true, hidden: false, 'data-n': 3 }", want: `class="a b" id="x" disabled data-n="3"`, safe: true},
		{fn: "html.attrs", args: `{ ariaLabel: 'Close', tabIndex: 0, title: 'a "b" <c> & d' }`, want: `aria-label="Close" title="a "b" <c> & d"`, safe: true},
		{fn: "html.attrs", args: "{ xOn: { click: 'go()' }, x: { data: '{ open: false }' }, style: 'color: red' }", want: `x-on:click="go()" x-data="{ open: false }" style="color: red"`, safe: true},
		{fn: "html.attrs", args: "{ value: null, checked: undefined, autofocus: 'autofocus' }", want: `autofocus="autofocus"`, safe: true},
		{fn: "js.stringify", args: "'</script><b>'", want: `"\u003C\u002Fscript\u003E\u003Cb\u003E"`},
		{fn: "js.stringify", args: "{ a: [1, 'x'], b: null, c: undefined }", want: `{"a":[1,"x"],"b":null}`},
		{fn: "js.stringify", args: "new Date(0)", want: `new Date("1970-01-01T00:00:00.000Z")`},
		{fn: "js.stringify", args: "undefined", want: "undefined"},
		{fn: "js.stringify", args: `'\u2028\u2029'`, want: `"\u2028\u2029"`},
		{fn: "camelCase", args: "'hello_world-foo bar'", want: "helloworldfooBar"},
		{fn: "snakeCase", args: "'helloWorld FooBar'", want: "helloworld_foobar"},
		{fn: "dashCase", args: "'Hello World'", want: "hello-world"},
		{fn: "dashCase", args: "'helloWorld', { capitalize: true }", want: "Hello-World"},
		{fn: "pascalCase", args: "'user_profile-page'", want: "UserProfilePage"},
		{fn: "capitalCase", args: "'hello-world'", want: "Hello-World"},
		{fn: "sentenceCase", args: "'hello_world foo'", want: "Hello world foo"},
		{fn: "dotCase", args: "'helloWorld'", want: "hello.World"},
		{fn: "noCase", args: "'helloWorld_foo'", want: "hello world foo"},
		{fn: "titleCase", args: "'the lord of the rings'", want: "The Lord of the Rings"},
		{fn: "pluralize", args: "'box'", want: "boxes"},
		{fn: "pluralize", args: "'person', 2", want: "people"},
		{fn: "pluralize", args: "'apple', 1, true", want: "1 apple"},
		{fn: "sentence", args: "['a', 'b', 'c']", want: "a, b, and c"},
		{fn: "sentence", args: "['a', 'b'], { pairSeparator: ' or ' }", want: "a or b"},
		{fn: "prettyMs", args: "90061000", want: "1d"},
		{fn: "prettyMs", args: "1500, true", want: "2 seconds"},
		{fn: "toMs", args: "'1h'", want: 3600000.0},
		{fn: "toMs", args: "'2 days'", want: 172800000.0},
		{fn: "prettyBytes", args: "1536", want: "1.5KB"},
		{fn: "prettyBytes", args: "1048576, { unitSeparator: ' ' }", want: "1 MB"},
		{fn: "toBytes", args: "'1KB'", want: 1024.0},
		{fn: "ordinal", args: "22", want: "22nd"},
		{fn: "ordinal", args: "11", want: "11th"},
	}
	fns := globalFunctions()
	html := js.ObjectOf("safe", fns["html.safe"])
	for _, tt := range tests {
		t.Run(tt.fn+"("+tt.args+")", func(t *testing.T) {
			// The arguments may call html.safe, passed in as `html`.
			mk := eval(t, "(html) => ["+tt.args+"]").(*js.Function)
			args, err := edgert.Realm().Call(mk, js.Undefined, []any{html})
			if err != nil {
				t.Fatal(err)
			}
			got, err := edgert.Realm().Call(fns[tt.fn], js.Undefined, args.(*js.Array).Items)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			value, safe := unwrapSafe(t, got)
			if value != tt.want || safe != tt.safe {
				t.Errorf("got %#v (safe %v), want %#v (safe %v)", value, safe, tt.want, tt.safe)
			}
		})
	}
}

func TestInspect(t *testing.T) {
	got, err := edgert.Realm().Call(edgert.NewInspect("globals"), js.Undefined, []any{"hi"})
	if err != nil {
		t.Fatal(err)
	}
	value, safe := unwrapSafe(t, got)
	want := "<pre style=\"\n    padding: 30px 25px;\n    background-color: rgb(6, 21, 38);\n    color: rgb(214, 222, 235);\n" +
		"    border-radius: 6px;\n    font-size: 14px;\n    overflow: auto;\n    white-space: pre;\n    word-spacing: normal;\n" +
		"    word-break: normal;\n    word-wrap: normal;\n    tab-size: 4;\n    line-height: 1.4;\n    text-align: left;\n  \">" +
		"<code style=\"font-family: JetBrains Mono, Menlo, Monaco, monospace;\"><span style=\"color: rgb(173, 219, 103);\">'hi'</span></code></pre>"
	if !safe || value != want {
		t.Errorf("inspect('hi') = %#v (safe %v), want %q", value, safe, want)
	}
}

func TestComponentProps(t *testing.T) {
	props, err := edgert.NewComponentProps(eval(t, `{ a: 1, title: 'T "q"', class: ['x', 'y'], nested: { k: 'v' } }`))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		expr string // evaluated with p bound to the props
		want string // String(expr)
	}{
		{"JSON.stringify(p.all())", `{"a":1,"title":"T \"q\"","class":["x","y"],"nested":{"k":"v"}}`},
		{"p.has('a')", "true"},
		{"p.has('z')", "false"},
		{"p.get('nested.k')", "v"},
		{"p.get('z', 'fallback')", "fallback"},
		{"p.only(['a', 'title']).toAttrs().value", `a="1" title="T "q""`},
		{"p.except(['class', 'nested']).toAttrs().value", `a="1" title="T "q""`},
		{"p.merge({ class: ['y', 'z'], id: 'i' }).toAttrs().value", `class="y z x" id="i" a="1" title="T "q"" nested="[object Object]"`},
		// A class that is not an array: Edge adds the defaults object itself
		// to the class list, and classnames lists its truthy keys.
		{"p.merge({ class: 'm' }).toAttrs().value", `class="class x y" a="1" title="T "q"" nested="[object Object]"`},
		{"p.mergeIf(true, { id: 'i' }).toAttrs().value", `id="i" a="1" title="T "q"" class="x y" nested="[object Object]"`},
		{"p.mergeIf(false, { id: 'i' }).toAttrs().value", `a="1" title="T "q"" class="x y" nested="[object Object]"`},
		{"p.mergeUnless(false, { id: 'i' }).toAttrs().value", `id="i" a="1" title="T "q"" class="x y" nested="[object Object]"`},
		{"p.toAttrs().value", `a="1" title="T "q"" class="x y" nested="[object Object]"`},
		{"Object.keys(p).join()", "a,title,class,nested"},
		{"p.except(['a']).constructor.name", "ComponentProps"},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			f := eval(t, "(p) => String("+tt.expr+")").(*js.Function)
			got, err := edgert.Realm().Call(f, js.Undefined, []any{props})
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEdgeError(t *testing.T) {
	e, err := edgert.NewEdgeError("boom", "E_X", eval(t, "({ line: 3, col: 4, filename: 'a.edge' })"))
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Error" || e.Message != "boom" || !edgert.IsEdgeError(e) {
		t.Errorf("Error object %s: %q (EdgeError %v)", e.Name, e.Message, edgert.IsEdgeError(e))
	}
	keys, err := edgert.Ops().Keys(e)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"line", "col", "filename", "code"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("Object.keys = %v, want %v", keys, want)
	}
	want := &edgeerror.EdgeError{Message: "boom", Code: "E_X", Filename: "a.edge", Line: 3.0, Col: 4.0}
	if got := edgert.FromJS(e); !reflect.DeepEqual(got, want) {
		t.Errorf("FromJS = %#v, want %#v", got, want)
	}

	// An EdgeError created by Edge in Go keeps its message value through a
	// round trip into template code and back.
	for _, message := range []any{"boom", 42.0, js.Undefined} {
		ee := &edgeerror.EdgeError{Message: message, Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: 2.0, Col: 0.0}
		back := edgert.ToJS(ee)
		var je *js.Error
		if !errors.As(back, &je) || !edgert.IsEdgeError(je) {
			t.Fatalf("ToJS(%#v) = %#v, want an EdgeError instance", message, back)
		}
		if got := edgert.FromJS(je); !reflect.DeepEqual(got, ee) {
			t.Errorf("round trip of %#v = %#v", message, got)
		}
	}

	// this.message = message keeps a message that is not a string; an
	// undefined message becomes an own enumerable property. The expected values
	// are Node's.
	messages := []struct {
		message  any
		typeOf   string
		keys     []string
		toString string
	}{
		{42.0, "number", []string{"line", "col", "filename", "code"}, "Error: 42"},
		{js.Undefined, "undefined", []string{"line", "col", "filename", "message", "code"}, "Error"},
		{"boom", "string", []string{"line", "col", "filename", "code"}, "Error: boom"},
	}
	for _, tt := range messages {
		e, err := edgert.NewEdgeError(tt.message, "E_X", eval(t, "({ line: 3, col: 4, filename: 'a.edge' })"))
		if err != nil {
			t.Fatal(err)
		}
		message, err := edgert.Realm().Get(e, "message")
		if err != nil {
			t.Fatal(err)
		}
		keys, err := edgert.Ops().Keys(e)
		if err != nil {
			t.Fatal(err)
		}
		text, err := edgert.Realm().ToString(e)
		if err != nil {
			t.Fatal(err)
		}
		if js.TypeOf(message) != tt.typeOf || !reflect.DeepEqual(keys, tt.keys) || text != tt.toString {
			t.Errorf("new EdgeError(%v): typeof message %s, keys %v, String %q; want %s, %v, %q",
				tt.message, js.TypeOf(message), keys, text, tt.typeOf, tt.keys, tt.toString)
		}
	}

	if _, err := edgert.NewEdgeError("m", "E_X", nil); err == nil || err.Error() != "Cannot read properties of null (reading 'line')" {
		t.Errorf("null options: %v", err)
	}
}
