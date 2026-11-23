package jsrt

import (
	"errors"
	"runtime"

	"github.com/edge-js/edge-go/js"
)

// Async generators (ECMAScript §27.6) and for await. An async generator's body
// runs as a coroutine like a generator's; it suspends at an await as an async
// function does and at a yield as a generator does. Requests (next, return,
// throw) are queued: each returns a promise that the body settles when it
// reaches its next yield or its end, in the order they were made.

// asyncGenState is an async generator's [[AsyncGeneratorState]].
type asyncGenState uint8

const (
	asyncGenSuspendedStart asyncGenState = iota
	asyncGenSuspendedYield
	asyncGenExecuting
	// asyncGenDrainingQueue is the state while the queued requests of a
	// generator that has ended are being settled.
	asyncGenDrainingQueue
	asyncGenCompleted
)

// asyncGenRequest is an AsyncGeneratorRequest: the completion a next, return
// or throw call resumes the body with, and the promise it returned.
type asyncGenRequest struct {
	mode       resumeMode
	value      any
	capability *promiseCapability
}

// asyncGenerator holds an async generator object's internal slots.
type asyncGenerator struct {
	state asyncGenState
	queue []*asyncGenRequest
	// run is the body, until the first next starts it.
	run func() (any, error)
	co  *coroutine
}

// asyncGeneratorIntrinsics are %AsyncGeneratorFunction.prototype%, the
// prototype of async generator functions, and %AsyncGeneratorPrototype%,
// which async generator objects inherit through their function's prototype.
type asyncGeneratorIntrinsics struct {
	functionProto, proto *js.Object
}

// setupAsyncGenerators creates %AsyncGeneratorFunction%, its prototype,
// %AsyncGeneratorPrototype% with next, return and throw, and
// %AsyncIteratorPrototype% under it. AsyncGeneratorFunction is not a global;
// scripts reach it as Object.getPrototypeOf(async function* () {}).constructor.
func (r *Realm) setupAsyncGenerators() {
	fp := js.NewObject()
	r.protoLinks[fp] = r.functionProto
	asyncIteratorProto := js.NewObject()
	gp := js.NewObject()
	r.protoLinks[gp] = asyncIteratorProto
	r.asyncGenerators.functionProto, r.asyncGenerators.proto = fp, gp
	ctor := NewNative("AsyncGeneratorFunction", 1, func(_ any, args []any) (any, error) {
		return r.dynamicFunction("async function*", args)
	})
	hostOf(ctor, true).construct = func(args []any) (any, error) {
		return r.dynamicFunction("async function*", args)
	}
	r.extraProps(ctor, true).Define("prototype", fp, attrsConstant)
	fp.Define("constructor", ctor, js.ReadOnly|js.Hidden)
	fp.Define("prototype", gp, js.ReadOnly|js.Hidden)
	gp.Define("constructor", fp, js.ReadOnly|js.Hidden)
	request := func(name string, mode resumeMode) {
		r.method(gp, name, 1, func(this any, args []any) (any, error) {
			capability := r.newPromiseCapability()
			g := asyncGeneratorOf(this)
			if g == nil {
				capability.reject(js.NewTypeError("Method [AsyncGenerator].prototype." + name + " called on incompatible receiver " + r.safeString(this)))
				return capability.promise, nil
			}
			r.asyncGeneratorRequest(g, mode, js.Arg(args, 0), capability)
			return capability.promise, nil
		})
	}
	request("next", resumeNext)
	request("return", resumeReturn)
	request("throw", resumeThrow)
}

// asyncGeneratorOf returns the slots of an async generator object, nil for
// other values.
func asyncGeneratorOf(v any) *asyncGenerator {
	if d, ok := v.(*Derived); ok {
		return d.asyncGen
	}
	return nil
}

// newAsyncGenerator is a call of the async generator function f whose
// parameters are bound in fe: the generator object, whose prototype is
// f.prototype (%AsyncGeneratorPrototype% when that is not an object), with
// the body waiting for the first request.
func (r *Realm) newAsyncGenerator(f *js.Function, fi *funcInfo, fe *env) (any, error) {
	r.abandonOrphans()
	proto, err := r.getV(f, "prototype")
	if err != nil {
		return nil, err
	}
	if !js.IsObject(proto) {
		proto = r.asyncGenerators.proto
	}
	g := &asyncGenerator{run: func() (any, error) { return r.runBody(fi, fe) }}
	fe.agen = g
	obj := &Derived{Own: js.NewObject(), Proto: proto, asyncGen: g}
	// Like a generator, an async generator left suspended at a yield keeps a
	// goroutine parked until its object is unreachable.
	runtime.SetFinalizer(obj, func(*Derived) { r.generators.orphan(g) })
	return obj, nil
}

// asyncGeneratorRequest is the common part of next, return and throw: the
// request is queued, and the body resumed when it is suspended, or the
// queue settled when the body has ended.
func (r *Realm) asyncGeneratorRequest(g *asyncGenerator, mode resumeMode, v any, capability *promiseCapability) {
	if mode == resumeThrow && g.state == asyncGenSuspendedStart {
		g.state, g.run = asyncGenCompleted, nil
	}
	if g.state == asyncGenCompleted {
		switch mode {
		case resumeNext:
			capability.resolve(iterResult(js.Undefined, true))
			return
		case resumeThrow:
			capability.reject(v)
			return
		}
	}
	g.queue = append(g.queue, &asyncGenRequest{mode: mode, value: v, capability: capability})
	switch {
	case mode == resumeReturn && (g.state == asyncGenSuspendedStart || g.state == asyncGenCompleted):
		g.state, g.run = asyncGenDrainingQueue, nil
		r.asyncGeneratorAwaitReturn(g)
	case g.state == asyncGenSuspendedStart || g.state == asyncGenSuspendedYield:
		r.asyncGeneratorResume(g, mode, v)
	}
}

// asyncGeneratorResume is AsyncGeneratorResume: the body runs from where it
// is suspended until it suspends again.
func (r *Realm) asyncGeneratorResume(g *asyncGenerator, mode resumeMode, v any) {
	prev := g.state
	g.state = asyncGenExecuting
	if prev == asyncGenSuspendedStart {
		run := g.run
		g.run = nil
		g.co = r.startCoroutine(func() {
			res, err := run()
			r.asyncGeneratorFinish(g, res, err)
		})
		return
	}
	r.switchTo(g.co, resumptionFor(mode, v))
}

// resumptionFor is the resumption a request delivers to a suspended yield.
func resumptionFor(mode resumeMode, v any) resumption {
	switch mode {
	case resumeReturn:
		return resumption{err: &generatorReturn{v}}
	case resumeThrow:
		return resumption{err: ThrowValue(v)}
	}
	return resumption{value: v}
}

// asyncGeneratorFinish runs when the body ends: the head request gets the
// body's return value or exception, and the queue is drained.
func (r *Realm) asyncGeneratorFinish(g *asyncGenerator, res any, err error) {
	g.state, g.co = asyncGenDrainingQueue, nil
	var ret *generatorReturn
	if errors.As(err, &ret) {
		res, err = ret.value, nil
	}
	r.asyncGeneratorCompleteStep(g, res, err, true)
	r.asyncGeneratorDrainQueue(g)
}

// asyncGeneratorCompleteStep is AsyncGeneratorCompleteStep: the head request
// is removed and its promise settled with the iterator result for v, or
// rejected with err.
func (r *Realm) asyncGeneratorCompleteStep(g *asyncGenerator, v any, err error, done bool) {
	req := g.queue[0]
	g.queue[0] = nil
	g.queue = g.queue[1:]
	if err != nil {
		req.capability.reject(caughtValue(err))
		return
	}
	req.capability.resolve(iterResult(v, done))
}

// asyncGeneratorDrainQueue is AsyncGeneratorDrainQueue: the requests queued
// behind the end of the body settle in order; a return request awaits its
// value first.
func (r *Realm) asyncGeneratorDrainQueue(g *asyncGenerator) {
	for len(g.queue) > 0 {
		req := g.queue[0]
		switch req.mode {
		case resumeReturn:
			r.asyncGeneratorAwaitReturn(g)
			return
		case resumeThrow:
			r.asyncGeneratorCompleteStep(g, nil, ThrowValue(req.value), true)
		default:
			r.asyncGeneratorCompleteStep(g, js.Undefined, nil, true)
		}
	}
	g.state = asyncGenCompleted
}

// asyncGeneratorAwaitReturn is AsyncGeneratorAwaitReturn: the value of the
// head return request is awaited, and settles the request as the end of the
// generator, before the rest of the queue drains.
func (r *Realm) asyncGeneratorAwaitReturn(g *asyncGenerator) {
	req := g.queue[0]
	rec, err := r.intrinsicPromise(req.value)
	if err != nil {
		r.asyncGeneratorCompleteStep(g, nil, err, true)
		r.asyncGeneratorDrainQueue(g)
		return
	}
	r.thenNative(rec, func(x any) {
		r.asyncGeneratorCompleteStep(g, x, nil, true)
		r.asyncGeneratorDrainQueue(g)
	}, func(x any) {
		r.asyncGeneratorCompleteStep(g, nil, ThrowValue(x), true)
		r.asyncGeneratorDrainQueue(g)
	})
}

// asyncGeneratorYield is AsyncGeneratorYield for the value v, already
// awaited: the head request is fulfilled with it, and the body continues
// with the next queued request, or suspends until one comes.
func (r *Realm) asyncGeneratorYield(g *asyncGenerator, v any) (any, error) {
	r.asyncGeneratorCompleteStep(g, v, nil, false)
	var msg resumption
	if len(g.queue) > 0 {
		req := g.queue[0]
		msg = resumptionFor(req.mode, req.value)
	} else {
		g.state = asyncGenSuspendedYield
		var err error
		msg.value, err = r.park(r.async.current)
		msg.err = err
	}
	return r.unwrapYieldResumption(msg)
}

// unwrapYieldResumption is AsyncGeneratorUnwrapYieldResumption: a return
// request's value is awaited before it unwinds the body.
func (r *Realm) unwrapYieldResumption(msg resumption) (any, error) {
	var ret *generatorReturn
	if !errors.As(msg.err, &ret) {
		return msg.value, msg.err
	}
	v, err := r.await(ret.value)
	if err != nil {
		return nil, err
	}
	return nil, &generatorReturn{v}
}

// asyncIterator is an iterator as for await and yield* in an async generator
// use it: next, throw and return calls whose results have been awaited. throw
// and ret are nil when the iterator has no such method.
type asyncIterator struct {
	next, throw, ret func(v any) (any, error)
}

// asyncIteratorOf is GetIterator(v, async): an async generator object
// directly, any other iterable through CreateAsyncFromSyncIterator, whose
// results carry their awaited values. ok is false for a value that is not
// iterable.
func (r *Realm) asyncIteratorOf(v any) (*asyncIterator, bool) {
	if g := asyncGeneratorOf(v); g != nil {
		call := func(mode resumeMode) func(any) (any, error) {
			return func(x any) (any, error) {
				capability := r.newPromiseCapability()
				r.asyncGeneratorRequest(g, mode, x, capability)
				return r.await(capability.promise)
			}
		}
		return &asyncIterator{next: call(resumeNext), throw: call(resumeThrow), ret: call(resumeReturn)}, true
	}
	// AsyncFromSyncIteratorContinuation: the promise of the iterator result
	// once the sync result's value has settled, awaited.
	fromSync := func(res any, err error) (any, error) {
		if err != nil {
			return nil, err
		}
		out := res.(*js.Object)
		wrapper, err := r.intrinsicPromise(out.Get("value"))
		if err != nil {
			return nil, err
		}
		done := out.Get("done") == true
		unwrap := NewNative("", 1, func(_ any, args []any) (any, error) {
			return iterResult(js.Arg(args, 0), done), nil
		})
		return r.await(r.promiseThen(wrapper, unwrap, js.Undefined))
	}
	if g := generatorOf(v); g != nil {
		call := func(mode resumeMode) func(any) (any, error) {
			return func(x any) (any, error) { return fromSync(r.resumeGenerator(g, mode, x)) }
		}
		return &asyncIterator{next: call(resumeNext), throw: call(resumeThrow), ret: call(resumeReturn)}, true
	}
	it, ok := r.openIterator(v)
	if !ok {
		return nil, false
	}
	return &asyncIterator{next: func(any) (any, error) {
		item, more, err := it.step()
		if err != nil {
			return nil, err
		}
		return fromSync(iterResult(item, !more), nil)
	}}, true
}

// asyncIteratorClose is AsyncIteratorClose after the completion err of a loop
// body (nil for break or return): the iterator's return is called and
// awaited, and err wins over what that throws.
func (r *Realm) asyncIteratorClose(it *asyncIterator, err error) error {
	if it.ret == nil {
		return err
	}
	res, cerr := it.ret(js.Undefined)
	if err != nil && !isGeneratorReturn(err) {
		return err
	}
	if cerr != nil {
		return cerr
	}
	if !js.IsObject(res) {
		return js.NewTypeError("Iterator result " + r.safeString(res) + " is not an object")
	}
	return err
}

// iteratorStep reads an awaited iterator result: its value, and whether the
// iteration goes on.
func (r *Realm) iteratorStep(res any) (any, bool, error) {
	if !js.IsObject(res) {
		return nil, false, js.NewTypeError("Iterator result " + r.safeString(res) + " is not an object")
	}
	done, err := r.get(res, "done")
	if err != nil {
		return nil, false, err
	}
	if js.ToBoolean(done) {
		v, err := r.get(res, "value")
		return v, false, err
	}
	v, err := r.get(res, "value")
	return v, true, err
}

// evalAsyncYield evaluates yield and yield* in the body of the async
// generator g. A yielded value is awaited before it is handed out; yield*
// hands out the values of an async iterator (or of a sync one, awaited) and
// passes the requests it receives on to it.
func (r *Realm) evalAsyncYield(g *asyncGenerator, x *yieldExpr, e *env) (any, error) {
	var v any = js.Undefined
	if x.arg != nil {
		var err error
		if v, err = r.eval(x.arg, e); err != nil {
			return nil, err
		}
	}
	if !x.delegate {
		awaited, err := r.await(v)
		if err != nil {
			return nil, err
		}
		return r.asyncGeneratorYield(g, awaited)
	}
	it, ok := r.asyncIteratorOf(v)
	if !ok {
		return nil, js.NotAFunction("undefined")
	}
	res, err := it.next(js.Undefined)
	for {
		if err != nil {
			return nil, err
		}
		val, more, serr := r.iteratorStep(res)
		if serr != nil {
			return nil, serr
		}
		if !more {
			return val, nil
		}
		received, rerr := r.asyncGeneratorYield(g, val)
		var ret *generatorReturn
		switch {
		case errors.As(rerr, &ret):
			if it.ret == nil {
				return nil, rerr
			}
			if res, err = it.ret(ret.value); err != nil {
				return nil, err
			}
			if val, more, err = r.iteratorStep(res); err != nil {
				return nil, err
			}
			if !more {
				return nil, &generatorReturn{val}
			}
			// The inner iterator went on: what it produced is yielded.
			res = iterResult(val, false)
		case rerr != nil:
			if it.throw == nil {
				if cerr := r.asyncIteratorClose(it, nil); cerr != nil {
					return nil, cerr
				}
				return nil, js.NewTypeError("The iterator does not provide a 'throw' method.")
			}
			res, err = it.throw(CaughtValue(rerr))
		default:
			res, err = it.next(received)
		}
	}
}

// execForAwait runs for await (left of right): each result of the async
// iterator of the value of right is awaited, and its value bound and run
// through the body; leaving the loop early closes the iterator.
func (r *Realm) execForAwait(x *forInOf, rv any, e *env, labels []string) (ctrl, error) {
	if js.IsNullish(rv) {
		return normal, js.CannotReadProperties(rv, "Symbol(Symbol.asyncIterator)")
	}
	it, ok := r.asyncIteratorOf(rv)
	if !ok {
		return normal, js.NewTypeError(renderIterable(x.right) + " is not async iterable")
	}
	for {
		res, err := it.next(js.Undefined)
		if err != nil {
			return normal, err
		}
		v, more, err := r.iteratorStep(res)
		if err != nil || !more {
			return normal, err
		}
		ie, err := r.bindLoopValue(x, v, e)
		if err != nil {
			return normal, r.asyncIteratorClose(it, err)
		}
		c, err := r.exec(x.body, ie)
		if err != nil {
			return normal, r.asyncIteratorClose(it, err)
		}
		if more, out := loopContinues(c, labels); !more {
			return out, r.asyncIteratorClose(it, nil)
		}
	}
}
