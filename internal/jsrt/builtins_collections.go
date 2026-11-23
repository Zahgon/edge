package jsrt

import (
	"math"

	"github.com/edge-js/edge-go/js"
)

// Map, Set, WeakMap, WeakSet, their iterators, and the iteration protocol the
// interpreter uses for for-of, spread, destructuring and the builtins that take
// iterables.

// orderedEntries is the entry list of a Map or Set: insertion order, with
// deleted entries left in place as holes so that live iterators keep their
// position, as the specification's List of Records does.
type orderedEntries struct {
	keys, values []any
	deleted      []bool
	// index maps a normalized key to its live entry.
	index map[any]int
	size  int
}

func newOrderedEntries() *orderedEntries {
	return &orderedEntries{index: map[any]int{}}
}

// nanKey stands for NaN in the index, since NaN is not equal to itself in Go.
type nanKey struct{}

// normalizeKey maps a key to its SameValueZero class: -0 is +0 and every NaN is
// one key.
func normalizeKey(k any) any {
	if f, ok := k.(float64); ok {
		if math.IsNaN(f) {
			return nanKey{}
		}
		if f == 0 {
			return 0.0
		}
	}
	return k
}

// find returns the position of k's live entry.
func (m *orderedEntries) find(k any) (int, bool) {
	i, ok := m.index[normalizeKey(k)]
	return i, ok
}

func (m *orderedEntries) get(k any) (any, bool) {
	if i, ok := m.find(k); ok {
		return m.values[i], true
	}
	return js.Undefined, false
}

func (m *orderedEntries) set(k, v any) {
	if i, ok := m.find(k); ok {
		m.values[i] = v
		return
	}
	if f, ok := k.(float64); ok && f == 0 {
		// The entry keeps +0 for -0.
		k = 0.0
	}
	m.index[normalizeKey(k)] = len(m.keys)
	m.keys = append(m.keys, k)
	m.values = append(m.values, v)
	m.deleted = append(m.deleted, false)
	m.size++
}

func (m *orderedEntries) remove(k any) bool {
	i, ok := m.find(k)
	if !ok {
		return false
	}
	delete(m.index, normalizeKey(k))
	m.deleted[i] = true
	m.keys[i], m.values[i] = js.Undefined, js.Undefined
	m.size--
	return true
}

func (m *orderedEntries) clear() {
	for i := range m.keys {
		m.deleted[i] = true
		m.keys[i], m.values[i] = js.Undefined, js.Undefined
	}
	m.index = map[any]int{}
	m.size = 0
}

// next returns the position of the first live entry at or after i.
func (m *orderedEntries) next(i int) (int, bool) {
	for ; i < len(m.keys); i++ {
		if !m.deleted[i] {
			return i, true
		}
	}
	return i, false
}

// liveKeys returns the keys of the live entries in order.
func (m *orderedEntries) liveKeys() []any {
	out := make([]any, 0, m.size)
	for i, k := range m.keys {
		if !m.deleted[i] {
			out = append(out, k)
		}
	}
	return out
}

// mapObject is a Map; weak marks a WeakMap.
type mapObject struct {
	entries *orderedEntries
	weak    bool
}

// setObject is a Set; weak marks a WeakSet.
type setObject struct {
	entries *orderedEntries
	weak    bool
}

// iterKind selects what a collection iterator yields.
type iterKind uint8

const (
	iterKeys iterKind = iota
	iterValues
	iterEntries
)

// collectionIterator is a Map Iterator or Set Iterator. It walks the live entry
// list, so entries added during iteration are visited and deleted ones are not.
type collectionIterator struct {
	m    *mapObject
	set  *setObject
	kind iterKind
	pos  int
	done bool
}

func (it *collectionIterator) entries() *orderedEntries {
	if it.set != nil {
		return it.set.entries
	}
	return it.m.entries
}

// step returns the next value and whether there was one.
func (it *collectionIterator) step() (any, bool) {
	if it.done {
		return js.Undefined, false
	}
	e := it.entries()
	i, ok := e.next(it.pos)
	if !ok {
		it.done = true
		return js.Undefined, false
	}
	it.pos = i + 1
	k, v := e.keys[i], e.values[i]
	if it.set != nil {
		v = k
	}
	switch it.kind {
	case iterKeys:
		return k, true
	case iterValues:
		return v, true
	}
	return js.NewArray(k, v), true
}

// stepper yields the values of an iteration one at a time; ok is false once it
// is exhausted.
type stepper func() (v any, ok bool)

// iteration is an iteration in progress. step returns the next value, ok
// false once the iteration is done; a generator's step runs its body and may
// fail. A consumer that stops before the end closes the iteration (stop, or
// abort when it stops because of an error).
type iteration struct {
	step func() (v any, ok bool, err error)
	// gen is the generator being iterated, nil for the builtin iterables,
	// whose iterators have no return method.
	gen *generatorRecord
	r   *Realm
}

// stop is IteratorClose after a normal completion: a generator's return()
// runs, and what it throws is the error.
func (it *iteration) stop() error {
	if it == nil || it.gen == nil || it.gen.state == generatorCompleted {
		return nil
	}
	return it.r.closeGenerator(it.gen)
}

// abort is IteratorClose after an abrupt completion err: a generator's return()
// runs, and err wins over what that throws.
func (it *iteration) abort(err error) error {
	if it == nil || it.gen == nil || it.gen.state == generatorCompleted || it.gen.state == generatorExecuting {
		return err
	}
	if cerr := it.r.closeGenerator(it.gen); cerr != nil && isGeneratorReturn(err) {
		return cerr
	}
	return err
}

// openIterator starts the iteration protocol on v: arrays, strings (by code
// point), arguments objects, Maps, Sets, the builtin iterators and generator
// objects are iterable. Arrays and collections are read live. ok is false for
// a value that is not iterable.
func (r *Realm) openIterator(v any) (*iteration, bool) {
	if g := generatorOf(v); g != nil {
		return &iteration{r: r, gen: g, step: func() (any, bool, error) {
			res, err := r.resumeGenerator(g, resumeNext, js.Undefined)
			if err != nil {
				return nil, false, err
			}
			out := res.(*js.Object)
			if out.Get("done") == true {
				return js.Undefined, false, nil
			}
			return out.Get("value"), true, nil
		}}, true
	}
	next, ok := r.openBuiltinIterator(v)
	if !ok {
		return nil, false
	}
	return &iteration{r: r, step: func() (any, bool, error) {
		v, more := next()
		return v, more, nil
	}}, true
}

// openBuiltinIterator is openIterator for the builtin iterables.
func (r *Realm) openBuiltinIterator(v any) (stepper, bool) {
	switch x := v.(type) {
	case *js.Array:
		i := 0
		return func() (any, bool) {
			if i >= len(x.Items) {
				return js.Undefined, false
			}
			i++
			return x.Items[i-1], true
		}, true
	case string:
		return sliceStepper(stringCodePoints(x)), true
	case *primitiveObject:
		if s, ok := x.value.(string); ok {
			return sliceStepper(stringCodePoints(s)), true
		}
	case *arguments:
		i := 0
		return func() (any, bool) {
			if i >= len(x.items) {
				return js.Undefined, false
			}
			i++
			return x.item(i - 1), true
		}, true
	case *iterator:
		return func() (any, bool) {
			if x.pos >= len(x.items) {
				return js.Undefined, false
			}
			x.pos++
			return x.items[x.pos-1], true
		}, true
	case *mapObject:
		if !x.weak {
			return (&collectionIterator{m: x, kind: iterEntries}).step, true
		}
	case *setObject:
		if !x.weak {
			return (&collectionIterator{set: x, kind: iterValues}).step, true
		}
	case *collectionIterator:
		return x.step, true
	}
	return nil, false
}

func sliceStepper(items []any) stepper {
	i := 0
	return func() (any, bool) {
		if i >= len(items) {
			return js.Undefined, false
		}
		i++
		return items[i-1], true
	}
}

// forEachIterated runs fn on each value the iteration of v yields, stopping at
// the first error. A value that is not iterable fails with the error
// notIterable builds.
func (r *Realm) forEachIterated(v any, notIterable func() error, fn func(any) error) error {
	it, ok := r.openIterator(v)
	if !ok {
		return notIterable()
	}
	for {
		item, more, err := it.step()
		if err != nil || !more {
			return err
		}
		if err := fn(item); err != nil {
			return it.abort(err)
		}
	}
}

// iterateN collects at most n values (all of them when n < 0) of the
// iteration of v, closing it when values are left, as array destructuring
// does. Other values are not iterable and fail with the error notIterable
// builds.
func (r *Realm) iterateN(v any, n int, notIterable func() error) ([]any, error) {
	it, ok := r.openIterator(v)
	if !ok {
		return nil, notIterable()
	}
	var out []any
	for n < 0 || len(out) < n {
		item, more, err := it.step()
		if err != nil || !more {
			return out, err
		}
		out = append(out, item)
	}
	return out, it.stop()
}

// iterate collects the values the iteration protocol yields for v. Other values
// are not iterable and fail with the error notIterable builds.
func (r *Realm) iterate(v any, notIterable func() error) ([]any, error) {
	var out []any
	err := r.forEachIterated(v, notIterable, func(item any) error {
		out = append(out, item)
		return nil
	})
	return out, err
}

// notIterableValue is V8's TypeError for a builtin's argument that is not
// iterable, described without source text.
func notIterableValue(v any) error {
	return js.NewTypeError(describeValue(v) + " is not iterable (cannot read property Symbol(Symbol.iterator))")
}

func (r *Realm) setupCollections() {
	r.setupMap("Map", r.mapProto, false)
	r.setupMap("WeakMap", r.weakMapProto, true)
	r.setupSet("Set", r.setProto, false)
	r.setupSet("WeakSet", r.weakSetProto, true)
	r.setupCollectionIterator("Map Iterator", r.mapIteratorProto)
	r.setupCollectionIterator("Set Iterator", r.setIteratorProto)
}

// incompatibleReceiver is V8's TypeError for a method called on the wrong kind
// of object.
func (r *Realm) incompatibleReceiver(method string, this any) error {
	return js.NewTypeError("Method " + method + " called on incompatible receiver " + r.safeString(this))
}

// canBeHeldWeakly reports whether v can be a WeakMap key or WeakSet value.
func canBeHeldWeakly(v any) bool {
	return js.IsObject(v)
}

// collectionConstructor returns the [[Call]] of Map and Set, which require new.
func collectionConstructor(name string) NativeFunc {
	return func(any, []any) (any, error) {
		return nil, js.NewTypeError("Constructor " + name + " requires 'new'")
	}
}

func (r *Realm) setupMap(name string, proto *js.Object, weak bool) {
	newMap := func(args []any) (any, error) {
		m := &mapObject{entries: newOrderedEntries(), weak: weak}
		src := js.Arg(args, 0)
		if js.IsNullish(src) {
			return m, nil
		}
		err := r.forEachIterated(src, func() error { return notIterableValue(src) }, func(en any) error {
			if !js.IsObject(en) {
				return js.NewTypeError("Iterator value " + r.safeString(en) + " is not an entry object")
			}
			k, err := r.get(en, "0")
			if err != nil {
				return err
			}
			v, err := r.get(en, "1")
			if err != nil {
				return err
			}
			if weak && !canBeHeldWeakly(k) {
				return js.NewTypeError("Invalid value used as weak map key")
			}
			m.entries.set(k, v)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return m, nil
	}
	ctor := r.constructor(name, 0, proto, collectionConstructor(name), newMap)
	if !weak {
		r.method(ctor, "groupBy", 2, func(_ any, args []any) (any, error) {
			m := &mapObject{entries: newOrderedEntries()}
			var keys []any
			err := r.groupBy("Map.groupBy", args, func(k any) (string, error) {
				keys = append(keys, k)
				return "", nil
			}, func(_ string, v any) {
				k := keys[len(keys)-1]
				if g, ok := m.entries.get(k); ok {
					g.(*js.Array).Items = append(g.(*js.Array).Items, v)
					return
				}
				m.entries.set(k, js.NewArray(v))
			})
			return m, err
		})
	}
	thisMap := func(this any, method string) (*mapObject, error) {
		if m, ok := this.(*mapObject); ok && m.weak == weak {
			return m, nil
		}
		return nil, r.incompatibleReceiver(name+".prototype."+method, this)
	}
	r.method(proto, "get", 1, func(this any, args []any) (any, error) {
		m, err := thisMap(this, "get")
		if err != nil {
			return nil, err
		}
		v, _ := m.entries.get(js.Arg(args, 0))
		return v, nil
	})
	r.method(proto, "set", 2, func(this any, args []any) (any, error) {
		m, err := thisMap(this, "set")
		if err != nil {
			return nil, err
		}
		k := js.Arg(args, 0)
		if weak && !canBeHeldWeakly(k) {
			return nil, js.NewTypeError("Invalid value used as weak map key")
		}
		m.entries.set(k, js.Arg(args, 1))
		return m, nil
	})
	r.method(proto, "has", 1, func(this any, args []any) (any, error) {
		m, err := thisMap(this, "has")
		if err != nil {
			return nil, err
		}
		_, ok := m.entries.find(js.Arg(args, 0))
		return ok, nil
	})
	r.method(proto, "delete", 1, func(this any, args []any) (any, error) {
		m, err := thisMap(this, "delete")
		if err != nil {
			return nil, err
		}
		return m.entries.remove(js.Arg(args, 0)), nil
	})
	if weak {
		return
	}
	r.method(proto, "clear", 0, func(this any, _ []any) (any, error) {
		m, err := thisMap(this, "clear")
		if err != nil {
			return nil, err
		}
		m.entries.clear()
		return js.Undefined, nil
	})
	iter := func(method string, kind iterKind) {
		r.method(proto, method, 0, func(this any, _ []any) (any, error) {
			m, err := thisMap(this, method)
			if err != nil {
				return nil, err
			}
			return &collectionIterator{m: m, kind: kind}, nil
		})
	}
	iter("entries", iterEntries)
	r.method(proto, "forEach", 1, func(this any, args []any) (any, error) {
		m, err := thisMap(this, "forEach")
		if err != nil {
			return nil, err
		}
		cb, ok := js.Arg(args, 0).(*js.Function)
		if !ok {
			return nil, js.NotAFunction(describeValue(js.Arg(args, 0)))
		}
		for i, more := m.entries.next(0); more; i, more = m.entries.next(i + 1) {
			if _, err := r.call(cb, js.Arg(args, 1), []any{m.entries.values[i], m.entries.keys[i], m}); err != nil {
				return nil, err
			}
		}
		return js.Undefined, nil
	})
	iter("keys", iterKeys)
	r.getter(proto, "size", func(this any) (any, error) {
		m, err := thisMap(this, "size")
		if err != nil {
			return nil, r.incompatibleReceiver("get "+name+".prototype.size", this)
		}
		return float64(m.entries.size), nil
	})
	iter("values", iterValues)
}

func (r *Realm) setupSet(name string, proto *js.Object, weak bool) {
	newSet := func(args []any) (any, error) {
		s := &setObject{entries: newOrderedEntries(), weak: weak}
		src := js.Arg(args, 0)
		if js.IsNullish(src) {
			return s, nil
		}
		err := r.forEachIterated(src, func() error { return notIterableValue(src) }, func(v any) error {
			if weak && !canBeHeldWeakly(v) {
				return js.NewTypeError("Invalid value used in weak set")
			}
			s.entries.set(v, js.Undefined)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	r.constructor(name, 0, proto, collectionConstructor(name), newSet)
	thisSet := func(this any, method string) (*setObject, error) {
		if s, ok := this.(*setObject); ok && s.weak == weak {
			return s, nil
		}
		return nil, r.incompatibleReceiver(name+".prototype."+method, this)
	}
	r.method(proto, "has", 1, func(this any, args []any) (any, error) {
		s, err := thisSet(this, "has")
		if err != nil {
			return nil, err
		}
		_, ok := s.entries.find(js.Arg(args, 0))
		return ok, nil
	})
	r.method(proto, "add", 1, func(this any, args []any) (any, error) {
		s, err := thisSet(this, "add")
		if err != nil {
			return nil, err
		}
		v := js.Arg(args, 0)
		if weak && !canBeHeldWeakly(v) {
			return nil, js.NewTypeError("Invalid value used in weak set")
		}
		s.entries.set(v, js.Undefined)
		return s, nil
	})
	r.method(proto, "delete", 1, func(this any, args []any) (any, error) {
		s, err := thisSet(this, "delete")
		if err != nil {
			return nil, err
		}
		return s.entries.remove(js.Arg(args, 0)), nil
	})
	if weak {
		return
	}
	// setOp registers one of the set-algebra methods of ES2025; other is read
	// through its has and keys, which for Map and Set arguments are their own.
	setOp := func(method string, op func(s, other *setObject) any) {
		r.method(proto, method, 1, func(this any, args []any) (any, error) {
			s, err := thisSet(this, method)
			if err != nil {
				return nil, err
			}
			other, err := r.setLike(js.Arg(args, 0))
			if err != nil {
				return nil, err
			}
			return op(s, other), nil
		})
	}
	setOp("difference", func(s, other *setObject) any {
		return filterSet(s, func(v any) bool { _, ok := other.entries.find(v); return !ok })
	})
	r.method(proto, "clear", 0, func(this any, _ []any) (any, error) {
		s, err := thisSet(this, "clear")
		if err != nil {
			return nil, err
		}
		s.entries.clear()
		return js.Undefined, nil
	})
	r.method(proto, "entries", 0, func(this any, _ []any) (any, error) {
		s, err := thisSet(this, "entries")
		if err != nil {
			return nil, err
		}
		return &collectionIterator{set: s, kind: iterEntries}, nil
	})
	r.method(proto, "forEach", 1, func(this any, args []any) (any, error) {
		s, err := thisSet(this, "forEach")
		if err != nil {
			return nil, err
		}
		cb, ok := js.Arg(args, 0).(*js.Function)
		if !ok {
			return nil, js.NotAFunction(describeValue(js.Arg(args, 0)))
		}
		for i, more := s.entries.next(0); more; i, more = s.entries.next(i + 1) {
			v := s.entries.keys[i]
			if _, err := r.call(cb, js.Arg(args, 1), []any{v, v, s}); err != nil {
				return nil, err
			}
		}
		return js.Undefined, nil
	})
	setOp("intersection", func(s, other *setObject) any {
		// The smaller set is iterated, so its order is the result's.
		if s.entries.size > other.entries.size {
			return filterSet(other, func(v any) bool { _, ok := s.entries.find(v); return ok })
		}
		return filterSet(s, func(v any) bool { _, ok := other.entries.find(v); return ok })
	})
	setOp("isSubsetOf", func(s, other *setObject) any {
		return filterSet(s, func(v any) bool { _, ok := other.entries.find(v); return !ok }).entries.size == 0
	})
	setOp("isSupersetOf", func(s, other *setObject) any {
		return filterSet(other, func(v any) bool { _, ok := s.entries.find(v); return !ok }).entries.size == 0
	})
	setOp("isDisjointFrom", func(s, other *setObject) any {
		return filterSet(s, func(v any) bool { _, ok := other.entries.find(v); return ok }).entries.size == 0
	})
	r.getter(proto, "size", func(this any) (any, error) {
		s, err := thisSet(this, "size")
		if err != nil {
			return nil, r.incompatibleReceiver("get "+name+".prototype.size", this)
		}
		return float64(s.entries.size), nil
	})
	setOp("symmetricDifference", func(s, other *setObject) any {
		out := filterSet(s, func(v any) bool { _, ok := other.entries.find(v); return !ok })
		for _, v := range other.entries.liveKeys() {
			if _, ok := s.entries.find(v); !ok {
				out.entries.set(v, js.Undefined)
			}
		}
		return out
	})
	setOp("union", func(s, other *setObject) any {
		out := filterSet(s, func(any) bool { return true })
		for _, v := range other.entries.liveKeys() {
			out.entries.set(v, js.Undefined)
		}
		return out
	})
	values := r.method(proto, "values", 0, func(this any, _ []any) (any, error) {
		s, err := thisSet(this, "values")
		if err != nil {
			return nil, err
		}
		return &collectionIterator{set: s, kind: iterValues}, nil
	})
	// Set.prototype.keys is the same function as values.
	r.define(proto, "keys", values)
}

// filterSet returns a new Set of the values of s that keep accepts.
func filterSet(s *setObject, keep func(any) bool) *setObject {
	out := &setObject{entries: newOrderedEntries()}
	for _, v := range s.entries.liveKeys() {
		if keep(v) {
			out.entries.set(v, js.Undefined)
		}
	}
	return out
}

// setLike is GetSetRecord for the set-algebra methods: a Set, or a Map seen
// through its keys.
func (r *Realm) setLike(v any) (*setObject, error) {
	switch x := v.(type) {
	case *setObject:
		if !x.weak {
			return x, nil
		}
	case *mapObject:
		if !x.weak {
			out := &setObject{entries: newOrderedEntries()}
			for _, k := range x.entries.liveKeys() {
				out.entries.set(k, js.Undefined)
			}
			return out, nil
		}
	}
	return nil, js.NewTypeError("Set operations take a Set or a Map, not " + r.safeString(v))
}

// setupCollectionIterator installs next on %MapIteratorPrototype% or
// %SetIteratorPrototype%.
func (r *Realm) setupCollectionIterator(tag string, proto *js.Object) {
	r.protoLinks[proto] = r.iteratorProto
	r.method(proto, "next", 0, func(this any, _ []any) (any, error) {
		it, ok := this.(*collectionIterator)
		if !ok || (tag == "Set Iterator") != (it.set != nil) {
			return nil, r.incompatibleReceiver(tag+".prototype.next", this)
		}
		v, more := it.step()
		if !more {
			return js.ObjectOf("value", js.Undefined, "done", true), nil
		}
		return js.ObjectOf("value", v, "done", false), nil
	})
}

// getter installs a builtin accessor property with only a getter, such as
// Map.prototype.size.
func (r *Realm) getter(o *js.Object, name string, fn func(this any) (any, error)) {
	f := NewNative("get "+name, 0, func(this any, _ []any) (any, error) { return fn(this) })
	o.Define(name, &js.Accessor{Get: f}, js.Hidden)
}
