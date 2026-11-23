// Package lodash ports the functions Edge calls from @poppinss/utils/lodash, the
// lodash 4.17.5 custom build (`lodash include="pick,pickBy,omit,omitBy,has,get,set,
// unset,mergeWith,merge,size,clone,cloneWith,cloneDeep,cloneDeepWith,toPath"`):
// merge, set, get, has, pick, omit and size.
//
// The functions work on JavaScript values of the template runtime. Every read,
// write, key enumeration and type test on a value goes through the realm (package
// jsval), in the order lodash performs them, so getters, toString/valueOf
// methods, prototype chains and the errors V8 throws are the same. The build is
// sloppy-mode code: assignments that JavaScript refuses are ignored, deletes of
// non-configurable properties report false instead of throwing.
package lodash

import (
	"math"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// maxSafeInteger is Number.MAX_SAFE_INTEGER, lodash's MAX_SAFE_INTEGER.
const maxSafeInteger = 9007199254740991

var (
	// reIsUint is lodash's reIsUint, /^(?:0|[1-9]\d*)$/.
	reIsUint = jsval.MustCompile(`^(?:0|[1-9]\d*)$`, "")
)

// env holds the builtins the build captured when it loaded (Object.create,
// Object.defineProperty, the Map and Set size getters behind the native isMap
// and isSet checks, the source text of Object), read from the realm's globals
// when a call starts.
type env struct {
	ops            *jsval.Ops
	objectCreate   *js.Function
	defineProperty *js.Function
	mapSize        *js.Function
	setSize        *js.Function
	objectProto    any
	// objectSource is Function.prototype.toString.call(Object), lodash's
	// objectCtorString.
	objectSource string
}

func newEnv(ops *jsval.Ops) (*env, error) {
	e := &env{ops: ops}
	var err error
	if e.objectCreate, err = ops.BuiltinFunction("Object", "create"); err != nil {
		return nil, err
	}
	if e.defineProperty, err = ops.BuiltinFunction("Object", "defineProperty"); err != nil {
		return nil, err
	}
	if e.mapSize, err = ops.SizeGetter("Map"); err != nil {
		return nil, err
	}
	if e.setSize, err = ops.SizeGetter("Set"); err != nil {
		return nil, err
	}
	if e.objectProto, err = ops.Builtin("Object", "prototype"); err != nil {
		return nil, err
	}
	object, err := ops.Builtin("Object")
	if err != nil {
		return nil, err
	}
	if e.objectSource, err = ops.FunctionSource(object); err != nil {
		return nil, err
	}
	return e, nil
}

// isObject is lodash's isObject: an object or a function.
func isObject(v any) bool { return js.IsObject(v) }

// isObjectLike is lodash's isObjectLike: a non-null value of typeof "object".
func isObjectLike(v any) bool { return v != nil && js.TypeOf(v) == "object" }

// isBuffer is Buffer.isBuffer, which the build uses as isBuffer: the value model
// represents a Buffer as a []byte.
func isBuffer(v any) bool {
	_, ok := v.([]byte)
	return ok
}

// eq is lodash's eq, SameValueZero. Buffers compare by identity of their memory.
func eq(a, b any) bool {
	x, xb := a.([]byte)
	y, yb := b.([]byte)
	if xb || yb {
		return xb && yb && len(x) == len(y) && cap(x) == cap(y) && (len(x) == 0 || &x[0] == &y[0])
	}
	return js.SameValueZero(a, b)
}

// isLength is lodash's isLength.
func isLength(v any) bool {
	n, ok := v.(float64)
	return ok && n > -1 && n == math.Trunc(n) && n <= maxSafeInteger
}

// tag is lodash's baseGetTag: Object.prototype.toString.call(v).
func (e *env) tag(v any) (string, error) { return e.ops.Tag(v) }

// isFunction is lodash's isFunction.
func (e *env) isFunction(v any) (bool, error) {
	if !isObject(v) {
		return false, nil
	}
	t, err := e.tag(v)
	switch t {
	case "[object Function]", "[object GeneratorFunction]", "[object AsyncFunction]", "[object Proxy]":
		return true, err
	}
	return false, err
}

// isArray is Array.isArray.
func (e *env) isArray(v any) (bool, error) { return e.ops.IsArray(v) }

// isArguments is lodash's isArguments: an object-like value tagged
// "[object Arguments]".
func (e *env) isArguments(v any) (bool, error) {
	if !isObjectLike(v) {
		return false, nil
	}
	t, err := e.tag(v)
	return t == "[object Arguments]", err
}

// isArrayLike is lodash's isArrayLike: not null, a valid length, not a function.
func (e *env) isArrayLike(v any) (bool, error) {
	if js.IsNullish(v) {
		return false, nil
	}
	n, err := e.ops.Get(v, "length")
	if err != nil || !isLength(n) {
		return false, err
	}
	fn, err := e.isFunction(v)
	return !fn, err
}

// isArrayLikeObject is lodash's isArrayLikeObject.
func (e *env) isArrayLikeObject(v any) (bool, error) {
	if !isObjectLike(v) {
		return false, nil
	}
	return e.isArrayLike(v)
}

// isPrototype is lodash's isPrototype: v is the prototype object of its own
// constructor (or Object.prototype when it has none).
func (e *env) isPrototype(v any) (bool, error) {
	var ctor any = v
	if js.ToBoolean(v) {
		c, err := e.ops.Get(v, "constructor")
		if err != nil {
			return false, err
		}
		ctor = c
	}
	var proto any
	if _, ok := ctor.(*js.Function); ok {
		p, err := e.ops.Get(ctor, "prototype")
		if err != nil {
			return false, err
		}
		proto = p
	}
	if !js.ToBoolean(proto) {
		proto = e.objectProto
	}
	return eq(v, proto), nil
}

// isPlainObject is lodash's isPlainObject: an object tagged "[object Object]"
// whose prototype is null or has an own constructor that is Object (compared by
// its source text, after checking `Ctor instanceof Ctor`).
func (e *env) isPlainObject(v any) (bool, error) {
	if !isObjectLike(v) {
		return false, nil
	}
	if t, err := e.tag(v); err != nil || t != "[object Object]" {
		return false, err
	}
	proto, err := e.proto(v)
	if err != nil || proto == nil {
		return proto == nil, err
	}
	own, err := e.ops.HasOwn(proto, "constructor")
	if err != nil || !own {
		return false, err
	}
	ctor, err := e.ops.Get(proto, "constructor")
	if err != nil {
		return false, err
	}
	f, ok := ctor.(*js.Function)
	if !ok {
		return false, nil
	}
	if inst, err := e.ops.InstanceOf(f, f); err != nil || !inst {
		return false, err
	}
	src, err := e.ops.FunctionSource(f)
	return src == e.objectSource, err
}

// proto is lodash's getPrototype: Object.getPrototypeOf(Object(v)).
func (e *env) proto(v any) (any, error) {
	obj, err := e.ops.ToObject(v)
	if err != nil {
		return nil, err
	}
	return e.ops.Proto(obj)
}

// isMap and isSet are the native checks of Node's util binding that the build
// uses: whether v has a Map's (Set's) internal slots, which the size getter of
// Map.prototype (Set.prototype) requires of its receiver.
func (e *env) isMap(v any) bool { return e.hasSlots(e.mapSize, v) }

func (e *env) isSet(v any) bool { return e.hasSlots(e.setSize, v) }

func (e *env) hasSlots(sizeGetter *js.Function, v any) bool {
	if !js.IsObject(v) {
		return false
	}
	// The getter throws for any receiver without the slots; the native check
	// reports false instead of an error.
	_, err := e.ops.Realm.Call(sizeGetter, v, nil)
	return err == nil
}

// isIndex is lodash's isIndex(value, length): a number, or a value whose string
// form matches /^(?:0|[1-9]\d*)$/, that is an integer from 0 below length. Like
// the original, it converts value again for each comparison.
func (e *env) isIndex(v any, length float64) (bool, error) {
	if length == 0 || math.IsNaN(length) {
		return false, nil
	}
	if js.TypeOf(v) != "number" {
		s, err := e.ops.ToString(v)
		if err != nil || !reIsUint.Test(s) {
			return false, err
		}
	}
	for _, test := range []func(n float64) bool{
		func(n float64) bool { return -1 < n },
		func(n float64) bool { return math.Mod(n, 1) == 0 },
		func(n float64) bool { return n < length },
	} {
		n, err := e.ops.ToNumber(v)
		if err != nil || !test(n) {
			return false, err
		}
	}
	return true, nil
}

// baseAssignValue is lodash's baseAssignValue: object[key] = value, except that
// "__proto__" is defined as an own data property with Object.defineProperty.
func (e *env) baseAssignValue(object any, key string, value any) error {
	if key == "__proto__" {
		desc := js.ObjectOf("configurable", true, "enumerable", true, "value", nil, "writable", true)
		desc.Set("value", value)
		_, err := e.ops.Realm.Call(e.defineProperty, js.Undefined, []any{object, key, desc})
		return err
	}
	return e.ops.Set(object, key, value, false)
}

// assignValue is lodash's assignValue: assigns unless object already has an own
// property key holding the same value (SameValueZero; undefined only when the
// key exists).
func (e *env) assignValue(object any, key string, value any) error {
	current, err := e.ops.Get(object, key)
	if err != nil {
		return err
	}
	own, err := e.ops.HasOwn(object, key)
	if err != nil {
		return err
	}
	if own && eq(current, value) {
		if value != js.Undefined {
			return nil
		}
		in, err := e.ops.HasProperty(object, key)
		if err != nil || in {
			return err
		}
	}
	return e.baseAssignValue(object, key, value)
}

// assignMergeValue is lodash's assignMergeValue: assigns a defined value that
// differs (SameValueZero) from object[key], and undefined only when key is not
// in object.
func (e *env) assignMergeValue(object any, key string, value any) error {
	same := value == js.Undefined
	if !same {
		current, err := e.ops.Get(object, key)
		if err != nil {
			return err
		}
		same = eq(current, value)
	}
	if same {
		if value != js.Undefined {
			return nil
		}
		in, err := e.ops.HasProperty(object, key)
		if err != nil || in {
			return err
		}
	}
	return e.baseAssignValue(object, key, value)
}

// length reads v.length.
func (e *env) length(v any) (any, error) { return e.ops.Get(v, "length") }

// arrayLikeKeys is lodash's arrayLikeKeys: the indexes of an array, arguments
// object or Buffer, then its other for-in keys (own only unless inherited is set),
// skipping length, a Buffer's offset and parent and keys that are indexes.
func (e *env) arrayLikeKeys(v any, inherited bool) ([]string, error) {
	isArr, err := e.isArray(v)
	if err != nil {
		return nil, err
	}
	isArgs, isBuf := false, false
	if !isArr {
		if isArgs, err = e.isArguments(v); err != nil {
			return nil, err
		}
		isBuf = !isArgs && isBuffer(v)
	}
	skipIndexes := isArr || isArgs || isBuf
	var keys []string
	if skipIndexes {
		n, err := e.length(v)
		if err != nil {
			return nil, err
		}
		for i := 0; js.ToNumber(n) > float64(i); i++ {
			keys = append(keys, js.NumberToString(float64(i)))
		}
	}
	count := float64(len(keys))
	err = e.ops.ForIn(v, func(key string) (bool, error) {
		if !inherited {
			own, err := e.ops.HasOwn(v, key)
			if err != nil || !own {
				return false, err
			}
		}
		if skipIndexes {
			if key == "length" || (isBuf && (key == "offset" || key == "parent")) {
				return false, nil
			}
			idx, err := e.isIndex(key, count)
			if err != nil || idx {
				return false, err
			}
		}
		keys = append(keys, key)
		return false, nil
	})
	return keys, err
}

// keysIn is lodash's keysIn: own and inherited enumerable keys, without the
// constructor key of a prototype object.
func (e *env) keysIn(v any) ([]string, error) {
	arrayLike, err := e.isArrayLike(v)
	if err != nil {
		return nil, err
	}
	if arrayLike {
		return e.arrayLikeKeys(v, true)
	}
	var keys []string
	if isObject(v) {
		isProto, err := e.isPrototype(v)
		if err != nil {
			return nil, err
		}
		err = e.ops.ForIn(v, func(key string) (bool, error) {
			if key == "constructor" {
				if isProto {
					return false, nil
				}
				own, err := e.ops.HasOwn(v, key)
				if err != nil || !own {
					return false, err
				}
			}
			keys = append(keys, key)
			return false, nil
		})
		return keys, err
	}
	err = e.ops.ForIn(v, func(key string) (bool, error) {
		keys = append(keys, key)
		return false, nil
	})
	return keys, err
}

// baseKeys is lodash's baseKeys: Object.keys(Object(v)), or for a prototype
// object its own for-in keys but constructor.
func (e *env) baseKeys(v any) ([]string, error) {
	isProto, err := e.isPrototype(v)
	if err != nil {
		return nil, err
	}
	obj, err := e.ops.ToObject(v)
	if err != nil {
		return nil, err
	}
	if !isProto {
		return e.ops.Keys(obj)
	}
	var keys []string
	err = e.ops.ForIn(obj, func(key string) (bool, error) {
		own, err := e.ops.HasOwn(v, key)
		if err == nil && own && key != "constructor" {
			keys = append(keys, key)
		}
		return false, err
	})
	return keys, err
}

// copyArray is lodash's copyArray: a new array of source.length items read from
// source.
func (e *env) copyArray(source any) (*js.Array, error) {
	n, err := e.length(source)
	if err != nil {
		return nil, err
	}
	size, err := e.ops.ToNumber(n)
	if err != nil {
		return nil, err
	}
	if size < 0 || size != math.Trunc(size) || size > js.MaxArrayLength {
		return nil, js.InvalidArrayLength()
	}
	out := &js.Array{Items: make([]any, int(size))}
	for i := range out.Items {
		if out.Items[i], err = e.ops.Get(source, js.NumberToString(float64(i))); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// toPlainObject is lodash's toPlainObject: copyObject of source's keysIn into a
// new plain object, each value stored with baseAssignValue.
func (e *env) toPlainObject(source any) (*js.Object, error) {
	keys, err := e.keysIn(source)
	if err != nil {
		return nil, err
	}
	out := js.NewObject()
	for _, k := range keys {
		v, err := e.ops.Get(source, k)
		if err != nil {
			return nil, err
		}
		if err := e.baseAssignValue(out, k, v); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// initCloneObject is lodash's initCloneObject: Object.create of the value's
// prototype, or a plain object for a prototype object or one whose constructor
// is not a function.
func (e *env) initCloneObject(v any) (any, error) {
	ctor, err := e.ops.Get(v, "constructor")
	if err != nil {
		return nil, err
	}
	if _, ok := ctor.(*js.Function); !ok {
		return js.NewObject(), nil
	}
	if isProto, err := e.isPrototype(v); err != nil || isProto {
		return js.NewObject(), err
	}
	proto, err := e.proto(v)
	if err != nil {
		return nil, err
	}
	if !isObject(proto) {
		return js.NewObject(), nil
	}
	return e.ops.Realm.Call(e.objectCreate, js.Undefined, []any{proto})
}

// stack is lodash's Stack as baseMerge and baseClone use it: source objects
// being copied, mapped to their copies, so circular references reuse them.
type stack struct {
	keys, values []any
}

func (s *stack) get(k any) (any, bool) {
	for i := len(s.keys) - 1; i >= 0; i-- {
		if eq(s.keys[i], k) {
			return s.values[i], true
		}
	}
	return nil, false
}

func (s *stack) set(k, v any) {
	for i := range s.keys {
		if eq(s.keys[i], k) {
			s.values[i] = v
			return
		}
	}
	s.keys = append(s.keys, k)
	s.values = append(s.values, v)
}

func (s *stack) remove(k any) {
	for i := range s.keys {
		if eq(s.keys[i], k) {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			s.values = append(s.values[:i], s.values[i+1:]...)
			return
		}
	}
}
