package tags

import (
	"strconv"
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// includeAllowedExpressions are the expressions a partial's path may be.
var includeAllowedExpressions = []string{
	"Literal", "Identifier", "CallExpression", "TemplateLiteral", "MemberExpression", "LogicalExpression", "ConditionalExpression",
}

// getRenderExpression returns the call that compiles and renders a partial with
// the template's state and its local variables.
func getRenderExpression(p *parser.Parser, parsedExpression *acorn.Node) string {
	localVariables := p.Stack.Values()
	renderArgs := []string{p.Stringify(parsedExpression)}
	callFnArgs := []string{"template", "state", "$context"}
	if len(localVariables) > 0 {
		quoted := make([]string, len(localVariables))
		for i, v := range localVariables {
			quoted[i] = `"` + js.ToString(v) + `"`
		}
		renderArgs = append(renderArgs, strings.Join(quoted, ","))
		callFnArgs = append(callFnArgs, parser.JoinValues(localVariables, ","))
	}
	return "template.compilePartial(" + strings.Join(renderArgs, ",") + ")(" + strings.Join(callFnArgs, ",") + ")"
}

// Include is the @include tag: it renders a partial in the scope of the template.
func Include() *parser.Tag {
	return &parser.Tag{TagName: "include", Seekable: true, Compile: compileInclude}
}

func compileInclude(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	awaitKeyword := ""
	if p.AsyncMode() {
		awaitKeyword = "await "
	}
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, includeAllowedExpressions, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid argument type for the @include tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	buffer.OutputExpression(awaitKeyword+getRenderExpression(p, parsed), token.Filename, token.Loc.Start.Line, false)
	return nil
}

// IncludeIf is the @includeIf tag: `@includeIf(condition, partial)`.
func IncludeIf() *parser.Tag {
	return &parser.Tag{TagName: "includeIf", Seekable: true, Compile: compileIncludeIf}
}

func compileIncludeIf(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	awaitKeyword := ""
	if p.AsyncMode() {
		awaitKeyword = "await "
	}
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, []string{"SequenceExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid argument type for the @includeIf tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	if len(parsed.Expressions) != 2 {
		return edgeerror.At("@includeIf expects a total of 2 arguments", "E_ARGUMENTS_MIS_MATCH", token.Filename, parsed.Loc.Start.Line, parsed.Loc.Start.Column)
	}
	conditional, include := parsed.Expressions[0], parsed.Expressions[1]
	err = IsNotSubsetOf(conditional, []string{"SequenceExpression"}, func() error {
		return unallowedAt(`"`+conditional.Type+`" is not a valid 1st argument type for the @includeIf tag`, token.Filename, conditional)
	})
	if err != nil {
		return err
	}
	err = IsSubsetOf(include, includeAllowedExpressions, func() error {
		return unallowedAt(`"`+include.Type+`" is not a valid 2nd argument type for the @includeIf tag`, token.Filename, include)
	})
	if err != nil {
		return err
	}
	buffer.WriteStatement("if ("+p.Stringify(conditional)+") {", token.Filename, token.Loc.Start.Line)
	buffer.OutputExpression(awaitKeyword+getRenderExpression(p, include), token.Filename, token.Loc.Start.Line, false)
	buffer.WriteStatement("}", token.Filename, -1)
	return nil
}

// NewError is the @newError tag: `@newError(message, filename?, line?, col?)`
// raises an error pointing at the given position, the tag's own by default.
func NewError() *parser.Tag {
	return &parser.Tag{TagName: "newError", Seekable: true, NoNewLine: true, Compile: compileNewError}
}

func compileNewError(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	message := p.Stringify(parsed)
	filename := "$filename"
	line := strconv.Itoa(token.Loc.Start.Line)
	col := strconv.Itoa(token.Loc.Start.Col)
	if parsed.Type == "SequenceExpression" {
		expressions := parsed.Expressions
		message = p.Stringify(expressions[0])
		if len(expressions) > 1 {
			filename = p.Stringify(expressions[1])
		}
		if len(expressions) > 2 {
			line = p.Stringify(expressions[2])
		}
		if len(expressions) > 3 {
			col = p.Stringify(expressions[3])
		}
	}
	buffer.WriteStatement("template.newError("+message+", "+filename+", "+line+", "+col+")", token.Filename, token.Loc.Start.Line)
	return nil
}
