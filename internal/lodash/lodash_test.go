package lodash

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// The expected values were printed (as JSON) by @poppinss/utils 7.1.2's lodash
// build on Node 24.
func TestFunctions(t *testing.T) {
	for _, c := range []struct {
		name, args, want string
		call             func(ops *jsval.Ops, a []any) (any, error)
	}{
		{"get", `[{ a: [{ b: { c: 3 } }] }, "a[0].b.c"]`, `3`, recordedCalls["get"]},
		{"get default", `[{ a: 1 }, "x.y", "dflt"]`, `"dflt"`, recordedCalls["get"]},
		{"get array path", `[{ "x.y": 1 }, ["x.y"]]`, `1`, recordedCalls["get"]},
		{"set", `[{}, "a[0].b", 1]`, `{"a":[{"b":1}]}`, recordedCalls["set"]},
		{"set existing", `[{ a: { z: 1 } }, "a.b", 2]`, `{"a":{"z":1,"b":2}}`, recordedCalls["set"]},
		{"set primitive", `[5, "a", 1]`, `5`, recordedCalls["set"]},
		{"has", `[{ a: { b: 1 } }, "a.b"]`, `true`, recordedCalls["has"]},
		{"has missing", `[{ a: { b: 1 } }, "a.c"]`, `false`, recordedCalls["has"]},
		{"has inherited", `[Object.create({ x: 1 }), "x"]`, `false`, recordedCalls["has"]},
		{"has index", `[[1, 2], "1"]`, `true`, recordedCalls["has"]},
		{"pick", `[{ a: 1, b: { c: 2, d: 3 }, e: 4 }, ["a", "b.c"], "z"]`, `{"a":1,"b":{"c":2}}`, recordedCalls["pick"]},
		{"omit", `[{ a: 1, b: { c: 2, d: 3 }, e: 4 }, "a", "b.c"]`, `{"b":{"d":3},"e":4}`, recordedCalls["omit"]},
		{"merge", `[{ a: [1, { x: 1 }], b: { c: 1 } }, { a: [9, { y: 2 }], b: { d: 2 }, e: undefined }, { b: { c: undefined } }]`,
			`{"a":[9,{"x":1,"y":2}],"b":{"c":1,"d":2}}`, recordedCalls["merge"]},
		{"size string", `["abc"]`, `3`, recordedCalls["size"]},
		{"size astral", `["😀👍🏽"]`, `2`, recordedCalls["size"]},
		{"size flag", `["🇺🇸"]`, `1`, recordedCalls["size"]},
		{"size object", `[{ a: 1, b: 2 }]`, `2`, recordedCalls["size"]},
		{"size map", `[new Map([[1, 2]])]`, `1`, recordedCalls["size"]},
		{"size null", `[null]`, `0`, recordedCalls["size"]},
	} {
		ops := jsvaltest.NewOps(t)
		args := jsvaltest.MustEval(t, ops, c.args).(*js.Array).Items
		res, err := c.call(ops, args)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		got, _, err := ops.Realm.JSONStringify(res, js.Undefined, js.Undefined)
		if err != nil || got != c.want {
			t.Errorf("%s: got %s, %v; want %s", c.name, got, err, c.want)
		}
	}
}

// Merge keeps objects that are not plain (a Date here) by reference.
func TestMergeKeepsDate(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	d := jsvaltest.MustEval(t, ops, `new Date(0)`)
	res, err := Merge(ops, js.NewObject(), js.ObjectOf("d", d))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(*js.Object).Get("d"); got != d {
		t.Errorf("merged d = %v, want the source's Date", got)
	}
}
