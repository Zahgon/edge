package js

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/edge-js/edge-go/internal/jsdate"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// ToString is the ToString abstract operation for values whose conversion runs
// no JavaScript code (the realm's ToString calls user-defined toString and
// valueOf): arrays join their items with commas (null and undefined as ""),
// plain objects give "[object Object]", dates use Date.prototype.toString (local
// time), errors give "Name: message".
func ToString(v any) string {
	return toString(v, nil)
}

func toString(v any, seen []*Array) string {
	switch x := v.(type) {
	case UndefinedType:
		return "undefined"
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return NumberToString(x)
	case string:
		return x
	case BigInt:
		return x.String()
	case *Array:
		// Array.prototype.join skips an array already being joined (cycle detection).
		for _, s := range seen {
			if s == x {
				return ""
			}
		}
		seen = append(seen, x)
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			if !IsNullish(it) {
				parts[i] = toString(it, seen)
			}
		}
		return strings.Join(parts, ",")
	case *Date:
		return DateToString(x)
	case *RegExp:
		return "/" + x.Source + "/" + x.Flags
	case *Error:
		switch {
		case x.Name == "":
			return x.Message
		case x.Message == "":
			return x.Name
		}
		return x.Name + ": " + x.Message
	case *Function:
		return FunctionText(x)
	}
	return "[object Object]"
}

// FunctionSource is implemented by the Internal slots of functions whose
// Function.prototype.toString text is their own: the source text of a function
// defined in JavaScript, or the text a host gave its function. ok is false for
// the "[native code]" form.
type FunctionSource interface {
	FunctionSource() (text string, ok bool)
}

// FunctionText is Function.prototype.toString for f: the source text of a
// function defined in JavaScript (for one made by the Function constructor,
// V8's "function anonymous(params\n) {\nbody\n}"), else the form V8 gives
// builtin functions, "function name() { [native code] }".
func FunctionText(f *Function) string {
	if s, ok := f.Internal.(FunctionSource); ok {
		if text, ok := s.FunctionSource(); ok {
			return text
		}
	}
	return "function " + f.Name + "() { [native code] }"
}

// ToNumber is Number(v) for values whose conversion runs no JavaScript code: the
// ToNumber abstract operation, except that a BigInt converts to the nearest
// number (where ToNumber itself throws).
func ToNumber(v any) float64 {
	switch x := v.(type) {
	case UndefinedType:
		return math.NaN()
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case string:
		return StringToNumber(x)
	case BigInt:
		return x.Float64()
	case *Date:
		return x.Ms
	case *Array:
		return StringToNumber(ToString(x))
	}
	return math.NaN()
}

// ToIntegerOrInfinity is the ToIntegerOrInfinity abstract operation on a number:
// NaN is 0, infinities stay, anything else loses its fraction (and -0 its sign).
func ToIntegerOrInfinity(n float64) float64 {
	if math.IsNaN(n) {
		return 0
	}
	if math.IsInf(n, 0) {
		return n
	}
	return math.Trunc(n) + 0 // + 0 turns -0 into +0
}

// ToUint32 is the ToUint32 abstract operation on a number.
func ToUint32(n float64) uint32 {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	n = math.Mod(math.Trunc(n), 4294967296)
	if n < 0 {
		n += 4294967296
	}
	return uint32(n)
}

// ToInt32 is the ToInt32 abstract operation on a number, the conversion behind
// value | 0.
func ToInt32(n float64) int32 { return int32(ToUint32(n)) }

// StringToNumber is ToNumber applied to a string: the trimmed text must be a whole
// StringNumericLiteral; the empty string is 0 and anything else is NaN.
func StringToNumber(s string) float64 {
	s = jsstr.TrimSpace(s)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			return radixValue(s[2:], base)
		}
	}
	body := s
	if body[0] == '+' || body[0] == '-' {
		body = body[1:]
	}
	if body == "Infinity" {
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	if !isDecimalLiteral(body) {
		return math.NaN()
	}
	return DecimalValue(s)
}

// isDecimalLiteral reports whether s is an unsigned StrUnsignedDecimalLiteral
// without "Infinity": digits with an optional fraction and exponent.
func isDecimalLiteral(s string) bool {
	i, digits := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

// radixValue is the value of the digits of a 0x, 0o or 0b string literal,
// NaN when s is empty or holds anything else.
func radixValue(s string, base int) float64 {
	if s == "" {
		return math.NaN()
	}
	for _, c := range s {
		if d := jsstr.DigitValue(c); d < 0 || d >= base {
			return math.NaN()
		}
	}
	return DigitsValue(s, base)
}

// DigitsValue is the number that digits of the given base (10 or a power of
// two, already validated, without sign or prefix) denote, rounded once to the
// nearest double, ties to even, as numeric literals, Number(string) and
// parseInt round it.
func DigitsValue(digits string, base int) float64 {
	if base == 10 {
		return DecimalValue(digits)
	}
	n, ok := new(big.Int).SetString(digits, base)
	if !ok {
		panic("js: DigitsValue of digits not of base " + strconv.Itoa(base) + ": " + strconv.Quote(digits))
	}
	v, _ := new(big.Float).SetInt(n).Float64() // the accuracy is not needed
	return v
}

// DecimalValue is the number a decimal literal denotes: text is an optional
// sign, then digits with an optional fraction and an optional exponent, already
// validated. It is rounded once to the nearest double, ties to even, and is
// ±Infinity beyond the largest double and ±0 below the smallest, as numeric
// literals, Number(string), parseFloat and JSON.parse round it.
func DecimalValue(text string) float64 {
	// Valid text leaves ParseFloat only ErrRange, whose result is that rounding.
	v, err := strconv.ParseFloat(text, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic("js: DecimalValue of an invalid decimal literal " + strconv.Quote(text))
	}
	return v
}

// StrictEquals is the === operator.
func StrictEquals(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case UndefinedType:
		_, ok := b.(UndefinedType)
		return ok
	case nil:
		return b == nil
	}
	return a == b
}

// SameValueZero is the equality of Array.prototype.includes, Map and Set.
func SameValueZero(a, b any) bool {
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if ok1 && ok2 && math.IsNaN(x) && math.IsNaN(y) {
		return true
	}
	return StrictEquals(a, b)
}

// DateToString is Date.prototype.toString: the date in the local time zone, as in
// "Tue Jan 02 2024 10:00:00 GMT+0000 (Coordinated Universal Time)", or
// "Invalid Date".
func DateToString(d *Date) string { return jsdate.ToString(d.Ms) }

// Time returns the date as a time.Time in the local time zone, and false for an
// invalid date.
func (d *Date) Time() (time.Time, bool) {
	if math.IsNaN(d.Ms) {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(d.Ms)), true
}

// DecodeUTF8 decodes bytes as the WHATWG UTF-8 decoder does, which is what Node's
// fs.readFileSync(path, 'utf-8') uses: each maximal invalid subsequence becomes one U+FFFD. Go's []rune
// conversion would instead replace every invalid byte separately.
func DecodeUTF8(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			sb.WriteByte(c)
			i++
			continue
		}
		var need int
		lower, upper := byte(0x80), byte(0xBF)
		switch {
		case 0xC2 <= c && c <= 0xDF:
			need = 1
		case 0xE0 <= c && c <= 0xEF:
			need = 2
			if c == 0xE0 {
				lower = 0xA0
			} else if c == 0xED {
				upper = 0x9F
			}
		case 0xF0 <= c && c <= 0xF4:
			need = 3
			if c == 0xF0 {
				lower = 0x90
			} else if c == 0xF4 {
				upper = 0x8F
			}
		default:
			sb.WriteRune(utf8.RuneError)
			i++
			continue
		}
		j := i + 1
		for k := 0; k < need; k++ {
			if j >= len(b) || b[j] < lower || b[j] > upper {
				break
			}
			lower, upper = 0x80, 0xBF
			j++
		}
		if j-i == need+1 {
			sb.Write(b[i:j])
		} else {
			sb.WriteRune(utf8.RuneError)
		}
		i = j
	}
	return sb.String()
}
