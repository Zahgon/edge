package jsrt

import (
	"math"
	"strings"

	"github.com/edge-js/edge-go/js"
)

// BigInt and BigInt.prototype.

func (r *Realm) setupBigInt() {
	bp := r.bigintProto
	bigint := r.constructor("BigInt", 1, bp, func(_ any, args []any) (any, error) {
		prim, err := r.toPrimitive(js.Arg(args, 0), "number")
		if err != nil {
			return nil, err
		}
		if n, ok := prim.(float64); ok {
			return js.NumberToBigInt(n)
		}
		return r.toBigInt(prim)
	}, nil)
	asN := func(name string, signed bool) {
		r.method(bigint, name, 2, func(_ any, args []any) (any, error) {
			bits, err := r.toIndex(js.Arg(args, 0))
			if err != nil {
				return nil, err
			}
			b, err := r.toBigInt(js.Arg(args, 1))
			if err != nil {
				return nil, err
			}
			if signed {
				return b.AsIntN(bits), nil
			}
			return b.AsUintN(bits)
		})
	}
	asN("asUintN", false)
	asN("asIntN", true)

	thisBigInt := func(this any, method string) (js.BigInt, error) {
		return thisPrimitive[js.BigInt](this, "BigInt", method)
	}
	r.method(bp, "toLocaleString", 0, func(this any, _ []any) (any, error) {
		b, err := thisBigInt(this, "toLocaleString")
		if err != nil {
			return nil, err
		}
		return groupThousands(b.String()), nil
	})
	r.method(bp, "toString", 0, func(this any, args []any) (any, error) {
		b, err := thisBigInt(this, "toString")
		if err != nil {
			return nil, err
		}
		radix, err := r.intArg(args, 0, 10)
		if err != nil {
			return nil, err
		}
		if radix < 2 || radix > 36 {
			return nil, js.NewRangeError("toString() radix argument must be between 2 and 36")
		}
		return b.Text(int(radix)), nil
	})
	r.method(bp, "valueOf", 0, func(this any, _ []any) (any, error) { return thisBigInt(this, "valueOf") })
}

// toBigInt is the ToBigInt abstract operation, with V8's errors for the values
// that do not convert (numbers among them: BigInt(n) converts a number
// itself).
func (r *Realm) toBigInt(v any) (js.BigInt, error) {
	prim, err := r.toPrimitive(v, "number")
	if err != nil {
		return js.BigInt{}, err
	}
	switch x := prim.(type) {
	case js.BigInt:
		return x, nil
	case bool:
		if x {
			return js.BigIntOf(1), nil
		}
		return js.BigIntOf(0), nil
	case string:
		b, ok := js.StringToBigInt(x)
		if !ok {
			return js.BigInt{}, js.NewSyntaxError("Cannot convert " + x + " to a BigInt")
		}
		return b, nil
	}
	return js.BigInt{}, js.NewTypeError("Cannot convert " + js.ToString(prim) + " to a BigInt")
}

// toIndex is the ToIndex abstract operation: an integer from 0 to 2^53 - 1,
// else V8's RangeError.
func (r *Realm) toIndex(v any) (uint64, error) {
	n, err := r.toNumber(v)
	if err != nil {
		return 0, err
	}
	n = js.ToIntegerOrInfinity(n)
	if n < 0 || n > 1<<53-1 || math.IsInf(n, 0) {
		return 0, js.NewRangeError("Invalid value: not (convertible to) a safe integer")
	}
	return uint64(n), nil
}

// groupThousands writes an integer's decimal digits in groups of three
// separated by commas, as toLocaleString does in the default en-US locale.
func groupThousands(dec string) string {
	sign := ""
	if strings.HasPrefix(dec, "-") {
		sign, dec = "-", dec[1:]
	}
	var b strings.Builder
	b.WriteString(sign)
	for i, c := range dec {
		if i > 0 && (len(dec)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
