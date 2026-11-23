package jsrt

import (
	"io"
	"math"

	"github.com/edge-js/edge-go/js"
)

// DefaultMaxCallDepth is the default bound on nested JavaScript function calls: the
// depth at which Node 24 overflows its stack for a simple recursive function.
const DefaultMaxCallDepth = 10450

// Options configures a Realm.
type Options struct {
	// Now returns the current time in milliseconds since the epoch, for Date.now()
	// and new Date(). When nil, both throw an Error, so output never depends on
	// the wall clock by accident.
	Now func() float64
	// MaxCallDepth bounds nested calls of JavaScript functions; deeper calls throw
	// RangeError "Maximum call stack size exceeded". Zero means DefaultMaxCallDepth.
	MaxCallDepth int
}

// Realm is a JavaScript global environment: the global object, the builtin
// constructors and prototypes, and the per-object state the Go value model has no
// room for. Templates compiled in the same realm share globals, as templates in one
// Node process do (sloppy-mode code that assigns an undeclared name creates a
// global). A Realm is not safe for concurrent use.
type Realm struct {
	global *js.Object

	objectProto, functionProto, arrayProto, stringProto *js.Object
	numberProto, booleanProto, dateProto, regexpProto   *js.Object
	bigintProto                                         *js.Object
	iteratorProto, mapProto, setProto                   *js.Object
	weakMapProto, weakSetProto                          *js.Object
	mapIteratorProto, setIteratorProto                  *js.Object
	errorProtos                                         map[string]*js.Object
	// objectToString is Object.prototype.toString, which V8's messages look for.
	objectToString *js.Function
	// functionCall is Function.prototype.call, whose error for a receiver that
	// is not callable names the call site.
	functionCall *js.Function
	// thrower is the accessor of a strict-mode arguments object's callee.
	thrower *js.Accessor

	// protoLinks gives the prototype of objects whose prototype is not the one
	// their Go type implies: builtin prototypes and Object.setPrototypeOf targets.
	protoLinks map[any]any
	// levels holds the integrity level of objects passed to
	// Object.preventExtensions, seal or freeze.
	levels map[any]integrity
	// extra holds the property tables of objects whose Go type has no room for
	// one (arrays, functions, regular expressions, ...).
	extra map[any]*js.Object
	// privates holds the private fields, methods and accessors of objects
	// that classes have initialized.
	privates map[any]privateElements

	// deletedGlobals records the names of Node globals code has deleted from
	// the global object (see GlobalDefined).
	deletedGlobals map[string]bool

	// joining holds the arrays Array.prototype.join is currently joining.
	joining []*js.Array

	// stdout and stderr receive the console's output.
	stdout, stderr io.Writer

	// async holds the job queue, the promises and the suspended async functions.
	async asyncState
	// generators holds the generator intrinsics, asyncGenerators those of
	// async generators.
	generators      generatorIntrinsics
	asyncGenerators asyncGeneratorIntrinsics

	now      func() float64
	depth    int
	maxDepth int
}

// NewRealm returns a realm with its ECMAScript builtins installed on its global
// object.
func NewRealm(opts Options) *Realm {
	r := &Realm{
		global:         js.NewObject(),
		errorProtos:    map[string]*js.Object{},
		protoLinks:     map[any]any{},
		levels:         map[any]integrity{},
		deletedGlobals: map[string]bool{},
		extra:          map[any]*js.Object{},
		privates:       map[any]privateElements{},
		async:          newAsyncState(),
		now:            opts.Now,
		maxDepth:       opts.MaxCallDepth,
	}
	if r.maxDepth <= 0 {
		r.maxDepth = DefaultMaxCallDepth
	}
	r.objectProto = js.NewObject()
	r.functionProto = js.NewObject()
	r.arrayProto = js.NewObject()
	r.stringProto = js.NewObject()
	r.numberProto = js.NewObject()
	r.booleanProto = js.NewObject()
	r.bigintProto = js.NewObject()
	r.dateProto = js.NewObject()
	r.regexpProto = js.NewObject()
	r.iteratorProto = js.NewObject()
	r.mapProto = js.NewObject()
	r.setProto = js.NewObject()
	r.weakMapProto = js.NewObject()
	r.weakSetProto = js.NewObject()
	r.mapIteratorProto = js.NewObject()
	r.setIteratorProto = js.NewObject()
	r.setupGlobals()
	r.setupObject()
	r.setupFunction()
	r.setupArray()
	r.setupString()
	r.setupNumber()
	r.setupBoolean()
	r.setupMath()
	r.setupJSON()
	r.setupDate()
	r.setupRegExp()
	r.setupErrors()
	r.setupIterator()
	r.setupCollections()
	r.setupConsole()
	r.setupPromise()
	r.setupAsyncFunction()
	r.setupGenerators()
	r.setupAsyncGenerators()
	r.setupBigInt()
	return r
}

// Global returns the global object. Hosts may add globals (for example a process
// object) before running templates.
func (r *Realm) Global() *js.Object { return r.global }

// method installs a builtin method on an object or a constructor.
func (r *Realm) method(o any, name string, length int, fn NativeFunc) *js.Function {
	f := NewNative(name, length, fn)
	r.define(o, name, f)
	return f
}

// constructor creates a builtin constructor with its prototype object, installs it
// as a global, and registers its [[Construct]] behaviour (nil: not constructible).
func (r *Realm) constructor(name string, length int, proto *js.Object, call NativeFunc, construct func([]any) (any, error)) *js.Function {
	f := NewNative(name, length, call)
	if proto != nil {
		r.extraProps(f, true).Define("prototype", proto, attrsConstant)
		r.define(proto, "constructor", f)
	}
	if construct != nil {
		hostOf(f, true).construct = construct
	}
	r.define(r.global, name, f)
	return f
}

// staticObject creates a namespace object such as Math or JSON on the global.
func (r *Realm) staticObject(name string) *js.Object {
	o := js.NewObject()
	r.define(r.global, name, o)
	return o
}

// isUndefined reports whether v is undefined.
func isUndefined(v any) bool {
	_, ok := v.(js.UndefinedType)
	return ok
}

func (r *Realm) setupGlobals() {
	g := r.global
	r.define(g, "globalThis", g)
	r.defineConstants(g, "undefined", js.Undefined, "NaN", math.NaN(), "Infinity", math.Inf(1))
	r.setupGlobalFunctions()
}
