package tags

import (
	"slices"
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// Each is the @each tag: a loop over an array, a string or an object's entries,
// `@each(item in list)` or `@each((item, index) in list)`, with an optional @else
// for an empty list.
func Each() *parser.Tag {
	return &parser.Tag{TagName: "each", Block: true, Seekable: true, Compile: compileEach}
}

// getLoopItemAndIndex returns the names of the loop item and the index from the
// left side of the `in` expression: an identifier or a sequence of two.
func getLoopItemAndIndex(lhs *acorn.Node, filename string) (item, index string, err error) {
	err = IsSubsetOf(lhs, []string{"SequenceExpression", "Identifier"}, func() error {
		return unallowedAt(`invalid left hand side "`+lhs.Type+`" expression for the @each tag`, filename, lhs)
	})
	if err != nil {
		return "", "", err
	}
	if lhs.Type != "SequenceExpression" {
		return lhs.Name, "", nil
	}
	// The message interpolates the node object itself, as the original does.
	value, key := lhs.Expressions[0], lhs.Expressions[1]
	err = IsSubsetOf(value, []string{"Identifier"}, func() error {
		return unallowedAt(`"[object Object].type" is not allowed as value identifier for @each tag`, filename, value)
	})
	if err != nil {
		return "", "", err
	}
	err = IsSubsetOf(key, []string{"Identifier"}, func() error {
		return unallowedAt(`"[object Object].type" is not allowed as key identifier for @each tag`, filename, key)
	})
	if err != nil {
		return "", "", err
	}
	return value.Name, key.Name, nil
}

func compileEach(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	awaitKeyword, loopFunctionName, asyncKeyword := "", "loop", ""
	if p.AsyncMode() {
		awaitKeyword, loopFunctionName, asyncKeyword = "await ", "loopAsync", "async "
	}
	// The AST is not transformed: the loop variables must not become state reads.
	statement, err := p.GenerateAST(token.Properties.JsArg, token.Loc, token.Filename)
	if err != nil {
		return err
	}
	if statement == nil {
		return js.NewTypeError("Cannot destructure property 'expression' of 'parser.utils.generateAST(...)' as it is undefined.")
	}
	expression, err := statementExpression(statement)
	if err != nil {
		return err
	}
	err = IsSubsetOf(expression, []string{"BinaryExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not valid expression for the @each tag`, token.Filename, expression)
	})
	if err != nil {
		return err
	}
	elseIndex := slices.IndexFunc(token.Children, func(child lexer.Token) bool { return lexer.IsTag(child, "else") })
	var elseChildren []lexer.Token
	if elseIndex > -1 {
		elseChildren = token.Children[elseIndex:]
		token.Children = token.Children[:elseIndex:elseIndex]
	}
	listNode, err := p.TransformAST(expression.Right, token.Filename)
	if err != nil {
		return err
	}
	list := p.Stringify(listNode)
	item, index, err := getLoopItemAndIndex(expression.Left, token.Filename)
	if err != nil {
		return err
	}
	if elseIndex > -1 {
		buffer.WriteStatement("if(template.size("+list+")) {", token.Filename, token.Loc.Start.Line)
	}
	loopCallbackArgs := []string{item}
	if index != "" {
		loopCallbackArgs = append(loopCallbackArgs, index)
	}
	buffer.WriteStatement(awaitKeyword+"template."+loopFunctionName+"("+list+", "+asyncKeyword+"function ("+strings.Join(loopCallbackArgs, ",")+") {", token.Filename, token.Loc.Start.Line)
	// The loop variables are local to the loop body.
	p.Stack.DefineScope()
	p.Stack.DefineVariable(item)
	if index != "" {
		p.Stack.DefineVariable(index)
	}
	if err := processChildren(p, token.Children, buffer); err != nil {
		return err
	}
	p.Stack.ClearScope()
	buffer.WriteExpression("})", token.Filename, -1)
	if elseIndex > -1 {
		if err := processChildren(p, elseChildren, buffer); err != nil {
			return err
		}
		buffer.WriteStatement("}", token.Filename, -1)
	}
	return nil
}
