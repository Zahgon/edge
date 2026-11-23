package jsnum

import "math"

// Pow is Math.pow and the ** operator: V8's math::pow, which settles the cases
// where ECMAScript and C differ and the two its optimizing compilers
// strength-reduce, then calls the C library's pow. Node's C library is glibc,
// whose pow (sysdeps/ieee754/dbl-64/e_pow.c, the ARM optimized-routines
// algorithm) glibcPow ports as it runs on arm64: with fused multiply-adds
// where it asks for them and where GCC contracts them (see fdlibm.go), and
// with round-half-away rounding to integers.
func Pow(x, y float64) float64 {
	switch {
	case math.IsNaN(y):
		return math.NaN()
	case math.IsInf(y, 0) && (x == 1 || x == -1):
		// ECMAScript, unlike C, gives NaN here.
		return math.NaN()
	case y == 2:
		return x * x
	case y == 0.5:
		if math.IsInf(x, 0) {
			return math.Inf(1)
		}
		return math.Sqrt(x + 0) // +0 for -0
	}
	return glibcPow(x, y)
}

// Constants of glibc's pow.
const (
	powLn2Hi = 0x1.62e42fefa3800p-1
	powLn2Lo = 0x1.ef35793c76730p-45
	// powOff is the bit pattern of 0x1.69555p-1: log_inline reduces x to
	// z in [powOff, 2*powOff).
	powOff = 0x3fe6955500000000
	// signBias marks a negative result in exp_inline's scale.
	signBias = 0x800 << 7
	// N/ln2 and -ln2/N in two parts, for N = 128.
	invLn2N   = 0x1.71547652b82fep0 * 128
	negLn2HiN = -0x1.62e42fefa0000p-8
	negLn2LoN = -0x1.cf79abc9e3b3ap-47
)

// powLogPoly holds the coefficients of log1p(r) - r around 0, scaled as
// log_inline evaluates them.
var powLogPoly = [7]float64{
	-0x1p-1,
	0x1.555555555556p-2 * -2,
	-0x1.0000000000006p-2 * -2,
	0x1.999999959554ep-3 * 4,
	-0x1.555555529a47ap-3 * 4,
	0x1.2495b9b4845e9p-3 * -8,
	-0x1.0002b8b263fc3p-3 * -8,
}

// expPoly holds the coefficients C2..C5 of exp(r) - 1 - r.
var expPoly = [4]float64{
	0x1.ffffffffffdbdp-2,
	0x1.555555555543cp-3,
	0x1.55555cf172b91p-5,
	0x1.1111167a4d017p-7,
}

// top12 returns the sign and exponent bits of x.
func top12(x float64) uint32 { return uint32(math.Float64bits(x) >> 52) }

// powLog is log_inline: log(x) as hi + lo, with lo carrying about 15 more
// bits; ix is the bit pattern of x, normalized when x is subnormal.
func powLog(ix uint64) (hi, lo float64) {
	A := &powLogPoly
	tmp := ix - powOff
	i := (tmp >> (52 - 7)) % 128
	k := int64(tmp) >> 52 // arithmetic shift
	iz := ix - tmp&(0xfff<<52)
	z := math.Float64frombits(iz)
	kd := float64(k)
	// log(x) = k*Ln2 + log(c) + log1p(z/c-1).
	e := &powLogTable[i]
	r := math.FMA(z, e.invc, -1.0)
	// k*Ln2 + log(c) + r.
	t1 := math.FMA(kd, powLn2Hi, e.logc)
	t2 := t1 + r
	lo1 := math.FMA(kd, powLn2Lo, e.logctail)
	lo2 := t1 - t2 + r
	ar := float64(A[0] * r) // A[0] = -0.5
	ar2 := float64(r * ar)
	ar3 := float64(r * ar2)
	// k*Ln2 + log(c) + r + A[0]*r*r.
	h := t2 + ar2
	lo3 := math.FMA(ar, r, -ar2)
	lo4 := t2 - h + ar2
	// p = log1p(r) - r - A[0]*r*r, fused into the sum of the low parts.
	q := math.FMA(ar2, math.FMA(ar2, math.FMA(r, A[6], A[5]), math.FMA(r, A[4], A[3])), math.FMA(r, A[2], A[1]))
	l := math.FMA(ar3, q, lo1+lo2+lo3+lo4)
	y := h + l
	return y, h - y + l
}

// powExp is exp_inline: sign*exp(x+xtail) for |xtail| < 2^-8/N, the sign
// negative when signBias is set in sign.
func powExp(x, xtail float64, sign uint64) float64 {
	abstop := top12(x) & 0x7ff
	if abstop-top12(0x1p-54) >= top12(512.0)-top12(0x1p-54) {
		if abstop-top12(0x1p-54) >= 0x80000000 {
			// tiny x: avoid spurious underflow; 0 is common input.
			one := 1.0 + x
			if sign != 0 {
				return -one
			}
			return one
		}
		if abstop >= top12(1024.0) {
			// Infinity and NaN are already handled.
			if math.Float64bits(x)>>63 != 0 {
				return powUnderflow(sign)
			}
			return powOverflow(sign)
		}
		// Large x is special cased below.
		abstop = 0
	}
	// exp(x) = 2^(k/N) * exp(r), with exp(r) in [2^(-1/2N),2^(1/2N)].
	z := float64(invLn2N * x)
	kd := math.Round(z) // half away from zero, as arm64's frinta
	ki := uint64(int64(int32(kd)))
	r := math.FMA(kd, negLn2LoN, math.FMA(kd, negLn2HiN, x))
	r += xtail
	// 2^(k/N) ~= scale * (1 + tail).
	idx := 2 * (ki % 128)
	top := (ki + sign) << (52 - 7)
	tail := math.Float64frombits(expTable[idx])
	// This is only a valid scale when -1023*N < k < 1024*N.
	sbits := expTable[idx+1] + top
	// exp(x) = 2^(k/N) * exp(r) ~= scale + scale * (tail + exp(r) - 1).
	r2 := float64(r * r)
	C := &expPoly
	tmp := math.FMA(float64(r2*r2), math.FMA(r, C[3], C[2]), math.FMA(r2, math.FMA(r, C[1], C[0]), tail+r))
	if abstop == 0 {
		return powExpSpecial(tmp, sbits, ki)
	}
	scale := math.Float64frombits(sbits)
	return math.FMA(scale, tmp, scale)
}

// powExpSpecial is exp_inline's specialcase: scale*(1+tmp) when it may
// overflow or underflow. sbits is scale's bit pattern, whose exponent may
// have overflowed into the sign bit; ki is k, negative when the result may
// underflow.
func powExpSpecial(tmp float64, sbits, ki uint64) float64 {
	if ki&0x80000000 == 0 {
		// k > 0: the exponent of scale might have overflowed by <= 460.
		sbits -= 1009 << 52
		scale := math.Float64frombits(sbits)
		return 0x1p1009 * math.FMA(scale, tmp, scale)
	}
	// k < 0: the subnormal range needs care.
	sbits += 1022 << 52
	scale := math.Float64frombits(sbits) // signed
	st := float64(scale * tmp)
	y := scale + st
	if math.Abs(y) < 1.0 {
		// Round y to the right precision before scaling it into the
		// subnormal range, avoiding double rounding.
		one := 1.0
		if y < 0.0 {
			one = -1.0
		}
		lo := scale - y + st
		hi := one + y
		lo = one - hi + y + lo
		y = (hi + lo) - one
		if y == 0 { // fix the sign of 0
			y = math.Float64frombits(sbits & 0x8000000000000000)
		}
	}
	return 0x1p-1022 * y
}

// powOverflow and powUnderflow are __math_oflow and __math_uflow: an
// infinity or zero with the sign sign selects.
func powOverflow(sign uint64) float64 {
	if sign != 0 {
		return math.Inf(-1)
	}
	return math.Inf(1)
}

func powUnderflow(sign uint64) float64 {
	if sign != 0 {
		return math.Copysign(0, -1)
	}
	return 0
}

// checkInt reports what kind of number the bits iy of a non-zero finite
// double are: 0 not an integer, 1 an odd integer, 2 an even integer.
func checkInt(iy uint64) int {
	e := int(iy >> 52 & 0x7ff)
	switch {
	case e < 0x3ff:
		return 0
	case e > 0x3ff+52:
		return 2
	case iy&(1<<(0x3ff+52-e)-1) != 0:
		return 0
	case iy&(1<<(0x3ff+52-e)) != 0:
		return 1
	}
	return 2
}

// zeroInfNaN reports whether the bits i are those of 0, an infinity or NaN.
func zeroInfNaN(i uint64) bool {
	return 2*i-1 >= 2*math.Float64bits(math.Inf(1))-1
}

// glibcPow is glibc's __pow.
func glibcPow(x, y float64) float64 {
	var sign uint64
	ix, iy := math.Float64bits(x), math.Float64bits(y)
	topx, topy := top12(x), top12(y)
	one := math.Float64bits(1.0)
	inf := math.Float64bits(math.Inf(1))
	if topx-0x001 >= 0x7ff-0x001 || (topy&0x7ff)-0x3be >= 0x43e-0x3be {
		// Special cases: (x < 0x1p-126 or inf or nan) or
		// (|y| < 0x1p-65 or |y| >= 0x1p63 or nan).
		if zeroInfNaN(iy) {
			switch {
			case 2*iy == 0, ix == one:
				return 1.0
			case 2*ix > 2*inf || 2*iy > 2*inf:
				return x + y
			case 2*ix == 2*one:
				return 1.0
			case (2*ix < 2*one) == (iy>>63 == 0):
				return 0.0 // |x|<1 && y==inf or |x|>1 && y==-inf
			}
			return y * y
		}
		if zeroInfNaN(ix) {
			x2 := x * x
			if ix>>63 != 0 && checkInt(iy) == 1 {
				x2 = -x2
			}
			if iy>>63 != 0 {
				return 1 / x2
			}
			return x2
		}
		// Here x and y are non-zero finite.
		if ix>>63 != 0 {
			// Finite x < 0.
			switch checkInt(iy) {
			case 0:
				return math.NaN()
			case 1:
				sign = signBias
			}
			ix &= 0x7fffffffffffffff
			topx &= 0x7ff
		}
		if (topy&0x7ff)-0x3be >= 0x43e-0x3be {
			// sign is 0 here because y is not odd.
			if ix == one {
				return 1.0
			}
			if topy&0x7ff < 0x3be {
				// |y| < 2^-65, x^y ~= 1 + y*log(x).
				if ix > one {
					return 1.0 + y
				}
				return 1.0 - y
			}
			if (ix > one) == (topy < 0x800) {
				return powOverflow(0)
			}
			return powUnderflow(0)
		}
		if topx == 0 {
			// Normalize subnormal x so the exponent becomes negative.
			ix = math.Float64bits(x * 0x1p52)
			ix &= 0x7fffffffffffffff
			ix -= 52 << 52
		}
	}
	hi, lo := powLog(ix)
	ehi := float64(y * hi)
	elo := math.FMA(y, lo, math.FMA(y, hi, -ehi))
	return powExp(ehi, elo, sign)
}
