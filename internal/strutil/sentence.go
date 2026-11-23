package strutil

import (
	"math"
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Sentence is string.sentence(values) with string values and options already
// resolved: "" for no value, the value itself for one, the two joined with
// pairSeparator, else all but the last joined with separator and the last after
// lastSeparator. @poppinss/string's defaults are " and ", ", " and ", and ".
func Sentence(values []string, separator, lastSeparator, pairSeparator string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + pairSeparator + values[1]
	}
	return strings.Join(values[:len(values)-1], separator) + lastSeparator + values[len(values)-1]
}

// JSSentence is the sentence global: string.sentence(values, options), on any
// values (read through values.length, values[i], values.slice(0, -1).join(...)) and
// options (pairSeparator read with `options?.pairSeparator`, separator and
// lastSeparator through Object.assign of the defaults and options).
func JSSentence(ops *jsval.Ops, args []any) (any, error) {
	values, options := js.Arg(args, 0), js.Arg(args, 1)
	for _, n := range []float64{0, 1, 2} {
		length, err := lengthOf(ops, values)
		if err != nil {
			return nil, err
		}
		if !js.StrictEquals(length, n) {
			continue
		}
		switch n {
		case 0:
			return "", nil
		case 1:
			return elementOf(ops, values, 0)
		}
		return sentencePair(ops, values, options)
	}
	defaults := js.ObjectOf("separator", ", ", "lastSeparator", ", and ")
	assign, err := ops.BuiltinFunction("Object", "assign")
	if err != nil {
		return nil, err
	}
	normalized, err := ops.Realm.Call(assign, js.Undefined, []any{defaults, options})
	if err != nil {
		return nil, err
	}
	head, err := sliceAllButLast(ops, values)
	if err != nil {
		return nil, err
	}
	separator, err := ops.Get(normalized, "separator")
	if err != nil {
		return nil, err
	}
	joined, err := ops.Invoke(head, "join", []any{separator}, "values.slice(...).join")
	if err != nil {
		return nil, err
	}
	out, err := templateString(ops, joined)
	if err != nil {
		return nil, err
	}
	lastSeparator, err := ops.Get(normalized, "lastSeparator")
	if err != nil {
		return nil, err
	}
	sep, err := templateString(ops, lastSeparator)
	if err != nil {
		return nil, err
	}
	length, err := lengthOf(ops, values)
	if err != nil {
		return nil, err
	}
	n, err := ops.ToNumber(length)
	if err != nil {
		return nil, err
	}
	last, err := elementOf(ops, values, n-1)
	if err != nil {
		return nil, err
	}
	tail, err := templateString(ops, last)
	if err != nil {
		return nil, err
	}
	return out + sep + tail, nil
}

// sentencePair is `${values[0]}${options?.pairSeparator || " and "}${values[1]}`.
func sentencePair(ops *jsval.Ops, values, options any) (any, error) {
	first, err := elementOf(ops, values, 0)
	if err != nil {
		return nil, err
	}
	a, err := templateString(ops, first)
	if err != nil {
		return nil, err
	}
	var pair any = js.Undefined
	if !js.IsNullish(options) {
		if pair, err = ops.Get(options, "pairSeparator"); err != nil {
			return nil, err
		}
	}
	if !js.ToBoolean(pair) {
		pair = " and "
	}
	sep, err := templateString(ops, pair)
	if err != nil {
		return nil, err
	}
	second, err := elementOf(ops, values, 1)
	if err != nil {
		return nil, err
	}
	b, err := templateString(ops, second)
	if err != nil {
		return nil, err
	}
	return a + sep + b, nil
}

// sliceAllButLast is values.slice(0, -1).
func sliceAllButLast(ops *jsval.Ops, values any) (any, error) {
	return stringCall(ops, values, "slice", "values.slice", []any{0.0, -1.0}, func(s string) any {
		u := jsstr.ToUTF16(s)
		return jsstr.FromUTF16(u[:max(len(u)-1, 0)])
	})
}

// Ordinal is string.ordinal(value) for a number: its English ordinal, as
// "1st", "22nd", "13th". Infinite and NaN values throw.
func Ordinal(n float64) (string, error) {
	abs := math.Abs(n)
	if math.IsInf(abs, 0) || math.IsNaN(abs) {
		return "", errOrdinal()
	}
	return js.NumberToString(n) + ordinalSuffix(abs), nil
}

// JSOrdinal is the ordinal global: string.ordinal(value), where a string is read
// with Number.parseInt and anything else with Math.abs's ToNumber, and the suffix
// follows `${value}`.
func JSOrdinal(ops *jsval.Ops, args []any) (any, error) {
	value := js.Arg(args, 0)
	var n float64
	if s, ok := value.(string); ok {
		n = jsnum.ParseInt(s, 0)
	} else {
		var err error
		if n, err = ops.ToNumber(value); err != nil {
			return nil, err
		}
	}
	n = math.Abs(n)
	if math.IsInf(n, 0) || math.IsNaN(n) {
		return nil, errOrdinal()
	}
	text, err := templateString(ops, value)
	if err != nil {
		return nil, err
	}
	return text + ordinalSuffix(n), nil
}

func errOrdinal() error {
	return js.NewError("Cannot ordinalize invalid or infinite numbers")
}

// ordinalSuffix is the English ordinal suffix of the finite, non-negative n.
func ordinalSuffix(n float64) string {
	if percent := math.Mod(n, 100); percent >= 10 && percent <= 20 {
		return "th"
	}
	switch math.Mod(n, 10) {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}
