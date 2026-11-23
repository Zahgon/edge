package acorn

import (
	"fmt"
	"strconv"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// noChar is what acorn reads past the end of the input: charCodeAt returns NaN
// there, which compares unequal to every character and fails every range test.
const noChar rune = -1

// Scope flags.
const (
	scopeTop = 1 << iota
	scopeFunction
	scopeAsync
	scopeGenerator
	scopeArrow
	scopeSimpleCatch
	scopeSuper
	scopeDirectSuper
	scopeSwitch = 1024
	scopeVar    = scopeTop | scopeFunction
)

func functionFlags(async, generator bool) int {
	flags := scopeFunction
	if async {
		flags |= scopeAsync
	}
	if generator {
		flags |= scopeGenerator
	}
	return flags
}

// Binding kinds for checkLVal* and declareName.
const (
	bindNone        = iota // not a binding
	bindVar                // var-style binding
	bindLexical            // let- or const-style binding
	bindFunction           // function declaration
	bindSimpleCatch        // simple (identifier pattern) catch binding
	bindOutside            // a function name as bound inside the function
)

// forInit says whether an expression is the head of a for statement, where `in` is
// not an operator, and whether that is a for-await head.
type forInit int

const (
	notForInit forInit = iota
	inForInit
	inForAwaitInit
)

// scope tracks the names declared in one lexical scope to detect redeclarations.
type scope struct {
	flags     int
	vars      []string
	lexical   []string
	functions []string
}

// label is an entry of acorn's label stack. Loops and switches push entries without
// a name; kind is "loop", "switch" or "" for a label on another statement.
type label struct {
	name           string
	kind           string
	statementStart int
}

// destructuringErrors records the positions of constructs that are errors in an
// expression but valid in a pattern (or the reverse), until the parser knows which
// of the two it has read. -1 means none.
type destructuringErrors struct {
	shorthandAssign     int
	trailingComma       int
	parenthesizedAssign int
	parenthesizedBind   int
	doubleProto         int
}

func newDestructuringErrors() *destructuringErrors {
	return &destructuringErrors{-1, -1, -1, -1, -1}
}

// parser is acorn's Parser for one input, with the tokenizer state it carries.
type parser struct {
	ecmaVersion               int
	locations                 bool
	allowAwaitOutsideFunction bool
	onToken                   func(*Token)

	keywords                wordSet
	reservedWords           wordSet
	reservedWordsStrict     wordSet
	reservedWordsStrictBind wordSet

	// input holds the UTF-16 code units of the source; every offset indexes it.
	input []uint16
	// containsEsc tells whether the last word read contained a \u escape.
	containsEsc bool

	pos       int
	curLine   int
	lineStart int

	// The current token.
	typ      *tokenType
	value    any
	start    int
	end      int
	startLoc *Position
	endLoc   *Position

	// The previous token.
	lastTokStart    int
	lastTokEnd      int
	lastTokStartLoc *Position
	lastTokEndLoc   *Position

	context     []*tokContext
	exprAllowed bool

	strict                   bool
	potentialArrowAt         int
	potentialArrowInForAwait bool
	yieldPos                 int
	awaitPos                 int
	awaitIdentPos            int
	labels                   []*label
	scopeStack               []*scope
	regexpState              *regexpState
	inTemplateElement        bool
}

// Parse parses input as a script the way acorn.parse(input, options) does and
// returns the Program node, or the *SyntaxError acorn throws.
//
// OnToken is called for every token before the parser moves past it, so tokens
// before a syntax error have been reported when Parse returns the error.
func Parse(input string, opts Options) (program *Node, err error) {
	ecmaVersion := opts.EcmaVersion
	switch {
	case ecmaVersion == 0:
		ecmaVersion = 11
	case ecmaVersion >= 2015:
		ecmaVersion -= 2009
	}
	if ecmaVersion > 11 {
		return nil, fmt.Errorf("acorn: ecmaVersion %d is newer than ECMAScript 2020, the newest grammar this parser implements", opts.EcmaVersion)
	}
	p := newParser(jsstr.ToUTF16(input), ecmaVersion, opts)
	defer func() {
		if r := recover(); r != nil {
			syntaxErr, ok := r.(*SyntaxError)
			if !ok {
				panic(r)
			}
			program, err = nil, syntaxErr
		}
	}()
	return p.parse(), nil
}

func newParser(input []uint16, ecmaVersion int, opts Options) *parser {
	p := &parser{
		ecmaVersion:               ecmaVersion,
		locations:                 opts.Locations,
		allowAwaitOutsideFunction: opts.AllowAwaitOutsideFunction,
		onToken:                   opts.OnToken,
		input:                     input,
		curLine:                   1,
		typ:                       tokEOF,
		potentialArrowAt:          -1,
	}
	if ecmaVersion >= 6 {
		p.keywords = keywords6
	} else {
		p.keywords = keywords5
	}
	reserved := reservedWordsBefore5
	switch {
	case ecmaVersion >= 6:
		reserved = reservedWords6
	case ecmaVersion == 5:
		reserved = reservedWords5
	}
	p.reservedWords, p.reservedWordsStrict, p.reservedWordsStrictBind = reserved.reserved, reserved.strict, reserved.strictBind

	p.startLoc = p.curPosition()
	p.endLoc = p.startLoc
	p.context = []*tokContext{ctxBStat}
	p.exprAllowed = true
	p.strict = p.strictDirective(p.pos)
	p.enterScope(scopeTop)
	return p
}

func (p *parser) parse() *Node {
	node := p.startNode()
	p.nextToken()
	return p.parseTopLevel(node)
}

// raise throws acorn's SyntaxError for the offset pos. Parse recovers it.
func (p *parser) raise(pos int, message string) {
	loc := getLineInfo(p.input, pos)
	panic(&SyntaxError{
		Message:  message + " (" + strconv.Itoa(loc.Line) + ":" + strconv.Itoa(loc.Column) + ")",
		Pos:      pos,
		Loc:      loc,
		RaisedAt: p.pos,
	})
}

// raiseRecoverable is raise: acorn distinguishes the two only for its loose parser.
func (p *parser) raiseRecoverable(pos int, message string) { p.raise(pos, message) }

func (p *parser) unexpected() { p.raise(p.start, "Unexpected token") }

func (p *parser) unexpectedAt(pos int) { p.raise(pos, "Unexpected token") }

func (p *parser) curPosition() *Position {
	if !p.locations {
		return nil
	}
	return &Position{Line: p.curLine, Column: p.pos - p.lineStart}
}

// getLineInfo is acorn's getLineInfo: the line and column of an offset, counting
// CRLF as one line break.
func getLineInfo(input []uint16, offset int) *Position {
	line, cur := 1, 0
	for {
		next := nextLineBreak(input, cur, offset)
		if next < 0 {
			return &Position{Line: line, Column: offset - cur}
		}
		line++
		cur = next
	}
}

// Scope tracking.

func (p *parser) enterScope(flags int) {
	p.scopeStack = append(p.scopeStack, &scope{flags: flags})
}

func (p *parser) exitScope() {
	p.scopeStack = p.scopeStack[:len(p.scopeStack)-1]
}

func (p *parser) currentScope() *scope { return p.scopeStack[len(p.scopeStack)-1] }

func (p *parser) currentVarScope() *scope {
	for i := len(p.scopeStack) - 1; ; i-- {
		if s := p.scopeStack[i]; s.flags&scopeVar != 0 {
			return s
		}
	}
}

func (p *parser) currentThisScope() *scope {
	for i := len(p.scopeStack) - 1; ; i-- {
		if s := p.scopeStack[i]; s.flags&scopeVar != 0 && s.flags&scopeArrow == 0 {
			return s
		}
	}
}

func (p *parser) inFunction() bool  { return p.currentVarScope().flags&scopeFunction != 0 }
func (p *parser) inGenerator() bool { return p.currentVarScope().flags&scopeGenerator != 0 }
func (p *parser) inAsync() bool     { return p.currentVarScope().flags&scopeAsync != 0 }

// canAwait tells whether await starts an await expression here: inside an async
// function, or at the top level when AllowAwaitOutsideFunction is set.
func (p *parser) canAwait() bool {
	for i := len(p.scopeStack) - 1; i >= 0; i-- {
		if flags := p.scopeStack[i].flags; flags&scopeFunction != 0 {
			return flags&scopeAsync != 0
		}
	}
	return p.allowAwaitOutsideFunction
}

func (p *parser) allowSuper() bool       { return p.currentThisScope().flags&scopeSuper != 0 }
func (p *parser) allowDirectSuper() bool { return p.currentThisScope().flags&scopeDirectSuper != 0 }

func (p *parser) allowNewDotTarget() bool {
	for i := len(p.scopeStack) - 1; i >= 0; i-- {
		if flags := p.scopeStack[i].flags; flags&scopeFunction != 0 && flags&scopeArrow == 0 {
			return true
		}
	}
	return false
}

// treatFunctionsAsVarInScope: at the top level of a function or script, function
// declarations are var declarations rather than lexical ones.
func treatFunctionsAsVarInScope(s *scope) bool {
	return s.flags&(scopeFunction|scopeTop) != 0
}

func (p *parser) treatFunctionsAsVar() bool { return treatFunctionsAsVarInScope(p.currentScope()) }

func (p *parser) declareName(name string, bindingType int, pos int) {
	redeclared := false
	switch bindingType {
	case bindLexical:
		s := p.currentScope()
		redeclared = contains(s.lexical, name) || contains(s.functions, name) || contains(s.vars, name)
		s.lexical = append(s.lexical, name)
	case bindSimpleCatch:
		s := p.currentScope()
		s.lexical = append(s.lexical, name)
	case bindFunction:
		s := p.currentScope()
		if p.treatFunctionsAsVar() {
			redeclared = contains(s.lexical, name)
		} else {
			redeclared = contains(s.lexical, name) || contains(s.vars, name)
		}
		s.functions = append(s.functions, name)
	default:
		for i := len(p.scopeStack) - 1; i >= 0; i-- {
			s := p.scopeStack[i]
			if contains(s.lexical, name) && !(s.flags&scopeSimpleCatch != 0 && s.lexical[0] == name) ||
				!treatFunctionsAsVarInScope(s) && contains(s.functions, name) {
				redeclared = true
				break
			}
			s.vars = append(s.vars, name)
			if s.flags&scopeVar != 0 {
				break
			}
		}
	}
	if redeclared {
		p.raiseRecoverable(pos, "Identifier '"+name+"' has already been declared")
	}
}

func contains(list []string, name string) bool {
	for _, s := range list {
		if s == name {
			return true
		}
	}
	return false
}

// Nodes.

func (p *parser) startNode() *Node { return p.startNodeAt(p.start, p.startLoc) }

func (p *parser) startNodeAt(pos int, loc *Position) *Node {
	node := &Node{Start: pos}
	if p.locations {
		node.Loc = &SourceLocation{Start: loc}
	}
	return node
}

func (p *parser) finishNode(node *Node, typ string) *Node {
	return p.finishNodeAt(node, typ, p.lastTokEnd, p.lastTokEndLoc)
}

func (p *parser) finishNodeAt(node *Node, typ string, pos int, loc *Position) *Node {
	node.Type = typ
	node.End = pos
	if p.locations {
		node.Loc.End = loc
	}
	return node
}

// copyNode copies every property of node, its location object included, as acorn
// does for the value of a shorthand property.
func (p *parser) copyNode(node *Node) *Node {
	c := *node
	return &c
}

// Parser utilities.

func (p *parser) eat(t *tokenType) bool {
	if p.typ == t {
		p.next(false)
		return true
	}
	return false
}

func (p *parser) isContextual(name string) bool {
	return p.typ == tokName && p.value == name && !p.containsEsc
}

func (p *parser) eatContextual(name string) bool {
	if !p.isContextual(name) {
		return false
	}
	p.next(false)
	return true
}

func (p *parser) canInsertSemicolon() bool {
	return p.typ == tokEOF || p.typ == tokBraceR || hasLineBreak(p.input[p.lastTokEnd:p.start])
}

func (p *parser) insertSemicolon() bool { return p.canInsertSemicolon() }

func (p *parser) semicolon() {
	if !p.eat(tokSemi) && !p.insertSemicolon() {
		p.unexpected()
	}
}

func (p *parser) afterTrailingComma(t *tokenType, notNext bool) bool {
	if p.typ == t {
		if !notNext {
			p.next(false)
		}
		return true
	}
	return false
}

func (p *parser) expect(t *tokenType) {
	if !p.eat(t) {
		p.unexpected()
	}
}

// checkPatternErrors raises the errors refs recorded that make an expression an
// invalid pattern. refs may be nil when there is nothing to check.
func (p *parser) checkPatternErrors(refs *destructuringErrors, isAssign bool) {
	if refs == nil {
		return
	}
	if refs.trailingComma > -1 {
		p.raiseRecoverable(refs.trailingComma, "Comma is not permitted after the rest element")
	}
	parens := refs.parenthesizedBind
	if isAssign {
		parens = refs.parenthesizedAssign
	}
	if parens > -1 {
		if isAssign {
			p.raiseRecoverable(parens, "Assigning to rvalue")
		}
		p.raiseRecoverable(parens, "Parenthesized pattern")
	}
}

func (p *parser) checkExpressionErrors(refs *destructuringErrors, andThrow bool) bool {
	if refs == nil {
		return false
	}
	if !andThrow {
		return refs.shorthandAssign >= 0 || refs.doubleProto >= 0
	}
	if refs.shorthandAssign >= 0 {
		p.raise(refs.shorthandAssign, "Shorthand property assignments are valid only in destructuring patterns")
	}
	if refs.doubleProto >= 0 {
		p.raiseRecoverable(refs.doubleProto, "Redefinition of __proto__ property")
	}
	return false
}

func (p *parser) checkYieldAwaitInDefaultParams() {
	if p.yieldPos != 0 && (p.awaitPos == 0 || p.yieldPos < p.awaitPos) {
		p.raise(p.yieldPos, "Yield expression cannot be a default value")
	}
	if p.awaitPos != 0 {
		p.raise(p.awaitPos, "Await expression cannot be a default value")
	}
}

func isSimpleAssignTarget(expr *Node) bool {
	return expr.Type == "Identifier" || expr.Type == "MemberExpression"
}
