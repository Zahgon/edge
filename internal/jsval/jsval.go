// Package jsval applies the JavaScript operations the ported libraries (classnames,
// lodash, @poppinss/inspect, stringify-attributes, ...) perform on template values.
// Every operation goes through the realm's own builtins (Object.prototype.toString,
// hasOwnProperty, Object.getPrototypeOf, ...), so getters, user toString/valueOf
// methods, prototype chains and the errors they throw behave exactly as they do when
// the libraries run in V8.
package jsval

import (
	"fmt"
	"strings"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsrt/unicase"
	"github.com/edge-js/edge-go/js"
)

// Ops runs JavaScript operations in one realm. The builtins it relies on are read
// once, when New is called, as the libraries capture `Object.prototype.toString` and
// friends when their module loads; create it with the realm, before template code
// can replace them.
type Ops struct {
	Realm *jsrt.Realm

	objectToString       *js.Function
	hasOwnProperty       *js.Function
	propertyIsEnumerable *js.Function
	getPrototypeOf       *js.Function
	getOwnPropertyNames  *js.Function
	isArray              *js.Function
	functionToString     *js.Function
	isPrototypeOf        *js.Function
	objectCtor           *js.Function
}

// New reads the builtins of r that the operations use.
func New(r *jsrt.Realm) (*Ops, error) {
	o := &Ops{Realm: r}
	for _, b := range []struct {
		dst  **js.Function
		path []string
	}{
		{&o.objectToString, []string{"Object", "prototype", "toString"}},
		{&o.hasOwnProperty, []string{"Object", "prototype", "hasOwnProperty"}},
		{&o.propertyIsEnumerable, []string{"Object", "prototype", "propertyIsEnumerable"}},
		{&o.isPrototypeOf, []string{"Object", "prototype", "isPrototypeOf"}},
		{&o.getPrototypeOf, []string{"Object", "getPrototypeOf"}},
		{&o.getOwnPropertyNames, []string{"Object", "getOwnPropertyNames"}},
		{&o.isArray, []string{"Array", "isArray"}},
		{&o.functionToString, []string{"Function", "prototype", "toString"}},
		{&o.objectCtor, []string{"Object"}},
	} {
		f, err := o.BuiltinFunction(b.path...)
		if err != nil {
			return nil, err
		}
		*b.dst = f
	}
	return o, nil
}

// Builtin reads a value reachable from the global object, such as
// Builtin("Map", "prototype", "forEach").
func (o *Ops) Builtin(path ...string) (any, error) {
	var v any = o.Realm.Global()
	for _, key := range path {
		next, err := o.Realm.Get(v, key)
		if err != nil {
			return nil, err
		}
		v = next
	}
	return v, nil
}

// BuiltinFunction is Builtin for a path that names a function.
func (o *Ops) BuiltinFunction(path ...string) (*js.Function, error) {
	v, err := o.Builtin(path...)
	if err != nil {
		return nil, err
	}
	f, ok := v.(*js.Function)
	if !ok {
		return nil, fmt.Errorf("jsval: the realm's %v is not a function", path)
	}
	return f, nil
}

// Get is v[key], running getters; null and undefined throw V8's TypeError.
func (o *Ops) Get(v any, key string) (any, error) { return o.Realm.Get(v, key) }

// Set is v[key] = val in strict-mode code (strict) or sloppy-mode code.
func (o *Ops) Set(v any, key string, val any, strict bool) error {
	return o.Realm.Set(v, key, val, strict)
}

// Delete is delete v[key] in strict-mode or sloppy-mode code.
func (o *Ops) Delete(v any, key string, strict bool) error {
	return o.Realm.Delete(v, key, strict)
}

// ToString is String(v) for values other than symbols: an object's toString (or
// valueOf) runs.
func (o *Ops) ToString(v any) (string, error) { return o.Realm.ToString(v) }

// ToPrimitive is the ToPrimitive operation with hint "default", "number" or
// "string": an object's valueOf and toString run in the order the hint gives, a
// Date treating "default" as "string", as V8 converts the operands of + and ==.
func (o *Ops) ToPrimitive(v any, hint string) (any, error) { return o.Realm.ToPrimitive(v, hint) }

// ToNumber is Number(v): ToPrimitive with hint number, then the numeric
// conversion.
func (o *Ops) ToNumber(v any) (float64, error) {
	p, err := o.ToPrimitive(v, "number")
	if err != nil {
		return 0, err
	}
	return js.ToNumber(p), nil
}

// Add is the + operator.
func (o *Ops) Add(a, b any) (any, error) { return js.Add(a, b, o.ToPrimitive) }

// AddString is `a + b` where the caller knows one operand is a string, so the
// result is one too.
func (o *Ops) AddString(a, b any) (string, error) {
	v, err := o.Add(a, b)
	if err != nil {
		return "", err
	}
	return js.ToString(v), nil
}

// ToObject is Object(v): null and undefined become a new empty object, as the
// Object function (not the abstract ToObject) converts them; primitives are boxed.
func (o *Ops) ToObject(v any) (any, error) {
	return o.Realm.Call(o.objectCtor, js.Undefined, []any{v})
}

// Tag is Object.prototype.toString.call(v), such as "[object Array]".
func (o *Ops) Tag(v any) (string, error) {
	res, err := o.Realm.Call(o.objectToString, v, nil)
	if err != nil {
		return "", err
	}
	return js.ToString(res), nil
}

// IsArray is Array.isArray(v).
func (o *Ops) IsArray(v any) (bool, error) {
	res, err := o.Realm.Call(o.isArray, js.Undefined, []any{v})
	return js.ToBoolean(res), err
}

// HasOwn is Object.prototype.hasOwnProperty.call(v, key); null and undefined
// throw V8's TypeError.
func (o *Ops) HasOwn(v any, key string) (bool, error) {
	res, err := o.Realm.Call(o.hasOwnProperty, v, []any{key})
	return js.ToBoolean(res), err
}

// IsEnumerable is Object.prototype.propertyIsEnumerable.call(v, key).
func (o *Ops) IsEnumerable(v any, key string) (bool, error) {
	res, err := o.Realm.Call(o.propertyIsEnumerable, v, []any{key})
	return js.ToBoolean(res), err
}

// Proto is Object.getPrototypeOf(v): nil (null) at the end of a chain.
func (o *Ops) Proto(v any) (any, error) {
	return o.Realm.Call(o.getPrototypeOf, js.Undefined, []any{v})
}

// IsPrototypeOf is proto.isPrototypeOf(v): whether proto is on v's prototype
// chain, which is what `v instanceof C` tests with proto = C.prototype.
func (o *Ops) IsPrototypeOf(proto, v any) (bool, error) {
	res, err := o.Realm.Call(o.isPrototypeOf, proto, []any{v})
	return js.ToBoolean(res), err
}

// InstanceOf is `v instanceof ctor` for a constructor without Symbol.hasInstance:
// whether ctor.prototype is on v's prototype chain. A primitive is never an
// instance.
func (o *Ops) InstanceOf(v any, ctor *js.Function) (bool, error) {
	if !js.IsObject(v) {
		return false, nil
	}
	proto, err := o.Get(ctor, "prototype")
	if err != nil {
		return false, err
	}
	if !js.IsObject(proto) {
		return false, js.NewTypeError("Function has non-object prototype '" + js.ToString(proto) + "' in instanceof check")
	}
	return o.IsPrototypeOf(proto, v)
}

// HasProperty is `key in Object(v)`: v or its prototype chain has the property.
func (o *Ops) HasProperty(v any, key string) (bool, error) {
	obj, err := o.ToObject(v)
	if err != nil {
		return false, err
	}
	for obj != nil {
		own, err := o.HasOwn(obj, key)
		if err != nil || own {
			return own, err
		}
		if obj, err = o.Proto(obj); err != nil {
			return false, err
		}
	}
	return false, nil
}

// Keys is Object.keys(v): own enumerable string keys in V8's order.
func (o *Ops) Keys(v any) ([]string, error) { return o.Realm.OwnEnumerableKeys(v) }

// OwnNames is Object.getOwnPropertyNames(v): every own string key, enumerable or
// not, in V8's order.
func (o *Ops) OwnNames(v any) ([]string, error) {
	res, err := o.Realm.Call(o.getOwnPropertyNames, js.Undefined, []any{v})
	if err != nil {
		return nil, err
	}
	arr, ok := res.(*js.Array)
	if !ok {
		return nil, fmt.Errorf("jsval: Object.getOwnPropertyNames returned %T", res)
	}
	names := make([]string, len(arr.Items))
	for i, k := range arr.Items {
		names[i] = js.ToString(k)
	}
	return names, nil
}

// ForIn runs body for every key a `for (key in v)` loop visits, in V8's order: the
// enumerable string keys of v, then of each prototype, skipping a key an earlier
// object already had (enumerable or not). As in V8, a key deleted by the loop
// body before its turn is not visited. Null and undefined visit nothing; body
// returning stop ends the loop.
func (o *Ops) ForIn(v any, body func(key string) (stop bool, err error)) error {
	if js.IsNullish(v) {
		return nil
	}
	obj, err := o.ToObject(v)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var keys []string
	for cur := obj; cur != nil; {
		names, err := o.OwnNames(cur)
		if err != nil {
			return err
		}
		for _, k := range names {
			if seen[k] {
				continue
			}
			seen[k] = true
			enumerable, err := o.IsEnumerable(cur, k)
			if err != nil {
				return err
			}
			if enumerable {
				keys = append(keys, k)
			}
		}
		if cur, err = o.Proto(cur); err != nil {
			return err
		}
	}
	for _, k := range keys {
		present, err := o.HasProperty(obj, k)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		stop, err := body(k)
		if err != nil || stop {
			return err
		}
	}
	return nil
}

// Call calls f with this and args. callee is the source text of the call's callee
// as the library writes it; when f is not a function the error is V8's
// "<callee> is not a function".
func (o *Ops) Call(f any, this any, args []any, callee string) (any, error) {
	fn, ok := f.(*js.Function)
	if !ok {
		return nil, js.NotAFunction(callee)
	}
	return o.Realm.Call(fn, this, args)
}

// Invoke is the method call v[key](...args), whose callee reads callee in the
// library's source.
func (o *Ops) Invoke(v any, key string, args []any, callee string) (any, error) {
	m, err := o.Get(v, key)
	if err != nil {
		return nil, err
	}
	return o.Call(m, v, args, callee)
}

// FunctionSource is Function.prototype.toString.call(v): the source text of a
// function, "function name() { [native code] }" for builtins; other values throw
// V8's TypeError.
func (o *Ops) FunctionSource(v any) (string, error) {
	res, err := o.Realm.Call(o.functionToString, v, nil)
	if err != nil {
		return "", err
	}
	return js.ToString(res), nil
}

// NewHostFunction returns a function JavaScript can call (a callback passed to
// forEach, a replacer), with the given name and length.
func NewHostFunction(name string, length int, call func(this any, args []any) (any, error)) *js.Function {
	return &js.Function{Name: name, Length: length, Call: call}
}

// ToUpperCase is String.prototype.toUpperCase: the full Unicode uppercase
// mapping, so "ß" becomes "SS".
func ToUpperCase(s string) string {
	if jsstr.IsASCII(s) {
		return strings.ToUpper(s)
	}
	return jsstr.FromUTF16(unicase.ToUpper(jsstr.ToUTF16(s)))
}

// ToLowerCase is String.prototype.toLowerCase: the full Unicode lowercase
// mapping, with the final-sigma rule.
func ToLowerCase(s string) string {
	if jsstr.IsASCII(s) {
		return strings.ToLower(s)
	}
	return jsstr.FromUTF16(unicase.ToLower(jsstr.ToUTF16(s)))
}

// MustCompile compiles one of a library's own regular expressions, which are
// valid: an error is a defect of the port.
func MustCompile(source, flags string) *jsregexp.Regexp {
	re, err := jsregexp.Compile(source, flags)
	if err != nil {
		panic(err)
	}
	return re
}

// SizeGetter is Object.getOwnPropertyDescriptor(<ctor>.prototype, "size").get,
// the getter of Map.prototype.size or Set.prototype.size, which libraries call
// to tell Maps and Sets from other objects (it throws for any other receiver).
func (o *Ops) SizeGetter(ctor string) (*js.Function, error) {
	getOwnPropertyDescriptor, err := o.BuiltinFunction("Object", "getOwnPropertyDescriptor")
	if err != nil {
		return nil, err
	}
	proto, err := o.Builtin(ctor, "prototype")
	if err != nil {
		return nil, err
	}
	desc, err := o.Realm.Call(getOwnPropertyDescriptor, js.Undefined, []any{proto, "size"})
	if err != nil {
		return nil, err
	}
	get, err := o.Get(desc, "get")
	if err != nil {
		return nil, err
	}
	f, ok := get.(*js.Function)
	if !ok {
		return nil, fmt.Errorf("jsval: %s.prototype.size has no getter", ctor)
	}
	return f, nil
}
