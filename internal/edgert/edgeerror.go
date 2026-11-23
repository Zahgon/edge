package edgert

import (
	"errors"
	"sync"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// Errors Edge raises are *edgeerror.EdgeError values in Go. When one is
// thrown into template code it becomes an instance of the EdgeError class
// (an Error object), so that `error instanceof EdgeError` holds in
// template.reThrow; when one comes back out of template code it becomes the
// Go value again.

// edgeErrorClass is `class EdgeError extends Error`.
var edgeErrorClass = sync.OnceValue(func() *jsrt.Class {
	return NewClass("EdgeError", 3, Realm().ErrorClass(), func(this any, args []any) error {
		message, code, options := js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2)
		props := []string{"line", "col", "filename"}
		values := make([]any, len(props))
		for i, key := range props {
			v, err := Realm().Get(options, key)
			if err != nil {
				return err
			}
			values[i] = v
		}
		e := this.(*js.Error)
		initEdgeError(e, message, code, values[0], values[1], values[2])
		return nil
	})
})

// initEdgeError sets the own properties the EdgeError constructor gives an
// instance after super(message): the class fields line, col and filename,
// then `this.message = message`, code, line, col and filename.
//
// The assignment keeps the message as it was given. super(message) defined a
// non-enumerable message unless message is undefined; then the assignment
// creates an enumerable one, after the class fields.
func initEdgeError(e *js.Error, message, code, line, col, filename any) {
	if e.Props == nil {
		e.Props = js.NewObject()
	}
	for _, key := range []string{"line", "col", "filename"} {
		e.Props.Set(key, js.Undefined)
	}
	if message == js.Undefined {
		e.Props.Set("message", message)
	} else {
		e.SetMessage(message)
	}
	e.Props.Set("code", code)
	e.Props.Set("line", line)
	e.Props.Set("col", col)
	e.Props.Set("filename", filename)
}

// NewEdgeError is `new EdgeError(message, code, options)` with options read
// from a JavaScript value (getters run; null and undefined throw).
func NewEdgeError(message any, code string, options any) (*js.Error, error) {
	v, err := edgeErrorClass().Construct([]any{message, code, options})
	if err != nil {
		return nil, err
	}
	return v.(*js.Error), nil
}

// ToJS returns the Error object template code sees for a Go error: an
// EdgeError instance for an *edgeerror.EdgeError, the error itself otherwise.
func ToJS(err error) error {
	var ee *edgeerror.EdgeError
	if !errors.As(err, &ee) {
		return err
	}
	e := edgeErrorClass().Instance(nil).(*js.Error)
	// super(message) converts the message unless it is undefined.
	if _, undefined := ee.Message.(js.UndefinedType); !undefined {
		s, serr := Realm().ToString(ee.Message)
		if serr != nil {
			return serr
		}
		e.Message = s
	}
	initEdgeError(e, ee.Message, ee.Code, ee.Line, ee.Col, ee.Filename)
	return e
}

// IsEdgeError reports whether v is an instance of the EdgeError class:
// `v instanceof EdgeError`.
func IsEdgeError(v any) bool {
	return edgeErrorClass().Is(v)
}

// FromJS returns the Go error for an error that came out of template code: an
// *edgeerror.EdgeError for an EdgeError instance, the error itself otherwise.
func FromJS(err error) error {
	var e *js.Error
	if !errors.As(err, &e) || !IsEdgeError(e) {
		return err
	}
	message, _ := js.GetOwn(e, "message")
	prop := func(key string) any {
		if e.Props == nil {
			return js.Undefined
		}
		return e.Props.Get(key)
	}
	return &edgeerror.EdgeError{
		Message:  message,
		Code:     js.ToString(prop("code")),
		Filename: prop("filename"),
		Line:     prop("line"),
		Col:      prop("col"),
	}
}
