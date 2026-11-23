package acorn

// toAssignable converts an expression atom to an assignable pattern if possible.
// isBinding is set for parameters and declarations, where member expressions are
// not allowed.
func (p *parser) toAssignable(node *Node, isBinding bool, refs *destructuringErrors) *Node {
	if p.ecmaVersion < 6 || node == nil {
		p.checkPatternErrors(refs, true)
		return node
	}
	switch node.Type {
	case "Identifier":
		if p.inAsync() && node.Name == "await" {
			p.raise(node.Start, "Cannot use 'await' as identifier inside an async function")
		}

	case "ObjectPattern", "ArrayPattern", "AssignmentPattern", "RestElement":

	case "ObjectExpression":
		node.Type = "ObjectPattern"
		p.checkPatternErrors(refs, true)
		for _, prop := range node.Properties {
			p.toAssignable(prop, isBinding, nil)
			// AssignmentRestProperty: it is a syntax error if the target of `...`
			// is an array or object literal.
			if prop.Type == "RestElement" && (prop.Argument.Type == "ArrayPattern" || prop.Argument.Type == "ObjectPattern") {
				p.raise(prop.Argument.Start, "Unexpected token")
			}
		}

	case "Property":
		// An AssignmentProperty has type Property.
		if node.Kind != "init" {
			p.raise(node.Key.Start, "Object pattern can't contain getter or setter")
		}
		p.toAssignable(node.Value, isBinding, nil)

	case "ArrayExpression":
		node.Type = "ArrayPattern"
		p.checkPatternErrors(refs, true)
		p.toAssignableList(node.Elements, isBinding)

	case "SpreadElement":
		node.Type = "RestElement"
		p.toAssignable(node.Argument, isBinding, nil)
		if node.Argument.Type == "AssignmentPattern" {
			p.raise(node.Argument.Start, "Rest elements cannot have a default value")
		}

	case "AssignmentExpression":
		if node.Operator != "=" {
			p.raise(node.Left.End, "Only '=' operator can be used for specifying default value.")
		}
		node.Type = "AssignmentPattern"
		node.Operator = ""
		p.toAssignable(node.Left, isBinding, nil)

	case "ChainExpression":
		p.raiseRecoverable(node.Start, "Optional chaining cannot appear in left-hand side")

	case "MemberExpression":
		if isBinding {
			p.raise(node.Start, "Assigning to rvalue")
		}

	default:
		p.raise(node.Start, "Assigning to rvalue")
	}
	return node
}

// toAssignableList converts a list of expression atoms to a binding list.
func (p *parser) toAssignableList(exprList []*Node, isBinding bool) []*Node {
	for _, elt := range exprList {
		if elt != nil {
			p.toAssignable(elt, isBinding, nil)
		}
	}
	if n := len(exprList); n > 0 {
		last := exprList[n-1]
		if p.ecmaVersion == 6 && isBinding && last != nil && last.Type == "RestElement" && last.Argument.Type != "Identifier" {
			p.unexpectedAt(last.Argument.Start)
		}
	}
	return exprList
}

// parseSpread parses a spread element.
func (p *parser) parseSpread(refs *destructuringErrors) *Node {
	node := p.startNode()
	p.next(false)
	node.Argument = p.parseMaybeAssign(notForInit, refs)
	return p.finishNode(node, "SpreadElement")
}

func (p *parser) parseRestBinding() *Node {
	node := p.startNode()
	p.next(false)
	// In ECMAScript 2015 the rest element of a parameter list must be an
	// identifier.
	if p.ecmaVersion == 6 && p.typ != tokName {
		p.unexpected()
	}
	node.Argument = p.parseBindingAtom()
	return p.finishNode(node, "RestElement")
}

// parseBindingAtom parses an assignable atom: an identifier or, from ECMAScript
// 2015, an array or object pattern.
func (p *parser) parseBindingAtom() *Node {
	if p.ecmaVersion >= 6 {
		switch p.typ {
		case tokBracketL:
			node := p.startNode()
			p.next(false)
			node.Elements = p.parseBindingList(tokBracketR, true, true)
			return p.finishNode(node, "ArrayPattern")
		case tokBraceL:
			return p.parseObj(true, nil)
		}
	}
	return p.parseIdent(false)
}

// parseBindingList parses the elements of an array pattern or a parameter list up
// to close. allowEmpty turns holes into nil elements.
func (p *parser) parseBindingList(close *tokenType, allowEmpty, allowTrailingComma bool) []*Node {
	elts, first := []*Node{}, true
	for !p.eat(close) {
		if first {
			first = false
		} else {
			p.expect(tokComma)
		}
		switch {
		case allowEmpty && p.typ == tokComma:
			elts = append(elts, nil)
		case allowTrailingComma && p.afterTrailingComma(close, false):
			return elts
		case p.typ == tokEllipsis:
			elts = append(elts, p.parseRestBinding())
			if p.typ == tokComma {
				p.raiseRecoverable(p.start, "Comma is not permitted after the rest element")
			}
			p.expect(close)
			return elts
		default:
			elts = append(elts, p.parseMaybeDefault(p.start, p.startLoc, nil))
		}
	}
	return elts
}

// parseMaybeDefault parses an assignment pattern around the given atom (or the
// binding atom that follows) if a default value follows it.
func (p *parser) parseMaybeDefault(startPos int, startLoc *Position, left *Node) *Node {
	if left == nil {
		left = p.parseBindingAtom()
	}
	if p.ecmaVersion < 6 || !p.eat(tokEq) {
		return left
	}
	node := p.startNodeAt(startPos, startLoc)
	node.Left = left
	node.Right = p.parseMaybeAssign(notForInit, nil)
	return p.finishNode(node, "AssignmentPattern")
}

// The three checkLVal functions verify that a node is an lvalue, something that
// can be bound or assigned to: that no bound or assigned identifier is a reserved
// word, that the names of bindings are declared in their scope, and, when
// checkClashes is not nil, that no parameter name repeats.
//
// checkLValSimple accepts identifiers and member expressions (a += 1, function
// and class names), checkLValPattern also object and array patterns (a = 1,
// const a = 1, catch (a)), and checkLValInnerPattern also defaults and rest
// elements (the inside of patterns, and parameters). bindingType is bindNone for
// an assignment target and the kind of binding otherwise.

func (p *parser) checkLValSimple(expr *Node, bindingType int, checkClashes map[string]bool) {
	isBind := bindingType != bindNone
	switch expr.Type {
	case "Identifier":
		if p.strict && p.reservedWordsStrictBind[expr.Name] {
			if isBind {
				p.raiseRecoverable(expr.Start, "Binding "+expr.Name+" in strict mode")
			}
			p.raiseRecoverable(expr.Start, "Assigning to "+expr.Name+" in strict mode")
		}
		if isBind {
			if bindingType == bindLexical && expr.Name == "let" {
				p.raiseRecoverable(expr.Start, "let is disallowed as a lexically bound name")
			}
			if checkClashes != nil {
				if checkClashes[expr.Name] {
					p.raiseRecoverable(expr.Start, "Argument name clash")
				}
				checkClashes[expr.Name] = true
			}
			if bindingType != bindOutside {
				p.declareName(expr.Name, bindingType, expr.Start)
			}
		}

	case "ChainExpression":
		p.raiseRecoverable(expr.Start, "Optional chaining cannot appear in left-hand side")

	case "MemberExpression":
		if isBind {
			p.raiseRecoverable(expr.Start, "Binding member expression")
		}

	default:
		if isBind {
			p.raise(expr.Start, "Binding rvalue")
		}
		p.raise(expr.Start, "Assigning to rvalue")
	}
}

func (p *parser) checkLValPattern(expr *Node, bindingType int, checkClashes map[string]bool) {
	switch expr.Type {
	case "ObjectPattern":
		for _, prop := range expr.Properties {
			p.checkLValInnerPattern(prop, bindingType, checkClashes)
		}
	case "ArrayPattern":
		for _, elem := range expr.Elements {
			if elem != nil {
				p.checkLValInnerPattern(elem, bindingType, checkClashes)
			}
		}
	default:
		p.checkLValSimple(expr, bindingType, checkClashes)
	}
}

func (p *parser) checkLValInnerPattern(expr *Node, bindingType int, checkClashes map[string]bool) {
	switch expr.Type {
	case "Property":
		// An AssignmentProperty has type Property.
		p.checkLValInnerPattern(expr.Value, bindingType, checkClashes)
	case "AssignmentPattern":
		p.checkLValPattern(expr.Left, bindingType, checkClashes)
	case "RestElement":
		p.checkLValPattern(expr.Argument, bindingType, checkClashes)
	default:
		p.checkLValPattern(expr, bindingType, checkClashes)
	}
}
