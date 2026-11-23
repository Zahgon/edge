package prettyhtml

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
)

func depth(n int) *int { return &n }

// The expected values below were printed by @poppinss/inspect 1.0.1 on Node 24
// in the UTC time zone.

func TestInspect(t *testing.T) {
	withLocal(t, "UTC")
	ops := jsvaltest.NewOps(t)
	for _, c := range []struct {
		src  string
		opts Options
		want string
		err  string
	}{
		{src: `undefined`, want: `"undefined"`},
		{src: `-0`, want: `-0`},
		{src: `'it\'s \\ \n\u0001'`, want: `"'it\'s \\ \n\x01'"`},
		{src: `'q'`, opts: Options{QuoteStyle: "double"}, want: `""q""`},
		{src: `'q'`, opts: Options{QuoteStyle: "x"}, err: `TypeError: option "quoteStyle" must be "single" or "double"`},
		{src: `[1, "a", [true, null]]`, want: "[\n  1,\n  \"'a'\",\n  [\n    true,\n    null\n  ]\n]"},
		{src: `{ a: 1, "b c": [], "0": {} }`, want: "{\n  \"0\": {},\n  \"a\": 1,\n  \"'b c'\": []\n}"},
		{src: `new Map([["k", { a: 1 }], [1, "v"]])`, want: "\"Map(2) ['k' => {\n  \"a\": 1\n}, 1 => 'v']\""},
		{src: `new Set([1, "s", [2]])`, want: "\"Set(3) [1, \"'s'\", [\n  2\n]]\""},
		{src: `new WeakMap()`, want: `"WeakMap {?}"`},
		{src: `(function () { var o = { n: 1 }; o.me = o; return o })()`, want: "{\n  \"n\": 1,\n  \"me\": \"[Circular]\"\n}"},
		{src: `{ a: { b: { c: 1 } } }`, opts: Options{Depth: depth(2)}, want: "{\n  \"a\": {\n    \"b\": \"[Object]\"\n  }\n}"},
		{src: `{ a: { b: 1 } }`, opts: Options{Depth: depth(1), NoWrap: true}, want: "{\n  \"a\": [Object]\n}"},
		{src: `function foo() {}`, want: `"[Function: foo]"`},
		{src: `() => 1`, want: `"[Function]"`},
		{src: `Object(5)`, want: `"Object(5)"`},
		{src: `Object("x")`, want: `"Object("'x'")"`},
		{src: `new Boolean(false)`, want: `"Object(false)"`},
		{src: `/a\/b/g`, want: `"RegExp /a\/b/g"`},
		{src: `new Date(0)`, want: `"'Thu Jan 01 1970 00:00:00 GMT+0000 (Coordinated Universal Time)'"`},
		{src: `new Date(0)`, opts: Options{NoWrap: true}, want: `Thu Jan 01 1970 00:00:00 GMT+0000 (Coordinated Universal Time)`},
		{src: `(function () { var e = new TypeError("bad"); e.code = 7; return e })()`, want: "\"Error [TypeError: bad]\"{\n  \"code\": 7\n}"},
		// Keys that read as numbers below the length are taken for indexes.
		{src: `Object.assign([1, 2], { "-1": "x", "1.5": "y", extra: true })`, want: "[\n  1,\n  2,\n  \"extra\": true\n]"},
		{src: `({ nodeName: "DIV", getAttribute: function () {}, attributes: [{ name: "id", value: "a\"b" }], childNodes: [1] })`, want: `"<div id="a&quot;b">...</div>"`},
		{src: `({ get boom() { throw new Error("getter") } })`, err: "Error: getter"},
	} {
		got, err := Inspect(ops, jsvaltest.MustEval(t, ops, c.src), c.opts)
		if c.err != "" {
			if err == nil || jsvaltest.DescribeError(err) != c.err {
				t.Errorf("%s: got %q, %v; want error %s", c.src, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s:\n got %q, %v\nwant %q", c.src, got, err, c.want)
		}
	}
}

func TestHTML(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	const head = `<pre style="` + stylePre + `"><code style="` + styleCode + `">`
	const tail = `</code></pre>`
	for _, c := range []struct{ src, want string }{
		{`null`, `<span style="color: rgb(255, 203, 139);">null</span>`},
		{`"a<b>&\"c"`, `<span style="color: rgb(173, 219, 103);">'a&lt;b&gt;&amp;"c'</span>`},
		{`{ "k": [1, "x"], s: new Set(["a"]) }`, "{\n" +
			`  <span style="color: rgb(127, 219, 202);">"k"</span>: [` + "\n" +
			`    <span style="color: rgb(199, 146, 234);">1</span>,` + "\n" +
			`    <span style="color: rgb(173, 219, 103);">'x'</span>` + "\n" +
			"  ],\n" +
			`  <span style="color: rgb(127, 219, 202);">"s"</span>: <span style="color: #999;">Set(1)</span> <span style="color: rgb(255, 203, 139);">["'a'"]</span>` + "\n" +
			"}"},
		{`new Map([[1, 2]])`, `<span style="color: #999;">Map(1)</span> <span style="color: rgb(255, 203, 139);">[1 => 2]</span>`},
		{`new Error("e")`, `<span style="color: rgb(255, 86, 86);">[Error: e]</span>`},
		{`function f() {}`, `<span style="color: rgb(255, 203, 139);">[Function: f]</span>`},
		{`undefined`, `<span style="color: #999;">undefined</span>`},
	} {
		got, err := HTML(ops, jsvaltest.MustEval(t, ops, c.src))
		if want := head + c.want + tail; err != nil || got != want {
			t.Errorf("%s:\n got %q, %v\nwant %q", c.src, got, err, want)
		}
	}
}
