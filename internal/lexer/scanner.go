package lexer

import (
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// Scanner reads chunks of a statement until its closing pattern appears, ignoring
// the pattern while a tolerated pair (parentheses, curly braces) is open. Feed it a
// line at a time; "\n" marks a line break.
//
//	s := lexer.NewScanner(")", [2]string{"(", ")"}, 1, 0)
//	s.Scan("2 + 2 * (3))")
//	s.Closed     // true
//	s.Match()    // "2 + 2 * (3)"
//	s.LeftOver() // ""
type Scanner struct {
	pattern     []uint16
	tolerateLhs []uint16
	tolerateRhs []uint16
	toleration  int

	// Closed reports whether the closing pattern has been found.
	Closed bool
	// Loc is the position after the closing pattern once Closed, the position the
	// scanner reached before.
	Loc LexerPosition

	match    []uint16
	leftOver []uint16
}

// NewScanner is `new Scanner(pattern, toleratePair, line, col)`.
func NewScanner(pattern string, toleratePair [2]string, line, col int) *Scanner {
	return &Scanner{
		pattern:     jsstr.ToUTF16(pattern),
		tolerateLhs: jsstr.ToUTF16(toleratePair[0]),
		tolerateRhs: jsstr.ToUTF16(toleratePair[1]),
		Loc:         LexerPosition{Line: line, Col: col},
	}
}

// Match is the text scanned before the closing pattern.
func (s *Scanner) Match() string { return jsstr.FromUTF16(s.match) }

// LeftOver is the rest of the chunk after the closing pattern.
func (s *Scanner) LeftOver() string { return jsstr.FromUTF16(s.leftOver) }

// Scan feeds one chunk (a line, or "\n" for a line break) to the scanner.
func (s *Scanner) Scan(chunk string) { s.scan(jsstr.ToUTF16(chunk)) }

// matchesPattern reports whether the pattern starts at chars[at]; the original
// compares one character at a time, a missing character never matching.
func (s *Scanner) matchesPattern(chars []uint16, at int) bool {
	for i, c := range s.pattern {
		if at+i >= len(chars) || chars[at+i] != c {
			return false
		}
	}
	return true
}

// isChar is `char === pair`, where a pair of more than one code unit never equals
// the single character.
func isChar(c uint16, pair []uint16) bool {
	return len(pair) == 1 && pair[0] == c
}

func (s *Scanner) scan(chunk []uint16) {
	if len(chunk) == 1 && chunk[0] == '\n' {
		s.Loc.Line++
		s.Loc.Col = 0
		s.match = append(s.match, '\n')
		return
	}
	if isBlank(chunk) {
		return
	}
	i := 0
	for i < len(chunk) {
		c := chunk[i]
		if s.toleration == 0 && s.matchesPattern(chunk, i) {
			i += len(s.pattern)
			s.Closed = true
			break
		}
		if isChar(c, s.tolerateLhs) {
			s.toleration++
		}
		if isChar(c, s.tolerateRhs) {
			s.toleration--
		}
		s.match = append(s.match, c)
		i++
	}
	if s.Closed {
		s.Loc.Col += i
		s.leftOver = chunk[i:]
	}
}
