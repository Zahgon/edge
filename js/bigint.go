package js

import (
	"math"
	"math/big"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// BigInt is a JavaScript BigInt: an integer of any size. It is a primitive
// value: two BigInts holding the same integer are equal with Go's ==, as they
// are === in JavaScript, so they also work as Go map keys.
type BigInt struct {
	// dec is the integer in decimal, with a leading "-" when negative and no
	// leading zeros.
	dec string
}

// maxBigIntBits is the largest number of bits V8 lets a BigInt have
// (BigInt::kMaxLengthBits); a larger result is a RangeError.
const maxBigIntBits = 1 << 30

// NewBigInt returns the BigInt holding x.
func NewBigInt(x *big.Int) BigInt { return BigInt{x.String()} }

// BigIntOf returns the BigInt holding n.
func BigIntOf(n int64) BigInt { return NewBigInt(big.NewInt(n)) }

// ParseBigInt returns the BigInt that the digits (in the given base, without
// prefix, sign or separators) denote; ok is false when they are not digits of
// that base.
func ParseBigInt(digits string, base int) (BigInt, bool) {
	x, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return BigInt{}, false
	}
	return NewBigInt(x), true
}

// Int returns the integer as a new big.Int.
func (b BigInt) Int() *big.Int {
	x := new(big.Int)
	if b.dec != "" {
		x.SetString(b.dec, 10)
	}
	return x
}

// String is ToString(b): the integer in decimal, without the n of a literal.
func (b BigInt) String() string {
	if b.dec == "" {
		return "0"
	}
	return b.dec
}

// Sign returns -1, 0 or 1 for a negative, zero or positive b.
func (b BigInt) Sign() int {
	switch {
	case b.dec == "" || b.dec == "0":
		return 0
	case b.dec[0] == '-':
		return -1
	}
	return 1
}

// Text is BigInt.prototype.toString(radix): b in the given base (2 to 36),
// with lower-case letters.
func (b BigInt) Text(radix int) string { return b.Int().Text(radix) }

// Float64 is Number(b): the nearest double, ties to even, ±Infinity beyond
// the largest finite double.
func (b BigInt) Float64() float64 {
	f, _ := new(big.Float).SetInt(b.Int()).Float64()
	return f
}

// StringToBigInt is StringToBigInt: the BigInt a string denotes, as BigInt(s)
// and comparisons with a string convert it. Surrounding white space is ignored,
// an empty string is 0n, and 0x, 0o and 0b prefixes are allowed without a sign;
// ok is false for any other text (fractions, exponents, Infinity).
func StringToBigInt(s string) (BigInt, bool) {
	s = jsstr.TrimSpace(s)
	if s == "" {
		return BigInt{"0"}, true
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] | 0x20 {
		case 'x':
			base = 16
		case 'o':
			base = 8
		case 'b':
			base = 2
		}
		if base != 0 {
			return parseBigIntDigits(s[2:], base)
		}
	}
	sign := ""
	if s[0] == '+' || s[0] == '-' {
		if s[0] == '-' {
			sign = "-"
		}
		s = s[1:]
	}
	v, ok := parseBigIntDigits(s, 10)
	if !ok {
		return BigInt{}, false
	}
	if sign == "-" {
		return NewBigInt(new(big.Int).Neg(v.Int())), true
	}
	return v, true
}

// parseBigIntDigits parses a non-empty run of digits of base and nothing else.
func parseBigIntDigits(s string, base int) (BigInt, bool) {
	if s == "" {
		return BigInt{}, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		d := -1
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c|0x20 >= 'a' && c|0x20 <= 'z':
			d = int(c|0x20-'a') + 10
		}
		if d < 0 || d >= base {
			return BigInt{}, false
		}
	}
	return ParseBigInt(s, base)
}

// NumberToBigInt is NumberToBigInt, BigInt(n) for a number: n must be an
// integer, else V8's RangeError.
func NumberToBigInt(n float64) (BigInt, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
		return BigInt{}, NewRangeError("The number " + NumberToString(n) + " cannot be converted to a BigInt because it is not an integer")
	}
	x, _ := new(big.Float).SetFloat64(n).Int(nil)
	return NewBigInt(x), nil
}

// CompareBigIntNumber compares b with the number n exactly: -1, 0 or 1, and
// ok false when n is NaN.
func CompareBigIntNumber(b BigInt, n float64) (cmp int, ok bool) {
	switch {
	case math.IsNaN(n):
		return 0, false
	case math.IsInf(n, 1):
		return -1, true
	case math.IsInf(n, -1):
		return 1, true
	}
	return new(big.Float).SetInt(b.Int()).Cmp(new(big.Float).SetFloat64(n)), true
}

// ErrBigIntMix is V8's TypeError for an operator applied to a BigInt and a
// value of another type.
func ErrBigIntMix() *Error {
	return NewTypeError("Cannot mix BigInt and other types, use explicit conversions")
}

// BigIntBinary applies the arithmetic, bitwise or shift operator op to two
// BigInts, with V8's errors: division by zero, a negative exponent, >>> and a
// result beyond V8's largest BigInt. A negative exponent is reported with
// V8's own wording, "undefined must be positive".
func BigIntBinary(op string, a, b BigInt) (BigInt, error) {
	x, y := a.Int(), b.Int()
	z := new(big.Int)
	switch op {
	case "+":
		z.Add(x, y)
	case "-":
		z.Sub(x, y)
	case "*":
		if x.BitLen()+y.BitLen() > maxBigIntBits {
			return BigInt{}, errBigIntTooBig()
		}
		z.Mul(x, y)
	case "/", "%":
		if y.Sign() == 0 {
			return BigInt{}, NewRangeError("Division by zero")
		}
		if op == "/" {
			z.Quo(x, y)
		} else {
			z.Rem(x, y)
		}
	case "**":
		if y.Sign() < 0 {
			return BigInt{}, NewRangeError("undefined must be positive")
		}
		if x.CmpAbs(big.NewInt(1)) <= 0 {
			// 0, 1 and -1 stay small whatever the exponent.
			if x.Sign() < 0 && y.Bit(0) == 0 {
				return BigIntOf(1), nil
			}
			if x.Sign() == 0 && y.Sign() == 0 {
				return BigIntOf(1), nil
			}
			return a, nil
		}
		if !y.IsInt64() || y.Int64() > maxBigIntBits || y.Int64()*int64(x.BitLen()-1) > maxBigIntBits {
			return BigInt{}, errBigIntTooBig()
		}
		z.Exp(x, y, nil)
	case "&":
		z.And(x, y)
	case "|":
		z.Or(x, y)
	case "^":
		z.Xor(x, y)
	case "<<", ">>":
		left := op == "<<"
		if y.Sign() < 0 {
			left = !left
			y.Neg(y)
		}
		if !left {
			if !y.IsInt64() || y.Int64() > int64(x.BitLen()) {
				// Every bit is shifted out: what is left is the sign.
				if x.Sign() < 0 {
					return BigIntOf(-1), nil
				}
				return BigIntOf(0), nil
			}
			z.Rsh(x, uint(y.Int64()))
			break
		}
		if x.Sign() == 0 {
			return a, nil
		}
		if !y.IsInt64() || y.Int64()+int64(x.BitLen()) > maxBigIntBits {
			return BigInt{}, errBigIntTooBig()
		}
		z.Lsh(x, uint(y.Int64()))
	case ">>>":
		return BigInt{}, NewTypeError("BigInts have no unsigned right shift, use >> instead")
	default:
		return BigInt{}, NewSyntaxError("Unexpected token '" + op + "'")
	}
	if z.BitLen() > maxBigIntBits {
		return BigInt{}, errBigIntTooBig()
	}
	return NewBigInt(z), nil
}

// errBigIntTooBig is V8's RangeError for a BigInt beyond its largest size.
func errBigIntTooBig() *Error { return NewRangeError("Maximum BigInt size exceeded") }

// Negate is -b.
func (b BigInt) Negate() BigInt { return NewBigInt(new(big.Int).Neg(b.Int())) }

// Not is ~b, -b - 1.
func (b BigInt) Not() BigInt { return NewBigInt(new(big.Int).Not(b.Int())) }

// AsIntN is BigInt.asIntN(bits, b): b modulo 2^bits as a signed integer.
func (b BigInt) AsIntN(bits uint64) BigInt {
	if bits == 0 {
		return BigIntOf(0)
	}
	x := b.Int()
	if uint64(x.BitLen()) < bits {
		return b
	}
	m := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	x.Mod(x, m)
	if x.Bit(int(bits-1)) == 1 {
		x.Sub(x, m)
	}
	return NewBigInt(x)
}

// AsUintN is BigInt.asUintN(bits, b): b modulo 2^bits.
func (b BigInt) AsUintN(bits uint64) (BigInt, error) {
	x := b.Int()
	if x.Sign() >= 0 && uint64(x.BitLen()) <= bits {
		return b, nil
	}
	if bits > maxBigIntBits {
		return BigInt{}, errBigIntTooBig()
	}
	m := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	return NewBigInt(x.Mod(x, m)), nil
}
