package jsrt

// Parsing of yield. Inside a generator body yield is an operator with the
// precedence of an assignment; elsewhere it is an identifier (a reserved word
// in strict-mode code).

// parseYield parses "yield", "yield value" or "yield* iterable". A bare yield
// ends at a line break or at a token that cannot start its operand.
func (p *parser) parseYield(noIn bool) expr {
	pos := p.tok.pos
	p.next()
	if p.tok.nlBefore || p.yieldOperandAbsent() {
		return &yieldExpr{base{pos}, nil, false}
	}
	delegate := p.eat("*")
	return &yieldExpr{base{pos}, p.parseAssignment(noIn), delegate}
}

// yieldOperandAbsent reports a token after yield that ends the expression.
func (p *parser) yieldOperandAbsent() bool {
	switch p.tok.kind {
	case tEOF:
		return true
	case tPunct:
		switch p.tok.text {
		case ")", "]", "}", ",", ";", ":", "?", "=>":
			return true
		}
	case tKeyword:
		return p.tok.text == "in"
	}
	return false
}
