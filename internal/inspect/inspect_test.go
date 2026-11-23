package inspect_test

import (
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// eval runs src as the body of a function in a new realm and returns its
// result with the realm, whose console writes to out.
func eval(t *testing.T, src string, out *strings.Builder) (*jsrt.Realm, any) {
	t.Helper()
	r := jsrt.NewRealm(jsrt.Options{})
	r.SetConsole(out, out)
	fn, err := r.NewFunction("", src, false)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, err := r.Call(fn, js.Undefined, nil)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return r, v
}

// TestInspect checks util.inspect with Node's default options, as console.log
// prints a value that is not a string: primitives, quoting, arrays grouped
// into columns and cut at 100 items, nesting beyond depth 2, objects with
// unusual keys, null prototypes, cycles, accessors, collections and their
// iterators, boxed primitives, promises, arguments and the namespace objects.
// The expected values were printed by Node v24.22.1.
func TestInspect(t *testing.T) {
	tests := []struct{ src, want string }{
		{`return -0`,
			`-0`},
		{`return 'it\'s "q"'`,
			"`it's \"q\"`"},
		{`return 'a\nb'`,
			`'a\nb'`},
		{`return [1, 'two', null, undefined, true]`,
			`[ 1, 'two', null, undefined, true ]`},
		{`return { a: 1, 'b-c': 2, 3: 'x', nested: { deep: { deeper: { deepest: 1 } } } }`,
			`{ '3': 'x', a: 1, 'b-c': 2, nested: { deep: { deeper: [Object] } } }`},
		{`return Array.from({ length: 30 }, (_, i) => i)`,
			`[
   0,  1,  2,  3,  4,  5,  6,  7,  8,
   9, 10, 11, 12, 13, 14, 15, 16, 17,
  18, 19, 20, 21, 22, 23, 24, 25, 26,
  27, 28, 29
]`},
		{`return Array.from({ length: 120 }, (_, i) => i % 3)`,
			`[
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0, 1, 2, 0, 1, 2, 0, 1, 2,
  0, 1, 2, 0,
  ... 20 more items
]`},
		{`return ['aaaaaaaaaaaaaaaaaaaa', 'bbbbbbbbbbbbbbbbbbbbbbbb', 'cccccccccccccccccccccccc', 'dddddddddddddddddddd']`,
			`[
  'aaaaaaaaaaaaaaaaaaaa',
  'bbbbbbbbbbbbbbbbbbbbbbbb',
  'cccccccccccccccccccccccc',
  'dddddddddddddddddddd'
]`},
		{`return new Map([['k', { v: 1 }], [2, [3]]])`,
			`Map(2) { 'k' => { v: 1 }, 2 => [ 3 ] }`},
		{`return new Set(['a', 1, null])`,
			`Set(3) { 'a', 1, null }`},
		{`return new Date(0)`,
			`1970-01-01T00:00:00.000Z`},
		{`return [function named() {}, () => {}, async function af() {}]`,
			`[ [Function: named], [Function (anonymous)], [AsyncFunction: af] ]`},
		{`const o = Object.create(null); o.k = 'v'; return o`,
			`[Object: null prototype] { k: 'v' }`},
		{`const o = { name: 'self' }; o.me = o; return o`,
			`<ref *1> { name: 'self', me: [Circular *1] }`},
		{`return { get g() { return 1 }, set s(v) {}, get gs() { return 1 }, set gs(v) {} }`,
			`{ g: [Getter], s: [Setter], gs: [Getter/Setter] }`},
		{`return [new Number(3), new String('s'), new Boolean(false)]`,
			`[ [Number: 3], [String: 's'], [Boolean: false] ]`},
		{`return [Promise.resolve(4), (() => { const p = Promise.reject(5); p.catch(() => {}); return p })(), new Promise(() => {})]`,
			`[ Promise { 4 }, Promise { <rejected> 5 }, Promise { <pending> } ]`},
		{`return (function () { return arguments })(1, 'a')`,
			`[Arguments] { '0': 1, '1': 'a' }`},
		{`return /re+/gi`,
			`/re+/gi`},
		{`return [[1, [2, [3, [4, [5]]]]]]`,
			`[ [ 1, [ 2, [Array] ] ] ]`},
		{`return { ['multi\nline key']: 1, valid_id: 2, 'with space': 3 }`,
			`{ 'multi\nline key': 1, valid_id: 2, 'with space': 3 }`},
		{`return [new Map([[1, 2]]).entries(), new Set([1]).values()]`,
			`[ [Map Entries] { [ 1, 2 ] }, [Set Iterator] { 1 } ]`},
		{`return Math`,
			`Object [Math] {}`},
		{`return JSON`,
			`Object [JSON] {}`},
		{`return globalThis.Infinity`,
			`Infinity`},
		{`return { u: undefined, n: null, nan: NaN, inf: -Infinity }`,
			`{ u: undefined, n: null, nan: NaN, inf: -Infinity }`},
		{`return new WeakMap()`,
			`WeakMap { <items unknown> }`},
	}
	for _, tt := range tests {
		var out strings.Builder
		r, v := eval(t, tt.src, &out)
		if got := r.Inspect(v); got != tt.want {
			t.Errorf("%s\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}

// TestFormat checks util.format as console.log applies it: the %s, %d, %i,
// %f, %j, %o, %O, %c and %% directives, unknown directives, arguments left
// over after the format, -0 and values with their own toString. The expected
// values were printed by Node v24.22.1.
func TestFormat(t *testing.T) {
	tests := []struct{ src, want string }{
		{`console.log('%s and %s', 'one', { a: 1 })`,
			`one and { a: 1 }`},
		{`console.log('%d %i %f', '42.5', '42.5', '42.5abc')`,
			`42.5 42 42.5`},
		{`console.log('%j', { a: [1, 'x'] })`,
			`{"a":[1,"x"]}`},
		{`const o = {}; o.o = o; console.log('%j', o)`,
			`[Circular]`},
		{`console.log('%o', { a: [1, 2] })`,
			`{ a: [ 1, 2, [length]: 2 ] }`},
		{`console.log('%O', { a: { b: { c: { d: 1 } } } })`,
			`{ a: { b: { c: [Object] } } }`},
		{`console.log('100%% done %c styled', 'color: red')`,
			`100% done  styled`},
		{`console.log('extra', 1, 'args', [2])`,
			`extra 1 args [ 2 ]`},
		{`console.log('%s', null, undefined, '%s')`,
			`null undefined %s`},
		{`console.log('%x unknown %s', 'v')`,
			`%x unknown v`},
		{`console.log(-0, [-0], '%d', -0)`,
			`-0 [ -0 ] %d -0`},
		{`console.log('%s', -0, 1.5)`,
			`-0 1.5`},
		{`console.log('%s', [1, [2]], new Error('e1').message)`,
			`[ 1, [Array] ] e1`},
		{`console.log('%i', 2 ** 70)`,
			`1`},
		{`console.log({ toString() { return 'custom' } } + '', '%s', { toString() { return 'custom' } })`,
			`custom %s { toString: [Function: toString] }`},
		{`class Pt { constructor() { this.x = 1 } } console.log('%s', new Pt())`,
			`Pt { x: 1 }`},
	}
	for _, tt := range tests {
		var out strings.Builder
		eval(t, tt.src, &out)
		if got := strings.TrimSuffix(out.String(), "\n"); got != tt.want {
			t.Errorf("%s\n got %q\nwant %q", tt.src, got, tt.want)
		}
	}
}
