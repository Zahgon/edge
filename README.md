# Edge

[![go][gh-workflow-image]][gh-workflow-url] [![license-image]][license-url]

Edge is a simple, modern and batteries included template engine. Edge is similar to writing
JavaScript: if you know JavaScript, you know Edge.

This module is Edge 6.5.1 for Go. Templates are compiled exactly as Edge compiles them (the same
lexer, the same parser, the same generated code) and run by a JavaScript interpreter with V8's
semantics, so a template renders to the same bytes, and fails with the same errors, as it does with
`edge.js` on Node.js. Learn the template syntax at [edgejs.dev](https://edgejs.dev).

```sh
go get github.com/edge-js/edge-go
```

## Usage

```go
package main

import (
	"fmt"
	"path/filepath"

	edge "github.com/edge-js/edge-go"
)

func main() {
	views, err := filepath.Abs("views")
	if err != nil {
		panic(err)
	}
	e := edge.Create(edge.Options{Cache: true})
	e.Mount(views)
	e.Global("appName", "Acme")

	html, err := e.Render("welcome", edge.O(
		"user", edge.O("username", "virk"),
		"tasks", edge.A("Write docs", "Ship"),
	))
	if err != nil {
		panic(err)
	}
	fmt.Println(html)
}
```

`views/welcome.edge`:

```edge
<h1>Hello {{ user.username }}, welcome to {{ appName }}</h1>

@each(task in tasks)
  @!component('components/task', { title: task })
@end
```

### JavaScript values

Everything a template sees is a JavaScript value, written in Go with package
`github.com/edge-js/edge-go/js`: `js.Undefined`, `nil` for `null`, `float64`, `string`, `bool`,
objects built with `edge.O(key, value, ...)` (keys keep their order), arrays built with
`edge.A(...)`, `*js.Date` and functions:

```go
e.Global("shout", &js.Function{Name: "shout", Length: 1, Call: func(this any, args []any) (any, error) {
	return strings.ToUpper(js.ToString(js.Arg(args, 0))) + "!", nil
}})
```

### The API

Edge's JavaScript API maps mechanically to Go: `edge.x(...)` is `e.X(...)`, names are Go-cased, and
a call that throws returns a non-nil error.

| Edge (JavaScript) | Go |
| :-- | :-- |
| `import edge from 'edge.js'` | `edge.Default` |
| `Edge.create(options)`, `edge.configure(options)` | `edge.Create(edge.Options{...})`, `e.Configure(...)` |
| `await edge.render(path, state)` | `e.Render(path, state)` |
| `edge.renderSync(path, state)` | `e.RenderSync(path, state)` |
| `await edge.renderRaw(source, state)` | `e.RenderRaw(source, state, js.Undefined)` |
| `edge.renderRawSync(source, state)` | `e.RenderRawSync(source, state)` |
| `edge.share(locals).render(...)` | `r, err := e.Share(locals)`, then `r.Render(...)` |
| `edge.createRenderer()` | `e.CreateRenderer()` |
| `edge.mount(dir)`, `edge.mount(disk, dir)`, `edge.unmount(disk)` | `e.Mount(dir)`, `e.MountDisk(disk, dir)`, `e.Unmount(disk)` |
| `edge.registerTemplate(path, { template })`, `edge.removeTemplate(path)` | `e.RegisterTemplate(path, edge.LoaderTemplate{Template: ...})`, `e.RemoveTemplate(path)` |
| `edge.global(name, value)`, `edge.globals` | `e.Global(name, value)`, `e.Globals()` |
| `edge.registerTag(tag)`, `edge.use(plugin, options)`, `edge.onRender(fn)` | `e.RegisterTag(tag)`, `e.Use(plugin, options)`, `e.OnRender(fn)` |
| `edge.processor.process(event, handler)` | `e.Processor().Process(event, handler)` |
| `edge.compiler`, `edge.asyncCompiler`, `edge.loader` | `e.Compiler()`, `e.AsyncCompiler()`, `e.Loader()` |
| `compiler.compile(path)`, `compiler.compileRaw(source)`, `compiler.tokenizeRaw(source)` | `c.Compile(path, nil)`, `c.CompileRaw(source, js.Undefined)`, `c.TokenizeRaw(source, js.Undefined)` |
| `Template.macro(name, fn)`, `Template.getter(name, fn, singleton)` | `edge.Macro(name, fn)`, `edge.Getter(name, fn, singleton)` |

`Render` and `RenderRaw` use the async compiler, where templates may `await`; the Go call returns
once the promise has settled. [MIGRATION.md](MIGRATION.md) maps every TypeScript module and
dependency of Edge to its Go package.

### Errors

A call that throws in JavaScript returns the thrown value as the error. Problems Edge can point at
in a template are `*edge.EdgeError` values, with the template's filename and line:

```go
_, err := e.RenderRawSync("Hello\n{{ user.name }}", nil)
var ee *edge.EdgeError
if errors.As(err, &ee) {
	fmt.Println(ee.Code, ee.Filename, ee.Line, ee.Message)
	// E_RUNTIME_EXCEPTION eval.edge 2 Cannot read properties of undefined (reading 'name')
}
```

Other JavaScript errors are `*js.Error` values (name and message as V8 reports them), and any
other thrown value is a `*js.Thrown`.

### Disks, in-memory templates and components

```go
e.Mount("/srv/app/views")                 // the default disk: e.Render("welcome", ...)
e.MountDisk("admin", "/srv/admin/views") // a named disk: e.Render("admin::dashboard", ...)
e.RegisterTemplate("components/button", edge.LoaderTemplate{Template: "<button>{{{ $slots.main() }}}</button>"})
```

As in Edge, a template path is joined to the mounted directory as it is given, so mount directories
by their absolute path. Components (templates in the `components` directory of a disk, and in-memory templates) can be
used as tags: `components/form/input.edge` is `@form.input(...)`, `admin::components/panel.edge`
is `@admin.panel(...)`.

### Tags, globals, processors and plugins

```go
e.RegisterTag(&edge.Tag{
	TagName: "upper",
	Block:   true,
	Compile: func(p *edge.Parser, b *edge.Buffer, token *edge.TagToken) error {
		b.OutputRaw("<strong>")
		for _, child := range token.Children {
			if err := p.ProcessToken(child, b); err != nil {
				return err
			}
		}
		b.OutputRaw("</strong>")
		return nil
	},
})

e.Processor().Process("output", func(d edge.ProcessorData) (string, bool, error) {
	return strings.TrimSpace(d.Output), true, nil
})

e.Use(func(e *edge.Edge, firstRun bool, options any) error {
	if firstRun {
		e.Global("version", "1.0.0")
	}
	return nil
}, js.Undefined)
```

`edge.Macro` and `edge.Getter` add properties to the `template` object of every template, as
`Template.macro` and `Template.getter` do.

### Migrating from Edge 5

The migrate plugin brings back layouts, sections, `@super`, `@set` and the compat props:

```go
import "github.com/edge-js/edge-go/plugins/migrate"

e.Use(migrate.Migrate, js.Undefined)
```

### Example

`go run ./examples` serves the page of `examples/views` on http://127.0.0.1:3000;
`go run ./examples -once` prints it.

## How it reproduces Edge

Edge turns a template into a JavaScript function and V8 runs it. This module follows the same
chain, step for step:

1. **Lexer** (`internal/lexer`, edge-lexer): the template becomes tags, mustaches, comments, raw
   text and newlines, with the same positions and the same errors (`E_UNCLOSED_TAG`, ...).
2. **acorn** (`internal/acorn`): every mustache and tag argument is parsed as JavaScript into an
   ESTree, with acorn's syntax error messages.
3. **edge-parser's transforms** (`internal/parser`): free identifiers become `state.x` unless
   they are local variables or JavaScript globals, and expressions Edge does not allow raise
   `E_UNALLOWED_EXPRESSION`.
4. **astring** (`internal/astring`): the transformed tree is written back as source text, and the
   buffer of generated statements becomes the template function's body, the same text Edge
   produces (visible through the `compiled` processor and `Function.prototype.toString`).
5. **The JavaScript interpreter** (`internal/jsrt`): the body is compiled with the `Function` (or
   `AsyncFunction`) constructor and run against the `template`, `state` and `$context` objects,
   with V8's semantics and error messages for the language and its builtins, Dates in the time
   zone of `TZ`, and Node's `console` formatting.

The output then goes through the same libraries Edge uses, ported to Go: he for escaping,
property-information and classnames for `html.attrs` and `$props.toAttrs()`, @poppinss/inspect for
`inspect()`, @poppinss/string for the string helpers, and Edge's stacks and processors.

## Contributing

`go vet ./...` and `go test ./...` must pass. All templates of a process run in one JavaScript
realm, as they do in one Node.js process; the package is not safe for concurrent use.

[gh-workflow-image]: https://img.shields.io/github/actions/workflow/status/edge-js/edge/go.yml?style=for-the-badge
[gh-workflow-url]: https://github.com/edge-js/edge/actions/workflows/go.yml "Github action"

[license-image]: https://img.shields.io/badge/license-MIT-blueviolet?style=for-the-badge
[license-url]: LICENSE.md "license"
