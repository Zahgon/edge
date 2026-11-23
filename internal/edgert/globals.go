package edgert

import (
	"sync"

	"github.com/edge-js/edge-go/internal/classnames"
	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsstringify"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/prettyhtml"
	"github.com/edge-js/edge-go/internal/strutil"
	"github.com/edge-js/edge-go/js"
)

// The functions of Edge's globals (src/edge/globals.ts) and of the migrate
// plugin's globals (src/migrate/globals.ts). Both modules define their own
// nl2br, inspect, truncate and excerpt arrow functions, built here once per
// module (keyed "globals.<name>" and "migrate.<name>"); the escaping
// functions, js-stringify and the @poppinss/string helpers are the same
// function objects in both.

// Global is one global: its name and value.
type Global struct {
	Name  string
	Value any
}

// libraryFunction returns a function of a library as Edge's globals expose
// it, with the library's name and length, running fn with the realm's
// operations.
func libraryFunction(key, name string, length int, fn func(ops *jsval.Ops, args []any) (any, error)) *js.Function {
	return NewFunction(key, name, length, func(_ any, args []any) (any, error) {
		return fn(Ops(), args)
	})
}

// StringHelpers are the @poppinss/string helpers both modules expose as
// globals, in their order: camelCase ... ordinal.
var StringHelpers = sync.OnceValue(func() []Global {
	helpers := []struct {
		global, name string
		length       int
		fn           func(ops *jsval.Ops, args []any) (any, error)
	}{
		{"camelCase", "camelCase", 1, strutil.JSCamelCase},
		{"snakeCase", "snakeCase", 1, strutil.JSSnakeCase},
		{"dashCase", "dashCase", 2, strutil.JSDashCase},
		{"pascalCase", "pascalCase", 1, strutil.JSPascalCase},
		{"capitalCase", "capitalCase", 1, strutil.JSCapitalCase},
		{"sentenceCase", "sentenceCase", 1, strutil.JSSentenceCase},
		{"dotCase", "dotCase", 2, strutil.JSDotCase},
		{"noCase", "noCase", 2, strutil.JSNoCase},
		{"titleCase", "titleCase", 1, strutil.JSTitleCase},
		{"pluralize", "pluralize", 3, strutil.JSPluralize},
		{"sentence", "sentence", 2, strutil.JSSentence},
		{"prettyMs", "format", 2, strutil.JSFormatMilliseconds},
		{"toMs", "parse", 1, strutil.JSParseMilliseconds},
		{"prettyBytes", "format", 2, strutil.JSFormatBytes},
		{"toBytes", "parse", 1, strutil.JSParseBytes},
		{"ordinal", "ordinal", 1, strutil.JSOrdinal},
	}
	out := make([]Global, len(helpers))
	for i, h := range helpers {
		out[i] = Global{h.global, libraryFunction("globals."+h.global, h.name, h.length, h.fn)}
	}
	return out
})

// JSStringify is js-stringify's stringify: js.stringify, and the migrate
// plugin's stringify.
var JSStringify = sync.OnceValue(func() *js.Function {
	return NewFunction("globals.js.stringify", "stringify", 1, func(_ any, args []any) (any, error) {
		return jsstringify.Stringify(Ops(), js.Arg(args, 0))
	})
})

// ClassNames is the bundled classnames function: html.classNames.
var ClassNames = sync.OnceValue(func() *js.Function {
	return NewFunction("globals.html.classNames", "classNames", 0, func(_ any, args []any) (any, error) {
		return classnames.ClassNames(Ops(), args)
	})
})

// Attrs is html.attrs(values): the values as HTML attributes, marked safe.
var Attrs = sync.OnceValue(func() *js.Function {
	return NewFunction("globals.html.attrs", "attrs", 1, func(_ any, args []any) (any, error) {
		attrs, err := StringifyAttributes(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return HTMLSafe(attrs)
	})
})

// lineBreaks is nl2br's pattern: a line break and the character before it,
// unless that is the end of a tag.
var lineBreaks = func() *jsregexp.Regexp {
	re, err := jsregexp.CompileLiteral(`([^>\r\n]?)(\r\n|\n\r|\r|\n)`, "g")
	if err != nil {
		panic(err)
	}
	return re
}()

// NewNl2br returns an nl2br function, recorded under module ("globals" or
// "migrate"): undefined for a falsy value, else String(value) with a <br>
// before every line break that does not follow a tag's ">".
func NewNl2br(module string) *js.Function {
	return NewFunction(module+".nl2br", "nl2br", 1, func(_ any, args []any) (any, error) {
		value := js.Arg(args, 0)
		if !js.ToBoolean(value) {
			return js.Undefined, nil
		}
		s, err := Realm().ToString(value)
		if err != nil {
			return nil, err
		}
		return lineBreaks.ReplaceString(s, "$1<br>"), nil
	})
}

// NewInspect returns an inspect function, recorded under module: the HTML
// dump of @poppinss/inspect, marked safe.
func NewInspect(module string) *js.Function {
	return NewFunction(module+".inspect", "inspect", 1, func(_ any, args []any) (any, error) {
		html, err := prettyhtml.HTML(Ops(), js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return HTMLSafe(html)
	})
}

// NewTruncate returns a truncate function, recorded under module.
func NewTruncate(module string) *js.Function {
	return libraryFunction(module+".truncate", "truncate", 1, strutil.JSTruncate)
}

// NewExcerpt returns an excerpt function, recorded under module.
func NewExcerpt(module string) *js.Function {
	return libraryFunction(module+".excerpt", "excerpt", 1, strutil.JSExcerpt)
}
