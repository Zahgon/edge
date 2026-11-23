// Package classnames is classnames 2.5.1 as Edge bundles it: html.classNames,
// and the class attribute of html.attrs and $props.toAttrs.
package classnames

import (
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// ClassNames is classNames(...args): the class names of the truthy arguments,
// joined with single spaces. Strings and numbers are names themselves; arrays
// contribute the class names of their items; an object whose toString is a
// user-defined function (not Object.prototype.toString and not a native
// function) contributes what that method returns; any other object
// contributes its own enumerable keys (in for-in order) whose values are
// truthy. Other values contribute nothing. Every property read, toString call
// and string concatenation runs in the realm, with V8's conversions and
// errors.
func ClassNames(ops *jsval.Ops, args []any) (string, error) {
	var classes any = ""
	for _, arg := range args {
		if !js.ToBoolean(arg) {
			continue
		}
		value, err := parseValue(ops, arg)
		if err != nil {
			return "", err
		}
		if classes, err = appendClass(ops, classes, value); err != nil {
			return "", err
		}
	}
	return js.ToString(classes), nil
}

// parseValue is classnames' parseValue(arg).
func parseValue(ops *jsval.Ops, arg any) (any, error) {
	switch arg.(type) {
	case string, float64:
		return arg, nil
	}
	if js.TypeOf(arg) != "object" {
		return "", nil
	}
	isArray, err := ops.IsArray(arg)
	if err != nil {
		return nil, err
	}
	if isArray {
		items, err := listFromArrayLike(ops, arg)
		if err != nil {
			return nil, err
		}
		return ClassNames(ops, items)
	}
	custom, err := hasCustomToString(ops, arg)
	if err != nil {
		return nil, err
	}
	if custom {
		return ops.Invoke(arg, "toString", nil, "arg.toString")
	}
	var classes any = ""
	err = ops.ForIn(arg, func(key string) (bool, error) {
		own, err := ops.HasOwn(arg, key)
		if err != nil || !own {
			return false, err
		}
		value, err := ops.Get(arg, key)
		if err != nil || !js.ToBoolean(value) {
			return false, err
		}
		classes, err = appendClass(ops, classes, key)
		return false, err
	})
	return classes, err
}

// hasCustomToString is
//
//	arg.toString !== Object.prototype.toString && !arg.toString.toString().includes("[native code]")
//
// with Object.prototype.toString read when the test runs.
func hasCustomToString(ops *jsval.Ops, arg any) (bool, error) {
	method, err := ops.Get(arg, "toString")
	if err != nil {
		return false, err
	}
	objectToString, err := ops.Builtin("Object", "prototype", "toString")
	if err != nil {
		return false, err
	}
	if method == objectToString {
		return false, nil
	}
	method, err = ops.Get(arg, "toString")
	if err != nil {
		return false, err
	}
	source, err := ops.Invoke(method, "toString", nil, "arg.toString.toString")
	if err != nil {
		return false, err
	}
	native, err := ops.Invoke(source, "includes", []any{"[native code]"}, "arg.toString.toString(...).includes")
	if err != nil {
		return false, err
	}
	return !js.ToBoolean(native), nil
}

// appendClass is classnames' appendClass(value, newClass), with JavaScript's +
// operator, so an object returned by a custom toString converts as + converts
// it.
func appendClass(ops *jsval.Ops, value, newClass any) (any, error) {
	if !js.ToBoolean(newClass) {
		return value, nil
	}
	if js.ToBoolean(value) {
		withSpace, err := ops.Add(value, " ")
		if err != nil {
			return nil, err
		}
		return ops.Add(withSpace, newClass)
	}
	return ops.Add(value, newClass)
}

// listFromArrayLike is the argument list classNames.apply(null, arg) builds
// from an array: its length, then each index read in turn.
func listFromArrayLike(ops *jsval.Ops, arr any) ([]any, error) {
	length, err := ops.Get(arr, "length")
	if err != nil {
		return nil, err
	}
	n := int(js.ToNumber(length))
	items := make([]any, n)
	for i := range items {
		if items[i], err = ops.Get(arr, js.ToString(float64(i))); err != nil {
			return nil, err
		}
	}
	return items, nil
}
