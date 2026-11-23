package jsrt

import (
	"errors"
	"runtime"
	"sync"

	"github.com/edge-js/edge-go/js"
)

// Generator functions. Calling one binds its parameters and returns a
// generator object; the body runs as a coroutine (see async.go) that the
// object's next, return and throw resume, and a yield hands control back to
// the caller with the yielded value, exactly as await hands it to the job
// loop.

// generatorState is a generator's [[GeneratorState]].
type generatorState uint8

const (
	generatorSuspendedStart generatorState = iota
	generatorSuspendedYield
	generatorExecuting
	generatorCompleted
)

// generatorRecord holds a generator object's internal slots.
type generatorRecord struct {
	state generatorState
	// run is the body, until the first next starts it.
	run func() (any, error)
	co  *coroutine
	// yielded is the value of the yield the body is suspended at.
	yielded any
	// finished, result and err are the body's completion once it has ended.
	finished bool
	result   any
	err      error
}

// generatorReturn is the completion gen.return(value) resumes a suspended
// yield with: it unwinds the body like a return statement, through finally
// blocks but past catch clauses.
type generatorReturn struct{ value any }

// Error describes the completion; it is never shown, as the completion ends
// at the generator it unwinds.
func (g *generatorReturn) Error() string { return "generator return" }

// isGeneratorReturn reports a return completion travelling as an error.
func isGeneratorReturn(err error) bool {
	var ret *generatorReturn
	return errors.As(err, &ret)
}

// generatorIntrinsics is the realm's share of generators: the intrinsics, and
// the generators and async generators whose objects became unreachable, which
// a finalizer records from another goroutine.
type generatorIntrinsics struct {
	// functionProto is %GeneratorFunction.prototype%, the prototype of
	// generator functions; proto is %GeneratorPrototype%.
	functionProto, proto *js.Object
	mu                   sync.Mutex
	orphans              []suspendable
}

// suspendable is a generator or an async generator, whose body may be parked
// at a yield.
type suspendable interface {
	// parkedAtYield returns the coroutine of a body suspended at a yield, nil
	// when the body is not.
	parkedAtYield() *coroutine
}

func (g *generatorRecord) parkedAtYield() *coroutine {
	if g.state == generatorSuspendedYield {
		return g.co
	}
	return nil
}

func (g *asyncGenerator) parkedAtYield() *coroutine {
	if g.state == asyncGenSuspendedYield {
		return g.co
	}
	return nil
}

// orphan records a generator whose object has become unreachable. It runs in
// a finalizer, on another goroutine than the realm's.
func (gi *generatorIntrinsics) orphan(g suspendable) {
	gi.mu.Lock()
	gi.orphans = append(gi.orphans, g)
	gi.mu.Unlock()
}

// setupGenerators creates %GeneratorFunction%, its prototype and
// %GeneratorPrototype% with next, return and throw. GeneratorFunction is not a
// global; scripts reach it as Object.getPrototypeOf(function* () {}).constructor.
func (r *Realm) setupGenerators() {
	fp := js.NewObject()
	r.protoLinks[fp] = r.functionProto
	gp := js.NewObject()
	r.protoLinks[gp] = r.iteratorProto
	r.generators.functionProto, r.generators.proto = fp, gp
	ctor := NewNative("GeneratorFunction", 1, func(_ any, args []any) (any, error) {
		return r.dynamicFunction("function*", args)
	})
	hostOf(ctor, true).construct = func(args []any) (any, error) {
		return r.dynamicFunction("function*", args)
	}
	r.extraProps(ctor, true).Define("prototype", fp, attrsConstant)
	fp.Define("constructor", ctor, js.ReadOnly|js.Hidden)
	fp.Define("prototype", gp, js.ReadOnly|js.Hidden)
	gp.Define("constructor", fp, js.ReadOnly|js.Hidden)
	resume := func(name string, mode resumeMode) {
		r.method(gp, name, 1, func(this any, args []any) (any, error) {
			g := generatorOf(this)
			if g == nil {
				return nil, js.NewTypeError("Method [Generator].prototype." + name + " called on incompatible receiver " + r.safeString(this))
			}
			return r.resumeGenerator(g, mode, js.Arg(args, 0))
		})
	}
	resume("next", resumeNext)
	resume("return", resumeReturn)
	resume("throw", resumeThrow)
}

// isGeneratorFunction reports a function defined by a generator function or
// method.
func isGeneratorFunction(f *js.Function) bool {
	c, ok := closureOf(f)
	return ok && c.fi.generator
}

// generatorOf returns the slots of a generator object, nil for other values.
func generatorOf(v any) *generatorRecord {
	if d, ok := v.(*Derived); ok {
		return d.generator
	}
	return nil
}

// newGenerator is a call of the generator function f whose parameters are
// bound: it returns the generator object, whose prototype is f.prototype
// (%GeneratorPrototype% when that is not an object), with the body run
// waiting for the first next.
func (r *Realm) newGenerator(f *js.Function, run func() (any, error), fe *env) (any, error) {
	r.abandonOrphans()
	proto, err := r.getV(f, "prototype")
	if err != nil {
		return nil, err
	}
	if !js.IsObject(proto) {
		proto = r.generators.proto
	}
	g := &generatorRecord{run: run}
	fe.gen = g
	obj := &Derived{Own: js.NewObject(), Proto: proto, generator: g}
	// A generator left suspended at a yield keeps a goroutine parked; once the
	// object is unreachable nothing can resume it, so it is abandoned.
	runtime.SetFinalizer(obj, func(*Derived) { r.generators.orphan(g) })
	return obj, nil
}

// abandonOrphans ends the coroutines of unreachable generators suspended at a
// yield. It runs when a generator is created and when the host's outermost
// Await returns, so parked goroutines do not outlive their objects by more
// than one of those.
func (r *Realm) abandonOrphans() {
	r.generators.mu.Lock()
	orphans := r.generators.orphans
	r.generators.orphans = nil
	r.generators.mu.Unlock()
	for _, g := range orphans {
		if co := g.parkedAtYield(); co != nil {
			r.switchTo(co, resumption{abandon: true})
		}
	}
}

// resumeMode is the method a generator is resumed by.
type resumeMode uint8

const (
	resumeNext resumeMode = iota
	resumeReturn
	resumeThrow
)

// resumeGenerator is GeneratorResume and GeneratorResumeAbrupt: it runs the
// body from where it is suspended until its next yield or its end, and
// returns the iterator result.
func (r *Realm) resumeGenerator(g *generatorRecord, mode resumeMode, v any) (any, error) {
	switch g.state {
	case generatorExecuting:
		return nil, js.NewTypeError("Generator is already running")
	case generatorSuspendedStart:
		if mode != resumeNext {
			g.state, g.run = generatorCompleted, nil
			break
		}
		run := g.run
		g.run = nil
		g.state = generatorExecuting
		g.co = r.startCoroutine(func() {
			res, err := run()
			g.finished, g.result, g.err = true, res, err
		})
		return r.generatorStep(g)
	case generatorSuspendedYield:
		g.state = generatorExecuting
		msg := resumption{value: v}
		switch mode {
		case resumeReturn:
			msg = resumption{err: &generatorReturn{v}}
		case resumeThrow:
			msg = resumption{err: ThrowValue(v)}
		}
		r.switchTo(g.co, msg)
		return r.generatorStep(g)
	}
	// A completed generator.
	switch mode {
	case resumeReturn:
		return iterResult(v, true), nil
	case resumeThrow:
		return nil, ThrowValue(v)
	}
	return iterResult(js.Undefined, true), nil
}

// generatorStep reports where the body stopped after a resumption: at a
// yield, or at its end, whose return value, return completion or exception
// completes the generator.
func (r *Realm) generatorStep(g *generatorRecord) (any, error) {
	if !g.finished {
		g.state = generatorSuspendedYield
		v := g.yielded
		g.yielded = nil
		return iterResult(v, false), nil
	}
	g.state, g.co = generatorCompleted, nil
	res, err := g.result, g.err
	g.result, g.err = nil, nil
	var ret *generatorReturn
	switch {
	case errors.As(err, &ret):
		return iterResult(ret.value, true), nil
	case err != nil:
		return nil, err
	}
	return iterResult(res, true), nil
}

// iterResult is CreateIterResultObject.
func iterResult(v any, done bool) *js.Object {
	return js.ObjectOf("value", v, "done", done)
}

// generatorYield suspends the running generator body at yield v and returns
// what it is resumed with: the argument of next, the exception of throw, or
// a generatorReturn for return.
func (r *Realm) generatorYield(g *generatorRecord, v any) (any, error) {
	g.yielded = v
	return r.park(r.async.current)
}

// evalYield evaluates yield and yield* in the body of the generator whose
// function scope encloses e.
func (r *Realm) evalYield(x *yieldExpr, e *env) (any, error) {
	vs := varScope(e)
	if vs.agen != nil {
		return r.evalAsyncYield(vs.agen, x, e)
	}
	g := vs.gen
	var v any = js.Undefined
	if x.arg != nil {
		var err error
		if v, err = r.eval(x.arg, e); err != nil {
			return nil, err
		}
	}
	if !x.delegate {
		return r.generatorYield(g, v)
	}
	if inner := generatorOf(v); inner != nil {
		return r.yieldFromGenerator(g, inner)
	}
	// Any other iterable is one whose iterator, like the builtin ones, has
	// neither throw nor return.
	return js.Undefined, r.forEachIterated(v, func() error { return r.notIterable(x.arg, v) }, func(item any) error {
		_, err := r.generatorYield(g, item)
		if err != nil && !isGeneratorReturn(err) {
			return js.NewTypeError("The iterator does not provide a 'throw' method.")
		}
		return err
	})
}

// yieldFromGenerator is yield* of another generator: every resumption of g
// is passed on to inner, every value inner yields is yielded by g, and the
// value inner returns is the value of the expression.
func (r *Realm) yieldFromGenerator(g, inner *generatorRecord) (any, error) {
	mode, received := resumeNext, any(js.Undefined)
	for {
		res, err := r.resumeGenerator(inner, mode, received)
		if err != nil {
			return nil, err
		}
		out := res.(*js.Object)
		if done := out.Get("done"); done == true {
			if mode == resumeReturn {
				return nil, &generatorReturn{out.Get("value")}
			}
			return out.Get("value"), nil
		}
		received, err = r.generatorYield(g, out.Get("value"))
		var ret *generatorReturn
		switch {
		case errors.As(err, &ret):
			mode, received = resumeReturn, ret.value
		case err != nil:
			mode, received = resumeThrow, CaughtValue(err)
		default:
			mode = resumeNext
		}
	}
}

// closeGenerator is IteratorClose for a generator a loop or destructuring
// leaves early: it calls return() and passes on what that throws.
func (r *Realm) closeGenerator(g *generatorRecord) error {
	_, err := r.resumeGenerator(g, resumeReturn, js.Undefined)
	return err
}
