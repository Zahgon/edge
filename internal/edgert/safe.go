package edgert

import (
	"sync"

	"github.com/edge-js/edge-go/internal/htmlent"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/js"
)

// safeValueClass is SafeValue: a value marked as safe, which escaping returns
// as it is.
var safeValueClass = sync.OnceValue(func() *jsrt.Class {
	return NewClass("SafeValue", 1, nil, func(this any, args []any) error {
		// The class field `value;`, then `this.value = value`.
		this.(*jsrt.Derived).Own.Set("value", js.Arg(args, 0))
		return nil
	})
})

// HTMLSafe is htmlSafe(value): new SafeValue(value).
func HTMLSafe(value any) (any, error) {
	return safeValueClass().Construct([]any{value})
}

// Escape is escape(input): the value of a SafeValue, or he.escape(String(input)).
func Escape(input any) (any, error) {
	if safeValueClass().Is(input) {
		return Realm().Get(input, "value")
	}
	s, err := Realm().ToString(input)
	if err != nil {
		return nil, err
	}
	return htmlent.Escape(s), nil
}

// EscapeFunction is the escape function the globals share: html.escape, and
// the migrate plugin's e.
var EscapeFunction = sync.OnceValue(func() *js.Function {
	return NewFunction("globals.html.escape", "escape", 1, func(_ any, args []any) (any, error) {
		return Escape(js.Arg(args, 0))
	})
})

// HTMLSafeFunction is the htmlSafe function the globals share: html.safe, and
// the migrate plugin's safe.
var HTMLSafeFunction = sync.OnceValue(func() *js.Function {
	return NewFunction("globals.html.safe", "htmlSafe", 1, func(_ any, args []any) (any, error) {
		return HTMLSafe(js.Arg(args, 0))
	})
})
