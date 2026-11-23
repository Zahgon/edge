package jsrt

import "github.com/edge-js/edge-go/js"

// The internal slots of function objects live in js.Function.Internal, so a
// function and everything it closes over are collected with it: a function
// defined in JavaScript holds its *closure, a bound function its
// *boundFunction, and a builtin or host function with more than a call
// behaviour its *hostFunction. A plain host function has none.

// boundFunction holds [[BoundTargetFunction]] and [[BoundArguments]] of a
// function made by Function.prototype.bind.
type boundFunction struct {
	target *js.Function
	args   []any
}

// hostFunction holds what a function implemented in Go has beyond its call
// behaviour.
type hostFunction struct {
	// construct is [[Construct]]; nil when the function is not a constructor.
	construct func(args []any) (any, error)
	// async marks a host async function, whose call returns a promise.
	async bool
	// class marks a class constructor, which cannot be called without new.
	class bool
	// source is the text Function.prototype.toString returns, "" for the
	// "[native code]" form.
	source string
	// resolving is the Go side of a promise's resolve or reject function.
	resolving func(any)
}

// FunctionSource is the source text of the function.
func (c *closure) FunctionSource() (string, bool) { return c.fi.source, true }

// FunctionSource is V8's text for a bound function, which has no name in it.
func (*boundFunction) FunctionSource() (string, bool) { return "function () { [native code] }", true }

// FunctionSource is the text the host gave the function, if any.
func (h *hostFunction) FunctionSource() (string, bool) { return h.source, h.source != "" }

// closureOf returns the code and scope of a function defined in JavaScript.
func closureOf(f *js.Function) (*closure, bool) {
	c, ok := f.Internal.(*closure)
	return c, ok
}

// boundOf returns the target and arguments of a bound function.
func boundOf(f *js.Function) (*boundFunction, bool) {
	b, ok := f.Internal.(*boundFunction)
	return b, ok
}

// hostOf returns the slots of a builtin or host function, creating them when
// create is set and f has none. It is nil for functions defined in JavaScript
// and bound functions.
func hostOf(f *js.Function, create bool) *hostFunction {
	h, _ := f.Internal.(*hostFunction)
	if h == nil && create && f.Internal == nil {
		h = &hostFunction{}
		f.Internal = h
	}
	return h
}

// constructBehaviour returns the [[Construct]] of a builtin or host
// constructor.
func constructBehaviour(f *js.Function) (func(args []any) (any, error), bool) {
	if h := hostOf(f, false); h != nil && h.construct != nil {
		return h.construct, true
	}
	return nil, false
}
