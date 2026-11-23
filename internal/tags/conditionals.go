package tags

import (
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
)

// If is the @if tag: a conditional block.
func If() *parser.Tag {
	return &parser.Tag{TagName: "if", Block: true, Seekable: true, Compile: compileIf}
}

func compileIf(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	return compileCondition(p, buffer, token, "if (", "@if")
}

// Unless is the @unless tag: the inverse of @if.
func Unless() *parser.Tag {
	return &parser.Tag{TagName: "unless", Block: true, Seekable: true, Compile: compileUnless}
}

func compileUnless(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	return compileCondition(p, buffer, token, "if (!", "@unless")
}

// compileCondition writes the if statement of @if and @unless around the children.
// Sequence expressions are not allowed as the condition.
func compileCondition(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken, opening, tagName string) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsNotSubsetOf(parsed, []string{"SequenceExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid argument type for the `+tagName+` tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	buffer.WriteStatement(opening+p.Stringify(parsed)+") {", token.Filename, token.Loc.Start.Line)
	if err := processChildren(p, token.Children, buffer); err != nil {
		return err
	}
	buffer.WriteStatement("}", token.Filename, -1)
	return nil
}

// ElseIf is the @elseif tag. It is an inline tag, so everything between @if and
// @elseif stays a child of @if.
func ElseIf() *parser.Tag {
	return &parser.Tag{TagName: "elseif", Seekable: true, Compile: compileElseIf}
}

func compileElseIf(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsNotSubsetOf(parsed, []string{"SequenceExpression"}, func() error {
		return unallowedAt("{"+token.Properties.JsArg+"} is not a valid argument type for the @elseif tag", token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	buffer.WriteStatement("} else if ("+p.Stringify(parsed)+") {", token.Filename, token.Loc.Start.Line)
	return nil
}

// Else is the @else tag of @if, @unless and @each.
func Else() *parser.Tag {
	return &parser.Tag{TagName: "else", Compile: compileElse}
}

func compileElse(_ *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	buffer.WriteStatement("} else {", token.Filename, -1)
	return nil
}
