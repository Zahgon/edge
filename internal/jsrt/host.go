package jsrt

import (
	"io"
	"sync"

	"github.com/edge-js/edge-go/internal/inspect"
	"github.com/edge-js/edge-go/js"
)

// The host API: what Go code running JavaScript needs. Every operation behaves
// as the JavaScript construct it names, with V8's errors: a JavaScript
// exception comes back as the error, a *js.Error for an Error object and a
// *js.Thrown for any other value (CaughtValue recovers the value either way).

// defaultRealm is the realm Default returns.
var defaultRealm = sync.OnceValue(func() *Realm { return NewRealm(Options{}) })

// Default returns the process-wide realm. Node has one global object per
// process, and code compiled anywhere in the process shares it: a global one
// piece of code creates is visible to code compiled later, and a builtin one
// piece of code replaces is replaced for all. Hosts that stand for one Node
// process run all their JavaScript in this realm. Like every Realm it is not
// safe for concurrent use; callers serialize their use of it.
func Default() *Realm { return defaultRealm() }

// NewFunction is what `new Function(params, body)` does, or, when async is
// true, `new AsyncFunction(params, body)`, where AsyncFunction is
// (async function(){}).constructor. params is the parameter list as one string
// ("template,state,$context"). The function's source text, which
// Function.prototype.toString returns, is V8's:
// "function anonymous(params\n) {\nbody\n}". Syntax errors are returned as the
// *js.Error V8 throws (name SyntaxError and V8's message).
func (r *Realm) NewFunction(params, body string, async bool) (*js.Function, error) {
	kind := "function"
	if async {
		kind = "async function"
	}
	f, err := r.dynamicFunction(kind, []any{params, body})
	if err != nil {
		return nil, err
	}
	return f.(*js.Function), nil
}

// Call calls f with this and args, as Function.prototype.apply(this, args) does.
// A sloppy-mode function called with this undefined sees the global object.
func (r *Realm) Call(f *js.Function, this any, args []any) (any, error) {
	return r.call(f, this, args)
}

// CallValue calls v with this and args, as a builtin calls a callback; a v that
// is not a function throws V8's TypeError for it ("number 1 is not a
// function").
func (r *Realm) CallValue(v, this any, args []any) (any, error) {
	return r.callValue(v, this, args)
}

// Invoke is v[name](...args): the method is read with its getters and called
// with v as this.
func (r *Realm) Invoke(v any, name string, args ...any) (any, error) {
	return r.invoke(v, name, args...)
}

// Construct is `new f(...args)`; a function that is not a constructor throws
// V8's TypeError "<name> is not a constructor".
func (r *Realm) Construct(f *js.Function, args []any) (any, error) {
	return r.construct(f, args, f.Name)
}

// Await is what `await v` in the host's own async function does: it runs the
// queued jobs, as Node drains its microtask queue, and returns v's fulfilment
// value, or its rejection reason as an error (a *js.Error for Error objects, a
// *js.Thrown for other values). A value that is not a promise is returned as it
// is, after the jobs have run. ErrNeverSettled reports a promise still pending
// when no job is left.
//
// When an Await called from the host with no JavaScript running returns, the
// async functions still suspended are waiting for promises that no queued job
// will settle; they are abandoned, so their goroutines end, and they never
// resume. An Await nested in JavaScript (a host function that awaits) leaves them
// to the outer one. A host async function (NewAsyncNative) awaits with the
// await it is given instead.
func (r *Realm) Await(v any) (any, error) {
	outermost := r.async.hosting == 0 && r.async.current == nil && r.depth == 0
	r.async.hosting++
	defer func() {
		r.async.hosting--
		if outermost {
			r.abandonSuspended()
			r.abandonOrphans()
		}
	}()
	rec, ok := r.promiseOf(v)
	if !ok {
		r.runJobs()
		return v, nil
	}
	var (
		settled, rejected bool
		result            any
	)
	r.thenNative(rec, func(x any) {
		settled, result = true, x
	}, func(x any) {
		settled, rejected, result = true, true, x
	})
	r.runJobs()
	switch {
	case !settled:
		return nil, ErrNeverSettled
	case rejected:
		return nil, ThrowValue(result)
	}
	return result, nil
}

// IsPromise reports whether v is a promise of this realm.
func (r *Realm) IsPromise(v any) bool {
	_, ok := r.promiseOf(v)
	return ok
}

// PromiseResolve is Promise.resolve(v): v itself when it is a promise, else a
// promise resolved with v (adopting the state of a thenable).
func (r *Realm) PromiseResolve(v any) (any, error) {
	return r.promiseResolve(r.async.promiseCtor, v)
}

// Then is promise.then(onFulfilled, onRejected) with Go reactions, run as
// promise jobs once the promise settles. It returns the promise the reactions'
// results settle: the value onFulfilled or onRejected returns fulfils it, an
// error rejects it. A nil reaction passes the value or the reason through.
func (r *Realm) Then(promise any, onFulfilled, onRejected func(v any) (any, error)) (any, error) {
	rec, ok := r.promiseOf(promise)
	if !ok {
		return nil, js.NewTypeError("Method Promise.prototype.then called on incompatible receiver " + r.safeString(promise))
	}
	wrap := func(name string, fn func(any) (any, error)) any {
		if fn == nil {
			return js.Undefined
		}
		return NewNative(name, 1, func(_ any, args []any) (any, error) { return fn(js.Arg(args, 0)) })
	}
	return r.promiseThen(rec, wrap("", onFulfilled), wrap("", onRejected)), nil
}

// NullProto returns a new object whose prototype is null, as Object.create(null)
// builds it.
func NullProto() *Derived {
	return WithProto(nil, nil)
}

// ObjectPrototype returns Object.prototype, for objects created with
// WithProto that inherit from it through another prototype.
func (r *Realm) ObjectPrototype() *js.Object { return r.objectProto }

// SetConsole directs the console global of the realm: console.log, info and
// debug write to stdout, console.error and warn to stderr. A new realm writes
// to os.Stdout and os.Stderr.
func (r *Realm) SetConsole(stdout, stderr io.Writer) {
	r.stdout, r.stderr = stdout, stderr
}

// ConsoleLog is console.log(...args) called from the host: util.format of the
// arguments and a line break, written to the realm's stdout. Formatting may
// call JavaScript code (toString, valueOf, toJSON); its errors are returned, as
// are write errors.
func (r *Realm) ConsoleLog(args ...any) error {
	return r.writeConsole(r.stdout, args)
}

// Get is v[key]: the property's value on v or its prototype chain, a getter's
// result for an accessor. Primitives read through their prototype; null and
// undefined throw V8's TypeError "Cannot read properties of undefined (reading
// 'key')".
func (r *Realm) Get(v any, key string) (any, error) {
	return r.getV(v, key)
}

// Set is v[key] = val in strict-mode code (strict) or sloppy-mode code: a
// setter runs for an accessor, and a refused write (read-only property,
// non-extensible object, property of a primitive) is V8's TypeError in strict
// mode and ignored in sloppy mode. Null and undefined throw V8's TypeError
// "Cannot set properties of undefined (setting 'key')".
func (r *Realm) Set(v any, key string, val any, strict bool) error {
	return r.put(v, key, val, strict)
}

// Delete is delete v[key]: deleting a non-configurable property is V8's
// TypeError in strict mode and does nothing in sloppy mode.
func (r *Realm) Delete(v any, key string, strict bool) error {
	_, err := r.deleteProp(v, key, strict)
	return err
}

// Has is `key in v`; v that is not an object throws V8's TypeError.
func (r *Realm) Has(v any, key string) (bool, error) {
	if !js.IsObject(v) {
		return false, js.NewTypeError("Cannot use 'in' operator to search for '" + key + "' in " + r.safeString(v))
	}
	return r.hasProperty(v, key), nil
}

// HasOwn is Object.hasOwn(v, key); null and undefined throw V8's TypeError.
func (r *Realm) HasOwn(v any, key string) (bool, error) {
	if _, err := toObject(v); err != nil {
		return false, err
	}
	return r.hasOwn(v, key), nil
}

// PrototypeOf is Object.getPrototypeOf(v): nil at the end of the chain.
// Primitives report the prototype of their wrapper objects; null and undefined
// throw V8's TypeError.
func (r *Realm) PrototypeOf(v any) (any, error) {
	if _, err := toObject(v); err != nil {
		return nil, err
	}
	return r.protoOf(v), nil
}

// InstanceOf is `v instanceof f` for a constructor f.
func (r *Realm) InstanceOf(v any, f *js.Function) bool {
	return r.instanceOf(v, f)
}

// ToString is String(v) as template output converts values: the
// ToString abstract operation, which calls an object's toString or valueOf.
func (r *Realm) ToString(v any) (string, error) {
	return r.toString(v)
}

// ToNumber is the ToNumber abstract operation (Number(v) for v that is not a
// BigInt): an object's valueOf or toString runs.
func (r *Realm) ToNumber(v any) (float64, error) {
	return r.toNumber(v)
}

// ToPrimitive is the ToPrimitive abstract operation with hint "default",
// "number" or "string"; Date objects treat "default" as "string".
func (r *Realm) ToPrimitive(v any, hint string) (any, error) {
	return r.toPrimitive(v, hint)
}

// ToPropertyKey is the ToPropertyKey abstract operation, which turns the key of
// v[key] into a string.
func (r *Realm) ToPropertyKey(v any) (string, error) {
	return r.toPropertyKey(v)
}

// LooselyEquals is the == operator.
func (r *Realm) LooselyEquals(a, b any) (bool, error) {
	return r.looseEquals(a, b)
}

// ClassOf is the tag Object.prototype.toString.call(v) shows between
// "[object " and "]", such as "Array", "Function", "AsyncFunction", "Map",
// "Promise", "Error" or "global".
func (r *Realm) ClassOf(v any) string {
	return r.classOf(v)
}

// OwnEnumerableKeys is Object.keys(v): v's own enumerable string keys in V8's
// order (integer indexes ascending, then the other keys in creation order),
// the order for-in visits them in. Null and undefined throw V8's TypeError.
func (r *Realm) OwnEnumerableKeys(v any) ([]string, error) {
	if _, err := toObject(v); err != nil {
		return nil, err
	}
	return r.ownKeys(v), nil
}

// OwnKeys is Object.getOwnPropertyNames(v): all of v's own string keys,
// enumerable or not, in V8's order. Null and undefined throw V8's TypeError.
func (r *Realm) OwnKeys(v any) ([]string, error) {
	if _, err := toObject(v); err != nil {
		return nil, err
	}
	return r.ownPropertyKeys(v, true), nil
}

// Entry is one [key, value] pair of Object.entries.
type Entry struct {
	Key   string
	Value any
}

// Entries is Object.entries(v): v's own enumerable string-keyed properties in
// V8's key order, their values read with getters at the time each is reached
// (a property a getter deleted or made non-enumerable meanwhile is skipped).
// Null and undefined throw V8's TypeError.
func (r *Realm) Entries(v any) ([]Entry, error) {
	if _, err := toObject(v); err != nil {
		return nil, err
	}
	var out []Entry
	err := r.enumerableOwn(v, func(k string, val any) { out = append(out, Entry{k, val}) })
	return out, err
}

// ForOf runs fn on each value `for (const x of v)` visits, stopping at fn's
// first error: an array's items by index (live, so items the loop adds are
// visited), a string's code points, a Map's [key, value] entries, a Set's
// values, an arguments object's items and the values of a builtin iterator. A
// value that is not iterable throws V8's TypeError for a value that has no
// source text ("<v> is not iterable (cannot read property
// Symbol(Symbol.iterator))").
func (r *Realm) ForOf(v any, fn func(item any) error) error {
	return r.forEachIterated(v, func() error { return notIterableValue(v) }, fn)
}

// CaughtValue is the value a catch clause binds for an error a call returned:
// the value of a *js.Thrown, the *js.Error itself, and for any other Go error
// a new Error object whose message is the Go error's text and whose Orig is
// the Go error. A host function returns ThrowValue(v) to throw a value v.
func CaughtValue(err error) any {
	return caughtValue(err)
}

// Inspect is util.inspect(v) with Node's default options, as console.log
// prints a value that is not a string.
func (r *Realm) Inspect(v any) string {
	return inspect.Inspect(inspectHost{r}, v)
}

// JSONStringify is JSON.stringify(value, replacer, space) for any value in the
// js value model, run in the Default realm: Date values use their toJSON,
// objects' own toJSON methods are honoured and cycles throw V8's TypeError. ok
// is false when the result is undefined.
func JSONStringify(v, replacer, space any) (s string, ok bool, err error) {
	return Default().JSONStringify(v, replacer, space)
}

// GetProperty is v[key] read by the host without running getters (an
// accessor reads as undefined), in the Default realm: v's own property, else
// what it inherits from its prototype chain, else undefined. Null and
// undefined have no properties.
func GetProperty(v any, key string) any {
	return Default().dataProperty(v, key)
}

// HasProperty is `key in Object(v)`, in the Default realm; null and undefined
// have no properties.
func HasProperty(v any, key string) bool {
	if js.IsNullish(v) {
		return false
	}
	return Default().hasProperty(v, key)
}
