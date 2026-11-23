// Package jsstringify is js-stringify 1.0.2 as Edge bundles it: the js.stringify
// global, which writes a value as JavaScript source that is safe to place inside
// an HTML <script> element.
package jsstringify

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// scriptSafe replaces what JSON.stringify leaves in its output but a script
// element or an older JavaScript parser would trip on: the line and paragraph
// separators, and <, > and / (which could close the element).
var scriptSafe = strings.NewReplacer(
	"\u2028", `\u2028`,
	"\u2029", `\u2029`,
	"<", `\u003C`,
	">", `\u003E`,
	"/", `\u002F`,
)

// String is stringify(s) for a string s: JSON.stringify(s) with the
// replacements of scriptSafe. edge-parser writes every string literal of the
// generated code with it.
func String(s string) string { return scriptSafe.Replace(jsstr.QuoteJSON(s)) }

// Stringify is stringify(obj):
//
//	if (obj instanceof Date) return "new Date(" + stringify(obj.toISOString()) + ")"
//	if (obj === undefined) return "undefined"
//	return JSON.stringify(obj).replace(...)
//
// The instanceof test and obj.toISOString() run in the realm, so an object
// inheriting from Date.prototype is a Date here and its toISOString throws V8's
// TypeError. A value JSON.stringify returns undefined for (a function) makes
// the .replace call throw V8's TypeError.
func Stringify(ops *jsval.Ops, obj any) (string, error) {
	date, err := ops.BuiltinFunction("Date")
	if err != nil {
		return "", err
	}
	isDate, err := ops.InstanceOf(obj, date)
	if err != nil {
		return "", err
	}
	if isDate {
		iso, err := ops.Invoke(obj, "toISOString", nil, "obj.toISOString")
		if err != nil {
			return "", err
		}
		inner, err := Stringify(ops, iso)
		if err != nil {
			return "", err
		}
		return "new Date(" + inner + ")", nil
	}
	if obj == js.Undefined {
		return "undefined", nil
	}
	json, ok, err := ops.Realm.JSONStringify(obj, js.Undefined, js.Undefined)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", js.CannotReadProperties(js.Undefined, "replace")
	}
	return scriptSafe.Replace(json), nil
}
