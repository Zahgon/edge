// Package astring is a Go port of astring 1.9.0, the JavaScript code generator
// edge-parser uses to turn the ESTree nodes of package acorn back into source text.
//
// Generate is astring's generate(node) with its default options: two-space
// indentation, "\n" line ends, no comments and no source map. The output is the
// same text, byte for byte, for every node type acorn produces from a script,
// including astring's parenthesization by operator precedence, Literal nodes
// printed from their raw text when they have one, and nodes built by hand without
// raw text or locations. Where astring throws (a node type it has no generator
// for, a missing child node, a Literal value JSON.stringify rejects), Generate
// returns the same TypeError.
//
// astring is MIT licensed: Copyright (c) 2015, David Bonnet <david@bonnet.cc>. The
// permission notice of the MIT License applies to this port as to the original.
package astring

import (
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// operatorPrecedence orders the binary and logical operators.
var operatorPrecedence = map[string]int{
	"||": 2, "??": 3, "&&": 4, "|": 5, "^": 6, "&": 7,
	"==": 8, "!=": 8, "===": 8, "!==": 8,
	"<": 9, ">": 9, "<=": 9, ">=": 9, "in": 9, "instanceof": 9,
	"<<": 10, ">>": 10, ">>>": 10,
	"+": 11, "-": 11,
	"*": 12, "%": 12, "/": 12,
	"**": 13,
}

// needsParentheses is the precedence of expressions that are always wrapped in
// parentheses when they are operands.
const needsParentheses = 17

// expressionsPrecedence is astring's EXPRESSIONS_PRECEDENCE. Node types missing
// from it have an undefined precedence, which compares false with every number.
var expressionsPrecedence = map[string]int{
	"ArrayExpression":          20,
	"TaggedTemplateExpression": 20,
	"ThisExpression":           20,
	"Identifier":               20,
	"PrivateIdentifier":        20,
	"Literal":                  18,
	"TemplateLiteral":          20,
	"Super":                    20,
	"SequenceExpression":       20,
	"MemberExpression":         19,
	"ChainExpression":          19,
	"CallExpression":           19,
	"NewExpression":            19,
	"ArrowFunctionExpression":  needsParentheses,
	"ClassExpression":          needsParentheses,
	"FunctionExpression":       needsParentheses,
	"ObjectExpression":         needsParentheses,
	"UpdateExpression":         16,
	"UnaryExpression":          15,
	"AwaitExpression":          15,
	"BinaryExpression":         14,
	"LogicalExpression":        13,
	"ConditionalExpression":    4,
	"AssignmentExpression":     3,
	"YieldExpression":          2,
	"RestElement":              1,
}

const (
	indent  = "  "
	lineEnd = "\n"
)

// Generate returns the JavaScript source text of node as astring's
// generate(node) writes it. node is a tree of ESTree nodes, such as acorn
// produces from a script or a subtree of one. The error is the TypeError
// astring throws for a tree it cannot print.
func Generate(node *acorn.Node) (code string, err error) {
	s := &state{}
	defer func() {
		if r := recover(); r != nil {
			thrown, ok := r.(generateError)
			if !ok {
				panic(r)
			}
			code, err = "", thrown.err
		}
	}()
	s.dispatch(node, "state.generator", "node")
	return s.out.String(), nil
}

// generateError carries what a generator throws up to Generate, which returns
// it: the generators write straight into the output, as astring's do, and a
// throw abandons the whole text.
type generateError struct{ err error }

// state is astring's State: the output and the current indentation level.
type state struct {
	out         strings.Builder
	indentLevel int
}

func (s *state) write(code string) { s.out.WriteString(code) }

// fail throws err out of Generate.
func (s *state) fail(err error) { panic(generateError{err}) }

// generatorFunc writes one node type, as a method of astring's GENERATOR.
type generatorFunc func(s *state, node *acorn.Node)

// generator is astring's GENERATOR: the function that writes each node type.
var generator map[string]generatorFunc

func init() {
	generator = map[string]generatorFunc{
		"Program":                  (*state).program,
		"BlockStatement":           (*state).block,
		"StaticBlock":              (*state).staticBlock,
		"EmptyStatement":           func(s *state, _ *acorn.Node) { s.write(";") },
		"ExpressionStatement":      (*state).expressionStatement,
		"IfStatement":              (*state).ifStatement,
		"LabeledStatement":         (*state).labeledStatement,
		"BreakStatement":           func(s *state, node *acorn.Node) { s.jump("break", node) },
		"ContinueStatement":        func(s *state, node *acorn.Node) { s.jump("continue", node) },
		"WithStatement":            (*state).withStatement,
		"SwitchStatement":          (*state).switchStatement,
		"ReturnStatement":          (*state).returnStatement,
		"ThrowStatement":           (*state).throwStatement,
		"TryStatement":             (*state).tryStatement,
		"WhileStatement":           (*state).whileStatement,
		"DoWhileStatement":         (*state).doWhileStatement,
		"ForStatement":             (*state).forStatement,
		"ForInStatement":           (*state).forInOfStatement,
		"ForOfStatement":           (*state).forInOfStatement,
		"DebuggerStatement":        func(s *state, _ *acorn.Node) { s.write("debugger;") },
		"FunctionDeclaration":      (*state).function,
		"FunctionExpression":       (*state).function,
		"VariableDeclaration":      (*state).variableDeclaration,
		"VariableDeclarator":       (*state).variableDeclarator,
		"ClassDeclaration":         (*state).class,
		"ClassExpression":          (*state).class,
		"ClassBody":                (*state).block,
		"ImportDeclaration":        (*state).importDeclaration,
		"ImportAttribute":          (*state).importAttribute,
		"ImportExpression":         (*state).importExpression,
		"ExportDefaultDeclaration": (*state).exportDefaultDeclaration,
		"ExportNamedDeclaration":   (*state).exportNamedDeclaration,
		"ExportAllDeclaration":     (*state).exportAllDeclaration,
		"MethodDefinition":         (*state).methodDefinition,
		"PropertyDefinition":       (*state).propertyDefinition,
		"ArrowFunctionExpression":  (*state).arrowFunction,
		"ThisExpression":           func(s *state, _ *acorn.Node) { s.write("this") },
		"Super":                    func(s *state, _ *acorn.Node) { s.write("super") },
		"RestElement":              (*state).spread,
		"SpreadElement":            (*state).spread,
		"YieldExpression":          (*state).yieldExpression,
		"AwaitExpression":          (*state).awaitExpression,
		"TemplateLiteral":          (*state).templateLiteral,
		"TemplateElement":          func(s *state, node *acorn.Node) { s.write(node.TemplateValue.Raw) },
		"TaggedTemplateExpression": (*state).taggedTemplate,
		"ArrayExpression":          (*state).arrayExpression,
		"ArrayPattern":             (*state).arrayExpression,
		"ObjectExpression":         (*state).objectExpression,
		"Property":                 (*state).property,
		"ObjectPattern":            (*state).objectPattern,
		"SequenceExpression":       func(s *state, node *acorn.Node) { s.formatSequence(node.Expressions) },
		"UnaryExpression":          (*state).unaryExpression,
		"UpdateExpression":         (*state).updateExpression,
		"AssignmentExpression":     (*state).assignment,
		"AssignmentPattern":        (*state).assignment,
		"BinaryExpression":         (*state).binaryExpression,
		"LogicalExpression":        (*state).binaryExpression,
		"ConditionalExpression":    (*state).conditionalExpression,
		"NewExpression":            (*state).newExpression,
		"CallExpression":           (*state).callExpression,
		"ChainExpression":          func(s *state, node *acorn.Node) { s.node(node.Expression, "node.expression") },
		"MemberExpression":         (*state).memberExpression,
		"MetaProperty":             func(s *state, node *acorn.Node) { s.write(node.Meta.Name + "." + node.Property.Name) },
		"Identifier":               func(s *state, node *acorn.Node) { s.write(node.Name) },
		"PrivateIdentifier":        func(s *state, node *acorn.Node) { s.write("#" + node.Name) },
		"Literal":                  (*state).literal,
		"RegExpLiteral":            (*state).regExpLiteral,
	}
}

// node writes child as astring's `this[<at>.type](<at>, state)` does, where at
// is the JavaScript expression the generator reads the child from (such as
// "node.test"). V8 names that expression in the TypeError astring throws when
// the child is missing or of a type without a generator.
func (s *state) node(child *acorn.Node, at string) {
	s.dispatch(child, "this", at)
}

// dispatch writes node as `<table>[<at>.type](<at>, state)`.
func (s *state) dispatch(node *acorn.Node, table, at string) {
	gen, ok := generator[s.typeOf(node)]
	if !ok {
		s.fail(js.NotAFunction(table + "[" + at + ".type]"))
	}
	gen(s, node)
}

// typeOf reads node.type, which throws when the node is missing.
func (s *state) typeOf(node *acorn.Node) string {
	if node == nil {
		s.fail(js.CannotReadProperties(js.Undefined, "type"))
	}
	return node.Type
}

// formatSequence writes nodes in parentheses, separated by commas.
func (s *state) formatSequence(nodes []*acorn.Node) {
	s.write("(")
	for i, n := range nodes {
		if i == 0 {
			s.dispatch(n, "generator", "nodes[0]")
			continue
		}
		s.write(", ")
		s.dispatch(n, "generator", "param")
	}
	s.write(")")
}

// expressionNeedsParenthesis tells whether node, an operand of parent, must be
// wrapped in parentheses; isRightHand is set for the right operand of a binary
// operator.
func (s *state) expressionNeedsParenthesis(node, parent *acorn.Node, isRightHand bool) bool {
	nodePrecedence, nodeKnown := expressionsPrecedence[s.typeOf(node)]
	if nodeKnown && nodePrecedence == needsParentheses {
		return true
	}
	parentPrecedence, parentKnown := expressionsPrecedence[parent.Type]
	if nodeKnown != parentKnown || nodePrecedence != parentPrecedence {
		return nodeKnown && parentKnown &&
			(!isRightHand && nodePrecedence == 15 && parentPrecedence == 14 && parent.Operator == "**" ||
				nodePrecedence < parentPrecedence)
	}
	if !nodeKnown || nodePrecedence != 13 && nodePrecedence != 14 {
		// Not a binary or logical expression.
		return false
	}
	if node.Operator == "**" && parent.Operator == "**" {
		// Exponentiation is right-associative.
		return !isRightHand
	}
	if nodePrecedence == 13 && parentPrecedence == 13 && (node.Operator == "??" || parent.Operator == "??") {
		// Mixing ?? with || or && requires parentheses.
		return true
	}
	nodeOperator, nodeOperatorKnown := operatorPrecedence[node.Operator]
	parentOperator, parentOperatorKnown := operatorPrecedence[parent.Operator]
	if !nodeOperatorKnown || !parentOperatorKnown {
		return false
	}
	if isRightHand {
		return nodeOperator <= parentOperator
	}
	return nodeOperator < parentOperator
}

// formatExpression writes node, an operand of parent, with the parentheses its
// precedence requires.
func (s *state) formatExpression(node, parent *acorn.Node, isRightHand bool) {
	if s.expressionNeedsParenthesis(node, parent, isRightHand) {
		s.write("(")
		s.dispatch(node, "generator", "node")
		s.write(")")
		return
	}
	s.dispatch(node, "generator", "node")
}

// formatParenthesizedIf writes child (read from at), wrapped in parentheses when
// wrap is set.
func (s *state) formatParenthesizedIf(child *acorn.Node, at string, wrap bool) {
	if wrap {
		s.write("(")
		s.node(child, at)
		s.write(")")
		return
	}
	s.node(child, at)
}

// precedenceBelow tells whether node needs parentheses as the operand of an
// expression of precedence level: it always does, or its precedence is lower.
func (s *state) precedenceBelow(node *acorn.Node, level int) bool {
	precedence, known := expressionsPrecedence[s.typeOf(node)]
	return known && (precedence == needsParentheses || precedence < level)
}

// hasCallExpression tells whether node is a call, or member accesses on a call,
// which the callee of a new expression must wrap in parentheses.
func hasCallExpression(node *acorn.Node) bool {
	for {
		switch node.Type {
		case "CallExpression":
			return true
		case "MemberExpression":
			node = node.Object
		default:
			return false
		}
	}
}

// statements writes a statement list, each statement on its own line.
func (s *state) statements(list []*acorn.Node, statementIndent string) {
	for _, statement := range list {
		s.write(statementIndent)
		s.node(statement, "statement")
		s.write(lineEnd)
	}
}

func (s *state) program(node *acorn.Node) {
	s.statements(node.BodyList, strings.Repeat(indent, s.indentLevel))
}

// block writes the braced statement list of a BlockStatement or ClassBody.
func (s *state) block(node *acorn.Node) {
	blockIndent := strings.Repeat(indent, s.indentLevel)
	s.indentLevel++
	s.write("{")
	if len(node.BodyList) > 0 {
		s.write(lineEnd)
		s.statements(node.BodyList, blockIndent+indent)
		s.write(blockIndent)
	}
	s.write("}")
	s.indentLevel--
}

func (s *state) staticBlock(node *acorn.Node) {
	s.write("static ")
	s.block(node)
}

func (s *state) expressionStatement(node *acorn.Node) {
	expression := node.Expression
	precedence, known := expressionsPrecedence[s.typeOf(expression)]
	wrap := known && precedence == needsParentheses
	if known && precedence == expressionsPrecedence["AssignmentExpression"] {
		wrap = strings.HasPrefix(s.typeOf(expression.Left), "O")
	}
	s.formatParenthesizedIf(expression, "node.expression", wrap)
	s.write(";")
}

func (s *state) ifStatement(node *acorn.Node) {
	s.write("if (")
	s.node(node.Test, "node.test")
	s.write(") ")
	s.node(node.Consequent, "node.consequent")
	if node.Alternate != nil {
		s.write(" else ")
		s.node(node.Alternate, "node.alternate")
	}
}

func (s *state) labeledStatement(node *acorn.Node) {
	s.node(node.Label, "node.label")
	s.write(": ")
	s.node(node.Body, "node.body")
}

// jump writes a break or continue statement.
func (s *state) jump(keyword string, node *acorn.Node) {
	s.write(keyword)
	if node.Label != nil {
		s.write(" ")
		s.node(node.Label, "node.label")
	}
	s.write(";")
}

func (s *state) withStatement(node *acorn.Node) {
	s.write("with (")
	s.node(node.Object, "node.object")
	s.write(") ")
	s.node(node.Body, "node.body")
}

func (s *state) switchStatement(node *acorn.Node) {
	switchIndent := strings.Repeat(indent, s.indentLevel)
	s.indentLevel += 2
	caseIndent := switchIndent + indent
	s.write("switch (")
	s.node(node.Discriminant, "node.discriminant")
	s.write(") {" + lineEnd)
	for _, occurence := range node.Cases {
		if occurence.Test != nil {
			s.write(caseIndent + "case ")
			s.node(occurence.Test, "occurence.test")
			s.write(":" + lineEnd)
		} else {
			s.write(caseIndent + "default:" + lineEnd)
		}
		s.statements(occurence.ConsequentList, caseIndent+indent)
	}
	s.indentLevel -= 2
	s.write(switchIndent + "}")
}

func (s *state) returnStatement(node *acorn.Node) {
	s.write("return")
	if node.Argument != nil {
		s.write(" ")
		s.node(node.Argument, "node.argument")
	}
	s.write(";")
}

func (s *state) throwStatement(node *acorn.Node) {
	s.write("throw ")
	s.node(node.Argument, "node.argument")
	s.write(";")
}

func (s *state) tryStatement(node *acorn.Node) {
	s.write("try ")
	s.node(node.Block, "node.block")
	if handler := node.Handler; handler != nil {
		if handler.Param == nil {
			s.write(" catch ")
		} else {
			s.write(" catch (")
			s.node(handler.Param, "handler.param")
			s.write(") ")
		}
		s.node(handler.Body, "handler.body")
	}
	if node.Finalizer != nil {
		s.write(" finally ")
		s.node(node.Finalizer, "node.finalizer")
	}
}

func (s *state) whileStatement(node *acorn.Node) {
	s.write("while (")
	s.node(node.Test, "node.test")
	s.write(") ")
	s.node(node.Body, "node.body")
}

func (s *state) doWhileStatement(node *acorn.Node) {
	s.write("do ")
	s.node(node.Body, "node.body")
	s.write(" while (")
	s.node(node.Test, "node.test")
	s.write(");")
}

func (s *state) forStatement(node *acorn.Node) {
	s.write("for (")
	if init := node.Init; init != nil {
		if init.Type == "VariableDeclaration" {
			s.formatVariableDeclaration(init)
		} else {
			s.node(init, "init")
		}
	}
	s.write("; ")
	if node.Test != nil {
		s.node(node.Test, "node.test")
	}
	s.write("; ")
	if node.Update != nil {
		s.node(node.Update, "node.update")
	}
	s.write(") ")
	s.node(node.Body, "node.body")
}

// forInOfStatement writes a for-in, for-of or for-await-of statement.
func (s *state) forInOfStatement(node *acorn.Node) {
	if node.Await {
		s.write("for await (")
	} else {
		s.write("for (")
	}
	if left := node.Left; s.typeOf(left) == "VariableDeclaration" {
		s.formatVariableDeclaration(left)
	} else {
		s.node(left, "left")
	}
	if node.Type == "ForInStatement" {
		s.write(" in ")
	} else {
		s.write(" of ")
	}
	s.node(node.Right, "node.right")
	s.write(") ")
	s.node(node.Body, "node.body")
}

// function writes a function declaration or expression.
func (s *state) function(node *acorn.Node) {
	if node.Async {
		s.write("async ")
	}
	if node.Generator {
		s.write("function* ")
	} else {
		s.write("function ")
	}
	if node.Id != nil {
		s.write(node.Id.Name)
	}
	s.formatSequence(node.Params)
	s.write(" ")
	s.node(node.Body, "node.body")
}

func (s *state) formatVariableDeclaration(node *acorn.Node) {
	s.write(node.Kind + " ")
	for i, declarator := range node.Declarations {
		if i > 0 {
			s.write(", ")
		}
		s.variableDeclarator(declarator)
	}
}

func (s *state) variableDeclaration(node *acorn.Node) {
	s.formatVariableDeclaration(node)
	s.write(";")
}

func (s *state) variableDeclarator(node *acorn.Node) {
	s.node(node.Id, "node.id")
	if node.Init != nil {
		s.write(" = ")
		s.node(node.Init, "node.init")
	}
}

// class writes a class declaration or expression.
func (s *state) class(node *acorn.Node) {
	s.write("class ")
	if node.Id != nil {
		s.write(node.Id.Name + " ")
	}
	if superClass := node.SuperClass; superClass != nil {
		s.write("extends ")
		if superClass.Type != "ClassExpression" && s.precedenceBelow(superClass, expressionsPrecedence["ClassExpression"]) {
			s.formatParenthesizedIf(superClass, "node.superClass", true)
		} else {
			s.node(superClass, "superClass")
		}
		s.write(" ")
	}
	s.block(node.Body)
}

// The module declarations: package acorn parses scripts, so their nodes carry no
// specifiers, declaration or attributes, which astring then reads as undefined.

// undefinedSpecifiers is what astring throws destructuring the length of
// node.specifiers when they are undefined.
func undefinedSpecifiers() *js.Error {
	return js.NewTypeError("Cannot destructure property 'length' of 'specifiers' as it is undefined.")
}

func (s *state) importDeclaration(*acorn.Node) {
	s.write("import ")
	s.fail(undefinedSpecifiers())
}

func (s *state) importAttribute(node *acorn.Node) {
	generator["Identifier"](s, node.Key)
	s.write(": ")
	s.literal(node.Value)
}

func (s *state) importExpression(node *acorn.Node) {
	s.write("import(")
	s.node(node.Source, "node.source")
	s.write(")")
}

func (s *state) exportDefaultDeclaration(*acorn.Node) {
	s.write("export default ")
	s.fail(js.CannotReadProperties(js.Undefined, "type"))
}

func (s *state) exportNamedDeclaration(*acorn.Node) {
	s.write("export {")
	s.fail(undefinedSpecifiers())
}

func (s *state) exportAllDeclaration(node *acorn.Node) {
	s.write("export * from ")
	s.literal(node.Source)
	s.write(";")
}

// methodDefinition writes a class method, or an object method, getter or setter.
func (s *state) methodDefinition(node *acorn.Node) {
	if node.Static {
		s.write("static ")
	}
	if node.Kind == "get" || node.Kind == "set" {
		s.write(node.Kind + " ")
	}
	if node.Value.Async {
		s.write("async ")
	}
	if node.Value.Generator {
		s.write("*")
	}
	s.propertyKey(node)
	s.formatSequence(node.Value.Params)
	s.write(" ")
	s.node(node.Value.Body, "node.value.body")
}

// propertyDefinition writes a class field.
func (s *state) propertyDefinition(node *acorn.Node) {
	if node.Static {
		s.write("static ")
	}
	s.propertyKey(node)
	if node.Value == nil {
		if s.typeOf(node.Key)[0] != 'F' {
			s.write(";")
		}
		return
	}
	s.write(" = ")
	s.node(node.Value, "node.value")
	s.write(";")
}

func (s *state) propertyKey(node *acorn.Node) {
	if node.Computed {
		s.write("[")
		s.node(node.Key, "node.key")
		s.write("]")
		return
	}
	s.node(node.Key, "node.key")
}

func (s *state) arrowFunction(node *acorn.Node) {
	if node.Async {
		s.write("async ")
	}
	if params := node.Params; len(params) == 1 && params[0].Type == "Identifier" {
		s.write(params[0].Name)
	} else {
		s.formatSequence(params)
	}
	s.write(" => ")
	if body := s.typeOf(node.Body); body == "ObjectExpression" || body == "ObjectPattern" {
		s.write("(")
		s.objectExpression(node.Body)
		s.write(")")
	} else {
		s.node(node.Body, "node.body")
	}
}

// spread writes a RestElement or SpreadElement.
func (s *state) spread(node *acorn.Node) {
	s.write("...")
	s.node(node.Argument, "node.argument")
}

func (s *state) yieldExpression(node *acorn.Node) {
	if node.Delegate {
		s.write("yield*")
	} else {
		s.write("yield")
	}
	if node.Argument != nil {
		s.write(" ")
		s.node(node.Argument, "node.argument")
	}
}

func (s *state) awaitExpression(node *acorn.Node) {
	s.write("await ")
	s.formatExpression(node.Argument, node, false)
}

func (s *state) templateLiteral(node *acorn.Node) {
	s.write("`")
	for i, expression := range node.Expressions {
		s.write(node.Quasis[i].TemplateValue.Raw)
		s.write("${")
		s.node(expression, "expression")
		s.write("}")
	}
	s.write(node.Quasis[len(node.Quasis)-1].TemplateValue.Raw)
	s.write("`")
}

func (s *state) taggedTemplate(node *acorn.Node) {
	s.formatExpression(node.Tag, node, false)
	s.node(node.Quasi, "node.quasi")
}

// arrayExpression writes an ArrayExpression or ArrayPattern.
func (s *state) arrayExpression(node *acorn.Node) {
	s.write("[")
	for i, element := range node.Elements {
		if i > 0 {
			s.write(", ")
		}
		if element != nil {
			s.node(element, "element")
		}
	}
	// A trailing hole needs its own comma to be kept.
	if n := len(node.Elements); n > 0 && node.Elements[n-1] == nil {
		s.write(", ")
	}
	s.write("]")
}

func (s *state) objectExpression(node *acorn.Node) {
	objectIndent := strings.Repeat(indent, s.indentLevel)
	s.indentLevel++
	s.write("{")
	if len(node.Properties) > 0 {
		s.write(lineEnd)
		for i, property := range node.Properties {
			if i > 0 {
				s.write("," + lineEnd)
			}
			s.write(objectIndent + indent)
			s.node(property, "property")
		}
		s.write(lineEnd + objectIndent + "}")
	} else {
		s.write("}")
	}
	s.indentLevel--
}

func (s *state) property(node *acorn.Node) {
	switch {
	case node.Method || node.Kind != "init":
		s.methodDefinition(node)
	case node.Shorthand:
		s.node(node.Value, "node.value")
	default:
		s.propertyKey(node)
		s.write(": ")
		s.node(node.Value, "node.value")
	}
}

func (s *state) objectPattern(node *acorn.Node) {
	s.write("{")
	for i, property := range node.Properties {
		if i > 0 {
			s.write(", ")
		}
		s.node(property, "properties[i]")
	}
	s.write("}")
}

func (s *state) unaryExpression(node *acorn.Node) {
	if !node.Prefix {
		s.node(node.Argument, "node.argument")
		s.write(node.Operator)
		return
	}
	operator, argument := node.Operator, node.Argument
	s.write(operator)
	parens := s.expressionNeedsParenthesis(argument, node, false)
	// A space separates keyword operators from their operand, and + or - from a
	// nested + or - that would otherwise make ++ or --.
	repeatsSign := (argument.Type == "UnaryExpression" || argument.Type == "UpdateExpression") && argument.Prefix &&
		(operator == "+" || operator == "-") && strings.HasPrefix(argument.Operator, operator)
	if !parens && (len(operator) > 1 || repeatsSign) {
		s.write(" ")
	}
	if parens {
		if len(operator) > 1 {
			s.write(" (")
		} else {
			s.write("(")
		}
		s.node(argument, "type")
		s.write(")")
		return
	}
	s.node(argument, "type")
}

func (s *state) updateExpression(node *acorn.Node) {
	if node.Prefix {
		s.write(node.Operator)
		s.node(node.Argument, "node.argument")
	} else {
		s.node(node.Argument, "node.argument")
		s.write(node.Operator)
	}
}

// assignment writes an AssignmentExpression or AssignmentPattern.
func (s *state) assignment(node *acorn.Node) {
	s.node(node.Left, "node.left")
	if node.Type == "AssignmentPattern" {
		s.write(" = ")
	} else {
		s.write(" " + node.Operator + " ")
	}
	s.node(node.Right, "node.right")
}

// binaryExpression writes a BinaryExpression or LogicalExpression.
func (s *state) binaryExpression(node *acorn.Node) {
	// An in expression is parenthesized wherever it appears, as it may not
	// stand in the head of a for statement.
	isIn := node.Operator == "in"
	if isIn {
		s.write("(")
	}
	s.formatExpression(node.Left, node, false)
	s.write(" " + node.Operator + " ")
	s.formatExpression(node.Right, node, true)
	if isIn {
		s.write(")")
	}
}

func (s *state) conditionalExpression(node *acorn.Node) {
	test := node.Test
	precedence, known := expressionsPrecedence[s.typeOf(test)]
	s.formatParenthesizedIf(test, "test", known && (precedence == needsParentheses || precedence <= expressionsPrecedence["ConditionalExpression"]))
	s.write(" ? ")
	s.node(node.Consequent, "node.consequent")
	s.write(" : ")
	s.node(node.Alternate, "node.alternate")
}

func (s *state) newExpression(node *acorn.Node) {
	s.write("new ")
	callee := node.Callee
	s.formatParenthesizedIf(callee, "node.callee", s.precedenceBelow(callee, expressionsPrecedence["CallExpression"]) || hasCallExpression(callee))
	s.formatSequence(node.Arguments)
}

func (s *state) callExpression(node *acorn.Node) {
	s.formatParenthesizedIf(node.Callee, "node.callee", s.precedenceBelow(node.Callee, expressionsPrecedence["CallExpression"]))
	if node.Optional {
		s.write("?.")
	}
	s.formatSequence(node.Arguments)
}

func (s *state) memberExpression(node *acorn.Node) {
	s.formatParenthesizedIf(node.Object, "node.object", s.precedenceBelow(node.Object, expressionsPrecedence["MemberExpression"]))
	switch {
	case node.Computed:
		if node.Optional {
			s.write("?.")
		}
		s.write("[")
		s.node(node.Property, "node.property")
		s.write("]")
	case node.Optional:
		s.write("?.")
		s.node(node.Property, "node.property")
	default:
		s.write(".")
		s.node(node.Property, "node.property")
	}
}

func (s *state) literal(node *acorn.Node) {
	switch {
	case node.Raw != "":
		s.write(node.Raw)
	case node.Regex != nil:
		s.regExpLiteral(node)
	case node.Bigint != "":
		s.write(node.Bigint + "n")
	default:
		text, ok, err := jsrt.JSONStringify(node.LiteralValue, nil, nil)
		if err != nil {
			s.fail(err)
		}
		if !ok {
			// JSON.stringify(undefined) is undefined, which astring appends
			// as the text "undefined".
			text = "undefined"
		}
		s.write(text)
	}
}

func (s *state) regExpLiteral(node *acorn.Node) {
	s.write("/" + node.Regex.Pattern + "/" + node.Regex.Flags)
}
