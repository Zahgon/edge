// Package migrate is Edge's migrate plugin (edge.js/plugins/migrate): the
// compat mode for templates written for Edge 5. Templates may extend a
// layout with @layout and override its @section blocks (keeping the
// layout's content with @super), assign variables with @set, and
// components receive the compat Props as $props. The plugin also replaces
// the globals with the compat ones, which add raise, e, safe and stringify.
package migrate

import (
	"sync"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/tags"
	"github.com/edge-js/edge-go/js"
)

// Migrate is the plugin: e.Use(migrate.Migrate, js.Undefined). It turns
// compat mode on for the instance and its compilers, sets the compat globals
// and registers @layout, @section, @set and @super.
func Migrate(e *edge.Edge, _ bool, _ any) error {
	e.Compat = true
	e.Compiler().Compat = true
	e.AsyncCompiler().Compat = true
	for _, g := range compatGlobals() {
		e.Global(g.Name, g.Value)
	}
	for _, tag := range tags.Migrate() {
		if tag.TagName == "set" {
			tag.Boot = edge.BootSetValue
		}
		e.RegisterTag(tag)
	}
	return nil
}

// compatGlobals are the plugin's globals, in their order.
var compatGlobals = sync.OnceValue(func() []edgert.Global {
	globals := []edgert.Global{
		{Name: "nl2br", Value: edgert.NewNl2br("migrate")},
		{Name: "inspect", Value: edgert.NewInspect("migrate")},
		{Name: "truncate", Value: edgert.NewTruncate("migrate")},
		{Name: "raise", Value: raise},
		{Name: "excerpt", Value: edgert.NewExcerpt("migrate")},
		{Name: "e", Value: edgert.EscapeFunction()},
		{Name: "stringify", Value: edgert.JSStringify()},
		{Name: "safe", Value: edgert.HTMLSafeFunction()},
	}
	return append(globals, edgert.StringHelpers()...)
})

// raise is raise(message, options): throws `new Error(message)`, or with
// options an E_RUNTIME_EXCEPTION EdgeError at the options' filename, line and
// col.
var raise = edgert.NewFunction("migrate.raise", "raise", 2, func(_ any, args []any) (any, error) {
	message, options := js.Arg(args, 0), js.Arg(args, 1)
	if js.ToBoolean(options) {
		e, err := edgert.NewEdgeError(message, "E_RUNTIME_EXCEPTION", options)
		if err != nil {
			return nil, err
		}
		return nil, e
	}
	errorCtor, err := edgert.Ops().BuiltinFunction("Error")
	if err != nil {
		return nil, err
	}
	e, err := edgert.Realm().Construct(errorCtor, []any{message})
	if err != nil {
		return nil, err
	}
	return nil, jsrt.ThrowValue(e)
})
