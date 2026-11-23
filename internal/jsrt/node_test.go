package jsrt

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/internal/jsdate"
	"github.com/edge-js/edge-go/js"
)

// TestNodeFixtures replays testdata/*.json.gz, recorded from Node 24 by
// testdata/record.js (see there for the format): every case body is compiled
// with the Function constructor and called in one realm per fixture, and its
// return value, error and console output must be what Node produced.
func TestNodeFixtures(t *testing.T) {
	files, err := filepath.Glob("testdata/*.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures in testdata")
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".json.gz"), func(t *testing.T) {
			runNodeFixture(t, file)
		})
	}
}

// maxReported bounds the differing cases reported in detail per fixture.
const maxReported = 20

func runNodeFixture(t *testing.T, file string) {
	data, err := fixture.ReadGzip(file)
	if err != nil {
		t.Fatal(err)
	}
	// The fixtures hold strings with lone surrogates, which only the realm's
	// JSON.parse keeps as they are.
	doc, err := JSONParse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	// A fixed clock: the cases may construct dates but never print the time.
	r := NewRealm(Options{Now: func() float64 { return 0 }})
	var stdout, stderr bytes.Buffer
	r.SetConsole(&stdout, &stderr)
	helpers := map[string]any{}
	if hs, ok := js.GetOwn(doc, "helpers"); ok {
		for _, name := range js.OwnKeys(hs) {
			src, _ := js.GetOwn(hs, name)
			f, err := r.NewFunction("", "return ("+js.ToString(src)+")", false)
			if err != nil {
				t.Fatalf("helper %s: %v", name, err)
			}
			if helpers[name], err = r.Call(f, js.Undefined, nil); err != nil {
				t.Fatalf("helper %s: %v", name, err)
			}
		}
	}
	defaultLocals, _ := js.GetOwn(doc, "locals")
	cases, _ := js.GetOwn(doc, "cases")
	failures := 0
	for _, c := range cases.(*js.Array).Items {
		field := func(key string) string {
			v, _ := js.GetOwn(c, key)
			if s, ok := v.(string); ok {
				return s
			}
			return ""
		}
		stdout.Reset()
		stderr.Reset()
		ret, errText := runNodeCase(r, c, defaultLocals, helpers)
		got := [4]string{ret, errText, stdout.String(), stderr.String()}
		want := [4]string{field("ret"), field("err"), field("stdout"), field("stderr")}
		if got != want {
			failures++
			if failures <= maxReported {
				t.Errorf("%s\n got: %q\nwant: %q", field("name"), got, want)
			}
		}
	}
	if failures > maxReported {
		t.Errorf("%d of %d cases differ from Node", failures, len(cases.(*js.Array).Items))
	}
}

// runNodeCase compiles and calls one case, returning what record.js records:
// the return value and the error text.
func runNodeCase(r *Realm, c, defaultLocals any, helpers map[string]any) (ret, errText string) {
	src, _ := js.GetOwn(c, "src")
	fn, err := r.NewFunction("locals, escapeFn, include, rethrow", src.(string), false)
	if err != nil {
		return "", nodeErrorText(r, caughtValue(err))
	}
	locals := NullProto()
	data, ok := js.GetOwn(c, "locals")
	if !ok {
		data, ok = defaultLocals, defaultLocals != nil
	}
	if ok {
		decoded, err := decodeFixtureValue(r, data)
		if err != nil {
			return "", "decoding locals: " + err.Error()
		}
		for _, k := range r.ownKeys(decoded) {
			v, _ := r.getOwn(decoded, k)
			locals.Own.Set(k, v)
		}
	}
	var this any = js.Undefined
	if v, ok := js.GetOwn(c, "this"); ok {
		if this, err = decodeFixtureValue(r, v); err != nil {
			return "", "decoding this: " + err.Error()
		}
	}
	v, err := r.Call(fn, this, []any{locals, helpers["escapeFn"], helpers["include"], helpers["rethrow"]})
	if err != nil {
		return "", nodeErrorText(r, caughtValue(err))
	}
	if s, ok := v.(string); ok {
		return s, ""
	}
	return "(" + js.TypeOf(v) + ") " + safeDescribe(r, v), ""
}

// nodeErrorText is record.js's errorText.
func nodeErrorText(r *Realm, v any) string {
	if e, ok := v.(*js.Error); ok {
		name, _ := r.getV(e, "name")
		msg, _ := r.getV(e, "message")
		return safeDescribe(r, name) + ": " + safeDescribe(r, msg)
	}
	return "thrown " + js.TypeOf(v) + " " + safeDescribe(r, v)
}

// safeDescribe is String(v), or "<String failed>" when that throws.
func safeDescribe(r *Realm, v any) string {
	s, err := r.toString(v)
	if err != nil {
		return "<String failed>"
	}
	return s
}

// decodeFixtureValue turns the JSON of a fixture value into the value it stands
// for, resolving the tagged objects record.js documents.
func decodeFixtureValue(r *Realm, v any) (any, error) {
	switch x := v.(type) {
	case *js.Array:
		out := make([]any, len(x.Items))
		for i, it := range x.Items {
			d, err := decodeFixtureValue(r, it)
			if err != nil {
				return nil, err
			}
			out[i] = d
		}
		return js.NewArray(out...), nil
	case *js.Object:
		switch {
		case x.Has("$undef"):
			return js.Undefined, nil
		case x.Has("$nan"):
			return math.NaN(), nil
		case x.Has("$inf"):
			return math.Inf(int(js.ToNumber(x.Get("$inf")))), nil
		case x.Has("$negzero"):
			return math.Copysign(0, -1), nil
		case x.Has("$date"):
			ms, err := decodeFixtureValue(r, x.Get("$date"))
			if err != nil {
				return nil, err
			}
			return &js.Date{Ms: jsdate.TimeClip(js.ToNumber(ms))}, nil
		case x.Has("$fn"):
			var params []string
			for _, p := range x.Get("params").(*js.Array).Items {
				params = append(params, js.ToString(p))
			}
			return r.NewFunction(strings.Join(params, ","), js.ToString(x.Get("$fn")), false)
		case x.Has("$re"):
			return r.newRegExp(js.ToString(x.Get("$re")), js.ToString(x.Get("flags")))
		}
		o := js.NewObject()
		for _, k := range x.Keys() {
			d, err := decodeFixtureValue(r, x.Get(k))
			if err != nil {
				return nil, err
			}
			o.Set(k, d)
		}
		return o, nil
	}
	return v, nil
}
