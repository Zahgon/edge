package jsrt

import (
	"math"
	"math/bits"
	"math/rand/v2"

	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/js"
)

// Number and Math.

func (r *Realm) setupNumber() {
	np := r.numberProto
	toNum := func(args []any) (any, error) {
		if len(args) == 0 {
			return 0.0, nil
		}
		return r.numberOf(args[0])
	}
	num := r.constructor("Number", 1, np, func(_ any, args []any) (any, error) { return toNum(args) }, func(args []any) (any, error) {
		n, err := toNum(args)
		if err != nil {
			return nil, err
		}
		return &primitiveObject{value: n}, nil
	})
	r.defineConstants(num,
		"MAX_SAFE_INTEGER", float64(1<<53-1),
		"MIN_SAFE_INTEGER", -float64(1<<53-1),
		"EPSILON", math.Pow(2, -52),
		"MAX_VALUE", math.MaxFloat64,
		"MIN_VALUE", math.SmallestNonzeroFloat64,
		"NaN", math.NaN(),
		"POSITIVE_INFINITY", math.Inf(1),
		"NEGATIVE_INFINITY", math.Inf(-1),
	)
	isInt := func(v any) (float64, bool) {
		n, ok := v.(float64)
		return n, ok && !math.IsInf(n, 0) && n == math.Trunc(n)
	}
	r.method(num, "isInteger", 1, func(_ any, args []any) (any, error) {
		_, ok := isInt(js.Arg(args, 0))
		return ok, nil
	})
	r.method(num, "isSafeInteger", 1, func(_ any, args []any) (any, error) {
		n, ok := isInt(js.Arg(args, 0))
		return ok && math.Abs(n) <= 1<<53-1, nil
	})
	r.method(num, "isFinite", 1, func(_ any, args []any) (any, error) {
		n, ok := js.Arg(args, 0).(float64)
		return ok && !math.IsNaN(n) && !math.IsInf(n, 0), nil
	})
	r.method(num, "isNaN", 1, func(_ any, args []any) (any, error) {
		n, ok := js.Arg(args, 0).(float64)
		return ok && math.IsNaN(n), nil
	})
	// Number.parseFloat and Number.parseInt are the global functions themselves.
	r.define(num, "parseFloat", r.global.Get("parseFloat"))
	r.define(num, "parseInt", r.global.Get("parseInt"))

	thisNumber := func(this any, method string) (float64, error) {
		return thisPrimitive[float64](this, "Number", method)
	}
	r.method(np, "valueOf", 0, func(this any, _ []any) (any, error) { return thisNumber(this, "valueOf") })
	r.method(np, "toString", 1, func(this any, args []any) (any, error) {
		x, err := thisNumber(this, "toString")
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
		return jsnum.ToStringRadix(x, int(radix)), nil
	})
	r.method(np, "toFixed", 1, func(this any, args []any) (any, error) {
		x, err := thisNumber(this, "toFixed")
		if err != nil {
			return nil, err
		}
		d, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		return jsnum.ToFixed(x, d)
	})
	r.method(np, "toPrecision", 1, func(this any, args []any) (any, error) {
		x, err := thisNumber(this, "toPrecision")
		if err != nil {
			return nil, err
		}
		if isUndefined(js.Arg(args, 0)) {
			return js.NumberToString(x), nil
		}
		p, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		return jsnum.ToPrecision(x, p)
	})
	r.method(np, "toExponential", 1, func(this any, args []any) (any, error) {
		x, err := thisNumber(this, "toExponential")
		if err != nil {
			return nil, err
		}
		f, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		return jsnum.ToExponential(x, f, !isUndefined(js.Arg(args, 0)))
	})
}

func (r *Realm) setupMath() {
	mo := r.staticObject("Math")
	r.defineConstants(mo,
		"E", math.E, "LN10", math.Ln10, "LN2", math.Ln2, "LOG10E", math.Log10E, "LOG2E", math.Log2E,
		"PI", math.Pi, "SQRT1_2", math.Sqrt2/2, "SQRT2", math.Sqrt2,
	)
	// unary installs a Math function of one number.
	unary := func(name string, f func(float64) float64) {
		r.method(mo, name, 1, func(_ any, args []any) (any, error) {
			x, err := r.toNumber(js.Arg(args, 0))
			if err != nil {
				return nil, err
			}
			return f(x), nil
		})
	}
	unary("abs", math.Abs)
	unary("acos", jsnum.Acos)
	unary("acosh", jsnum.Acosh)
	unary("asin", jsnum.Asin)
	unary("asinh", jsnum.Asinh)
	unary("atan", jsnum.Atan)
	unary("atanh", jsnum.Atanh)
	unary("cbrt", jsnum.Cbrt)
	unary("ceil", math.Ceil)
	unary("cos", jsnum.Cos)
	unary("cosh", jsnum.Cosh)
	unary("exp", jsnum.Exp)
	unary("expm1", jsnum.Expm1)
	unary("floor", math.Floor)
	unary("fround", func(x float64) float64 { return float64(float32(x)) })
	unary("log", jsnum.Log)
	unary("log1p", jsnum.Log1p)
	unary("log10", jsnum.Log10)
	unary("log2", jsnum.Log2)
	unary("sin", jsnum.Sin)
	unary("sinh", jsnum.Sinh)
	unary("sqrt", math.Sqrt)
	unary("tan", jsnum.Tan)
	unary("tanh", jsnum.Tanh)
	unary("trunc", math.Trunc)
	unary("round", jsnum.Round)
	unary("sign", func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		}
		return x // NaN, +0 and -0 are their own sign
	})
	unary("clz32", func(x float64) float64 { return float64(bits.LeadingZeros32(js.ToUint32(x))) })
	numbers := func(args []any) ([]float64, error) {
		out := make([]float64, len(args))
		for i, a := range args {
			n, err := r.toNumber(a)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	}
	r.method(mo, "max", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers(args)
		if err != nil {
			return nil, err
		}
		best := math.Inf(-1)
		for _, n := range ns {
			if math.IsNaN(n) {
				return math.NaN(), nil
			}
			if n > best || (n == 0 && best == 0 && !math.Signbit(n)) {
				best = n
			}
		}
		return best, nil
	})
	r.method(mo, "min", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers(args)
		if err != nil {
			return nil, err
		}
		best := math.Inf(1)
		for _, n := range ns {
			if math.IsNaN(n) {
				return math.NaN(), nil
			}
			if n < best || (n == 0 && best == 0 && math.Signbit(n)) {
				best = n
			}
		}
		return best, nil
	})
	r.method(mo, "pow", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers([]any{js.Arg(args, 0), js.Arg(args, 1)})
		if err != nil {
			return nil, err
		}
		return jsnum.Pow(ns[0], ns[1]), nil
	})
	r.method(mo, "atan2", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers([]any{js.Arg(args, 0), js.Arg(args, 1)})
		if err != nil {
			return nil, err
		}
		return jsnum.Atan2(ns[0], ns[1]), nil
	})
	r.method(mo, "imul", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers([]any{js.Arg(args, 0), js.Arg(args, 1)})
		if err != nil {
			return nil, err
		}
		return float64(js.ToInt32(ns[0]) * js.ToInt32(ns[1])), nil
	})
	r.method(mo, "hypot", 2, func(_ any, args []any) (any, error) {
		ns, err := numbers(args)
		if err != nil {
			return nil, err
		}
		return jsnum.Hypot(ns), nil
	})
	r.method(mo, "random", 0, func(any, []any) (any, error) {
		return rand.Float64(), nil
	})
}
