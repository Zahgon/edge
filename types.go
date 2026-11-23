package edge

import (
	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// EdgeError is an instance of edge-error's EdgeError: the error Edge raises
// for a problem it can point at in a template (E_CANNOT_SEEK_STATEMENT,
// E_UNALLOWED_EXPRESSION, ...) and the error a runtime exception of a
// template becomes (E_RUNTIME_EXCEPTION). Template code sees it as a
// JavaScript Error object.
type EdgeError = edgeerror.EdgeError

// The tokens of edge-lexer. Token is one of *TagToken, *MustacheToken,
// *RawToken, *NewLineToken and *CommentToken.
type (
	Token         = lexer.Token
	LexerPosition = lexer.LexerPosition
	LexerLoc      = lexer.LexerLoc
	TagProperties = lexer.TagProperties
	TagToken      = lexer.TagToken
	MustacheToken = lexer.MustacheToken
	RawToken      = lexer.RawToken
	NewLineToken  = lexer.NewLineToken
	CommentToken  = lexer.CommentToken
)

// Node is an ESTree node as acorn produces it (ecmaVersion 2020, with
// locations).
type Node = acorn.Node

// Tag is a tag definition. Compile is called by the parser for every token
// of the tag; Boot, when set, is called by RegisterTag and may add macros and
// getters to the Template class with Macro and Getter.
type Tag = parser.Tag

// Parser is edge-parser's Parser as tags see it: parser.utils.* are its
// methods (GenerateAST, TransformAST, Stringify, ...).
type Parser = parser.Parser

// Stack is edge-parser's Stack: the variables defined in the scopes of the
// template being compiled.
type Stack = parser.Stack

// Buffer is edge-parser's EdgeBuffer: the generated code of a template.
type Buffer = parser.EdgeBuffer

// ClaimTagFn claims a name that is not a registered tag: the returned tag's
// Seekable, Block and NoNewLine tell the lexer how to read it. nil leaves the
// name alone.
type ClaimTagFn = parser.ClaimTagFn

// O returns an object literal: keys and values alternating.
func O(kv ...any) *js.Object { return js.ObjectOf(kv...) }

// A returns an array literal.
func A(items ...any) *js.Array { return js.NewArray(items...) }
