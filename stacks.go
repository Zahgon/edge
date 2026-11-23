package edge

import (
	"math"
	"reflect"
	"sync"

	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// stacks is the state of a Stacks instance, a template's collection of stack
// placeholders and the content pushed to them, filled in before the output
// is returned. Stack names and source ids are JavaScript values compared as
// a Map compares its keys.
type stacks struct {
	contentSources valueMap[*valueMap[struct{}]]
	// seededPlaceholders holds content pushed before its placeholder exists.
	seededPlaceholders valueMap[[]any]
	// placeholders holds the declared placeholders and their content.
	placeholders valueMap[[]any]
}

// stacksState holds the private fields of Stacks instances.
var stacksState edgert.Slots[jsrt.Derived, *stacks]

// stacksClass is Stacks, the class of template.stacks, which the @stack,
// @pushTo and @pushOnceTo tags call.
var stacksClass = sync.OnceValue(func() *jsrt.Class {
	c := edgert.NewClass("Stacks", 0, nil, func(this any, _ []any) error {
		stacksState.Attach(this.(*jsrt.Derived), &stacks{})
		return nil
	})
	method := func(name string, length int, field string, fn func(s *stacks, this any, args []any) (any, error)) {
		edgert.DefineMethod(c.Proto, "Stacks."+name, name, length, func(this any, args []any) (any, error) {
			s, err := stacksState.Get(this, field)
			if err != nil {
				return nil, err
			}
			return fn(s, this, args)
		})
	}
	method("create", 1, "#placeholders", func(s *stacks, _ any, args []any) (any, error) {
		return s.create(js.Arg(args, 0))
	})
	method("pushTo", 2, "#placeholders", func(s *stacks, this any, args []any) (any, error) {
		s.pushTo(js.Arg(args, 0), js.Arg(args, 1))
		return this, nil
	})
	method("pushToTop", 2, "#placeholders", func(s *stacks, this any, args []any) (any, error) {
		s.pushToTop(js.Arg(args, 0), js.Arg(args, 1))
		return this, nil
	})
	method("pushOnceTo", 3, "#contentSources", func(s *stacks, this any, args []any) (any, error) {
		return js.Undefined, s.pushOnce(this, "pushTo", js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
	})
	method("pushOnceToTop", 3, "#contentSources", func(s *stacks, this any, args []any) (any, error) {
		return js.Undefined, s.pushOnce(this, "pushToTop", js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
	})
	method("fillPlaceholders", 1, "#placeholders", func(s *stacks, _ any, args []any) (any, error) {
		return s.fillPlaceholders(js.Arg(args, 0))
	})
	return c
})

// newStacks is `new Stacks()`.
func newStacks() (any, error) {
	return stacksClass().Construct(nil)
}

// placeholderFor returns the text a stack's placeholder renders as.
func placeholderFor(name any) (string, error) {
	s, err := edgert.Realm().ToString(name)
	if err != nil {
		return "", err
	}
	return "<!-- @edge.stacks." + s + " -->", nil
}

// create declares a stack and returns its placeholder. Declaring a stack twice
// is an error.
func (s *stacks) create(name any) (any, error) {
	if s.placeholders.has(name) {
		text, err := edgert.Realm().ToString(name)
		if err != nil {
			return nil, err
		}
		return nil, js.NewError(`Cannot declare stack "` + text + `" for multiple times`)
	}
	seeded, _ := s.seededPlaceholders.get(name)
	s.seededPlaceholders.remove(name)
	s.placeholders.set(name, append([]any{}, seeded...))
	return placeholderFor(name)
}

// pushTo appends content to a stack, seeding it when the stack is not
// declared yet.
func (s *stacks) pushTo(name, contents any) {
	if placeholder, ok := s.placeholders.get(name); ok {
		s.placeholders.set(name, append(placeholder, contents))
		return
	}
	seeded, _ := s.seededPlaceholders.get(name)
	s.seededPlaceholders.set(name, append(seeded, contents))
}

// pushToTop prepends content to a stack, seeding it when the stack is not
// declared yet.
func (s *stacks) pushToTop(name, contents any) {
	if placeholder, ok := s.placeholders.get(name); ok {
		s.placeholders.set(name, append([]any{contents}, placeholder...))
		return
	}
	seeded, _ := s.seededPlaceholders.get(name)
	s.seededPlaceholders.set(name, append([]any{contents}, seeded...))
}

// pushOnce calls this[push](name, contents) unless sourceId has already
// pushed to the stack, and records the source.
func (s *stacks) pushOnce(this any, push string, name, sourceId, contents any) error {
	sources, ok := s.contentSources.get(name)
	if ok && sources.has(sourceId) {
		return nil
	}
	if _, err := edgert.Ops().Invoke(this, push, []any{name, contents}, "this."+push); err != nil {
		return err
	}
	if !ok {
		sources = &valueMap[struct{}]{}
		s.contentSources.set(name, sources)
	}
	sources.set(sourceId, struct{}{})
	return nil
}

// fillPlaceholders replaces each declared placeholder in contents with its
// content joined by line breaks (os.EOL), using contents.replace, which
// expands the $ patterns of the content, and forgets the placeholders.
func (s *stacks) fillPlaceholders(contents any) (any, error) {
	for i, name := range s.placeholders.keys {
		placeholder, err := placeholderFor(name)
		if err != nil {
			return nil, err
		}
		joined, err := edgert.Ops().Invoke(js.NewArray(s.placeholders.values[i]...), "join", []any{"\n"}, "sources.join")
		if err != nil {
			return nil, err
		}
		if contents, err = edgert.Ops().Invoke(contents, "replace", []any{placeholder, joined}, "contents.replace"); err != nil {
			return nil, err
		}
	}
	s.placeholders = valueMap[[]any]{}
	s.seededPlaceholders = valueMap[[]any]{}
	return contents, nil
}

// valueMap is a map keyed by JavaScript values, compared with SameValueZero
// and iterated in insertion order, as a Map is.
type valueMap[V any] struct {
	keys   []any
	values []V
	index  map[any]int
}

// nanKey stands for NaN, which SameValueZero equates with itself.
type nanKey struct{}

// pointerKey stands for an object whose Go representation is not comparable,
// identified by its address.
type pointerKey struct{ p uintptr }

// sameValueZeroKey returns a comparable Go key that two values share exactly
// when SameValueZero holds between them.
func sameValueZeroKey(v any) any {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) {
			return nanKey{}
		}
		if x == 0 {
			return float64(0)
		}
		return x
	case nil:
		return nil
	}
	if !reflect.TypeOf(v).Comparable() {
		return pointerKey{reflect.ValueOf(v).Pointer()}
	}
	return v
}

func (m *valueMap[V]) has(k any) bool {
	_, ok := m.index[sameValueZeroKey(k)]
	return ok
}

func (m *valueMap[V]) get(k any) (V, bool) {
	if i, ok := m.index[sameValueZeroKey(k)]; ok {
		return m.values[i], true
	}
	var zero V
	return zero, false
}

func (m *valueMap[V]) set(k any, v V) {
	key := sameValueZeroKey(k)
	if i, ok := m.index[key]; ok {
		m.values[i] = v
		return
	}
	if m.index == nil {
		m.index = map[any]int{}
	}
	m.index[key] = len(m.keys)
	m.keys = append(m.keys, k)
	m.values = append(m.values, v)
}

func (m *valueMap[V]) remove(k any) {
	key := sameValueZeroKey(k)
	i, ok := m.index[key]
	if !ok {
		return
	}
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	m.values = append(m.values[:i], m.values[i+1:]...)
	delete(m.index, key)
	for j := i; j < len(m.keys); j++ {
		m.index[sameValueZeroKey(m.keys[j])] = j
	}
}
