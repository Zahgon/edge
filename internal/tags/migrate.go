package tags

import (
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// Set is the migrate plugin's @set tag: `@set(key, value)` defines or updates a
// variable, `@set(collection, key, value)` sets a property path on a collection.
func Set() *parser.Tag {
	return &parser.Tag{TagName: "set", Seekable: true, NoNewLine: true, Compile: compileSet}
}

// regExpFlagsOrder is the order of RegExp.prototype.flags.
const regExpFlagsOrder = "dgimsuvy"

// literalValue is `node.value`: the value of a literal (a RegExp object for a
// regular expression, a BigInt for a BigInt literal), undefined for other nodes.
func literalValue(node *acorn.Node) any {
	if node.Type != "Literal" {
		return js.Undefined
	}
	if node.Regex != nil {
		var flags strings.Builder
		for _, f := range regExpFlagsOrder {
			if strings.ContainsRune(node.Regex.Flags, f) {
				flags.WriteRune(f)
			}
		}
		return &js.RegExp{Source: node.Regex.Pattern, Flags: flags.String()}
	}
	return node.LiteralValue
}

func compileSet(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, []string{"SequenceExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid key-value pair for the @slot tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	expressions := parsed.Expressions
	if len(expressions) < 2 || len(expressions) > 3 {
		return edgeerror.At("@set tag accepts a minimum of 2 or maximum or 3 arguments", "E_INVALID_ARGUMENTS_COUNT", token.Filename, parsed.Loc.Start.Line, parsed.Loc.Start.Column)
	}
	var collection, key, value *acorn.Node
	if len(expressions) == 3 {
		collection, key, value = expressions[0], expressions[1], expressions[2]
	} else {
		key, value = expressions[0], expressions[1]
	}
	keyValue := literalValue(key)
	keyText := js.ToString(keyValue)
	if collection != nil {
		buffer.WriteExpression("template.setValue("+p.Stringify(collection)+", '"+keyText+"', "+p.Stringify(value)+")", token.Filename, token.Loc.Start.Line)
		return nil
	}
	// An existing variable is updated; it is never declared twice.
	expression := "let " + keyText + " = " + p.Stringify(value)
	if p.Stack.HasValue(keyValue) {
		expression = keyText + " = " + p.Stringify(value)
	}
	buffer.WriteExpression(expression, token.Filename, token.Loc.Start.Line)
	p.Stack.DefineValue(keyValue)
	return nil
}

// Super is the migrate plugin's @super tag. The compiler merges it into the parent
// section; one anywhere else is an error.
func Super() *parser.Tag {
	return &parser.Tag{TagName: "super", Compile: compileSuper}
}

func compileSuper(_ *parser.Parser, _ *parser.EdgeBuffer, token *lexer.TagToken) error {
	return edgeerror.At("@super tag must appear as top level tag inside the @section tag", "E_ORPHAN_SUPER_TAG", token.Filename, token.Loc.Start.Line, token.Loc.Start.Col)
}

// Layout is the migrate plugin's @layout tag. The compiler reads it from the first
// line of a template; it compiles to nothing.
func Layout() *parser.Tag {
	return &parser.Tag{TagName: "layout", Seekable: true, NoNewLine: true, Compile: compileLayout}
}

func compileLayout(*parser.Parser, *parser.EdgeBuffer, *lexer.TagToken) error { return nil }

// Section is the migrate plugin's @section tag: its children are compiled in place
// (the compiler has already merged a layout's sections).
func Section() *parser.Tag {
	return &parser.Tag{TagName: "section", Block: true, Seekable: true, Compile: compileSection}
}

func compileSection(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	return processChildren(p, token.Children, buffer)
}
