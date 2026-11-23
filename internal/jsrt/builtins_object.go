package jsrt

import (
	"math"

	"github.com/edge-js/edge-go/js"
)

// Object, Object.prototype and property descriptors.

// classOf is the tag Object.prototype.toString reports: the @@toStringTag v
// has or inherits, for the objects that define Symbol.toStringTag in
// JavaScript, else the builtin class of v.
func (r *Realm) classOf(v any) string {
	if tag := r.toStringTag(v); tag != "" {
		return tag
	}
	switch x := v.(type) {
	case *arguments:
		return "Arguments"
	case *iterator:
		return x.tag
	case *primitiveObject:
		return js.ClassOf(x.value)
	}
	return js.ClassOf(v)
}

// toStringTag is the string-valued Symbol.toStringTag property v has or
// inherits, "" when it has none: Map.prototype, Set.prototype, their weak
// counterparts and iterator prototypes, Promise.prototype and
// AsyncFunction.prototype carry one, and so do Math, JSON and Node's global
// object. An object made with Object.create(Set.prototype) reports "Set" as in
// V8, and a Map whose prototype was replaced does not.
func (r *Realm) toStringTag(v any) string {
	if !js.IsObject(v) {
		return ""
	}
	for p := v; p != nil; p = r.protoOf(p) {
		switch p {
		case any(r.mapProto):
			return "Map"
		case any(r.setProto):
			return "Set"
		case any(r.weakMapProto):
			return "WeakMap"
		case any(r.weakSetProto):
			return "WeakSet"
		case any(r.mapIteratorProto):
			return "Map Iterator"
		case any(r.setIteratorProto):
			return "Set Iterator"
		case any(r.async.promiseProto):
			return "Promise"
		case any(r.async.asyncFunctionProto):
			return "AsyncFunction"
		case any(r.generators.functionProto):
			return "GeneratorFunction"
		case any(r.generators.proto):
			return "Generator"
		case any(r.asyncGenerators.functionProto):
			return "AsyncGeneratorFunction"
		case any(r.asyncGenerators.proto):
			return "AsyncGenerator"
		case any(r.bigintProto):
			return "BigInt"
		case any(r.global):
			return "global"
		case r.global.Get("Math"):
			return "Math"
		case r.global.Get("JSON"):
			return "JSON"
		}
	}
	return ""
}

// toObject is ToObject for the builtins that accept any non-nullish value.
// Primitives are returned as they are: the builtins read their properties
// through the wrapper prototypes without allocating a wrapper.
func toObject(v any) (any, error) {
	if js.IsNullish(v) {
		return nil, ErrNullishToObject()
	}
	return v, nil
}

// objectFromValue is Object(v): a new object for null or undefined, a wrapper
// object for a primitive, v itself otherwise.
func (r *Realm) objectFromValue(v any) any {
	switch v.(type) {
	case nil, js.UndefinedType:
		return js.NewObject()
	case string, float64, bool, js.BigInt:
		return &primitiveObject{value: v}
	}
	return v
}

// sameValue is Object.is: like === but NaN equals NaN and +0 differs from -0.
func sameValue(a, b any) bool {
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if ok1 && ok2 {
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
		return x == y && math.Signbit(x) == math.Signbit(y)
	}
	return js.StrictEquals(a, b)
}

// propertyDescriptor is a Property Descriptor record: each field is present only
// when its has flag is set.
type propertyDescriptor struct {
	value                                 any
	get, set                              *js.Function
	writable, enumerable, configurable    bool
	hasValue, hasGet, hasSet, hasWritable bool
	hasEnumerable, hasConfigurable        bool
}

func (d *propertyDescriptor) isAccessor() bool { return d.hasGet || d.hasSet }
func (d *propertyDescriptor) isData() bool     { return d.hasValue || d.hasWritable }

// toPropertyDescriptor is ToPropertyDescriptor, reading the fields in the
// specification's order and failing with V8's messages.
func (r *Realm) toPropertyDescriptor(v any) (*propertyDescriptor, error) {
	if !js.IsObject(v) {
		return nil, js.NewTypeError("Property description must be an object: " + r.safeString(v))
	}
	d := &propertyDescriptor{}
	flag := func(key string, has *bool, val *bool) error {
		if !r.hasProperty(v, key) {
			return nil
		}
		x, err := r.get(v, key)
		if err != nil {
			return err
		}
		*has, *val = true, js.ToBoolean(x)
		return nil
	}
	if err := flag("enumerable", &d.hasEnumerable, &d.enumerable); err != nil {
		return nil, err
	}
	if err := flag("configurable", &d.hasConfigurable, &d.configurable); err != nil {
		return nil, err
	}
	if r.hasProperty(v, "value") {
		x, err := r.get(v, "value")
		if err != nil {
			return nil, err
		}
		d.value, d.hasValue = x, true
	}
	if err := flag("writable", &d.hasWritable, &d.writable); err != nil {
		return nil, err
	}
	accessor := func(key, what string, has *bool, fn **js.Function) error {
		if !r.hasProperty(v, key) {
			return nil
		}
		x, err := r.get(v, key)
		if err != nil {
			return err
		}
		*has = true
		switch f := x.(type) {
		case *js.Function:
			*fn = f
		case js.UndefinedType:
		default:
			return js.NewTypeError(what + " must be a function: " + r.safeString(x))
		}
		return nil
	}
	if err := accessor("get", "Getter", &d.hasGet, &d.get); err != nil {
		return nil, err
	}
	if err := accessor("set", "Setter", &d.hasSet, &d.set); err != nil {
		return nil, err
	}
	if d.isAccessor() && d.isData() {
		return nil, js.NewTypeError("Invalid property descriptor. Cannot both specify accessors and a value or writable attribute, " + r.safeString(v))
	}
	return d, nil
}

// fromPropertyDescriptor is FromPropertyDescriptor for v's own property key:
// the object Object.getOwnPropertyDescriptor returns, or undefined.
func (r *Realm) fromPropertyDescriptor(v any, key string) any {
	slot, attrs, ok := r.ownProperty(v, key)
	if !ok {
		return js.Undefined
	}
	o := js.NewObject()
	if acc, isAcc := slot.(*js.Accessor); isAcc {
		o.Set("get", funcOrUndefined(acc.Get))
		o.Set("set", funcOrUndefined(acc.Set))
	} else {
		o.Set("value", slot)
		o.Set("writable", attrs&js.ReadOnly == 0)
	}
	o.Set("enumerable", attrs&js.Hidden == 0)
	o.Set("configurable", attrs&js.Fixed == 0)
	return o
}

func funcOrUndefined(f *js.Function) any {
	if f == nil {
		return js.Undefined
	}
	return f
}

// attrsOf converts a descriptor's flags, absent ones false, into attributes.
func (d *propertyDescriptor) attrsOf() js.Attr {
	var a js.Attr
	if !d.writable && !d.isAccessor() {
		a |= js.ReadOnly
	}
	if !d.enumerable {
		a |= js.Hidden
	}
	if !d.configurable {
		a |= js.Fixed
	}
	return a
}

// defineOwnProperty is [[DefineOwnProperty]] (ValidateAndApplyPropertyDescriptor)
// as Object.defineProperty runs it: an invalid change throws V8's TypeError.
func (r *Realm) defineOwnProperty(v any, key string, d *propertyDescriptor) error {
	cur, attrs, ok := r.ownProperty(v, key)
	if !ok {
		if !r.isExtensible(v) {
			return js.NewTypeError("Cannot define property " + key + ", object is not extensible")
		}
		var slot any = js.Undefined
		if d.isAccessor() {
			slot = &js.Accessor{Get: d.get, Set: d.set}
		} else if d.hasValue {
			slot = d.value
		}
		return r.storeOwn(v, key, slot, d.attrsOf())
	}
	curAcc, wasAccessor := cur.(*js.Accessor)
	if attrs&js.Fixed != 0 {
		redefine := js.NewTypeError("Cannot redefine property: " + key)
		if d.hasConfigurable && d.configurable || d.hasEnumerable && d.enumerable == (attrs&js.Hidden != 0) {
			return redefine
		}
		switch {
		case !d.isAccessor() && !d.isData():
		case wasAccessor != d.isAccessor():
			return redefine
		case wasAccessor:
			if d.hasGet && d.get != curAcc.Get || d.hasSet && d.set != curAcc.Set {
				return redefine
			}
		case attrs&js.ReadOnly != 0:
			if d.hasWritable && d.writable || d.hasValue && !sameValue(d.value, cur) {
				return redefine
			}
		}
	}
	slot := cur
	switch {
	case d.isAccessor():
		acc := &js.Accessor{}
		if wasAccessor {
			*acc = *curAcc
		}
		if d.hasGet {
			acc.Get = d.get
		}
		if d.hasSet {
			acc.Set = d.set
		}
		slot = acc
		attrs &^= js.ReadOnly
	case d.isData():
		if wasAccessor {
			slot = js.Undefined
			attrs |= js.ReadOnly
		}
		if d.hasValue {
			slot = d.value
		}
		if d.hasWritable {
			attrs = setAttr(attrs, js.ReadOnly, !d.writable)
		}
	}
	if d.hasEnumerable {
		attrs = setAttr(attrs, js.Hidden, !d.enumerable)
	}
	if d.hasConfigurable {
		attrs = setAttr(attrs, js.Fixed, !d.configurable)
	}
	return r.storeOwn(v, key, slot, attrs)
}

func setAttr(a, bit js.Attr, on bool) js.Attr {
	if on {
		return a | bit
	}
	return a &^ bit
}

// storeOwn writes an own property with the given attributes. The properties the
// value model keeps in Go fields take the value only: array items, an array's
// length, a function's name and length, an error's message and name.
func (r *Realm) storeOwn(v any, key string, slot any, attrs js.Attr) error {
	_, isAcc := slot.(*js.Accessor)
	switch x := v.(type) {
	case *js.Array:
		if key == "length" && !isAcc {
			return r.setArrayLength(x, slot, true)
		}
		if i, ok := js.ArrayIndex(key); ok && !isAcc {
			x.SetIndex(i, slot)
			return nil
		}
	case *js.Function:
		if (key == "name" || key == "length") && !isAcc {
			if key == "name" {
				x.Name = js.ToString(slot)
			} else {
				x.Length = int(js.ToIntegerOrInfinity(js.ToNumber(slot)))
			}
			return nil
		}
	case *js.Error:
		if key == "message" && !isAcc {
			x.SetMessage(slot)
			if x.Props != nil && x.Props.Has(key) {
				x.Props.SetAttrs(key, attrs)
			}
			return nil
		}
		if key == "name" && !isAcc {
			s, err := r.toString(slot)
			if err != nil {
				return err
			}
			x.Name = s
			return nil
		}
	case *js.RegExp:
		if key == "lastIndex" && !isAcc {
			r.regexpState(x).lastIndex = slot
			return nil
		}
	case *arguments:
		if i, ok := js.ArrayIndex(key); ok && i < len(x.mapped) {
			x.mapped[i] = nil
		}
	}
	r.table(v, true).Define(key, slot, attrs)
	return nil
}

// createDataProperty is CreateDataProperty: an own writable, enumerable,
// configurable property, defined rather than assigned, so no setter runs.
func (r *Realm) createDataProperty(o *js.Object, key string, v any) {
	o.Define(key, v, 0)
}

// copyDataProperties is CopyDataProperties for object spread and rest: the own
// enumerable properties of src, read with their getters, except the excluded keys.
func (r *Realm) copyDataProperties(target *js.Object, src any, excluded map[string]bool) error {
	if js.IsNullish(src) {
		return nil
	}
	for _, k := range r.ownKeys(src) {
		if excluded[k] {
			continue
		}
		v, err := r.get(src, k)
		if err != nil {
			return err
		}
		r.createDataProperty(target, k, v)
	}
	return nil
}

// enumerableOwn lists the own enumerable properties of v as Object.keys,
// Object.values and Object.entries see them: a key deleted or made
// non-enumerable by a getter read earlier is skipped.
func (r *Realm) enumerableOwn(v any, visit func(key string, val any)) error {
	for _, k := range r.ownKeys(v) {
		_, attrs, ok := r.ownProperty(v, k)
		if !ok || attrs&js.Hidden != 0 {
			continue
		}
		val, err := r.get(v, k)
		if err != nil {
			return err
		}
		visit(k, val)
	}
	return nil
}

func stringsToArray(keys []string) *js.Array {
	out := make([]any, len(keys))
	for i, k := range keys {
		out[i] = k
	}
	return js.NewArray(out...)
}

// defineProperties is ObjectDefineProperties: every descriptor is read before
// any property is defined.
func (r *Realm) defineProperties(o, props any) error {
	if js.IsNullish(props) {
		return ErrNullishToObject()
	}
	type entry struct {
		key  string
		desc *propertyDescriptor
	}
	var entries []entry
	for _, k := range r.ownKeys(props) {
		dv, err := r.get(props, k)
		if err != nil {
			return err
		}
		d, err := r.toPropertyDescriptor(dv)
		if err != nil {
			return err
		}
		entries = append(entries, entry{k, d})
	}
	for _, e := range entries {
		if err := r.defineOwnProperty(o, e.key, e.desc); err != nil {
			return err
		}
	}
	return nil
}

func (r *Realm) setupObject() {
	op := r.objectProto
	obj := r.constructor("Object", 1, op, func(_ any, args []any) (any, error) {
		return r.objectFromValue(js.Arg(args, 0)), nil
	}, func(args []any) (any, error) {
		return r.objectFromValue(js.Arg(args, 0)), nil
	})
	r.setupObjectPrototype()

	keyArg := func(args []any, i int) (string, error) { return r.toPropertyKey(js.Arg(args, i)) }
	r.method(obj, "keys", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return stringsToArray(r.ownKeys(v)), nil
	})
	r.method(obj, "values", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		out := js.NewArray()
		err = r.enumerableOwn(v, func(_ string, val any) { out.Items = append(out.Items, val) })
		return out, err
	})
	r.method(obj, "entries", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		out := js.NewArray()
		err = r.enumerableOwn(v, func(k string, val any) { out.Items = append(out.Items, js.NewArray(k, val)) })
		return out, err
	})
	r.method(obj, "getOwnPropertyNames", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return stringsToArray(r.ownPropertyKeys(v, true)), nil
	})
	r.method(obj, "assign", 2, func(_ any, args []any) (any, error) {
		target, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		target = r.objectFromValue(target)
		for _, src := range args[min(1, len(args)):] {
			if js.IsNullish(src) {
				continue
			}
			for _, k := range r.ownKeys(src) {
				v, err := r.get(src, k)
				if err != nil {
					return nil, err
				}
				if err := r.put(target, k, v, true); err != nil {
					return nil, err
				}
			}
		}
		return target, nil
	})
	r.method(obj, "fromEntries", 1, func(_ any, args []any) (any, error) {
		src := js.Arg(args, 0)
		if js.IsNullish(src) {
			return nil, js.NewTypeError(js.ToString(src) + " is not iterable")
		}
		o := js.NewObject()
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
			key, err := r.toPropertyKey(k)
			if err != nil {
				return err
			}
			r.createDataProperty(o, key, v)
			return nil
		})
		return o, err
	})
	r.method(obj, "create", 2, func(_ any, args []any) (any, error) {
		proto := js.Arg(args, 0)
		if !js.IsObject(proto) && proto != nil {
			return nil, js.NewTypeError("Object prototype may only be an Object or null: " + r.safeString(proto))
		}
		o := &Derived{Own: js.NewObject(), Proto: proto}
		if props := js.Arg(args, 1); !isUndefined(props) {
			if err := r.defineProperties(o, props); err != nil {
				return nil, err
			}
		}
		return o, nil
	})
	r.method(obj, "getPrototypeOf", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return r.protoOf(v), nil
	})
	r.method(obj, "setPrototypeOf", 2, func(_ any, args []any) (any, error) {
		v, proto := js.Arg(args, 0), js.Arg(args, 1)
		if js.IsNullish(v) {
			return nil, js.NewTypeError("Object.setPrototypeOf called on null or undefined")
		}
		if !js.IsObject(proto) && proto != nil {
			return nil, js.NewTypeError("Object prototype may only be an Object or null: " + r.safeString(proto))
		}
		if !js.IsObject(v) {
			return v, nil
		}
		return v, r.setPrototype(v, proto)
	})
	r.method(obj, "defineProperty", 3, func(_ any, args []any) (any, error) {
		o := js.Arg(args, 0)
		if !js.IsObject(o) {
			return nil, js.NewTypeError("Object.defineProperty called on non-object")
		}
		key, err := keyArg(args, 1)
		if err != nil {
			return nil, err
		}
		d, err := r.toPropertyDescriptor(js.Arg(args, 2))
		if err != nil {
			return nil, err
		}
		return o, r.defineOwnProperty(o, key, d)
	})
	r.method(obj, "defineProperties", 2, func(_ any, args []any) (any, error) {
		o := js.Arg(args, 0)
		if !js.IsObject(o) {
			return nil, js.NewTypeError("Object.defineProperties called on non-object")
		}
		return o, r.defineProperties(o, js.Arg(args, 1))
	})
	r.method(obj, "getOwnPropertyDescriptor", 2, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		key, err := keyArg(args, 1)
		if err != nil {
			return nil, err
		}
		return r.fromPropertyDescriptor(v, key), nil
	})
	r.method(obj, "getOwnPropertyDescriptors", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		out := js.NewObject()
		for _, k := range r.ownPropertyKeys(v, true) {
			r.createDataProperty(out, k, r.fromPropertyDescriptor(v, k))
		}
		return out, nil
	})
	integrityMethods := []struct {
		set, test string
		level     integrity
	}{
		{"preventExtensions", "", nonExtensible},
		{"seal", "isSealed", sealed},
		{"freeze", "isFrozen", frozen},
	}
	for _, m := range integrityMethods {
		r.method(obj, m.set, 1, func(_ any, args []any) (any, error) {
			v := js.Arg(args, 0)
			r.setIntegrity(v, m.level)
			return v, nil
		})
		if m.test != "" {
			r.method(obj, m.test, 1, func(_ any, args []any) (any, error) {
				return r.testIntegrity(js.Arg(args, 0), m.level), nil
			})
		}
	}
	r.method(obj, "isExtensible", 1, func(_ any, args []any) (any, error) {
		return r.isExtensible(js.Arg(args, 0)), nil
	})
	r.method(obj, "is", 2, func(_ any, args []any) (any, error) {
		return sameValue(js.Arg(args, 0), js.Arg(args, 1)), nil
	})
	r.method(obj, "groupBy", 2, func(_ any, args []any) (any, error) {
		groups := WithProto(nil, nil)
		err := r.groupBy("Object.groupBy", args, func(k any) (string, error) { return r.toPropertyKey(k) }, func(key string, v any) {
			if g, ok := groups.Own.Get(key).(*js.Array); ok && groups.Own.Has(key) {
				g.Items = append(g.Items, v)
				return
			}
			groups.Own.Set(key, js.NewArray(v))
		})
		return groups, err
	})
	r.method(obj, "hasOwn", 2, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		key, err := keyArg(args, 1)
		if err != nil {
			return nil, err
		}
		return r.hasOwn(v, key), nil
	})
}

func (r *Realm) setupObjectPrototype() {
	op := r.objectProto
	r.method(op, "hasOwnProperty", 1, func(this any, args []any) (any, error) {
		key, err := r.toPropertyKey(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		if _, err := toObject(this); err != nil {
			return nil, err
		}
		return r.hasOwn(this, key), nil
	})
	r.method(op, "isPrototypeOf", 1, func(this any, args []any) (any, error) {
		v := js.Arg(args, 0)
		if !js.IsObject(v) {
			return false, nil
		}
		for p := r.protoOf(v); p != nil; p = r.protoOf(p) {
			if p == this {
				return true, nil
			}
		}
		return false, nil
	})
	r.method(op, "propertyIsEnumerable", 1, func(this any, args []any) (any, error) {
		key, err := r.toPropertyKey(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return r.hasOwn(this, key) && !r.isOwnNonEnumerable(this, key), nil
	})
	r.objectToString = r.method(op, "toString", 0, func(this any, _ []any) (any, error) {
		return "[object " + r.classOf(this) + "]", nil
	})
	r.method(op, "toLocaleString", 0, func(this any, _ []any) (any, error) {
		m, err := r.getV(this, "toString")
		if err != nil {
			return nil, err
		}
		return r.callValue(m, this, nil)
	})
	r.method(op, "valueOf", 0, func(this any, _ []any) (any, error) {
		v, err := toObject(this)
		if err != nil {
			return nil, err
		}
		return r.objectFromValue(v), nil
	})
	getProto := NewNative("get __proto__", 0, func(this any, _ []any) (any, error) {
		v, err := toObject(this)
		if err != nil {
			return nil, err
		}
		return r.protoOf(v), nil
	})
	setProto := NewNative("set __proto__", 1, func(this any, args []any) (any, error) {
		if js.IsNullish(this) {
			return nil, js.NewTypeError("Object.prototype.__proto__ called on null or undefined")
		}
		proto := js.Arg(args, 0)
		if (!js.IsObject(proto) && proto != nil) || !js.IsObject(this) {
			return js.Undefined, nil
		}
		return js.Undefined, r.setPrototype(this, proto)
	})
	op.Define("__proto__", &js.Accessor{Get: getProto, Set: setProto}, js.Hidden)
}

// groupBy is GroupBy for Object.groupBy and Map.groupBy: it calls the callback
// args[1] with each value of the iterable args[0] and its index, converts the
// result with toKey and adds the value to that key's group.
func (r *Realm) groupBy(method string, args []any, toKey func(any) (string, error), add func(key string, v any)) error {
	items := js.Arg(args, 0)
	if js.IsNullish(items) {
		return js.NewTypeError(method + " called on null or undefined")
	}
	cb, ok := js.Arg(args, 1).(*js.Function)
	if !ok {
		return js.NotAFunction(r.safeString(js.Arg(args, 1)))
	}
	i := 0
	return r.forEachIterated(items, func() error { return notIterableValue(items) }, func(v any) error {
		k, err := r.call(cb, js.Undefined, []any{v, float64(i)})
		if err != nil {
			return err
		}
		i++
		key, err := toKey(k)
		if err != nil {
			return err
		}
		add(key, v)
		return nil
	})
}
