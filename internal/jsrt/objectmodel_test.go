package jsrt

import (
	"testing"

	"github.com/edge-js/edge-go/js"
)

// evalJSON runs `return JSON.stringify(<src>)` as strict-mode code in r.
func evalJSON(t *testing.T, r *Realm, src string) string {
	t.Helper()
	f, err := r.NewFunction("", `"use strict"; return JSON.stringify(`+src+`)`, false)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	v, err := r.Call(f, js.Undefined, nil)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s: JSON.stringify returned %v", src, v)
	}
	return s
}

// TestOwnKeysAndTags checks the key order of objects whose builtin
// properties live outside their property table, and the tags
// Object.prototype.toString finds through Symbol.toStringTag. The expected
// values were printed by Node v24.22.1.
func TestOwnKeysAndTags(t *testing.T) {
	r := NewRealm(Options{})
	tests := []struct {
		src, want string
	}{
		{`Object.getOwnPropertyNames(new String('ab'))`, `["0","1","length"]`},
		{`Object.keys(new String('ab'))`, `["0","1"]`},
		{`(() => { const s = new String('ab'); s.x = 1; s[5] = 2; return Object.getOwnPropertyNames(s) })()`, `["0","1","5","length","x"]`},
		{`(() => { const e = new Error('m'); e[0] = 1; return Object.getOwnPropertyNames(e) })()`, `["0","stack","message"]`},
		{`(() => { const e = new Error('m'); e.b = 1; e[3] = 1; e[1] = 2; return Object.getOwnPropertyNames(e) })()`, `["1","3","stack","message","b"]`},
		{`(function () { arguments.x = 1; arguments[5] = 2; return Object.getOwnPropertyNames(arguments) })(7, 8)`, `["0","1","5","length","callee","x"]`},
		{`(function () { arguments[5] = 2; return Object.keys(arguments) })(7, 8)`, `["0","1","5"]`},
		{`(() => { const f = function () {}; f[2] = 1; f.z = 1; return Object.getOwnPropertyNames(f) })()`, `["2","length","name","prototype","z"]`},
		{`(() => { const a = [1]; a.x = 1; a[1] = 2; return Object.getOwnPropertyNames(a) })()`, `["0","1","length","x"]`},
		{`Object.prototype.toString.call(Object.create(Set.prototype))`, `"[object Set]"`},
		{`Object.prototype.toString.call(Object.create(Map.prototype))`, `"[object Map]"`},
		{`Object.prototype.toString.call(Object.create(Promise.prototype))`, `"[object Promise]"`},
		{`Object.prototype.toString.call(Object.setPrototypeOf(new Map(), null))`, `"[object Object]"`},
		{`Object.prototype.toString.call(Object.setPrototypeOf(new Set(), Object.prototype))`, `"[object Object]"`},
		{`Object.prototype.toString.call(Object.create(Math))`, `"[object Math]"`},
		{`Object.prototype.toString.call(new Map().entries())`, `"[object Map Iterator]"`},
		{`Object.prototype.toString.call(Object.create(new Set().values()))`, `"[object Set Iterator]"`},
	}
	for _, tt := range tests {
		if got := evalJSON(t, r, tt.src); got != tt.want {
			t.Errorf("%s\n got %s\nwant %s", tt.src, got, tt.want)
		}
	}
}
