package lodash

import (
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Merge is _.merge(object, ...sources): the own and inherited enumerable
// properties of each truthy source, left to right, merged recursively into
// Object(object), which is mutated and returned.
//
// Plain objects and arguments objects merge into the destination value (a copy
// of the prototype-less shape when the destination is not an object); arrays
// and array-likes merge index by index into the destination array (a copy of an
// array-like destination, else a new array); any other object (Dates, RegExps,
// functions, Maps, instances of constructors) and every primitive is assigned
// by reference, except that undefined never overwrites an existing key. Circular
// sources reuse the copy made for them.
//
// Like the build's createAssigner, three or more sources that look like the
// arguments of an iteratee call (value, index, collection) merge only the first.
func Merge(ops *jsval.Ops, object any, sources ...any) (any, error) {
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	count := len(sources)
	if count > 2 && js.ToBoolean(sources[2]) {
		guard, err := e.isIterateeCall(sources[0], sources[1], sources[2])
		if err != nil {
			return nil, err
		}
		if guard {
			count = 1
		}
	}
	target, err := ops.ToObject(object)
	if err != nil {
		return nil, err
	}
	for i := 0; i < count; i++ {
		if source := sources[i]; js.ToBoolean(source) {
			if err := e.baseMerge(target, source, float64(i), nil); err != nil {
				return nil, err
			}
		}
	}
	return target, nil
}

// isIterateeCall is lodash's isIterateeCall: whether object[index] is value, for
// an object collection and an index that is a valid array index of an
// array-like collection or a key in it.
func (e *env) isIterateeCall(value, index, object any) (bool, error) {
	if !isObject(object) {
		return false, nil
	}
	switch js.TypeOf(index) {
	case "number":
		arrayLike, err := e.isArrayLike(object)
		if err != nil || !arrayLike {
			return false, err
		}
		n, err := e.length(object)
		if err != nil {
			return false, err
		}
		idx, err := e.isIndex(index, js.ToNumber(n))
		if err != nil || !idx {
			return false, err
		}
	case "string":
		in, err := e.ops.HasProperty(object, index.(string))
		if err != nil || !in {
			return false, err
		}
	default:
		return false, nil
	}
	key, err := e.propertyKey(index)
	if err != nil {
		return false, err
	}
	current, err := e.ops.Get(object, key)
	if err != nil {
		return false, err
	}
	return eq(current, value), nil
}

// baseMerge is lodash's baseMerge: every keysIn key of source, read from
// Object(source), merged into object. srcIndex is the source's position among
// the arguments: from the second source on, a function in object is replaced
// rather than merged into.
func (e *env) baseMerge(object, source any, srcIndex float64, st *stack) error {
	if eq(object, source) {
		return nil
	}
	from, err := e.ops.ToObject(source)
	if err != nil {
		return err
	}
	keys, err := e.keysIn(source)
	if err != nil {
		return err
	}
	for _, key := range keys {
		srcValue, err := e.ops.Get(from, key)
		if err != nil {
			return err
		}
		if !isObject(srcValue) {
			if err := e.assignMergeValue(object, key, srcValue); err != nil {
				return err
			}
			continue
		}
		if st == nil {
			st = &stack{}
		}
		if err := e.baseMergeDeep(object, source, key, srcIndex, st); err != nil {
			return err
		}
	}
	return nil
}

// safeGet is lodash's safeGet: object[key], except undefined for "__proto__".
func (e *env) safeGet(object any, key string) (any, error) {
	if key == "__proto__" {
		return js.Undefined, nil
	}
	return e.ops.Get(object, key)
}

// baseMergeDeep is lodash's baseMergeDeep for a source value that is an object.
func (e *env) baseMergeDeep(object, source any, key string, srcIndex float64, st *stack) error {
	objValue, err := e.safeGet(object, key)
	if err != nil {
		return err
	}
	srcValue, err := e.safeGet(source, key)
	if err != nil {
		return err
	}
	if stacked, ok := st.get(srcValue); ok && js.ToBoolean(stacked) {
		return e.assignMergeValue(object, key, stacked)
	}
	newValue, recurse, err := e.mergeTarget(objValue, srcValue, srcIndex)
	if err != nil {
		return err
	}
	if recurse {
		st.set(srcValue, newValue)
		err := e.baseMerge(newValue, srcValue, srcIndex, st)
		st.remove(srcValue)
		if err != nil {
			return err
		}
	}
	return e.assignMergeValue(object, key, newValue)
}

// mergeTarget chooses the value baseMergeDeep stores for srcValue and whether
// srcValue is merged into it: arrays and Buffers go into the destination array
// (or a copy of an array-like destination, or a new array; a Buffer otherwise
// becomes a slice of itself), plain and arguments objects into the destination
// object (a plain copy of an arguments object, or Object.create of the source's
// prototype when the destination is not an object or, from the second source
// on, is a function); anything else is taken as it is.
func (e *env) mergeTarget(objValue, srcValue any, srcIndex float64) (any, bool, error) {
	isArr, err := e.isArray(srcValue)
	if err != nil {
		return nil, false, err
	}
	isBuf := !isArr && isBuffer(srcValue)
	if isArr || isBuf {
		if objArr, err := e.isArray(objValue); err != nil || objArr {
			return objValue, err == nil, err
		}
		if arrayLike, err := e.isArrayLikeObject(objValue); err != nil {
			return nil, false, err
		} else if arrayLike {
			copied, err := e.copyArray(objValue)
			return copied, err == nil, err
		}
		if isBuf {
			buf := srcValue.([]byte)
			return buf[:len(buf):len(buf)], false, nil
		}
		return &js.Array{}, true, nil
	}
	plain, err := e.isPlainObject(srcValue)
	if err != nil {
		return nil, false, err
	}
	if !plain {
		if plain, err = e.isArguments(srcValue); err != nil {
			return nil, false, err
		}
	}
	if !plain {
		return srcValue, false, nil
	}
	if args, err := e.isArguments(objValue); err != nil {
		return nil, false, err
	} else if args {
		copied, err := e.toPlainObject(objValue)
		return copied, err == nil, err
	}
	replace := !isObject(objValue)
	if !replace && srcIndex != 0 {
		if replace, err = e.isFunction(objValue); err != nil {
			return nil, false, err
		}
	}
	if replace {
		clone, err := e.initCloneObject(srcValue)
		return clone, err == nil, err
	}
	return objValue, true, nil
}
