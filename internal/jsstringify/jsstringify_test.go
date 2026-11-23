package jsstringify

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
)

// The expected values were printed by js.stringify (js-stringify 1.0.2 as Edge
// bundles it) on Node 24.
func TestStringify(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	for _, c := range []struct{ src, want, err string }{
		{src: `undefined`, want: `undefined`},
		{src: `new Date(0)`, want: `new Date("1970-01-01T00:00:00.000Z")`},
		{src: `"</script>"`, want: `"\u003C\u002Fscript\u003E"`},
		{src: `"\u2028x\u2029"`, want: `"\u2028x\u2029"`},
		{src: `{ a: [1, "/"], b: null, c: undefined }`, want: `{"a":[1,"\u002F"],"b":null}`},
		{src: `function () {}`, err: "TypeError: Cannot read properties of undefined (reading 'replace')"},
		{src: `new Date(NaN)`, err: "RangeError: Invalid time value"},
		{src: `Object.create(Date.prototype)`, err: "TypeError: Method Date.prototype.toISOString called on incompatible receiver [object Object]"},
	} {
		got, err := Stringify(ops, jsvaltest.MustEval(t, ops, c.src))
		if c.err != "" {
			if err == nil || jsvaltest.DescribeError(err) != c.err {
				t.Errorf("%s: got %q, %v; want error %s", c.src, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.src, got, err, c.want)
		}
	}
}
