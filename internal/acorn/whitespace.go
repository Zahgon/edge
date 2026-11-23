package acorn

import (
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// nextLineBreak returns the offset just after the first line break in
// code[from:end] (a CRLF pair counting as one), or -1.
func nextLineBreak(code []uint16, from, end int) int {
	for i := from; i < end; i++ {
		next := rune(-1)
		if i < len(code) {
			next = rune(code[i])
		}
		if jsstr.IsLineTerminator(next) {
			if i < end-1 && next == 13 && i+1 < len(code) && code[i+1] == 10 {
				return i + 2
			}
			return i + 1
		}
	}
	return -1
}

// hasLineBreak is acorn's lineBreak.test on a piece of the input.
func hasLineBreak(s []uint16) bool {
	for _, c := range s {
		if jsstr.IsLineTerminator(rune(c)) {
			return true
		}
	}
	return false
}

// skipWhiteSpace returns the end of the run of white space and comments starting at
// pos, as acorn's skipWhiteSpace regular expression
// /(?:\s|\/\/.*|\/\*[^]*?\*\/)*/ matches it. An unterminated block comment ends the
// run before its "/*".
func skipWhiteSpace(input []uint16, pos int) int {
	for pos < len(input) {
		c := rune(input[pos])
		switch {
		case jsstr.IsSpace(c):
			pos++
		case c == '/' && pos+1 < len(input) && input[pos+1] == '/':
			pos += 2
			for pos < len(input) && !jsstr.IsLineTerminator(rune(input[pos])) {
				pos++
			}
		case c == '/' && pos+1 < len(input) && input[pos+1] == '*':
			end := jsstr.Index(input, []uint16{'*', '/'}, pos+2)
			if end < 0 {
				return pos
			}
			pos = end + 2
		default:
			return pos
		}
	}
	return pos
}
