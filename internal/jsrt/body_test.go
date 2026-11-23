package jsrt

import (
	"strings"
	"testing"

	"github.com/edge-js/edge-go/js"
)

// bodyCase is a JavaScript function body and what running it prints.
type bodyCase struct {
	src, want string
}

// runBody runs src as the body of an async function in a new realm, awaits
// its promise, and returns what the tests compare: a string result as it is,
// another value as "(typeof) String(value)", an exception (a SyntaxError of
// the body included) as "throw Name: message". With the prefix "inspect:" it
// is util.inspect of the result, with "log:" what console.log printed.
func runBody(src string) string {
	r := NewRealm(Options{})
	var logs strings.Builder
	r.SetConsole(&logs, &logs)
	mode := ""
	for _, m := range []string{"inspect:", "log:"} {
		if rest, ok := strings.CutPrefix(src, m); ok {
			mode, src = m, rest
		}
	}
	fn, err := r.NewFunction("", src, true)
	if err != nil {
		return "throw " + thrownText(r, err)
	}
	v, err := r.Call(fn, js.Undefined, nil)
	if err == nil {
		v, err = r.Await(v)
	}
	if err != nil {
		return "throw " + thrownText(r, err)
	}
	switch mode {
	case "inspect:":
		return r.Inspect(v)
	case "log:":
		return strings.TrimSuffix(logs.String(), "\n")
	}
	if s, ok := v.(string); ok {
		return s
	}
	s, err := r.ToString(v)
	if err != nil {
		return "throw " + thrownText(r, err)
	}
	return "(" + js.TypeOf(v) + ") " + s
}

// thrownText is "Name: message" for a thrown Error object, String(value)
// for any other thrown value.
func thrownText(r *Realm, err error) string {
	if e, ok := CaughtValue(err).(*js.Error); ok {
		return e.Name + ": " + e.Message
	}
	s, serr := r.ToString(CaughtValue(err))
	if serr != nil {
		return "(unprintable thrown value)"
	}
	return s
}

// runBodies checks each case with runBody.
func runBodies(t *testing.T, cases []bodyCase) {
	t.Helper()
	for _, c := range cases {
		if got := runBody(c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
