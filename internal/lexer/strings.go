package lexer

import "github.com/edge-js/edge-go/internal/jsrt/jsstr"

// isBlank is `!chunk.trim()` over code units. Every white space character lies in
// the Basic Multilingual Plane, so a surrogate is never blank.
func isBlank(chunk []uint16) bool {
	for _, c := range chunk {
		if !jsstr.IsSpace(rune(c)) {
			return false
		}
	}
	return true
}

// isSpaceUnit is the regular expression class \s for one code unit.
func isSpaceUnit(c uint16) bool { return jsstr.IsSpace(rune(c)) }
