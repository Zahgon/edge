package jsrt

import (
	"slices"

	"github.com/edge-js/edge-go/js"
)

// Classes (ECMAScript §15.7): ClassDefinitionEvaluation, the [[Construct]] of
// class constructors, super calls and properties, and private names.
//
// A class constructor is a closure whose class field holds the class: the
// instance fields and private methods its constructor installs on every new
// object. Its [[Call]] throws, as V8's does. The prototype chain of a derived
// class runs through the parent class (the constructor's [[Prototype]]) and
// the parent's prototype object (the prototype's [[Prototype]]).

// classRecord is what a class constructor needs beyond its code: the elements
// InitializeInstanceElements installs on each instance, in order.
type classRecord struct {
	name string
	// methods are the private methods and accessors of instances, which every
	// instance gets before its fields.
	methods []*privateName
	fields  []*classField
	// explicitCtor reports a constructor written in the class body; a derived
	// class without one passes its arguments to the parent constructor.
	explicitCtor bool
}

// classField is a field definition: the key (a property key or a private
// name) and the initializer, run as a method of the instance (nil when the
// field has no initializer).
type classField struct {
	key     string
	private *privateName
	init    *js.Function
}

// privateName is a Private Name: the identity of #x for one evaluation of a
// class body. A private method or accessor keeps its functions here; every
// object the class initializes shares them.
type privateName struct {
	desc string
	kind classElementKind
	// method is a private method; get and set are the halves of a private
	// accessor (either may be nil).
	method, get, set *js.Function
}

// privateElements are the private fields, methods and accessors an object
// carries, by private name.
type privateElements map[*privateName]*privateElement

// privateElement is one private element of an object: a field's value, or a
// method or accessor whose functions are its private name's.
type privateElement struct {
	value any
}

// evalClass is ClassDefinitionEvaluation: it creates the class constructor
// and its prototype, defines the methods and accessors, and runs the static
// fields and blocks. name is the inferred name of an anonymous class
// expression.
func (r *Realm) evalClass(ci *classInfo, e *env, name string) (*js.Function, error) {
	ce := newEnv(e)
	ce.strict = true
	if ci.name != "" {
		name = ci.name
		ce.vars[ci.name] = &binding{}
	}
	if len(ci.privateNames) > 0 {
		ce.privates = map[string]*privateName{}
		for _, n := range ci.privateNames {
			ce.privates[n] = &privateName{desc: n}
		}
	}
	protoParent, ctorParent := any(r.objectProto), any(r.functionProto)
	if ci.heritage != nil {
		sv, err := r.eval(ci.heritage, ce)
		if err != nil {
			return nil, err
		}
		switch {
		case sv == nil:
			protoParent = nil
		case !isConstructor(sv):
			return nil, js.NewTypeError("Class extends value " + r.safeString(sv) + " is not a constructor or null")
		default:
			pp, err := r.get(sv, "prototype")
			if err != nil {
				return nil, err
			}
			if pp != nil && !js.IsObject(pp) {
				return nil, js.NewTypeError("Class extends value does not have valid prototype property " + r.safeString(pp))
			}
			protoParent, ctorParent = pp, sv
		}
	}
	var proto any
	if protoParent == any(r.objectProto) {
		proto = js.NewObject()
	} else {
		proto = WithProto(nil, protoParent)
	}
	rec := &classRecord{name: name, explicitCtor: ci.explicitCtor}
	f := &js.Function{Name: name, Length: ci.ctor.length}
	f.Call = func(any, []any) (any, error) {
		if f.Name == "" {
			return nil, js.NewTypeError("Class constructors cannot be invoked without 'new'")
		}
		return nil, js.NewTypeError("Class constructor " + f.Name + " cannot be invoked without 'new'")
	}
	f.Internal = &closure{fi: ci.ctor, env: ce, home: proto, class: rec}
	if ctorParent != any(r.functionProto) {
		r.protoLinks[f] = ctorParent
	}
	r.table(f, true).Define("prototype", proto, attrsConstant)
	r.define(proto, "constructor", f)

	// The elements in order: methods and accessors are defined now, fields
	// and static blocks are collected with their keys evaluated.
	var staticMethods []*privateName
	var staticElements []any // *classField or the static block's *js.Function
	for _, el := range ci.elements {
		home := proto
		if el.static {
			home = f
		}
		key := el.key
		if el.computed != nil {
			kv, err := r.eval(el.computed, ce)
			if err != nil {
				return nil, err
			}
			if key, err = r.toPropertyKey(kv); err != nil {
				return nil, err
			}
			if el.static && key == "prototype" {
				return nil, js.NewTypeError("Classes may not have a static property named 'prototype'")
			}
		}
		switch el.kind {
		case elemMethod, elemGetter, elemSetter:
			fnName := key
			if el.kind != elemMethod {
				fnName = accessorPrefix(el.kind == elemGetter) + key
			}
			m := r.makeMethod(el.fn, ce, fnName, home)
			if el.private {
				pn := ce.privates[key]
				pn.kind = el.kind
				switch el.kind {
				case elemMethod:
					pn.method = m
				case elemGetter:
					pn.get = m
				default:
					pn.set = m
				}
				// A getter and a setter of one name share it.
				list := &rec.methods
				if el.static {
					list = &staticMethods
				}
				if !slices.Contains(*list, pn) {
					*list = append(*list, pn)
				}
				continue
			}
			d := &propertyDescriptor{enumerable: false, hasEnumerable: true, configurable: true, hasConfigurable: true}
			switch el.kind {
			case elemMethod:
				d.value, d.hasValue, d.writable, d.hasWritable = m, true, true, true
			case elemGetter:
				d.get, d.hasGet = m, true
			default:
				d.set, d.hasSet = m, true
			}
			if err := r.defineOwnProperty(home, key, d); err != nil {
				return nil, err
			}
		case elemField:
			fd := &classField{key: key}
			if el.private {
				fd.private = ce.privates[key]
				fd.private.kind = elemField
			}
			if el.fn != nil {
				fd.init = r.makeMethod(el.fn, ce, "", home)
			}
			if el.static {
				staticElements = append(staticElements, fd)
			} else {
				rec.fields = append(rec.fields, fd)
			}
		case elemStaticBlock:
			staticElements = append(staticElements, r.makeMethod(el.fn, ce, "", f))
		}
	}
	if ci.name != "" {
		b := ce.vars[ci.name]
		b.value, b.initialized = f, true
	}
	if err := r.addPrivateMethods(f, staticMethods, name); err != nil {
		return nil, err
	}
	for _, el := range staticElements {
		var err error
		switch x := el.(type) {
		case *classField:
			err = r.defineField(f, x)
		case *js.Function:
			_, err = r.call(x, f, nil)
		}
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

// constructClass is the [[Construct]] of a class constructor. A base class
// creates the object from new.target's prototype and initializes its fields
// before the constructor body runs; a derived class leaves that to super().
func (r *Realm) constructClass(c *closure, f *js.Function, args []any, newTarget *js.Function) (any, error) {
	if err := r.enterCall(); err != nil {
		return nil, err
	}
	defer r.leaveCall()
	fi := c.fi
	if !fi.derived {
		proto, err := r.prototypeFromConstructor(newTarget, r.objectProto)
		if err != nil {
			return nil, err
		}
		obj := WithProto(nil, proto)
		if err := r.initializeInstance(obj, f); err != nil {
			return nil, err
		}
		fe, err := r.enterClosure(c, f, obj, args, newTarget)
		if err != nil {
			return nil, err
		}
		res, err := r.runBody(fi, fe)
		if err != nil {
			return nil, err
		}
		if js.IsObject(res) {
			return res, nil
		}
		return obj, nil
	}
	if !c.class.explicitCtor {
		// constructor(...args) { super(...args) }
		return r.superConstruct(f, args, newTarget)
	}
	fe, err := r.enterClosure(c, f, js.Undefined, args, newTarget)
	if err != nil {
		return nil, err
	}
	res, err := r.runBody(fi, fe)
	if err != nil {
		return nil, err
	}
	if js.IsObject(res) {
		return res, nil
	}
	if !isUndefined(res) {
		return nil, js.NewTypeError("Derived constructors may only return object or undefined")
	}
	return r.thisOf(fe)
}

// superConstruct is the work of super(...args) in the constructor f: the
// parent constructor (f's [[Prototype]]) constructs the object with
// new.target passed on, and f's fields are initialized on it.
func (r *Realm) superConstruct(f *js.Function, args []any, newTarget *js.Function) (any, error) {
	parent := r.protoOf(f)
	pf, ok := parent.(*js.Function)
	if !ok || !isConstructor(pf) {
		// V8 names a function parent (Function.prototype among them) by its
		// name, "null" when it has none.
		shown := "null"
		switch {
		case ok && pf.Name != "":
			shown = pf.Name
		case !ok && parent != nil && parent != any(r.functionProto):
			shown = r.safeString(parent)
		}
		of := "anonymous class"
		if f.Name != "" {
			of = f.Name
		}
		return nil, js.NewTypeError("Super constructor " + shown + " of " + of + " is not a constructor")
	}
	obj, err := r.constructWith(pf, args, newTarget, pf.Name)
	if err != nil {
		return nil, err
	}
	if err := r.initializeInstance(obj, f); err != nil {
		return nil, err
	}
	return obj, nil
}

// evalSuperCall evaluates super(args): it binds the constructor's this to
// the object the parent constructor returns.
func (r *Realm) evalSuperCall(n *superCall, e *env) (any, error) {
	te := thisEnv(e)
	args, err := r.evalArgs(n.args, e)
	if err != nil {
		return nil, err
	}
	nt, _ := te.newTarget.(*js.Function)
	obj, err := r.superConstruct(te.callee, args, nt)
	if err != nil {
		return nil, err
	}
	if !te.thisUninit {
		return nil, js.NewReferenceError("Super constructor may only be called once")
	}
	te.this, te.thisUninit = obj, false
	return obj, nil
}

// initializeInstance is InitializeInstanceElements: the private methods and
// then the fields of the class f, in order, on the new object obj.
func (r *Realm) initializeInstance(obj any, f *js.Function) error {
	c, _ := closureOf(f)
	if c == nil || c.class == nil {
		return nil
	}
	if err := r.addPrivateMethods(obj, c.class.methods, c.class.name); err != nil {
		return err
	}
	for _, fd := range c.class.fields {
		if err := r.defineField(obj, fd); err != nil {
			return err
		}
	}
	return nil
}

// addPrivateMethods is PrivateMethodOrAccessorAdd for each of a class's
// private methods and accessors.
func (r *Realm) addPrivateMethods(obj any, methods []*privateName, className string) error {
	if len(methods) == 0 {
		return nil
	}
	els := r.privateElementsOf(obj, true)
	if _, ok := els[methods[0]]; ok {
		return js.NewTypeError("Cannot initialize private methods of class " + className + " twice on the same object")
	}
	for _, pn := range methods {
		els[pn] = &privateElement{}
	}
	return nil
}

// defineField is DefineField: the initializer's value (undefined without
// one), named after the field when it is an anonymous function, becomes the
// object's own property or private field.
func (r *Realm) defineField(obj any, fd *classField) error {
	var v any = js.Undefined
	if fd.init != nil {
		c, _ := closureOf(fd.init)
		name := fd.key
		if fd.private != nil {
			name = fd.private.desc
		}
		var err error
		if v, err = r.runFieldInitializer(c, fd.init, obj, name); err != nil {
			return err
		}
	}
	if fd.private != nil {
		els := r.privateElementsOf(obj, true)
		if _, ok := els[fd.private]; ok {
			return js.NewTypeError("Cannot initialize " + fd.private.desc + " twice on the same object")
		}
		els[fd.private] = &privateElement{value: v}
		return nil
	}
	return r.defineOwnProperty(obj, fd.key, &propertyDescriptor{
		value: v, hasValue: true, writable: true, hasWritable: true,
		enumerable: true, hasEnumerable: true, configurable: true, hasConfigurable: true,
	})
}

// runFieldInitializer evaluates a field's initializer as a method of obj,
// naming an anonymous function or class it creates after the field.
func (r *Realm) runFieldInitializer(c *closure, f *js.Function, obj any, name string) (any, error) {
	if err := r.enterCall(); err != nil {
		return nil, err
	}
	defer r.leaveCall()
	fe, err := r.enterClosure(c, f, obj, nil, js.Undefined)
	if err != nil {
		return nil, err
	}
	return r.evalNamed(c.fi.exprBody, fe, name)
}

// privateElementsOf returns the private elements of obj, creating the table
// when create is set. Primitives have none.
func (r *Realm) privateElementsOf(obj any, create bool) privateElements {
	els := r.privates[obj]
	if els == nil && create && js.IsObject(obj) {
		els = privateElements{}
		r.privates[obj] = els
	}
	return els
}

// resolvePrivateName finds the private name #x that code in e refers to: the
// one of the innermost enclosing class that declares it.
func resolvePrivateName(e *env, name string) *privateName {
	for ; e != nil; e = e.parent {
		if pn, ok := e.privates[name]; ok {
			return pn
		}
	}
	return nil
}

// privateGet is PrivateGet: obj.#x.
func (r *Realm) privateGet(obj any, pn *privateName) (any, error) {
	if js.IsNullish(obj) {
		return nil, js.CannotReadProperties(obj, pn.desc)
	}
	el, ok := r.privateElementsOf(obj, false)[pn]
	if !ok {
		return nil, js.NewTypeError("Cannot read private member " + pn.desc + " from an object whose class did not declare it")
	}
	switch pn.kind {
	case elemField:
		return el.value, nil
	case elemMethod:
		return pn.method, nil
	}
	if pn.get == nil {
		return nil, js.NewTypeError("'" + pn.desc + "' was defined without a getter")
	}
	return r.call(pn.get, obj, nil)
}

// privateSet is PrivateSet: obj.#x = v.
func (r *Realm) privateSet(obj any, pn *privateName, v any) error {
	if js.IsNullish(obj) {
		return js.NewTypeError("Cannot set properties of " + js.ToString(obj) + " (setting '" + pn.desc + "')")
	}
	el, ok := r.privateElementsOf(obj, false)[pn]
	if !ok {
		return js.NewTypeError("Cannot write private member " + pn.desc + " to an object whose class did not declare it")
	}
	switch pn.kind {
	case elemField:
		el.value = v
		return nil
	case elemMethod:
		return js.NewTypeError("Private method '" + pn.desc + "' is not writable")
	}
	if pn.set == nil {
		return js.NewTypeError("'" + pn.desc + "' was defined without a setter")
	}
	_, err := r.call(pn.set, obj, []any{v})
	return err
}

// evalPrivateIn evaluates "#x in obj".
func (r *Realm) evalPrivateIn(n *privateIn, e *env) (any, error) {
	obj, err := r.eval(n.obj, e)
	if err != nil {
		return nil, err
	}
	if !js.IsObject(obj) {
		return nil, js.NewTypeError("Cannot use 'in' operator to search for '" + n.name + "' in " + r.safeString(obj))
	}
	_, ok := r.privateElementsOf(obj, false)[resolvePrivateName(e, n.name)]
	return ok, nil
}

// superBase is the object super.x reads from in e: the prototype of the
// method's home object, with the this value a call on it uses.
func (r *Realm) superBase(e *env) (base, this any, err error) {
	this, err = r.thisOf(e)
	if err != nil {
		return nil, nil, err
	}
	return r.protoOf(thisEnv(e).home), this, nil
}

// superGet is super[key]: the property of the home object's prototype, with
// a getter called on this.
func (r *Realm) superGet(base any, key string, this any) (any, error) {
	if base == nil {
		return nil, js.CannotReadProperties(nil, key)
	}
	slot, _, ok := r.findProperty(base, key)
	if !ok {
		return js.Undefined, nil
	}
	if acc, isAcc := slot.(*js.Accessor); isAcc {
		if acc.Get == nil {
			return js.Undefined, nil
		}
		return r.call(acc.Get, this, nil)
	}
	return slot, nil
}

// superSet is super[key] = v: an inherited setter is called on this, a
// read-only property refuses the write, and anything else writes this's own
// property.
func (r *Realm) superSet(base any, key string, v, this any) error {
	if base != nil {
		if slot, attrs, ok := r.findProperty(base, key); ok {
			if acc, isAcc := slot.(*js.Accessor); isAcc {
				if acc.Set == nil {
					return js.NewTypeError("Cannot set property " + key + " of " + r.safeString(base) + " which has only a getter")
				}
				_, err := r.call(acc.Set, this, []any{v})
				return err
			}
			if attrs&js.ReadOnly != 0 {
				return r.readOnlyError(this, key)
			}
		}
	}
	if _, _, own := r.ownProperty(this, key); own {
		return r.writeOwn(this, key, v, true)
	}
	return r.addProperty(this, key, v, true)
}
