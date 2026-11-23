package lodash

import (
	"os"
	"testing"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// The fixtures are recorded from Node 24 by testdata/record.mjs. Every case is
// an expression building {args, log}; the result, the arguments after the call
// and the getter log are rendered by testdata/serialize.js, here evaluated in
// the interpreter, so shared, copied and mutated objects compare as well as
// values.

var recordedCalls = map[string]func(ops *jsval.Ops, args []any) (any, error){
	"merge": func(ops *jsval.Ops, a []any) (any, error) { return Merge(ops, js.Arg(a, 0), restOf(a)...) },
	"set":   func(ops *jsval.Ops, a []any) (any, error) { return Set(ops, js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)) },
	"get":   func(ops *jsval.Ops, a []any) (any, error) { return Get(ops, js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)) },
	"has": func(ops *jsval.Ops, a []any) (any, error) {
		return Has(ops, js.Arg(a, 0), js.Arg(a, 1))
	},
	"pick": func(ops *jsval.Ops, a []any) (any, error) { return Pick(ops, js.Arg(a, 0), restOf(a)...) },
	"omit": func(ops *jsval.Ops, a []any) (any, error) { return Omit(ops, js.Arg(a, 0), restOf(a)...) },
	"size": func(ops *jsval.Ops, a []any) (any, error) { return Size(ops, js.Arg(a, 0)) },
}

// restOf is the arguments after the first, as a rest parameter collects them.
func restOf(args []any) []any {
	if len(args) < 2 {
		return nil
	}
	return args[1:]
}

func TestRecorded(t *testing.T) {
	source, err := os.ReadFile("testdata/serialize.js")
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range recordedCalls {
		t.Run(name, func(t *testing.T) {
			cases := jsvaltest.Load(t, "testdata/"+name+".json.gz")
			if len(cases) == 0 {
				t.Fatal("no cases")
			}
			jsvaltest.Replay(t, cases, func(src string) (string, error) {
				// Each case gets a fresh realm: _.set and _.merge may
				// write through to shared prototypes.
				ops := jsvaltest.NewOps(t)
				serialize := jsvaltest.MustEval(t, ops, string(source))
				input, err := jsvaltest.Eval(ops.Realm, src)
				if err != nil {
					return "", err
				}
				args, err := ops.Get(input, "args")
				if err != nil {
					return "", err
				}
				result, err := call(ops, args.(*js.Array).Items)
				if err != nil {
					return "", err
				}
				log, err := ops.Get(input, "log")
				if err != nil {
					return "", err
				}
				out, err := ops.Call(serialize, js.Undefined, []any{&js.Array{Items: []any{result, args, log}}}, "serialize")
				if err != nil {
					return "", err
				}
				return js.ToString(out), nil
			})
		})
	}
}
