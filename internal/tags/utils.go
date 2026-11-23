// Package tags holds Edge's built-in tags (src/tags) and the tags of the migrate
// plugin (src/migrate/tags), with the helpers of Edge's utils.ts they use.
//
// The tags' Boot functions are left nil: booting a tag adds macros to the Template
// class of the runtime, which attaches them.
package tags

import (
	"crypto/rand"
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// UnallowedExpression is the E_UNALLOWED_EXPRESSION error at loc.
func UnallowedExpression(message, filename string, loc lexer.LexerPosition) *edgeerror.EdgeError {
	return edgeerror.At(message, "E_UNALLOWED_EXPRESSION", filename, loc.Line, loc.Col)
}

// unallowedAt is UnallowedExpression at the position of a node.
func unallowedAt(message, filename string, node *acorn.Node) error {
	loc, err := parser.GetExpressionLoc(node)
	if err != nil {
		return err
	}
	return UnallowedExpression(message, filename, loc)
}

// IsSubsetOf calls errorCallback, and returns its error, when the expression's type
// is not one of expressions. A nil expression is JavaScript's undefined.
func IsSubsetOf(expression *acorn.Node, expressions []string, errorCallback func() error) error {
	if expression == nil {
		return js.CannotReadProperties(js.Undefined, "type")
	}
	for _, e := range expressions {
		if e == expression.Type {
			return nil
		}
	}
	return errorCallback()
}

// IsNotSubsetOf calls errorCallback, and returns its error, when the expression's
// type is one of expressions.
func IsNotSubsetOf(expression *acorn.Node, expressions []string, errorCallback func() error) error {
	for _, e := range expressions {
		if e == expression.Type {
			return errorCallback()
		}
	}
	return nil
}

// ParseJsArg generates and transforms the AST of a tag's argument.
func ParseJsArg(p *parser.Parser, token *lexer.TagToken) (*acorn.Node, error) {
	ast, err := p.GenerateAST(token.Properties.JsArg, token.Loc, token.Filename)
	if err != nil {
		return nil, err
	}
	return p.TransformAST(ast, token.Filename)
}

// statementExpression is `statement.expression` for the statement generateAST
// returns, checked with isSubsetOf: the expression of an expression statement. A
// function declaration carries acorn's legacy `expression: false`, whose type is
// undefined, so the check fails and reading the location of `false` throws; every
// other statement has no expression, and reading its type throws.
func statementExpression(statement *acorn.Node) (*acorn.Node, error) {
	switch {
	case statement.Type == "FunctionDeclaration":
		return nil, js.CannotReadProperties(js.Undefined, "start")
	case statement.Expression == nil:
		return nil, js.CannotReadProperties(js.Undefined, "type")
	}
	return statement.Expression, nil
}

// StringifiedObject builds the source text of an object literal whose values are
// evaluated when the compiled template runs, not when it is compiled.
type StringifiedObject struct {
	obj string
}

// AddSpread adds a spread entry (`...expression`, written by the caller).
func (o *StringifiedObject) AddSpread(key string) {
	if o.obj != "" {
		o.obj += ", "
	}
	o.obj += key
}

// Add adds a key and the source text of its value; a computed key is written in
// brackets.
func (o *StringifiedObject) Add(key, value string, isComputed bool) {
	if isComputed {
		key = "[" + key + "]"
	}
	if o.obj != "" {
		o.obj += ", "
	}
	o.obj += key + ": " + value
}

// Flush returns the object literal and empties the object.
func (o *StringifiedObject) Flush() string {
	obj := "{ " + o.obj + " }"
	o.obj = ""
	return obj
}

// nanoidAlphabet is the alphabet of Edge's copy of nanoid: nanoid's URL-friendly
// alphabet without its "-", 63 characters for 64 possible indexes.
const nanoidAlphabet = "useandom26T198340PX75pxJACKVERYMINDBUSHWOLF_GQZbfghjklqvwyzrict"

// Nanoid returns a random id of length characters picked by `seed[63 & random]`.
// Index 63 is past the end of the alphabet: JavaScript reads undefined there and
// the id gets the text "undefined".
func Nanoid(length int) string {
	random := make([]byte, length)
	// crypto/rand.Read never returns an error (it aborts the program instead).
	rand.Read(random)
	var out strings.Builder
	for _, b := range random {
		if i := int(63 & b); i < len(nanoidAlphabet) {
			out.WriteByte(nanoidAlphabet[i])
		} else {
			out.WriteString("undefined")
		}
	}
	return out.String()
}

// processChildren compiles a token's children into a buffer.
func processChildren(p *parser.Parser, children []lexer.Token, buffer *parser.EdgeBuffer) error {
	for _, child := range children {
		if err := p.ProcessToken(child, buffer); err != nil {
			return err
		}
	}
	return nil
}
