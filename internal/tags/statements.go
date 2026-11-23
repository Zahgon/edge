package tags

import (
	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// Let is the @let tag: it declares a variable in the current scope of the
// template, `@let(name = value)`, with destructuring allowed.
func Let() *parser.Tag {
	return &parser.Tag{TagName: "let", Seekable: true, NoNewLine: true, Compile: compileLet}
}

// identifierName is `node.name`: the name of an identifier, undefined for any other
// node.
func identifierName(node *acorn.Node) any {
	if node.Type == "Identifier" {
		return node.Name
	}
	return js.Undefined
}

// patternName is `node.argument ? node.argument.name : node.name` for an entry of a
// destructuring pattern: the name a rest element or an identifier binds.
func patternName(node *acorn.Node) any {
	if node.Argument != nil {
		return identifierName(node.Argument)
	}
	if node.Type == "Property" {
		return identifierName(node.Value)
	}
	return identifierName(node)
}

func compileLet(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	statement, err := p.GenerateAST("let "+token.Properties.JsArg, token.Loc, token.Filename)
	if err != nil {
		return err
	}
	if statement == nil {
		return js.CannotReadProperties(js.Undefined, "declarations")
	}
	// `let` followed by anything else than a binding is an expression statement.
	if statement.Type != "VariableDeclaration" {
		return js.CannotReadProperties(js.Undefined, "0")
	}
	declarator := statement.Declarations[0]
	key, value := declarator.Id, declarator.Init
	err = IsSubsetOf(key, []string{"ObjectPattern", "Identifier", "ArrayPattern"}, func() error {
		return unallowedAt("Invalid variable name for the @let tag", token.Filename, key)
	})
	if err != nil {
		return err
	}
	switch key.Type {
	case "Identifier":
		p.Stack.DefineVariable(key.Name)
	case "ObjectPattern":
		for _, property := range key.Properties {
			p.Stack.DefineValue(patternName(property))
		}
	case "ArrayPattern":
		for _, element := range key.Elements {
			if element == nil {
				return js.CannotReadProperties(nil, "argument")
			}
			p.Stack.DefineValue(patternName(element))
		}
	}
	keySource := p.Stringify(key)
	if value == nil {
		return js.CannotReadProperties(nil, "type")
	}
	transformed, err := p.TransformAST(value, token.Filename)
	if err != nil {
		return err
	}
	buffer.WriteExpression("let "+keySource+" = "+p.Stringify(transformed), token.Filename, token.Loc.Start.Line)
	return nil
}

// Assign is the @assign tag: it assigns a new value to an existing variable.
func Assign() *parser.Tag {
	return &parser.Tag{TagName: "assign", Seekable: true, NoNewLine: true, Compile: compileAssign}
}

func compileAssign(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, []string{"AssignmentExpression"}, func() error {
		return unallowedAt("Invalid expression for the @assign tag", token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	buffer.WriteExpression(p.Stringify(parsed), token.Filename, token.Loc.Start.Line)
	return nil
}

// Eval is the @eval tag: it evaluates an expression without writing its value.
func Eval() *parser.Tag {
	return &parser.Tag{TagName: "eval", Seekable: true, NoNewLine: true, Compile: compileEval}
}

func compileEval(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	buffer.WriteExpression(p.Stringify(parsed), token.Filename, token.Loc.Start.Line)
	return nil
}

// Debugger is the @debugger tag: a debugger statement in the compiled template.
func Debugger() *parser.Tag {
	return &parser.Tag{TagName: "debugger", NoNewLine: true, Compile: compileDebugger}
}

func compileDebugger(_ *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	buffer.WriteExpression("debugger", token.Filename, token.Loc.Start.Line)
	return nil
}
