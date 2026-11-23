package jsrt

import (
	"github.com/edge-js/edge-go/js"
)

// Promises (ECMAScript §27.2). A promise is a *Derived object whose prototype is
// Promise.prototype (or a subclass's prototype) and whose promise field holds
// its internal slots. NewPromiseCapability(C) builds an intrinsic promise for
// %Promise% and calls new C(executor) for any other constructor, as then (through
// the species constructor) and the statics (through their receiver) do.
//
// Unhandled rejections are not tracked: HostPromiseRejectionTracker does nothing,
// so a rejected promise nobody handles is never reported.

// promiseState is [[PromiseState]].
type promiseState uint8

const (
	promisePending promiseState = iota
	promiseFulfilled
	promiseRejected
)

// promiseRecord holds the internal slots of a promise.
type promiseRecord struct {
	state promiseState
	// result is [[PromiseResult]]: the fulfilment value or the rejection reason.
	result                            any
	fulfillReactions, rejectReactions []*promiseReaction
}

// promiseReaction is a PromiseReaction Record. handler is the JavaScript function
// passed to then, or nil for an empty handler (the value passes through, or the
// reason is rethrown). native, when set, is a continuation of the runtime itself
// (the resumption of an await, or the host's Await) and replaces handler; such
// reactions have no capability.
type promiseReaction struct {
	capability *promiseCapability
	rejects    bool
	handler    *js.Function
	native     func(arg any)
}

// promiseCapability is a PromiseCapability Record. Its resolving functions are
// kept as Go functions; resolverFunction wraps one when JavaScript code
// receives it. promise is a *Derived promise, except for a capability of a
// constructor that is not a subclass of Promise.
type promiseCapability struct {
	promise         any
	resolve, reject func(any)
}

// newPromise is a new pending promise: OrdinaryCreateFromConstructor with
// Promise.prototype and its internal slots.
func (r *Realm) newPromise() (*Derived, *promiseRecord) {
	rec := &promiseRecord{}
	return &Derived{Own: js.NewObject(), Proto: r.async.promiseProto, promise: rec}, rec
}

// promiseOf returns the internal slots of v when v is a promise (IsPromise).
func (r *Realm) promiseOf(v any) (*promiseRecord, bool) {
	d, ok := v.(*Derived)
	if !ok || d.promise == nil {
		return nil, false
	}
	return d.promise, true
}

// newPromiseCapability is NewPromiseCapability(%Promise%).
func (r *Realm) newPromiseCapability() *promiseCapability {
	p, rec := r.newPromise()
	resolve, reject := r.createResolvingFunctions(p, rec)
	return &promiseCapability{promise: p, resolve: resolve, reject: reject}
}

// promiseCapabilityFor is NewPromiseCapability(C): for a constructor other
// than %Promise%, new C(executor), where the executor records the resolving
// functions it is given.
func (r *Realm) promiseCapabilityFor(c any) (*promiseCapability, error) {
	if c == any(r.async.promiseCtor) {
		return r.newPromiseCapability(), nil
	}
	ctor, ok := c.(*js.Function)
	if !ok || !isConstructor(ctor) {
		return nil, js.NewTypeError(r.safeString(c) + " is not a constructor")
	}
	var resolveFn, rejectFn any = js.Undefined, js.Undefined
	executor := NewNative("", 2, func(_ any, args []any) (any, error) {
		if !isUndefined(resolveFn) || !isUndefined(rejectFn) {
			return nil, js.NewTypeError("Promise executor has already been invoked with non-undefined arguments")
		}
		resolveFn, rejectFn = js.Arg(args, 0), js.Arg(args, 1)
		return js.Undefined, nil
	})
	p, err := r.construct(ctor, []any{executor}, ctor.Name)
	if err != nil {
		return nil, err
	}
	resolve, ok1 := resolveFn.(*js.Function)
	reject, ok2 := rejectFn.(*js.Function)
	if !ok1 || !ok2 {
		return nil, js.NewTypeError("Promise resolve or reject function is not callable")
	}
	return &promiseCapability{promise: p, resolve: r.resolvingFunction(resolve), reject: r.resolvingFunction(reject)}, nil
}

// resolvingFunction is the Go side of a capability's resolve or reject
// function: the intrinsic one itself when f wraps it (the case of every
// subclass of Promise), otherwise a call of f. The realm has no
// HostReportErrors for the jobs that settle promises, so what such a
// function of a constructor unrelated to Promise throws is dropped.
func (r *Realm) resolvingFunction(f *js.Function) func(any) {
	if h := hostOf(f, false); h != nil && h.resolving != nil {
		return h.resolving
	}
	return func(v any) {
		_, _ = r.call(f, js.Undefined, []any{v})
	}
}

// speciesConstructor is SpeciesConstructor(promise, %Promise%): the
// promise's constructor when it is %Promise% or inherits from it (whose
// Symbol.species getter returns the constructor itself), else %Promise%.
func (r *Realm) speciesConstructor(promise any) (any, error) {
	c, err := r.get(promise, "constructor")
	if err != nil {
		return nil, err
	}
	if isUndefined(c) {
		return r.async.promiseCtor, nil
	}
	if !js.IsObject(c) {
		return nil, js.NewTypeError("The .constructor property is not an object")
	}
	if c == any(r.async.promiseCtor) || r.inheritsFrom(c, r.async.promiseCtor) {
		return c, nil
	}
	return r.async.promiseCtor, nil
}

// createResolvingFunctions is CreateResolvingFunctions: a resolve and a reject
// function sharing one [[AlreadyResolved]] flag, so only the first call of
// either has an effect.
func (r *Realm) createResolvingFunctions(p *Derived, rec *promiseRecord) (resolve, reject func(any)) {
	alreadyResolved := false
	resolve = func(resolution any) {
		if !alreadyResolved {
			alreadyResolved = true
			r.resolvePromise(p, rec, resolution)
		}
	}
	reject = func(reason any) {
		if !alreadyResolved {
			alreadyResolved = true
			r.settlePromise(rec, promiseRejected, reason)
		}
	}
	return resolve, reject
}

// resolverFunction is a promise resolve or reject function as JavaScript sees
// it: an anonymous function of length 1 that returns undefined.
func resolverFunction(f func(any)) *js.Function {
	fn := NewNative("", 1, func(_ any, args []any) (any, error) {
		f(js.Arg(args, 0))
		return js.Undefined, nil
	})
	fn.Internal = &hostFunction{resolving: f}
	return fn
}

// resolvePromise is the body of a promise resolve function once it has checked
// [[AlreadyResolved]]: a thenable resolution is adopted through a
// PromiseResolveThenableJob, anything else fulfils the promise.
func (r *Realm) resolvePromise(p *Derived, rec *promiseRecord, resolution any) {
	if resolution == any(p) {
		r.settlePromise(rec, promiseRejected, js.NewTypeError("Chaining cycle detected for promise #<Promise>"))
		return
	}
	if !js.IsObject(resolution) {
		r.settlePromise(rec, promiseFulfilled, resolution)
		return
	}
	then, err := r.getV(resolution, "then")
	if err != nil {
		r.settlePromise(rec, promiseRejected, caughtValue(err))
		return
	}
	thenFn, ok := then.(*js.Function)
	if !ok {
		r.settlePromise(rec, promiseFulfilled, resolution)
		return
	}
	r.enqueueJob(func() {
		// PromiseResolveThenableJob: then is called with fresh resolving
		// functions, and a throw from it rejects the promise unless one of them
		// already ran.
		resolve, reject := r.createResolvingFunctions(p, rec)
		if _, err := r.call(thenFn, resolution, []any{resolverFunction(resolve), resolverFunction(reject)}); err != nil {
			reject(caughtValue(err))
		}
	})
}

// settlePromise is FulfillPromise or RejectPromise: it records the outcome and
// queues the reactions registered so far (TriggerPromiseReactions).
func (r *Realm) settlePromise(rec *promiseRecord, state promiseState, v any) {
	reactions := rec.fulfillReactions
	if state == promiseRejected {
		reactions = rec.rejectReactions
	}
	rec.state, rec.result = state, v
	rec.fulfillReactions, rec.rejectReactions = nil, nil
	for _, re := range reactions {
		r.enqueueReaction(re, v)
	}
}

// enqueueReaction queues a PromiseReactionJob for reaction with argument.
func (r *Realm) enqueueReaction(re *promiseReaction, arg any) {
	r.enqueueJob(func() {
		if re.native != nil {
			re.native(arg)
			return
		}
		var (
			value any
			err   error
		)
		switch {
		case re.handler != nil:
			value, err = r.call(re.handler, js.Undefined, []any{arg})
		case re.rejects:
			err = ThrowValue(arg)
		default:
			value = arg
		}
		if re.capability == nil {
			return
		}
		if err != nil {
			re.capability.reject(caughtValue(err))
		} else {
			re.capability.resolve(value)
		}
	})
}

// performThen is PerformPromiseThen: it registers the two reactions, or queues
// the matching one at once when the promise has already settled.
func (r *Realm) performThen(rec *promiseRecord, onFulfilled, onRejected *promiseReaction) {
	onRejected.rejects = true
	switch rec.state {
	case promisePending:
		rec.fulfillReactions = append(rec.fulfillReactions, onFulfilled)
		rec.rejectReactions = append(rec.rejectReactions, onRejected)
	case promiseFulfilled:
		r.enqueueReaction(onFulfilled, rec.result)
	default:
		r.enqueueReaction(onRejected, rec.result)
	}
}

// thenNative registers Go continuations on a promise, as await and the host do:
// PerformPromiseThen without a result capability.
func (r *Realm) thenNative(rec *promiseRecord, onFulfilled, onRejected func(any)) {
	r.performThen(rec, &promiseReaction{native: onFulfilled}, &promiseReaction{native: onRejected})
}

// promiseThen is PerformPromiseThen(promise, onFulfilled, onRejected,
// NewPromiseCapability(%Promise%)) and returns the derived promise. Handlers that
// are not functions are empty.
func (r *Realm) promiseThen(rec *promiseRecord, onFulfilled, onRejected any) any {
	return r.promiseThenWith(rec, onFulfilled, onRejected, r.newPromiseCapability())
}

// promiseThenWith is PerformPromiseThen with the result capability given.
func (r *Realm) promiseThenWith(rec *promiseRecord, onFulfilled, onRejected any, capability *promiseCapability) any {
	f, _ := onFulfilled.(*js.Function)
	rj, _ := onRejected.(*js.Function)
	r.performThen(rec,
		&promiseReaction{capability: capability, handler: f},
		&promiseReaction{capability: capability, handler: rj})
	return capability.promise
}

// promiseResolve is PromiseResolve(C, x): x itself when it is a promise whose
// constructor property is C, otherwise a new promise of C resolved with x.
func (r *Realm) promiseResolve(c, x any) (any, error) {
	if _, ok := r.promiseOf(x); ok {
		ctor, err := r.getV(x, "constructor")
		if err != nil {
			return nil, err
		}
		if ctor == c {
			return x, nil
		}
	}
	capability, err := r.promiseCapabilityFor(c)
	if err != nil {
		return nil, err
	}
	capability.resolve(x)
	return capability.promise, nil
}

// intrinsicPromise is PromiseResolve(%Promise%, x) for the runtime's own
// awaits: the promise's internal slots.
func (r *Realm) intrinsicPromise(x any) (*promiseRecord, error) {
	p, err := r.promiseResolve(r.async.promiseCtor, x)
	if err != nil {
		return nil, err
	}
	rec, _ := r.promiseOf(p)
	return rec, nil
}

// invoke is Invoke(v, name, args): v[name](...args).
func (r *Realm) invoke(v any, name string, args ...any) (any, error) {
	f, err := r.getV(v, name)
	if err != nil {
		return nil, err
	}
	return r.callValue(f, v, args)
}

// setupPromise installs Promise, its prototype methods and statics, and
// AggregateError.
func (r *Realm) setupPromise() {
	proto := js.NewObject()
	r.async.promiseProto = proto
	ctor := r.constructor("Promise", 1, proto, func(any, []any) (any, error) {
		return nil, js.NewTypeError("Promise constructor cannot be invoked without 'new'")
	}, r.constructPromise)
	r.async.promiseCtor = ctor

	r.method(proto, "then", 2, func(this any, args []any) (any, error) {
		rec, ok := r.promiseOf(this)
		if !ok {
			return nil, js.NewTypeError("Method Promise.prototype.then called on incompatible receiver " + r.safeString(this))
		}
		c, err := r.speciesConstructor(this)
		if err != nil {
			return nil, err
		}
		capability, err := r.promiseCapabilityFor(c)
		if err != nil {
			return nil, err
		}
		return r.promiseThenWith(rec, js.Arg(args, 0), js.Arg(args, 1), capability), nil
	})
	r.method(proto, "catch", 1, func(this any, args []any) (any, error) {
		return r.invoke(this, "then", js.Undefined, js.Arg(args, 0))
	})
	r.method(proto, "finally", 1, func(this any, args []any) (any, error) {
		if !js.IsObject(this) {
			return nil, js.NewTypeError("Promise.prototype.finally called on non-object")
		}
		onFinally, ok := js.Arg(args, 0).(*js.Function)
		if !ok {
			return r.invoke(this, "then", js.Arg(args, 0), js.Arg(args, 0))
		}
		c, err := r.speciesConstructor(this)
		if err != nil {
			return nil, err
		}
		// Both handlers call onFinally, wait for what it returns, and then pass on
		// the original outcome.
		after := func(outcome *js.Function) (any, error) {
			res, err := r.call(onFinally, js.Undefined, nil)
			if err != nil {
				return nil, err
			}
			p, err := r.promiseResolve(c, res)
			if err != nil {
				return nil, err
			}
			return r.invoke(p, "then", outcome)
		}
		thenFinally := NewNative("", 1, func(_ any, args []any) (any, error) {
			value := js.Arg(args, 0)
			return after(NewNative("", 0, func(any, []any) (any, error) { return value, nil }))
		})
		catchFinally := NewNative("", 1, func(_ any, args []any) (any, error) {
			reason := js.Arg(args, 0)
			return after(NewNative("", 0, func(any, []any) (any, error) { return nil, ThrowValue(reason) }))
		})
		return r.invoke(this, "then", thenFinally, catchFinally)
	})

	r.method(ctor, "resolve", 1, func(this any, args []any) (any, error) {
		if !js.IsObject(this) {
			return nil, js.NewTypeError("PromiseResolve called on non-object")
		}
		return r.promiseResolve(this, js.Arg(args, 0))
	})
	r.method(ctor, "reject", 1, func(this any, args []any) (any, error) {
		if !js.IsObject(this) {
			return nil, js.NewTypeError("PromiseReject called on non-object")
		}
		capability, err := r.promiseCapabilityFor(this)
		if err != nil {
			return nil, err
		}
		capability.reject(js.Arg(args, 0))
		return capability.promise, nil
	})
	r.combinator(ctor, "all", r.promiseAll)
	r.combinator(ctor, "allSettled", r.promiseAllSettled)
	r.combinator(ctor, "any", r.promiseAny)
	r.combinator(ctor, "race", r.promiseRace)

	r.setupAggregateError()
}

// constructPromise is new Promise(executor): the executor runs synchronously
// with the promise's resolving functions, and a throw from it rejects the promise.
func (r *Realm) constructPromise(args []any) (any, error) {
	executor, ok := js.Arg(args, 0).(*js.Function)
	if !ok {
		return nil, js.NotAFunction("Promise resolver " + r.safeString(js.Arg(args, 0)))
	}
	p, rec := r.newPromise()
	resolve, reject := r.createResolvingFunctions(p, rec)
	if _, err := r.call(executor, js.Undefined, []any{resolverFunction(resolve), resolverFunction(reject)}); err != nil {
		reject(caughtValue(err))
	}
	return p, nil
}

// combinatorStep handles the element at index of a Promise.all-style iteration:
// it returns the handlers to attach to the promise Promise.resolve made of it.
type combinatorStep func(index int) (onFulfilled, onRejected any)

// combinatorBody runs a combinator over the iterated values; it returns the step
// for each element and a function called once iteration ends.
type combinatorBody func(capability *promiseCapability) (step combinatorStep, finish func())

// combinator installs Promise.all, allSettled, any or race. They share the
// algorithm's frame: the receiver's resolve method is read once, each value is
// passed through it, and the handlers the body supplies are attached with then;
// an abrupt completion anywhere rejects the returned promise instead of
// throwing.
func (r *Realm) combinator(ctor *js.Function, name string, body combinatorBody) {
	r.method(ctor, name, 1, func(this any, args []any) (any, error) {
		if !js.IsObject(this) {
			return nil, js.NewTypeError("Promise." + name + " called on non-object")
		}
		capability, err := r.promiseCapabilityFor(this)
		if err != nil {
			return nil, err
		}
		fail := func(err error) (any, error) {
			capability.reject(caughtValue(err))
			return capability.promise, nil
		}
		resolveFn, err := r.getV(this, "resolve")
		if err != nil {
			return fail(err)
		}
		if _, ok := resolveFn.(*js.Function); !ok {
			return fail(js.NewTypeError("Promise resolve or reject function is not callable"))
		}
		iterable := js.Arg(args, 0)
		items, err := r.iterate(iterable, func() error { return notIterableValue(iterable) })
		if err != nil {
			return fail(err)
		}
		step, finish := body(capability)
		for i, item := range items {
			next, err := r.callValue(resolveFn, this, []any{item})
			if err != nil {
				return fail(err)
			}
			onFulfilled, onRejected := step(i)
			if _, err := r.invoke(next, "then", onFulfilled, onRejected); err != nil {
				return fail(err)
			}
		}
		finish()
		return capability.promise, nil
	})
}

// countdown tracks [[RemainingElements]]: it starts at one for the iteration
// itself, and done runs when the last pending element (or the end of iteration)
// counts it down to zero.
type countdown struct {
	remaining int
	done      func()
}

func (c *countdown) add() { c.remaining++ }

func (c *countdown) settle() {
	c.remaining--
	if c.remaining == 0 {
		c.done()
	}
}

// elementFunction is a Promise.all resolve element function (or one of its
// siblings): it records its value once; later calls are ignored.
func elementFunction(alreadyCalled *bool, record func(any)) *js.Function {
	return NewNative("", 1, func(_ any, args []any) (any, error) {
		if !*alreadyCalled {
			*alreadyCalled = true
			record(js.Arg(args, 0))
		}
		return js.Undefined, nil
	})
}

// promiseAll is Promise.all: the values in iteration order once all fulfil, or the
// first rejection.
func (r *Realm) promiseAll(capability *promiseCapability) (combinatorStep, func()) {
	var values []any
	count := &countdown{remaining: 1, done: func() {
		capability.resolve(js.NewArray(values...))
	}}
	step := func(i int) (any, any) {
		values = append(values, js.Undefined)
		count.add()
		called := false
		return elementFunction(&called, func(v any) {
			values[i] = v
			count.settle()
		}), resolverFunction(capability.reject)
	}
	return step, count.settle
}

// promiseAllSettled is Promise.allSettled: an object describing each outcome once
// every promise has settled.
func (r *Realm) promiseAllSettled(capability *promiseCapability) (combinatorStep, func()) {
	var values []any
	count := &countdown{remaining: 1, done: func() {
		capability.resolve(js.NewArray(values...))
	}}
	step := func(i int) (any, any) {
		values = append(values, js.Undefined)
		count.add()
		called := false
		onFulfilled := elementFunction(&called, func(v any) {
			values[i] = js.ObjectOf("status", "fulfilled", "value", v)
			count.settle()
		})
		onRejected := elementFunction(&called, func(v any) {
			values[i] = js.ObjectOf("status", "rejected", "reason", v)
			count.settle()
		})
		return onFulfilled, onRejected
	}
	return step, count.settle
}

// promiseAny is Promise.any: the first fulfilment, or an AggregateError holding
// every rejection reason in iteration order.
func (r *Realm) promiseAny(capability *promiseCapability) (combinatorStep, func()) {
	var reasons []any
	count := &countdown{remaining: 1, done: func() {
		capability.reject(r.newAggregateError(js.NewArray(reasons...), "All promises were rejected"))
	}}
	step := func(i int) (any, any) {
		reasons = append(reasons, js.Undefined)
		count.add()
		called := false
		return resolverFunction(capability.resolve), elementFunction(&called, func(v any) {
			reasons[i] = v
			count.settle()
		})
	}
	return step, count.settle
}

// promiseRace is Promise.race: the first promise to settle decides.
func (r *Realm) promiseRace(capability *promiseCapability) (combinatorStep, func()) {
	step := func(int) (any, any) {
		return resolverFunction(capability.resolve), resolverFunction(capability.reject)
	}
	return step, func() {}
}

// newAggregateError builds an AggregateError with its errors list.
func (r *Realm) newAggregateError(errors *js.Array, msg string) *js.Error {
	return &js.Error{Name: "AggregateError", Message: msg, Props: js.ObjectOf("errors", errors)}
}

// setupAggregateError installs AggregateError(errors, message), whose prototype
// inherits from Error.prototype.
func (r *Realm) setupAggregateError() {
	const name = "AggregateError"
	proto := js.NewObject()
	r.errorProtos[name] = proto
	r.protoLinks[proto] = r.errorProtos["Error"]
	r.define(proto, "name", name)
	r.define(proto, "message", "")
	build := func(args []any) (any, error) {
		msg := ""
		if m := js.Arg(args, 1); !isUndefined(m) {
			s, err := r.toString(m)
			if err != nil {
				return nil, err
			}
			msg = s
		}
		errs := js.Arg(args, 0)
		items, err := r.iterate(errs, func() error { return notIterableValue(errs) })
		if err != nil {
			return nil, err
		}
		return r.newAggregateError(js.NewArray(items...), msg), nil
	}
	r.constructor(name, 2, proto, func(_ any, args []any) (any, error) { return build(args) }, build)
}
