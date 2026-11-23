package jsrt

import (
	"math"
	"slices"
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsrt/unicase"
	"github.com/edge-js/edge-go/js"
)

// stringCodePoints splits s into the strings the string iterator yields: code
// points, with unpaired surrogates on their own.
func stringCodePoints(s string) []any {
	u := jsstr.ToUTF16(s)
	out := make([]any, 0, len(u))
	for i := 0; i < len(u); {
		_, n := jsstr.CodePointAt(u, i)
		out = append(out, jsstr.FromUTF16(u[i:i+n]))
		i += n
	}
	return out
}

// thisString coerces the receiver of a String.prototype method.
func (r *Realm) thisString(this any, method string) (string, error) {
	if js.IsNullish(this) {
		return "", js.NewTypeError("String.prototype." + method + " called on null or undefined")
	}
	return r.toString(this)
}

// intArg is ToIntegerOrInfinity(args[i]), with def for undefined.
func (r *Realm) intArg(args []any, i int, def float64) (float64, error) {
	v := js.Arg(args, i)
	if isUndefined(v) {
		return def, nil
	}
	n, err := r.toNumber(v)
	if err != nil {
		return 0, err
	}
	return js.ToIntegerOrInfinity(n), nil
}

// setupString installs String and String.prototype; the parts run in the
// order V8 defines the properties, which Object.getOwnPropertyNames reports.
func (r *Realm) setupString() {
	r.setupStringConstructor()
	r.setupStringAccess()
	r.setupStringSearch()
	r.setupStringPadding()
	r.setupStringSlicing()
	r.setupStringCaseAndTrim()
	r.setupStringRegExpMethods()
	r.setupStringWellFormed()
}

// setupStringConstructor installs String with its static methods, and
// String.prototype.toString and valueOf.
func (r *Realm) setupStringConstructor() {
	sp := r.stringProto
	toStr := func(args []any) (any, error) {
		if len(args) == 0 {
			return "", nil
		}
		return r.toString(args[0])
	}
	str := r.constructor("String", 1, sp, func(_ any, args []any) (any, error) { return toStr(args) }, func(args []any) (any, error) {
		s, err := toStr(args)
		if err != nil {
			return nil, err
		}
		return &primitiveObject{value: s}, nil
	})
	r.method(str, "fromCharCode", 1, func(_ any, args []any) (any, error) {
		u := make([]uint16, len(args))
		for i, a := range args {
			n, err := r.toNumber(a)
			if err != nil {
				return nil, err
			}
			u[i] = uint16(js.ToUint32(n))
		}
		return jsstr.FromUTF16(u), nil
	})
	r.method(str, "fromCodePoint", 1, func(_ any, args []any) (any, error) {
		var u []uint16
		for _, a := range args {
			n, err := r.toNumber(a)
			if err != nil {
				return nil, err
			}
			if n != math.Trunc(n) || n < 0 || n > 0x10FFFF {
				s, err := r.toString(a)
				if err != nil {
					return nil, err
				}
				return nil, js.NewRangeError("Invalid code point " + s)
			}
			u = jsstr.AppendCodePoint(u, rune(n))
		}
		return jsstr.FromUTF16(u), nil
	})
	r.method(str, "raw", 1, func(_ any, args []any) (any, error) {
		strs, err := r.getV(js.Arg(args, 0), "raw")
		if err != nil {
			return nil, err
		}
		if js.IsNullish(strs) {
			return nil, ErrNullishToObject()
		}
		parts, err := r.arrayLike(strs)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for i, p := range parts {
			s, err := r.toString(p)
			if err != nil {
				return nil, err
			}
			b.WriteString(s)
			if i+1 < len(parts) && i+1 < len(args) {
				s, err := r.toString(args[i+1])
				if err != nil {
					return nil, err
				}
				b.WriteString(s)
			}
		}
		return b.String(), nil
	})

	r.method(sp, "toString", 0, func(this any, _ []any) (any, error) { return thisPrimitive[string](this, "String", "toString") })
	r.method(sp, "valueOf", 0, func(this any, _ []any) (any, error) { return thisPrimitive[string](this, "String", "valueOf") })
}

// stringMethod installs a String.prototype method that receives the coerced
// receiver, also as UTF-16.
func (r *Realm) stringMethod(name string, length int, fn func(s string, u []uint16, args []any) (any, error)) {
	r.method(r.stringProto, name, length, func(this any, args []any) (any, error) {
		s, err := r.thisString(this, name)
		if err != nil {
			return nil, err
		}
		return fn(s, jsstr.ToUTF16(s), args)
	})
}

// setupStringAccess installs charAt, charCodeAt, codePointAt, at and concat.
func (r *Realm) setupStringAccess() {
	r.stringMethod("charAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return "", err
		}
		return jsstr.FromUTF16(u[int(i) : int(i)+1]), nil
	})
	r.stringMethod("charCodeAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return math.NaN(), err
		}
		return float64(u[int(i)]), nil
	})
	r.stringMethod("codePointAt", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil || i < 0 || i >= float64(len(u)) {
			return js.Undefined, err
		}
		cp, _ := jsstr.CodePointAt(u, int(i))
		return float64(cp), nil
	})
	r.stringMethod("at", 1, func(_ string, u []uint16, args []any) (any, error) {
		i, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			i += float64(len(u))
		}
		if i < 0 || i >= float64(len(u)) {
			return js.Undefined, nil
		}
		return jsstr.FromUTF16(u[int(i) : int(i)+1]), nil
	})
	r.stringMethod("concat", 1, func(s string, _ []uint16, args []any) (any, error) {
		var b strings.Builder
		b.WriteString(s)
		for _, a := range args {
			t, err := r.toString(a)
			if err != nil {
				return nil, err
			}
			b.WriteString(t)
		}
		return b.String(), nil
	})
}

// setupStringSearch installs includes, startsWith, endsWith, indexOf and
// lastIndexOf.
func (r *Realm) setupStringSearch() {
	// searchArg converts the search string of includes/startsWith/endsWith, which
	// refuse regular expressions.
	searchArg := func(name string, args []any) ([]uint16, error) {
		if _, isRE := js.Arg(args, 0).(*js.RegExp); isRE {
			return nil, js.NewTypeError("First argument to String.prototype." + name + " must not be a regular expression")
		}
		t, err := r.toString(js.Arg(args, 0))
		return jsstr.ToUTF16(t), err
	}
	r.stringMethod("includes", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("includes", args)
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		return jsstr.Index(u, search, js.RelativeIndex(math.Max(pos, 0), len(u))) >= 0, nil
	})
	r.stringMethod("startsWith", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("startsWith", args)
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		start := js.RelativeIndex(math.Max(pos, 0), len(u))
		return start+len(search) <= len(u) && slices.Equal(u[start:start+len(search)], search), nil
	})
	r.stringMethod("endsWith", 1, func(_ string, u []uint16, args []any) (any, error) {
		search, err := searchArg("endsWith", args)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		e := js.RelativeIndex(math.Max(end, 0), len(u))
		start := e - len(search)
		return start >= 0 && slices.Equal(u[start:e], search), nil
	})
	r.stringMethod("indexOf", 1, func(_ string, u []uint16, args []any) (any, error) {
		t, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		pos, err := r.intArg(args, 1, 0)
		if err != nil {
			return nil, err
		}
		return float64(jsstr.Index(u, jsstr.ToUTF16(t), js.RelativeIndex(math.Max(pos, 0), len(u)))), nil
	})
	r.stringMethod("lastIndexOf", 1, func(_ string, u []uint16, args []any) (any, error) {
		t, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		search := jsstr.ToUTF16(t)
		n, err := r.toNumber(js.Arg(args, 1))
		if err != nil {
			return nil, err
		}
		pos := math.Inf(1)
		if !math.IsNaN(n) {
			pos = js.ToIntegerOrInfinity(n)
		}
		return float64(jsstr.LastIndex(u, search, js.RelativeIndex(math.Max(pos, 0), len(u)))), nil
	})
}

// setupStringPadding installs padStart, padEnd and repeat.
func (r *Realm) setupStringPadding() {
	pad := func(atStart bool) func(string, []uint16, []any) (any, error) {
		return func(s string, u []uint16, args []any) (any, error) {
			maxLen, err := r.intArg(args, 0, 0)
			if err != nil {
				return nil, err
			}
			if maxLen <= float64(len(u)) {
				return s, nil
			}
			fill := " "
			if f := js.Arg(args, 1); !isUndefined(f) {
				if fill, err = r.toString(f); err != nil {
					return nil, err
				}
			}
			if fill == "" {
				return s, nil
			}
			if maxLen > js.MaxStringLength {
				return nil, js.InvalidStringLength()
			}
			fu := jsstr.ToUTF16(fill)
			need := int(maxLen) - len(u)
			filler := make([]uint16, 0, need)
			for len(filler) < need {
				filler = append(filler, fu[:min(len(fu), need-len(filler))]...)
			}
			if atStart {
				return jsstr.FromUTF16(filler) + s, nil
			}
			return s + jsstr.FromUTF16(filler), nil
		}
	}
	r.stringMethod("padStart", 1, pad(true))
	r.stringMethod("padEnd", 1, pad(false))
	r.stringMethod("repeat", 1, func(s string, _ []uint16, args []any) (any, error) {
		n, err := r.toNumber(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		count := js.ToIntegerOrInfinity(n)
		if count < 0 || math.IsInf(count, 1) {
			return nil, js.NewRangeError("Invalid count value: " + js.NumberToString(count))
		}
		if s == "" || count == 0 {
			return "", nil
		}
		if float64(jsstr.Length(s))*count > js.MaxStringLength {
			return nil, js.InvalidStringLength()
		}
		return strings.Repeat(s, int(count)), nil
	})
}

// setupStringSlicing installs slice, substring and substr.
func (r *Realm) setupStringSlicing() {
	r.stringMethod("slice", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		from, to := js.RelativeIndex(start, len(u)), js.RelativeIndex(end, len(u))
		if from >= to {
			return "", nil
		}
		return jsstr.FromUTF16(u[from:to]), nil
	})
	r.stringMethod("substring", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		end, err := r.intArg(args, 1, float64(len(u)))
		if err != nil {
			return nil, err
		}
		from := js.RelativeIndex(math.Max(start, 0), len(u))
		to := js.RelativeIndex(math.Max(end, 0), len(u))
		if from > to {
			from, to = to, from
		}
		return jsstr.FromUTF16(u[from:to]), nil
	})
	r.stringMethod("substr", 2, func(_ string, u []uint16, args []any) (any, error) {
		start, err := r.intArg(args, 0, 0)
		if err != nil {
			return nil, err
		}
		length, err := r.intArg(args, 1, math.Inf(1))
		if err != nil {
			return nil, err
		}
		from := js.RelativeIndex(start, len(u))
		n := math.Min(math.Max(length, 0), float64(len(u)-from))
		if n <= 0 {
			return "", nil
		}
		return jsstr.FromUTF16(u[from : from+int(n)]), nil
	})
}

// setupStringCaseAndTrim installs the case conversions and the trim methods.
func (r *Realm) setupStringCaseAndTrim() {
	sp := r.stringProto
	lower := func(_ string, u []uint16, _ []any) (any, error) { return jsstr.FromUTF16(unicase.ToLower(u)), nil }
	upper := func(_ string, u []uint16, _ []any) (any, error) { return jsstr.FromUTF16(unicase.ToUpper(u)), nil }
	r.stringMethod("toLowerCase", 0, lower)
	r.stringMethod("toLocaleLowerCase", 0, lower)
	r.stringMethod("toUpperCase", 0, upper)
	r.stringMethod("toLocaleUpperCase", 0, upper)
	trim := func(left, right bool) func(string, []uint16, []any) (any, error) {
		return func(s string, _ []uint16, _ []any) (any, error) {
			if left {
				s = strings.TrimLeftFunc(s, jsstr.IsSpace)
			}
			if right {
				s = strings.TrimRightFunc(s, jsstr.IsSpace)
			}
			return s, nil
		}
	}
	r.stringMethod("trim", 0, trim(true, true))
	trimStart := trim(true, false)
	trimEnd := trim(false, true)
	r.stringMethod("trimStart", 0, trimStart)
	r.stringMethod("trimEnd", 0, trimEnd)
	// trimLeft and trimRight are the same function objects as trimStart/trimEnd.
	r.define(sp, "trimLeft", sp.Get("trimStart"))
	r.define(sp, "trimRight", sp.Get("trimEnd"))
}

// setupStringRegExpMethods installs split, match, matchAll, search, replace
// and replaceAll.
func (r *Realm) setupStringRegExpMethods() {
	r.stringMethod("split", 2, func(s string, u []uint16, args []any) (any, error) {
		return r.split(s, u, args)
	})
	r.stringMethod("match", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringMatch(s, js.Arg(args, 0))
	})
	r.stringMethod("matchAll", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringMatchAll(s, js.Arg(args, 0))
	})
	r.stringMethod("search", 1, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringSearch(s, js.Arg(args, 0))
	})
	r.stringMethod("replace", 2, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringReplace(s, js.Arg(args, 0), js.Arg(args, 1), false)
	})
	r.stringMethod("replaceAll", 2, func(s string, _ []uint16, args []any) (any, error) {
		return r.stringReplace(s, js.Arg(args, 0), js.Arg(args, 1), true)
	})
}

// setupStringWellFormed installs isWellFormed and toWellFormed.
func (r *Realm) setupStringWellFormed() {
	// A string is well formed when it has no unpaired surrogate;
	// toWellFormed replaces each one with U+FFFD.
	r.stringMethod("isWellFormed", 0, func(s string, _ []uint16, _ []any) (any, error) {
		return jsstr.ToWellFormed(s) == s, nil
	})
	r.stringMethod("toWellFormed", 0, func(s string, _ []uint16, _ []any) (any, error) {
		return jsstr.ToWellFormed(s), nil
	})
}

// split is String.prototype.split for a string or regular expression separator.
func (r *Realm) split(s string, u []uint16, args []any) (any, error) {
	sep, limitArg := js.Arg(args, 0), js.Arg(args, 1)
	if re, ok := sep.(*js.RegExp); ok {
		return r.regexpSplit(re, s, limitArg)
	}
	limit := uint32(math.MaxUint32)
	if !isUndefined(limitArg) {
		n, err := r.toNumber(limitArg)
		if err != nil {
			return nil, err
		}
		limit = js.ToUint32(n)
	}
	if isUndefined(sep) {
		if limit == 0 {
			return js.NewArray(), nil
		}
		return js.NewArray(s), nil
	}
	sepStr, err := r.toString(sep)
	if err != nil {
		return nil, err
	}
	out := js.NewArray()
	for _, piece := range jsstr.Split(s, sepStr, limit) {
		out.Items = append(out.Items, piece)
	}
	return out, nil
}
