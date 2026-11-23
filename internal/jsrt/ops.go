package jsrt

import (
	"math"

	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/js"
)

// toPrimitive is the ToPrimitive abstract operation. hint is "string", "number" or
// "default"; Date objects treat "default" as "string". Objects use their (possibly
// user-defined) valueOf and toString methods.
func (r *Realm) toPrimitive(v any, hint string) (any, error) {
	if !js.IsObject(v) {
		return v, nil
	}
	if hint == "default" {
		hint = "number"
		if _, isDate := v.(*js.Date); isDate {
			hint = "string"
		}
	}
	order := [2]string{"valueOf", "toString"}
	if hint == "string" {
		order = [2]string{"toString", "valueOf"}
	}
	for _, name := range order {
		m, err := r.get(v, name)
		if err != nil {
			return nil, err
		}
		if f, ok := m.(*js.Function); ok {
			res, err := r.call(f, v, nil)
			if err != nil {
				return nil, err
			}
			if !js.IsObject(res) {
				return res, nil
			}
		}
	}
	return nil, js.NewTypeError("Cannot convert object to primitive value")
}

// toString is the ToString abstract operation. Primitives convert with
// js.ToString; objects go through ToPrimitive with hint string.
func (r *Realm) toString(v any) (string, error) {
	if js.IsObject(v) {
		p, err := r.toPrimitive(v, "string")
		if err != nil {
			return "", err
		}
		v = p
	}
	return js.ToString(v), nil
}

// toNumber is the ToNumber abstract operation; a BigInt is V8's TypeError.
func (r *Realm) toNumber(v any) (float64, error) {
	if js.IsObject(v) {
		p, err := r.toPrimitive(v, "number")
		if err != nil {
			return 0, err
		}
		v = p
	}
	if _, ok := v.(js.BigInt); ok {
		return 0, errBigIntToNumber()
	}
	return js.ToNumber(v), nil
}

// errBigIntToNumber is V8's TypeError for ToNumber of a BigInt.
func errBigIntToNumber() error {
	return js.NewTypeError("Cannot convert a BigInt value to a number")
}

// numberOf is Number(v): ToNumeric, with a BigInt converted to the nearest
// number (where ToNumber throws).
func (r *Realm) numberOf(v any) (float64, error) {
	n, err := r.toNumeric(v)
	if err != nil {
		return 0, err
	}
	if b, ok := n.(js.BigInt); ok {
		return b.Float64(), nil
	}
	return n.(float64), nil
}

// toNumeric is the ToNumeric abstract operation: a BigInt, or a number.
func (r *Realm) toNumeric(v any) (any, error) {
	if js.IsObject(v) {
		p, err := r.toPrimitive(v, "number")
		if err != nil {
			return nil, err
		}
		v = p
	}
	if b, ok := v.(js.BigInt); ok {
		return b, nil
	}
	return js.ToNumber(v), nil
}

// toPropertyKey converts a computed member key to the string the property is stored
// under; a BigInt key is its decimal digits.
func (r *Realm) toPropertyKey(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return js.NumberToString(x), nil
	}
	p, err := r.toPrimitive(v, "string")
	if err != nil {
		return "", err
	}
	return js.ToString(p), nil
}

// looseEquals is the == operator.
func (r *Realm) looseEquals(a, b any) (bool, error) { return js.LooselyEqual(a, b, r.toPrimitive) }

// lessThan is IsLessThan (see js.LessThan) with this realm's ToPrimitive.
func (r *Realm) lessThan(a, b any, leftFirst bool) (lt bool, ok bool, err error) {
	return js.LessThan(a, b, leftFirst, r.toPrimitive)
}

// relational evaluates < > <= >=.
func (r *Realm) relational(op string, a, b any) (bool, error) {
	switch op {
	case "<":
		lt, ok, err := r.lessThan(a, b, true)
		return ok && lt, err
	case ">":
		lt, ok, err := r.lessThan(b, a, false)
		return ok && lt, err
	case "<=":
		lt, ok, err := r.lessThan(b, a, false)
		return ok && !lt, err
	default: // ">="
		lt, ok, err := r.lessThan(a, b, true)
		return ok && !lt, err
	}
}

// add is the + operator.
func (r *Realm) add(a, b any) (any, error) { return js.Add(a, b, r.toPrimitive) }

// arith evaluates the numeric binary operators other than +, on numbers or
// on BigInts (which do not mix).
func (r *Realm) arith(op string, a, b any) (any, error) {
	na, err := r.toNumeric(a)
	if err != nil {
		return nil, err
	}
	nb, err := r.toNumeric(b)
	if err != nil {
		return nil, err
	}
	ba, aBig := na.(js.BigInt)
	bb, bBig := nb.(js.BigInt)
	switch {
	case aBig && bBig:
		return js.BigIntBinary(op, ba, bb)
	case aBig || bBig:
		return nil, js.ErrBigIntMix()
	}
	x, y := na.(float64), nb.(float64)
	switch op {
	case "-":
		return x - y, nil
	case "*":
		return x * y, nil
	case "/":
		return x / y, nil
	case "%":
		return jsMod(x, y), nil
	case "**":
		return jsnum.Pow(x, y), nil
	case "|":
		return float64(js.ToInt32(x) | js.ToInt32(y)), nil
	case "&":
		return float64(js.ToInt32(x) & js.ToInt32(y)), nil
	case "^":
		return float64(js.ToInt32(x) ^ js.ToInt32(y)), nil
	case "<<":
		return float64(js.ToInt32(x) << (js.ToUint32(y) & 31)), nil
	case ">>":
		return float64(js.ToInt32(x) >> (js.ToUint32(y) & 31)), nil
	case ">>>":
		return float64(js.ToUint32(x) >> (js.ToUint32(y) & 31)), nil
	}
	return nil, js.NewSyntaxError("Unexpected token '" + op + "'")
}

// jsMod is Number::remainder: the sign follows the dividend.
func jsMod(x, y float64) float64 {
	if math.IsInf(y, 0) && !math.IsInf(x, 0) && !math.IsNaN(x) {
		return x
	}
	return math.Mod(x, y)
}

// instanceOf is the instanceof operator for function right-hand sides.
func (r *Realm) instanceOf(v any, f *js.Function) bool {
	if b, ok := boundOf(f); ok {
		f = b.target
	}
	proto, _ := r.lookup(f, "prototype")
	return js.IsObject(proto) && js.IsObject(v) && r.inheritsFrom(v, proto)
}

// ErrNullishToObject is V8's TypeError for ToObject on undefined or null, as
// Object.keys(null), "x" in undefined and the like throw it.
func ErrNullishToObject() *js.Error {
	return js.NewTypeError("Cannot convert undefined or null to object")
}
