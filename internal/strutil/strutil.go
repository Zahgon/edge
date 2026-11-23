// Package strutil ports the string helpers of @poppinss/utils 7.1.2 that Edge's
// globals and loader use. @poppinss/utils/string re-exports @poppinss/string 1.7.2,
// which builds them on case-anything 3.1.7 (camelCase, snakeCase, ...), pluralize
// 8.0.0, and bundled copies of bytes 3.1.2, @lukeed/ms 2.0.2 and truncatise 0.0.8.
//
// Each helper has a string-level form (CamelCase, Plural, FormatBytes, ...) and a
// JavaScript-level form (JSCamelCase, JSPluralize, ...) that takes the call's
// arguments as JavaScript values and follows the library's code for any value:
// string methods called on a non-string go through the realm (a String object
// converts itself, a missing method is V8's "<callee> is not a function"), options
// are read with property gets that run getters, and template literals and the +
// operator convert objects with their own toString/valueOf.
package strutil

import (
	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// charAt is String.prototype.charAt on code units.
func charAt(u []uint16, i int) string {
	if i < 0 || i >= len(u) {
		return ""
	}
	return jsstr.FromUTF16(u[i : i+1])
}

// unitAt is s[i] on code units: the one-unit string, or undefined past the ends.
func unitAt(u []uint16, i int) any {
	if i < 0 || i >= len(u) {
		return js.Undefined
	}
	return jsstr.FromUTF16(u[i : i+1])
}

// ReplaceFunc is String.prototype.replace(re, fn) with a replacer function: every
// match when re has the g flag, else the first, replaced by what fn returns. fn
// receives the arguments JavaScript passes a replacer, as Captures gives them (the
// match, then each group or js.Undefined), and the match's index in code units.
func ReplaceFunc(re *jsregexp.Regexp, s string, fn func(captures []any, index int) (string, error)) (string, error) {
	units := jsstr.ToUTF16(s)
	var out []uint16
	matched := false
	last, pos := 0, 0
	for pos <= len(units) {
		caps := re.Exec(units, pos)
		if caps == nil {
			break
		}
		matched = true
		rep, err := fn(jsregexp.Captures(units, caps), caps[0])
		if err != nil {
			return "", err
		}
		out = append(out, units[last:caps[0]]...)
		out = append(out, jsstr.ToUTF16(rep)...)
		last = caps[1]
		if !re.Global() {
			break
		}
		if caps[1] == caps[0] {
			pos = re.AdvanceStringIndex(units, caps[1])
		} else {
			pos = caps[1]
		}
	}
	if !matched {
		return s, nil
	}
	return jsstr.FromUTF16(append(out, units[last:]...)), nil
}

// matchAll returns the capture positions of every match of the global expression
// re in units, as String.prototype.matchAll iterates them.
func matchAll(re *jsregexp.Regexp, units []uint16) [][]int {
	var out [][]int
	for pos := 0; pos <= len(units); {
		caps := re.Exec(units, pos)
		if caps == nil {
			break
		}
		out = append(out, caps)
		if caps[1] == caps[0] {
			pos = re.AdvanceStringIndex(units, caps[1])
		} else {
			pos = caps[1]
		}
	}
	return out
}

// stringCall is recv.method(...args) where the library expects recv to be a
// string: a string runs native, any other value goes through the realm (a String
// object's builtin method converts it; null and undefined throw V8's TypeError for
// the property read; a value without the method throws "<callee> is not a
// function"; a user-defined method is called).
func stringCall(ops *jsval.Ops, recv any, method, callee string, args []any, native func(s string) any) (any, error) {
	if s, ok := recv.(string); ok {
		return native(s), nil
	}
	return ops.Invoke(recv, method, args, callee)
}

// realmRegExp builds a RegExp object of the realm, for library calls that hand one
// of their regular expressions to a method of a non-string value.
func realmRegExp(ops *jsval.Ops, re *jsregexp.Regexp) (any, error) {
	ctor, err := ops.BuiltinFunction("RegExp")
	if err != nil {
		return nil, err
	}
	return ops.Realm.Call(ctor, js.Undefined, []any{re.Source(), re.Flags()})
}

// templateString is `${v}`: ToString, which runs an object's toString. It is
// also how the library's string operations read a value that is still not a
// string after its leading string method calls, which only user-defined
// methods returning other values produce.
func templateString(ops *jsval.Ops, v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return ops.ToString(v)
}

// getIfTruthy is `options && options[key]`: the property of a truthy options value,
// options itself otherwise.
func getIfTruthy(ops *jsval.Ops, options any, key string) (any, error) {
	if !js.ToBoolean(options) {
		return options, nil
	}
	return ops.Get(options, key)
}

// lengthOf is v.length. A string's length is read natively, so the helpers'
// Go entry points, which pass no realm (ops nil), work on strings.
func lengthOf(ops *jsval.Ops, v any) (any, error) {
	if s, ok := v.(string); ok {
		return float64(jsstr.Length(s)), nil
	}
	return ops.Get(v, "length")
}
