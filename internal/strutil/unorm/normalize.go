// Package unorm implements the canonical Unicode normalization forms NFD and NFC as
// String.prototype.normalize computes them in Node (ICU), for the strings of the
// JavaScript value model. The tables come from gen/main.go.
package unorm

import (
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// Hangul syllable arithmetic (Unicode §3.12).
const (
	sBase  = 0xAC00
	lBase  = 0x1100
	vBase  = 0x1161
	tBase  = 0x11A7
	lCount = 19
	vCount = 21
	tCount = 28
	nCount = vCount * tCount
	sCount = lCount * nCount
)

// NFD is s.normalize("NFD"). s is a JavaScript string in jsstr's WTF-8 form; lone
// surrogates are left as they are, as V8 leaves them.
func NFD(s string) string {
	if jsstr.IsASCII(s) {
		return s
	}
	return fromCodePoints(decompose(codePoints(s)))
}

// NFC is s.normalize("NFC").
func NFC(s string) string {
	if jsstr.IsASCII(s) {
		return s
	}
	return fromCodePoints(compose(decompose(codePoints(s))))
}

// codePoints splits s into code points, a lone surrogate counting as one.
func codePoints(s string) []rune {
	u := jsstr.ToUTF16(s)
	out := make([]rune, 0, len(u))
	for i := 0; i < len(u); {
		r, n := jsstr.CodePointAt(u, i)
		out = append(out, r)
		i += n
	}
	return out
}

func fromCodePoints(cps []rune) string {
	u := make([]uint16, 0, len(cps))
	for _, r := range cps {
		u = jsstr.AppendCodePoint(u, r)
	}
	return jsstr.FromUTF16(u)
}

// decompose replaces every code point with its full canonical decomposition and
// puts the result in canonical order: each run of non-starters is stably sorted by
// combining class.
func decompose(cps []rune) []rune {
	out := make([]rune, 0, len(cps))
	for _, r := range cps {
		switch {
		case r >= sBase && r < sBase+sCount:
			s := r - sBase
			out = append(out, lBase+s/nCount, vBase+(s%nCount)/tCount)
			if s%tCount != 0 {
				out = append(out, tBase+s%tCount)
			}
		case decompositions[r] != nil:
			out = append(out, decompositions[r]...)
		default:
			out = append(out, r)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && combiningClass[out[j]] != 0 && combiningClass[out[j-1]] > combiningClass[out[j]]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// compose is the canonical composition algorithm over a decomposed, canonically
// ordered sequence: each character combines with the last starter unless a
// character between them blocks it (one of the same or a higher combining class;
// characters of class 0 become the new starter).
func compose(cps []rune) []rune {
	out := make([]rune, 0, len(cps))
	starter := -1
	var last uint8
	for _, r := range cps {
		c := combiningClass[r]
		if starter >= 0 && (len(out)-1 == starter || last < c) {
			if composite, ok := composePair(out[starter], r); ok {
				out[starter] = composite
				continue
			}
		}
		if c == 0 {
			starter = len(out)
		}
		last = c
		out = append(out, r)
	}
	return out
}

// composePair returns the primary composite of a and b, if any.
func composePair(a, b rune) (rune, bool) {
	switch {
	case a >= lBase && a < lBase+lCount && b >= vBase && b < vBase+vCount:
		return sBase + ((a-lBase)*vCount+(b-vBase))*tCount, true
	case a >= sBase && a < sBase+sCount && (a-sBase)%tCount == 0 && b > tBase && b < tBase+tCount:
		return a + (b - tBase), true
	}
	c, ok := compositions[[2]rune{a, b}]
	return c, ok
}
