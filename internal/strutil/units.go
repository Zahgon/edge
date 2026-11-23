package strutil

import (
	"math"
	"strings"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Durations: @lukeed/ms 2.0.2 as @poppinss/string bundles it.

const (
	msSec  = 1e3
	msMin  = msSec * 60
	msHour = msMin * 60
	msDay  = msHour * 24
	msYear = msDay * 365.25
)

var msPattern = jsval.MustCompile(`^(-?(?:\d+)?\.?\d+) *(m(?:illiseconds?|s(?:ecs?)?))?(s(?:ec(?:onds?|s)?)?)?(m(?:in(?:utes?|s)?)?)?(h(?:ours?|rs?)?)?(d(?:ays?)?)?(w(?:eeks?|ks?)?)?(y(?:ears?|rs?)?)?$`, "")

// FormatMilliseconds is string.milliseconds.format(ms, long): "1.5s" style
// rounding to the largest unit below the value, "2 minutes" when long.
func FormatMilliseconds(ms float64, long bool) string {
	out, err := formatMilliseconds(nil, ms, long)
	if err != nil {
		// Numbers convert without calling JavaScript code.
		panic(err)
	}
	return out
}

// JSFormatMilliseconds is the prettyMs global: string.milliseconds.format(ms,
// long). The comparisons, negation and division convert ms as JavaScript's
// operators do, once per operation.
func JSFormatMilliseconds(ops *jsval.Ops, args []any) (any, error) {
	return formatMilliseconds(ops, js.Arg(args, 0), js.ToBoolean(js.Arg(args, 1)))
}

// formatMilliseconds is @lukeed/ms's format(num, long). ops may be nil when num
// is a number.
func formatMilliseconds(ops *jsval.Ops, num any, long bool) (string, error) {
	negative, err := lessThan(ops, num, 0)
	if err != nil {
		return "", err
	}
	pfx := ""
	if negative {
		pfx = "-"
	}
	abs := num
	if negative, err = lessThan(ops, num, 0); err != nil {
		return "", err
	}
	if negative {
		n, err := toNumber(ops, num)
		if err != nil {
			return "", err
		}
		abs = -n
	}
	below, err := lessThan(ops, abs, msSec)
	if err != nil {
		return "", err
	}
	if below {
		suffix := "ms"
		if long {
			suffix = " ms"
		}
		return addStrings(ops, num, suffix)
	}
	for _, u := range []struct {
		limit, size float64
		name        string
	}{{msMin, msSec, "second"}, {msHour, msMin, "minute"}, {msDay, msHour, "hour"}, {msYear, msDay, "day"}} {
		if below, err = lessThan(ops, abs, u.limit); err != nil {
			return "", err
		}
		if below {
			return msUnit(ops, abs, u.size, pfx, u.name, long)
		}
	}
	return msUnit(ops, abs, msYear, pfx, "year", long)
}

// msUnit is fmt(abs / size, pfx, name, long): the value rounded half up unless it
// is an int32 already, then the unit's initial or its long name, plural unless 1.
func msUnit(ops *jsval.Ops, abs any, size float64, pfx, name string, long bool) (string, error) {
	n, err := toNumber(ops, abs)
	if err != nil {
		return "", err
	}
	val := n / size
	num := val
	if float64(js.ToInt32(val)) != val {
		num = float64(js.ToInt32(val + .5))
	}
	if !long {
		return pfx + js.NumberToString(num) + name[:1], nil
	}
	if num != 1 {
		name += "s"
	}
	return pfx + js.NumberToString(num) + " " + name, nil
}

// lessThan is `a < b` for a number b.
func lessThan(ops *jsval.Ops, a any, b float64) (bool, error) {
	if x, ok := a.(float64); ok {
		return x < b, nil
	}
	lt, ok, err := js.LessThan(a, b, true, ops.ToPrimitive)
	return ok && lt, err
}

// toNumber is ToNumber(v), native for a number.
func toNumber(ops *jsval.Ops, v any) (float64, error) {
	if x, ok := v.(float64); ok {
		return x, nil
	}
	return ops.ToNumber(v)
}

// addStrings is `a + b` for a string b.
func addStrings(ops *jsval.Ops, a any, b string) (string, error) {
	switch x := a.(type) {
	case float64:
		return js.NumberToString(x) + b, nil
	case string:
		return js.Concat(x, b)
	}
	return ops.AddString(a, b)
}

// ParseMilliseconds is string.milliseconds.parse(duration) for a string: the
// duration in milliseconds, or the Error `Invalid duration expression "..."` when
// @lukeed/ms's parse returns undefined (no match, or a zero or NaN amount).
func ParseMilliseconds(duration string) (float64, error) {
	if n, ok := parseMilliseconds(jsval.ToLowerCase(duration)); ok {
		return n, nil
	}
	return 0, invalidDuration(duration)
}

func invalidDuration(text string) error {
	return js.NewError(`Invalid duration expression "` + text + `"`)
}

// JSParseMilliseconds is the toMs global: string.milliseconds.parse(duration),
// which returns numbers as they are and runs `val.toLowerCase().match(RGX)` on
// anything else.
func JSParseMilliseconds(ops *jsval.Ops, args []any) (any, error) {
	duration := js.Arg(args, 0)
	if n, ok := duration.(float64); ok {
		return n, nil
	}
	lowered, err := stringCall(ops, duration, "toLowerCase", "val.toLowerCase", nil, func(s string) any { return jsval.ToLowerCase(s) })
	if err != nil {
		return nil, err
	}
	s, err := templateString(ops, lowered)
	if err != nil {
		return nil, err
	}
	if n, ok := parseMilliseconds(s); ok {
		return n, nil
	}
	text, err := templateString(ops, duration)
	if err != nil {
		return nil, err
	}
	return nil, invalidDuration(text)
}

// parseMilliseconds is @lukeed/ms's parse on the lowercased text; ok is false
// where it returns undefined.
func parseMilliseconds(s string) (float64, bool) {
	units := jsstr.ToUTF16(s)
	caps := msPattern.Exec(units, 0)
	if caps == nil {
		return 0, false
	}
	groups := jsregexp.Captures(units, caps)
	num := jsnum.ParseFloat(js.ToString(groups[1]))
	if num == 0 || math.IsNaN(num) {
		return 0, false
	}
	for i, size := range []float64{msSec, msMin, msHour, msDay, msDay * 7, msYear} {
		if groups[3+i] != js.Undefined {
			return num * size, true
		}
	}
	return num, true
}

// Sizes: the bytes 3.1.2 package as @poppinss/string bundles it.

var (
	bytesThousands = jsval.MustCompile(`\B(?=(\d{3})+(?!\d))`, "g")
	bytesDecimals  = jsval.MustCompile(`(?:\.0*|(\.[^0]+)0+)$`, "")
	bytesPattern   = jsval.MustCompile(`^((-|\+)?(\d+(?:\.\d+)?)) *(kb|mb|gb|tb|pb)$`, "i")
)

// bytesMap is the package's map of lowercase units to sizes; ok reports
// whether `map[key]` is truthy. The map is a plain object, so "constructor" and
// "__proto__" find Object.prototype's members, which divide to NaN.
func bytesMap(key string) (size float64, ok bool) {
	switch key {
	case "b":
		return 1, true
	case "kb":
		return 1024, true
	case "mb":
		return 1 << 20, true
	case "gb":
		return 1 << 30, true
	case "tb":
		return 1 << 40, true
	case "pb":
		return 1 << 50, true
	case "constructor", "__proto__":
		return math.NaN(), true
	}
	return 0, false
}

// BytesOptions are the options of string.bytes.format. DecimalPlaces is 2 by
// default (DefaultBytesOptions).
type BytesOptions struct {
	DecimalPlaces      float64
	FixedDecimals      bool
	ThousandsSeparator string
	UnitSeparator      string
	Unit               string
}

// DefaultBytesOptions are the options bytes.format uses when given none.
var DefaultBytesOptions = BytesOptions{DecimalPlaces: 2}

// FormatBytes is string.bytes.format(value, options): value scaled to opts.Unit
// (or the largest binary unit not above it), with toFixed(DecimalPlaces), trailing
// zeros dropped unless FixedDecimals, and the separators. ok is false for NaN and
// infinities, where the package returns null. A DecimalPlaces outside 0 to 100
// throws toFixed's RangeError.
func FormatBytes(value float64, opts BytesOptions) (out string, ok bool, err error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", false, nil
	}
	unit := opts.Unit
	if _, known := bytesMap(jsval.ToLowerCase(unit)); unit == "" || !known {
		unit = magnitudeUnit(value)
	}
	size, _ := bytesMap(jsval.ToLowerCase(unit))
	str, err := bytesFixed(value/size, opts.DecimalPlaces, opts.FixedDecimals)
	if err != nil {
		return "", false, err
	}
	if opts.ThousandsSeparator != "" {
		str = groupThousands(str, func(head string) (string, error) {
			return bytesThousands.ReplaceString(head, opts.ThousandsSeparator), nil
		})
	}
	return str + opts.UnitSeparator + unit, true, nil
}

// magnitudeUnit is the largest unit not above |value|.
func magnitudeUnit(value float64) string {
	mag := math.Abs(value)
	for _, u := range []string{"PB", "TB", "GB", "MB", "KB"} {
		if size, _ := bytesMap(strings.ToLower(u)); mag >= size {
			return u
		}
	}
	return "B"
}

// bytesFixed is `x.toFixed(decimalPlaces)`, with the trailing zeros (and a bare
// point) removed unless fixed.
func bytesFixed(x, decimalPlaces float64, fixed bool) (string, error) {
	str, err := jsnum.ToFixed(x, js.ToIntegerOrInfinity(decimalPlaces))
	if err != nil {
		return "", err
	}
	if !fixed {
		str = bytesDecimals.ReplaceString(str, "$1")
	}
	return str, nil
}

// groupThousands is `str.split(".").map((s, i) => i === 0 ? s.replace(...) :
// s).join(".")` with the replacement of the integer part done by replace.
func groupThousands(str string, replace func(head string) (string, error)) string {
	parts := jsstr.Split(str, ".", 1<<32-1)
	head, err := replace(parts[0])
	if err != nil {
		panic(err)
	}
	parts[0] = head
	return strings.Join(parts, ".")
}

// JSFormatBytes is the prettyBytes global: string.bytes.format(value, options),
// reading options as the package does (thousandsSeparator, unitSeparator,
// decimalPlaces (twice when defined), fixedDecimals, unit) and calling
// unit.toLowerCase() on a user unit. A function thousandsSeparator is a replace
// callback.
func JSFormatBytes(ops *jsval.Ops, args []any) (any, error) {
	v, options := js.Arg(args, 0), js.Arg(args, 1)
	value, isNumber := v.(float64)
	if !isNumber || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, nil
	}
	read := func(key string, fallback any) (any, error) {
		x, err := getIfTruthy(ops, options, key)
		if err != nil || js.ToBoolean(x) {
			return x, err
		}
		return fallback, nil
	}
	thousands, err := read("thousandsSeparator", "")
	if err != nil {
		return nil, err
	}
	unitSeparator, err := read("unitSeparator", "")
	if err != nil {
		return nil, err
	}
	var decimalPlaces any = 2.0
	if js.ToBoolean(options) {
		dp, err := ops.Get(options, "decimalPlaces")
		if err != nil {
			return nil, err
		}
		if dp != js.Undefined {
			if decimalPlaces, err = ops.Get(options, "decimalPlaces"); err != nil {
				return nil, err
			}
		}
	}
	fixed, err := getIfTruthy(ops, options, "fixedDecimals")
	if err != nil {
		return nil, err
	}
	unit, err := read("unit", "")
	if err != nil {
		return nil, err
	}
	lowerUnit := func() (string, error) {
		l, err := stringCall(ops, unit, "toLowerCase", "unit.toLowerCase", nil, func(s string) any { return jsval.ToLowerCase(s) })
		if err != nil {
			return "", err
		}
		return templateString(ops, l)
	}
	known := false
	if js.ToBoolean(unit) {
		key, err := lowerUnit()
		if err != nil {
			return nil, err
		}
		_, known = bytesMap(key)
	}
	if !known {
		unit = magnitudeUnit(value)
	}
	key, err := lowerUnit()
	if err != nil {
		return nil, err
	}
	size, _ := bytesMap(key)
	places, err := ops.ToNumber(decimalPlaces)
	if err != nil {
		return nil, err
	}
	str, err := bytesFixed(value/size, places, js.ToBoolean(fixed))
	if err != nil {
		return nil, err
	}
	if js.ToBoolean(thousands) {
		if str, err = jsGroupThousands(ops, str, thousands); err != nil {
			return nil, err
		}
	}
	withSeparator, err := ops.Add(str, unitSeparator)
	if err != nil {
		return nil, err
	}
	return ops.Add(withSeparator, unit)
}

// jsGroupThousands is the thousands grouping with a separator of any type: a
// function is called as a replace callback (match, group, offset, string) and
// its result converted, anything else is converted to a replacement template.
func jsGroupThousands(ops *jsval.Ops, str string, separator any) (string, error) {
	parts := jsstr.Split(str, ".", 1<<32-1)
	var head string
	var err error
	if f, ok := separator.(*js.Function); ok {
		head, err = ReplaceFunc(bytesThousands, parts[0], func(c []any, index int) (string, error) {
			r, err := ops.Realm.Call(f, js.Undefined, append(c, float64(index), parts[0]))
			if err != nil {
				return "", err
			}
			return ops.ToString(r)
		})
	} else {
		var template string
		if template, err = ops.ToString(separator); err == nil {
			head = bytesThousands.ReplaceString(parts[0], template)
		}
	}
	if err != nil {
		return "", err
	}
	parts[0] = head
	return strings.Join(parts, "."), nil
}

// ParseBytes is string.bytes.parse(text) for a string: the number of bytes, ok
// false where the package returns null.
func ParseBytes(text string) (float64, bool) {
	units := jsstr.ToUTF16(text)
	value, unit := 0.0, "b"
	if caps := bytesPattern.Exec(units, 0); caps == nil {
		value = jsnum.ParseInt(text, 10)
	} else {
		g := jsregexp.Captures(units, caps)
		value = jsnum.ParseFloat(g[1].(string))
		unit = jsval.ToLowerCase(g[4].(string))
	}
	if math.IsNaN(value) {
		return 0, false
	}
	size, _ := bytesMap(unit)
	return math.Floor(size * value), true
}

// JSParseBytes is the toBytes global: string.bytes.parse(unit): numbers as they
// are, strings parsed, null for anything else.
func JSParseBytes(_ *jsval.Ops, args []any) (any, error) {
	switch x := js.Arg(args, 0).(type) {
	case float64:
		return x, nil
	case string:
		if n, ok := ParseBytes(x); ok {
			return n, nil
		}
	}
	return nil, nil
}
