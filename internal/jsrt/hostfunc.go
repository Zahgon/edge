package jsrt

import (
	"github.com/edge-js/edge-go/js"
)

// Functions and classes the host implements in Go. Template code sees them as
// ordinary JavaScript functions and class instances: name and length are
// observable through .name, .length and util.inspect, instanceof and
// x.constructor.name work, and an error a Go body returns is thrown into the
// calling JavaScript code (see CaughtValue for how a Go error becomes a value).

// NativeFunc is the Go body of a function: the this value and the arguments
// of the call. A returned error is what the call throws.
type NativeFunc func(this any, args []any) (any, error)

// NewNative returns a function implemented in Go, named name and with the
// given length, as builtin functions are: Function.prototype.toString shows
// "function name() { [native code] }" and the function is not a constructor.
func NewNative(name string, length int, fn NativeFunc) *js.Function {
	return &js.Function{Name: name, Length: length, Call: fn}
}

// AsyncNativeFunc is the Go body of a host async function. await is the await
// operator of that body: it suspends the body until the awaited value settles
// and returns its fulfilment value, or its rejection reason as an error, with
// the same job ordering as `await` in V8. await must only be called by the body
// it was passed to, while that body runs.
type AsyncNativeFunc func(await func(v any) (any, error), this any, args []any) (any, error)

// NewAsyncNative returns a host async function: calling it runs body at once,
// as an async function runs until its first await, and returns a promise that
// the body's result fulfils or its error rejects. It is an AsyncFunction for
// Object.prototype.toString and util.inspect.
//
// asyncEach(collection, iteratee), whose JavaScript body is
// `for (const [k, v] of collection.entries()) await iteratee(v, k)`, is
//
//	r.NewAsyncNative("asyncEach", 2, func(await func(any) (any, error), _ any, args []any) (any, error) {
//		... for each item: if _, err := await(callIteratee(v, k)); err != nil { return nil, err }
//		return js.Undefined, nil
//	})
func (r *Realm) NewAsyncNative(name string, length int, body AsyncNativeFunc) *js.Function {
	f := &js.Function{Name: name, Length: length}
	f.Internal = &hostFunction{async: true}
	f.Call = func(this any, args []any) (any, error) {
		if err := r.enterCall(); err != nil {
			return nil, err
		}
		defer r.leaveCall()
		return r.callAsync(func() (any, error) {
			co := r.async.current
			await := func(v any) (any, error) {
				if r.async.current != co {
					panic("jsrt: an async host function awaited outside its own body")
				}
				return r.await(v)
			}
			return body(await, this, args)
		}), nil
	}
	return f
}

// SetSource sets the text Function.prototype.toString returns for a function
// implemented in Go, for a host function that stands for one written in
// JavaScript. Functions defined in JavaScript keep their own source.
func SetSource(f *js.Function, source string) {
	if h := hostOf(f, true); h != nil {
		h.source = source
	}
}

// DefineMethod installs fn as a method of target, non-enumerable as class and
// builtin methods are, and returns the function object.
func (r *Realm) DefineMethod(target any, name string, length int, fn NativeFunc) *js.Function {
	return r.method(target, name, length, fn)
}

// DefineValue defines (or redefines) target's own data property key with the
// given attributes, as Object.defineProperty does, without running setters.
func (r *Realm) DefineValue(target any, key string, value any, attrs js.Attr) error {
	return r.defineOwnProperty(target, key, &propertyDescriptor{
		value: value, hasValue: true,
		writable: attrs&js.ReadOnly == 0, hasWritable: true,
		enumerable: attrs&js.Hidden == 0, hasEnumerable: true,
		configurable: attrs&js.Fixed == 0, hasConfigurable: true,
	})
}

// DefineAccessor defines target's own accessor property key with a getter and a
// setter (either may be nil) and the given attributes (js.ReadOnly has no
// meaning for accessors), as Object.defineProperty does.
func (r *Realm) DefineAccessor(target any, key string, get, set *js.Function, attrs js.Attr) error {
	return r.defineOwnProperty(target, key, &propertyDescriptor{
		get: get, hasGet: true,
		set: set, hasSet: true,
		enumerable: attrs&js.Hidden == 0, hasEnumerable: true,
		configurable: attrs&js.Fixed == 0, hasConfigurable: true,
	})
}

// Class is a class the host defines, as `class Name extends Base {}` creates
// it: a constructor whose prototype property is Proto, Proto's constructor
// property pointing back at it.
type Class struct {
	Name  string
	Ctor  *js.Function
	Proto *js.Object
	realm *Realm
	// isError marks a subclass of Error, whose instances are Error objects.
	isError bool
	init    ClassInit
}

// ClassInit is a class constructor's body after super(): this is the new
// instance (a *Derived, or a *js.Error for a subclass of Error) and args the
// arguments of new.
type ClassInit func(this any, args []any) error

// NewClass defines `class name extends base { constructor(...) { super(); init } }`.
// base is nil for a class without extends (whose prototype inherits from
// Object.prototype) or another host class, whose constructor runs first and
// whose prototype the new prototype inherits from; a subclass of an Error
// class makes Error objects. init may be nil. Calling the constructor without
// new throws V8's TypeError; new creates an instance and runs the
// constructors. Methods are added with DefineMethod on Proto, statics on Ctor.
func (r *Realm) NewClass(name string, length int, base *Class, init ClassInit) *Class {
	c := &Class{Name: name, Proto: js.NewObject(), realm: r, init: init}
	c.Ctor = &js.Function{Name: name, Length: length}
	c.Ctor.Call = func(any, []any) (any, error) {
		return nil, js.NewTypeError("Class constructor " + name + " cannot be invoked without 'new'")
	}
	c.Ctor.Internal = &hostFunction{class: true, construct: c.Construct}
	r.table(c.Ctor, true).Define("prototype", c.Proto, attrsConstant)
	r.define(c.Proto, "constructor", c.Ctor)
	if base != nil {
		c.isError = base.isError
		r.protoLinks[c.Proto] = base.Proto
		r.protoLinks[c.Ctor] = base.Ctor
		if init == nil {
			c.init = base.init
		} else {
			c.init = func(this any, args []any) error {
				if err := base.init(this, args); err != nil {
					return err
				}
				return init(this, args)
			}
		}
	}
	return c
}

// ErrorClass is the base for subclasses of Error the host defines: Error
// itself as NewClass sees it. Its init is the Error constructor's, so
// `class EdgeError extends Error { constructor(message, code, options) {
// super(message); ... } }` is
//
//	r.NewClass("EdgeError", 3, r.ErrorClass(), func(this any, args []any) error { ... })
//
// whose instances are *js.Error values with the new class's prototype, an own
// message when args[0] is not undefined (converted with ToString as
// super(message) converts it) and the name Error.prototype supplies.
func (r *Realm) ErrorClass() *Class {
	ctor := r.global.Get("Error").(*js.Function)
	return &Class{Name: "Error", Ctor: ctor, Proto: r.errorProtos["Error"], realm: r, isError: true,
		init: func(this any, args []any) error {
			msg := js.Arg(args, 0)
			if isUndefined(msg) {
				return nil
			}
			s, err := r.toString(msg)
			if err != nil {
				return err
			}
			this.(*js.Error).Message = s
			return nil
		}}
}

// Construct is `new C(...args)`: a new instance with C's prototype, on which the
// constructors of the class chain run.
func (c *Class) Construct(args []any) (any, error) {
	obj := c.Instance(nil)
	if c.init != nil {
		if err := c.init(obj, args); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

// Instance returns a new instance of C without running its constructor: a
// *Derived whose own properties are own (a new empty object when nil), or for
// a subclass of Error a *js.Error whose extra properties are own.
func (c *Class) Instance(own *js.Object) any {
	if c.isError {
		e := &js.Error{Name: "Error", Props: own}
		if c.Proto != c.realm.errorProtos["Error"] {
			c.realm.protoLinks[e] = c.Proto
		}
		return e
	}
	return WithProto(own, c.Proto)
}

// Is reports whether v is an instance of C: `v instanceof C`.
func (c *Class) Is(v any) bool {
	return c.realm.instanceOf(v, c.Ctor)
}
