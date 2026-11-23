// Package jsvaltest supports the differential tests of the ported libraries: it
// builds JavaScript input values from source text in a realm, describes results
// and errors the way the Node recorders print them, and loads the recorded cases.
package jsvaltest

import (
	"fmt"
	"testing"

	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// NewOps returns the operations of a fresh realm.
func NewOps(t testing.TB) *jsval.Ops {
	t.Helper()
	ops, err := jsval.New(jsrt.NewRealm(jsrt.Options{}))
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

// Eval evaluates the JavaScript expression src in r, as `(function () { return
// (src) })()` would.
func Eval(r *jsrt.Realm, src string) (any, error) {
	f, err := r.NewFunction("", "return ("+src+"\n)", false)
	if err != nil {
		return nil, err
	}
	return r.Call(f, js.Undefined, nil)
}

// MustEval is Eval failing the test on an error.
func MustEval(t testing.TB, ops *jsval.Ops, src string) any {
	t.Helper()
	v, err := Eval(ops.Realm, src)
	if err != nil {
		t.Fatalf("evaluating %s: %v", src, err)
	}
	return v
}

// DescribeError renders an error as the recorders print a thrown value: "Name:
// message" for an Error object, "thrown: String(value)" for anything else.
func DescribeError(err error) string {
	switch e := err.(type) {
	case *js.Error:
		return e.Name + ": " + e.Message
	case *js.Thrown:
		return "thrown: " + js.ToString(e.Value)
	}
	return "go: " + err.Error()
}

// Case is one recorded case: the source of the input (an expression or a list
// of arguments, as the recorder defines it), and either the result or the thrown
// error as DescribeError renders it.
type Case struct {
	Src string
	Out string
	Err string
}

// Load reads the recorded cases of a gzip-compressed JSON file: an array of
// [src, out] or [src, null, err] items. The realm's JSON.parse reads it, so strings
// with lone surrogates survive.
func Load(t testing.TB, path string) []Case {
	t.Helper()
	data, err := fixture.ReadGzip(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsrt.JSONParse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := doc.(*js.Array)
	if !ok {
		t.Fatalf("%s: not an array", path)
	}
	cases := make([]Case, len(arr.Items))
	for i, item := range arr.Items {
		row, ok := item.(*js.Array)
		if !ok || len(row.Items) < 2 {
			t.Fatalf("%s: item %d is not a case", path, i)
		}
		c := Case{Src: js.ToString(row.Items[0])}
		if s, ok := row.Items[1].(string); ok {
			c.Out = s
		} else if len(row.Items) > 2 {
			c.Err = js.ToString(row.Items[2])
		}
		cases[i] = c
	}
	return cases
}

// Replay checks every case: run returns the result string for a case's source or
// the error it throws. Mismatches are reported, at most maxReported in detail.
func Replay(t *testing.T, cases []Case, run func(src string) (string, error)) {
	t.Helper()
	const maxReported = 20
	failed := 0
	for _, c := range cases {
		out, err := run(c.Src)
		var got, want string
		if err != nil {
			got = "throws " + DescribeError(err)
		} else {
			got = fmt.Sprintf("%q", out)
		}
		if c.Err != "" {
			want = "throws " + c.Err
		} else {
			want = fmt.Sprintf("%q", c.Out)
		}
		if got != want {
			failed++
			if failed <= maxReported {
				t.Errorf("%s\n got: %s\nwant: %s", c.Src, got, want)
			}
		}
	}
	if failed > 0 {
		t.Errorf("%d of %d cases differ", failed, len(cases))
	}
}
