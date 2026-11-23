package acorn

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// Statement contexts passed to parseStatement: "" for a statement list, otherwise
// the construct whose single body statement is parsed ("if", "label", "do",
// "while", "for", "with"), with "label" appended for labeled bodies.
const noContext = ""

// Function statement flags.
const (
	funcStatement        = 1
	funcHangingStatement = 2
)

var (
	loopLabel   = &label{kind: "loop", statementStart: -1}
	switchLabel = &label{kind: "switch", statementStart: -1}
)

// parseTopLevel reads statements up to the end of the input into the Program node.
func (p *parser) parseTopLevel(node *Node) *Node {
	node.BodyList = []*Node{}
	for p.typ != tokEOF {
		node.BodyList = append(node.BodyList, p.parseStatement(noContext, true))
	}
	p.adaptDirectivePrologue(node.BodyList)
	p.next(false)
	node.SourceType = "script"
	return p.finishNode(node, "Program")
}

// strictDirective tells whether the directive prologue starting at start holds a
// "use strict" directive, looking at the source text ahead of the tokenizer.
func (p *parser) strictDirective(start int) bool {
	if p.ecmaVersion < 5 {
		return false
	}
	for {
		// Try to find a string literal.
		start = skipWhiteSpace(p.input, start)
		content, end, ok := p.stringLiteralAt(start)
		if !ok {
			return false
		}
		if content == "use strict" {
			spaceEnd := skipWhiteSpace(p.input, end)
			next := p.charCodeAt(spaceEnd)
			return next == ';' || next == '}' ||
				hasLineBreak(p.input[end:spaceEnd]) &&
					!(isStrictDirectiveContinuation(next) ||
						next == '!' && p.charCodeAt(spaceEnd+1) == '=' ||
						next == 'i' && p.keywordOpAt(spaceEnd))
		}
		start = skipWhiteSpace(p.input, end)
		if p.charCodeAt(start) == ';' {
			start++
		}
	}
}

// isStrictDirectiveContinuation is acorn's /[(`.[+\-/*%<>=,?^&]/: characters that
// continue an expression after a string literal on the next line.
func isStrictDirectiveContinuation(c rune) bool {
	switch c {
	case '(', '`', '.', '[', '+', '-', '/', '*', '%', '<', '>', '=', ',', '?', '^', '&':
		return true
	}
	return false
}

// stringLiteralAt matches acorn's /^(?:'((?:\\[^]|[^'\\])*?)'|"((?:\\[^]|[^"\\])*?)")/
// at start: a quoted string, escapes left as written. It returns the text between
// the quotes and the offset after the closing quote.
func (p *parser) stringLiteralAt(start int) (string, int, bool) {
	quote := p.charCodeAt(start)
	if quote != '\'' && quote != '"' {
		return "", 0, false
	}
	for i := start + 1; i < len(p.input); i++ {
		switch rune(p.input[i]) {
		case '\\':
			i++
		case quote:
			return p.slice(start+1, i), i + 1, true
		}
	}
	return "", 0, false
}

// keywordOpAt scans for in or instanceof at pos, knowing there is an i there.
func (p *parser) keywordOpAt(pos int) bool {
	end, stop := pos+1, min(len(p.input), pos+11)
	for end < stop {
		ch := p.fullCharCodeAt(end)
		if !isIdentifierChar(ch, true) {
			break
		}
		if ch <= 0xffff {
			end++
		} else {
			end += 2
		}
	}
	return end == pos+2 && p.slice(pos, end) == "in" || end == pos+10 && p.slice(pos, end) == "instanceof"
}

func (p *parser) isLet(context string) bool {
	if p.ecmaVersion < 6 || !p.isContextual("let") {
		return false
	}
	next := skipWhiteSpace(p.input, p.pos)
	nextCh := p.fullCharCodeAt(next)
	// For ambiguous cases, determine if a LexicalDeclaration (or only a Statement)
	// is allowed here. If context is not empty then only a Statement is allowed.
	// However, `let [` is an explicit negative lookahead for ExpressionStatement,
	// so special-case it first.
	if nextCh == '[' || nextCh == '\\' {
		return true
	}
	if context != noContext {
		return false
	}
	if nextCh == '{' {
		return true
	}
	if isIdentifierStart(nextCh, true) {
		start := next
		for {
			if nextCh <= 0xffff {
				next++
			} else {
				next += 2
			}
			nextCh = p.fullCharCodeAt(next)
			if !isIdentifierChar(nextCh, true) {
				break
			}
		}
		if nextCh == '\\' {
			return true
		}
		if ident := p.slice(start, next); ident != "in" && ident != "instanceof" {
			return true
		}
	}
	return false
}

// isAsyncFunction checks for async [no LineTerminator here] function.
func (p *parser) isAsyncFunction() bool {
	if p.ecmaVersion < 8 || !p.isContextual("async") {
		return false
	}
	next := skipWhiteSpace(p.input, p.pos)
	if hasLineBreak(p.input[p.pos:next]) || next+8 > len(p.input) || p.slice(next, next+8) != "function" {
		return false
	}
	if next+8 == len(p.input) {
		return true
	}
	after := p.fullCharCodeAt(next + 8)
	return !(isIdentifierChar(after, true) || after == '\\')
}

// parseStatement parses a single statement.
//
// If expecting a statement and finding a slash operator, parse a regular
// expression literal. This is to handle cases like `if (foo) /blah/.exec(foo)`,
// where looking at the previous token does not help.
func (p *parser) parseStatement(context string, topLevel bool) *Node {
	starttype, node := p.typ, p.startNode()
	kind := ""
	if p.isLet(context) {
		starttype = tokVar
		kind = "let"
	}

	// Most types of statements are recognized by the keyword they start with.
	switch starttype {
	case tokBreak, tokContinue:
		return p.parseBreakContinueStatement(node, starttype.keyword)
	case tokDebugger:
		return p.parseDebuggerStatement(node)
	case tokDo:
		return p.parseDoStatement(node)
	case tokFor:
		return p.parseForStatement(node)
	case tokFunction:
		// A function as the sole body of an if statement or a labeled statement
		// works in sloppy mode, but not when the labeled statement is itself the
		// sole body of an if statement.
		if context != noContext && (p.strict || context != "if" && context != "label") && p.ecmaVersion >= 6 {
			p.unexpected()
		}
		return p.parseFunctionStatement(node, false, context == noContext)
	case tokClass:
		if context != noContext {
			p.unexpected()
		}
		return p.parseClass(node, true)
	case tokIf:
		return p.parseIfStatement(node)
	case tokReturn:
		return p.parseReturnStatement(node)
	case tokSwitch:
		return p.parseSwitchStatement(node)
	case tokThrow:
		return p.parseThrowStatement(node)
	case tokTry:
		return p.parseTryStatement(node)
	case tokConst, tokVar:
		if kind == "" {
			kind = p.value.(string)
		}
		if context != noContext && kind != "var" {
			p.unexpected()
		}
		return p.parseVarStatement(node, kind)
	case tokWhile:
		return p.parseWhileStatement(node)
	case tokWith:
		return p.parseWithStatement(node)
	case tokBraceL:
		return p.parseBlock(true, node, false)
	case tokSemi:
		return p.parseEmptyStatement(node)
	case tokExport, tokImport:
		if p.ecmaVersion > 10 && starttype == tokImport {
			next := skipWhiteSpace(p.input, p.pos)
			if nextCh := p.charCodeAt(next); nextCh == '(' || nextCh == '.' {
				return p.parseExpressionStatement(node, p.parseExpression(notForInit, nil))
			}
		}
		if !topLevel {
			p.raise(p.start, "'import' and 'export' may only appear at the top level")
		}
		p.raise(p.start, "'import' and 'export' may appear only with 'sourceType: module'")
	}

	// If the statement does not start with a statement keyword or a brace, it is
	// an ExpressionStatement or LabeledStatement. We simply start parsing an
	// expression, and afterwards, if the next token is a colon and the
	// expression was a simple Identifier node, we switch to interpreting it as a
	// label.
	if p.isAsyncFunction() {
		if context != noContext {
			p.unexpected()
		}
		p.next(false)
		return p.parseFunctionStatement(node, true, context == noContext)
	}
	maybeName := p.value
	expr := p.parseExpression(notForInit, nil)
	if starttype == tokName && expr.Type == "Identifier" && p.eat(tokColon) {
		return p.parseLabeledStatement(node, maybeName.(string), expr, context)
	}
	return p.parseExpressionStatement(node, expr)
}

func (p *parser) parseBreakContinueStatement(node *Node, keyword string) *Node {
	isBreak := keyword == "break"
	p.next(false)
	if p.eat(tokSemi) || p.insertSemicolon() {
		node.Label = nil
	} else if p.typ != tokName {
		p.unexpected()
	} else {
		node.Label = p.parseIdent(false)
		p.semicolon()
	}

	// Verify that there is an actual destination to break or continue to.
	i := 0
	for ; i < len(p.labels); i++ {
		lab := p.labels[i]
		if node.Label == nil || lab.name == node.Label.Name {
			if lab.kind != "" && (isBreak || lab.kind == "loop") {
				break
			}
			if node.Label != nil && isBreak {
				break
			}
		}
	}
	if i == len(p.labels) {
		p.raise(node.Start, "Unsyntactic "+keyword)
	}
	if isBreak {
		return p.finishNode(node, "BreakStatement")
	}
	return p.finishNode(node, "ContinueStatement")
}

func (p *parser) parseDebuggerStatement(node *Node) *Node {
	p.next(false)
	p.semicolon()
	return p.finishNode(node, "DebuggerStatement")
}

func (p *parser) parseDoStatement(node *Node) *Node {
	p.next(false)
	p.labels = append(p.labels, loopLabel)
	node.Body = p.parseStatement("do", false)
	p.labels = p.labels[:len(p.labels)-1]
	p.expect(tokWhile)
	node.Test = p.parseParenExpression()
	if p.ecmaVersion >= 6 {
		p.eat(tokSemi)
	} else {
		p.semicolon()
	}
	return p.finishNode(node, "DoWhileStatement")
}

// parseForStatement disambiguates between a for loop and a for-in or for-of loop:
// it parses the init declaration or expression with the in operator disallowed and
// then checks whether the next token is in or of. With no init part (a semicolon
// right after the opening parenthesis) it is a regular for loop.
func (p *parser) parseForStatement(node *Node) *Node {
	p.next(false)
	awaitAt := -1
	if p.ecmaVersion >= 9 && p.canAwait() && p.eatContextual("await") {
		awaitAt = p.lastTokStart
	}
	p.labels = append(p.labels, loopLabel)
	p.enterScope(0)
	p.expect(tokParenL)
	if p.typ == tokSemi {
		if awaitAt > -1 {
			p.unexpectedAt(awaitAt)
		}
		return p.parseFor(node, nil)
	}
	isLet := p.isLet(noContext)
	if p.typ == tokVar || p.typ == tokConst || isLet {
		init := p.startNode()
		kind := "let"
		if !isLet {
			kind = p.value.(string)
		}
		p.next(false)
		p.parseVar(init, true, kind)
		p.finishNode(init, "VariableDeclaration")
		return p.parseForAfterInit(node, init, awaitAt)
	}
	startsWithLet := p.isContextual("let")
	containsEsc := p.containsEsc
	refs := newDestructuringErrors()
	initPos := p.start
	var init *Node
	if awaitAt > -1 {
		init = p.parseExprSubscripts(refs, inForAwaitInit)
	} else {
		init = p.parseExpression(inForInit, refs)
	}
	isForOf := p.typ != tokIn && p.ecmaVersion >= 6 && p.isContextual("of")
	if p.typ == tokIn || isForOf {
		if awaitAt > -1 {
			if p.typ == tokIn {
				p.unexpectedAt(awaitAt)
			}
			node.Await = true
		} else if isForOf && p.ecmaVersion >= 8 {
			if init.Start == initPos && !containsEsc && init.Type == "Identifier" && init.Name == "async" {
				p.unexpected()
			}
		}
		if startsWithLet && isForOf {
			p.raise(init.Start, "The left-hand side of a for-of loop may not start with 'let'.")
		}
		p.toAssignable(init, false, refs)
		p.checkLValPattern(init, bindNone, nil)
		return p.parseForIn(node, init)
	}
	p.checkExpressionErrors(refs, true)
	if awaitAt > -1 {
		p.unexpectedAt(awaitAt)
	}
	return p.parseFor(node, init)
}

// parseForAfterInit continues a for statement after a declaration in its head.
func (p *parser) parseForAfterInit(node, init *Node, awaitAt int) *Node {
	if (p.typ == tokIn || p.ecmaVersion >= 6 && p.isContextual("of")) && len(init.Declarations) == 1 {
		if p.typ == tokIn {
			if p.ecmaVersion >= 9 && awaitAt > -1 {
				p.unexpectedAt(awaitAt)
			}
		} else if p.ecmaVersion >= 9 {
			node.Await = awaitAt > -1
		}
		return p.parseForIn(node, init)
	}
	if awaitAt > -1 {
		p.unexpectedAt(awaitAt)
	}
	return p.parseFor(node, init)
}

func (p *parser) parseFunctionStatement(node *Node, isAsync, declarationPosition bool) *Node {
	p.next(false)
	statement := funcStatement
	if !declarationPosition {
		statement |= funcHangingStatement
	}
	return p.parseFunction(node, statement, isAsync, notForInit)
}

func (p *parser) parseIfStatement(node *Node) *Node {
	p.next(false)
	node.Test = p.parseParenExpression()
	// Function declarations are allowed in the branches in sloppy mode only.
	node.Consequent = p.parseStatement("if", false)
	if p.eat(tokElse) {
		node.Alternate = p.parseStatement("if", false)
	}
	return p.finishNode(node, "IfStatement")
}

func (p *parser) parseReturnStatement(node *Node) *Node {
	if !p.inFunction() {
		p.raise(p.start, "'return' outside of function")
	}
	p.next(false)

	// In return (and break/continue), the keywords with optional arguments, we
	// eagerly look for a semicolon or the possibility to insert one.
	if !p.eat(tokSemi) && !p.insertSemicolon() {
		node.Argument = p.parseExpression(notForInit, nil)
		p.semicolon()
	}
	return p.finishNode(node, "ReturnStatement")
}

func (p *parser) parseSwitchStatement(node *Node) *Node {
	p.next(false)
	node.Discriminant = p.parseParenExpression()
	node.Cases = []*Node{}
	p.expect(tokBraceL)
	p.labels = append(p.labels, switchLabel)
	p.enterScope(scopeSwitch)

	// Statements under must be grouped (by label) in SwitchCase nodes. cur is the
	// node that statements are currently added to.
	var cur *Node
	for sawDefault := false; p.typ != tokBraceR; {
		if p.typ == tokCase || p.typ == tokDefault {
			isCase := p.typ == tokCase
			if cur != nil {
				p.finishNode(cur, "SwitchCase")
			}
			cur = p.startNode()
			node.Cases = append(node.Cases, cur)
			cur.ConsequentList = []*Node{}
			p.next(false)
			if isCase {
				cur.Test = p.parseExpression(notForInit, nil)
			} else {
				if sawDefault {
					p.raiseRecoverable(p.lastTokStart, "Multiple default clauses")
				}
				sawDefault = true
			}
			p.expect(tokColon)
		} else {
			if cur == nil {
				p.unexpected()
			}
			cur.ConsequentList = append(cur.ConsequentList, p.parseStatement(noContext, false))
		}
	}
	p.exitScope()
	if cur != nil {
		p.finishNode(cur, "SwitchCase")
	}
	p.next(false) // closing brace
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(node, "SwitchStatement")
}

func (p *parser) parseThrowStatement(node *Node) *Node {
	p.next(false)
	if hasLineBreak(p.input[p.lastTokEnd:p.start]) {
		p.raise(p.lastTokEnd, "Illegal newline after throw")
	}
	node.Argument = p.parseExpression(notForInit, nil)
	p.semicolon()
	return p.finishNode(node, "ThrowStatement")
}

func (p *parser) parseCatchClauseParam() *Node {
	param := p.parseBindingAtom()
	simple := param.Type == "Identifier"
	if simple {
		p.enterScope(scopeSimpleCatch)
		p.checkLValPattern(param, bindSimpleCatch, nil)
	} else {
		p.enterScope(0)
		p.checkLValPattern(param, bindLexical, nil)
	}
	p.expect(tokParenR)
	return param
}

func (p *parser) parseTryStatement(node *Node) *Node {
	p.next(false)
	node.Block = p.parseBlock(true, nil, false)
	if p.typ == tokCatch {
		clause := p.startNode()
		p.next(false)
		if p.eat(tokParenL) {
			clause.Param = p.parseCatchClauseParam()
		} else {
			if p.ecmaVersion < 10 {
				p.unexpected()
			}
			p.enterScope(0)
		}
		clause.Body = p.parseBlock(false, nil, false)
		p.exitScope()
		node.Handler = p.finishNode(clause, "CatchClause")
	}
	if p.eat(tokFinally) {
		node.Finalizer = p.parseBlock(true, nil, false)
	}
	if node.Handler == nil && node.Finalizer == nil {
		p.raise(node.Start, "Missing catch or finally clause")
	}
	return p.finishNode(node, "TryStatement")
}

func (p *parser) parseVarStatement(node *Node, kind string) *Node {
	p.next(false)
	p.parseVar(node, false, kind)
	p.semicolon()
	return p.finishNode(node, "VariableDeclaration")
}

func (p *parser) parseWhileStatement(node *Node) *Node {
	p.next(false)
	node.Test = p.parseParenExpression()
	p.labels = append(p.labels, loopLabel)
	node.Body = p.parseStatement("while", false)
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(node, "WhileStatement")
}

func (p *parser) parseWithStatement(node *Node) *Node {
	if p.strict {
		p.raise(p.start, "'with' in strict mode")
	}
	p.next(false)
	node.Object = p.parseParenExpression()
	node.Body = p.parseStatement("with", false)
	return p.finishNode(node, "WithStatement")
}

func (p *parser) parseEmptyStatement(node *Node) *Node {
	p.next(false)
	return p.finishNode(node, "EmptyStatement")
}

func (p *parser) parseLabeledStatement(node *Node, maybeName string, expr *Node, context string) *Node {
	for _, l := range p.labels {
		if l.name == maybeName {
			p.raise(expr.Start, "Label '"+maybeName+"' is already declared")
		}
	}
	kind := ""
	if p.typ.isLoop {
		kind = "loop"
	} else if p.typ == tokSwitch {
		kind = "switch"
	}
	for i := len(p.labels) - 1; i >= 0; i-- {
		l := p.labels[i]
		if l.statementStart != node.Start {
			break
		}
		// Update information about previous labels on this node.
		l.statementStart = p.start
		l.kind = kind
	}
	p.labels = append(p.labels, &label{name: maybeName, kind: kind, statementStart: p.start})
	bodyContext := "label"
	if context != noContext {
		bodyContext = context
		if !strings.Contains(context, "label") {
			bodyContext = context + "label"
		}
	}
	node.Body = p.parseStatement(bodyContext, false)
	p.labels = p.labels[:len(p.labels)-1]
	node.Label = expr
	return p.finishNode(node, "LabeledStatement")
}

func (p *parser) parseExpressionStatement(node, expr *Node) *Node {
	node.Expression = expr
	p.semicolon()
	return p.finishNode(node, "ExpressionStatement")
}

// parseBlock parses a braced list of statements. exitStrict ends the strict mode a
// "use strict" function body started, before the closing brace is read.
func (p *parser) parseBlock(createNewLexicalScope bool, node *Node, exitStrict bool) *Node {
	if node == nil {
		node = p.startNode()
	}
	node.BodyList = []*Node{}
	p.expect(tokBraceL)
	if createNewLexicalScope {
		p.enterScope(0)
	}
	for p.typ != tokBraceR {
		node.BodyList = append(node.BodyList, p.parseStatement(noContext, false))
	}
	if exitStrict {
		p.strict = false
	}
	p.next(false)
	if createNewLexicalScope {
		p.exitScope()
	}
	return p.finishNode(node, "BlockStatement")
}

// parseFor parses a regular for loop after its init part.
func (p *parser) parseFor(node, init *Node) *Node {
	node.Init = init
	p.expect(tokSemi)
	if p.typ != tokSemi {
		node.Test = p.parseExpression(notForInit, nil)
	}
	p.expect(tokSemi)
	if p.typ != tokParenR {
		node.Update = p.parseExpression(notForInit, nil)
	}
	p.expect(tokParenR)
	node.Body = p.parseStatement("for", false)
	p.exitScope()
	p.labels = p.labels[:len(p.labels)-1]
	return p.finishNode(node, "ForStatement")
}

// parseForIn parses for-in and for-of loops, which are almost the same from the
// parser's perspective.
func (p *parser) parseForIn(node, init *Node) *Node {
	isForIn := p.typ == tokIn
	p.next(false)

	if init.Type == "VariableDeclaration" && init.Declarations[0].Init != nil &&
		(!isForIn || p.ecmaVersion < 8 || p.strict || init.Kind != "var" || init.Declarations[0].Id.Type != "Identifier") {
		loop := "for-of"
		if isForIn {
			loop = "for-in"
		}
		p.raise(init.Start, loop+" loop variable declaration may not have an initializer")
	}
	node.Left = init
	if isForIn {
		node.Right = p.parseExpression(notForInit, nil)
	} else {
		node.Right = p.parseMaybeAssign(notForInit, nil)
	}
	p.expect(tokParenR)
	node.Body = p.parseStatement("for", false)
	p.exitScope()
	p.labels = p.labels[:len(p.labels)-1]
	if isForIn {
		return p.finishNode(node, "ForInStatement")
	}
	return p.finishNode(node, "ForOfStatement")
}

// parseVar parses a list of variable declarations.
func (p *parser) parseVar(node *Node, isFor bool, kind string) *Node {
	node.Declarations = []*Node{}
	node.Kind = kind
	for {
		decl := p.startNode()
		p.parseVarID(decl, kind)
		if p.eat(tokEq) {
			if isFor {
				decl.Init = p.parseMaybeAssign(inForInit, nil)
			} else {
				decl.Init = p.parseMaybeAssign(notForInit, nil)
			}
		} else if kind == "const" && !(p.typ == tokIn || p.ecmaVersion >= 6 && p.isContextual("of")) {
			p.unexpected()
		} else if decl.Id.Type != "Identifier" && !(isFor && (p.typ == tokIn || p.isContextual("of"))) {
			p.raise(p.lastTokEnd, "Complex binding patterns require an initialization value")
		}
		node.Declarations = append(node.Declarations, p.finishNode(decl, "VariableDeclarator"))
		if !p.eat(tokComma) {
			break
		}
	}
	return node
}

func (p *parser) parseVarID(decl *Node, kind string) {
	decl.Id = p.parseBindingAtom()
	if kind == "var" {
		p.checkLValPattern(decl.Id, bindVar, nil)
	} else {
		p.checkLValPattern(decl.Id, bindLexical, nil)
	}
}

// parseFunction parses a function declaration (statement has funcStatement) or
// expression.
func (p *parser) parseFunction(node *Node, statement int, isAsync bool, fi forInit) *Node {
	p.initFunction(node)
	if p.ecmaVersion >= 9 || p.ecmaVersion >= 6 && !isAsync {
		if p.typ == tokStar && statement&funcHangingStatement != 0 {
			p.unexpected()
		}
		node.Generator = p.eat(tokStar)
	}
	if p.ecmaVersion >= 8 {
		node.Async = isAsync
	}

	if statement&funcStatement != 0 {
		node.Id = p.parseIdent(false)
		if statement&funcHangingStatement == 0 {
			// A regular function declaration in sloppy mode follows the Annex B
			// semantics (bindFunction); otherwise the binding depends on the
			// current scope.
			binding := bindFunction
			if p.strict || node.Generator || node.Async {
				binding = bindLexical
				if p.treatFunctionsAsVar() {
					binding = bindVar
				}
			}
			p.checkLValSimple(node.Id, binding, nil)
		}
	}

	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
	p.yieldPos, p.awaitPos, p.awaitIdentPos = 0, 0, 0
	p.enterScope(functionFlags(node.Async, node.Generator))

	if statement&funcStatement == 0 && p.typ == tokName {
		node.Id = p.parseIdent(false)
	}

	p.parseFunctionParams(node)
	p.parseFunctionBody(node, false, false, fi)

	p.yieldPos, p.awaitPos, p.awaitIdentPos = oldYieldPos, oldAwaitPos, oldAwaitIdentPos
	if statement&funcStatement != 0 {
		return p.finishNode(node, "FunctionDeclaration")
	}
	return p.finishNode(node, "FunctionExpression")
}

func (p *parser) parseFunctionParams(node *Node) {
	p.expect(tokParenL)
	node.Params = p.parseBindingList(tokParenR, false, p.ecmaVersion >= 8)
	p.checkYieldAwaitInDefaultParams()
}

// parseClass parses a class declaration or expression. A class body is always
// strict mode code.
func (p *parser) parseClass(node *Node, isStatement bool) *Node {
	p.next(false)
	oldStrict := p.strict
	p.strict = true

	p.parseClassID(node, isStatement)
	p.parseClassSuper(node)
	classBody := p.startNode()
	hadConstructor := false
	classBody.BodyList = []*Node{}
	p.expect(tokBraceL)
	for p.typ != tokBraceR {
		element := p.parseClassElement(node.SuperClass != nil)
		if element != nil {
			classBody.BodyList = append(classBody.BodyList, element)
			if element.Kind == "constructor" {
				if hadConstructor {
					p.raiseRecoverable(element.Start, "Duplicate constructor in the same class")
				}
				hadConstructor = true
			}
		}
	}
	p.strict = oldStrict
	p.next(false)
	node.Body = p.finishNode(classBody, "ClassBody")
	if isStatement {
		return p.finishNode(node, "ClassDeclaration")
	}
	return p.finishNode(node, "ClassExpression")
}

func (p *parser) parseClassElement(constructorAllowsSuper bool) *Node {
	if p.eat(tokSemi) {
		return nil
	}

	node := p.startNode()
	keyName := ""
	isGenerator, isAsync, isStatic := false, false, false
	kind := "method"

	if p.eatContextual("static") {
		if p.isClassElementNameStart() || p.typ == tokStar {
			isStatic = true
		} else {
			keyName = "static"
		}
	}
	node.Static = isStatic
	if keyName == "" && p.ecmaVersion >= 8 && p.eatContextual("async") {
		if (p.isClassElementNameStart() || p.typ == tokStar) && !p.canInsertSemicolon() {
			isAsync = true
		} else {
			keyName = "async"
		}
	}
	if keyName == "" && (p.ecmaVersion >= 9 || !isAsync) && p.eat(tokStar) {
		isGenerator = true
	}
	if keyName == "" && !isAsync && !isGenerator {
		lastValue := p.value
		if p.eatContextual("get") || p.eatContextual("set") {
			if p.isClassElementNameStart() {
				kind = lastValue.(string)
			} else {
				keyName = lastValue.(string)
			}
		}
	}

	// The element name. When async, get, set or static was not a modifier, the
	// last token is the name.
	if keyName != "" {
		node.Computed = false
		node.Key = p.startNodeAt(p.lastTokStart, p.lastTokStartLoc)
		node.Key.Name = keyName
		p.finishNode(node.Key, "Identifier")
	} else {
		p.parsePropertyName(node)
	}

	// The element value: before ECMAScript 2022 every element is a method.
	isConstructor := !node.Static && checkKeyName(node, "constructor")
	allowsDirectSuper := isConstructor && constructorAllowsSuper
	if isConstructor && kind != "method" {
		p.raise(node.Key.Start, "Constructor can't have get/set modifier")
	}
	if isConstructor {
		node.Kind = "constructor"
	} else {
		node.Kind = kind
	}
	return p.parseClassMethod(node, isGenerator, isAsync, allowsDirectSuper)
}

func (p *parser) isClassElementNameStart() bool {
	return p.typ == tokName || p.typ == tokNum || p.typ == tokString || p.typ == tokBracketL || p.typ.keyword != ""
}

func (p *parser) parseClassMethod(method *Node, isGenerator, isAsync, allowsDirectSuper bool) *Node {
	key := method.Key
	if method.Kind == "constructor" {
		if isGenerator {
			p.raise(key.Start, "Constructor can't be a generator")
		}
		if isAsync {
			p.raise(key.Start, "Constructor can't be an async method")
		}
	} else if method.Static && checkKeyName(method, "prototype") {
		p.raise(key.Start, "Classes may not have a static property named prototype")
	}

	value := p.parseMethod(isGenerator, isAsync, allowsDirectSuper)
	method.Value = value

	if method.Kind == "get" && len(value.Params) != 0 {
		p.raiseRecoverable(value.Start, "getter should have no params")
	}
	if method.Kind == "set" && len(value.Params) != 1 {
		p.raiseRecoverable(value.Start, "setter should have exactly one param")
	}
	if method.Kind == "set" && value.Params[0].Type == "RestElement" {
		p.raiseRecoverable(value.Params[0].Start, "Setter cannot use rest params")
	}
	return p.finishNode(method, "MethodDefinition")
}

func (p *parser) parseClassID(node *Node, isStatement bool) {
	if p.typ == tokName {
		node.Id = p.parseIdent(false)
		if isStatement {
			p.checkLValSimple(node.Id, bindLexical, nil)
		}
	} else if isStatement {
		p.unexpected()
	}
}

func (p *parser) parseClassSuper(node *Node) {
	if p.eat(tokExtends) {
		node.SuperClass = p.parseExprSubscripts(nil, notForInit)
	}
}

// checkKeyName tells whether a property or class element has the plain name name.
func checkKeyName(node *Node, name string) bool {
	if node.Computed {
		return false
	}
	key := node.Key
	return key.Type == "Identifier" && key.Name == name || key.Type == "Literal" && key.LiteralValue == name
}

// adaptDirectivePrologue sets the directive of the leading string statements.
func (p *parser) adaptDirectivePrologue(statements []*Node) {
	for _, s := range statements {
		if !p.isDirectiveCandidate(s) {
			break
		}
		raw := jsstr.ToUTF16(s.Expression.Raw)
		directive := jsstr.FromUTF16(raw[1 : len(raw)-1])
		s.Directive = &directive
	}
}

func (p *parser) isDirectiveCandidate(statement *Node) bool {
	if p.ecmaVersion < 5 || statement.Type != "ExpressionStatement" || statement.Expression.Type != "Literal" {
		return false
	}
	if _, isString := statement.Expression.LiteralValue.(string); !isString {
		return false
	}
	// Parenthesized strings are not directives.
	first := p.charCodeAt(statement.Start)
	return first == '"' || first == '\''
}
