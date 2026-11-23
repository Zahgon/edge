package jsrt

import (
	"testing"

	"github.com/edge-js/edge-go/js"
)

// TestArrayAndStringAdditions checks Array.prototype.copyWithin and the
// well-formed string methods. The expected values were printed by Node
// v24.22.1.
func TestArrayAndStringAdditions(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		src, want string
	}{
		{`[1, 2, 3, 4, 5].copyWithin(0, 3)`, `[4,5,3,4,5]`},
		{`[1, 2, 3, 4, 5].copyWithin(1, 0)`, `[1,1,2,3,4]`},
		{`[1, 2, 3, 4, 5].copyWithin(-2, 0, 2)`, `[1,2,3,1,2]`},
		{`[1, 2, 3, 4, 5].copyWithin(0, -3, -1)`, `[3,4,3,4,5]`},
		{`[1, 2, 3, 4, 5].copyWithin(2, 1, 1)`, `[1,2,3,4,5]`},
		{`[1, 2, 3].copyWithin()`, `[1,2,3]`},
		{`[1, 2, 3, 4, 5].copyWithin(1.7, '3')`, `[1,4,5,4,5]`},
		{`[].copyWithin.length`, `2`},
		{`(() => { try { Object.freeze([1, 2, 3]).copyWithin(0, 1) } catch (e) { return e.message } })()`, `"Cannot assign to read only property '0' of object '[object Array]'"`},
		{`(() => { try { Object.freeze([1, 2, 3]).copyWithin(1, 0) } catch (e) { return e.message } })()`, `"Cannot assign to read only property '2' of object '[object Array]'"`},
		{`Object.freeze([1, 2, 3]).copyWithin(0, 3)`, `[1,2,3]`},
		{`['ab', 'a\uD800', '\uDC00b', '😀', '', 'x􏿿y\uD800'].map((s) => s.isWellFormed())`, `[true,false,false,true,true,false]`},
		{`['ab', 'a\uD800', '\uDC00b', '😀', 'x􏿿y\uD800'].map((s) => s.toWellFormed()).map((s) => [...s].map((c) => c.codePointAt(0).toString(16)).join(' '))`, `["61 62","61 fffd","fffd 62","1f600","78 10ffff 79 fffd"]`},
		{`[String.prototype.isWellFormed.length, String.prototype.toWellFormed.length, String.prototype.isWellFormed.call(12)]`, `[0,0,true]`},
		{`(() => { try { String.prototype.toWellFormed.call(null) } catch (e) { return e.message } })()`, `"String.prototype.toWellFormed called on null or undefined"`},
	}
	for _, tt := range tests {
		if got := evalJSON(t, r, tt.src); got != tt.want {
			t.Errorf("%s\n got %s\nwant %s", tt.src, got, tt.want)
		}
	}
}

// TestErrorMessages checks V8's messages where they depend on the call site.
// The expected values were printed by Node v24.22.1.
func TestErrorMessages(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		src, want string
	}{
		{`[[1], [2]].with(-3, 'w')`, "Invalid index : -3"},
		{`[1].with(4, 'w')`, "Invalid index : 4"},
		{`state.fn.call.call(1)`, "state.fn.call.call is not a function"},
		{`Function.prototype.call.call(1)`, "Function.prototype.call.call is not a function"},
		{`state.fn.bind.call(1)`, "Bind must be called on a function"},
		{`(() => { const [a] = state.n })()`, "number 42 is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{`(() => { const [a] = state.nul })()`, "object null is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{`(() => { const [a] = state.f() })()`, "number 5 is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{`(() => { const [a] = 42 })()`, "42 is not iterable"},
		{`(() => { const [a] = -1 })()`, "-1 is not iterable"},
		{`(() => { const [a] = null })()`, "null is not iterable"},
		{`(() => { const [a] = {} })()`, "{} is not iterable"},
		{`(() => { const [a] = { b: 1 } })()`, "{(intermediate value)} is not iterable"},
		{`(() => { const [a] = state })()`, "state is not iterable"},
		{`(() => { const [a] = !state })()`, "(!state) is not iterable"},
		{`(() => { const [a] = state.n ? 1 : 2 })()`, "(intermediate value)(intermediate value)(intermediate value) is not iterable"},
		{`(() => { const [a] = () => 1 })()`, "(intermediate value) is not iterable"},
		{`(() => { const [a] = new state.C() })()`, "state.C is not a function or its return value is not iterable"},
		{`(() => { const [a] = state.n + 1 })()`, "number 43 is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{`(() => { let a; [a] = 42 })()`, "number 42 is not iterable (cannot read property Symbol(Symbol.iterator))"},
		{`(([a]) => a)(null)`, "object null is not iterable (cannot read property Symbol(Symbol.iterator))"},
	}
	state := `({ n: 42, nul: null, f() { return 5 }, fn() {}, C: function C() { return 1 } })`
	for _, tt := range tests {
		f, err := r.NewFunction("state", "return "+tt.src, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.src, err)
		}
		mk, err := r.NewFunction("", "return "+state, false)
		if err != nil {
			t.Fatal(err)
		}
		st, err := r.Call(mk, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Call(f, nil, []any{st})
		if e, ok := CaughtValue(err).(*js.Error); !ok || e.Message != tt.want {
			t.Errorf("%s: %v; want %q", tt.src, err, tt.want)
		}
	}
}

// TestInspectErrors checks how console.log prints errors without stack
// frames and with a stack code assigned. The expected values were printed by
// Node v24.22.1.
func TestInspectErrors(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		src, want string
	}{
		{`(() => { const e = new Error("boom"); delete e.stack; return e })()`, "[Error: boom]"},
		{`Object.assign(new RangeError("r"), { stack: "custom stack" })`, "[custom stack]"},
		{`[Object.assign(new RangeError("r"), { stack: "custom stack" })]`, "[ [custom stack] ]"},
	}
	for _, tt := range tests {
		f, err := r.NewFunction("", "return "+tt.src, false)
		if err != nil {
			t.Fatal(err)
		}
		v, err := r.Call(f, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Inspect(v); got != tt.want {
			t.Errorf("inspect(%s) = %q; want %q", tt.src, got, tt.want)
		}
	}
}
