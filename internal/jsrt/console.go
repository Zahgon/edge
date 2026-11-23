package jsrt

import (
	"io"
	"os"

	"github.com/edge-js/edge-go/internal/inspect"
	"github.com/edge-js/edge-go/internal/jsrt/jsnum"
	"github.com/edge-js/edge-go/js"
)

// The console global: Node's console methods format their arguments with
// util.format (package inspect) and write one line to the realm's stdout or
// stderr.

// setupConsole installs console with log, info and debug writing to stdout and
// error and warn writing to stderr.
func (r *Realm) setupConsole() {
	r.stdout, r.stderr = os.Stdout, os.Stderr
	c := js.NewObject()
	r.define(r.global, "console", c)
	for _, m := range []struct {
		name   string
		stderr bool
	}{{"log", false}, {"warn", true}, {"error", true}, {"info", false}, {"debug", false}} {
		r.method(c, m.name, 0, func(_ any, args []any) (any, error) {
			w := r.stdout
			if m.stderr {
				w = r.stderr
			}
			return js.Undefined, r.writeConsole(w, args)
		})
	}
}

// writeConsole writes util.format(...args) and a line break to w.
func (r *Realm) writeConsole(w io.Writer, args []any) error {
	s, err := inspect.Format(inspectHost{r}, args)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, s+"\n")
	return err
}

// inspectHost gives package inspect the realm's view of its objects.
type inspectHost struct{ r *Realm }

// Prototype is [[GetPrototypeOf]].
func (h inspectHost) Prototype(v any) any { return h.r.protoOf(v) }

// OwnKeys is [[OwnPropertyKeys]] for string keys, the enumerable ones unless
// hidden is set.
func (h inspectHost) OwnKeys(v any, hidden bool) []string { return h.r.ownPropertyKeys(v, hidden) }

// OwnProperty is [[GetOwnProperty]] as inspect reads it, without running a
// getter.
func (h inspectHost) OwnProperty(v any, key string) (inspect.Property, bool) {
	slot, attrs, ok := h.r.ownProperty(v, key)
	if !ok {
		return inspect.Property{}, false
	}
	p := inspect.Property{Enumerable: attrs&js.Hidden == 0}
	if acc, isAcc := slot.(*js.Accessor); isAcc {
		p.Getter, p.Setter = acc.Get != nil, acc.Set != nil
	} else {
		p.Value = slot
	}
	return p, true
}

// Describe reports the kind and the internal slots of v, as Node's type
// checks (isMap, isPromise, ...) and its internal bindings see them.
func (h inspectHost) Describe(v any) inspect.Object {
	r := h.r
	switch x := v.(type) {
	case *js.Array:
		return inspect.Object{Kind: inspect.Array}
	case *js.Function:
		switch async, generator := r.isAsyncFunction(x), isGeneratorFunction(x); {
		case async && generator:
			return inspect.Object{Kind: inspect.Function, Async: true, Generator: true, Tag: "AsyncGeneratorFunction"}
		case async:
			return inspect.Object{Kind: inspect.Function, Async: true, Tag: "AsyncFunction"}
		case generator:
			return inspect.Object{Kind: inspect.Function, Generator: true, Tag: "GeneratorFunction"}
		}
		return inspect.Object{Kind: inspect.Function}
	case *js.Error:
		return inspect.Object{Kind: inspect.Error}
	case *js.Date:
		return inspect.Object{Kind: inspect.Date}
	case *js.RegExp:
		return inspect.Object{Kind: inspect.RegExp}
	case *arguments:
		return inspect.Object{Kind: inspect.Arguments}
	case *primitiveObject:
		return inspect.Object{Kind: inspect.Boxed, Primitive: x.value}
	case *mapObject:
		if x.weak {
			return inspect.Object{Kind: inspect.WeakMap, Tag: "WeakMap"}
		}
		o := inspect.Object{Kind: inspect.Map, Tag: "Map"}
		for i, more := x.entries.next(0); more; i, more = x.entries.next(i + 1) {
			o.Values = append(o.Values, x.entries.keys[i], x.entries.values[i])
		}
		return o
	case *setObject:
		if x.weak {
			return inspect.Object{Kind: inspect.WeakSet, Tag: "WeakSet"}
		}
		return inspect.Object{Kind: inspect.Set, Tag: "Set", Values: x.entries.liveKeys()}
	case *collectionIterator:
		return describeIterator(x)
	case *iterator:
		return inspect.Object{Tag: x.tag}
	}
	if rec, ok := r.promiseOf(v); ok {
		o := inspect.Object{Kind: inspect.Promise, Tag: "Promise", Primitive: rec.result}
		switch rec.state {
		case promiseFulfilled:
			o.State = inspect.Fulfilled
		case promiseRejected:
			o.State = inspect.Rejected
		}
		return o
	}
	switch tag := r.classOf(v); tag {
	case "Math", "JSON", "global", "Generator", "AsyncGenerator", "GeneratorFunction", "AsyncFunction", "AsyncGeneratorFunction":
		return inspect.Object{Tag: tag}
	}
	return inspect.Object{}
}

// describeIterator previews the entries a Map or Set iterator has yet to
// produce, without advancing it.
func describeIterator(it *collectionIterator) inspect.Object {
	o := inspect.Object{Kind: inspect.MapIterator, Tag: "Map Iterator"}
	if it.set != nil {
		o.Kind, o.Tag = inspect.SetIterator, "Set Iterator"
	}
	if it.done {
		return o
	}
	e := it.entries()
	o.KeyValue = it.kind == iterEntries
	for i, more := e.next(it.pos); more; i, more = e.next(i + 1) {
		k, v := e.keys[i], e.values[i]
		if it.set != nil {
			v = k
		}
		switch it.kind {
		case iterKeys:
			o.Values = append(o.Values, k)
		case iterValues:
			o.Values = append(o.Values, v)
		default:
			o.Values = append(o.Values, k, v)
		}
	}
	return o
}

// ToString is String(v).
func (h inspectHost) ToString(v any) (string, error) { return h.r.toString(v) }

// ToNumber is Number(v).
func (h inspectHost) ToNumber(v any) (float64, error) { return h.r.numberOf(v) }

// ParseInt is parseInt(v).
func (h inspectHost) ParseInt(v any) (float64, error) {
	s, err := h.r.toString(v)
	if err != nil {
		return 0, err
	}
	return jsnum.ParseInt(s, 0), nil
}

// ParseFloat is parseFloat(v).
func (h inspectHost) ParseFloat(v any) (float64, error) {
	s, err := h.r.toString(v)
	if err != nil {
		return 0, err
	}
	return jsnum.ParseFloat(s), nil
}

// JSONStringify is JSON.stringify(v).
func (h inspectHost) JSONStringify(v any) (string, bool, error) {
	return h.r.JSONStringify(v, js.Undefined, js.Undefined)
}
