# From edge.js to Go

This module is Edge 6.5.1 (`edge.js`) rewritten in Go. This file shows where each part of the
TypeScript sources and of their dependencies went, and how JavaScript values are written in Go.

## Edge's modules

| TypeScript (edge.js 6.5.1) | Go |
| :-- | :-- |
| `index.ts` (`edge` default export, `Edge`, `EdgeError`, `edgeGlobals`) | package `edge`: `edge.Default`, `edge.Create`, `edge.EdgeError`, `edge.EdgeGlobals` |
| `src/edge/main.ts` (`Edge`) | `edge.go` (`Edge`, `Options`, `PluginFn`) |
| `src/edge/renderer.ts` (`EdgeRenderer`) | `renderer.go` (`Renderer`) |
| `src/edge/globals.ts` (`edgeGlobals`) | `globals.go`; the functions themselves in `internal/edgert` |
| `src/edge/stacks.ts` (`Stacks`) | `stacks.go` |
| `src/template.ts` (`Template`, `SafeValue`, `escape`, `htmlSafe`) | `template.go` (`Macro`, `Getter`); `SafeValue`, `escape` and `htmlSafe` in `internal/edgert` |
| `src/compiler.ts` (`Compiler`) | `compiler.go` (`Compiler`, `ClaimTagFn`) |
| `src/loader.ts` (`Loader`) | `loader.go` (`Loader`, `LoaderContract`, `LoaderTemplate`) |
| `src/cache_manager.ts` (`CacheManager`) | `cache_manager.go` |
| `src/processor.ts` (`Processor`) | `processor.go` (`Processor`, `ProcessorData`, `ProcessorHandler`) |
| `src/plugins/supercharged.ts` | `supercharged.go` |
| `src/types.ts` | `types.go`, and the declarations of the files above |
| `src/component/props.ts` (`ComponentProps`) | `internal/edgert/props.go` |
| `src/utils.ts` | `stringifyAttributes`: `internal/edgert/attributes.go`; `each`, `asyncEach`: `internal/edgert/macros.go`; `StringifiedObject`, `isSubsetOf`, `isNotSubsetOf`, `unallowedExpression`, `parseJsArg`, `nanoid`: `internal/tags/utils.go` |
| `src/tags/*.ts` | `internal/tags`: `if`, `unless`, `else`, `elseif` in `conditionals.go`; `each`; `component`, `slot`, `inject` in `component.go`; `include`, `includeIf`, `newError` in `include.go`; `let`, `assign`, `eval`, `debugger` in `statements.go`; `stack`, `pushTo`, `pushOnceTo` in `stacks.go`; the registration order in `tags.go` |
| `src/migrate/plugin.ts` (`migrate`) | package `plugins/migrate` (`migrate.Migrate`) |
| `src/migrate/globals.ts` | `plugins/migrate/migrate.go` (`compatGlobals`) |
| `src/migrate/props.ts` (`Props`) | `internal/edgert/compat_props.go` |
| `src/migrate/tags/*.ts` | `internal/tags/migrate.go` (`@layout`, `@section`, `@set`, `@super`); the compiler's merging of a layout's sections with the template's (`Compiler#mergeSections`) is in `compiler.go` |
| `examples/index.ts` | `examples/main.go` |

A plugin is a Go function with the original's arguments, `func(e *edge.Edge, firstRun bool,
options any) error`; a tag is an `*edge.Tag` whose `Compile` receives the parser, the buffer and the
token, as a tag's `compile` method does.

## Dependencies

| npm package | Go package |
| :-- | :-- |
| edge-lexer 6.0.5 | `internal/lexer` (tokens exported from package `edge` as `TagToken`, `MustacheToken`, `RawToken`, `NewLineToken`, `CommentToken`) |
| edge-parser 9.1.1 (`Parser`, `EdgeBuffer`, `Stack`, transforms) | `internal/parser` (exported from package `edge` as `Parser`, `Buffer`, `Stack`) |
| acorn 8.19.0 | `internal/acorn` (ESTree nodes, exported as `edge.Node`) |
| astring 1.9.0 | `internal/astring` |
| edge-error 4.0.2 | `internal/edgeerror` (`edge.EdgeError`); the JavaScript class templates see in `internal/edgert` |
| he 1.2.0 (`escape`) | `internal/htmlent` |
| js-stringify 1.0.2 | `internal/jsstringify` |
| classnames 2.5.1 | `internal/classnames` |
| property-information 7.2.0 | `internal/propinfo` |
| stringify-attributes 4.0.0, escape-goat 4.0.0 | `internal/stringifyattrs`, `internal/escapegoat` |
| @poppinss/inspect 1.0.1 | `internal/prettyhtml` |
| @poppinss/macroable 1.1.2 | `edge.Macro`, `edge.Getter` (`template.go`) |
| @poppinss/string 1.7.2 (case-anything, pluralize, slugify, ms, bytes) | `internal/strutil`, with `internal/strutil/unorm` for `String.prototype.normalize` |
| @poppinss/utils 7.1.2 `lodash` | `internal/lodash` |
| Node's `node:path`, `node:fs` | `internal/nodepath`, `internal/nodefs` |
| V8 (running the compiled template functions) | `internal/jsrt` and its packages `jsnum` (number formatting and Math), `jsstr` (UTF-16 strings), `unicase` (case mapping); `internal/jsregexp` (regular expressions); `internal/jsdate` (Date and time zones); `internal/inspect` (`util.inspect` and `util.format` for `console`) |

`internal/jsval` holds the JavaScript operations the ported libraries apply to template values
through the realm (property reads, conversions, `instanceof`, `for...in`).

## JavaScript values in Go

Values passed to Edge (state, `Share`, `Global`, component props) and values coming back (the
`State` of an output processor, `GetState`, a function's arguments) are JavaScript values written
with package `js`:

| JavaScript | Go |
| :-- | :-- |
| `undefined` | `js.Undefined` (of type `js.UndefinedType`) |
| `null` | `nil` |
| a number | `float64` |
| a string | `string` (UTF-8; a lone surrogate is kept in its three-byte WTF-8 form) |
| a boolean | `bool` |
| a BigInt | `js.BigInt` (`js.BigIntOf(n)`, `js.NewBigInt(x)` from a `*big.Int`) |
| an array | `*js.Array` (`edge.A(items...)`, `Items` holds the elements) |
| an object | `*js.Object` (`edge.O(key, value, ...)`; `Get`, `Set`, `Has`, `Delete`, `Keys` in JavaScript's key order) |
| a function | `*js.Function` (`Name`, `Length`, `Call(this, args)`; `js.Arg(args, i)` reads an argument) |
| a `Date` | `*js.Date` (`Ms`, the time value; NaN for an invalid date) |
| a thrown `Error` | `*js.Error` (`Name`, `Message`, other own properties in `Props`) |
| a thrown `EdgeError` | `*edge.EdgeError` (`Message`, `Code`, `Filename`, `Line`, `Col`) |
| any other thrown value | `*js.Thrown` (`Value`) |

`js.ToString`, `js.ToNumber` and `js.TypeOf` are `String(v)`, `Number(v)` and `typeof v`.

A call that throws in JavaScript returns its exception as the Go error, and a call that returns a
promise (`Render`, `RenderRaw`) returns once the promise has settled: its value, or its rejection
as the error.
