package jsrt

import (
	"runtime"

	"github.com/edge-js/edge-go/js"
)

// Async functions. The interpreter walks the syntax tree with Go recursion, so an
// await deep inside an expression cannot return to the job loop by unwinding.
// Instead each call of an async function runs its body on a goroutine of its own,
// a coroutine, and control passes between goroutines like a baton: whoever
// resumes a coroutine blocks until it suspends at an await or finishes, so
// exactly one goroutine runs interpreter code at any time and the realm needs no
// locking. The channels order every hand-over, which is what the race detector
// checks.
//
// The call depth behind "Maximum call stack size exceeded" follows the
// JavaScript stack: a suspended body keeps only the frames it pushed itself, and
// when a job resumes it those frames sit on top of the job's.
//
// A coroutine's goroutine ends when its body completes, or when the body is
// abandoned: it then exits from its suspension point through runtime.Goexit,
// running no more JavaScript (only Go deferred calls). A body parked at an
// await is abandoned when the host's outermost Await returns, as no job is
// left that could resume it. A generator or async generator parked at a yield
// is resumed by its object alone; once the object is unreachable, a finalizer
// records it and the next generator creation or outermost Await abandons it.
// A suspended generator whose object stays reachable (from a global, or from
// a closure the suspended body itself holds) keeps its goroutine as long as
// that holds, as V8 keeps the generator's frame.

// coroutine is the goroutine running one async function body.
type coroutine struct {
	resume chan resumption
	// yield carries control back to the resumer: nil when the body suspends or
	// returns, or the value of a Go panic the body raised, re-raised there.
	yield chan any
	// base is the call depth of whoever resumed the body; frames is the number of
	// frames the body had pushed on top of it when it last suspended.
	base, frames int
	// done is set once the goroutine has ended.
	done bool
}

// resumption is what a suspended await receives: the awaited promise's value or
// rejection, or the instruction to abandon the body.
type resumption struct {
	value   any
	err     error
	abandon bool
}

// startCoroutine runs body as a new coroutine until it first suspends or ends,
// and returns the coroutine.
func (r *Realm) startCoroutine(body func()) *coroutine {
	co := &coroutine{resume: make(chan resumption), yield: make(chan any)}
	go func() {
		defer func() {
			// recover is nil when the body ended normally or was abandoned
			// (runtime.Goexit).
			p := recover()
			co.done = true
			co.yield <- p
		}()
		<-co.resume
		body()
	}()
	r.switchTo(co, resumption{})
	return co
}

// switchTo hands control to co with msg and waits until it suspends again or ends.
func (r *Realm) switchTo(co *coroutine, msg resumption) {
	if co.done {
		return
	}
	prev, depth := r.async.current, r.depth
	delete(r.async.suspended, co)
	r.async.current = co
	co.base = depth
	r.depth = depth + co.frames
	co.resume <- msg
	p := <-co.yield
	r.async.current, r.depth = prev, depth
	if p != nil {
		panic(p)
	}
}

// suspend parks the running coroutine until a job resumes it, and returns what
// it was resumed with. An abandoned coroutine ends its goroutine here without
// running any more JavaScript; only Go deferred calls run.
func (r *Realm) suspend(co *coroutine) (any, error) {
	r.async.suspended[co] = true
	return r.park(co)
}

// park hands control back to whoever resumed co and waits to be resumed. A
// generator parks at a yield without being registered as suspended, as only
// its own object resumes it.
func (r *Realm) park(co *coroutine) (any, error) {
	co.frames = r.depth - co.base
	co.yield <- nil
	msg := <-co.resume
	if msg.abandon {
		runtime.Goexit()
	}
	return msg.value, msg.err
}

// abandonSuspended ends the goroutines of all suspended coroutines.
func (r *Realm) abandonSuspended() {
	for co := range r.async.suspended {
		r.switchTo(co, resumption{abandon: true})
	}
}

// callAsync runs an async function's body, run, as AsyncFunctionStart does: the
// body starts at once and runs until its first await; its completion resolves or
// rejects the promise returned to the caller.
func (r *Realm) callAsync(run func() (any, error)) any {
	capability := r.newPromiseCapability()
	r.startCoroutine(func() {
		v, err := run()
		if err != nil {
			capability.reject(caughtValue(err))
			return
		}
		capability.resolve(v)
	})
	return capability.promise
}

// await is the await operator: the operand becomes a promise (PromiseResolve,
// which returns a native promise as it is), the coroutine suspends, and the
// reaction job of that promise resumes it with the value, or throws the reason
// at the await.
func (r *Realm) await(v any) (any, error) {
	co := r.async.current
	if co == nil {
		return nil, js.NewSyntaxError("await is only valid in async functions and the top level bodies of modules")
	}
	rec, err := r.intrinsicPromise(v)
	if err != nil {
		return nil, err
	}
	r.thenNative(rec, func(x any) {
		r.switchTo(co, resumption{value: x})
	}, func(x any) {
		r.switchTo(co, resumption{err: ThrowValue(x)})
	})
	return r.suspend(co)
}

// setupAsyncFunction creates %AsyncFunction% and %AsyncFunction.prototype%, the
// constructor and prototype of async functions. Neither is a global; scripts
// reach them as (async function () {}).constructor.
func (r *Realm) setupAsyncFunction() {
	proto := js.NewObject()
	r.async.asyncFunctionProto = proto
	r.protoLinks[proto] = r.functionProto
	ctor := NewNative("AsyncFunction", 1, func(_ any, args []any) (any, error) {
		return r.dynamicFunction("async function", args)
	})
	hostOf(ctor, true).construct = func(args []any) (any, error) {
		return r.dynamicFunction("async function", args)
	}
	r.extraProps(ctor, true).Define("prototype", proto, attrsConstant)
	r.define(proto, "constructor", ctor)
}

// isAsyncFunction reports a function defined by an async function, arrow or
// method.
func (r *Realm) isAsyncFunction(f *js.Function) bool {
	if c, ok := closureOf(f); ok {
		return c.fi.async
	}
	h := hostOf(f, false)
	return h != nil && h.async
}
