package jsrt

import (
	"slices"
	"strconv"

	"github.com/edge-js/edge-go/js"
)

// The object model. Each Go type of the value model keeps some properties in its
// own fields (an array's items and length, a function's name and length, an
// error's message); every other own property lives in a property table, a
// *js.Object that records attributes, accessors and extensibility. A plain object
// is its own table, a Derived object's table is Own, an error's is Props, and
// the realm keeps the tables of the other objects (extra). The operations below
// are the ECMAScript internal methods over that model: [[GetOwnProperty]],
// [[Get]], [[Set]], [[Delete]], [[DefineOwnProperty]], [[OwnPropertyKeys]] and
// the integrity levels of Object.freeze and Object.seal.

// Derived is an object with an explicit prototype, as Object.create(proto) builds
// it. Own holds its own properties; Proto is the next object on the prototype
// chain, or nil for Object.create(null).
type Derived struct {
	Own   *js.Object
	Proto any
	// promise holds the internal slots of a promise, nil for other objects.
	promise *promiseRecord
	// generator holds the internal slots of a generator object, asyncGen
	// those of an async generator object.
	generator *generatorRecord
	asyncGen  *asyncGenerator
}

// WithProto returns an object whose own properties are own (a new empty object
// when nil) and whose prototype is proto.
func WithProto(own *js.Object, proto any) *Derived {
	if own == nil {
		own = js.NewObject()
	}
	return &Derived{Own: own, Proto: proto}
}

// arguments is a function's arguments object: array-like but not an array. In a
// sloppy-mode function with simple parameters the object is mapped: its first
// indexes alias the parameter bindings, which mapped holds (nil once an index is
// unmapped). A strict-mode arguments object has no callee.
type arguments struct {
	items  []any
	mapped []*binding
	callee *js.Function
	strict bool
}

// item returns the value at index i, reading through the parameter binding while
// the index is mapped.
func (a *arguments) item(i int) any {
	if i < len(a.mapped) && a.mapped[i] != nil {
		return a.mapped[i].value
	}
	return a.items[i]
}

// setItem stores v at index i, writing through to the parameter binding while
// the index is mapped.
func (a *arguments) setItem(i int, v any) {
	a.items[i] = v
	if i < len(a.mapped) && a.mapped[i] != nil {
		a.mapped[i].value = v
	}
}

// primitiveObject is a Boolean, Number, String or BigInt object: the wrapper
// ToObject makes for a primitive, as new Number(1), Object('s'), Object(1n)
// and a sloppy-mode function's this do. value is the wrapped primitive.
type primitiveObject struct {
	value any
}

// primitiveValue returns the primitive a wrapper object holds ([[NumberData]],
// [[StringData]], [[BooleanData]], [[BigIntData]]), or v itself.
func primitiveValue(v any) any {
	if p, ok := v.(*primitiveObject); ok {
		return p.value
	}
	return v
}

// thisPrimitive is thisNumberValue, thisStringValue, thisBooleanValue and
// thisBigIntValue: the primitive of type T that this is or wraps, else V8's
// TypeError for the method of Type.prototype.
func thisPrimitive[T any](this any, typeName, method string) (T, error) {
	v, ok := primitiveValue(this).(T)
	if !ok {
		return v, js.NewTypeError(typeName + ".prototype." + method + " requires that 'this' be a " + typeName)
	}
	return v, nil
}

// iterator is a built-in iterator (Array Iterator, String Iterator, RegExp String
// Iterator) over values computed when it was created.
type iterator struct {
	items []any
	pos   int
	// tag is the class Object.prototype.toString reports, e.g. "Array Iterator".
	tag string
}

// integrity is how far Object.preventExtensions, Object.seal or Object.freeze
// went on an object. It qualifies the properties a Go type keeps in its own
// fields; the property table records its own entries' attributes.
type integrity uint8

const (
	extensible integrity = iota
	nonExtensible
	sealed
	frozen
)

// level returns v's integrity level.
func (r *Realm) level(v any) integrity {
	return r.levels[v]
}

// protoOf returns the [[Prototype]] of v: a prototype object, or nil at the end of
// the chain. Primitives report the prototype their wrapper object would have.
func (r *Realm) protoOf(v any) any {
	if p, ok := r.protoLinks[v]; ok {
		return p
	}
	switch x := v.(type) {
	case *js.Object:
		if x == r.objectProto {
			return nil
		}
		return r.objectProto
	case *Derived:
		return x.Proto
	case *js.Array:
		return r.arrayProto
	case string:
		return r.stringProto
	case float64:
		return r.numberProto
	case bool:
		return r.booleanProto
	case js.BigInt:
		return r.bigintProto
	case *primitiveObject:
		return r.protoOf(x.value)
	case *js.Function:
		switch async, generator := r.isAsyncFunction(x), isGeneratorFunction(x); {
		case async && generator:
			return r.asyncGenerators.functionProto
		case async:
			return r.async.asyncFunctionProto
		case generator:
			return r.generators.functionProto
		}
		return r.functionProto
	case *js.Date:
		return r.dateProto
	case *js.RegExp:
		return r.regexpProto
	case *js.Error:
		if p, ok := r.errorProtos[x.Name]; ok {
			return p
		}
		return r.errorProtos["Error"]
	case *arguments:
		return r.objectProto
	case *iterator:
		return r.iteratorProto
	case *mapObject:
		if x.weak {
			return r.weakMapProto
		}
		return r.mapProto
	case *setObject:
		if x.weak {
			return r.weakSetProto
		}
		return r.setProto
	case *collectionIterator:
		if x.set != nil {
			return r.setIteratorProto
		}
		return r.mapIteratorProto
	}
	return nil
}

// setPrototype is [[SetPrototypeOf]] for Object.setPrototypeOf and __proto__: it
// fails for a non-extensible object whose prototype would change and for a
// cycle.
func (r *Realm) setPrototype(v, proto any) error {
	cur := r.protoOf(v)
	if cur == proto {
		return nil
	}
	if !r.isExtensible(v) {
		return js.NewTypeError("#<" + r.constructorName(v) + "> is not extensible")
	}
	for p := proto; p != nil; p = r.protoOf(p) {
		if p == v {
			return js.NewTypeError("Cyclic __proto__ value")
		}
	}
	if d, ok := v.(*Derived); ok {
		d.Proto = proto
		return nil
	}
	r.protoLinks[v] = proto
	return nil
}

// table returns the property table of v, creating it when create is set: the
// object itself for a plain object, Own for a Derived object, Props for an error
// and the realm's side table for the other objects. Primitives have none.
func (r *Realm) table(v any, create bool) *js.Object {
	switch x := v.(type) {
	case *js.Object:
		return x
	case *Derived:
		return x.Own
	case *js.Error:
		if x.Props == nil && create {
			x.Props = js.NewObject()
		}
		return x.Props
	case nil, js.UndefinedType, string, float64, bool, js.BigInt:
		return nil
	}
	return r.extraProps(v, create)
}

// extraProps returns the side table holding v's own properties beyond the ones its
// Go representation stores, creating it if asked.
func (r *Realm) extraProps(v any, create bool) *js.Object {
	p := r.extra[v]
	if p == nil && create {
		p = js.NewObject()
		r.extra[v] = p
	}
	return p
}

// Attributes of the properties the value model keeps in Go fields.
const (
	// attrsLength is an array's length: writable only.
	attrsLength = js.Hidden | js.Fixed
	// attrsBuiltin is a builtin data property: writable and configurable.
	attrsBuiltin = js.Hidden
	// attrsConstant is a constant such as Math.PI or NaN.
	attrsConstant = js.ReadOnly | js.Hidden | js.Fixed
	// attrsNameLength is a function's name and length.
	attrsNameLength = js.ReadOnly | js.Hidden
	// attrsStringIndex is a string's character.
	attrsStringIndex = js.ReadOnly | js.Fixed
)

// levelAttrs adds the attributes an integrity level imposes on a data property.
func levelAttrs(a js.Attr, level integrity) js.Attr {
	switch level {
	case sealed:
		a |= js.Fixed
	case frozen:
		a |= js.Fixed | js.ReadOnly
	}
	return a
}

// ownProperty is [[GetOwnProperty]]: the value slot of v's own property key (an
// *js.Accessor for an accessor property), its attributes, and whether v has it.
func (r *Realm) ownProperty(v any, key string) (any, js.Attr, bool) {
	level := r.level(v)
	switch x := v.(type) {
	case string:
		val, ok := js.GetOwn(x, key)
		if key == "length" {
			return val, attrsConstant, ok
		}
		return val, attrsStringIndex, ok
	case float64, bool, js.BigInt, nil, js.UndefinedType:
		return nil, 0, false
	case *js.Array:
		if key == "length" {
			return float64(len(x.Items)), levelAttrs(attrsLength, level), true
		}
		if i, ok := js.ArrayIndex(key); ok {
			if i < len(x.Items) {
				return x.Items[i], levelAttrs(0, level), true
			}
			return nil, 0, false
		}
	case *primitiveObject:
		if s, ok := x.value.(string); ok {
			if val, attrs, ok := r.ownProperty(s, key); ok {
				return val, attrs, true
			}
		}
	case *js.Function:
		switch key {
		case "name":
			return x.Name, levelAttrs(attrsNameLength, level), true
		case "length":
			return float64(x.Length), levelAttrs(attrsNameLength, level), true
		case "arguments", "caller":
			if r.hasLegacyCallerAndArguments(x) {
				return nil, attrsConstant, true
			}
		case "prototype":
			if t := r.table(x, false); t == nil || !t.Has(key) {
				if proto := r.functionPrototype(x); proto != nil {
					return proto, r.table(x, false).Attrs(key), true
				}
			}
		}
	case *js.Error:
		switch key {
		case "message", "stack":
			val, _ := js.GetOwn(x, key)
			attrs := js.Attr(attrsBuiltin)
			if x.Props != nil && x.Props.Has(key) {
				attrs = x.Props.Attrs(key)
			}
			return val, levelAttrs(attrs, level), true
		case "name":
			if !js.IsNativeErrorName(x.Name) {
				return x.Name, levelAttrs(attrsBuiltin, level), true
			}
		}
	case *js.RegExp:
		if key == "lastIndex" {
			return r.regexpState(x).lastIndex, levelAttrs(js.Hidden|js.Fixed, level), true
		}
		// The flag and source accessors live on RegExp.prototype in JavaScript;
		// they are read as own values here.
		if val, ok := r.regexpProp(x, key); ok {
			return val, attrsConstant, true
		}
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.items) {
			if t := r.table(x, false); t == nil || !t.Has(key) {
				return x.item(i), levelAttrs(0, level), true
			}
		}
		if t := r.table(x, false); t == nil || !t.Has(key) {
			switch key {
			case "length":
				return float64(len(x.items)), levelAttrs(attrsBuiltin, level), true
			case "callee":
				if x.strict {
					return r.throwerAccessor(), js.Hidden | js.Fixed, true
				}
				return x.callee, levelAttrs(attrsBuiltin, level), true
			}
		}
	}
	if t := r.table(v, false); t != nil && t.Has(key) {
		return t.Get(key), t.Attrs(key), true
	}
	return nil, 0, false
}

// getOwn returns the value slot of v's own property key, and whether v has it.
func (r *Realm) getOwn(v any, key string) (any, bool) {
	val, _, ok := r.ownProperty(v, key)
	return val, ok
}

// findProperty walks v's prototype chain for key and returns the value slot of
// the first object that has it. v must not be null or undefined.
func (r *Realm) findProperty(v any, key string) (any, js.Attr, bool) {
	for v != nil {
		if val, attrs, ok := r.ownProperty(v, key); ok {
			return val, attrs, true
		}
		v = r.protoOf(v)
	}
	return js.Undefined, 0, false
}

// get is [[Get]]: the value of key on v or its prototype chain, calling an
// accessor's getter with v as this. v must not be null or undefined.
func (r *Realm) get(v any, key string) (any, error) {
	val, _, _ := r.findProperty(v, key)
	if acc, ok := val.(*js.Accessor); ok {
		if acc.Get == nil {
			return js.Undefined, nil
		}
		return r.call(acc.Get, v, nil)
	}
	return val, nil
}

// lookup is dataProperty that also reports whether v or its prototype chain has
// the property.
func (r *Realm) lookup(v any, key string) (any, bool) {
	val, _, ok := r.findProperty(v, key)
	if _, isAcc := val.(*js.Accessor); isAcc {
		return js.Undefined, ok
	}
	return val, ok
}

// dataProperty is V8's GetDataProperty: the value of key on v or its prototype
// chain without running a getter (an accessor reads as undefined).
func (r *Realm) dataProperty(v any, key string) any {
	if js.IsNullish(v) {
		return js.Undefined
	}
	val, _ := r.lookup(v, key)
	return val
}

// hasProperty is the HasProperty operation behind "in" and with-scope lookup.
func (r *Realm) hasProperty(v any, key string) bool {
	_, _, ok := r.findProperty(v, key)
	return ok
}

// hasOwn is Object.prototype.hasOwnProperty.
func (r *Realm) hasOwn(v any, key string) bool {
	_, ok := r.getOwn(v, key)
	return ok
}

// getV reads key from any value, as GetV does: primitives read through their
// prototype; null and undefined throw V8's TypeError.
func (r *Realm) getV(v any, key string) (any, error) {
	if js.IsNullish(v) {
		return nil, js.CannotReadProperties(v, key)
	}
	return r.get(v, key)
}

// put is [[Set]] (OrdinarySet): it calls an inherited or own setter, refuses to
// write a read-only property, changes an own writable property or creates a new
// one on an extensible v. When a write is refused, strict reports V8's TypeError
// (as strict-mode code and the builtins do) and sloppy mode ignores it.
func (r *Realm) put(v any, key string, val any, strict bool) error {
	if js.IsNullish(v) {
		return js.NewTypeError("Cannot set properties of " + js.ToString(v) + " (setting '" + key + "')")
	}
	for o := v; o != nil; o = r.protoOf(o) {
		slot, attrs, ok := r.ownProperty(o, key)
		if !ok {
			continue
		}
		if acc, isAcc := slot.(*js.Accessor); isAcc {
			if acc == r.thrower && strict {
				// V8 reports a write to a strict-mode arguments object's callee
				// as a read-only property.
				return r.readOnlyError(v, key)
			}
			if acc.Set == nil {
				if strict {
					return js.NewTypeError("Cannot set property " + key + " of " + r.safeString(v) + " which has only a getter")
				}
				return nil
			}
			_, err := r.call(acc.Set, v, []any{val})
			return err
		}
		if attrs&js.ReadOnly != 0 {
			if strict {
				return r.readOnlyError(v, key)
			}
			return nil
		}
		if o == v {
			return r.writeOwn(v, key, val, strict)
		}
		break
	}
	return r.addProperty(v, key, val, strict)
}

// readOnlyError is V8's TypeError for writing a read-only property of v.
func (r *Realm) readOnlyError(v any, key string) error {
	switch x := v.(type) {
	case string, float64, bool, js.BigInt:
		return js.NewTypeError("Cannot assign to read only property '" + key + "' of " + js.TypeOf(x) + " '" + js.ToString(x) + "'")
	case *js.Function:
		return js.NewTypeError("Cannot assign to read only property '" + key + "' of function '" + js.FunctionText(x) + "'")
	}
	return js.NewTypeError("Cannot assign to read only property '" + key + "' of object '" + r.safeString(v) + "'")
}

// notExtensibleError is V8's TypeError for adding key to a non-extensible object.
func notExtensibleError(key string) error {
	return js.NewTypeError("Cannot add property " + key + ", object is not extensible")
}

// addProperty creates key as a new writable, enumerable, configurable data
// property of v.
func (r *Realm) addProperty(v any, key string, val any, strict bool) error {
	switch x := v.(type) {
	case string, float64, bool, js.BigInt:
		if strict {
			return js.NewTypeError("Cannot create property '" + key + "' on " + js.TypeOf(x) + " '" + js.ToString(x) + "'")
		}
		return nil
	}
	if !r.isExtensible(v) {
		if strict {
			return notExtensibleError(key)
		}
		return nil
	}
	switch x := v.(type) {
	case *js.Array:
		if i, ok := js.ArrayIndex(key); ok {
			x.SetIndex(i, val)
			return nil
		}
	case *js.Error:
		if key == "name" {
			// The model keeps an error's name in a field.
			s, err := r.toString(val)
			if err != nil {
				return err
			}
			x.Name = s
			return nil
		}
	}
	r.table(v, true).Set(key, val)
	return nil
}

// writeOwn stores val in v's own writable data property key.
func (r *Realm) writeOwn(v any, key string, val any, strict bool) error {
	switch x := v.(type) {
	case *js.Array:
		if key == "length" {
			return r.setArrayLength(x, val, strict)
		}
		if i, ok := js.ArrayIndex(key); ok && i < len(x.Items) {
			x.Items[i] = val
			return nil
		}
	case *js.Error:
		switch key {
		case "message":
			x.SetMessage(val)
			return nil
		case "stack":
			// An assigned stack replaces the one the model derives from name
			// and message, staying non-enumerable.
			if x.Props == nil || !x.Props.Has(key) {
				r.table(x, true).Define(key, val, js.Hidden)
			} else {
				x.Props.Set(key, val)
			}
			return nil
		case "name":
			// The model stores the name as a string; convert with this realm's
			// ToString.
			s, err := r.toString(val)
			if err != nil {
				return err
			}
			x.Name = s
			return nil
		}
	case *js.RegExp:
		if key == "lastIndex" {
			r.regexpState(x).lastIndex = val
			return nil
		}
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.items) {
			if t := r.table(x, false); t == nil || !t.Has(key) {
				x.setItem(i, val)
				return nil
			}
		}
		if key == "length" || key == "callee" {
			if t := r.table(x, false); t == nil || !t.Has(key) {
				r.table(x, true).Set(key, val)
				return nil
			}
		}
	}
	if t := r.table(v, false); t != nil && t.Has(key) {
		t.Set(key, val)
	}
	return nil
}

// setArrayLength implements ArraySetLength: truncation or extension. Truncating
// a sealed array stops at its first non-configurable element.
func (r *Realm) setArrayLength(a *js.Array, val any, strict bool) error {
	n, err := r.toNumber(val)
	if err != nil {
		return err
	}
	l := js.ToUint32(n)
	if float64(l) != n {
		return js.InvalidArrayLength()
	}
	if int(l) < len(a.Items) && r.level(a) >= sealed {
		if strict {
			return r.deleteError(a, strconv.Itoa(len(a.Items)-1))
		}
		return nil
	}
	a.SetLength(int(l))
	return nil
}

// deleteError is V8's TypeError for deleting a non-configurable property.
func (r *Realm) deleteError(v any, key string) error {
	return js.NewTypeError("Cannot delete property '" + key + "' of " + r.safeString(v))
}

// deleteProp is [[Delete]] behind the delete operator: it reports false for a
// non-configurable property, which strict-mode code reports as V8's TypeError.
func (r *Realm) deleteProp(v any, key string, strict bool) (bool, error) {
	if js.IsNullish(v) {
		return false, ErrNullishToObject()
	}
	if v == any(r.global) {
		r.deletedGlobals[key] = true
	}
	_, attrs, ok := r.ownProperty(v, key)
	if !ok {
		return true, nil
	}
	if attrs&js.Fixed != 0 {
		if strict {
			return false, r.deleteError(v, key)
		}
		return false, nil
	}
	switch x := v.(type) {
	case *js.Array:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.Items) {
			js.DeleteProperty(x, key)
			return true, nil
		}
	case *js.Error:
		if key == "message" {
			// The model keeps the message in a field; deleting it leaves the
			// empty message Error.prototype has.
			x.Message = ""
			if x.Props != nil {
				x.Props.Delete(key)
			}
			return true, nil
		}
	case *js.Function:
		if key == "name" || key == "length" {
			// A function's name and length are configurable, but the model keeps
			// them in fields; deleting one is accepted and leaves it in place.
			return true, nil
		}
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.items) {
			if i < len(x.mapped) {
				x.mapped[i] = nil
			}
		}
	}
	if t := r.table(v, false); t != nil {
		t.Delete(key)
	}
	return true, nil
}

// ownKeys returns v's own enumerable string keys in JavaScript order.
func (r *Realm) ownKeys(v any) []string {
	return r.ownPropertyKeys(v, false)
}

// ownPropertyKeys is [[OwnPropertyKeys]] for string keys: v's own keys in
// JavaScript order (integer indexes, then the other keys in creation order),
// only the enumerable ones unless all is set.
func (r *Realm) ownPropertyKeys(v any, all bool) []string {
	var keys []string
	switch x := v.(type) {
	case string:
		keys = js.OwnKeys(x)
		if all {
			keys = append(keys, "length")
		}
		return keys
	case *primitiveObject:
		if s, ok := x.value.(string); ok {
			keys = js.IndexKeys(len(js.OwnKeys(s)))
			if all {
				keys = append(keys, "length")
			}
		}
	case *js.Array:
		keys = js.IndexKeys(len(x.Items))
		if all {
			keys = append(keys, "length")
		}
	case *arguments:
		keys = js.IndexKeys(len(x.items))
		if all {
			keys = append(keys, "length", "callee")
		}
	case *js.Function:
		if all {
			keys = append(keys, "length", "name")
			if r.hasLegacyCallerAndArguments(x) {
				keys = append(keys, "arguments", "caller")
			}
			// A constructor's prototype is created on first use.
			if r.functionPrototype(x) != nil {
				keys = append(keys, "prototype")
			}
		}
	case *js.Error:
		if all {
			keys = append(keys, "stack", "message")
		}
	case *js.RegExp:
		if all {
			keys = append(keys, "lastIndex")
		}
	}
	t := r.table(v, false)
	if t == nil {
		return keys
	}
	var tk []string
	if all {
		tk = t.Keys()
	} else {
		tk = t.EnumerableKeys()
	}
	if len(keys) == 0 {
		return tk
	}
	// OrdinaryOwnPropertyKeys: integer indexes ascending, then the other keys
	// in creation order, the builtin ones first.
	return mergeOwnKeys(keys, tk)
}

// mergeOwnKeys orders the keys of an object whose builtin keys (its items,
// then properties such as length or message) live outside its property
// table: integer indexes of both ascending, then the builtin names, then the
// table's other keys in creation order.
func mergeOwnKeys(builtin, table []string) []string {
	var idx []int
	var names []string
	seen := map[string]bool{}
	add := func(k string) {
		if seen[k] {
			return
		}
		seen[k] = true
		if i, ok := js.ArrayIndex(k); ok {
			idx = append(idx, i)
		} else {
			names = append(names, k)
		}
	}
	for _, k := range builtin {
		add(k)
	}
	for _, k := range table {
		add(k)
	}
	slices.Sort(idx)
	keys := make([]string, 0, len(idx)+len(names))
	for _, i := range idx {
		keys = append(keys, strconv.Itoa(i))
	}
	return append(keys, names...)
}

// isOwnNonEnumerable reports an own key that exists but is not enumerable; it
// shadows the same key further up the chain during for-in.
func (r *Realm) isOwnNonEnumerable(v any, key string) bool {
	_, attrs, ok := r.ownProperty(v, key)
	return ok && attrs&js.Hidden != 0
}

// forInKeys lists the keys for-in visits: own enumerable keys, then those of each
// prototype not already seen, skipping keys shadowed by non-enumerable ones.
func (r *Realm) forInKeys(v any) []string {
	if js.IsNullish(v) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for v != nil {
		for _, k := range r.ownPropertyKeys(v, true) {
			if seen[k] {
				continue
			}
			seen[k] = true
			if !r.isOwnNonEnumerable(v, k) {
				out = append(out, k)
			}
		}
		v = r.protoOf(v)
	}
	return out
}

// hasLegacyCallerAndArguments reports whether f is a sloppy-mode ordinary
// function, which in V8 owns arguments and caller properties (null outside a
// call).
func (r *Realm) hasLegacyCallerAndArguments(f *js.Function) bool {
	c, ok := closureOf(f)
	return ok && !c.fi.strict && !c.fi.arrow && !c.fi.method && !c.fi.async
}

// functionPrototype returns the prototype object of a user-defined constructor,
// creating it on first use: a non-enumerable, non-configurable property whose
// object has a non-enumerable constructor.
func (r *Realm) functionPrototype(f *js.Function) any {
	t := r.table(f, false)
	if t != nil && t.Has("prototype") {
		return t.Get("prototype")
	}
	c, ok := closureOf(f)
	if !ok || c.fi.arrow || c.fi.async && !c.fi.generator {
		return nil
	}
	var proto any
	switch {
	case c.fi.async:
		// An async generator function's prototype is likewise the prototype
		// of its async generator objects.
		proto = WithProto(nil, r.asyncGenerators.proto)
	case c.fi.generator:
		// A generator function's prototype is the prototype of its generator
		// objects: it inherits %GeneratorPrototype% and has no constructor.
		proto = WithProto(nil, r.generators.proto)
	case c.fi.method:
		return nil
	default:
		o := js.NewObject()
		o.Define("constructor", f, attrsBuiltin)
		proto = o
	}
	attrs := js.Hidden | js.Fixed
	if r.level(f) == frozen {
		attrs |= js.ReadOnly
	}
	r.table(f, true).Define("prototype", proto, attrs)
	return proto
}

// isExtensible is IsExtensible: primitives are not, objects are until
// Object.preventExtensions, seal or freeze.
func (r *Realm) isExtensible(v any) bool {
	if !js.IsObject(v) {
		return false
	}
	if r.level(v) != extensible {
		return false
	}
	t := r.table(v, false)
	return t == nil || t.Extensible()
}

// setIntegrity is SetIntegrityLevel (and Object.preventExtensions for
// nonExtensible): no new properties, and for sealed or frozen every own
// property non-configurable, every data property read-only when frozen.
func (r *Realm) setIntegrity(v any, level integrity) {
	if !js.IsObject(v) {
		return
	}
	if f, ok := v.(*js.Function); ok {
		r.functionPrototype(f)
	}
	t := r.table(v, true)
	t.PreventExtensions()
	if level > r.levels[v] {
		r.levels[v] = level
	}
	if level < sealed {
		return
	}
	for _, k := range t.Keys() {
		a := t.Attrs(k) | js.Fixed
		if _, isAcc := t.Get(k).(*js.Accessor); !isAcc && level == frozen {
			a |= js.ReadOnly
		}
		t.SetAttrs(k, a)
	}
}

// testIntegrity is TestIntegrityLevel: whether v is non-extensible and, for
// sealed or frozen, all its own properties are non-configurable (and its data
// properties read-only when frozen).
func (r *Realm) testIntegrity(v any, level integrity) bool {
	if !js.IsObject(v) {
		return true
	}
	if r.isExtensible(v) {
		return false
	}
	if r.level(v) >= level {
		return true
	}
	for _, k := range r.ownPropertyKeys(v, true) {
		slot, attrs, _ := r.ownProperty(v, k)
		if level >= sealed && attrs&js.Fixed == 0 {
			return false
		}
		if _, isAcc := slot.(*js.Accessor); level == frozen && !isAcc && attrs&js.ReadOnly == 0 {
			return false
		}
	}
	return true
}

// safeString is V8's NoSideEffectsToString, how error messages show an object:
// a function as its source, an error as "Name: message", an object whose
// toString is Object.prototype.toString as #<Constructor>, any other object as
// [object Class], and a primitive as String(value).
func (r *Realm) safeString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case *js.Function:
		return js.FunctionText(x)
	case *js.Error:
		return js.ToString(x)
	case *iterator, *collectionIterator:
		// %IteratorPrototype%.constructor is an accessor, so V8 finds no
		// constructor name and shows the class.
		return "[object " + r.classOf(v) + "]"
	}
	if !js.IsObject(v) {
		return js.ToString(v)
	}
	if r.dataProperty(v, "toString") == r.objectToString {
		if ctor, ok := r.dataProperty(v, "constructor").(*js.Function); ok && ctor.Name != "" {
			return "#<" + ctor.Name + ">"
		}
	}
	return "[object " + r.classOf(v) + "]"
}

// constructorName is the name of the constructor V8 reports for an object in
// messages such as "#<Object> is not extensible": the name of the function
// found as v's constructor, else "Object".
func (r *Realm) constructorName(v any) string {
	if ctor, ok := r.dataProperty(v, "constructor").(*js.Function); ok && ctor.Name != "" {
		return ctor.Name
	}
	return "Object"
}

// throwerAccessor is %ThrowTypeError% as the getter and setter of a strict-mode
// arguments object's callee.
func (r *Realm) throwerAccessor() *js.Accessor {
	if r.thrower == nil {
		f := NewNative("", 0, func(any, []any) (any, error) {
			return nil, js.NewTypeError("'caller', 'callee', and 'arguments' properties may not be accessed on strict mode functions or the arguments objects for calls to them")
		})
		r.thrower = &js.Accessor{Get: f, Set: f}
	}
	return r.thrower
}

// define installs a non-enumerable property, as builtins are, on an object or on
// a constructor function.
func (r *Realm) define(target any, name string, v any) {
	r.table(target, true).Define(name, v, attrsBuiltin)
}

// defineConstants installs read-only, non-enumerable, non-configurable values,
// such as Math.PI and the global NaN.
func (r *Realm) defineConstants(target any, kv ...any) {
	t := r.table(target, true)
	for i := 0; i+1 < len(kv); i += 2 {
		t.Define(kv[i].(string), kv[i+1], attrsConstant)
	}
}

// guardArray checks a mutation of an array that Object.preventExtensions, seal
// or freeze restricted before the mutation runs. steps replays the property
// writes and deletions the specification's algorithm performs, in order, through
// set and del; the first one the array refuses fails with V8's TypeError.
func (r *Realm) guardArray(a *js.Array, steps func(set, del func(i int) error) error) error {
	level := r.level(a)
	if level == extensible {
		return nil
	}
	n := len(a.Items)
	set := func(i int) error {
		switch {
		case i >= n && level >= nonExtensible:
			return notExtensibleError(strconv.Itoa(i))
		case i < n && level == frozen:
			return r.readOnlyError(a, strconv.Itoa(i))
		}
		return nil
	}
	del := func(i int) error {
		if level >= sealed {
			return r.deleteError(a, strconv.Itoa(i))
		}
		return nil
	}
	return steps(set, del)
}
