package edgert

import (
	"sync"

	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/lodash"
	"github.com/edge-js/edge-go/js"
)

// componentPropsValues holds the private #values field of ComponentProps
// instances.
var componentPropsValues Slots[jsrt.Derived, any]

// valuesOf reads this.#values.
func valuesOf(this any) (any, error) {
	return componentPropsValues.Get(this, "#values")
}

var (
	componentPropsOnce sync.Once
	componentProps     *jsrt.Class
)

// componentPropsClass is ComponentProps, the $props of a component: the props
// given to the component, copied onto the instance, and helpers to read and
// serialize them. Its methods create instances of the class itself.
func componentPropsClass() *jsrt.Class {
	componentPropsOnce.Do(func() { componentProps = defineComponentProps() })
	return componentProps
}

// defineComponentProps defines the ComponentProps class.
func defineComponentProps() *jsrt.Class {
	c := NewClass("ComponentProps", 1, nil, func(this any, args []any) error {
		values := js.Arg(args, 0)
		componentPropsValues.Attach(this.(*jsrt.Derived), values)
		return ObjectAssign(this, values)
	})
	method := func(name string, length int, fn jsrt.NativeFunc) {
		DefineMethod(c.Proto, "ComponentProps."+name, name, length, fn)
	}
	method("all", 0, func(this any, _ []any) (any, error) {
		return valuesOf(this)
	})
	method("has", 1, func(this any, args []any) (any, error) {
		values, err := valuesOf(this)
		if err != nil {
			return nil, err
		}
		return lodash.Has(Ops(), values, js.Arg(args, 0))
	})
	method("get", 2, func(this any, args []any) (any, error) {
		values, err := valuesOf(this)
		if err != nil {
			return nil, err
		}
		return lodash.Get(Ops(), values, js.Arg(args, 0), js.Arg(args, 1))
	})
	method("only", 1, func(this any, args []any) (any, error) {
		values, err := valuesOf(this)
		if err != nil {
			return nil, err
		}
		picked, err := lodash.Pick(Ops(), values, js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return NewComponentProps(picked)
	})
	method("except", 1, func(this any, args []any) (any, error) {
		values, err := valuesOf(this)
		if err != nil {
			return nil, err
		}
		omitted, err := lodash.Omit(Ops(), values, js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return NewComponentProps(omitted)
	})
	method("merge", 1, func(this any, args []any) (any, error) {
		return mergeProps(this, js.Arg(args, 0))
	})
	method("mergeIf", 2, func(this any, args []any) (any, error) {
		if js.ToBoolean(js.Arg(args, 0)) {
			return Ops().Invoke(this, "merge", []any{js.Arg(args, 1)}, "this.merge")
		}
		return this, nil
	})
	method("mergeUnless", 2, func(this any, args []any) (any, error) {
		if !js.ToBoolean(js.Arg(args, 0)) {
			return Ops().Invoke(this, "merge", []any{js.Arg(args, 1)}, "this.merge")
		}
		return this, nil
	})
	method("toAttrs", 0, func(this any, _ []any) (any, error) {
		values, err := valuesOf(this)
		if err != nil {
			return nil, err
		}
		attrs, err := StringifyAttributes(values)
		if err != nil {
			return nil, err
		}
		return HTMLSafe(attrs)
	})
	DefineMethod(c.Ctor, "ComponentProps.create", "create", 1, func(_ any, args []any) (any, error) {
		return NewComponentProps(js.Arg(args, 0))
	})
	return c
}

// NewComponentProps is `new ComponentProps(values)`.
func NewComponentProps(values any) (any, error) {
	return componentPropsClass().Construct([]any{values})
}

// mergeProps is ComponentProps.merge(values): the given values as defaults
// under the props. When both define a truthy class, the result's class is the
// list of unique entries of the defaults' class (the defaults object itself
// when that class is not an array, as Edge wraps `values` rather than
// `values.class`) followed by those of the props' class.
func mergeProps(this, values any) (any, error) {
	defaultClass, err := readClass(values)
	if err != nil {
		return nil, err
	}
	own, err := valuesOf(this)
	if err != nil {
		return nil, err
	}
	var classes *uniqueValues
	if js.ToBoolean(defaultClass) {
		ownClass, err := readClass(own)
		if err != nil {
			return nil, err
		}
		if js.ToBoolean(ownClass) {
			classes = &uniqueValues{}
			if err := classes.addEach(values, values); err != nil {
				return nil, err
			}
			if err := classes.addEach(own, nil); err != nil {
				return nil, err
			}
		}
	}
	merged := js.NewObject()
	if err := Spread(merged, values); err != nil {
		return nil, err
	}
	if err := Spread(merged, own); err != nil {
		return nil, err
	}
	if classes != nil {
		merged.Set("class", &js.Array{Items: classes.items})
	}
	return NewComponentProps(merged)
}

// readClass reads of.class.
func readClass(of any) (any, error) {
	if js.IsNullish(of) {
		return nil, js.CannotReadProperties(of, "class")
	}
	return Realm().Get(of, "class")
}

// uniqueValues collects values once each, compared with SameValueZero, in
// insertion order, as a Set does.
type uniqueValues struct{ items []any }

// addEach adds the entries of of.class when it is an array, else the single
// entry wrap (of.class itself when wrap is nil), with Array.prototype.forEach.
func (u *uniqueValues) addEach(of, wrap any) error {
	list, err := readClass(of)
	if err != nil {
		return err
	}
	isArray, err := Ops().IsArray(list)
	if err != nil {
		return err
	}
	switch {
	case isArray || wrap == nil:
		if list, err = readClass(of); err != nil {
			return err
		}
		if !isArray {
			list = &js.Array{Items: []any{list}}
		}
	default:
		list = &js.Array{Items: []any{wrap}}
	}
	add := jsrt.NewNative("", 1, func(_ any, args []any) (any, error) {
		u.add(js.Arg(args, 0))
		return js.Undefined, nil
	})
	_, err = Ops().Invoke(list, "forEach", []any{add}, "forEach")
	return err
}

func (u *uniqueValues) add(v any) {
	for _, item := range u.items {
		if js.SameValueZero(item, v) {
			return
		}
	}
	u.items = append(u.items, v)
}
