package escapegoat

import (
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// TestRecorded replays testdata/escape.json.gz, recorded from Node 24 by
// testdata/record.mjs.
func TestRecorded(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	cases := jsvaltest.Load(t, "testdata/escape.json.gz")
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	jsvaltest.Replay(t, cases, func(src string) (string, error) {
		s, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return HTMLEscape(js.ToString(s)), nil
	})
}
