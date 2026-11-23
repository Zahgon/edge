// Package jsrt is the JavaScript engine the Edge port runs compiled templates
// on: it parses JavaScript source text and evaluates it with V8's semantics and
// V8's error messages, as Node 24 runs the functions Edge builds with
// new Function and AsyncFunction.
//
// A Realm holds a global environment: the global object, the builtin
// constructors and prototypes (Object, Function, Array, String, Number,
// Boolean, BigInt, Math, JSON, Date, RegExp, Map, Set, WeakMap, WeakSet,
// Promise, the Error classes, the iterator and generator objects) and the
// console, which prints with Node's util.format and util.inspect. Values are
// those of package js; objects the Go value model has no room for (prototype
// links, extra properties of arrays and functions, private class elements) are
// kept in the realm.
//
// Source text is parsed into a syntax tree (parser.go and its companions) and
// evaluated by walking it (interp.go): functions, arrows, classes, generators,
// async functions and async generators, destructuring, spread, optional
// chaining, labelled statements, with, sloppy- and strict-mode scoping and
// Annex B. Async bodies and generators run as coroutines (async.go), and
// promise reactions run on the realm's job queue (jobs.go). The host API
// (host.go, hostfunc.go) lets Go code call JavaScript, define native functions
// and classes, and read and write properties as JavaScript does.
package jsrt
