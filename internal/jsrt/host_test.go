package jsrt

import (
	"errors"
	"testing"

	"github.com/edge-js/edge-go/js"
)

// The expected values in this file were printed by Node v24.22.1.

// asyncEach is Edge's asyncEach as a host async function:
//
//	async function asyncEach(collection, iteratee) {
//	  if (Array.isArray(collection)) { for (let [key, value] of collection.entries()) await iteratee(value, key); return }
//	  if (typeof collection === 'string') { let index = 0; for (let value of collection) await iteratee(value, index++); return }
//	  if (collection && typeof collection === 'object') for (let [key, value] of Object.entries(collection)) await iteratee(value, key)
//	}
func asyncEach(r *Realm) *js.Function {
	return r.NewAsyncNative("asyncEach", 2, func(await func(any) (any, error), _ any, args []any) (any, error) {
		collection, iteratee := js.Arg(args, 0), js.Arg(args, 1)
		step := func(value, key any) error {
			v, err := r.CallValue(iteratee, js.Undefined, []any{value, key})
			if err != nil {
				return err
			}
			_, err = await(v)
			return err
		}
		switch c := collection.(type) {
		case *js.Array:
			index := 0.0
			return js.Undefined, r.ForOf(c, func(item any) error {
				index++
				return step(item, index-1)
			})
		case string:
			index := 0.0
			return js.Undefined, r.ForOf(c, func(item any) error {
				index++
				return step(item, index-1)
			})
		}
		if !js.IsObject(collection) || js.TypeOf(collection) != "object" {
			return js.Undefined, nil
		}
		entries, err := r.Entries(collection)
		if err != nil {
			return nil, err
		}
		for _, en := range entries {
			if err := step(en.Value, en.Key); err != nil {
				return nil, err
			}
		}
		return js.Undefined, nil
	})
}

// runAsync compiles body as `new AsyncFunction(params, body)`, calls it with
// args and awaits its promise.
func runAsync(t *testing.T, r *Realm, params, body string, args ...any) (any, error) {
	t.Helper()
	f, err := r.NewFunction(params, body, true)
	if err != nil {
		t.Fatalf("compiling %q: %v", body, err)
	}
	p, err := r.Call(f, js.Undefined, args)
	if err != nil {
		t.Fatalf("calling %q: %v", body, err)
	}
	return r.Await(p)
}

// TestAsyncNativeOrdering checks that a host async function interleaves with
// other promise jobs exactly as the JavaScript async function it stands for.
func TestAsyncNativeOrdering(t *testing.T) {
	const body = `const log = [];
const p = asyncEach(state.items, async (v, k) => { log.push('it' + k + v); await null; log.push('after' + v); });
Promise.resolve().then(() => log.push('t1')).then(() => log.push('t2')).then(() => log.push('t3')).then(() => log.push('t4')).then(() => log.push('t5')).then(() => log.push('t6')).then(() => log.push('t7'));
p.then(() => log.push('done'));
await p;
await null; await null; await null; await null;
return log.join(' ');`
	r := NewRealm(Options{})
	resolved, err := r.PromiseResolve(1.0)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		items any
		want  string
	}{
		{"array", js.NewArray(1.0, 2.0, 3.0), "it01 after1 t1 it12 t2 after2 t3 it23 t4 after3 t5 t6 done t7"},
		{"string", "ab", "it0a aftera t1 it1b t2 afterb t3 t4 done t5 t6 t7"},
		{"object", js.ObjectOf("x", 1, "y", 2), "itx1 after1 t1 ity2 t2 after2 t3 t4 done t5 t6 t7"},
		{"empty array", js.NewArray(), "t1 done t2 t3 t4 t5"},
		{"null", nil, "t1 done t2 t3 t4 t5"},
		{"promise item", js.NewArray("s", resolved), "it0s afters t1 it1[object Promise] t2 after[object Promise] t3 t4 done t5 t6 t7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runAsync(t, r, "asyncEach,state", body, asyncEach(r), js.ObjectOf("items", tt.items))
			if err != nil || got != tt.want {
				t.Errorf("got %v, %v\nwant %q", got, err, tt.want)
			}
		})
	}
	t.Run("sync iteratee", func(t *testing.T) {
		got, err := runAsync(t, r, "asyncEach", `const log = []; const p = asyncEach([1,2], (v) => { log.push('i' + v); });
Promise.resolve().then(() => log.push('t1')).then(() => log.push('t2')).then(() => log.push('t3')).then(() => log.push('t4'));
p.then(() => log.push('done')); await p; await null; await null; await null; return log.join(' ');`, asyncEach(r))
		if want := "i1 i2 t1 t2 done t3 t4"; err != nil || got != want {
			t.Errorf("got %v, %v\nwant %q", got, err, want)
		}
	})
	t.Run("rejection", func(t *testing.T) {
		got, err := runAsync(t, r, "asyncEach", `try { await asyncEach([1,2], (v) => { if (v === 2) throw new TypeError('bad ' + v); }); } catch (e) { return e.name + ': ' + e.message; }`, asyncEach(r))
		if want := "TypeError: bad 2"; err != nil || got != want {
			t.Errorf("got %v, %v\nwant %q", got, err, want)
		}
	})
}

// TestHostFunctions checks what template code observes of host functions.
func TestHostFunctions(t *testing.T) {
	r := NewRealm(Options{})
	each := asyncEach(r)
	upper := NewNative("upper", 1, func(_ any, args []any) (any, error) {
		s, err := r.ToString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return "<" + s + ">", nil
	})
	failing := NewNative("failing", 0, func(any, []any) (any, error) {
		return nil, errors.New("disk full")
	})
	tests := []struct {
		body string
		want string
	}{
		{`return [util.name, util.length, typeof util].join()`, "asyncEach,2,function"},
		{`return Object.prototype.toString.call(util)`, "[object AsyncFunction]"},
		{`return String(util)`, "function asyncEach() { [native code] }"},
		{`return upper.name + upper.length + upper('x')`, "upper1<x>"},
		{`try { failing() } catch (e) { return [e instanceof Error, e.name, e.message].join() }`, "true,Error,disk full"},
		{`return String(util(null, null) instanceof Promise)`, "true"},
		{`return String(this === globalThis) + String(globalThis)`, "true[object global]"},
		{`return String(new Function('"use strict"; return this')())`, "undefined"},
	}
	for _, tt := range tests {
		f, err := r.NewFunction("util,upper,failing", tt.body, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.body, err)
		}
		got, err := r.Call(f, js.Undefined, []any{each, upper, failing})
		if err != nil || got != tt.want {
			t.Errorf("%s = %v, %v; want %q", tt.body, got, err, tt.want)
		}
	}
	if got := r.Inspect(each); got != "[AsyncFunction: asyncEach]" {
		t.Errorf("inspect(asyncEach) = %q", got)
	}
	SetSource(upper, "function upper(value) { return value.toUpperCase() }")
	if got := js.ToString(upper); got != "function upper(value) { return value.toUpperCase() }" {
		t.Errorf("String(upper) = %q", got)
	}
}

// TestFunctionSource checks Function.prototype.toString for the functions the
// Function and AsyncFunction constructors create, as js.ToString shows them.
func TestFunctionSource(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		params, body string
		async        bool
		want         string
	}{
		{"template,state,$context", "return 1", false, "function anonymous(template,state,$context\n) {\nreturn 1\n}"},
		{"template,state,$context", "let out = \"\";\nreturn out;", true, "async function anonymous(template,state,$context\n) {\nlet out = \"\";\nreturn out;\n}"},
		{"", "", false, "function anonymous(\n) {\n\n}"},
	}
	for _, tt := range tests {
		f, err := r.NewFunction(tt.params, tt.body, tt.async)
		if err != nil {
			t.Fatal(err)
		}
		if got := js.ToString(f); got != tt.want {
			t.Errorf("String(f) = %q; want %q", got, tt.want)
		}
		if got := r.Inspect(f); got != map[bool]string{false: "[Function: anonymous]", true: "[AsyncFunction: anonymous]"}[tt.async] {
			t.Errorf("inspect(f) = %q", got)
		}
	}
	if got := js.ToString(r.global.Get("Math").(*js.Object).Get("max")); got != "function max() { [native code] }" {
		t.Errorf("String(Math.max) = %q", got)
	}
}

// edgeErrorClass is edge-error's EdgeError, with its class fields:
//
//	class EdgeError extends Error {
//	  constructor(message, code, options) { super(message); this.message = message; this.code = code; this.line = options.line; this.col = options.col; this.filename = options.filename }
//	  line; col; filename
//	}
func edgeErrorClass(r *Realm) *Class {
	return r.NewClass("EdgeError", 3, r.ErrorClass(), func(this any, args []any) error {
		for _, field := range []string{"line", "col", "filename"} {
			if err := r.DefineValue(this, field, js.Undefined, 0); err != nil {
				return err
			}
		}
		message := js.Arg(args, 0)
		if isUndefined(message) {
			// super(undefined) creates no message; the assignment below creates
			// an enumerable one after the fields.
			if err := r.DefineValue(this, "message", js.Undefined, 0); err != nil {
				return err
			}
		} else if err := r.Set(this, "message", message, true); err != nil {
			return err
		}
		if err := r.Set(this, "code", js.Arg(args, 1), true); err != nil {
			return err
		}
		options := js.Arg(args, 2)
		for _, field := range []string{"line", "col", "filename"} {
			v, err := r.Get(options, field)
			if err != nil {
				return err
			}
			if err := r.Set(this, field, v, true); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestErrorClass checks a host subclass of Error: what JavaScript sees of its
// instances, and that one thrown through JavaScript is recognised in Go.
func TestErrorClass(t *testing.T) {
	r := NewRealm(Options{})
	edgeError := edgeErrorClass(r)
	raise := NewNative("raise", 1, func(_ any, args []any) (any, error) {
		e, err := edgeError.Construct([]any{js.Arg(args, 0), "E_X", js.ObjectOf("line", 3, "col", 0, "filename", "a.edge")})
		if err != nil {
			return nil, err
		}
		return nil, ThrowValue(e)
	})
	tests := []struct {
		body string
		want string
	}{
		{`try { raise('boom') } catch (e) { return JSON.stringify([String(e), e.name, e.message, Object.keys(e), e instanceof Error, e instanceof EdgeError, e.constructor.name, Object.prototype.toString.call(e)]) }`,
			`["Error: boom","Error","boom",["line","col","filename","code"],true,true,"EdgeError","[object Error]"]`},
		{`try { raise({a: 1}) } catch (e) { return JSON.stringify([String(e), typeof e.message, Object.keys(e)]) }`,
			`["Error: [object Object]","object",["line","col","filename","code"]]`},
		{`try { raise(undefined) } catch (e) { return JSON.stringify([String(e), typeof e.message, Object.keys(e)]) }`,
			`["Error","undefined",["line","col","filename","message","code"]]`},
		{`try { EdgeError('x') } catch (e) { return e.message }`, "Class constructor EdgeError cannot be invoked without 'new'"},
		{`return String(Object.getPrototypeOf(EdgeError) === Error) + new EdgeError('m', 'E', {}).message`, "truem"},
	}
	for _, tt := range tests {
		f, err := r.NewFunction("raise,EdgeError", tt.body, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.body, err)
		}
		got, err := r.Call(f, js.Undefined, []any{raise, edgeError.Ctor})
		if err != nil || got != tt.want {
			t.Errorf("%s\n got %v, %v\nwant %s", tt.body, got, err, tt.want)
		}
	}
	// A rethrown instance comes back to Go as the same object.
	f, err := r.NewFunction("raise", `try { raise('boom') } catch (error) { throw error }`, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Call(f, js.Undefined, []any{raise})
	thrown := CaughtValue(err)
	if !edgeError.Is(thrown) {
		t.Fatalf("thrown %v is not an EdgeError", thrown)
	}
	for key, want := range map[string]any{"message": "boom", "code": "E_X", "filename": "a.edge", "line": 3.0, "col": 0.0} {
		if got, err := r.Get(thrown, key); err != nil || got != want {
			t.Errorf("error.%s = %v, %v; want %v", key, got, err, want)
		}
	}
}

// TestClass checks a host class with methods and own enumerable properties, as
// Edge's ComponentProps is one.
func TestClass(t *testing.T) {
	r := NewRealm(Options{})
	props := r.NewClass("ComponentProps", 1, nil, func(this any, args []any) error {
		_, err := r.Invoke(r.global.Get("Object"), "assign", this, js.Arg(args, 0))
		return err
	})
	r.DefineMethod(props.Proto, "has", 1, func(this any, args []any) (any, error) {
		return r.HasOwn(this, js.ToString(js.Arg(args, 0)))
	})
	p, err := props.Construct([]any{js.ObjectOf("title", "x", "n", 1)})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Inspect(p); got != "ComponentProps { title: 'x', n: 1 }" {
		t.Errorf("inspect(props) = %q", got)
	}
	f, err := r.NewFunction("$props", `return [$props.has('title'), $props.has('x'), $props.constructor.name, $props instanceof Object, Object.keys($props)].join()`, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := r.Call(f, js.Undefined, []any{p}); err != nil || got != "true,false,ComponentProps,true,title,n" {
		t.Errorf("got %v, %v", got, err)
	}
	if got := r.Inspect(NullProto()); got != "[Object: null prototype] {}" {
		t.Errorf("inspect(Object.create(null)) = %q", got)
	}
}

// TestHostOperations checks the property operations against V8's results and
// errors.
func TestHostOperations(t *testing.T) {
	r := NewRealm(Options{})
	obj := js.ObjectOf("b", 1, "2", "x", "a", 3, "1", "y")
	entries, err := r.Entries(obj)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{"1", "y"}, {"2", "x"}, {"b", 1.0}, {"a", 3.0}}
	if len(entries) != len(want) {
		t.Fatalf("Object.entries = %v", entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("Object.entries[%d] = %v; want %v", i, entries[i], want[i])
		}
	}
	var items []any
	if err := r.ForOf("a😀b", func(v any) error { items = append(items, v); return nil }); err != nil || len(items) != 3 || items[1] != "😀" {
		t.Errorf("for of 'a😀b' = %v, %v", items, err)
	}
	errorTests := []struct {
		name string
		err  error
		want string
	}{
		{"get undefined", func() error { _, err := r.Get(js.Undefined, "y"); return err }(), "TypeError: Cannot read properties of undefined (reading 'y')"},
		{"set null", r.Set(nil, "y", 1.0, true), "TypeError: Cannot set properties of null (setting 'y')"},
		{"for of number", r.ForOf(1.0, func(any) error { return nil }), "TypeError: number 1 is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{"entries null", func() error { _, err := r.Entries(nil); return err }(), "TypeError: Cannot convert undefined or null to object"},
		{"in string", func() error { _, err := r.Has("s", "x"); return err }(), "TypeError: Cannot use 'in' operator to search for 'x' in s"},
		{"call number", func() error { _, err := r.CallValue(1.0, js.Undefined, nil); return err }(), "TypeError: number 1 is not a function"},
	}
	for _, tt := range errorTests {
		e, ok := tt.err.(*js.Error)
		if !ok || e.Name+": "+e.Message != tt.want {
			t.Errorf("%s: %v; want %s", tt.name, tt.err, tt.want)
		}
	}
}

// TestGlobalDefined checks the globals edge-parser leaves untransformed.
func TestGlobalDefined(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		name string
		want bool
	}{
		{"Math", true}, {"JSON", true}, {"process", true}, {"Buffer", true}, {"setTimeout", true},
		{"fetch", true}, {"globalThis", true}, {"global", true}, {"console", true},
		{"constructor", true}, {"toString", true}, {"hasOwnProperty", true}, {"__proto__", true},
		{"undefined", false}, {"require", false}, {"module", false}, {"__dirname", false},
		{"user", false}, {"username", false}, {"window", false}, {"document", false},
	}
	for _, tt := range tests {
		if got := r.GlobalDefined(tt.name); got != tt.want {
			t.Errorf("GlobalDefined(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
	f, err := r.NewFunction("", `globalThis.user = 1; username = 'x'; globalThis.gone = undefined; delete globalThis.process; delete globalThis.JSON`, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Call(f, js.Undefined, nil); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"user": true, "username": true, "gone": false, "process": false, "JSON": false, "Math": true} {
		if got := r.GlobalDefined(name); got != want {
			t.Errorf("after assignments, GlobalDefined(%q) = %v; want %v", name, got, want)
		}
	}
}

// TestDefault checks that the process-wide realm is one realm.
func TestDefault(t *testing.T) {
	if Default() != Default() {
		t.Fatal("Default returned two realms")
	}
}
