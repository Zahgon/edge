// Package edgeerror is edge-error 4.0.2: the error every part of Edge raises for a
// problem it can point at in a template.
package edgeerror

import "github.com/edge-js/edge-go/js"

// EdgeError is an instance of edge-error's EdgeError class. The fields hold the
// JavaScript values the constructor assigns: Message is the message as given (a
// string for every error Edge raises itself, any value for a template's @newError),
// Line and Col are numbers (float64) for positions Edge computes, and Filename is the
// template path.
type EdgeError struct {
	Message  any
	Code     string
	Filename any
	Line     any
	Col      any
}

// Options are the third argument of the EdgeError constructor: { line, col, filename }.
type Options struct {
	Line     any
	Col      any
	Filename any
}

// New is `new EdgeError(message, code, { line, col, filename })`.
func New(message any, code string, options Options) *EdgeError {
	return &EdgeError{Message: message, Code: code, Filename: options.Filename, Line: options.Line, Col: options.Col}
}

// At is New for the errors Edge raises at a template position: the line and the
// column become JavaScript numbers.
func At(message, code, filename string, line, col int) *EdgeError {
	return New(message, code, Options{Line: float64(line), Col: float64(col), Filename: filename})
}

// Error is String(error.message).
func (e *EdgeError) Error() string { return js.ToString(e.Message) }
