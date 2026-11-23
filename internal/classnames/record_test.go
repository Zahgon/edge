package classnames

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// TestRecorded replays testdata/classnames.json.gz, recorded from Node 24 by
// testdata/record.mjs: each case is an argument list for classNames.
func TestRecorded(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	cases := jsvaltest.Load(t, "testdata/classnames.json.gz")
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	jsvaltest.Replay(t, cases, func(src string) (string, error) {
		args, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return ClassNames(ops, args.(*js.Array).Items)
	})
}
