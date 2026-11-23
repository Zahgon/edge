package jsrt

// Parsing of classes, super, new.target and private names, with V8's early
// errors. All parts of a class, its name and heritage included, are
// strict-mode code.

// classScope is the private-name scope of a class body being parsed.
type classScope struct {
	// declared maps each private name the body declares to its declaration.
	declared map[string]privateDecl
	// refs are the private names the body uses; they are resolved when the
	// body ends, as a name may be used before its declaration.
	refs []privateRef
}

// privateDecl is how a private name is declared: a field, a method, or a
// getter and/or a setter.
type privateDecl struct {
	kind   classElementKind
	static bool
	// paired is set once a getter and a setter share the name.
	paired bool
}

// privateRef is a use of a private name, reported at pos when no enclosing
// class declares it.
type privateRef struct {
	name string
	pos  int
}

// parseClassDeclaration parses "class Name ... { ... }" as a statement, which
// declares Name as a lexical binding of the enclosing block.
func (p *parser) parseClassDeclaration() stmt {
	pos := p.tok.pos
	ci := p.parseClass(true)
	p.declareLexical(ci.name, "let")
	return &classDecl{base{pos}, ci}
}

// parseClass parses a class declaration or expression, from the keyword class
// to the closing brace of its body.
func (p *parser) parseClass(declaration bool) *classInfo {
	start := p.tok.pos
	scope := &classScope{declared: map[string]privateDecl{}}
	p.classes = append(p.classes, scope)
	p.next()
	ci := &classInfo{}
	if p.tok.kind == tIdent {
		if p.tok.text == "await" && p.fn.info.async {
			p.unexpected(p.tok)
		}
		p.checkBindingName(p.tok.text)
		ci.name = p.tok.text
		p.next()
	} else if declaration {
		p.unexpected(p.tok)
	}
	if p.eat("extends") {
		ci.heritage = p.parseLHS()
		if _, ok := ci.heritage.(*arrowParams); ok {
			p.unexpected(p.tok)
		}
	}
	p.expect("{")
	for !p.is("}") {
		if p.tok.kind == tEOF {
			p.unexpected(p.tok)
		}
		if p.eat(";") {
			continue
		}
		p.parseClassElement(ci, scope)
	}
	ci.source = p.lx.src[start:p.tok.end]
	p.classes = p.classes[:len(p.classes)-1]
	p.resolvePrivateNames(scope)
	// The token after the class is read with the strictness of the code
	// around it.
	p.next()
	if ci.ctor == nil {
		ci.ctor = &funcInfo{method: true, classCtor: true, derived: ci.heritage != nil, strict: true, simple: true, scope: &scopeInfo{}}
	}
	ci.ctor.name = ci.name
	ci.ctor.source = ci.source
	return ci
}

// resolvePrivateNames checks the private names a finished class body uses:
// those it does not declare belong to an enclosing class, and are an error
// when there is none.
func (p *parser) resolvePrivateNames(scope *classScope) {
	for _, ref := range scope.refs {
		if _, ok := scope.declared[ref.name]; ok {
			continue
		}
		if len(p.classes) == 0 {
			panic(&syntaxError{"Private field '" + ref.name + "' must be declared in an enclosing class", ref.pos})
		}
		outer := p.classes[len(p.classes)-1]
		outer.refs = append(outer.refs, ref)
	}
}

// usePrivateName records a use of the private name of the current token.
func (p *parser) usePrivateName() string {
	name := p.tok.text
	if len(p.classes) == 0 {
		p.fail("Private field '" + name + "' must be declared in an enclosing class")
	}
	scope := p.classes[len(p.classes)-1]
	scope.refs = append(scope.refs, privateRef{name, p.tok.pos})
	p.next()
	return name
}

// declarePrivateName records the declaration of a private name; only a getter
// and a setter (both static or both not) may share one.
func (p *parser) declarePrivateName(ci *classInfo, scope *classScope, name string, kind classElementKind, static bool) {
	if name == "#constructor" {
		p.fail("Classes may not have a field named 'constructor'")
	}
	prev, ok := scope.declared[name]
	if !ok {
		scope.declared[name] = privateDecl{kind: kind, static: static}
		ci.privateNames = append(ci.privateNames, name)
		return
	}
	accessors := (prev.kind == elemGetter && kind == elemSetter) || (prev.kind == elemSetter && kind == elemGetter)
	if !accessors || prev.paired || prev.static != static {
		p.fail(redeclared(name))
	}
	prev.paired = true
	scope.declared[name] = prev
}

// isClassElementNameEnd reports a token that ends a class element name, which
// makes a preceding static, async, get or set the element's name itself.
func isClassElementNameEnd(t token) bool {
	if t.kind == tEOF {
		return true
	}
	if t.kind != tPunct {
		return false
	}
	switch t.text {
	case "(", "=", ";", "}":
		return true
	}
	return false
}

// parseClassElement parses one method, accessor, field or static block.
func (p *parser) parseClassElement(ci *classInfo, scope *classScope) {
	static := false
	if p.isIdent("static") && !isClassElementNameEnd(p.peek()) {
		static = true
		p.next()
		if p.is("{") {
			ci.elements = append(ci.elements, &classElement{kind: elemStaticBlock, static: true, fn: p.parseStaticBlock()})
			return
		}
	}
	start := p.tok.pos
	kind := elemMethod
	async, generator := false, false
	if p.isIdent("async") {
		if t := p.peek(); !t.nlBefore && !isClassElementNameEnd(t) {
			async = true
			p.next()
		}
	}
	if p.is("*") {
		generator = true
		p.next()
	}
	if !async && !generator && (p.isIdent("get") || p.isIdent("set")) && !isClassElementNameEnd(p.peek()) {
		kind = elemGetter
		if p.tok.text == "set" {
			kind = elemSetter
		}
		p.next()
	}
	el := &classElement{kind: kind, static: static}
	if p.tok.kind == tPrivate {
		el.private = true
		el.key = p.tok.text
		p.next()
	} else {
		el.key, el.computed = p.parsePropertyName()
	}
	named := func(name string) bool { return !el.private && el.computed == nil && el.key == name }
	if static && named("prototype") {
		p.fail("Classes may not have a static property named 'prototype'")
	}
	if !p.is("(") {
		if kind != elemMethod || async || generator {
			p.unexpected(p.tok)
		}
		if named("constructor") {
			p.fail("Classes may not have a field named 'constructor'")
		}
		el.kind = elemField
		if el.private {
			p.declarePrivateName(ci, scope, el.key, elemField, static)
		}
		if p.eat("=") {
			el.fn = p.parseFieldInitializer()
		}
		p.semicolon()
		ci.elements = append(ci.elements, el)
		return
	}
	if !static && named("constructor") {
		switch {
		case kind != elemMethod:
			p.fail("Class constructor may not be an accessor")
		case generator:
			p.fail("Class constructor may not be a generator")
		case async:
			p.fail("Class constructor may not be an async method")
		case ci.ctor != nil:
			p.fail("A class may only have one constructor")
		}
		ci.ctor = p.parseFunctionTail(start, &funcInfo{method: true, classCtor: true, derived: ci.heritage != nil})
		ci.explicitCtor = true
		return
	}
	el.fn = p.parseFunctionTail(start, &funcInfo{name: el.key, method: true, async: async, generator: generator})
	if kind != elemMethod {
		p.checkAccessorParams(kind == elemGetter, el.fn)
	}
	if el.private {
		p.declarePrivateName(ci, scope, el.key, kind, static)
	}
	ci.elements = append(ci.elements, el)
}

// parseFieldInitializer parses the initializer after "field =": an expression
// evaluated as the body of a method of the instance (or of the class, for a
// static field), with this bound to it.
func (p *parser) parseFieldInitializer() *funcInfo {
	fi := &funcInfo{method: true, fieldInit: true, strict: true, simple: true}
	outer := p.fn
	p.fn = &funcCtx{info: fi, outer: outer, params: map[string]bool{}}
	p.pushBlock(true)
	fi.exprBody = p.parseAssignment(false)
	fi.scope = p.popBlock()
	p.fn = outer
	return fi
}

// parseStaticBlock parses "static { ... }" from its opening brace: a body run
// once, as a method of the class, when the class is defined.
func (p *parser) parseStaticBlock() *funcInfo {
	fi := &funcInfo{method: true, staticBlock: true, strict: true, simple: true}
	outer := p.fn
	p.fn = &funcCtx{info: fi, outer: outer, params: map[string]bool{}}
	p.expect("{")
	p.pushBlock(true)
	fi.body = p.parseStatementList("}")
	fi.scope = p.popBlock()
	p.expect("}")
	p.fn = outer
	return fi
}

// thisFunction returns the parsing state of the function whose this, super
// and new.target the current code sees: the nearest enclosing function that
// is not an arrow function.
func (p *parser) thisFunction() *funcCtx {
	ctx := p.fn
	for ctx != nil && ctx.info.arrow {
		ctx = ctx.outer
	}
	return ctx
}

// inClassInitializer reports code whose this is that of a class field
// initializer or static block, where arguments may not appear.
func (p *parser) inClassInitializer() bool {
	ctx := p.thisFunction()
	return ctx != nil && (ctx.info.fieldInit || ctx.info.staticBlock)
}

// parseSuper parses super(args) in a derived class constructor, or the super
// of super.x and super[x] in a method.
func (p *parser) parseSuper() expr {
	t := p.tok
	p.next()
	ctx := p.thisFunction()
	switch {
	case p.is("(") && ctx != nil && ctx.info.derived:
		return &superCall{base{t.pos}, p.parseArguments()}
	case (p.is(".") || p.is("[")) && ctx != nil && ctx.info.method:
		return &superRef{base{t.pos}}
	}
	p.failAt(t, "'super' keyword unexpected here")
	return nil
}

// parseNewTarget parses new.target after the keyword new.
func (p *parser) parseNewTarget(pos int) expr {
	p.next()
	if !p.isIdent("target") {
		p.unexpected(p.tok)
	}
	if p.thisFunction() == nil {
		p.fail("new.target expression is not allowed here")
	}
	p.next()
	return &newTarget{base{pos}}
}

// parsePrivateIn parses the private name of "#x in obj"; the binary
// expression parser supplies obj.
func (p *parser) parsePrivateIn() expr {
	pos := p.tok.pos
	if t := p.peek(); len(p.classes) > 0 && (t.kind != tKeyword || t.text != "in") {
		p.fail("Unexpected identifier '" + p.tok.text + "'")
	}
	return &privateIn{base{pos}, p.usePrivateName(), nil}
}

// checkOperand rejects a "#x" left without its "in obj", as the operand of
// an operator that binds more tightly than in.
func (p *parser) checkOperand(x expr) {
	if pi, ok := x.(*privateIn); ok && pi.obj == nil {
		panic(&syntaxError{"Unexpected identifier '" + pi.name + "'", pi.pos})
	}
}
