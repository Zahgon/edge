package jsrt

import (
	"bytes"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/js"
)

// asyncCase is one case of testdata/async/cases.json.gz, recorded from Node 24 by
// testdata/async/record.js (see there for the format).
type asyncCase struct {
	Name   string `json:"name"`
	Src    string `json:"src"`
	Sync   bool   `json:"sync"`
	Ret    string `json:"ret"`
	Err    string `json:"err"`
	Stdout string `json:"stdout"`
}

// TestAsyncFixtures replays the async cases: each body is compiled with the
// AsyncFunction constructor (or Function), called in a fresh realm, and its
// promise awaited; the result, the error and the console output must be what
// Node produced.
func TestAsyncFixtures(t *testing.T) {
	var doc struct {
		Cases []asyncCase `json:"cases"`
	}
	if err := fixture.DecodeGzipJSON("testdata/async/cases.json.gz", &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("no cases in testdata/async/cases.json.gz")
	}
	for _, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			r := NewRealm(Options{})
			var stdout bytes.Buffer
			r.SetConsole(&stdout, &stdout)
			ret, errText := runAsyncCase(r, c)
			got := [3]string{ret, errText, stdout.String()}
			want := [3]string{c.Ret, c.Err, c.Stdout}
			if got != want {
				t.Errorf("%s\n got: %q\nwant: %q", c.Src, got, want)
			}
		})
	}
}

// runAsyncCase compiles, calls and awaits one case, returning what record.js
// records: the result and the error text.
func runAsyncCase(r *Realm, c asyncCase) (ret, errText string) {
	fn, err := r.NewFunction("", c.Src, !c.Sync)
	if err != nil {
		return "", nodeErrorText(r, caughtValue(err))
	}
	v, err := r.Call(fn, js.Undefined, nil)
	if err == nil && !c.Sync {
		v, err = r.Await(v)
	}
	if err != nil {
		return "", nodeErrorText(r, caughtValue(err))
	}
	if s, ok := v.(string); ok {
		return s, ""
	}
	return "(" + js.TypeOf(v) + ") " + safeDescribe(r, v), ""
}

// evalAsync compiles src as the body of an async function, calls it and returns
// the promise.
func evalAsync(t *testing.T, r *Realm, src string) any {
	t.Helper()
	fn, err := r.NewFunction("", src, true)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.Call(fn, js.Undefined, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAwaitAbandonsSuspended checks that async functions left waiting for a
// promise nothing can settle do not keep their goroutines once Await returns.
func TestAwaitAbandonsSuspended(t *testing.T) {
	before := runtime.NumGoroutine()
	r := NewRealm(Options{})
	p := evalAsync(t, r, `
		var forever = new Promise(function () {});
		async function wait(n) { try { await forever; } finally { log.push('finally ' + n); } }
		var log = [];
		for (var i = 0; i < 20; i++) wait(i);
		globalThis.log = log;
		return 'returned';`)
	v, err := r.Await(p)
	if err != nil || v != "returned" {
		t.Fatalf("Await = %v, %v", v, err)
	}
	if n := len(r.async.suspended); n != 0 {
		t.Errorf("%d coroutines still suspended", n)
	}
	// An abandoned body runs no more JavaScript, not even its finally blocks.
	if log, _ := r.Global().Get("log").(*js.Array); log == nil || len(log.Items) != 0 {
		t.Errorf("log = %v", r.Global().Get("log"))
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines before, %d after", before, runtime.NumGoroutine())
		}
		time.Sleep(time.Millisecond)
	}
}

// TestAwaitNeverSettled checks the error for a promise that stays pending.
func TestAwaitNeverSettled(t *testing.T) {
	r := NewRealm(Options{})
	p := evalAsync(t, r, `await new Promise(function () {}); return 'unreachable';`)
	if _, err := r.Await(p); !errors.Is(err, ErrNeverSettled) {
		t.Fatalf("Await = %v, want ErrNeverSettled", err)
	}
}

// TestAwaitRunsJobsForPlainValues checks that awaiting a value that is not a
// promise still runs the queued jobs, as await does in Node.
func TestAwaitRunsJobsForPlainValues(t *testing.T) {
	r := NewRealm(Options{})
	fn, err := r.NewFunction("", `var o = {done: false}; Promise.resolve().then(function () { o.done = true; }); return o;`, false)
	if err != nil {
		t.Fatal(err)
	}
	o, err := r.Call(fn, js.Undefined, nil)
	if err != nil {
		t.Fatal(err)
	}
	if done, _ := r.getOwn(o, "done"); done != false {
		t.Fatalf("job ran before Await: done = %v", done)
	}
	v, err := r.Await(o)
	if err != nil || v != o {
		t.Fatalf("Await = %v, %v", v, err)
	}
	if done, _ := r.getOwn(o, "done"); done != true {
		t.Errorf("job did not run: done = %v", done)
	}
}

// TestCallDepthAcrossAwait checks that the call depth follows the JavaScript
// stack across suspensions: a body resumed by a job does not keep the frames of
// its original caller, and the depth is back to zero when everything has run.
func TestCallDepthAcrossAwait(t *testing.T) {
	r := NewRealm(Options{MaxCallDepth: 50})
	p := evalAsync(t, r, `
		async function down(n) { if (n === 0) return 0; await null; return 1 + await down(n - 1); }
		var deep = await down(200);
		function sync(n) { return n === 0 ? 0 : 1 + sync(n - 1); }
		try { sync(200); return 'no overflow'; } catch (e) { return deep + ' ' + e.message; }`)
	v, err := r.Await(p)
	if err != nil || v != "200 Maximum call stack size exceeded" {
		t.Fatalf("Await = %v, %v", v, err)
	}
	if r.depth != 0 {
		t.Errorf("depth after Await = %d", r.depth)
	}
}

// TestAwaitAcrossCalls checks that a realm's promises survive between Await
// calls: a promise settled by a later script resumes the function awaiting it.
func TestAwaitAcrossCalls(t *testing.T) {
	r := NewRealm(Options{})
	setup := evalAsync(t, r, `
		globalThis.gate = {};
		globalThis.gate.promise = new Promise(function (resolve) { globalThis.gate.open = resolve; });
		return 'ready';`)
	if v, err := r.Await(setup); err != nil || v != "ready" {
		t.Fatalf("setup = %v, %v", v, err)
	}
	p := evalAsync(t, r, `gate.open('opened'); return 'got ' + await gate.promise;`)
	if v, err := r.Await(p); err != nil || v != "got opened" {
		t.Fatalf("Await = %v, %v", v, err)
	}
}
