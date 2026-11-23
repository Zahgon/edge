package prettyhtml

import (
	"testing"
	"time"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// The fixtures are recorded from Node 24 by testdata/record.mjs.

func TestRecordedHTML(t *testing.T) {
	withLocal(t, "UTC")
	ops := jsvaltest.NewOps(t)
	jsvaltest.Replay(t, jsvaltest.Load(t, "testdata/html.json.gz"), func(src string) (string, error) {
		v, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return HTML(ops, v)
	})
}

func TestRecordedDatesInOtherZone(t *testing.T) {
	withLocal(t, "Asia/Kolkata")
	ops := jsvaltest.NewOps(t)
	jsvaltest.Replay(t, jsvaltest.Load(t, "testdata/dates-Asia_Kolkata.json.gz"), func(src string) (string, error) {
		v, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return HTML(ops, v)
	})
}

func TestRecordedInspect(t *testing.T) {
	withLocal(t, "UTC")
	ops := jsvaltest.NewOps(t)
	jsvaltest.Replay(t, jsvaltest.Load(t, "testdata/inspect.json.gz"), func(src string) (string, error) {
		pair, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		items := pair.(*js.Array).Items
		opts, err := optionsOf(ops, items[1])
		if err != nil {
			return "", err
		}
		return Inspect(ops, items[0], opts)
	})
}

// optionsOf reads the options object of a recorded inspect case.
func optionsOf(ops *jsval.Ops, o any) (Options, error) {
	var opts Options
	if own, err := ops.HasOwn(o, "quoteStyle"); err != nil || own {
		q, err := ops.Get(o, "quoteStyle")
		if err != nil {
			return opts, err
		}
		opts.QuoteStyle = js.ToString(q)
	}
	noWrap, err := ops.Get(o, "noWrap")
	if err != nil {
		return opts, err
	}
	opts.NoWrap = js.ToBoolean(noWrap)
	depth, err := ops.Get(o, "depth")
	if err != nil {
		return opts, err
	}
	if n, ok := depth.(float64); ok {
		d := int(n)
		opts.Depth = &d
	}
	return opts, nil
}

// withLocal sets the local time zone dates print in for the test, as TZ sets it
// for Node.
func withLocal(t *testing.T, name string) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s: %v", name, err)
	}
	saved := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = saved })
}

// The tests above must not hide an empty fixture.
func TestFixturesHaveCases(t *testing.T) {
	for _, f := range []string{"html", "inspect", "dates-Asia_Kolkata"} {
		if n := len(jsvaltest.Load(t, "testdata/"+f+".json.gz")); n == 0 {
			t.Errorf("%s: %d cases", f, n)
		}
	}
}
