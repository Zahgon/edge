package stringifyattrs

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
)

// TestRecorded replays testdata/stringify.json.gz, recorded from Node 24 by
// testdata/record.mjs: each case is an expression for the attributes object.
func TestRecorded(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	cases := jsvaltest.Load(t, "testdata/stringify.json.gz")
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	jsvaltest.Replay(t, cases, func(src string) (string, error) {
		v, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return StringifyAttributes(ops, v)
	})
}
