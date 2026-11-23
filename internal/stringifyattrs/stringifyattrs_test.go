package stringifyattrs

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
)

// The expected values were printed by stringify-attributes 4.0.0 on Node 24.
func TestStringifyAttributes(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	for _, c := range []struct{ src, want, err string }{
		{src: `{ class: ["a", "b"], disabled: true, hidden: false, title: 'x"y<z>&', n: 0, u: undefined, nul: null, "a&b": "it's" }`,
			want: ` class="a b" disabled title="x&quot;y&lt;z&gt;&amp;" n="0" a&amp;b="it&#39;s"`},
		{src: `{}`, want: ``},
		{src: `{ 2: "two", 1: "one", z: [] }`, want: ` 1="one" 2="two" z=""`},
		{src: `null`, err: "TypeError: Cannot convert undefined or null to object"},
	} {
		got, err := StringifyAttributes(ops, jsvaltest.MustEval(t, ops, c.src))
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
