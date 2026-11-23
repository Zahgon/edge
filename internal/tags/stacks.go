package tags

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
)

// parseStackName transforms the argument of @stack, @pushTo and @pushOnceTo, which
// may not be a sequence expression. The original tests the type with
// "SequenceExpression".includes(type).
func parseStackName(p *parser.Parser, token *lexer.TagToken, tagName string) (*acorn.Node, error) {
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return nil, err
	}
	if strings.Contains("SequenceExpression", parsed.Type) {
		loc, err := parser.GetExpressionLoc(parsed)
		if err != nil {
			return nil, err
		}
		return nil, edgeerror.At(`"`+token.Properties.JsArg+`" is not a valid argument type for the "`+tagName+`" tag`, "E_UNALLOWED_EXPRESSION", token.Filename, loc.Line, loc.Col)
	}
	return parsed, nil
}

// Stack is the @stack tag: the placeholder the pushed contents are written to.
func Stack() *parser.Tag {
	return &parser.Tag{TagName: "stack", Seekable: true, Compile: compileStack}
}

func compileStack(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	parsed, err := parseStackName(p, token, "@stack")
	if err != nil {
		return err
	}
	buffer.OutputExpression("template.stacks.create("+p.Stringify(parsed)+")", token.Filename, token.Loc.Start.Line, false)
	return nil
}

// generateStackID returns the unique variable name of a pushed content.
func generateStackID() string { return "stack_" + Nanoid(15) }

// compilePush compiles the children of @pushTo or @pushOnceTo into a variable of
// their own and returns the stack name's source and the variable.
func compilePush(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken, tagName string) (name, stackID string, err error) {
	parsed, err := parseStackName(p, token, tagName)
	if err != nil {
		return "", "", err
	}
	stackID = generateStackID()
	stackBuffer := buffer.Create(token.Filename, stackID)
	if err := processChildren(p, token.Children, stackBuffer); err != nil {
		return "", "", err
	}
	buffer.WriteStatement(stackBuffer.DisableFileAndLineVariables().DisableReturnStatement().DisableTryCatchBlock().Flush(), token.Filename, token.Loc.Start.Line)
	return p.Stringify(parsed), stackID, nil
}

// PushTo is the @pushTo tag: it pushes its contents to a stack.
func PushTo() *parser.Tag {
	return &parser.Tag{TagName: "pushTo", Block: true, Seekable: true, NoNewLine: true, Compile: compilePushTo}
}

func compilePushTo(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	name, stackID, err := compilePush(p, buffer, token, "@pushTo")
	if err != nil {
		return err
	}
	buffer.WriteExpression("template.stacks.pushTo("+name+", "+stackID+")", token.Filename, token.Loc.Start.Line)
	return nil
}

// PushOnceTo is the @pushOnceTo tag: it pushes its contents to a stack once per
// source position, however often the template renders.
func PushOnceTo() *parser.Tag {
	return &parser.Tag{TagName: "pushOnceTo", Block: true, Seekable: true, NoNewLine: true, Compile: compilePushOnceTo}
}

// pathSeparators are the characters of the filename replaced in a source id.
var pathSeparators = regexp.MustCompile(`\\|/`)

func compilePushOnceTo(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	name, stackID, err := compilePush(p, buffer, token, "@pushOnceTo")
	if err != nil {
		return err
	}
	line, col := token.Loc.Start.Line, token.Loc.Start.Col
	sourceID := pathSeparators.ReplaceAllString(token.Filename, "_") + "-" + strconv.Itoa(line) + "-" + strconv.Itoa(col)
	buffer.WriteExpression("template.stacks.pushOnceTo("+name+", '"+sourceID+"', "+stackID+")", token.Filename, line)
	return nil
}
