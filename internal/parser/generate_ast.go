package parser

import (
	"errors"
	"regexp"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/astring"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
)

// patchLoc moves an acorn location from the expression to the template: the line
// is offset by the line the token starts at, and a position on the expression's
// first line also by the token's column.
func patchLoc(loc *acorn.SourceLocation, lexerLoc lexer.LexerLoc) {
	if loc.Start.Line == 1 {
		loc.Start.Column += lexerLoc.Start.Col
	}
	loc.Start.Line += lexerLoc.Start.Line - 1
	loc.End.Line += lexerLoc.Start.Line - 1
}

// acornPosition matches the " (line:column)" acorn appends to its messages; the
// message keeps the space before it.
var acornPosition = regexp.MustCompile(`\(\d+:\d+\)`)

// GenerateAST parses jsArg, the expression of a token at lexerLoc, with acorn
// (ECMAScript 2020, locations, await allowed outside functions) and returns the
// first statement of the program, nil for an empty program. Every token's location
// is patched to point into the template, so the nodes carry template positions. A
// syntax error becomes an E_ACORN_ERROR EdgeError at its template position.
func (p *Parser) GenerateAST(jsArg string, lexerLoc lexer.LexerLoc, filename string) (*acorn.Node, error) {
	program, err := acorn.Parse(jsArg, acorn.Options{
		Locations:                 true,
		EcmaVersion:               2020,
		AllowAwaitOutsideFunction: true,
		OnToken:                   func(token *acorn.Token) { patchLoc(token.Loc, lexerLoc) },
	})
	if err != nil {
		var syntaxError *acorn.SyntaxError
		if !errors.As(err, &syntaxError) {
			return nil, err
		}
		// The error position is not reported through onToken: patch it here.
		line := syntaxError.Loc.Line + lexerLoc.Start.Line - 1
		col := syntaxError.Loc.Column
		if syntaxError.Loc.Line == 1 {
			col += lexerLoc.Start.Col
		}
		message := syntaxError.Message
		if m := acornPosition.FindStringIndex(message); m != nil {
			message = message[:m[0]] + message[m[1]:]
		}
		return nil, edgeerror.At(message, "E_ACORN_ERROR", filename, line, col)
	}
	if len(program.BodyList) == 0 {
		return nil, nil
	}
	return program.BodyList[0], nil
}

// Stringify prints a node as JavaScript source (astring's generate). Where
// astring throws (a node type it has no generator for, a missing child), the
// TypeError is thrown as a panic carrying a *js.Error: a tag's Compile function
// calls Stringify, and the parser returns the error from ProcessToken, as the
// exception propagates out of edge-parser.
func (p *Parser) Stringify(node *acorn.Node) string {
	code, err := astring.Generate(node)
	if err != nil {
		panic(stringifyError{err})
	}
	return code
}

// stringifyError is what Stringify panics with.
type stringifyError struct{ err error }

// recoverStringifyError turns a Stringify panic into *err.
func recoverStringifyError(err *error) {
	if r := recover(); r != nil {
		thrown, ok := r.(stringifyError)
		if !ok {
			panic(r)
		}
		*err = thrown.err
	}
}
