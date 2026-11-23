package lodash

import (
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Set is _.set(object, path, value): assigns value at path, creating the
// missing levels (an array when the next key is an index, a plain object
// otherwise; an existing object level is reused, a primitive one replaced), and
// returns object. Null and undefined objects are returned as they are, other
// primitives unchanged.
func Set(ops *jsval.Ops, object, path, value any) (any, error) {
	if js.IsNullish(object) || !isObject(object) {
		return object, nil
	}
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	p, err := e.castPath(path, object)
	if err != nil {
		return nil, err
	}
	return object, e.baseSet(object, p, value)
}

// baseSet is lodash's baseSet on an object with a cast path.
func (e *env) baseSet(object any, p path, value any) error {
	n, err := e.pathLen(p)
	if err != nil {
		return err
	}
	last := n - 1
	for i, cur := 0, object; !js.IsNullish(cur) && i < n; i++ {
		item, err := e.pathAt(p, i)
		if err != nil {
			return err
		}
		key, err := e.toKey(item)
		if err != nil {
			return err
		}
		newValue := value
		if i != last {
			current, err := e.ops.Get(cur, key)
			if err != nil {
				return err
			}
			newValue = current
			if !isObject(current) {
				next, err := e.pathAt(p, i+1)
				if err != nil {
					return err
				}
				idx, err := e.isIndex(next, maxSafeInteger)
				if err != nil {
					return err
				}
				if idx {
					newValue = &js.Array{}
				} else {
					newValue = js.NewObject()
				}
			}
		}
		if err := e.assignValue(cur, key, newValue); err != nil {
			return err
		}
		if cur, err = e.ops.Get(cur, key); err != nil {
			return err
		}
	}
	return nil
}

// Get is _.get(object, path, defaultValue): the value at path, or defaultValue
// when it resolves to undefined.
func Get(ops *jsval.Ops, object, path, defaultValue any) (any, error) {
	if js.IsNullish(object) {
		return defaultValue, nil
	}
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	p, err := e.castPath(path, object)
	if err != nil {
		return nil, err
	}
	v, err := e.baseGet(object, p)
	if err != nil {
		return nil, err
	}
	if v == js.Undefined {
		return defaultValue, nil
	}
	return v, nil
}

// Has is _.has(object, path): every key of path is an own property of the value
// before it (Object.prototype.hasOwnProperty), or the last one is an index below
// the length of an array or arguments object.
func Has(ops *jsval.Ops, object, path any) (bool, error) {
	if js.IsNullish(object) {
		return false, nil
	}
	e, err := newEnv(ops)
	if err != nil {
		return false, err
	}
	return e.hasPath(object, path, e.ops.HasOwn)
}

// hasIn is lodash's hasIn: path is in object, own or inherited.
func (e *env) hasIn(object, pathValue any) (bool, error) {
	if js.IsNullish(object) {
		return false, nil
	}
	return e.hasPath(object, pathValue, e.ops.HasProperty)
}

// flattenPaths is the flatRest of pick and omit: the path arguments with arrays
// and arguments objects among them spread one level.
func (e *env) flattenPaths(paths []any) ([]any, error) {
	var out []any
	for _, p := range paths {
		spread, err := e.isArray(p)
		if err == nil && !spread {
			spread, err = e.isArguments(p)
		}
		if err != nil {
			return nil, err
		}
		if !spread {
			out = append(out, p)
			continue
		}
		n, err := e.length(p)
		if err != nil {
			return nil, err
		}
		for i := 0; js.ToNumber(n) > float64(i); i++ {
			item, err := e.ops.Get(p, js.NumberToString(float64(i)))
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
	}
	return out, nil
}

// Pick is _.pick(object, ...paths): a new plain object holding, for every path
// (arrays and arguments objects among paths are spread one level) that is in
// object, own or inherited, the value at that path, stored at the same path.
func Pick(ops *jsval.Ops, object any, paths ...any) (any, error) {
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	flat, err := e.flattenPaths(paths)
	if err != nil {
		return nil, err
	}
	result := js.NewObject()
	if js.IsNullish(object) {
		return result, nil
	}
	for _, pathValue := range flat {
		p, err := e.castPath(pathValue, object)
		if err != nil {
			return nil, err
		}
		value, err := e.baseGet(object, p)
		if err != nil {
			return nil, err
		}
		in, err := e.hasIn(object, pathValue)
		if err != nil {
			return nil, err
		}
		if !in {
			continue
		}
		if p, err = e.castPath(pathValue, object); err != nil {
			return nil, err
		}
		if err := e.baseSet(result, p, value); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Omit is _.omit(object, ...paths): a new plain object with object's own and
// inherited enumerable properties, then every path (arrays and arguments objects
// among paths spread one level) deleted from it, last path first. When a path
// has several keys, the plain objects of the copy are cloned first (deeply;
// arrays and other objects stay shared), so object itself is never changed.
func Omit(ops *jsval.Ops, object any, paths ...any) (any, error) {
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	flat, err := e.flattenPaths(paths)
	if err != nil {
		return nil, err
	}
	var result any = js.NewObject()
	if js.IsNullish(object) {
		return result, nil
	}
	deep := false
	cast := make([]path, len(flat))
	for i, pathValue := range flat {
		if cast[i], err = e.castPath(pathValue, object); err != nil {
			return nil, err
		}
		if !deep {
			n, err := e.pathLen(cast[i])
			if err != nil {
				return nil, err
			}
			deep = n > 1
		}
	}
	keys, err := e.keysIn(object)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		v, err := e.ops.Get(object, k)
		if err != nil {
			return nil, err
		}
		if err := e.assignValue(result, k, v); err != nil {
			return nil, err
		}
	}
	if deep {
		if result, err = e.cloneOmit(result, &stack{}); err != nil {
			return nil, err
		}
	}
	for i := len(cast) - 1; i >= 0; i-- {
		if err := e.baseUnset(result, cast[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// cloneOmit is lodash's baseClone with omit's flags (deep, keysIn, symbols) and
// its customizer, which leaves every value but plain objects as it is: plain
// objects become new plain objects holding clones of their keysIn values. A
// plain object that is really a Map or a Set (its prototype replaced) makes
// lodash call set or add on the new plain object, which throws.
func (e *env) cloneOmit(value any, st *stack) (any, error) {
	plain, err := e.isPlainObject(value)
	if err != nil || !plain {
		return value, err
	}
	if cloned, ok := st.get(value); ok && js.ToBoolean(cloned) {
		return cloned, nil
	}
	result := js.NewObject()
	st.set(value, result)
	switch {
	case e.isSet(value):
		return result, e.forEachInto(value, result, "add", "i.add", st)
	case e.isMap(value):
		return result, e.forEachInto(value, result, "set", "i.set", st)
	}
	keys, err := e.keysIn(value)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		v, err := e.ops.Get(value, k)
		if err != nil {
			return nil, err
		}
		c, err := e.cloneOmit(v, st)
		if err != nil {
			return nil, err
		}
		if err := e.assignValue(result, k, c); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// forEachInto is baseClone's copy of a Set (method "add") or Map ("set"):
// value.forEach calling result[method] with the clone of each entry.
func (e *env) forEachInto(value any, result *js.Object, method, callee string, st *stack) error {
	cb := jsval.NewHostFunction("", 2, func(_ any, args []any) (any, error) {
		fn, err := e.ops.Get(result, method)
		if err != nil {
			return nil, err
		}
		c, err := e.cloneOmit(js.Arg(args, 0), st)
		if err != nil {
			return nil, err
		}
		callArgs := []any{c}
		if method == "set" {
			callArgs = []any{js.Arg(args, 1), c}
		}
		return e.ops.Call(fn, result, callArgs, callee)
	})
	_, err := e.ops.Invoke(value, "forEach", []any{cb}, "t.forEach")
	return err
}

// baseUnset is lodash's baseUnset: deletes the last key of path from the value
// at the rest of the path, when that value is not null or undefined.
func (e *env) baseUnset(object any, p path) error {
	n, err := e.pathLen(p)
	if err != nil {
		return err
	}
	parent := object
	if n >= 2 {
		size, err := e.pathLen(p)
		if err != nil {
			return err
		}
		parentPath := path{items: make([]any, max(size-1, 0))}
		for i := range parentPath.items {
			if parentPath.items[i], err = e.pathAt(p, i); err != nil {
				return err
			}
		}
		if parent, err = e.baseGet(object, parentPath); err != nil {
			return err
		}
	}
	if js.IsNullish(parent) {
		return nil
	}
	size, err := e.pathLen(p)
	if err != nil {
		return err
	}
	var last any = js.Undefined
	if size > 0 {
		if last, err = e.pathAt(p, size-1); err != nil {
			return err
		}
	}
	key, err := e.toKey(last)
	if err != nil {
		return err
	}
	return e.ops.Delete(parent, key, false)
}
