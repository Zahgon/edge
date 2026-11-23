package js

import (
	"strconv"
)

// Object is a plain JavaScript object. Keys enumerate as JavaScript orders them:
// array-index keys ascending, then the other string keys in insertion order.
//
// A property holds a value or, for an accessor property, an *Accessor. Its
// attributes are those of a property created by assignment (writable,
// enumerable, configurable) unless Define or SetAttrs recorded others.
type Object struct {
	keys   []string
	values map[string]any
	// attrs holds the attributes of the properties that have any.
	attrs map[string]Attr
	// nonExtensible is set by PreventExtensions.
	nonExtensible bool
}

// Attr is the set of attributes in which a property differs from one created by
// assignment, which is writable, enumerable and configurable.
type Attr uint8

const (
	// ReadOnly is [[Writable]]: false. It has no meaning for accessors.
	ReadOnly Attr = 1 << iota
	// Hidden is [[Enumerable]]: false.
	Hidden
	// Fixed is [[Configurable]]: false.
	Fixed
)

// Accessor is the value slot of an accessor property: its getter and setter, nil
// when it has none. The runtime calls them; Get returns the *Accessor itself.
type Accessor struct {
	Get, Set *Function
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{values: map[string]any{}}
}

// ObjectOf builds an object from alternating keys and values.
func ObjectOf(kv ...any) *Object {
	o := NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].(string)
		o.Set(k, Normalize(kv[i+1]))
	}
	return o
}

// ArrayIndex reports whether the property key k is an array index, the canonical
// decimal form of an integer from 0 to 2^32-2, and returns that integer. Array-index
// keys are the ones objects list first, in ascending order.
func ArrayIndex(k string) (int, bool) {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(k, 10, 64)
	if err != nil || n > 4294967294 {
		return 0, false
	}
	return int(n), true
}

// IndexKeys returns the array-index keys "0" to n-1: what Object.keys lists for
// an array or a string of length n.
func IndexKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = strconv.Itoa(i)
	}
	return keys
}

// Has reports whether k is an own key.
func (o *Object) Has(k string) bool {
	_, ok := o.values[k]
	return ok
}

// Get returns the value at k (an *Accessor for an accessor property), or
// Undefined.
func (o *Object) Get(k string) any {
	if v, ok := o.values[k]; ok {
		return v
	}
	return Undefined
}

// Set stores v at k, keeping k's position if it already exists.
func (o *Object) Set(k string, v any) {
	if _, ok := o.values[k]; !ok {
		o.insertKey(k)
	}
	o.values[k] = v
}

func (o *Object) insertKey(k string) {
	n, isIndex := ArrayIndex(k)
	if !isIndex {
		o.keys = append(o.keys, k)
		return
	}
	// Indexes sit before every string key, in ascending order.
	pos := 0
	for pos < len(o.keys) {
		m, ok := ArrayIndex(o.keys[pos])
		if !ok || m > n {
			break
		}
		pos++
	}
	o.keys = append(o.keys, "")
	copy(o.keys[pos+1:], o.keys[pos:])
	o.keys[pos] = k
}

// Define stores v at k with the attributes a, keeping k's position if it
// already exists.
func (o *Object) Define(k string, v any, a Attr) {
	o.Set(k, v)
	o.SetAttrs(k, a)
}

// Attrs returns the attributes of the own property k.
func (o *Object) Attrs(k string) Attr {
	return o.attrs[k]
}

// SetAttrs replaces the attributes of the own property k.
func (o *Object) SetAttrs(k string, a Attr) {
	if a == 0 {
		delete(o.attrs, k)
		return
	}
	if o.attrs == nil {
		o.attrs = map[string]Attr{}
	}
	o.attrs[k] = a
}

// Extensible reports whether properties can be added to o.
func (o *Object) Extensible() bool { return !o.nonExtensible }

// PreventExtensions makes o non-extensible, as Object.preventExtensions does.
func (o *Object) PreventExtensions() { o.nonExtensible = true }

// Delete removes k.
func (o *Object) Delete(k string) {
	if _, ok := o.values[k]; !ok {
		return
	}
	delete(o.values, k)
	delete(o.attrs, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns all own keys, enumerable or not, in JavaScript order.
func (o *Object) Keys() []string {
	return append([]string(nil), o.keys...)
}

// EnumerableKeys returns the own enumerable keys in JavaScript order, as
// Object.keys lists them.
func (o *Object) EnumerableKeys() []string {
	if len(o.attrs) == 0 {
		return o.Keys()
	}
	keys := make([]string, 0, len(o.keys))
	for _, k := range o.keys {
		if o.attrs[k]&Hidden == 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

// Len returns the number of keys.
func (o *Object) Len() int { return len(o.keys) }

// Clone returns a shallow copy of the properties, with their attributes; the
// copy is extensible.
func (o *Object) Clone() *Object {
	c := NewObject()
	for _, k := range o.keys {
		c.Define(k, o.values[k], o.attrs[k])
	}
	return c
}
