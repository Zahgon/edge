package jsrt

import (
	"unicode/utf8"
)

// Parsing of async functions. async is not a reserved word: it introduces an
// async function only when "function", a parameter or a method name follows it
// on the same line, and is an ordinary identifier otherwise.

// isAsyncFunction reports "async function" with no line break between the two
// words, the start of an async function declaration or expression.
func (p *parser) isAsyncFunction() bool {
	if !p.isIdent("async") {
		return false
	}
	t := p.peek()
	return t.kind == tKeyword && t.text == "function" && !t.escaped && !t.nlBefore
}

// awaitOutsideAsync reports that the token before the current one is the
// identifier await in a function that is not async. V8 names the misuse when a
// statement cannot end after it ("await x" in a plain function).
func (p *parser) awaitOutsideAsync() bool {
	const word = "await"
	start := p.prevEnd - len(word)
	if p.fn.info.async || start < 0 || p.lx.src[start:p.prevEnd] != word {
		return false
	}
	before, _ := utf8.DecodeLastRuneInString(p.lx.src[:start])
	return start == 0 || (!isIDPart(before) && before != '\\')
}

// parseAsyncPrimary parses what follows the identifier async at the start of a
// primary expression: an async function expression, or the parenthesized
// parameters of an async arrow function. It returns nil, having consumed nothing,
// when async is a plain identifier (a variable, or a call of a function named
// async).
func (p *parser) parseAsyncPrimary() expr {
	pos := p.tok.pos
	next := p.peek()
	if next.nlBefore {
		return nil
	}
	switch {
	case next.kind == tKeyword && next.text == "function" && !next.escaped:
		p.next()
		p.next()
		generator := p.eat("*")
		name := ""
		if p.tok.kind == tIdent {
			if p.tok.text == "await" {
				p.unexpected(p.tok)
			}
			name = p.tok.text
			p.next()
		}
		return &funcLit{base{pos}, p.parseFunctionTail(pos, &funcInfo{name: name, async: true, generator: generator})}
	case next.kind == tPunct && next.text == "(":
		// "async (a, b)" is a call unless "=>" follows the parentheses, so the
		// list is first tried as arrow parameters.
		s := p.save()
		p.next()
		var x expr
		if err := p.tryParse(func() { x = p.parseParenthesized() }); err == nil {
			if ap, ok := x.(*arrowParams); ok {
				ap.pos, ap.async = pos, true
				return ap
			}
		}
		p.restore(s)
	}
	return nil
}

// parseAsyncArrowIdent parses "async x => body", an async arrow function with a
// single parameter written without parentheses.
func (p *parser) parseAsyncArrowIdent() expr {
	pos := p.tok.pos
	p.next()
	param := p.tok
	if param.text == "await" {
		if p.fn.info.async {
			p.unexpected(param)
		}
		p.fail("'await' is not a valid identifier name in an async function")
	}
	p.next()
	if !p.is("=>") {
		p.unexpected(p.tok)
	}
	return p.parseArrowFunction(pos, []*patternElem{{target: &ident{base{param.pos}, param.text}}}, nil, true)
}

// parseAsyncMethod parses "async name(params) { body }", or the async
// generator method "async *name(params) { body }", in an object literal.
func (p *parser) parseAsyncMethod() *property {
	pos := p.tok.pos
	p.next()
	generator := p.eat("*")
	key, computed := p.parsePropertyName()
	if !p.is("(") {
		p.unexpected(p.tok)
	}
	fi := p.parseFunctionTail(pos, &funcInfo{name: key, method: true, async: true, generator: generator})
	return &property{kind: propMethod, key: key, computed: computed, value: &funcLit{base{pos}, fi}, pos: pos}
}
