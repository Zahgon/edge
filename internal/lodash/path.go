package lodash

import (
	"math"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

var (
	// reIsDeepProp and reIsPlainProp are lodash's isKey patterns.
	reIsDeepProp  = jsval.MustCompile(`\.|\[(?:[^[\]]*|(["'])(?:(?!\1)[^\\]|\\.)*?\1)\]`, "")
	reIsPlainProp = jsval.MustCompile(`^\w*$`, "")
	// rePropName splits a property path string; reEscapeChar unescapes quoted keys.
	rePropName   = jsval.MustCompile(`[^.[\]]+|\[(?:(-?\d+(?:\.\d+)?)|(["'])((?:(?!\2)[^\\]|\\.)*?)\2)\]|(?=(?:\.|\[\])(?:\.|\[\]|$))`, "g")
	reEscapeChar = jsval.MustCompile(`\\(\\)?`, "g")
)

// path is the result of lodash's castPath: either the caller's array, whose
// length and items are read from it each time, or a list lodash built.
type path struct {
	array any
	items []any
}

func (e *env) pathLen(p path) (int, error) {
	if p.array == nil {
		return len(p.items), nil
	}
	n, err := e.length(p.array)
	if err != nil {
		return 0, err
	}
	return int(js.ToNumber(n)), nil
}

func (e *env) pathAt(p path, i int) (any, error) {
	if p.array == nil {
		if i < len(p.items) {
			return p.items[i], nil
		}
		return js.Undefined, nil
	}
	return e.ops.Get(p.array, js.NumberToString(float64(i)))
}

// castPath is lodash's castPath: an array is the path itself, a key is a path of
// one item, anything else is parsed by stringToPath.
func (e *env) castPath(value, object any) (path, error) {
	isArr, err := e.isArray(value)
	if err != nil {
		return path{}, err
	}
	if isArr {
		return path{array: value}, nil
	}
	key, err := e.isKey(value, object)
	if err != nil {
		return path{}, err
	}
	if key {
		return path{items: []any{value}}, nil
	}
	s, err := e.toString(value)
	if err != nil {
		return path{}, err
	}
	return path{items: stringToPath(s)}, nil
}

// isKey is lodash's isKey: whether value names a property rather than a path.
// Numbers, booleans, null and undefined always do (as symbols would); strings and
// other values when their string form is a plain word, has no "." or brackets, or
// is a property of object.
func (e *env) isKey(value, object any) (bool, error) {
	switch js.TypeOf(value) {
	case "number", "boolean":
		return true, nil
	}
	if js.IsNullish(value) {
		return true, nil
	}
	s, err := e.ops.ToString(value)
	if err != nil || reIsPlainProp.Test(s) {
		return err == nil, err
	}
	if s, err = e.ops.ToString(value); err != nil || !reIsDeepProp.Test(s) {
		return err == nil, err
	}
	if js.IsNullish(object) {
		return false, nil
	}
	k, err := e.propertyKey(value)
	if err != nil {
		return false, err
	}
	return e.ops.HasProperty(object, k)
}

// propertyKey is ToPropertyKey: an object's string form with hint string.
func (e *env) propertyKey(v any) (string, error) {
	p, err := e.ops.ToPrimitive(v, "string")
	if err != nil {
		return "", err
	}
	return js.ToString(p), nil
}

// toString is lodash's toString as castPath uses it (never on arrays, which are
// paths already): "" for null and undefined, value + "" otherwise, keeping the
// sign of -0.
func (e *env) toString(v any) (string, error) {
	if js.IsNullish(v) {
		return "", nil
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	return e.keyString(v)
}

// keyString is `value + ""` with lodash's -0 rule: "-0" when the result is "0"
// and 1 / value is -Infinity.
func (e *env) keyString(v any) (string, error) {
	s, err := e.ops.AddString(v, "")
	if err != nil || s != "0" {
		return s, err
	}
	n, err := e.ops.ToNumber(v)
	if err != nil {
		return "", err
	}
	if math.IsInf(1/n, -1) {
		return "-0", nil
	}
	return s, nil
}

// toKey is lodash's toKey: strings as they are, other values as keyString makes
// them (the runtime has no symbols, which lodash would keep as they are).
func (e *env) toKey(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return e.keyString(v)
}

// stringToPath is lodash's stringToPath: the keys of a path string such as
// "a[0].b['c.d']". A leading "." yields an empty first key, numbers in brackets
// stay strings, quoted keys are unescaped, and empty segments ("a..b", "a[]")
// yield empty keys.
func stringToPath(s string) []any {
	units := jsstr.ToUTF16(s)
	var keys []any
	if len(units) > 0 && units[0] == '.' {
		keys = append(keys, "")
	}
	for pos := 0; pos <= len(units); {
		caps := rePropName.Exec(units, pos)
		if caps == nil {
			break
		}
		groups := jsregexp.Captures(units, caps)
		switch {
		case groups[2] != js.Undefined && js.ToBoolean(groups[2]):
			keys = append(keys, reEscapeChar.ReplaceString(groups[3].(string), "$1"))
		case groups[1] != js.Undefined && js.ToBoolean(groups[1]):
			keys = append(keys, groups[1])
		default:
			keys = append(keys, groups[0])
		}
		if caps[1] == caps[0] {
			pos = rePropName.AdvanceStringIndex(units, caps[1])
		} else {
			pos = caps[1]
		}
	}
	return keys
}

// baseGet is lodash's baseGet: follows path from object while the value is not
// null or undefined; undefined unless the whole path was followed.
func (e *env) baseGet(object any, p path) (any, error) {
	n, err := e.pathLen(p)
	if err != nil {
		return nil, err
	}
	i := 0
	for !js.IsNullish(object) && i < n {
		item, err := e.pathAt(p, i)
		if err != nil {
			return nil, err
		}
		i++
		key, err := e.toKey(item)
		if err != nil {
			return nil, err
		}
		if object, err = e.ops.Get(object, key); err != nil {
			return nil, err
		}
	}
	if i > 0 && i == n {
		return object, nil
	}
	return js.Undefined, nil
}

// hasPath is lodash's hasPath: every key of path is present according to has;
// a last index missing from an array or arguments object counts when it is
// below the length.
func (e *env) hasPath(object, pathValue any, has func(object any, key string) (bool, error)) (bool, error) {
	p, err := e.castPath(pathValue, object)
	if err != nil {
		return false, err
	}
	n, err := e.pathLen(p)
	if err != nil {
		return false, err
	}
	var (
		key    any = js.Undefined
		result bool
		broke  bool
		i      = 0
	)
	for ; i < n; i++ {
		item, err := e.pathAt(p, i)
		if err != nil {
			return false, err
		}
		k, err := e.toKey(item)
		if err != nil {
			return false, err
		}
		key = k
		result = false
		if !js.IsNullish(object) {
			if result, err = has(object, k); err != nil {
				return false, err
			}
		}
		if !result {
			broke = true
			break
		}
		if object, err = e.ops.Get(object, k); err != nil {
			return false, err
		}
	}
	if result || !broke || i+1 != n {
		return result, nil
	}
	var length any = 0.0
	if !js.IsNullish(object) {
		if length, err = e.length(object); err != nil {
			return false, err
		}
	}
	if !js.ToBoolean(length) || !isLength(length) {
		return false, nil
	}
	if idx, err := e.isIndex(key, length.(float64)); err != nil || !idx {
		return false, err
	}
	if isArr, err := e.isArray(object); err != nil || isArr {
		return isArr, err
	}
	return e.isArguments(object)
}
