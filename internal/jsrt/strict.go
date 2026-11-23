package jsrt

// Strict-mode parsing: the directive prologue and the early errors that apply
// only to strict-mode code, with V8's messages.

// strictReservedWords are the identifiers strict-mode code reserves.
var strictReservedWords = map[string]bool{
	"implements": true, "interface": true, "let": true, "package": true, "private": true,
	"protected": true, "public": true, "static": true, "yield": true,
}

// strict reports whether the code being parsed is strict-mode code: code in
// a function with a "use strict" directive (or inside one), and all class code.
func (p *parser) strict() bool { return len(p.classes) > 0 || p.fn != nil && p.fn.info.strict }

// checkBindingName applies the strict-mode restrictions on a name being declared
// (a variable, parameter, function or catch parameter).
func (p *parser) checkBindingName(name string) {
	if !p.strict() {
		return
	}
	switch {
	case name == "eval" || name == "arguments":
		p.fail("Unexpected eval or arguments in strict mode")
	case strictReservedWords[name]:
		p.fail("Unexpected strict mode reserved word")
	}
}

// checkAssignedName rejects eval and arguments as the target of an assignment,
// increment or decrement in strict-mode code.
func (p *parser) checkAssignedName(x node) {
	if id, ok := x.(*ident); ok && p.strict() && (id.name == "eval" || id.name == "arguments") {
		p.fail("Unexpected eval or arguments in strict mode")
	}
}

// checkStrictToken reports the current token when strict-mode code may not
// contain it: a legacy octal literal or escape, or a reserved word used as an
// identifier.
func (p *parser) checkStrictToken() {
	if !p.strict() {
		return
	}
	if p.tok.strictErr != "" {
		p.fail(p.tok.strictErr)
	}
}

// functionDeclarationMisplaced is the SyntaxError for a function declaration as
// the body of a loop, label or (in strict mode) if statement.
func (p *parser) functionDeclarationMisplaced() {
	if p.strict() {
		p.fail("In strict mode code, functions can only be declared at top level or inside a block.")
	}
	p.fail("In non-strict mode code, functions can only be declared at top level, inside a block, or as the body of an if statement.")
}

// parseDirectivePrologue parses the directive prologue at the start of a
// function body: the leading statements that are a lone string literal. A
// "use strict" directive (written without escapes) makes fi strict-mode code,
// which retroactively applies the strict-mode rules to the directives before it,
// the function's name and its parameters. It returns the prologue's statements.
func (p *parser) parseDirectivePrologue(fi *funcInfo) []stmt {
	var list []stmt
	var directives []token
	for p.tok.kind == tString {
		t := p.tok
		st := p.parseStatementListItem()
		list = append(list, st)
		es, ok := st.(*exprStmt)
		if !ok {
			break
		}
		if lit, ok := es.x.(*stringLit); !ok || lit.pos != t.pos {
			break
		}
		directives = append(directives, t)
		raw := p.lx.src[t.pos:t.end]
		if raw != `"use strict"` && raw != `'use strict'` {
			continue
		}
		if !fi.simple {
			p.failAt(t, "Illegal 'use strict' directive in function with non-simple parameter list")
		}
		if !fi.strict {
			fi.strict = true
			p.checkStrictFunction(fi)
			for _, d := range directives {
				if d.strictErr != "" {
					p.failAt(d, d.strictErr)
				}
			}
			p.checkStrictToken()
		}
	}
	return list
}

// checkStrictFunction applies the strict-mode rules to the name and parameters
// of a function whose own directive made it strict.
func (p *parser) checkStrictFunction(fi *funcInfo) {
	var names []string
	for _, el := range fi.params {
		names = boundNames(el.target, names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			p.fail("Duplicate parameter name not allowed in this context")
		}
		seen[n] = true
	}
	for _, n := range names {
		p.checkBindingName(n)
	}
	if fi.name != "" && !fi.method {
		p.checkBindingName(fi.name)
	}
}
