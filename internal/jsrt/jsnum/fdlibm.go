package jsnum

import "math"

// The Math functions V8 implements in src/base/ieee754.cc, its port of fdlibm
// (Sun's freely distributable libm, as FreeBSD's msun carries it on). Each
// function follows the C code operation for operation, rounding as Node's
// build of it does: GCC contracts a product into a fused multiply-add when
// every use of the product is an addition or subtraction in the same basic
// block (when both operands of an addition are products, the one computed
// first is fused). Those places use math.FMA; every other product that feeds
// an addition is rounded on its own (float64(a*b)) so that the Go compiler
// does not fuse it. A product the C code computes in two branches is
// computed once before them, as GCC hoists it, and is then not fused. The
// C code's "huge+x > one" tests only raise the inexact flag; they always hold
// where they are reached and are left out.

// highWord returns the sign, exponent and top 20 mantissa bits of x.
func highWord(x float64) int32 { return int32(math.Float64bits(x) >> 32) }

// lowWord returns the low 32 mantissa bits of x.
func lowWord(x float64) uint32 { return uint32(math.Float64bits(x)) }

// fromWords builds a double from its high and low words.
func fromWords(hi int32, lo uint32) float64 {
	return math.Float64frombits(uint64(uint32(hi))<<32 | uint64(lo))
}

// withLowWord returns x with its low word replaced.
func withLowWord(x float64, lo uint32) float64 { return fromWords(highWord(x), lo) }

// withHighWord returns x with its high word replaced.
func withHighWord(x float64, hi int32) float64 { return fromWords(hi, lowWord(x)) }

// poly evaluates c[0] + x*(c[1] + x*(... + x*c[n])) by Horner's rule with
// fused multiply-adds, as fdlibm's nested polynomial expressions compile.
func poly(x float64, c ...float64) float64 {
	acc := c[len(c)-1]
	for i := len(c) - 2; i >= 0; i-- {
		acc = math.FMA(x, acc, c[i])
	}
	return acc
}

// tiny is fdlibm's volatile 1e-300, added at run time so that results such as
// pi + tiny round as the C code's do.
var tiny = 1.0e-300

// Coefficients shared by acos and asin: the rational approximation of
// (asin(x)-x)/x^3 on [0, 0.5].
const (
	pio2Hi = 1.57079632679489655800e+00
	pio2Lo = 6.12323399573676603587e-17
	pS0    = 1.66666666666666657415e-01
	pS1    = -3.25565818622400915405e-01
	pS2    = 2.01212532134862925881e-01
	pS3    = -4.00555345006794114027e-02
	pS4    = 7.91534994289814532176e-04
	pS5    = 3.47933107596021167570e-05
	qS1    = -2.40339491173441421878e+00
	qS2    = 2.02094576023350569471e+00
	qS3    = -6.88283971605453293030e-01
	qS4    = 7.70381505559019352791e-02
)

// asinRational returns the numerator and denominator of the asin
// approximation at z.
func asinRational(z float64) (p, q float64) {
	p = float64(z * poly(z, pS0, pS1, pS2, pS3, pS4, pS5))
	q = poly(z, 1, qS1, qS2, qS3, qS4)
	return p, q
}

// Acos is Math.acos.
func Acos(x float64) float64 {
	const pi = 3.14159265358979311600e+00
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix >= 0x3ff00000 {
		if ix == 0x3ff00000 && lowWord(x) == 0 {
			if hx > 0 {
				return 0
			}
			return pi + 2.0*pio2Lo
		}
		return math.NaN()
	}
	switch {
	case ix < 0x3fe00000:
		if ix <= 0x3c600000 {
			return pio2Hi + pio2Lo
		}
		p, q := asinRational(float64(x * x))
		r := p / q
		return pio2Hi - (x - math.FMA(-x, r, pio2Lo))
	case hx < 0:
		z := float64((1 + x) * 0.5)
		p, q := asinRational(z)
		s := math.Sqrt(z)
		r := p / q
		w := math.FMA(r, s, -pio2Lo)
		return math.FMA(-2.0, s+w, pi)
	default:
		z := float64((1 - x) * 0.5)
		s := math.Sqrt(z)
		df := withLowWord(s, 0)
		c := math.FMA(-df, df, z) / (s + df)
		p, q := asinRational(z)
		r := p / q
		w := math.FMA(r, s, c)
		return 2.0 * (df + w)
	}
}

// Asin is Math.asin.
func Asin(x float64) float64 {
	const pio4Hi = 7.85398163397448278999e-01
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix >= 0x3ff00000 {
		if ix == 0x3ff00000 && lowWord(x) == 0 {
			return math.FMA(x, pio2Hi, float64(x*pio2Lo))
		}
		return math.NaN()
	}
	if ix < 0x3fe00000 {
		if ix < 0x3e400000 {
			return x
		}
		p, q := asinRational(float64(x * x))
		w := p / q
		return math.FMA(x, w, x)
	}
	w := 1 - math.Abs(x)
	t := float64(w * 0.5)
	p, q := asinRational(t)
	s := math.Sqrt(t)
	if ix >= 0x3fef3333 {
		w = p / q
		t = pio2Hi - math.FMA(2.0, math.FMA(s, w, s), -pio2Lo)
	} else {
		w = withLowWord(s, 0)
		c := math.FMA(-w, w, t) / (s + w)
		r := p / q
		p = math.FMA(2.0*s, r, -math.FMA(-2.0, c, pio2Lo))
		q = math.FMA(-2.0, w, pio4Hi)
		t = pio4Hi - (p - q)
	}
	if hx > 0 {
		return t
	}
	return -t
}

// atanHi and atanLo are atan(0.5), atan(1), atan(1.5) and atan(inf) split
// into high and low parts.
var atanHi = [...]float64{
	4.63647609000806093515e-01, // atan(0.5)hi
	7.85398163397448278999e-01, // atan(1.0)hi
	9.82793723247329054082e-01, // atan(1.5)hi
	1.57079632679489655800e+00, // atan(inf)hi
}

var atanLo = [...]float64{
	2.26987774529616870924e-17, // atan(0.5)lo
	3.06161699786838301793e-17, // atan(1.0)lo
	1.39033110312309984516e-17, // atan(1.5)lo
	6.12323399573676603587e-17, // atan(inf)lo
}

// aT holds the coefficients of the approximation of atan on [0, 7/16].
var aT = [...]float64{
	3.33333333333329318027e-01,
	-1.99999999998764832476e-01,
	1.42857142725034663711e-01,
	-1.11111104054623557880e-01,
	9.09088713343650656196e-02,
	-7.69187620504482999495e-02,
	6.66107313738753120669e-02,
	-5.83357013379057348645e-02,
	4.97687799461593236017e-02,
	-3.65315727442169155270e-02,
	1.62858201153657823623e-02,
}

// Atan is Math.atan.
func Atan(x float64) float64 {
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix >= 0x44100000 { // |x| >= 2^66
		if math.IsNaN(x) {
			return x + x
		}
		if hx > 0 {
			return atanHi[3] + atanLo[3]
		}
		return -atanHi[3] - atanLo[3]
	}
	id := -1
	if ix < 0x3fdc0000 { // |x| < 0.4375
		if ix < 0x3e400000 { // |x| < 2^-27
			return x
		}
	} else {
		x = math.Abs(x)
		switch {
		case ix < 0x3fe60000: // 7/16 <= |x| < 11/16
			id = 0
			x = math.FMA(2.0, x, -1) / (2.0 + x)
		case ix < 0x3ff30000: // 11/16 <= |x| < 19/16
			id = 1
			x = (x - 1) / (x + 1)
		case ix < 0x40038000: // |x| < 2.4375
			id = 2
			x = (x - 1.5) / math.FMA(1.5, x, 1)
		default: // 2.4375 <= |x| < 2^66
			id = 3
			x = -1.0 / x
		}
	}
	z := float64(x * x)
	w := float64(z * z)
	// The odd and even terms are evaluated separately to break up the
	// dependency chain.
	s2 := float64(w * poly(w, aT[1], aT[3], aT[5], aT[7], aT[9]))
	s1s2 := math.FMA(z, poly(w, aT[0], aT[2], aT[4], aT[6], aT[8], aT[10]), s2)
	xs := float64(x * s1s2) // computed by both branches below
	if id < 0 {
		return x - xs
	}
	z = atanHi[id] - ((xs - atanLo[id]) - x)
	if hx < 0 {
		return -z
	}
	return z
}

// Atan2 is Math.atan2(y, x).
func Atan2(y, x float64) float64 {
	const (
		piO4 = 7.8539816339744827900e-01
		piO2 = 1.5707963267948965580e+00
		pi   = 3.1415926535897931160e+00
		piLo = 1.2246467991473531772e-16
	)
	if math.IsNaN(x) || math.IsNaN(y) {
		return x + y
	}
	hx, lx := highWord(x), lowWord(x)
	ix := hx & 0x7fffffff
	hy, ly := highWord(y), lowWord(y)
	iy := hy & 0x7fffffff
	if hx == 0x3ff00000 && lx == 0 { // x = 1.0
		return Atan(y)
	}
	m := (hy>>31)&1 | (hx>>30)&2 // 2*sign(x) + sign(y)
	if iy == 0 && ly == 0 {
		switch m {
		case 0, 1:
			return y // atan(+-0, +anything) = +-0
		case 2:
			return pi + tiny // atan(+0, -anything) = pi
		default:
			return -pi - tiny // atan(-0, -anything) = -pi
		}
	}
	if ix == 0 && lx == 0 {
		if hy < 0 {
			return -piO2 - tiny
		}
		return piO2 + tiny
	}
	if ix == 0x7ff00000 {
		if iy == 0x7ff00000 {
			switch m {
			case 0:
				return piO4 + tiny
			case 1:
				return -piO4 - tiny
			case 2:
				return float64(3.0*piO4) + tiny
			default:
				return float64(-3.0*piO4) - tiny
			}
		}
		switch m {
		case 0:
			return 0
		case 1:
			return math.Copysign(0, -1)
		case 2:
			return pi + tiny
		default:
			return -pi - tiny
		}
	}
	if iy == 0x7ff00000 {
		if hy < 0 {
			return -piO2 - tiny
		}
		return piO2 + tiny
	}
	var z float64
	k := (iy - ix) >> 20
	switch {
	case k > 60: // |y/x| > 2^60
		z = piO2 + float64(0.5*piLo)
		m &= 1
	case hx < 0 && k < -60: // 0 > |y|/x > -2^-60
		z = 0
	default:
		z = Atan(math.Abs(y / x))
	}
	switch m {
	case 0:
		return z
	case 1:
		return -z
	case 2:
		return pi - (z - piLo)
	default:
		return (z - piLo) - pi
	}
}

// remPio2 is __ieee754_rem_pio2: it returns n and y[0]+y[1] = x - n*pi/2 with
// |y[0]+y[1]| <= pi/4.
func remPio2(x float64) (int32, [2]float64) {
	const (
		invpio2 = 6.36619772367581382433e-01 // 53 bits of 2/pi
		pio21   = 1.57079632673412561417e+00 // first 33 bits of pi/2
		pio21t  = 6.07710050650619224932e-11 // pi/2 - pio21
		pio22   = 6.07710050630396597660e-11 // second 33 bits of pi/2
		pio22t  = 2.02226624879595063154e-21 // pi/2 - (pio21+pio22)
		pio23   = 2.02226624871116645580e-21 // third 33 bits of pi/2
		pio23t  = 8.47842766036889956997e-32 // pi/2 - (pio21+pio22+pio23)
	)
	var y [2]float64
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix <= 0x3fe921fb { // |x| <= pi/4: no reduction needed
		y[0] = x
		return 0, y
	}
	if ix < 0x4002d97c { // |x| < 3pi/4: special case with n = +-1
		if hx > 0 {
			z := x - pio21
			if ix != 0x3ff921fb { // 33+53 bits of pi are enough
				y[0] = z - pio21t
				y[1] = (z - y[0]) - pio21t
			} else { // near pi/2: use 33+33+53 bits
				z -= pio22
				y[0] = z - pio22t
				y[1] = (z - y[0]) - pio22t
			}
			return 1, y
		}
		z := x + pio21
		if ix != 0x3ff921fb {
			y[0] = z + pio21t
			y[1] = (z - y[0]) + pio21t
		} else {
			z += pio22
			y[0] = z + pio22t
			y[1] = (z - y[0]) + pio22t
		}
		return -1, y
	}
	if ix <= 0x413921fb { // |x| <= 2^19 * pi/2: medium size
		t := math.Abs(x)
		n := int32(math.FMA(t, invpio2, 0.5))
		fn := float64(n)
		r := math.FMA(-fn, pio21, t)
		w := float64(fn * pio21t) // first round, good to 85 bits; used after the branches
		if n < 32 && ix != npio2HighWords[n-1] {
			y[0] = r - w // quick check for cancellation
		} else {
			j := ix >> 20
			y[0] = r - w
			i := j - (highWord(y[0])>>20)&0x7ff
			if i > 16 { // second iteration, good to 118 bits
				t = r
				r = math.FMA(-fn, pio22, t)
				w = math.FMA(fn, pio22t, -math.FMA(-fn, pio22, t-r))
				y[0] = r - w
				i = j - (highWord(y[0])>>20)&0x7ff
				if i > 49 { // third iteration, 151 bits
					t = r
					r = math.FMA(-fn, pio23, t)
					w = math.FMA(fn, pio23t, -math.FMA(-fn, pio23, t-r))
					y[0] = r - w
				}
			}
		}
		y[1] = (r - y[0]) - w
		if hx < 0 {
			y[0], y[1] = -y[0], -y[1]
			return -n, y
		}
		return n, y
	}
	if ix >= 0x7ff00000 { // Infinity or NaN
		y[0] = x - x
		y[1] = y[0]
		return 0, y
	}
	// Large |x|: split it into three 24-bit pieces for __kernel_rem_pio2.
	e0 := ix>>20 - 1046 // ilogb(z) - 23
	z := fromWords(ix-e0<<20, lowWord(x))
	var tx [3]float64
	for i := 0; i < 2; i++ {
		tx[i] = float64(int32(z))
		z = float64((z - tx[i]) * 16777216)
	}
	tx[2] = z
	nx := 3
	for tx[nx-1] == 0 { // skip zero terms
		nx--
	}
	n, yy := kernelRemPio2(tx[:nx], int(e0))
	if hx < 0 {
		return -n, [2]float64{-yy[0], -yy[1]}
	}
	return n, yy
}

// npio2HighWords holds the high words of n*pi/2 for n = 1..32.
var npio2HighWords = [32]int32{
	0x3ff921fb, 0x400921fb, 0x4012d97c, 0x401921fb, 0x401f6a7a, 0x4022d97c,
	0x4025fdbb, 0x402921fb, 0x402c463a, 0x402f6a7a, 0x4031475c, 0x4032d97c,
	0x40346b9c, 0x4035fdbb, 0x40378fdb, 0x403921fb, 0x403ab41b, 0x403c463a,
	0x403dd85a, 0x403f6a7a, 0x40407e4c, 0x4041475c, 0x4042106c, 0x4042d97c,
	0x4043a28c, 0x40446b9c, 0x404534ac, 0x4045fdbb, 0x4046c6cb, 0x40478fdb,
	0x404858eb, 0x404921fb,
}

// twoOverPi holds 2/pi in 24-bit chunks.
var twoOverPi = [...]int32{
	0xa2f983, 0x6e4e44, 0x1529fc, 0x2757d1, 0xf534dd, 0xc0db62, 0x95993c,
	0x439041, 0xfe5163, 0xabdebb, 0xc561b7, 0x246e3a, 0x424dd2, 0xe00649,
	0x2eea09, 0xd1921c, 0xfe1deb, 0x1cb129, 0xa73ee8, 0x8235f5, 0x2ebb44,
	0x84e99c, 0x7026b4, 0x5f7e41, 0x3991d6, 0x398353, 0x39f49c, 0x845f8b,
	0xbdf928, 0x3b1ff8, 0x97ffde, 0x05980f, 0xef2f11, 0x8b5a0a, 0x6d1f6d,
	0x367ecf, 0x27cb09, 0xb74f46, 0x3f669e, 0x5fea2d, 0x7527ba, 0xc7ebe5,
	0xf17b3d, 0x0739f7, 0x8a5292, 0xea6bfb, 0x5fb11f, 0x8d5d08, 0x560330,
	0x46fc7b, 0x6babf0, 0xcfbc20, 0x9af436, 0x1da9e3, 0x91615e, 0xe61b08,
	0x659985, 0x5f14a0, 0x68408d, 0xffd880, 0x4d7327, 0x310606, 0x1556ca,
	0x73a8c9, 0x60e27b, 0xc08c6b,
}

// pio2Chunks is pi/2 in 24-bit pieces.
var pio2Chunks = [...]float64{
	1.57079625129699707031e+00,
	7.54978941586159635335e-08,
	5.39030252995776476554e-15,
	3.28200341580791294123e-22,
	1.27065575308067607349e-29,
	1.22933308981111328932e-36,
	2.73370053816464559624e-44,
	2.16741683877804819444e-51,
}

// kernelRemPio2 is __kernel_rem_pio2 with prec = 2 (53-bit results), as
// __ieee754_rem_pio2 calls it: x holds the 24-bit pieces of the input scaled
// by 2^-e0; it returns n mod 8 and the reduced argument in two doubles.
func kernelRemPio2(x []float64, e0 int) (int32, [2]float64) {
	const (
		two24  = 1.67772160000000000000e+07
		twon24 = 5.96046447753906250000e-08
		jk     = 4 // init_jk[2]: terms of 2/pi needed for 53-bit precision
	)
	var (
		iq       [20]int32
		f, fq, q [20]float64
	)
	jp := jk
	jx := len(x) - 1
	jv := (e0 - 3) / 24
	if jv < 0 {
		jv = 0
	}
	q0 := e0 - 24*(jv+1)
	// Set up f[0] to f[jx+jk] where f[jx+jk] = twoOverPi[jv+jk].
	j := jv - jx
	m := jx + jk
	for i := 0; i <= m; i, j = i+1, j+1 {
		if j >= 0 {
			f[i] = float64(twoOverPi[j])
		}
	}
	// Compute q[0..jk].
	for i := 0; i <= jk; i++ {
		fw := 0.0
		for j := 0; j <= jx; j++ {
			fw = math.FMA(x[j], f[jx+i-j], fw)
		}
		q[i] = fw
	}
	jz := jk
	var (
		z  float64
		n  int32
		ih int32
	)
	for {
		// Distill q[] into iq[] reversingly.
		i := 0
		z = q[jz]
		for j := jz; j > 0; i, j = i+1, j-1 {
			fw := float64(int32(float64(twon24 * z)))
			iq[i] = int32(math.FMA(-two24, fw, z))
			z = q[j-1] + fw
		}
		z = math.Ldexp(z, q0)
		z = math.FMA(-8.0, math.Floor(z*0.125), z) // trim off integer >= 8
		n = int32(z)
		z -= float64(n)
		ih = 0
		switch {
		case q0 > 0: // need iq[jz-1] to determine n
			i := iq[jz-1] >> (24 - q0)
			n += i
			iq[jz-1] -= i << (24 - q0)
			ih = iq[jz-1] >> (23 - q0)
		case q0 == 0:
			ih = iq[jz-1] >> 23
		case z >= 0.5:
			ih = 2
		}
		if ih > 0 { // q > 0.5
			n++
			carry := int32(0)
			for i := 0; i < jz; i++ { // compute 1-q
				j := iq[i]
				if carry == 0 {
					if j != 0 {
						carry = 1
						iq[i] = 0x1000000 - j
					}
				} else {
					iq[i] = 0xffffff - j
				}
			}
			switch q0 { // rare case: chance is 1 in 12
			case 1:
				iq[jz-1] &= 0x7fffff
			case 2:
				iq[jz-1] &= 0x3fffff
			}
			if ih == 2 {
				z = 1 - z
				if carry != 0 {
					z -= math.Ldexp(1, q0)
				}
			}
		}
		// Check whether recomputation is needed.
		if z != 0 {
			break
		}
		j := int32(0)
		for i := jz - 1; i >= jk; i-- {
			j |= iq[i]
		}
		if j != 0 {
			break
		}
		// Need more terms of 2/pi: k = number of terms needed.
		k := 1
		for k <= jk && iq[jk-k] == 0 {
			k++
		}
		for i := jz + 1; i <= jz+k; i++ { // add q[jz+1] to q[jz+k]
			f[jx+i] = float64(twoOverPi[jv+i])
			fw := 0.0
			for j := 0; j <= jx; j++ {
				fw = math.FMA(x[j], f[jx+i-j], fw)
			}
			q[i] = fw
		}
		jz += k
	}
	// Chop off zero terms.
	if z == 0 {
		jz--
		q0 -= 24
		for iq[jz] == 0 {
			jz--
			q0 -= 24
		}
	} else { // break z into 24-bit pieces if necessary
		z = math.Ldexp(z, -q0)
		if z >= two24 {
			fw := float64(int32(float64(twon24 * z)))
			iq[jz] = int32(math.FMA(-two24, fw, z))
			jz++
			q0 += 24
			iq[jz] = int32(fw)
		} else {
			iq[jz] = int32(z)
		}
	}
	// Convert the integer bit chunks to floating point.
	fw := math.Ldexp(1, q0)
	for i := jz; i >= 0; i-- {
		q[i] = float64(fw * float64(iq[i]))
		fw *= twon24
	}
	// Compute pio2[0..jp] * q[jz..0].
	for i := jz; i >= 0; i-- {
		fw := 0.0
		for k := 0; k <= jp && k <= jz-i; k++ {
			fw = math.FMA(pio2Chunks[k], q[i+k], fw)
		}
		fq[jz-i] = fw
	}
	// Compress fq[] into y[].
	var y [2]float64
	fw = 0.0
	for i := jz; i >= 0; i-- {
		fw += fq[i]
	}
	if ih == 0 {
		y[0] = fw
	} else {
		y[0] = -fw
	}
	fw = fq[0] - fw
	for i := 1; i <= jz; i++ {
		fw += fq[i]
	}
	if ih == 0 {
		y[1] = fw
	} else {
		y[1] = -fw
	}
	return n & 7, y
}

// kernelSin is __kernel_sin: sin(x+y) for |x| <= pi/4; iy = 0 means y is 0.
func kernelSin(x, y float64, iy int) float64 {
	const (
		s1 = -1.66666666666666324348e-01
		s2 = 8.33333333332248946124e-03
		s3 = -1.98412698298579493134e-04
		s4 = 2.75573137070700676789e-06
		s5 = -2.50507602534068634195e-08
		s6 = 1.58969099521155010221e-10
	)
	ix := highWord(x) & 0x7fffffff
	if ix < 0x3e400000 && int32(x) == 0 { // |x| < 2^-27
		return x
	}
	z := float64(x * x)
	v := float64(z * x)
	r := poly(z, s2, s3, s4, s5, s6)
	if iy == 0 {
		return math.FMA(v, math.FMA(z, r, s1), x)
	}
	return x - math.FMA(-v, s1, math.FMA(z, math.FMA(0.5, y, -float64(v*r)), -y))
}

// kernelCos is __kernel_cos: cos(x+y) for |x| <= pi/4.
func kernelCos(x, y float64) float64 {
	const (
		c1 = 4.16666666666666019037e-02
		c2 = -1.38888888888741095749e-03
		c3 = 2.48015872894767294178e-05
		c4 = -2.75573143513906633035e-07
		c5 = 2.08757232129817482790e-09
		c6 = -1.13596475577881948265e-11
	)
	ix := highWord(x) & 0x7fffffff
	if ix < 0x3e400000 && int32(x) == 0 { // |x| < 2^-27
		return 1
	}
	z := float64(x * x)
	r := float64(z * poly(z, c1, c2, c3, c4, c5, c6))
	zr := math.FMA(z, r, -float64(x*y))
	if ix < 0x3fd33333 { // |x| < 0.3
		return 1 - math.FMA(0.5, z, -zr)
	}
	qx := 0.28125
	if ix <= 0x3fe90000 { // x <= 0.78125
		qx = fromWords(ix-0x00200000, 0) // x/4
	}
	iz := math.FMA(0.5, z, -qx)
	a := 1 - qx
	return a - (iz - zr)
}

// Sin is Math.sin.
func Sin(x float64) float64 {
	ix := highWord(x) & 0x7fffffff
	switch {
	case ix <= 0x3fe921fb:
		return kernelSin(x, 0, 0)
	case ix >= 0x7ff00000:
		return x - x
	}
	n, y := remPio2(x)
	switch n & 3 {
	case 0:
		return kernelSin(y[0], y[1], 1)
	case 1:
		return kernelCos(y[0], y[1])
	case 2:
		return -kernelSin(y[0], y[1], 1)
	}
	return -kernelCos(y[0], y[1])
}

// Cos is Math.cos.
func Cos(x float64) float64 {
	ix := highWord(x) & 0x7fffffff
	switch {
	case ix <= 0x3fe921fb:
		return kernelCos(x, 0)
	case ix >= 0x7ff00000:
		return x - x
	}
	n, y := remPio2(x)
	switch n & 3 {
	case 0:
		return kernelCos(y[0], y[1])
	case 1:
		return -kernelSin(y[0], y[1], 1)
	case 2:
		return -kernelCos(y[0], y[1])
	}
	return kernelSin(y[0], y[1], 1)
}

// tanT holds the coefficients of the odd polynomial of tan on [0, 0.67434].
var tanT = [...]float64{
	3.33333333333334091986e-01,
	1.33333333333201242699e-01,
	5.39682539762260521377e-02,
	2.18694882948595424599e-02,
	8.86323982359930005737e-03,
	3.59207910759131235356e-03,
	1.45620945432529025516e-03,
	5.88041240820264096874e-04,
	2.46463134818469906812e-04,
	7.81794442939557092300e-05,
	7.14072491382608190305e-05,
	-1.85586374855275456654e-05,
	2.59073051863633712884e-05,
}

// kernelTan is __kernel_tan: tan(x+y) for |x| <= pi/4 when k = 1, and
// -1/tan(x+y) when k = -1.
func kernelTan(x, y float64, k int32) float64 {
	const (
		pio4   = 7.85398163397448278999e-01
		pio4lo = 3.06161699786838301793e-17
	)
	// invert computes -1/(x+r) accurately when x+r = w was rounded.
	invert := func(w, x, r float64) float64 {
		z := withLowWord(w, 0)
		v := r - (z - x) // z+v = r+x
		a := -1.0 / w    // a = -1.0/w
		t := withLowWord(a, 0)
		s := math.FMA(t, z, 1.0)
		return math.FMA(a, math.FMA(t, v, s), t)
	}
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix < 0x3e300000 && int32(x) == 0 { // |x| < 2^-28
		switch {
		case ix|int32(lowWord(x))|(k+1) == 0:
			return 1 / math.Abs(x)
		case k == 1:
			return x
		default: // compute -1/(x+y) carefully
			return invert(x+y, x, y)
		}
	}
	if ix >= 0x3fe59428 { // |x| >= 0.6744
		if hx < 0 {
			x, y = -x, -y
		}
		z := pio4 - x
		w := pio4lo - y
		x = z + w
		y = 0.0
	}
	z := float64(x * x)
	w := float64(z * z)
	// Break x^5*(tanT[1]+x^2*tanT[2]+...) into
	// x^5(tanT[1]+x^4*tanT[3]+...+x^20*tanT[11]) +
	// x^5(x^2*(tanT[2]+x^4*tanT[4]+...+x^22*tanT[12]))
	r := poly(w, tanT[1], tanT[3], tanT[5], tanT[7], tanT[9], tanT[11])
	// v = x^2*(T[2]+...) is fused into r+v.
	rv := math.FMA(z, poly(w, tanT[2], tanT[4], tanT[6], tanT[8], tanT[10], tanT[12]), r)
	s := float64(z * x)
	r = math.FMA(z, math.FMA(s, rv, y), y)
	r = math.FMA(tanT[0], s, r)
	w = x + r
	if ix >= 0x3fe59428 {
		v := float64(k)
		return float64(1-(hx>>30)&2) * math.FMA(-2.0, x-(float64(w*w)/(w+v)-r), v)
	}
	if k == 1 {
		return w
	}
	return invert(w, x, r)
}

// Tan is Math.tan.
func Tan(x float64) float64 {
	ix := highWord(x) & 0x7fffffff
	switch {
	case ix <= 0x3fe921fb: // |x| ~< pi/4
		return kernelTan(x, 0, 1)
	case ix >= 0x7ff00000: // Infinity or NaN
		return x - x
	}
	n, y := remPio2(x)
	return kernelTan(y[0], y[1], 1-(n&1)<<1)
}

// Shared constants of exp, expm1, log and log1p.
const (
	ln2Hi  = 6.93147180369123816490e-01
	ln2Lo  = 1.90821492927058770002e-10
	invln2 = 1.44269504088896338700e+00
	two54  = 1.80143985094819840000e+16
	// The polynomial of log((1+s)/(1-s)) on [0, 0.1716].
	lg1 = 6.666666666666735130e-01
	lg2 = 3.999999999940941908e-01
	lg3 = 2.857142874366239149e-01
	lg4 = 2.222219843214978396e-01
	lg5 = 1.818357216161805012e-01
	lg6 = 1.531383769920937332e-01
	lg7 = 1.479819860511658591e-01
)

// twoToThe returns 2^k for -1022 <= k <= 1023, built from its exponent bits.
func twoToThe(k int32) float64 {
	return fromWords(0x3ff00000+k<<20, 0)
}

// Exp is Math.exp.
func Exp(x float64) float64 {
	const (
		oThreshold = 7.09782712893383973096e+02
		uThreshold = -7.45133219101941108420e+02
		p1         = 1.66666666666666019037e-01
		p2         = -2.77777777770155933842e-03
		p3         = 6.61375632143793436117e-05
		p4         = -1.65339022054652515390e-06
		p5         = 4.13813679705723846039e-08
		e          = 2.718281828459045
		twom1000   = 9.33263618503218878990e-302 // 2^-1000
		two1023    = 8.988465674311579539e307    // 2^1023
	)
	halF := [2]float64{0.5, -0.5}
	ln2HI := [2]float64{ln2Hi, -ln2Hi}
	ln2LO := [2]float64{ln2Lo, -ln2Lo}
	hx := uint32(highWord(x))
	xsb := int32(hx >> 31) // sign bit of x
	hx &= 0x7fffffff       // high word of |x|
	// Filter out non-finite arguments.
	if hx >= 0x40862e42 { // |x| >= 709.78...
		if hx >= 0x7ff00000 {
			if (hx&0xfffff)|lowWord(x) != 0 {
				return x + x // NaN
			}
			if xsb == 0 {
				return x // exp(+inf) = inf
			}
			return 0 // exp(-inf) = 0
		}
		if x > oThreshold {
			return math.Inf(1) // overflow
		}
		if x < uThreshold {
			return 0 // underflow
		}
	}
	// Argument reduction.
	var hi, lo float64
	var k int32
	switch {
	case hx > 0x3fd62e42: // |x| > 0.5 ln2
		if hx < 0x3ff0a2b2 { // and |x| < 1.5 ln2
			if x == 1.0 {
				return e
			}
			hi = x - ln2HI[xsb]
			lo = ln2LO[xsb]
			k = 1 - xsb - xsb
		} else {
			k = int32(math.FMA(invln2, x, halF[xsb]))
			t := float64(k)
			hi = math.FMA(-t, ln2HI[0], x) // t*ln2HI is exact here
			lo = float64(t * ln2LO[0])     // used after the branches
		}
		x = hi - lo
	case hx < 0x3e300000: // |x| < 2^-28
		return 1 + x
	}
	// x is now in the primary range.
	t := float64(x * x)
	var twopk float64
	if k >= -1021 {
		twopk = twoToThe(k)
	} else {
		twopk = twoToThe(k + 1000)
	}
	c := math.FMA(-t, poly(t, p1, p2, p3, p4, p5), x)
	if k == 0 {
		return 1 - (float64(x*c)/(c-2.0) - x)
	}
	y := 1 - ((lo - float64(x*c)/(2.0-c)) - hi)
	if k >= -1021 {
		if k == 1024 {
			return float64(y*2.0) * two1023
		}
		return y * twopk
	}
	return float64(y*twopk) * twom1000
}

// Expm1 is Math.expm1.
func Expm1(x float64) float64 {
	const (
		oThreshold = 7.09782712893383973096e+02
		// The rational approximation of x/(e^x-1) on [0, 0.5 ln2].
		q1 = -3.33333333333331316428e-02
		q2 = 1.58730158725481460165e-03
		q3 = -7.93650757867487942473e-05
		q4 = 4.00821782732936239552e-06
		q5 = -2.01099218183624371326e-07
	)
	hx := uint32(highWord(x))
	xsb := hx & 0x80000000 // sign bit of x
	hx &= 0x7fffffff       // high word of |x|
	// Filter out huge and non-finite arguments.
	if hx >= 0x4043687a { // |x| >= 56 ln2
		if hx >= 0x40862e42 { // |x| >= 709.78...
			if hx >= 0x7ff00000 {
				if (hx&0xfffff)|lowWord(x) != 0 {
					return x + x // NaN
				}
				if xsb == 0 {
					return x // expm1(+inf) = inf
				}
				return -1 // expm1(-inf) = -1
			}
			if x > oThreshold {
				return math.Inf(1) // overflow
			}
		}
		if xsb != 0 && x+tiny < 0.0 { // x < -56 ln2
			return tiny - 1 // -1 with inexact
		}
	}
	// Argument reduction.
	var hi, lo, c float64
	var k int32
	switch {
	case hx > 0x3fd62e42: // |x| > 0.5 ln2
		if hx < 0x3ff0a2b2 { // and |x| < 1.5 ln2
			if xsb == 0 {
				hi, lo, k = x-ln2Hi, ln2Lo, 1
			} else {
				hi, lo, k = x+ln2Hi, -ln2Lo, -1
			}
		} else {
			half := 0.5
			if xsb != 0 {
				half = -0.5
			}
			k = int32(math.FMA(invln2, x, half))
			t := float64(k)
			hi = math.FMA(-t, ln2Hi, x) // t*ln2Hi is exact here
			lo = float64(t * ln2Lo)     // used after the branches
		}
		x = hi - lo
		c = (hi - x) - lo
	case hx < 0x3c900000: // |x| < 2^-54: return x with inexact
		return x
	}
	// x is now in the primary range.
	hfx := float64(0.5 * x)
	hxs := float64(x * hfx)
	r1 := poly(hxs, 1, q1, q2, q3, q4, q5)
	t := math.FMA(-r1, hfx, 3.0)
	e := float64(hxs * ((r1 - t) / math.FMA(-x, t, 6.0)))
	if k == 0 {
		return x - math.FMA(x, e, -hxs) // c is 0
	}
	twopk := twoToThe(k)
	e = math.FMA(x, e-c, -c)
	e -= hxs
	switch {
	case k == -1:
		return math.FMA(0.5, x-e, -0.5)
	case k == 1:
		if x < -0.25 {
			return -2.0 * (e - (x + 0.5))
		}
		return math.FMA(2.0, x-e, 1)
	case k <= -2 || k > 56: // suffice to return exp(x)-1
		y := 1 - (e - x)
		if k == 1024 {
			y = float64(y*2.0) * 8.98846567431158e+307
		} else {
			y = y * twopk
		}
		return y - 1
	}
	var y float64
	if k < 20 {
		t := fromWords(0x3ff00000-(0x200000>>k), 0) // t = 1-2^-k
		y = t - (e - x)
		y = y * twopk
	} else {
		t := fromWords((0x3ff-k)<<20, 0) // 2^-k
		y = x - (e + t)
		y += 1
		y = y * twopk
	}
	return y
}

// Log is Math.log.
func Log(x float64) float64 {
	hx, lx := highWord(x), lowWord(x)
	k := int32(0)
	if hx < 0x00100000 { // x < 2^-1022
		if hx&0x7fffffff == 0 && lx == 0 {
			return math.Inf(-1) // log(+-0) = -inf
		}
		if hx < 0 {
			return math.NaN() // log(-#) = NaN
		}
		k -= 54
		x *= two54 // subnormal number, scale up x
		hx = highWord(x)
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	k += hx>>20 - 1023
	hx &= 0x000fffff
	i := (hx + 0x95f64) & 0x100000
	x = withHighWord(x, hx|(i^0x3ff00000)) // normalize x or x/2
	k += i >> 20
	f := x - 1.0
	dk := float64(k)
	if (0x000fffff & (2 + hx)) < 3 { // -2^-20 <= f < 2^-20
		if f == 0 {
			if k == 0 {
				return 0
			}
			return math.FMA(dk, ln2Hi, float64(dk*ln2Lo))
		}
		R := float64(f*f) * math.FMA(-0.33333333333333333, f, 0.5)
		if k == 0 {
			return f - R
		}
		return math.FMA(dk, ln2Hi, -(math.FMA(-dk, ln2Lo, R) - f))
	}
	s := f / (2.0 + f)
	z := float64(s * s)
	i = hx - 0x6147a
	w := float64(z * z)
	j := 0x6b851 - hx
	// t1 = w*(Lg2+...) is fused into t2+t1.
	t2 := float64(z * poly(w, lg1, lg3, lg5, lg7))
	i |= j
	R := math.FMA(w, poly(w, lg2, lg4, lg6), t2)
	if i > 0 {
		hfsq := float64(float64(0.5*f) * f)
		sq := float64(s * (hfsq + R)) // computed by both branches below
		if k == 0 {
			return f - (hfsq - sq)
		}
		return math.FMA(dk, ln2Hi, -((hfsq - math.FMA(dk, ln2Lo, sq)) - f))
	}
	sq := float64(s * (f - R)) // computed by both branches below
	if k == 0 {
		return f - sq
	}
	return math.FMA(dk, ln2Hi, -(math.FMA(-dk, ln2Lo, sq) - f))
}

// Log1p is Math.log1p.
func Log1p(x float64) float64 {
	hx := highWord(x)
	ax := hx & 0x7fffffff
	k := int32(1)
	var f, c float64
	var hu int32
	if hx < 0x3fda827a { // 1+x < sqrt(2)+
		if ax >= 0x3ff00000 { // x <= -1.0
			if x == -1.0 {
				return math.Inf(-1) // log1p(-1) = -inf
			}
			return math.NaN() // log1p(x<-1) = NaN
		}
		if ax < 0x3e200000 { // |x| < 2^-29
			if ax < 0x3c900000 { // |x| < 2^-54
				return x
			}
			return math.FMA(-float64(x*x), 0.5, x)
		}
		if hx > 0 || hx <= int32(-0x402d413c) { // sqrt(2)/2- <= 1+x < sqrt(2)+
			k = 0
			f = x
			hu = 1
		}
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	if k != 0 {
		var u float64
		if hx < 0x43400000 {
			u = 1.0 + x
			hu = highWord(u)
			k = hu>>20 - 1023
			if k > 0 { // correction term
				c = 1.0 - (u - x)
			} else {
				c = x - (u - 1.0)
			}
			c /= u
		} else {
			u = x
			hu = highWord(u)
			k = hu>>20 - 1023
			c = 0
		}
		hu &= 0x000fffff
		if hu < 0x6a09e { // u ~< sqrt(2)
			u = withHighWord(u, hu|0x3ff00000) // normalize u
		} else {
			k++
			u = withHighWord(u, hu|0x3fe00000) // normalize u/2
			hu = (0x00100000 - hu) >> 2
		}
		f = u - 1.0
	}
	hfsq := float64(float64(0.5*f) * f)
	dk := float64(k)
	if hu == 0 { // |f| < 2^-20
		if f == 0 {
			if k == 0 {
				return 0
			}
			c = math.FMA(dk, ln2Lo, c)
			return math.FMA(dk, ln2Hi, c)
		}
		R := hfsq * math.FMA(-0.66666666666666666, f, 1.0)
		if k == 0 {
			return f - R
		}
		return math.FMA(dk, ln2Hi, -((R - math.FMA(dk, ln2Lo, c)) - f))
	}
	s := f / (2.0 + f)
	z := float64(s * s)
	// R = z*(Lp1+...) is fused into hfsq+R, which both branches below compute.
	sq := float64(s * math.FMA(z, poly(z, lg1, lg2, lg3, lg4, lg5, lg6, lg7), hfsq))
	if k == 0 {
		return f - (hfsq - sq)
	}
	return math.FMA(dk, ln2Hi, -((hfsq - (sq + math.FMA(dk, ln2Lo, c))) - f))
}

// Log2 is Math.log2.
func Log2(x float64) float64 {
	const (
		ivln2hi = 1.44269504072144627571e+00 // high 33 bits of 1/ln2
		ivln2lo = 1.67517131648865118353e-10 // 1/ln2 - ivln2hi
	)
	hx, lx := highWord(x), lowWord(x)
	k := int32(0)
	if hx < 0x00100000 { // x < 2^-1022
		if hx&0x7fffffff == 0 && lx == 0 {
			return math.Inf(-1) // log(+-0) = -inf
		}
		if hx < 0 {
			return math.NaN() // log(-#) = NaN
		}
		k -= 54
		x *= two54 // subnormal number, scale up x
		hx = highWord(x)
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	if hx == 0x3ff00000 && lx == 0 {
		return 0 // log(1) = +0
	}
	k += hx>>20 - 1023
	hx &= 0x000fffff
	i := (hx + 0x95f64) & 0x100000
	x = withHighWord(x, hx|(i^0x3ff00000)) // normalize x or x/2
	k += i >> 20
	y := float64(k)
	f := x - 1.0
	// k_log1p(f): r = log(1+f) - f + hfsq. Its hfsq = 0.5*f*f is the one
	// computed here, fused into every use.
	hf := float64(0.5 * f)
	s := f / (2.0 + f)
	z := float64(s * s)
	w := float64(z * z)
	t2 := float64(z * poly(w, lg1, lg3, lg5, lg7))
	R := math.FMA(w, poly(w, lg2, lg4, lg6), t2)
	// f-hfsq must be split into hi and lo parts so that the multiplication by
	// ivln2hi is exact; r = s*(hfsq+R) is fused into lo.
	hi := withLowWord(math.FMA(-hf, f, f), 0)
	lo := math.FMA(s, math.FMA(hf, f, R), math.FMA(-hf, f, f-hi))
	valLo := math.FMA(lo+hi, ivln2lo, float64(lo*ivln2hi))
	// Spread the extra precision of y + hi*ivln2hi into valLo.
	w = math.FMA(hi, ivln2hi, y)
	valLo += math.FMA(hi, ivln2hi, y-w)
	return valLo + w
}

// Log10 is Math.log10.
func Log10(x float64) float64 {
	const (
		ivln10  = 4.34294481903251816668e-01
		log10Hi = 3.01029995663611771306e-01 // log10(2) high bits
		log10Lo = 3.69423907715893078616e-13 // log10(2) - log10Hi
	)
	hx, lx := highWord(x), lowWord(x)
	k := int32(0)
	if hx < 0x00100000 { // x < 2^-1022
		if hx&0x7fffffff == 0 && lx == 0 {
			return math.Inf(-1) // log(+-0) = -inf
		}
		if hx < 0 {
			return math.NaN() // log(-#) = NaN
		}
		k -= 54
		x *= two54 // subnormal number, scale up x
		hx, lx = highWord(x), lowWord(x)
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	if hx == 0x3ff00000 && lx == 0 {
		return 0 // log(1) = +0
	}
	k += hx>>20 - 1023
	i := int32(uint32(k) >> 31)
	hx = (hx & 0x000fffff) | (0x3ff-i)<<20
	y := float64(k + i)
	x = fromWords(hx, lx)
	z := math.FMA(y, log10Lo, float64(ivln10*Log(x)))
	return math.FMA(y, log10Hi, z)
}

// Acosh is Math.acosh.
func Acosh(x float64) float64 {
	const ln2 = 6.93147180559945286227e-01
	hx, lx := highWord(x), lowWord(x)
	switch {
	case hx < 0x3ff00000: // x < 1
		return math.NaN()
	case hx >= 0x41b00000: // x > 2^28
		if hx >= 0x7ff00000 { // x is inf or NaN
			return x + x
		}
		return Log(x) + ln2 // acosh(huge) = log(2x)
	case hx == 0x3ff00000 && lx == 0:
		return 0 // acosh(1) = 0
	case hx > 0x40000000: // 2^28 > x > 2
		return Log(math.FMA(2.0, x, -(1 / (x + math.Sqrt(math.FMA(x, x, -1))))))
	}
	// 1 < x < 2
	t := x - 1
	return Log1p(t + math.Sqrt(math.FMA(2.0, t, float64(t*t))))
}

// Asinh is Math.asinh.
func Asinh(x float64) float64 {
	const ln2 = 6.93147180559945286227e-01
	hx := highWord(x)
	ix := hx & 0x7fffffff
	if ix >= 0x7ff00000 {
		return x + x // x is inf or NaN
	}
	if ix < 0x3e300000 { // |x| < 2^-28
		return x
	}
	var w float64
	switch {
	case ix > 0x41b00000: // |x| > 2^28
		w = Log(math.Abs(x)) + ln2
	case ix > 0x40000000: // 2^28 > |x| > 2.0
		t := math.Abs(x)
		w = Log(math.FMA(2.0, t, 1/(math.Sqrt(math.FMA(x, x, 1))+t)))
	default: // 2.0 > |x| > 2^-28
		t := float64(x * x)
		w = Log1p(math.Abs(x) + t/(1+math.Sqrt(1+t)))
	}
	if hx > 0 {
		return w
	}
	return -w
}

// Atanh is Math.atanh.
func Atanh(x float64) float64 {
	hx, lx := highWord(x), lowWord(x)
	ix := hx & 0x7fffffff
	if ix > 0x3ff00000 || ix == 0x3ff00000 && lx != 0 { // |x| > 1 or NaN
		return math.NaN()
	}
	if ix == 0x3ff00000 {
		return math.Copysign(math.Inf(1), x)
	}
	if ix < 0x3e300000 { // |x| < 2^-28
		return x
	}
	x = withHighWord(x, ix) // x <- |x|
	var t float64
	if ix < 0x3fe00000 { // x < 0.5
		t = x + x
		t = float64(0.5 * Log1p(t+float64(t*x)/(1-x)))
	} else {
		t = float64(0.5 * Log1p((x+x)/(1-x)))
	}
	if hx >= 0 {
		return t
	}
	return -t
}

// Cosh is Math.cosh.
func Cosh(x float64) float64 {
	const coshOverflow = 710.4758600739439
	ix := highWord(x) & 0x7fffffff
	switch {
	case ix < 0x3fd62e43: // |x| in [0, 0.5*log2]: 1+expm1(|x|)^2/(2*exp(|x|))
		t := Expm1(math.Abs(x))
		w := 1 + t
		if ix < 0x3c800000 {
			return w // cosh(tiny) = 1
		}
		return 1 + float64(t*t)/(w+w)
	case ix < 0x40360000: // |x| in [0.5*log2, 22]: (exp(|x|)+1/exp(|x|))/2
		t := Exp(math.Abs(x))
		return math.FMA(0.5, t, 0.5/t)
	case ix < 0x40862e42: // |x| in [22, log(maxdouble)]: exp(|x|)/2
		return 0.5 * Exp(math.Abs(x))
	case math.Abs(x) <= coshOverflow: // |x| in [log(maxdouble), overflow threshold]
		w := Exp(float64(0.5 * math.Abs(x)))
		t := float64(0.5 * w)
		return t * w
	case ix >= 0x7ff00000: // x is inf or NaN
		return x * x
	}
	return math.Inf(1) // overflow
}

// Sinh is Math.sinh.
func Sinh(x float64) float64 {
	const (
		sinhOverflow = 710.4758600739439
		twoM28       = 3.725290298461914e-9 // 2^-28
		logMaxD      = 709.7822265625       // 0x40862E42 00000000, log(max double)
		shuge        = 1.0e307
	)
	h := 0.5
	if x < 0 {
		h = -0.5
	}
	ax := math.Abs(x)
	switch {
	case ax < 22: // |x| in [0, 22]: sign(x)*0.5*(E+E/(E+1))
		if ax < twoM28 {
			return x // sinh(tiny) = tiny with inexact
		}
		t := Expm1(ax)
		if ax < 1 {
			return h * math.FMA(2, t, -(float64(t*t)/(t+1)))
		}
		return float64(h * (t + t/(t+1)))
	case ax < logMaxD: // |x| in [22, log(maxdouble)]: 0.5*exp(|x|)
		return h * Exp(ax)
	case ax <= sinhOverflow: // |x| in [log(maxdouble), overflow threshold]
		w := Exp(float64(0.5 * ax))
		t := float64(h * w)
		return t * w
	}
	return x * shuge // overflow, or x is inf or NaN
}

// Tanh is Math.tanh.
func Tanh(x float64) float64 {
	jx := highWord(x)
	ix := jx & 0x7fffffff
	if ix >= 0x7ff00000 { // x is inf or NaN
		if jx >= 0 {
			return 1/x + 1 // tanh(+inf) = 1, tanh(NaN) = NaN
		}
		return 1/x - 1 // tanh(-inf) = -1
	}
	var z float64
	switch {
	case ix < 0x3e300000: // |x| < 2^-28
		return x
	case ix >= 0x40360000: // |x| >= 22: return +-1
		z = 1 - tiny
	case ix >= 0x3ff00000: // |x| >= 1
		t := Expm1(float64(2 * math.Abs(x)))
		z = 1 - 2/(t+2)
	default:
		t := Expm1(float64(-2 * math.Abs(x)))
		z = -t / (t + 2)
	}
	if jx >= 0 {
		return z
	}
	return -z
}

// Cbrt is Math.cbrt, FreeBSD's s_cbrt.c.
func Cbrt(x float64) float64 {
	const (
		b1 = 715094163 // (1023-1023/3-0.03306235651)*2**20
		b2 = 696219795 // (1023-1023/3-54/3-0.03306235651)*2**20
		p0 = 1.87595182427177009643
		p1 = -1.88497979543377169875
		p2 = 1.621429720105354466140
		p3 = -0.758397934778766047437
		p4 = 0.145996192886612446982
	)
	bits := math.Float64bits(x)
	hx := uint32(bits >> 32)
	low := uint32(bits)
	sign := hx & 0x80000000
	hx ^= sign
	if hx >= 0x7ff00000 {
		return x + x
	}
	var t float64
	if hx < 0x00100000 {
		if hx|low == 0 {
			return x
		}
		t = float64(math.Float64frombits(0x43500000<<32) * x)
		high := uint32(math.Float64bits(t) >> 32)
		t = math.Float64frombits(uint64(sign|((high&0x7fffffff)/3+b2)) << 32)
	} else {
		t = math.Float64frombits(uint64(sign|(hx/3+b1)) << 32)
	}
	r := float64(float64(t*t) * float64(t/x))
	t = float64(t * math.FMA(float64(float64(r*r)*r), poly(r, p3, p4), poly(r, p0, p1, p2)))
	t = math.Float64frombits((math.Float64bits(t) + 0x80000000) & 0xffffffffc0000000)
	s := float64(t * t)
	r = x / s
	w := t + t
	r = (r - t) / (w + r)
	return math.FMA(t, r, t)
}
