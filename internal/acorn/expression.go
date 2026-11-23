package acorn

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/js"
)

// Expressions nest from the most general expression type at the top to atomic,
// nondivisible expression types at the bottom. Binary operators use an operator
// precedence parser.

// propHash records the property names of an object literal already seen, to detect
// clashes.
type propHash struct {
	proto bool
	// kinds maps a name to the kinds (init, get, set) defined for it; it is used
	// before ECMAScript 2015 only.
	kinds map[string]map[string]bool
}

// literalKeyString is String(key.value) for a numeric or string property key.
func literalKeyString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case float64:
		return js.NumberToString(v)
	case js.BigInt:
		return v.String()
	}
	panic("acorn: a property key literal is a string, a number or a BigInt")
}

// checkPropClash checks a property name against the ones already in the object:
// getters and setters may not clash with each other or with an init property, and
// in strict mode init properties may not be repeated. From ECMAScript 2015 only a
// repeated __proto__ is checked.
func (p *parser) checkPropClash(prop *Node, hash *propHash, refs *destructuringErrors) {
	if p.ecmaVersion >= 9 && prop.Type == "SpreadElement" {
		return
	}
	if p.ecmaVersion >= 6 && (prop.Computed || prop.Method || prop.Shorthand) {
		return
	}
	key := prop.Key
	var name string
	switch key.Type {
	case "Identifier":
		name = key.Name
	case "Literal":
		name = literalKeyString(key.LiteralValue)
	default:
		return
	}
	kind := prop.Kind
	if p.ecmaVersion >= 6 {
		if name == "__proto__" && kind == "init" {
			if hash.proto {
				if refs != nil {
					if refs.doubleProto < 0 {
						refs.doubleProto = key.Start
					}
				} else {
					p.raiseRecoverable(key.Start, "Redefinition of __proto__ property")
				}
			}
			hash.proto = true
		}
		return
	}
	if hash.kinds == nil {
		hash.kinds = map[string]map[string]bool{}
	}
	other := hash.kinds[name]
	if other != nil {
		var redefinition bool
		if kind == "init" {
			redefinition = p.strict && other["init"] || other["get"] || other["set"]
		} else {
			redefinition = other["init"] || other[kind]
		}
		if redefinition {
			p.raiseRecoverable(key.Start, "Redefinition of property")
		}
	} else {
		other = map[string]bool{}
		hash.kinds[name] = other
	}
	other[kind] = true
}

// parseExpression parses a full expression, a sequence when it has commas. fi
// forbids the in operator in the head of a for statement; refs collects errors that
// depend on whether the expression turns out to be a pattern.
func (p *parser) parseExpression(fi forInit, refs *destructuringErrors) *Node {
	startPos, startLoc := p.start, p.startLoc
	expr := p.parseMaybeAssign(fi, refs)
	if p.typ == tokComma {
		node := p.startNodeAt(startPos, startLoc)
		node.Expressions = []*Node{expr}
		for p.eat(tokComma) {
			node.Expressions = append(node.Expressions, p.parseMaybeAssign(fi, refs))
		}
		return p.finishNode(node, "SequenceExpression")
	}
	return expr
}

// parseMaybeAssign parses an assignment expression, including the operators like
// +=.
func (p *parser) parseMaybeAssign(fi forInit, refs *destructuringErrors) *Node {
	if p.isContextual("yield") {
		if p.inGenerator() {
			return p.parseYield(fi)
		}
		// The tokenizer assumes an expression is allowed after yield, but this is
		// not that kind of yield.
		p.exprAllowed = false
	}

	ownDestructuringErrors := false
	oldParenAssign, oldTrailingComma, oldDoubleProto := -1, -1, -1
	if refs != nil {
		oldParenAssign = refs.parenthesizedAssign
		oldTrailingComma = refs.trailingComma
		oldDoubleProto = refs.doubleProto
		refs.parenthesizedAssign, refs.trailingComma = -1, -1
	} else {
		refs = newDestructuringErrors()
		ownDestructuringErrors = true
	}

	startPos, startLoc := p.start, p.startLoc
	if p.typ == tokParenL || p.typ == tokName {
		p.potentialArrowAt = p.start
		p.potentialArrowInForAwait = fi == inForAwaitInit
	}
	left := p.parseMaybeConditional(fi, refs)
	if p.typ.isAssign {
		node := p.startNodeAt(startPos, startLoc)
		node.Operator = p.value.(string)
		if p.typ == tokEq {
			left = p.toAssignable(left, false, refs)
		}
		if !ownDestructuringErrors {
			refs.parenthesizedAssign, refs.trailingComma = -1, -1
			// Reset these when detected inside this expression.
			if refs.shorthandAssign >= left.Start {
				refs.shorthandAssign = -1
			}
			if refs.doubleProto >= left.Start {
				refs.doubleProto = -1
			}
		}
		if p.typ == tokEq {
			p.checkLValPattern(left, bindNone, nil)
		} else {
			p.checkLValSimple(left, bindNone, nil)
		}
		node.Left = left
		p.next(false)
		node.Right = p.parseMaybeAssign(fi, nil)
		if oldDoubleProto > -1 {
			refs.doubleProto = oldDoubleProto
		}
		return p.finishNode(node, "AssignmentExpression")
	} else if ownDestructuringErrors {
		p.checkExpressionErrors(refs, true)
	}
	if oldParenAssign > -1 {
		refs.parenthesizedAssign = oldParenAssign
	}
	if oldTrailingComma > -1 {
		refs.trailingComma = oldTrailingComma
	}
	return left
}

// parseMaybeConditional parses a conditional (?:) expression.
func (p *parser) parseMaybeConditional(fi forInit, refs *destructuringErrors) *Node {
	startPos, startLoc := p.start, p.startLoc
	expr := p.parseExprOps(fi, refs)
	if p.checkExpressionErrors(refs, false) {
		return expr
	}
	if !(expr.Type == "ArrowFunctionExpression" && expr.Start == startPos) && p.eat(tokQuestion) {
		node := p.startNodeAt(startPos, startLoc)
		node.Test = expr
		node.Consequent = p.parseMaybeAssign(notForInit, nil)
		p.expect(tokColon)
		node.Alternate = p.parseMaybeAssign(fi, nil)
		return p.finishNode(node, "ConditionalExpression")
	}
	return expr
}

// parseExprOps starts the operator precedence parser.
func (p *parser) parseExprOps(fi forInit, refs *destructuringErrors) *Node {
	startPos, startLoc := p.start, p.startLoc
	expr := p.parseMaybeUnary(refs, false, false, fi)
	if p.checkExpressionErrors(refs, false) {
		return expr
	}
	if expr.Start == startPos && expr.Type == "ArrowFunctionExpression" {
		return expr
	}
	return p.parseExprOp(expr, startPos, startLoc, -1, fi)
}

// parseExprOp parses binary operators with the operator precedence parsing
// algorithm. left is the left-hand side of the operator; minPrec makes the function
// stop at an operator of lower precedence and defer it to its callers.
func (p *parser) parseExprOp(left *Node, leftStartPos int, leftStartLoc *Position, minPrec int, fi forInit) *Node {
	prec := p.typ.binop
	if prec != 0 && (fi == notForInit || p.typ != tokIn) && prec > minPrec {
		logical := p.typ == tokLogicalOR || p.typ == tokLogicalAND
		coalesce := p.typ == tokCoalesce
		if coalesce {
			// ?? has the precedence of the whole range of logical operators: its
			// right side must not hold logical expressions, so that mixing them is
			// detected.
			prec = tokLogicalAND.binop
		}
		op := p.value.(string)
		p.next(false)
		startPos, startLoc := p.start, p.startLoc
		right := p.parseExprOp(p.parseMaybeUnary(nil, false, false, fi), startPos, startLoc, prec, fi)
		node := p.buildBinary(leftStartPos, leftStartLoc, left, right, op, logical || coalesce)
		if logical && p.typ == tokCoalesce || coalesce && (p.typ == tokLogicalOR || p.typ == tokLogicalAND) {
			p.raiseRecoverable(p.start, "Logical expressions and coalesce expressions cannot be mixed. Wrap either by parentheses")
		}
		return p.parseExprOp(node, leftStartPos, leftStartLoc, minPrec, fi)
	}
	return left
}

func (p *parser) buildBinary(startPos int, startLoc *Position, left, right *Node, op string, logical bool) *Node {
	node := p.startNodeAt(startPos, startLoc)
	node.Left = left
	node.Operator = op
	node.Right = right
	if logical {
		return p.finishNode(node, "LogicalExpression")
	}
	return p.finishNode(node, "BinaryExpression")
}

// parseMaybeUnary parses unary operators, both prefix and postfix, and the **
// operator whose left side may not be a unary expression.
func (p *parser) parseMaybeUnary(refs *destructuringErrors, sawUnary, incDec bool, fi forInit) *Node {
	startPos, startLoc := p.start, p.startLoc
	var expr *Node
	if p.isContextual("await") && p.canAwait() {
		expr = p.parseAwait(fi)
		sawUnary = true
	} else if p.typ.prefix {
		node, update := p.startNode(), p.typ == tokIncDec
		node.Operator = p.value.(string)
		node.Prefix = true
		p.next(false)
		node.Argument = p.parseMaybeUnary(nil, true, update, fi)
		p.checkExpressionErrors(refs, true)
		if update {
			p.checkLValSimple(node.Argument, bindNone, nil)
		} else if p.strict && node.Operator == "delete" && node.Argument.Type == "Identifier" {
			p.raiseRecoverable(node.Start, "Deleting local variable in strict mode")
		} else {
			sawUnary = true
		}
		if update {
			expr = p.finishNode(node, "UpdateExpression")
		} else {
			expr = p.finishNode(node, "UnaryExpression")
		}
	} else {
		expr = p.parseExprSubscripts(refs, fi)
		if p.checkExpressionErrors(refs, false) {
			return expr
		}
		for p.typ.postfix && !p.canInsertSemicolon() {
			node := p.startNodeAt(startPos, startLoc)
			node.Operator = p.value.(string)
			node.Prefix = false
			node.Argument = expr
			p.checkLValSimple(expr, bindNone, nil)
			p.next(false)
			expr = p.finishNode(node, "UpdateExpression")
		}
	}

	if !incDec && !(expr.Type == "ArrowFunctionExpression" && expr.Start == startPos) && p.eat(tokStarstar) {
		if sawUnary {
			p.unexpectedAt(p.lastTokStart)
		}
		return p.buildBinary(startPos, startLoc, expr, p.parseMaybeUnary(nil, false, false, fi), "**", false)
	}
	return expr
}

// parseExprSubscripts parses call, dot, and []-subscript expressions.
func (p *parser) parseExprSubscripts(refs *destructuringErrors, fi forInit) *Node {
	startPos, startLoc := p.start, p.startLoc
	oldDoubleProto, oldShorthandAssign := -1, -1
	if refs != nil {
		oldDoubleProto = refs.doubleProto
		oldShorthandAssign = refs.shorthandAssign
		refs.doubleProto, refs.shorthandAssign = -1, -1
	}
	expr := p.parseExprAtom(refs, fi, false)
	if expr.Type == "ArrowFunctionExpression" && p.slice(p.lastTokStart, p.lastTokEnd) != ")" {
		return expr
	}
	result := p.parseSubscripts(expr, startPos, startLoc, false, fi)
	if refs != nil {
		if result.End > expr.End {
			p.checkExpressionErrors(refs, true)
			if refs.parenthesizedAssign >= result.Start {
				refs.parenthesizedAssign = -1
			}
			if refs.parenthesizedBind >= result.Start {
				refs.parenthesizedBind = -1
			}
			if refs.trailingComma >= result.Start {
				refs.trailingComma = -1
			}
		}
		if oldDoubleProto > -1 {
			refs.doubleProto = oldDoubleProto
		}
		if oldShorthandAssign > -1 {
			refs.shorthandAssign = oldShorthandAssign
		}
	}
	return result
}

func (p *parser) parseSubscripts(base *Node, startPos int, startLoc *Position, noCalls bool, fi forInit) *Node {
	maybeAsyncArrow := p.ecmaVersion >= 8 && base.Type == "Identifier" && base.Name == "async" &&
		p.lastTokEnd == base.End && !p.canInsertSemicolon() && base.End-base.Start == 5 &&
		p.potentialArrowAt == base.Start
	optionalChained := false

	for {
		element := p.parseSubscript(base, startPos, startLoc, noCalls, maybeAsyncArrow, optionalChained, fi)

		if element.Optional {
			optionalChained = true
		}
		if element.End == base.End || element.Type == "ArrowFunctionExpression" {
			if optionalChained {
				chainNode := p.startNodeAt(startPos, startLoc)
				chainNode.Expression = element
				element = p.finishNode(chainNode, "ChainExpression")
			}
			return element
		}

		base = element
		maybeAsyncArrow = false
	}
}

func (p *parser) shouldParseAsyncArrow() bool {
	return !p.canInsertSemicolon() && p.eat(tokArrow)
}

func (p *parser) parseSubscript(base *Node, startPos int, startLoc *Position, noCalls, maybeAsyncArrow, optionalChained bool, fi forInit) *Node {
	optionalSupported := p.ecmaVersion >= 11
	optional := optionalSupported && p.eat(tokQuestionDot)
	if noCalls && optional {
		p.raise(p.lastTokStart, "Optional chaining cannot appear in the callee of new expressions")
	}

	computed := p.eat(tokBracketL)
	if computed || optional && p.typ != tokParenL && p.typ != tokBackQuote || p.eat(tokDot) {
		node := p.startNodeAt(startPos, startLoc)
		node.Object = base
		if computed {
			node.Property = p.parseExpression(notForInit, nil)
			p.expect(tokBracketR)
		} else {
			node.Property = p.parseIdent(true)
		}
		node.Computed = computed
		node.Optional = optional
		return p.finishNode(node, "MemberExpression")
	}
	if !noCalls && p.eat(tokParenL) {
		refs := newDestructuringErrors()
		oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos
		p.yieldPos, p.awaitPos, p.awaitIdentPos = 0, 0, 0
		exprList := p.parseExprList(tokParenR, p.ecmaVersion >= 8, false, refs)
		if maybeAsyncArrow && !optional && p.shouldParseAsyncArrow() {
			p.checkPatternErrors(refs, false)
			p.checkYieldAwaitInDefaultParams()
			if p.awaitIdentPos > 0 {
				p.raise(p.awaitIdentPos, "Cannot use 'await' as identifier inside an async function")
			}
			p.yieldPos, p.awaitPos, p.awaitIdentPos = oldYieldPos, oldAwaitPos, oldAwaitIdentPos
			return p.parseArrowExpression(p.startNodeAt(startPos, startLoc), exprList, true, fi)
		}
		p.checkExpressionErrors(refs, true)
		p.yieldPos = orElse(oldYieldPos, p.yieldPos)
		p.awaitPos = orElse(oldAwaitPos, p.awaitPos)
		p.awaitIdentPos = orElse(oldAwaitIdentPos, p.awaitIdentPos)
		node := p.startNodeAt(startPos, startLoc)
		node.Callee = base
		node.Arguments = exprList
		node.Optional = optional
		return p.finishNode(node, "CallExpression")
	}
	if p.typ == tokBackQuote {
		if optional || optionalChained {
			p.raise(p.start, "Optional chaining cannot appear in the tag of tagged template expressions")
		}
		node := p.startNodeAt(startPos, startLoc)
		node.Tag = base
		node.Quasi = p.parseTemplate(true)
		return p.finishNode(node, "TaggedTemplateExpression")
	}
	return base
}

// orElse is JavaScript's a || b for positions, where 0 means none.
func orElse(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// parseExprAtom parses an atomic expression: a single token that is an expression,
// an expression started by a keyword like function or new, or an expression
// wrapped in (), [] or {}.
func (p *parser) parseExprAtom(refs *destructuringErrors, fi forInit, forNew bool) *Node {
	// A division operator in an expression position means the tokenizer got
	// confused: read a regular expression instead.
	if p.typ == tokSlash {
		p.readRegexp()
	}

	canBeArrow := p.potentialArrowAt == p.start
	switch p.typ {
	case tokSuper:
		if !p.allowSuper() {
			p.raise(p.start, "'super' keyword outside a method")
		}
		node := p.startNode()
		p.next(false)
		if p.typ == tokParenL && !p.allowDirectSuper() {
			p.raise(node.Start, "super() call outside constructor of a subclass")
		}
		// super may only be followed by [ Expression ], . IdentifierName or
		// ( Arguments ).
		if p.typ != tokDot && p.typ != tokBracketL && p.typ != tokParenL {
			p.unexpected()
		}
		return p.finishNode(node, "Super")

	case tokThis:
		node := p.startNode()
		p.next(false)
		return p.finishNode(node, "ThisExpression")

	case tokName:
		startPos, startLoc, containsEsc := p.start, p.startLoc, p.containsEsc
		id := p.parseIdent(false)
		if p.ecmaVersion >= 8 && !containsEsc && id.Name == "async" && !p.canInsertSemicolon() && p.eat(tokFunction) {
			p.overrideContext(ctxFExpr)
			return p.parseFunction(p.startNodeAt(startPos, startLoc), 0, true, fi)
		}
		if canBeArrow && !p.canInsertSemicolon() {
			if p.eat(tokArrow) {
				return p.parseArrowExpression(p.startNodeAt(startPos, startLoc), []*Node{id}, false, fi)
			}
			if p.ecmaVersion >= 8 && id.Name == "async" && p.typ == tokName && !containsEsc &&
				(!p.potentialArrowInForAwait || p.value != "of" || p.containsEsc) {
				id = p.parseIdent(false)
				if p.canInsertSemicolon() || !p.eat(tokArrow) {
					p.unexpected()
				}
				return p.parseArrowExpression(p.startNodeAt(startPos, startLoc), []*Node{id}, true, fi)
			}
		}
		return id

	case tokRegexp:
		value := p.value.(*RegexLiteral)
		node := p.parseLiteral(nil)
		node.Regex = &RegexLiteral{Pattern: value.Pattern, Flags: value.Flags}
		return node

	case tokNum, tokString:
		return p.parseLiteral(p.value)

	case tokNull, tokTrue, tokFalse:
		node := p.startNode()
		if p.typ != tokNull {
			node.LiteralValue = p.typ == tokTrue
		}
		node.Raw = p.typ.keyword
		p.next(false)
		return p.finishNode(node, "Literal")

	case tokParenL:
		start := p.start
		expr := p.parseParenAndDistinguishExpression(canBeArrow, fi)
		if refs != nil {
			if refs.parenthesizedAssign < 0 && !isSimpleAssignTarget(expr) {
				refs.parenthesizedAssign = start
			}
			if refs.parenthesizedBind < 0 {
				refs.parenthesizedBind = start
			}
		}
		return expr

	case tokBracketL:
		node := p.startNode()
		p.next(false)
		node.Elements = p.parseExprList(tokBracketR, true, true, refs)
		return p.finishNode(node, "ArrayExpression")

	case tokBraceL:
		p.overrideContext(ctxBExpr)
		return p.parseObj(false, refs)

	case tokFunction:
		node := p.startNode()
		p.next(false)
		return p.parseFunction(node, 0, false, notForInit)

	case tokClass:
		return p.parseClass(p.startNode(), false)

	case tokNew:
		return p.parseNew()

	case tokBackQuote:
		return p.parseTemplate(false)

	case tokImport:
		if p.ecmaVersion >= 11 {
			return p.parseExprImport(forNew)
		}
	}
	p.unexpected()
	return nil
}

func (p *parser) parseExprImport(forNew bool) *Node {
	node := p.startNode()

	// import is consumed as an identifier for import.meta; parseIdent(true) does
	// not check escape sequences, so they are checked here.
	if p.containsEsc {
		p.raiseRecoverable(p.start, "Escape sequence in keyword import")
	}
	p.next(false)

	if p.typ == tokParenL && !forNew {
		return p.parseDynamicImport(node)
	}
	if p.typ == tokDot {
		meta := p.startNodeAt(node.Start, node.locStart())
		meta.Name = "import"
		node.Meta = p.finishNode(meta, "Identifier")
		return p.parseImportMeta(node)
	}
	p.unexpected()
	return nil
}

// locStart is node.loc && node.loc.start.
func (n *Node) locStart() *Position {
	if n.Loc == nil {
		return nil
	}
	return n.Loc.Start
}

func (p *parser) parseDynamicImport(node *Node) *Node {
	p.next(false) // (
	node.Source = p.parseMaybeAssign(notForInit, nil)
	if !p.eat(tokParenR) {
		errorPos := p.start
		if p.eat(tokComma) && p.eat(tokParenR) {
			p.raiseRecoverable(errorPos, "Trailing comma is not allowed in import()")
		} else {
			p.unexpectedAt(errorPos)
		}
	}
	return p.finishNode(node, "ImportExpression")
}

// parseImportMeta reads import.meta, which scripts may not use.
func (p *parser) parseImportMeta(node *Node) *Node {
	p.next(false) // .
	containsEsc := p.containsEsc
	node.Property = p.parseIdent(true)
	if node.Property.Name != "meta" {
		p.raiseRecoverable(node.Property.Start, "The only valid meta property for import is 'import.meta'")
	}
	if containsEsc {
		p.raiseRecoverable(node.Start, "'import.meta' must not contain escaped characters")
	}
	p.raiseRecoverable(node.Start, "Cannot use 'import.meta' outside a module")
	return nil
}

func (p *parser) parseLiteral(value any) *Node {
	node := p.startNode()
	node.LiteralValue = value
	node.Raw = p.slice(p.start, p.end)
	if strings.HasSuffix(node.Raw, "n") {
		if b, ok := value.(js.BigInt); ok {
			node.Bigint = b.String()
		}
	}
	p.next(false)
	return p.finishNode(node, "Literal")
}

func (p *parser) parseParenExpression() *Node {
	p.expect(tokParenL)
	val := p.parseExpression(notForInit, nil)
	p.expect(tokParenR)
	return val
}

func (p *parser) parseParenAndDistinguishExpression(canBeArrow bool, fi forInit) *Node {
	if p.ecmaVersion < 6 {
		return p.parseParenExpression()
	}
	startPos, startLoc, allowTrailingComma := p.start, p.startLoc, p.ecmaVersion >= 8
	p.next(false)

	innerStartPos, innerStartLoc := p.start, p.startLoc
	exprList, first, lastIsComma := []*Node{}, true, false
	refs := newDestructuringErrors()
	oldYieldPos, oldAwaitPos := p.yieldPos, p.awaitPos
	spreadStart := 0
	p.yieldPos, p.awaitPos = 0, 0
	// awaitIdentPos is not saved, so that awaits nested in parameters are checked.
	for p.typ != tokParenR {
		if first {
			first = false
		} else {
			p.expect(tokComma)
		}
		if allowTrailingComma && p.afterTrailingComma(tokParenR, true) {
			lastIsComma = true
			break
		} else if p.typ == tokEllipsis {
			spreadStart = p.start
			exprList = append(exprList, p.parseRestBinding())
			if p.typ == tokComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			break
		} else {
			exprList = append(exprList, p.parseMaybeAssign(notForInit, refs))
		}
	}
	innerEndPos, innerEndLoc := p.lastTokEnd, p.lastTokEndLoc
	p.expect(tokParenR)

	if canBeArrow && !p.canInsertSemicolon() && p.eat(tokArrow) {
		p.checkPatternErrors(refs, false)
		p.checkYieldAwaitInDefaultParams()
		p.yieldPos, p.awaitPos = oldYieldPos, oldAwaitPos
		return p.parseArrowExpression(p.startNodeAt(startPos, startLoc), exprList, false, fi)
	}

	if len(exprList) == 0 || lastIsComma {
		p.unexpectedAt(p.lastTokStart)
	}
	if spreadStart != 0 {
		p.unexpectedAt(spreadStart)
	}
	p.checkExpressionErrors(refs, true)
	p.yieldPos = orElse(oldYieldPos, p.yieldPos)
	p.awaitPos = orElse(oldAwaitPos, p.awaitPos)

	if len(exprList) > 1 {
		val := p.startNodeAt(innerStartPos, innerStartLoc)
		val.Expressions = exprList
		return p.finishNodeAt(val, "SequenceExpression", innerEndPos, innerEndLoc)
	}
	return exprList[0]
}

// parseNew parses new expressions and new.target. The callee may hold [] and dot
// subscripts but not a call, unless parenthesized.
func (p *parser) parseNew() *Node {
	if p.containsEsc {
		p.raiseRecoverable(p.start, "Escape sequence in keyword new")
	}
	node := p.startNode()
	p.next(false)
	if p.ecmaVersion >= 6 && p.typ == tokDot {
		meta := p.startNodeAt(node.Start, node.locStart())
		meta.Name = "new"
		node.Meta = p.finishNode(meta, "Identifier")
		p.next(false)
		containsEsc := p.containsEsc
		node.Property = p.parseIdent(true)
		if node.Property.Name != "target" {
			p.raiseRecoverable(node.Property.Start, "The only valid meta property for new is 'new.target'")
		}
		if containsEsc {
			p.raiseRecoverable(node.Start, "'new.target' must not contain escaped characters")
		}
		if !p.allowNewDotTarget() {
			p.raiseRecoverable(node.Start, "'new.target' can only be used in functions and class static block")
		}
		return p.finishNode(node, "MetaProperty")
	}
	startPos, startLoc := p.start, p.startLoc
	node.Callee = p.parseSubscripts(p.parseExprAtom(nil, notForInit, true), startPos, startLoc, true, notForInit)
	if node.Callee.Type == "Super" {
		p.raiseRecoverable(startPos, "Invalid use of 'super'")
	}
	if p.eat(tokParenL) {
		node.Arguments = p.parseExprList(tokParenR, p.ecmaVersion >= 8, false, nil)
	} else {
		node.Arguments = []*Node{}
	}
	return p.finishNode(node, "NewExpression")
}

// normalizeTemplateRaw replaces CR LF and CR by LF in the raw text of a template
// element.
func normalizeTemplateRaw(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func (p *parser) parseTemplateElement(isTagged bool) *Node {
	elem := p.startNode()
	if p.typ == tokInvalidTemplate {
		if !isTagged {
			p.raiseRecoverable(p.start, "Bad escape sequence in untagged template literal")
		}
		elem.TemplateValue = &TemplateValue{Raw: normalizeTemplateRaw(p.value.(string))}
	} else {
		elem.TemplateValue = &TemplateValue{Raw: normalizeTemplateRaw(p.slice(p.start, p.end))}
		// The token is the eof of an unterminated template literal when it has
		// no string value; parseTemplate reports that next.
		if cooked, ok := p.value.(string); ok {
			elem.TemplateValue.Cooked = &cooked
		}
	}
	p.next(false)
	elem.Tail = p.typ == tokBackQuote
	return p.finishNode(elem, "TemplateElement")
}

func (p *parser) parseTemplate(isTagged bool) *Node {
	node := p.startNode()
	p.next(false)
	node.Expressions = []*Node{}
	curElt := p.parseTemplateElement(isTagged)
	node.Quasis = []*Node{curElt}
	for !curElt.Tail {
		if p.typ == tokEOF {
			p.raise(p.pos, "Unterminated template literal")
		}
		p.expect(tokDollarBraceL)
		node.Expressions = append(node.Expressions, p.parseExpression(notForInit, nil))
		p.expect(tokBraceR)
		curElt = p.parseTemplateElement(isTagged)
		node.Quasis = append(node.Quasis, curElt)
	}
	p.next(false)
	return p.finishNode(node, "TemplateLiteral")
}

func (p *parser) isAsyncProp(prop *Node) bool {
	return !prop.Computed && prop.Key.Type == "Identifier" && prop.Key.Name == "async" &&
		(p.typ == tokName || p.typ == tokNum || p.typ == tokString || p.typ == tokBracketL || p.typ.keyword != "" || p.ecmaVersion >= 9 && p.typ == tokStar) &&
		!hasLineBreak(p.input[p.lastTokEnd:p.start])
}

// parseObj parses an object literal or binding pattern.
func (p *parser) parseObj(isPattern bool, refs *destructuringErrors) *Node {
	node, first, hash := p.startNode(), true, &propHash{}
	node.Properties = []*Node{}
	p.next(false)
	for !p.eat(tokBraceR) {
		if !first {
			p.expect(tokComma)
			if p.ecmaVersion >= 5 && p.afterTrailingComma(tokBraceR, false) {
				break
			}
		} else {
			first = false
		}

		prop := p.parseProperty(isPattern, refs)
		if !isPattern {
			p.checkPropClash(prop, hash, refs)
		}
		node.Properties = append(node.Properties, prop)
	}
	if isPattern {
		return p.finishNode(node, "ObjectPattern")
	}
	return p.finishNode(node, "ObjectExpression")
}

func (p *parser) parseProperty(isPattern bool, refs *destructuringErrors) *Node {
	prop := p.startNode()
	var isGenerator, isAsync bool
	var startPos int
	var startLoc *Position
	if p.ecmaVersion >= 9 && p.eat(tokEllipsis) {
		if isPattern {
			prop.Argument = p.parseIdent(false)
			if p.typ == tokComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			return p.finishNode(prop, "RestElement")
		}
		prop.Argument = p.parseMaybeAssign(notForInit, refs)
		// A trailing comma after a spread is an error in a pattern; toAssignable
		// reports it.
		if p.typ == tokComma && refs != nil && refs.trailingComma < 0 {
			refs.trailingComma = p.start
		}
		return p.finishNode(prop, "SpreadElement")
	}
	if p.ecmaVersion >= 6 {
		prop.Method = false
		prop.Shorthand = false
		if isPattern || refs != nil {
			startPos, startLoc = p.start, p.startLoc
		}
		if !isPattern {
			isGenerator = p.eat(tokStar)
		}
	}
	containsEsc := p.containsEsc
	p.parsePropertyName(prop)
	if !isPattern && !containsEsc && p.ecmaVersion >= 8 && !isGenerator && p.isAsyncProp(prop) {
		isAsync = true
		isGenerator = p.ecmaVersion >= 9 && p.eat(tokStar)
		p.parsePropertyName(prop)
	}
	p.parsePropertyValue(prop, isPattern, isGenerator, isAsync, startPos, startLoc, refs, containsEsc)
	return p.finishNode(prop, "Property")
}

func (p *parser) parseGetterSetter(prop *Node) {
	kind := prop.Key.Name
	p.parsePropertyName(prop)
	prop.Value = p.parseMethod(false, false, false)
	prop.Kind = kind
	paramCount := 1
	if kind == "get" {
		paramCount = 0
	}
	if len(prop.Value.Params) != paramCount {
		start := prop.Value.Start
		if kind == "get" {
			p.raiseRecoverable(start, "getter should have no params")
		}
		p.raiseRecoverable(start, "setter should have exactly one param")
	}
	if kind == "set" && prop.Value.Params[0].Type == "RestElement" {
		p.raiseRecoverable(prop.Value.Params[0].Start, "Setter cannot use rest params")
	}
}

func (p *parser) parsePropertyValue(prop *Node, isPattern, isGenerator, isAsync bool, startPos int, startLoc *Position, refs *destructuringErrors, containsEsc bool) {
	if (isGenerator || isAsync) && p.typ == tokColon {
		p.unexpected()
	}

	switch {
	case p.eat(tokColon):
		if isPattern {
			prop.Value = p.parseMaybeDefault(p.start, p.startLoc, nil)
		} else {
			prop.Value = p.parseMaybeAssign(notForInit, refs)
		}
		prop.Kind = "init"
	case p.ecmaVersion >= 6 && p.typ == tokParenL:
		if isPattern {
			p.unexpected()
		}
		prop.Method = true
		prop.Value = p.parseMethod(isGenerator, isAsync, false)
		prop.Kind = "init"
	case !isPattern && !containsEsc && p.ecmaVersion >= 5 && !prop.Computed && prop.Key.Type == "Identifier" &&
		(prop.Key.Name == "get" || prop.Key.Name == "set") &&
		p.typ != tokComma && p.typ != tokBraceR && p.typ != tokEq:
		if isGenerator || isAsync {
			p.unexpected()
		}
		p.parseGetterSetter(prop)
	case p.ecmaVersion >= 6 && !prop.Computed && prop.Key.Type == "Identifier":
		if isGenerator || isAsync {
			p.unexpected()
		}
		p.checkUnreserved(prop.Key)
		if prop.Key.Name == "await" && p.awaitIdentPos == 0 {
			p.awaitIdentPos = startPos
		}
		switch {
		case isPattern:
			prop.Value = p.parseMaybeDefault(startPos, startLoc, p.copyNode(prop.Key))
		case p.typ == tokEq && refs != nil:
			if refs.shorthandAssign < 0 {
				refs.shorthandAssign = p.start
			}
			prop.Value = p.parseMaybeDefault(startPos, startLoc, p.copyNode(prop.Key))
		default:
			prop.Value = p.copyNode(prop.Key)
		}
		prop.Kind = "init"
		prop.Shorthand = true
	default:
		p.unexpected()
	}
}

func (p *parser) parsePropertyName(prop *Node) *Node {
	if p.ecmaVersion >= 6 {
		if p.eat(tokBracketL) {
			prop.Computed = true
			prop.Key = p.parseMaybeAssign(notForInit, nil)
			p.expect(tokBracketR)
			return prop.Key
		}
		prop.Computed = false
	}
	if p.typ == tokNum || p.typ == tokString {
		prop.Key = p.parseExprAtom(nil, notForInit, false)
	} else {
		prop.Key = p.parseIdent(true)
	}
	return prop.Key
}

// initFunction initializes an empty function node.
func (p *parser) initFunction(node *Node) {
	node.Id = nil
	node.Generator = false
	node.ExpressionBody = false
	node.Async = false
}

// parseMethod parses the parameters and body of an object or class method.
func (p *parser) parseMethod(isGenerator, isAsync, allowDirectSuper bool) *Node {
	node := p.startNode()
	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos

	p.initFunction(node)
	if p.ecmaVersion >= 6 {
		node.Generator = isGenerator
	}
	if p.ecmaVersion >= 8 {
		node.Async = isAsync
	}

	p.yieldPos, p.awaitPos, p.awaitIdentPos = 0, 0, 0
	flags := functionFlags(isAsync, node.Generator) | scopeSuper
	if allowDirectSuper {
		flags |= scopeDirectSuper
	}
	p.enterScope(flags)

	p.expect(tokParenL)
	node.Params = p.parseBindingList(tokParenR, false, p.ecmaVersion >= 8)
	p.checkYieldAwaitInDefaultParams()
	p.parseFunctionBody(node, false, true, notForInit)

	p.yieldPos, p.awaitPos, p.awaitIdentPos = oldYieldPos, oldAwaitPos, oldAwaitIdentPos
	return p.finishNode(node, "FunctionExpression")
}

// parseArrowExpression parses an arrow function with the given parameters.
func (p *parser) parseArrowExpression(node *Node, params []*Node, isAsync bool, fi forInit) *Node {
	oldYieldPos, oldAwaitPos, oldAwaitIdentPos := p.yieldPos, p.awaitPos, p.awaitIdentPos

	p.enterScope(functionFlags(isAsync, false) | scopeArrow)
	p.initFunction(node)
	if p.ecmaVersion >= 8 {
		node.Async = isAsync
	}

	p.yieldPos, p.awaitPos, p.awaitIdentPos = 0, 0, 0

	node.Params = p.toAssignableList(params, true)
	p.parseFunctionBody(node, true, false, fi)

	p.yieldPos, p.awaitPos, p.awaitIdentPos = oldYieldPos, oldAwaitPos, oldAwaitIdentPos
	return p.finishNode(node, "ArrowFunctionExpression")
}

// parseFunctionBody parses a function body and checks the parameters.
func (p *parser) parseFunctionBody(node *Node, isArrowFunction, isMethod bool, fi forInit) {
	isExpression := isArrowFunction && p.typ != tokBraceL
	oldStrict, useStrict := p.strict, false

	if isExpression {
		node.Body = p.parseMaybeAssign(fi, nil)
		node.ExpressionBody = true
		p.checkParams(node, false)
	} else {
		nonSimple := p.ecmaVersion >= 7 && !isSimpleParamList(node.Params)
		if !oldStrict || nonSimple {
			useStrict = p.strictDirective(p.end)
			// In a strict mode function, parameter names may not repeat and may
			// not be eval or arguments.
			if useStrict && nonSimple {
				p.raiseRecoverable(node.Start, "Illegal 'use strict' directive in function with non-simple parameter list")
			}
		}
		// A function starts a new scope for labels.
		oldLabels := p.labels
		p.labels = nil
		if useStrict {
			p.strict = true
		}

		// The parameters are var-declared names of the function scope, so that a
		// let or const in the body that clashes with one is an error.
		p.checkParams(node, !oldStrict && !useStrict && !isArrowFunction && !isMethod && isSimpleParamList(node.Params))
		// The function name may not be a forbidden identifier in strict mode,
		// such as eval.
		if p.strict && node.Id != nil {
			p.checkLValSimple(node.Id, bindOutside, nil)
		}
		node.Body = p.parseBlock(false, nil, useStrict && !oldStrict)
		node.ExpressionBody = false
		p.adaptDirectivePrologue(node.Body.BodyList)
		p.labels = oldLabels
	}
	p.exitScope()
}

func isSimpleParamList(params []*Node) bool {
	for _, param := range params {
		if param.Type != "Identifier" {
			return false
		}
	}
	return true
}

// checkParams checks the parameters for eval or arguments in strict mode and for
// duplicate names.
func (p *parser) checkParams(node *Node, allowDuplicates bool) {
	var nameHash map[string]bool
	if !allowDuplicates {
		nameHash = map[string]bool{}
	}
	for _, param := range node.Params {
		p.checkLValInnerPattern(param, bindVar, nameHash)
	}
}

// parseExprList parses a comma-separated list of expressions up to close.
// allowEmpty turns consecutive commas into nil elements, as array literals need.
func (p *parser) parseExprList(close *tokenType, allowTrailingComma, allowEmpty bool, refs *destructuringErrors) []*Node {
	elts, first := []*Node{}, true
	for !p.eat(close) {
		if !first {
			p.expect(tokComma)
			if allowTrailingComma && p.afterTrailingComma(close, false) {
				break
			}
		} else {
			first = false
		}

		var elt *Node
		switch {
		case allowEmpty && p.typ == tokComma:
		case p.typ == tokEllipsis:
			elt = p.parseSpread(refs)
			if refs != nil && p.typ == tokComma && refs.trailingComma < 0 {
				refs.trailingComma = p.start
			}
		default:
			elt = p.parseMaybeAssign(notForInit, refs)
		}
		elts = append(elts, elt)
	}
	return elts
}

func (p *parser) checkUnreserved(ref *Node) {
	start, end, name := ref.Start, ref.End, ref.Name
	if p.inGenerator() && name == "yield" {
		p.raiseRecoverable(start, "Cannot use 'yield' as identifier inside a generator")
	}
	if p.inAsync() && name == "await" {
		p.raiseRecoverable(start, "Cannot use 'await' as identifier inside an async function")
	}
	if p.keywords[name] {
		p.raise(start, "Unexpected keyword '"+name+"'")
	}
	if p.ecmaVersion < 6 && jsstr.Index(p.input[start:end], []uint16{'\\'}, 0) != -1 {
		return
	}
	reserved := p.reservedWords
	if p.strict {
		reserved = p.reservedWordsStrict
	}
	if reserved[name] {
		p.raiseRecoverable(start, "The keyword '"+name+"' is reserved")
	}
}

// parseIdent parses the next token as an identifier. With liberal (property
// names) keywords are identifiers too.
func (p *parser) parseIdent(liberal bool) *Node {
	node := p.parseIdentNode()
	p.next(liberal)
	p.finishNode(node, "Identifier")
	if !liberal {
		p.checkUnreserved(node)
		if node.Name == "await" && p.awaitIdentPos == 0 {
			p.awaitIdentPos = node.Start
		}
	}
	return node
}

func (p *parser) parseIdentNode() *Node {
	node := p.startNode()
	switch {
	case p.typ == tokName:
		node.Name = p.value.(string)
	case p.typ.keyword != "":
		node.Name = p.typ.keyword
		// The class and function keywords push a context that is popped when
		// their body ends; a keyword used as a name has no body, so its context
		// is popped here, unless it follows a dot, where no context was pushed.
		if (node.Name == "class" || node.Name == "function") &&
			(p.lastTokEnd != p.lastTokStart+1 || p.charCodeAt(p.lastTokStart) != '.') {
			p.popContext()
		}
		p.typ = tokName
	default:
		p.unexpected()
	}
	return node
}

// parseYield parses a yield expression inside a generator.
func (p *parser) parseYield(fi forInit) *Node {
	if p.yieldPos == 0 {
		p.yieldPos = p.start
	}
	node := p.startNode()
	p.next(false)
	if p.typ == tokSemi || p.canInsertSemicolon() || p.typ != tokStar && !p.typ.startsExpr {
		node.Delegate = false
		node.Argument = nil
	} else {
		node.Delegate = p.eat(tokStar)
		node.Argument = p.parseMaybeAssign(fi, nil)
	}
	return p.finishNode(node, "YieldExpression")
}

func (p *parser) parseAwait(fi forInit) *Node {
	if p.awaitPos == 0 {
		p.awaitPos = p.start
	}
	node := p.startNode()
	p.next(false)
	node.Argument = p.parseMaybeUnary(nil, true, false, fi)
	return p.finishNode(node, "AwaitExpression")
}
