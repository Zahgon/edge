package acorn

import "sort"

// codeRange is an inclusive range of code points.
type codeRange struct{ lo, hi rune }

// inRanges reports whether c lies in one of the sorted ranges rs.
func inRanges(c rune, rs []codeRange) bool {
	i := sort.Search(len(rs), func(i int) bool { return rs[i].hi >= c })
	return i < len(rs) && rs[i].lo <= c
}

// isIdentifierStart is acorn's isIdentifierStart: whether the code point starts an
// identifier. Code points above U+FFFF count only when astral is set (ecmaVersion 6
// and later). The not-a-character value noChar never matches.
func isIdentifierStart(code rune, astral bool) bool {
	switch {
	case code < 65:
		return code == '$'
	case code < 91:
		return true
	case code < 97:
		return code == '_'
	case code < 123:
		return true
	case code <= 0xffff:
		return code >= 0xaa && inRanges(code, identifierStartRanges)
	case !astral:
		return false
	}
	return inRanges(code, identifierStartRanges)
}

// isIdentifierChar is acorn's isIdentifierChar: whether the code point may continue
// an identifier.
func isIdentifierChar(code rune, astral bool) bool {
	switch {
	case code < 48:
		return code == '$'
	case code < 58:
		return true
	case code < 65:
		return false
	case code < 91:
		return true
	case code < 97:
		return code == '_'
	case code < 123:
		return true
	case code <= 0xffff:
		return code >= 0xaa && inRanges(code, identifierCharRanges)
	case !astral:
		return false
	}
	return inRanges(code, identifierCharRanges)
}
