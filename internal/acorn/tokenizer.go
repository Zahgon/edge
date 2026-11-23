package acorn

import (
	"strconv"

	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/js"
)

// tokContext is acorn's TokContext: the syntactic context the tokenizer tracks to
// tell a regular expression from a division.
type tokContext struct {
	token         string
	isExpr        bool
	preserveSpace bool
	// readsTemplate marks the template context, whose tokens are read by
	// tryReadTemplateToken.
	readsTemplate bool
	generator     bool
}

var (
	ctxBStat    = &tokContext{token: "{"}
	ctxBExpr    = &tokContext{token: "{", isExpr: true}
	ctxBTmpl    = &tokContext{token: "${"}
	ctxPStat    = &tokContext{token: "("}
	ctxPExpr    = &tokContext{token: "(", isExpr: true}
	ctxQTmpl    = &tokContext{token: "`", isExpr: true, preserveSpace: true, readsTemplate: true}
	ctxFStat    = &tokContext{token: "function"}
	ctxFExpr    = &tokContext{token: "function", isExpr: true}
	ctxFExprGen = &tokContext{token: "function", isExpr: true, generator: true}
	ctxFGen     = &tokContext{token: "function", generator: true}
)

func (p *parser) curContext() *tokContext {
	if len(p.context) == 0 {
		return nil
	}
	return p.context[len(p.context)-1]
}

func (p *parser) popContext() *tokContext {
	c := p.curContext()
	if len(p.context) > 0 {
		p.context = p.context[:len(p.context)-1]
	}
	return c
}

func (p *parser) braceIsBlock(prevType *tokenType) bool {
	parent := p.curContext()
	if parent == ctxFExpr || parent == ctxFStat {
		return true
	}
	if prevType == tokColon && (parent == ctxBStat || parent == ctxBExpr) {
		return !parent.isExpr
	}
	// The check for name && exprAllowed detects whether we are after a yield or
	// an of, see updateContext for names.
	if prevType == tokReturn || prevType == tokName && p.exprAllowed {
		return hasLineBreak(p.input[p.lastTokEnd:p.start])
	}
	if prevType == tokElse || prevType == tokSemi || prevType == tokEOF || prevType == tokParenR || prevType == tokArrow {
		return true
	}
	if prevType == tokBraceL {
		return parent == ctxBStat
	}
	if prevType == tokVar || prevType == tokConst || prevType == tokName {
		return false
	}
	return !p.exprAllowed
}

func (p *parser) inGeneratorContext() bool {
	for i := len(p.context) - 1; i >= 1; i-- {
		if c := p.context[i]; c.token == "function" {
			return c.generator
		}
	}
	return false
}

// updateContext maintains the context stack and exprAllowed after the token of
// type p.typ was read, prevType being the type of the token before it.
func (p *parser) updateContext(prevType *tokenType) {
	t := p.typ
	if t.keyword != "" && prevType == tokDot {
		p.exprAllowed = false
		return
	}
	switch t {
	case tokParenR, tokBraceR:
		if len(p.context) == 1 {
			p.exprAllowed = true
			return
		}
		out := p.popContext()
		if out == ctxBStat && p.curContext().token == "function" {
			out = p.popContext()
		}
		p.exprAllowed = !out.isExpr
	case tokBraceL:
		if p.braceIsBlock(prevType) {
			p.context = append(p.context, ctxBStat)
		} else {
			p.context = append(p.context, ctxBExpr)
		}
		p.exprAllowed = true
	case tokDollarBraceL:
		p.context = append(p.context, ctxBTmpl)
		p.exprAllowed = true
	case tokParenL:
		if prevType == tokIf || prevType == tokFor || prevType == tokWith || prevType == tokWhile {
			p.context = append(p.context, ctxPStat)
		} else {
			p.context = append(p.context, ctxPExpr)
		}
		p.exprAllowed = true
	case tokIncDec:
		// exprAllowed stays unchanged.
	case tokFunction, tokClass:
		if prevType.beforeExpr && prevType != tokElse &&
			!(prevType == tokSemi && p.curContext() != ctxPStat) &&
			!(prevType == tokReturn && hasLineBreak(p.input[p.lastTokEnd:p.start])) &&
			!((prevType == tokColon || prevType == tokBraceL) && p.curContext() == ctxBStat) {
			p.context = append(p.context, ctxFExpr)
		} else {
			p.context = append(p.context, ctxFStat)
		}
		p.exprAllowed = false
	case tokColon:
		if p.curContext().token == "function" {
			p.popContext()
		}
		p.exprAllowed = true
	case tokBackQuote:
		if p.curContext() == ctxQTmpl {
			p.popContext()
		} else {
			p.context = append(p.context, ctxQTmpl)
		}
		p.exprAllowed = false
	case tokStar:
		if prevType == tokFunction {
			index := len(p.context) - 1
			if p.context[index] == ctxFExpr {
				p.context[index] = ctxFExprGen
			} else {
				p.context[index] = ctxFGen
			}
		}
		p.exprAllowed = true
	case tokName:
		allowed := false
		if p.ecmaVersion >= 6 && prevType != tokDot {
			if p.value == "of" && !p.exprAllowed || p.value == "yield" && p.inGeneratorContext() {
				allowed = true
			}
		}
		p.exprAllowed = allowed
	default:
		p.exprAllowed = t.beforeExpr
	}
}

// overrideContext replaces the current context where the tokenizer could not infer
// it correctly.
func (p *parser) overrideContext(c *tokContext) {
	if p.curContext() != c {
		p.context[len(p.context)-1] = c
	}
}

// next moves to the next token, reporting the current one to OnToken.
func (p *parser) next(ignoreEscapeSequenceInKeyword bool) {
	if !ignoreEscapeSequenceInKeyword && p.typ.keyword != "" && p.containsEsc {
		p.raiseRecoverable(p.start, "Escape sequence in keyword "+p.typ.keyword)
	}
	if p.onToken != nil {
		tok := &Token{Type: p.typ.label, Value: p.value, Start: p.start, End: p.end}
		if p.locations {
			tok.Loc = &SourceLocation{Start: p.startLoc, End: p.endLoc}
		}
		p.onToken(tok)
	}
	p.lastTokEnd = p.end
	p.lastTokStart = p.start
	p.lastTokEndLoc = p.endLoc
	p.lastTokStartLoc = p.startLoc
	p.nextToken()
}

// nextToken reads a single token, updating the token properties of the parser.
func (p *parser) nextToken() {
	c := p.curContext()
	if c == nil || !c.preserveSpace {
		p.skipSpace()
	}
	p.start = p.pos
	if p.locations {
		p.startLoc = p.curPosition()
	}
	if p.pos >= len(p.input) {
		p.finishToken(tokEOF, nil)
		return
	}
	if c.readsTemplate {
		p.tryReadTemplateToken()
		return
	}
	p.readToken(p.fullCharCodeAt(p.pos))
}

func (p *parser) readToken(code rune) {
	// Identifier or keyword. \uXXXX sequences are allowed in identifiers, so a
	// backslash also dispatches to readWord.
	if isIdentifierStart(code, p.ecmaVersion >= 6) || code == '\\' {
		p.readWord()
		return
	}
	p.getTokenFromCode(code)
}

// charCodeAt is String.prototype.charCodeAt on the input, noChar past its end.
func (p *parser) charCodeAt(pos int) rune {
	if pos < 0 || pos >= len(p.input) {
		return noChar
	}
	return rune(p.input[pos])
}

// fullCharCodeAt reads the code point at pos, combining a surrogate pair. A lead
// surrogate at the very end of the input gives noChar, as the NaN acorn computes
// from the missing trail does.
func (p *parser) fullCharCodeAt(pos int) rune {
	code := p.charCodeAt(pos)
	if code == noChar || code <= 0xd7ff || code >= 0xdc00 {
		return code
	}
	next := p.charCodeAt(pos + 1)
	if next == noChar {
		return noChar
	}
	if next <= 0xdbff || next >= 0xe000 {
		return code
	}
	return code<<10 + next - 0x35fdc00
}

// slice is input.slice(start, end) as a Go string.
func (p *parser) slice(start, end int) string {
	return jsstr.FromUTF16(p.input[start:end])
}

// codePointToString is acorn's codePointToString. NaN (noChar) comes out as
// U+10000, as String.fromCharCode makes of NaN arithmetic.
func codePointToString(code rune) []uint16 {
	if code == noChar {
		return []uint16{0xd800, 0xdc00}
	}
	return jsstr.AppendCodePoint(nil, code)
}

func (p *parser) skipBlockComment() {
	start := p.pos
	end := jsstr.Index(p.input, []uint16{'*', '/'}, p.pos+2)
	p.pos += 2
	if end == -1 {
		p.raise(p.pos-2, "Unterminated comment")
	}
	p.pos = end + 2
	if p.locations {
		for pos := start; ; {
			nextBreak := nextLineBreak(p.input, pos, p.pos)
			if nextBreak < 0 {
				break
			}
			p.curLine++
			pos = nextBreak
			p.lineStart = nextBreak
		}
	}
}

func (p *parser) skipLineComment(startSkip int) {
	p.pos += startSkip
	ch := p.charCodeAt(p.pos)
	for p.pos < len(p.input) && !jsstr.IsLineTerminator(ch) {
		p.pos++
		ch = p.charCodeAt(p.pos)
	}
}

// skipSpace skips white space and comments; it runs before every token.
func (p *parser) skipSpace() {
	for p.pos < len(p.input) {
		ch := rune(p.input[p.pos])
		switch {
		case ch == 32 || ch == 160:
			p.pos++
		case ch == 13 || ch == 10 || ch == 0x2028 || ch == 0x2029:
			if ch == 13 && p.charCodeAt(p.pos+1) == 10 {
				p.pos++
			}
			p.pos++
			if p.locations {
				p.curLine++
				p.lineStart = p.pos
			}
		case ch == '/':
			switch p.charCodeAt(p.pos + 1) {
			case '*':
				p.skipBlockComment()
			case '/':
				p.skipLineComment(2)
			default:
				return
			}
		case ch > 8 && ch < 14 || ch >= 5760 && jsstr.IsWhiteSpace(ch):
			p.pos++
		default:
			return
		}
	}
}

// finishToken ends the token at the current position with the given type and
// value and updates the context.
func (p *parser) finishToken(t *tokenType, val any) {
	p.end = p.pos
	if p.locations {
		p.endLoc = p.curPosition()
	}
	prevType := p.typ
	p.typ = t
	p.value = val
	p.updateContext(prevType)
}

func (p *parser) readTokenDot() {
	next := p.charCodeAt(p.pos + 1)
	if next >= 48 && next <= 57 {
		p.readNumber(true)
		return
	}
	next2 := p.charCodeAt(p.pos + 2)
	if p.ecmaVersion >= 6 && next == '.' && next2 == '.' {
		p.pos += 3
		p.finishToken(tokEllipsis, nil)
		return
	}
	p.pos++
	p.finishToken(tokDot, nil)
}

func (p *parser) readTokenSlash() {
	next := p.charCodeAt(p.pos + 1)
	if p.exprAllowed {
		p.pos++
		p.readRegexp()
		return
	}
	if next == '=' {
		p.finishOp(tokAssign, 2)
		return
	}
	p.finishOp(tokSlash, 1)
}

func (p *parser) readTokenMultModuloExp(code rune) {
	next := p.charCodeAt(p.pos + 1)
	size := 1
	t := tokModulo
	if code == '*' {
		t = tokStar
	}
	// The exponentiation operators ** and **=.
	if p.ecmaVersion >= 7 && code == '*' && next == '*' {
		size++
		t = tokStarstar
		next = p.charCodeAt(p.pos + 2)
	}
	if next == '=' {
		p.finishOp(tokAssign, size+1)
		return
	}
	p.finishOp(t, size)
}

func (p *parser) readTokenPipeAmp(code rune) {
	next := p.charCodeAt(p.pos + 1)
	if next == code {
		if code == '|' {
			p.finishOp(tokLogicalOR, 2)
		} else {
			p.finishOp(tokLogicalAND, 2)
		}
		return
	}
	if next == '=' {
		p.finishOp(tokAssign, 2)
		return
	}
	if code == '|' {
		p.finishOp(tokBitwiseOR, 1)
	} else {
		p.finishOp(tokBitwiseAND, 1)
	}
}

func (p *parser) readTokenCaret() {
	if p.charCodeAt(p.pos+1) == '=' {
		p.finishOp(tokAssign, 2)
		return
	}
	p.finishOp(tokBitwiseXOR, 1)
}

func (p *parser) readTokenPlusMin(code rune) {
	next := p.charCodeAt(p.pos + 1)
	if next == code {
		if next == '-' && p.charCodeAt(p.pos+2) == '>' &&
			(p.lastTokEnd == 0 || hasLineBreak(p.input[p.lastTokEnd:p.pos])) {
			// A --> line comment.
			p.skipLineComment(3)
			p.skipSpace()
			p.nextToken()
			return
		}
		p.finishOp(tokIncDec, 2)
		return
	}
	if next == '=' {
		p.finishOp(tokAssign, 2)
		return
	}
	p.finishOp(tokPlusMin, 1)
}

func (p *parser) readTokenLtGt(code rune) {
	next := p.charCodeAt(p.pos + 1)
	size := 1
	if next == code {
		size = 2
		if code == '>' && p.charCodeAt(p.pos+2) == '>' {
			size = 3
		}
		if p.charCodeAt(p.pos+size) == '=' {
			p.finishOp(tokAssign, size+1)
			return
		}
		p.finishOp(tokBitShift, size)
		return
	}
	if next == '!' && code == '<' && p.charCodeAt(p.pos+2) == '-' && p.charCodeAt(p.pos+3) == '-' {
		// <!--, an XML-style comment that is a line comment.
		p.skipLineComment(4)
		p.skipSpace()
		p.nextToken()
		return
	}
	if next == '=' {
		size = 2
	}
	p.finishOp(tokRelational, size)
}

func (p *parser) readTokenEqExcl(code rune) {
	next := p.charCodeAt(p.pos + 1)
	if next == '=' {
		size := 2
		if p.charCodeAt(p.pos+2) == '=' {
			size = 3
		}
		p.finishOp(tokEquality, size)
		return
	}
	if code == '=' && next == '>' && p.ecmaVersion >= 6 {
		p.pos += 2
		p.finishToken(tokArrow, nil)
		return
	}
	if code == '=' {
		p.finishOp(tokEq, 1)
	} else {
		p.finishOp(tokPrefix, 1)
	}
}

func (p *parser) readTokenQuestion() {
	if p.ecmaVersion >= 11 {
		next := p.charCodeAt(p.pos + 1)
		if next == '.' {
			// Past the end of the input NaN fails both comparisons: "?." at the
			// very end is read as "?".
			if next2 := p.charCodeAt(p.pos + 2); next2 != noChar && (next2 < 48 || next2 > 57) {
				p.finishOp(tokQuestionDot, 2)
				return
			}
		}
		if next == '?' {
			p.finishOp(tokCoalesce, 2)
			return
		}
	}
	p.finishOp(tokQuestion, 1)
}

func (p *parser) unexpectedCharacter(code rune) {
	p.raise(p.pos, "Unexpected character '"+jsstr.FromUTF16(codePointToString(code))+"'")
}

func (p *parser) getTokenFromCode(code rune) {
	switch code {
	// The interpretation of a dot depends on whether it is followed by a digit or
	// another two dots.
	case '.':
		p.readTokenDot()
		return

	// Punctuation tokens.
	case '(', ')', ';', ',', '[', ']', '{', '}', ':':
		p.pos++
		p.finishToken(punctuation[code], nil)
		return

	case '`':
		if p.ecmaVersion < 6 {
			break
		}
		p.pos++
		p.finishToken(tokBackQuote, nil)
		return

	case '0':
		next := p.charCodeAt(p.pos + 1)
		if next == 'x' || next == 'X' {
			p.readRadixNumber(16)
			return
		}
		if p.ecmaVersion >= 6 {
			if next == 'o' || next == 'O' {
				p.readRadixNumber(8)
				return
			}
			if next == 'b' || next == 'B' {
				p.readRadixNumber(2)
				return
			}
		}
		// Anything else beginning with a digit is an integer, octal number, or
		// float.
		p.readNumber(false)
		return
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		p.readNumber(false)
		return

	case '"', '\'':
		p.readString(code)
		return

	case '/':
		p.readTokenSlash()
		return
	case '%', '*':
		p.readTokenMultModuloExp(code)
		return
	case '|', '&':
		p.readTokenPipeAmp(code)
		return
	case '^':
		p.readTokenCaret()
		return
	case '+', '-':
		p.readTokenPlusMin(code)
		return
	case '<', '>':
		p.readTokenLtGt(code)
		return
	case '=', '!':
		p.readTokenEqExcl(code)
		return
	case '?':
		p.readTokenQuestion()
		return
	case '~':
		p.finishOp(tokPrefix, 1)
		return
	}
	// '#' and every other character: private names start with # only from
	// ECMAScript 2022.
	p.unexpectedCharacter(code)
}

var punctuation = map[rune]*tokenType{
	'(': tokParenL, ')': tokParenR, ';': tokSemi, ',': tokComma,
	'[': tokBracketL, ']': tokBracketR, '{': tokBraceL, '}': tokBraceR, ':': tokColon,
}

func (p *parser) finishOp(t *tokenType, size int) {
	str := p.slice(p.pos, p.pos+size)
	p.pos += size
	p.finishToken(t, str)
}

func (p *parser) readRegexp() {
	escaped, inClass, start := false, false, p.pos
	for {
		if p.pos >= len(p.input) {
			p.raise(start, "Unterminated regular expression")
		}
		ch := p.input[p.pos]
		if jsstr.IsLineTerminator(rune(ch)) {
			p.raise(start, "Unterminated regular expression")
		}
		if !escaped {
			if ch == '[' {
				inClass = true
			} else if ch == ']' && inClass {
				inClass = false
			} else if ch == '/' && !inClass {
				break
			}
			escaped = ch == '\\'
		} else {
			escaped = false
		}
		p.pos++
	}
	pattern := p.input[start:p.pos]
	p.pos++
	flagsStart := p.pos
	flags := p.readWord1()
	if p.containsEsc {
		p.unexpectedAt(flagsStart)
	}

	if p.regexpState == nil {
		p.regexpState = newRegexpState(p)
	}
	state := p.regexpState
	state.reset(start, pattern, flags)
	p.validateRegExpFlags(state)
	p.validateRegExpPattern(state)

	p.finishToken(tokRegexp, &RegexLiteral{Pattern: jsstr.FromUTF16(pattern), Flags: flags})
}

// readInt reads digits in the given radix and returns their value, accumulated in
// double precision as acorn does. With hasLength it reads at most length digits
// and fails unless it read exactly that many; otherwise it fails when it read none.
func (p *parser) readInt(radix int, length int, hasLength bool) (float64, bool) {
	start, total := p.pos, 0.0
	for i := 0; !hasLength || i < length; i, p.pos = i+1, p.pos+1 {
		code := p.charCodeAt(p.pos)
		val := radix
		switch {
		case code >= 97:
			val = int(code - 97 + 10)
		case code >= 65:
			val = int(code - 65 + 10)
		case code >= 48 && code <= 57:
			val = int(code - 48)
		}
		if val >= radix {
			break
		}
		// The explicit conversion keeps the product from being fused with the
		// addition; JavaScript rounds it separately.
		total = float64(total*float64(radix)) + float64(val)
	}
	if p.pos == start || hasLength && p.pos-start != length {
		return 0, false
	}
	return total, true
}

// stringToBigInt is BigInt(str) for the text of a BigInt literal without its n.
func stringToBigInt(str string) js.BigInt {
	base := 10
	if len(str) > 1 && str[0] == '0' {
		switch str[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 10 {
			str = str[2:]
		}
	}
	n, ok := js.ParseBigInt(str, base)
	if !ok {
		// The tokenizer has read the digits of this base and nothing else.
		panic("acorn: BigInt literal text " + strconv.Quote(str) + " is not a number")
	}
	return n
}

func (p *parser) readRadixNumber(radix int) {
	start := p.pos
	p.pos += 2 // 0x
	val, ok := p.readInt(radix, 0, false)
	if !ok {
		p.raise(p.start+2, "Expected number in radix "+strconv.Itoa(radix))
	}
	if p.ecmaVersion >= 11 && p.charCodeAt(p.pos) == 'n' {
		bigVal := stringToBigInt(p.slice(start, p.pos))
		p.pos++
		p.finishToken(tokNum, bigVal)
		return
	}
	if isIdentifierStart(p.fullCharCodeAt(p.pos), true) {
		p.raise(p.pos, "Identifier directly after number")
	}
	p.finishToken(tokNum, val)
}

// readNumber reads an integer, legacy octal integer, or floating-point number.
func (p *parser) readNumber(startsWithDot bool) {
	start := p.pos
	if !startsWithDot {
		if _, ok := p.readInt(10, 0, false); !ok {
			p.raise(start, "Invalid number")
		}
	}
	octal := p.pos-start >= 2 && p.input[start] == '0'
	if octal && p.strict {
		p.raise(start, "Invalid number")
	}
	next := p.charCodeAt(p.pos)
	if !octal && !startsWithDot && p.ecmaVersion >= 11 && next == 'n' {
		val := stringToBigInt(p.slice(start, p.pos))
		p.pos++
		if isIdentifierStart(p.fullCharCodeAt(p.pos), true) {
			p.raise(p.pos, "Identifier directly after number")
		}
		p.finishToken(tokNum, val)
		return
	}
	if octal {
		for _, c := range p.input[start:p.pos] {
			if c == '8' || c == '9' {
				octal = false
				break
			}
		}
	}
	if next == '.' && !octal {
		p.pos++
		p.readInt(10, 0, false)
		next = p.charCodeAt(p.pos)
	}
	if (next == 'E' || next == 'e') && !octal {
		p.pos++
		next = p.charCodeAt(p.pos)
		if next == '+' || next == '-' {
			p.pos++
		}
		if _, ok := p.readInt(10, 0, false); !ok {
			p.raise(start, "Invalid number")
		}
	}
	if isIdentifierStart(p.fullCharCodeAt(p.pos), true) {
		p.raise(p.pos, "Identifier directly after number")
	}
	str := p.slice(start, p.pos)
	if octal {
		p.finishToken(tokNum, jsnum.ParseInt(str, 8))
	} else {
		p.finishToken(tokNum, jsnum.ParseFloat(str))
	}
}

// readCodePoint reads the code point of a \u escape, after the u.
func (p *parser) readCodePoint() float64 {
	if p.charCodeAt(p.pos) != '{' {
		return p.readHexChar(4)
	}
	if p.ecmaVersion < 6 {
		p.unexpected()
	}
	p.pos++
	codePos := p.pos
	code := p.readHexChar(jsstr.Index(p.input, []uint16{'}'}, p.pos) - p.pos)
	p.pos++
	if code > 0x10ffff {
		p.invalidStringToken(codePos, "Code point out of bounds")
	}
	return code
}

func (p *parser) readString(quote rune) {
	var out []uint16
	p.pos++
	chunkStart := p.pos
	for {
		if p.pos >= len(p.input) {
			p.raise(p.start, "Unterminated string constant")
		}
		ch := rune(p.input[p.pos])
		if ch == quote {
			break
		}
		switch {
		case ch == '\\':
			out = append(out, p.input[chunkStart:p.pos]...)
			out = append(out, p.readEscapedChar(false)...)
			chunkStart = p.pos
		case ch == 0x2028 || ch == 0x2029:
			if p.ecmaVersion < 10 {
				p.raise(p.start, "Unterminated string constant")
			}
			p.pos++
			if p.locations {
				p.curLine++
				p.lineStart = p.pos
			}
		default:
			if jsstr.IsLineTerminator(ch) {
				p.raise(p.start, "Unterminated string constant")
			}
			p.pos++
		}
	}
	out = append(out, p.input[chunkStart:p.pos]...)
	p.pos++
	p.finishToken(tokString, jsstr.FromUTF16(out))
}

// invalidTemplateEscape is panicked by invalidStringToken inside a template
// element, where tagged templates allow invalid escapes.
type invalidTemplateEscape struct{}

func (p *parser) tryReadTemplateToken() {
	p.inTemplateElement = true
	if !p.readTmplTokenValidEscapes() {
		p.readInvalidTemplateToken()
	}
	p.inTemplateElement = false
}

// readTmplTokenValidEscapes runs readTmplToken and reports false when it stopped at
// an invalid escape sequence.
func (p *parser) readTmplTokenValidEscapes() (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, invalid := r.(invalidTemplateEscape); !invalid {
				panic(r)
			}
			ok = false
		}
	}()
	p.readTmplToken()
	return true
}

func (p *parser) invalidStringToken(position int, message string) {
	if p.inTemplateElement && p.ecmaVersion >= 9 {
		panic(invalidTemplateEscape{})
	}
	p.raise(position, message)
}

func (p *parser) readTmplToken() {
	var out []uint16
	chunkStart := p.pos
	for {
		if p.pos >= len(p.input) {
			p.raise(p.start, "Unterminated template")
		}
		ch := rune(p.input[p.pos])
		if ch == '`' || ch == '$' && p.charCodeAt(p.pos+1) == '{' {
			if p.pos == p.start && (p.typ == tokTemplate || p.typ == tokInvalidTemplate) {
				if ch == '$' {
					p.pos += 2
					p.finishToken(tokDollarBraceL, nil)
					return
				}
				p.pos++
				p.finishToken(tokBackQuote, nil)
				return
			}
			out = append(out, p.input[chunkStart:p.pos]...)
			p.finishToken(tokTemplate, jsstr.FromUTF16(out))
			return
		}
		switch {
		case ch == '\\':
			out = append(out, p.input[chunkStart:p.pos]...)
			out = append(out, p.readEscapedChar(true)...)
			chunkStart = p.pos
		case jsstr.IsLineTerminator(ch):
			out = append(out, p.input[chunkStart:p.pos]...)
			p.pos++
			switch ch {
			case 13, 10:
				if ch == 13 && p.charCodeAt(p.pos) == 10 {
					p.pos++
				}
				out = append(out, '\n')
			default:
				out = append(out, uint16(ch))
			}
			if p.locations {
				p.curLine++
				p.lineStart = p.pos
			}
			chunkStart = p.pos
		default:
			p.pos++
		}
	}
}

// readInvalidTemplateToken finds the end of a template element holding an invalid
// escape sequence, without validating escapes.
func (p *parser) readInvalidTemplateToken() {
	for ; p.pos < len(p.input); p.pos++ {
		switch p.input[p.pos] {
		case '\\':
			p.pos++
		case '$':
			if p.charCodeAt(p.pos+1) != '{' {
				break
			}
			p.finishToken(tokInvalidTemplate, p.slice(p.start, p.pos))
			return
		case '`':
			p.finishToken(tokInvalidTemplate, p.slice(p.start, p.pos))
			return
		case '\r', '\n', 0x2028, 0x2029:
			if p.input[p.pos] == '\r' && p.charCodeAt(p.pos+1) == '\n' {
				p.pos++
			}
			p.curLine++
			p.lineStart = p.pos + 1
		}
	}
	p.raise(p.start, "Unterminated template")
}

// readEscapedChar reads the escape sequence after a backslash and returns the code
// units it stands for.
func (p *parser) readEscapedChar(inTemplate bool) []uint16 {
	p.pos++
	ch := p.charCodeAt(p.pos)
	p.pos++
	switch ch {
	case 'n':
		return []uint16{'\n'}
	case 'r':
		return []uint16{'\r'}
	case 'x':
		return []uint16{uint16(p.readHexChar(2))}
	case 'u':
		return codePointToString(rune(p.readCodePoint()))
	case 't':
		return []uint16{'\t'}
	case 'b':
		return []uint16{'\b'}
	case 'v':
		return []uint16{'\v'}
	case 'f':
		return []uint16{'\f'}
	case 13, 10:
		if ch == 13 && p.charCodeAt(p.pos) == 10 {
			p.pos++
		}
		if p.locations {
			p.lineStart = p.pos
			p.curLine++
		}
		return nil
	case '8', '9':
		if p.strict {
			p.invalidStringToken(p.pos-1, "Invalid escape sequence")
		}
		if inTemplate {
			p.invalidStringToken(p.pos-1, "Invalid escape sequence in template string")
		}
	}
	if ch >= '0' && ch <= '7' {
		digits := p.pos - 1
		for digits < len(p.input) && digits < p.pos+2 && p.input[digits] >= '0' && p.input[digits] <= '7' {
			digits++
		}
		octalStr := p.input[p.pos-1 : digits]
		octal := octalValue(octalStr)
		if octal > 255 {
			octalStr = octalStr[:len(octalStr)-1]
			octal = octalValue(octalStr)
		}
		p.pos += len(octalStr) - 1
		ch = p.charCodeAt(p.pos)
		if (len(octalStr) != 1 || octalStr[0] != '0' || ch == '8' || ch == '9') && (p.strict || inTemplate) {
			message := "Octal literal in strict mode"
			if inTemplate {
				message = "Octal literal in template string"
			}
			p.invalidStringToken(p.pos-1-len(octalStr), message)
		}
		return []uint16{uint16(octal)}
	}
	if jsstr.IsLineTerminator(ch) {
		// Unicode line terminators after a backslash are removed from strings
		// and templates.
		if p.locations {
			p.lineStart = p.pos
			p.curLine++
		}
		return nil
	}
	if ch == noChar {
		// String.fromCharCode(NaN), past the end of an unterminated string.
		return []uint16{0}
	}
	return []uint16{uint16(ch)}
}

func octalValue(digits []uint16) int {
	n := 0
	for _, d := range digits {
		n = n*8 + int(d-'0')
	}
	return n
}

// readHexChar reads the hexadecimal digits of \x, \u and \u{...} escapes; length
// is the number of digits the escape needs.
func (p *parser) readHexChar(length int) float64 {
	codePos := p.pos
	n, ok := p.readInt(16, length, true)
	if !ok {
		p.invalidStringToken(codePos, "Bad character escape sequence")
	}
	return n
}

// readWord1 reads an identifier and returns it, setting containsEsc when it holds
// a \u escape.
func (p *parser) readWord1() string {
	p.containsEsc = false
	var word []uint16
	first, chunkStart := true, p.pos
	astral := p.ecmaVersion >= 6
	for p.pos < len(p.input) {
		ch := p.fullCharCodeAt(p.pos)
		if isIdentifierChar(ch, astral) {
			if ch <= 0xffff {
				p.pos++
			} else {
				p.pos += 2
			}
		} else if ch == '\\' {
			p.containsEsc = true
			word = append(word, p.input[chunkStart:p.pos]...)
			escStart := p.pos
			p.pos++
			if p.charCodeAt(p.pos) != 'u' {
				p.invalidStringToken(p.pos, "Expecting Unicode escape sequence \\uXXXX")
			}
			p.pos++
			esc := rune(p.readCodePoint())
			valid := isIdentifierChar(esc, astral)
			if first {
				valid = isIdentifierStart(esc, astral)
			}
			if !valid {
				p.invalidStringToken(escStart, "Invalid Unicode escape")
			}
			word = append(word, codePointToString(esc)...)
			chunkStart = p.pos
		} else {
			break
		}
		first = false
	}
	return jsstr.FromUTF16(append(word, p.input[chunkStart:p.pos]...))
}

// readWord reads an identifier or keyword token.
func (p *parser) readWord() {
	word := p.readWord1()
	t := tokName
	if p.keywords[word] {
		t = keywordTypes[word]
	}
	p.finishToken(t, word)
}
