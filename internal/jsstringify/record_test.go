package jsstringify

import (
	"testing"
	"time"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
)

// TestRecorded replays testdata/stringify.json.gz, recorded from Node 24 in the
// UTC time zone by testdata/record.mjs: each case is an expression for the value.
func TestRecorded(t *testing.T) {
	saved := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = saved })
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
		return Stringify(ops, v)
	})
}
