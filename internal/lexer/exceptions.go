package lexer

import (
	"github.com/edge-js/edge-go/internal/edgeerror"
)

// CannotSeekStatement is raised for content on the same line after a tag's closing
// parenthesis, as in `@if(username) Hello {{ username }}`.
func CannotSeekStatement(chars string, pos LexerPosition, filename string) *edgeerror.EdgeError {
	return edgeerror.At(`Unexpected token "`+chars+`"`, "E_CANNOT_SEEK_STATEMENT", filename, pos.Line, pos.Col)
}

// UnclosedParen is raised when a tag's argument has no closing parenthesis.
func UnclosedParen(pos LexerPosition, filename string) *edgeerror.EdgeError {
	return edgeerror.At(`Missing token ")"`, "E_UNCLOSED_PAREN", filename, pos.Line, pos.Col)
}

// UnopenedParen is raised when a tag that takes an argument is not followed by an
// opening parenthesis, as in `@if username`.
func UnopenedParen(pos LexerPosition, filename string) *edgeerror.EdgeError {
	return edgeerror.At(`Missing token "("`, "E_UNOPENED_PAREN", filename, pos.Line, pos.Col)
}

// UnclosedCurlyBrace is raised when a mustache statement is never closed.
func UnclosedCurlyBrace(pos LexerPosition, filename string) *edgeerror.EdgeError {
	return edgeerror.At(`Missing token "}"`, "E_UNCLOSED_CURLY_BRACE", filename, pos.Line, pos.Col)
}

// UnclosedTag is raised when a block tag has no @end.
func UnclosedTag(tag string, pos LexerPosition, filename string) *edgeerror.EdgeError {
	return edgeerror.At("Unclosed tag "+tag, "E_UNCLOSED_TAG", filename, pos.Line, pos.Col)
}
