// Package edgert holds the JavaScript objects Edge's runtime hands to compiled
// templates and that both the edge package and the migrate plugin build on:
// SafeValue and escaping, the EdgeError class as templates see it, the
// component props classes (ComponentProps, and the compat Props the Template
// creates in compat mode), Edge's attribute serializer, the functions the
// globals of Edge and of the migrate plugin share, and the macros the bundled
// tags add to the Template class.
// Everything lives in the process-wide realm, as everything in one Node
// process shares one global object.
package edgert

import (
	"runtime"
	"sync"
	"weak"

	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Realm returns the process-wide realm compiled templates run in.
func Realm() *jsrt.Realm { return jsrt.Default() }

// Ops returns the JavaScript operations of the process-wide realm.
var Ops = sync.OnceValue(func() *jsval.Ops {
	ops, err := jsval.New(Realm())
	if err != nil {
		// The realm's own builtins are always present when the module loads.
		panic(err)
	}
	return ops
})

// Slots associates the private fields of class instances (#compiler,
// #values, ...) with the JavaScript objects (of Go type *O) they belong to.
// Instances are held weakly: an entry goes away with its object.
type Slots[O, T any] struct {
	mu sync.Mutex
	m  map[weak.Pointer[O]]T
}

// Attach records the private fields of obj. The fields must not refer to obj
// itself, or the object would never be collected.
func (s *Slots[O, T]) Attach(obj *O, fields T) {
	key := weak.Make(obj)
	s.mu.Lock()
	if s.m == nil {
		s.m = map[weak.Pointer[O]]T{}
	}
	s.m[key] = fields
	s.mu.Unlock()
	runtime.AddCleanup(obj, func(key weak.Pointer[O]) {
		s.mu.Lock()
		delete(s.m, key)
		s.mu.Unlock()
	}, key)
}

// Lookup returns the fields recorded for v, if v is an object that has them.
func (s *Slots[O, T]) Lookup(v any) (T, bool) {
	var zero T
	obj, ok := v.(*O)
	if !ok {
		return zero, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fields, found := s.m[weak.Make(obj)]
	return fields, found
}

// Get returns the private fields of v, reading the private member name (as
// "#values") the way V8 does: for null and undefined, and for an object of
// another class, the read throws V8's TypeError.
func (s *Slots[O, T]) Get(v any, name string) (T, error) {
	if fields, ok := s.Lookup(v); ok {
		return fields, nil
	}
	var zero T
	if js.IsNullish(v) {
		return zero, js.CannotReadProperties(v, name)
	}
	return zero, js.NewTypeError("Cannot read private member " + name + " from an object whose class did not declare it")
}

// Spread copies the own enumerable properties of v into target, as `...v` in
// an object literal does: null and undefined add nothing, getters run, and a
// key target already has keeps its position.
func Spread(target *js.Object, v any) error {
	if js.IsNullish(v) {
		return nil
	}
	entries, err := Realm().Entries(v)
	if err != nil {
		return err
	}
	for _, e := range entries {
		target.Set(e.Key, e.Value)
	}
	return nil
}

// ObjectAssign is Object.assign(target, source), read from the global Object
// at the time of the call as the original's code reads it.
func ObjectAssign(target, source any) error {
	object, err := Realm().Get(Realm().Global(), "Object")
	if err != nil {
		return err
	}
	_, err = Ops().Invoke(object, "assign", []any{target, source}, "Object.assign")
	return err
}
