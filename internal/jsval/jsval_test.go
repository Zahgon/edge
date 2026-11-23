package jsval_test

// The expected values were recorded on Node 24 by running the same JavaScript
// operations on values made from the same source text.

import (
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// stringCase is a value's source text and the string an operation returns
// for it, or the error it throws ("Name: message").
type stringCase struct {
	s, want, err string
}

// checkString runs op on the value of each case's source.
func checkString(t *testing.T, cases []stringCase, op func(ops *jsval.Ops, v any) (string, error)) {
	t.Helper()
	ops := jsvaltest.NewOps(t)
	for _, c := range cases {
		got, err := op(ops, jsvaltest.MustEval(t, ops, c.s))
		switch {
		case c.err != "":
			if err == nil || jsvaltest.DescribeError(err) != c.err {
				t.Errorf("%s: got %q, %v; want error %s", c.s, got, err, c.err)
			}
		case err != nil:
			t.Errorf("%s: unexpected error %v", c.s, err)
		case got != c.want:
			t.Errorf("%s: got %q, want %q", c.s, got, c.want)
		}
	}
}

func TestToString(t *testing.T) {
	checkString(t, []stringCase{
		{s: "[1, [2, 3], null, undefined]", want: "1,2,3,,"},
		{s: "({})", want: "[object Object]"},
		{s: "-0", want: "0"},
		{s: "1e21", want: "1e+21"},
		{s: "0.000001", want: "0.000001"},
		{s: "1e-7", want: "1e-7"},
		{s: "123456789012345680000", want: "123456789012345680000"},
		{s: "null", want: "null"},
		{s: "({ toString() { return 'T' } })", want: "T"},
		{s: "({ valueOf() { return 1 }, toString() { return 'S' } })", want: "S"},
		{s: "({ toString() { throw new Error('no') } })", err: "Error: no"},
		{s: "Object.create(null)", err: "TypeError: Cannot convert object to primitive value"},
		{s: "[function f() { return 1 }]", want: "function f() { return 1 }"},
	}, (*jsval.Ops).ToString)
}

func TestToNumber(t *testing.T) {
	checkString(t, []stringCase{
		{s: "' 12 '", want: "12"},
		{s: "'0x1f'", want: "31"},
		{s: "'0b101'", want: "5"},
		{s: "'1e3'", want: "1000"},
		{s: "[5]", want: "5"},
		{s: "[1, 2]", want: "NaN"},
		{s: "null", want: "0"},
		{s: "undefined", want: "NaN"},
		{s: "true", want: "1"},
		{s: "''", want: "0"},
		{s: "'12px'", want: "NaN"},
		{s: "({ valueOf() { return '7' } })", want: "7"},
		{s: "'-Infinity'", want: "-Infinity"},
		{s: "'1_000'", want: "NaN"},
		{s: "Object.create(null)", err: "TypeError: Cannot convert object to primitive value"},
	}, func(ops *jsval.Ops, v any) (string, error) {
		n, err := ops.ToNumber(v)
		return js.ToString(n), err
	})
}

func TestTag(t *testing.T) {
	checkString(t, []stringCase{
		{s: "[]", want: "[object Array]"},
		{s: "null", want: "[object Null]"},
		{s: "undefined", want: "[object Undefined]"},
		{s: "new Map()", want: "[object Map]"},
		{s: "/x/", want: "[object RegExp]"},
		{s: "Object.create(null)", want: "[object Object]"},
		{s: "function () {}", want: "[object Function]"},
		{s: "'s'", want: "[object String]"},
		{s: "1", want: "[object Number]"},
		{s: "new Error('x')", want: "[object Error]"},
		{s: "JSON", want: "[object JSON]"},
		{s: "Math", want: "[object Math]"},
		{s: "Promise.resolve()", want: "[object Promise]"},
		{s: "(function () { return arguments })()", want: "[object Arguments]"},
		{s: "new Date(0)", want: "[object Date]"},
		{s: "new Set()", want: "[object Set]"},
	}, (*jsval.Ops).Tag)
}

func TestKeys(t *testing.T) {
	join := func(keys []string, err error) (string, error) { return strings.Join(keys, ","), err }
	checkString(t, []stringCase{
		{s: "{ b: 1, a: 2, 1: 3, 0: 4 }", want: "0,1,b,a"},
		{s: "'ab'", want: "0,1"},
		{s: "[7, 8]", want: "0,1"},
		{s: "Object.defineProperty({ a: 1 }, 'b', { value: 2 })", want: "a"},
	}, func(ops *jsval.Ops, v any) (string, error) { return join(ops.Keys(v)) })
	checkString(t, []stringCase{
		{s: "[7, 8]", want: "0,1,length"},
		{s: "Object.defineProperty({ a: 1 }, 'b', { value: 2 })", want: "a,b"},
		{s: "function f(a) {}", want: "length,name,arguments,caller,prototype"},
		{s: "(a) => a", want: "length,name"},
		{s: "'xy'", want: "0,1,length"},
	}, func(ops *jsval.Ops, v any) (string, error) { return join(ops.OwnNames(v)) })
}

func TestForIn(t *testing.T) {
	checkString(t, []stringCase{
		{s: "Object.assign(Object.create({ p: 1, q: 2 }), { q: 3, r: 4 })", want: "q,r,p"},
		{s: "[1, 2]", want: "0,1"},
		{s: "'xy'", want: "0,1"},
		{s: "null", want: ""},
		{s: "Object.create(Object.defineProperty({ h: 1, v: 2 }, 'h', { enumerable: false }))", want: "v"},
	}, func(ops *jsval.Ops, v any) (string, error) {
		var keys []string
		err := ops.ForIn(v, func(key string) (bool, error) {
			keys = append(keys, key)
			return false, nil
		})
		return strings.Join(keys, ","), err
	})
}

func TestFunctionSource(t *testing.T) {
	checkString(t, []stringCase{
		{s: "function f(a, b) { return a + b }", want: "function f(a, b) { return a + b }"},
		{s: "Math.max", want: "function max() { [native code] }"},
		{s: "(x) => x * 2", want: "(x) => x * 2"},
		{s: "async function g() { await 1 }", want: "async function g() { await 1 }"},
		{s: "1", err: "TypeError: Function.prototype.toString requires that 'this' be a Function"},
	}, (*jsval.Ops).FunctionSource)
}

func TestAdd(t *testing.T) {
	// s is the source of the two operands of a + b; want is typeof and String
	// of the sum.
	checkString(t, []stringCase{
		{s: "[1, '2']", want: "string 12"},
		{s: "[[1], 2]", want: "string 12"},
		{s: "[({}), 'x']", want: "string [object Object]x"},
		{s: "[null, 'x']", want: "string nullx"},
		{s: "[true, 1]", want: "number 2"},
		{s: "[undefined, 1]", want: "number NaN"},
		{s: "['a', [1, [2]]]", want: "string a1,2"},
	}, func(ops *jsval.Ops, v any) (string, error) {
		operands := v.(*js.Array).Items
		sum, err := ops.Add(operands[0], operands[1])
		return js.TypeOf(sum) + " " + js.ToString(sum), err
	})
}

func TestHasProperty(t *testing.T) {
	tests := []struct {
		s, key string
		want   bool
	}{
		{"{}", "toString", true},
		{"'abc'", "length", true},
		{"'abc'", "1", true},
		{"[]", "push", true},
		{"{ a: 1 }", "b", false},
		{"Object.create(null)", "toString", false},
		{"5", "toFixed", true},
	}
	ops := jsvaltest.NewOps(t)
	for _, tt := range tests {
		got, err := ops.HasProperty(jsvaltest.MustEval(t, ops, tt.s), tt.key)
		if err != nil || got != tt.want {
			t.Errorf("%q in Object(%s) = %v, %v; want %v", tt.key, tt.s, got, err, tt.want)
		}
	}
}

func TestInstanceOf(t *testing.T) {
	tests := []struct {
		s, ctor string
		want    bool
	}{
		{"[]", "Array", true},
		{"Object.create(Array.prototype)", "Array", true},
		{"1", "Number", false},
		{"new Date(0)", "Object", true},
		{"{}", "Array", false},
	}
	ops := jsvaltest.NewOps(t)
	for _, tt := range tests {
		ctor, err := ops.BuiltinFunction(tt.ctor)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ops.InstanceOf(jsvaltest.MustEval(t, ops, tt.s), ctor)
		if err != nil || got != tt.want {
			t.Errorf("%s instanceof %s = %v, %v; want %v", tt.s, tt.ctor, got, err, tt.want)
		}
	}
}

func TestInvoke(t *testing.T) {
	// String(value[key]()), with the callee written value[k] as in the
	// recording.
	tests := []struct {
		s, key, want, err string
	}{
		{s: "undefined", key: "foo", err: "TypeError: Cannot read properties of undefined (reading 'foo')"},
		{s: "{}", key: "foo", err: "TypeError: value[k] is not a function"},
		{s: "null", key: "bar", err: "TypeError: Cannot read properties of null (reading 'bar')"},
		{s: "[3, 1, 2]", key: "sort", want: "1,2,3"},
		{s: "'abc'", key: "toUpperCase", want: "ABC"},
	}
	ops := jsvaltest.NewOps(t)
	for _, tt := range tests {
		v, err := ops.Invoke(jsvaltest.MustEval(t, ops, tt.s), tt.key, nil, "value[k]")
		if tt.err != "" {
			if err == nil || jsvaltest.DescribeError(err) != tt.err {
				t.Errorf("%s[%q](): got %v, want error %s", tt.s, tt.key, err, tt.err)
			}
			continue
		}
		if err != nil || js.ToString(v) != tt.want {
			t.Errorf("%s[%q]() = %v, %v; want %q", tt.s, tt.key, v, err, tt.want)
		}
	}
}

func TestCaseMapping(t *testing.T) {
	tests := []struct {
		s, upper, lower string
	}{
		{"straße", "STRASSE", "straße"},
		{"ﬁ", "FI", "ﬁ"},
		{"ΌΣΟΣ", "ΌΣΟΣ", "όσος"},
		{"İ", "İ", "i\u0307"},
		{"ǅ", "Ǆ", "ǆ"},
		{"ŉ", "ʼN", "ŉ"},
		{"ΑΣ.", "ΑΣ.", "ας."},
		{"Σ", "Σ", "σ"},
		{"ABC", "ABC", "abc"},
		{"ÀÉÎ", "ÀÉÎ", "àéî"},
		{"σς", "ΣΣ", "σς"},
	}
	for _, tt := range tests {
		if got := jsval.ToUpperCase(tt.s); got != tt.upper {
			t.Errorf("%+q.toUpperCase() = %+q, want %+q", tt.s, got, tt.upper)
		}
		if got := jsval.ToLowerCase(tt.s); got != tt.lower {
			t.Errorf("%+q.toLowerCase() = %+q, want %+q", tt.s, got, tt.lower)
		}
	}
}
