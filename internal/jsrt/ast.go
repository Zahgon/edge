package jsrt

import "github.com/edge-js/edge-go/js"

// The syntax tree the parser builds and the interpreter walks. Every node records
// the byte offset where it starts, so errors can be attributed, and function and
// class nodes keep their source text for Function.prototype.toString.

type node interface{ position() int }

type expr interface {
	node
	isExpr()
}

type stmt interface {
	node
	isStmt()
}

type base struct{ pos int }

func (b base) position() int { return b.pos }

// Expressions.
type (
	numberLit struct {
		base
		value float64
	}
	// bigintLit is a BigInt literal such as 0x1fn.
	bigintLit struct {
		base
		value js.BigInt
	}
	stringLit struct {
		base
		value string
	}
	boolLit struct {
		base
		value bool
	}
	nullLit       struct{ base }
	thisExpr      struct{ base }
	templateChunk struct {
		cooked   string
		cookedOK bool
		raw      string
	}
	templateLit struct {
		base
		chunks []templateChunk
		exprs  []expr
	}
	taggedTemplate struct {
		base
		tag   expr
		quasi *templateLit
	}
	regexpLit struct {
		base
		pattern, flags string
	}
	ident struct {
		base
		name string
	}
	// arrayLit elements are nil for holes. parens marks a parenthesized literal,
	// which cannot become a destructuring pattern.
	arrayLit struct {
		base
		elems  []expr
		parens bool
	}
	objectLit struct {
		base
		props  []*property
		parens bool
	}
	// invalidTarget is a call used as an assignment target, which V8 accepts when
	// parsing and rejects with a ReferenceError when evaluated.
	invalidTarget struct {
		base
		call *call
		msg  string
	}
	funcLit struct {
		base
		fn *funcInfo
	}
	member struct {
		base
		// obj is the object, or a *superRef for super.x and super[x].
		obj expr
		// name is the property for dot access; key is the computed key otherwise.
		// A private member (o.#x) has its private name, with the #, in name.
		name     string
		key      expr
		optional bool
		private  bool
	}
	// superRef is the super of super.x or super[x] in a method: the prototype
	// of the method's home object.
	superRef struct{ base }
	// superCall is super(args) in a derived class constructor.
	superCall struct {
		base
		args []expr
	}
	// newTarget is new.target.
	newTarget struct{ base }
	// privateIn is "#x in obj".
	privateIn struct {
		base
		name string
		obj  expr
	}
	classLit struct {
		base
		cls *classInfo
	}
	call struct {
		base
		callee   expr
		args     []expr
		optional bool
	}
	// optionalChain marks the end of an optional chain: a short circuit anywhere
	// inside yields undefined for the whole chain.
	optionalChain struct {
		base
		x expr
	}
	newExpr struct {
		base
		callee expr
		args   []expr
	}
	unary struct {
		base
		op string
		x  expr
	}
	update struct {
		base
		op     string
		prefix bool
		x      expr
	}
	binary struct {
		base
		op   string
		l, r expr
	}
	logical struct {
		base
		op   string
		l, r expr
		// parens records a parenthesized operand, which may be mixed with ??.
		parens bool
	}
	conditional struct {
		base
		test, cons, alt expr
	}
	assign struct {
		base
		op     string
		target node // an expr (identifier or member) or a pattern for "="
		value  expr
	}
	sequence struct {
		base
		list []expr
	}
	// yieldExpr is yield or, with delegate, yield* in a generator body; arg is
	// nil for a bare yield.
	yieldExpr struct {
		base
		arg      expr
		delegate bool
	}
	spread struct {
		base
		x expr
	}
)

// propKind is the kind of an object literal entry.
type propKind int

const (
	propInit propKind = iota
	propShorthand
	propMethod
	propGet
	propSet
	propSpread
	// propProto is "__proto__: value", which sets the prototype.
	propProto
)

type property struct {
	kind     propKind
	key      string // a non-computed key, already converted to its property name
	computed expr
	value    expr
	pos      int
}

// Patterns (binding and assignment targets).
type (
	patternElem struct {
		target node // ident, member, arrayPattern or objectPattern
		def    expr
	}
	arrayPattern struct {
		base
		elems []*patternElem // nil for elisions
		rest  node
	}
	patternProp struct {
		key      string
		computed expr
		elem     patternElem
	}
	objectPattern struct {
		base
		props []*patternProp
		rest  node
	}
)

// Statements.
type (
	varDecl struct {
		base
		kind  string // var, let or const
		decls []*declarator
	}
	declarator struct {
		target node
		init   expr
	}
	funcDecl struct {
		base
		fn *funcInfo
	}
	classDecl struct {
		base
		cls *classInfo
	}
	exprStmt struct {
		base
		x expr
	}
	block struct {
		base
		body  []stmt
		scope *scopeInfo
	}
	ifStmt struct {
		base
		test      expr
		cons, alt stmt
	}
	forStmt struct {
		base
		init   node // *varDecl, expr or nil
		test   expr
		update expr
		body   stmt
		scope  *scopeInfo
	}
	forInOf struct {
		base
		of bool
		// await marks for await (... of ...), which iterates an async iterator
		// (or a sync one through async-from-sync iteration) and awaits each
		// result.
		await bool
		left  node // *varDecl with one declarator, or an assignment target
		right expr
		body  stmt
		scope *scopeInfo
	}
	whileStmt struct {
		base
		test expr
		body stmt
	}
	doWhile struct {
		base
		body stmt
		test expr
	}
	breakStmt struct {
		base
		label string
	}
	continueStmt struct {
		base
		label string
	}
	returnStmt struct {
		base
		x expr
	}
	throwStmt struct {
		base
		x expr
	}
	tryStmt struct {
		base
		body       *block
		param      node
		handler    *block
		finalizer  *block
		catchScope *scopeInfo
	}
	switchCase struct {
		test expr // nil for default
		body []stmt
	}
	switchStmt struct {
		base
		disc  expr
		cases []*switchCase
		scope *scopeInfo
	}
	labeled struct {
		base
		label string
		body  stmt
	}
	withStmt struct {
		base
		obj  expr
		body stmt
	}
	emptyStmt struct{ base }
)

// scopeInfo lists the declarations a block (or function body) instantiates when it
// is entered: lexical bindings in TDZ and hoisted function declarations.
type scopeInfo struct {
	lets   []string
	consts []string
	funcs  []*funcInfo
}

// funcInfo is a parsed function: declaration, expression, arrow or method.
type funcInfo struct {
	name     string
	params   []*patternElem
	rest     node
	body     []stmt
	exprBody expr // an arrow's concise body
	arrow    bool
	method   bool
	// async marks an async function, arrow or method: a call runs the body as a
	// coroutine and returns a promise, and await is an operator inside it.
	async bool
	// generator marks a generator function or method: a call returns a
	// generator object whose body runs as a coroutine, and yield is an operator
	// inside it. With async it is an async generator.
	generator bool
	// classCtor marks the constructor of a class, derived that of a class with
	// an extends clause, where super(...) may be called.
	classCtor, derived bool
	// fieldInit marks the initializer of a class field, staticBlock a class
	// static block; both run as methods of the instance or the class, and
	// arguments may not appear in them.
	fieldInit, staticBlock bool
	// annexB marks a sloppy-mode function declared in a block that is also
	// bound in the enclosing function's var scope (Annex B.3.2.1).
	annexB bool
	// strict marks strict-mode code: a "use strict" directive in the body or
	// in an enclosing function.
	strict bool
	// simple reports a parameter list of plain identifiers only.
	simple bool
	// length is the number of parameters before the first default or rest.
	length int
	// varNames are the var-scoped names declared in the body (including Annex B
	// block functions).
	varNames []string
	// scope holds the body's top-level lexical declarations and functions.
	scope  *scopeInfo
	source string
}

func (*numberLit) isExpr()      {}
func (*bigintLit) isExpr()      {}
func (*invalidTarget) isExpr()  {}
func (*stringLit) isExpr()      {}
func (*boolLit) isExpr()        {}
func (*nullLit) isExpr()        {}
func (*thisExpr) isExpr()       {}
func (*templateLit) isExpr()    {}
func (*taggedTemplate) isExpr() {}
func (*regexpLit) isExpr()      {}
func (*ident) isExpr()          {}
func (*arrayLit) isExpr()       {}
func (*objectLit) isExpr()      {}
func (*funcLit) isExpr()        {}
func (*classLit) isExpr()       {}
func (*superRef) isExpr()       {}
func (*superCall) isExpr()      {}
func (*newTarget) isExpr()      {}
func (*privateIn) isExpr()      {}
func (*member) isExpr()         {}
func (*call) isExpr()           {}
func (*optionalChain) isExpr()  {}
func (*newExpr) isExpr()        {}
func (*unary) isExpr()          {}
func (*yieldExpr) isExpr()      {}
func (*update) isExpr()         {}
func (*binary) isExpr()         {}
func (*logical) isExpr()        {}
func (*conditional) isExpr()    {}
func (*assign) isExpr()         {}
func (*sequence) isExpr()       {}
func (*spread) isExpr()         {}
func (*arrayPattern) isExpr()   {}
func (*objectPattern) isExpr()  {}

func (*varDecl) isStmt()      {}
func (*funcDecl) isStmt()     {}
func (*classDecl) isStmt()    {}
func (*exprStmt) isStmt()     {}
func (*block) isStmt()        {}
func (*ifStmt) isStmt()       {}
func (*forStmt) isStmt()      {}
func (*forInOf) isStmt()      {}
func (*whileStmt) isStmt()    {}
func (*doWhile) isStmt()      {}
func (*breakStmt) isStmt()    {}
func (*continueStmt) isStmt() {}
func (*returnStmt) isStmt()   {}
func (*throwStmt) isStmt()    {}
func (*tryStmt) isStmt()      {}
func (*switchStmt) isStmt()   {}
func (*labeled) isStmt()      {}
func (*withStmt) isStmt()     {}
func (*emptyStmt) isStmt()    {}

// classInfo is a parsed class: declaration or expression.
type classInfo struct {
	name string
	// heritage is the expression of the extends clause, nil without one.
	heritage expr
	// ctor is the constructor: the class's own, or the default one, whose
	// body is empty (and calls super with the arguments in a derived class).
	ctor *funcInfo
	// explicitCtor reports a constructor written in the class body.
	explicitCtor bool
	elements     []*classElement
	// privateNames lists the private names the class body declares.
	privateNames []string
	source       string
}

// classElementKind is the kind of a class element.
type classElementKind uint8

const (
	elemMethod classElementKind = iota
	elemGetter
	elemSetter
	elemField
	elemStaticBlock
)

// classElement is a method, accessor, field or static block of a class body.
type classElement struct {
	kind   classElementKind
	static bool
	// key is the property name of a non-computed key, the private name (with
	// its #) when private is set; computed is the expression of a computed key.
	key      string
	private  bool
	computed expr
	// fn is the method or accessor, the field's initializer (an expression
	// body, nil for a field without one) or the static block's body.
	fn *funcInfo
}
