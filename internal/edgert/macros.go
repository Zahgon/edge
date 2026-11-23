package edgert

import (
	"sync"

	"github.com/edge-js/edge-go/internal/lodash"
	"github.com/edge-js/edge-go/js"
)

// The functions the bundled tags add to the Template class when they boot:
// @let, @assign and the migrate plugin's @set add lodash's set as
// template.setValue, @each adds each, asyncEach and lodash's size as
// template.loop, template.loopAsync and template.size.

// LodashSet is lodash's set as the minified build defines it: a function
// without a name taking (object, path, value).
var LodashSet = sync.OnceValue(func() *js.Function {
	return NewFunction("Template.setValue", "", 3, func(_ any, args []any) (any, error) {
		return lodash.Set(Ops(), js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
	})
})

// LodashSize is lodash's size, a function without a name taking (collection).
var LodashSize = sync.OnceValue(func() *js.Function {
	return NewFunction("Template.size", "", 1, func(_ any, args []any) (any, error) {
		return lodash.Size(Ops(), js.Arg(args, 0))
	})
})

// Each is each(collection, iteratee): iteratee(value, key) for the items of
// an array, the code points of a string (with their position among the code
// points) or the own enumerable properties of any other object.
var Each = sync.OnceValue(func() *js.Function {
	return NewFunction("Template.loop", "each", 2, func(_ any, args []any) (any, error) {
		collection, iteratee := js.Arg(args, 0), js.Arg(args, 1)
		return js.Undefined, forEachEntry(collection, func(value, key any) error {
			_, err := Ops().Call(iteratee, js.Undefined, []any{value, key}, "iteratee")
			return err
		})
	})
})

// AsyncEach is asyncEach(collection, iteratee): each, awaiting what every
// iteratee call returns before the next one.
var AsyncEach = sync.OnceValue(func() *js.Function {
	return NewAsyncFunction("Template.loopAsync", "asyncEach", 2, func(await func(any) (any, error), _ any, args []any) (any, error) {
		collection, iteratee := js.Arg(args, 0), js.Arg(args, 1)
		return js.Undefined, forEachEntry(collection, func(value, key any) error {
			result, err := Ops().Call(iteratee, js.Undefined, []any{value, key}, "iteratee")
			if err != nil {
				return err
			}
			_, err = await(result)
			return err
		})
	})
})

// forEachEntry visits the entries each and asyncEach visit: an array's
// entries() (live, by index), a string's code points numbered from 0, and
// Object.entries of any other non-null object. Functions and other primitives
// are not visited.
func forEachEntry(collection any, visit func(value, key any) error) error {
	isArray, err := Ops().IsArray(collection)
	if err != nil {
		return err
	}
	if isArray {
		index := 0
		return Realm().ForOf(collection, func(value any) error {
			key := float64(index)
			index++
			return visit(value, key)
		})
	}
	if s, ok := collection.(string); ok {
		index := 0
		return Realm().ForOf(s, func(value any) error {
			key := float64(index)
			index++
			return visit(value, key)
		})
	}
	if js.TypeOf(collection) != "object" || collection == nil {
		return nil
	}
	entries, err := Realm().Entries(collection)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := visit(e.Value, e.Key); err != nil {
			return err
		}
	}
	return nil
}
