package edgert

import (
	"sync"

	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/lodash"
	"github.com/edge-js/edge-go/internal/stringifyattrs"
	"github.com/edge-js/edge-go/js"
)

// compatProps holds what a Props instance keeps under Symbol.for("options"):
// the props it was created with.
var compatProps Slots[jsrt.Derived, any]

// propsClass is the migrate plugin's Props, the $props of components in
// compat mode: the props copied onto the instance, `next` holding the
// ComponentProps of the newer API, and helpers to read and serialize the
// props as HTML attributes.
var propsClass = sync.OnceValue(func() *jsrt.Class {
	c := NewClass("Props", 1, nil, func(this any, args []any) error {
		props := js.Arg(args, 0)
		obj := this.(*jsrt.Derived)
		// The class field `next;`, then the constructor body.
		obj.Own.Set("next", js.Undefined)
		compatProps.Attach(obj, props)
		if err := ObjectAssign(this, props); err != nil {
			return err
		}
		next, err := NewComponentProps(props)
		if err != nil {
			return err
		}
		return Realm().Set(this, "next", next, true)
	})
	method := func(name string, length int, fn jsrt.NativeFunc) {
		DefineMethod(c.Proto, "Props."+name, name, length, fn)
	}
	method("has", 1, func(this any, args []any) (any, error) {
		v, err := Ops().Invoke(this, "get", []any{js.Arg(args, 0)}, "this.get")
		if err != nil {
			return nil, err
		}
		return !js.IsNullish(v), nil
	})
	method("get", 2, func(this any, args []any) (any, error) {
		all, err := Ops().Invoke(this, "all", nil, "this.all")
		if err != nil {
			return nil, err
		}
		return lodash.Get(Ops(), all, js.Arg(args, 0), js.Arg(args, 1))
	})
	method("all", 0, func(this any, _ []any) (any, error) {
		if props, ok := compatProps.Lookup(this); ok {
			return props, nil
		}
		// this[Symbol.for("options")].props on an object without the symbol.
		if js.IsNullish(this) {
			return nil, js.CannotReadProperties(this, "Symbol(options)")
		}
		return nil, js.CannotReadProperties(js.Undefined, "props")
	})
	method("validate", 2, func(this any, args []any) (any, error) {
		key := js.Arg(args, 0)
		v, err := Ops().Invoke(this, "get", []any{key}, "this.get")
		if err != nil {
			return nil, err
		}
		_, err = Ops().Call(js.Arg(args, 1), js.Undefined, []any{key, v}, "validateFn")
		return js.Undefined, err
	})
	method("only", 1, func(this any, args []any) (any, error) {
		all, err := Ops().Invoke(this, "all", nil, "this.all")
		if err != nil {
			return nil, err
		}
		return lodash.Pick(Ops(), all, js.Arg(args, 0))
	})
	method("except", 1, func(this any, args []any) (any, error) {
		all, err := Ops().Invoke(this, "all", nil, "this.all")
		if err != nil {
			return nil, err
		}
		return lodash.Omit(Ops(), all, js.Arg(args, 0))
	})
	method("serialize", 1, func(this any, args []any) (any, error) {
		return serializeProps(this, "all", nil, js.Arg(args, 0), js.Arg(args, 1))
	})
	method("serializeOnly", 2, func(this any, args []any) (any, error) {
		return serializeProps(this, "only", []any{js.Arg(args, 0)}, js.Arg(args, 1), js.Arg(args, 2))
	})
	method("serializeExcept", 2, func(this any, args []any) (any, error) {
		return serializeProps(this, "except", []any{js.Arg(args, 0)}, js.Arg(args, 1), js.Arg(args, 2))
	})
	return c
})

// NewProps is `new Props(props)`.
func NewProps(props any) (any, error) {
	return propsClass().Construct([]any{props})
}

// serializeProps is Props.serialize, serializeOnly and serializeExcept: the
// props this[pick](...pickArgs) returns merged with mergeProps (the props
// first, unless prioritizeInline is false), className folded into class,
// serialized with stringify-attributes and marked safe.
func serializeProps(this any, pick string, pickArgs []any, mergeProps, prioritizeInline any) (any, error) {
	if _, undefined := prioritizeInline.(js.UndefinedType); undefined {
		prioritizeInline = true
	}
	props, err := Ops().Invoke(this, pick, pickArgs, "this."+pick)
	if err != nil {
		return nil, err
	}
	sources := []any{props, mergeProps}
	if !js.ToBoolean(prioritizeInline) {
		sources = []any{mergeProps, props}
	}
	attributes, err := lodash.Merge(Ops(), js.NewObject(), sources...)
	if err != nil {
		return nil, err
	}
	if _, ok := compatProps.Lookup(this); !ok {
		// The private method #mergeClassAttributes checks its receiver.
		return nil, js.NewTypeError("Receiver must be an instance of class anonymous")
	}
	if err := mergeClassAttributes(attributes); err != nil {
		return nil, err
	}
	html, err := stringifyattrs.StringifyAttributes(Ops(), attributes)
	if err != nil {
		return nil, err
	}
	return HTMLSafe(html)
}

// mergeClassAttributes appends a truthy props.className to props.class
// (made an array) and sets className to false.
func mergeClassAttributes(props any) error {
	className, err := Realm().Get(props, "className")
	if err != nil || !js.ToBoolean(className) {
		return err
	}
	class, err := Realm().Get(props, "class")
	if err != nil {
		return err
	}
	if !js.ToBoolean(class) {
		if err := Realm().Set(props, "class", &js.Array{}, true); err != nil {
			return err
		}
	}
	if class, err = Realm().Get(props, "class"); err != nil {
		return err
	}
	isArray, err := Ops().IsArray(class)
	if err != nil {
		return err
	}
	if !isArray {
		if err := Realm().Set(props, "class", &js.Array{Items: []any{class}}, true); err != nil {
			return err
		}
	}
	if class, err = Realm().Get(props, "class"); err != nil {
		return err
	}
	if className, err = Realm().Get(props, "className"); err != nil {
		return err
	}
	joined, err := Ops().Invoke(class, "concat", []any{className}, "props.class.concat")
	if err != nil {
		return err
	}
	if err := Realm().Set(props, "class", joined, true); err != nil {
		return err
	}
	return Realm().Set(props, "className", false, true)
}
