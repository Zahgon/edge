package jsnum

import "math"

// Math functions V8 implements in its own builtins rather than in its libm
// port (fdlibm.go) or the C library (pow.go).

// Round is Math.round: halves round towards +Infinity, and -0.5 <= x < 0 gives
// -0.
func Round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || x == math.Trunc(x) {
		return x
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	f := math.Floor(x)
	if x-f >= 0.5 {
		f++
	}
	return f
}

// Hypot is Math.hypot as V8's builtin computes it: the arguments are scaled by
// the largest magnitude and summed with Kahan compensation. An infinite
// argument wins over NaN.
func Hypot(values []float64) float64 {
	maxAbs := 0.0
	nan := false
	for _, v := range values {
		a := math.Abs(v)
		switch {
		case math.IsInf(a, 1):
			return math.Inf(1)
		case math.IsNaN(a):
			nan = true
		case a > maxAbs:
			maxAbs = a
		}
	}
	if nan {
		return math.NaN()
	}
	if maxAbs == 0 {
		return 0
	}
	sum, compensation := 0.0, 0.0
	for _, v := range values {
		n := math.Abs(v) / maxAbs
		summand := float64(n*n) - compensation
		preliminary := sum + summand
		compensation = (preliminary - sum) - summand
		sum = preliminary
	}
	return float64(math.Sqrt(sum) * maxAbs)
}
