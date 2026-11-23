package edgert

import (
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// NewFunction returns a function implemented in Go that stands for one the
// original defines in JavaScript: its name and length are the original's,
// and Function.prototype.toString shows the original's source, recorded
// under key in sources_gen.go.
func NewFunction(key, name string, length int, fn jsrt.NativeFunc) *js.Function {
	f := jsrt.NewNative(name, length, fn)
	jsrt.SetSource(f, mustSource(key))
	return f
}

// NewAsyncFunction is NewFunction for an async function.
func NewAsyncFunction(key, name string, length int, body jsrt.AsyncNativeFunc) *js.Function {
	f := Realm().NewAsyncNative(name, length, body)
	jsrt.SetSource(f, mustSource(key))
	return f
}

// DefineMethod defines a method on a class prototype or constructor,
// non-enumerable as class methods are, with the source text recorded under key.
func DefineMethod(target any, key, name string, length int, fn jsrt.NativeFunc) *js.Function {
	f := Realm().DefineMethod(target, name, length, fn)
	jsrt.SetSource(f, mustSource(key))
	return f
}

// NewClass defines a class with the source text recorded under its name.
func NewClass(name string, length int, base *jsrt.Class, init jsrt.ClassInit) *jsrt.Class {
	c := Realm().NewClass(name, length, base, init)
	jsrt.SetSource(c.Ctor, mustSource(name))
	return c
}

// mustSource returns the recorded source text of key. Every key the package
// asks for is one the generator records.
func mustSource(key string) string {
	src, ok := sources[key]
	if !ok {
		panic("edgert: no recorded source for " + key)
	}
	return src
}
