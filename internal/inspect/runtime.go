// Package inspect ports Node 24's util.inspect and util.format
// (lib/internal/util/inspect.js), colours off, for the values of the
// JavaScript interpreter: what console.log prints.
package inspect

import "github.com/edge-js/edge-go/js"

// Runtime is the interpreter's view of the objects it prints: what the value
// model alone does not record (prototypes, property attributes, accessors and
// the internal slots of Map, Set, promises and the like), and the abstract
// operations util.format runs, which may call JavaScript code.
type Runtime interface {
	// Prototype returns v's [[Prototype]], nil at the end of the chain.
	Prototype(v any) any
	// OwnKeys returns v's own string keys in JavaScript order: the
	// enumerable ones, or all of them when hidden is set.
	OwnKeys(v any, hidden bool) []string
	// OwnProperty returns v's own property key without running a getter.
	OwnProperty(v any, key string) (Property, bool)
	// Describe returns the kind and internal slots of the object v.
	Describe(v any) Object
	// ToString, ToNumber, ParseInt and ParseFloat are String(v), Number(v),
	// parseInt(v) and parseFloat(v), as %s, %d, %i and %f convert their
	// argument.
	ToString(v any) (string, error)
	ToNumber(v any) (float64, error)
	ParseInt(v any) (float64, error)
	ParseFloat(v any) (float64, error)
	// JSONStringify is JSON.stringify(v); ok is false for undefined.
	JSONStringify(v any) (s string, ok bool, err error)
}

// Property is an own property as Object.getOwnPropertyDescriptor reports it.
type Property struct {
	// Value is a data property's value.
	Value any
	// Getter and Setter report the functions of an accessor property.
	Getter, Setter bool
	Enumerable     bool
}

// Kind is the kind of object inspect formats, as Node tells them apart with
// its type checks (isMap, isPromise, isBoxedPrimitive, ...).
type Kind int

const (
	// Ordinary is any object without a more specific kind.
	Ordinary Kind = iota
	// Array is an array.
	Array
	// Function is a function; Object.Async and Object.Generator tell its kind.
	Function
	// Error is an Error object.
	Error
	// Date is a Date object.
	Date
	// RegExp is a regular expression object.
	RegExp
	// Arguments is a function's arguments object; Object.Values holds its items.
	Arguments
	// Boxed is a Number, String, Boolean or BigInt object; Object.Primitive is
	// its value.
	Boxed
	// Map is a Map, with its entries in Object.Values.
	Map
	// Set is a Set, with its values in Object.Values.
	Set
	// WeakMap is a WeakMap, whose entries are not listed.
	WeakMap
	// WeakSet is a WeakSet, whose values are not listed.
	WeakSet
	// MapIterator previews the entries a Map iterator has left in Values.
	MapIterator
	// SetIterator previews the values a Set iterator has left in Values.
	SetIterator
	// Promise has its state in State and its result in Primitive.
	Promise
)

// PromiseState is a promise's [[PromiseState]].
type PromiseState int

const (
	// Pending is a promise not settled yet.
	Pending PromiseState = iota
	// Fulfilled is a promise fulfilled with Object.Primitive.
	Fulfilled
	// Rejected is a promise rejected with Object.Primitive.
	Rejected
)

// Object describes an object's kind and internal slots.
type Object struct {
	Kind Kind
	// Tag is the object's Symbol.toStringTag, "" when it has none.
	Tag string
	// Values holds a Set's values, an arguments object's items, a Map's
	// entries as key, value pairs, and an iterator's preview (pairs when
	// KeyValue is set).
	Values   []any
	KeyValue bool
	// Primitive is a boxed primitive or a settled promise's result.
	Primitive any
	State     PromiseState
	// Async marks an async function, Generator a generator function.
	Async, Generator bool
}

// funcName returns a function's name as inspect reads it (the name property).
func funcName(rt Runtime, f *js.Function) string {
	if p, ok := rt.OwnProperty(f, "name"); ok {
		if s, ok := p.Value.(string); ok {
			return s
		}
	}
	return ""
}
