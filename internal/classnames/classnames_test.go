package classnames

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// The expected values were printed by html.classNames (classnames 2.5.1 as
// Edge bundles it) on Node 24.
func TestClassNames(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	for _, c := range []struct{ args, want, err string }{
		{args: `["a", { b: true, c: false }, ["d", ["e", 0, null]], 5, { toString() { return "custom" } }]`, want: "a b d e 5 custom"},
		{args: `["", null, undefined, false, true, () => "x"]`, want: ""},
		{args: `[Object.create(null)]`, err: "TypeError: Cannot read properties of undefined (reading 'toString')"},
		{args: `[new String("ab"), new Date(0)]`, want: "0 1"},
		{args: `[{ toString: Object.prototype.toString, k: 1 }, { toString() { return { valueOf() { return 7 } } } }]`, want: "toString k 7"},
	} {
		args := jsvaltest.MustEval(t, ops, c.args).(*js.Array).Items
		got, err := ClassNames(ops, args)
		if c.err != "" {
			if err == nil || jsvaltest.DescribeError(err) != c.err {
				t.Errorf("%s: got %q, %v; want error %s", c.args, got, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.args, got, err, c.want)
		}
	}
}
