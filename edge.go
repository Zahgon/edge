// Package edge is Edge 6.5.1, the template engine of AdonisJS, in Go.
//
// Templates are compiled exactly as Edge compiles them: edge-lexer turns the
// source into tokens, edge-parser parses every mustache and tag argument with
// acorn, rewrites free identifiers to read the template state and prints the
// result back with astring into a buffer of generated statements, and the
// buffer becomes a JavaScript function, run by the module's interpreter with
// V8's semantics against the objects Edge hands to templates (template,
// state, $context, the globals, $props, $slots).
//
// Values crossing the API are JavaScript values in the representation of
// package js: js.Undefined, nil for null, float64, string, bool, *js.Object,
// *js.Array, *js.Date and *js.Function. A call that throws in JavaScript
// returns the thrown value as the error: an *EdgeError, a *js.Error or a
// *js.Thrown.
//
// All templates run in one JavaScript realm, as all of Edge runs in one Node
// process: a global one template creates is seen by the others. The package
// is not safe for concurrent use.
package edge

import (
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/tags"
	"github.com/edge-js/edge-go/js"
)

// Version is the version of Edge this package ports.
const Version = "6.5.1"

// Options are the options of Create and Configure.
type Options struct {
	// Cache caches compiled templates by path.
	Cache bool
	// Loader loads templates; nil keeps the current loader, or creates the
	// default one.
	Loader LoaderContract
}

// PluginFn is a plugin: it is called with the Edge instance, whether this is
// its first run and the options it was registered with. A plugin whose
// options are an object with a truthy `recurring` runs before every render;
// the others run once, before the first.
type PluginFn func(e *Edge, firstRun bool, options any) error

// plugin is a registered plugin and whether it has run.
type plugin struct {
	fn       PluginFn
	executed bool
	options  any
}

// Edge is an instance of Edge: the tags, globals, loader, compilers and
// processor that render templates.
type Edge struct {
	// Compat is set by the migrate plugin.
	Compat bool

	bundledPlugins  []*plugin
	plugins         []*plugin
	renderCallbacks []func(r *Renderer) error
	processor       *Processor
	globals         *js.Object
	tags            map[string]*Tag
	loader          LoaderContract
	compiler        *Compiler
	asyncCompiler   *Compiler
}

// Default is the instance the original module exports as its default: an Edge
// created with no options when the package loads.
var Default = Create(Options{})

// Create returns a new Edge instance: Edge.create(options). The built-in tags
// are registered, and the supercharged components plugin is added, recurring
// unless templates are cached.
func Create(opts Options) *Edge {
	e := &Edge{
		processor: newProcessor(),
		globals:   EdgeGlobals.Clone(),
		tags:      map[string]*Tag{},
	}
	e.Configure(opts)
	for _, tag := range builtinTags() {
		e.RegisterTag(tag)
	}
	e.bundledPlugins = append(e.bundledPlugins, &plugin{
		fn:      pluginSuperCharged,
		options: js.ObjectOf("recurring", !opts.Cache),
	})
	return e
}

// builtinTags returns Edge's tags with their boot functions: @assign and
// @let add template.setValue, @each adds template.loop, template.loopAsync
// and template.size.
func builtinTags() []*Tag {
	list := tags.Builtin()
	for _, tag := range list {
		switch tag.TagName {
		case "assign", "let":
			tag.Boot = BootSetValue
		case "each":
			tag.Boot = bootLoop
		}
	}
	return list
}

// BootSetValue is the boot function of the tags that assign to nested paths
// (@assign, @let and the migrate plugin's @set): it adds lodash's set to the
// Template class as setValue.
func BootSetValue() error {
	return macro("setValue", edgert.LodashSet())
}

// bootLoop is the boot function of @each.
func bootLoop() error {
	for _, m := range []struct {
		name  string
		value *js.Function
	}{{"loopAsync", edgert.AsyncEach()}, {"loop", edgert.Each()}, {"size", edgert.LodashSize()}} {
		if err := macro(m.name, m.value); err != nil {
			return err
		}
	}
	return nil
}

// Configure applies options: the loader, when given, and new compilers, which
// cache templates when Cache is set.
func (e *Edge) Configure(opts Options) {
	if opts.Loader != nil {
		e.loader = opts.Loader
	} else if e.loader == nil {
		e.loader = NewLoader()
	}
	e.compiler = newCompiler(e.loader, e.tags, e.processor, opts.Cache, false)
	e.asyncCompiler = newCompiler(e.loader, e.tags, e.processor, opts.Cache, true)
}

// executePlugins runs the plugins that are due: the registered plugins, then
// the bundled ones, each list filtered before any of it runs.
func (e *Edge) executePlugins() error {
	for _, list := range [][]*plugin{e.plugins, e.bundledPlugins} {
		var due []*plugin
		for _, p := range list {
			recurring, err := isRecurring(p.options)
			if err != nil {
				return err
			}
			if recurring || !p.executed {
				due = append(due, p)
			}
		}
		for _, p := range due {
			if err := p.fn(e, !p.executed, p.options); err != nil {
				return err
			}
			p.executed = true
		}
	}
	return nil
}

// isRecurring is `options && options.recurring`.
func isRecurring(options any) (bool, error) {
	if !js.ToBoolean(options) {
		return false, nil
	}
	recurring, err := edgert.Realm().Get(options, "recurring")
	return js.ToBoolean(recurring), err
}

// Use registers a plugin, run with options before the first render (and
// before every render when it is recurring).
func (e *Edge) Use(pluginFn PluginFn, options any) *Edge {
	e.plugins = append(e.plugins, &plugin{fn: pluginFn, options: options})
	return e
}

// Mount mounts the directory of the default disk, whose templates are
// referenced without a disk prefix.
func (e *Edge) Mount(viewsDirectory string) *Edge {
	e.loader.Mount("default", viewsDirectory)
	return e
}

// MountDisk mounts a directory under a disk name, whose templates are
// referenced as "disk::path". An empty directory mounts diskName itself as
// the default disk, as edge.mount(dir) does.
func (e *Edge) MountDisk(diskName, viewsDirectory string) *Edge {
	if viewsDirectory == "" {
		viewsDirectory = diskName
		diskName = "default"
	}
	e.loader.Mount(diskName, viewsDirectory)
	return e
}

// Unmount removes a mounted disk.
func (e *Edge) Unmount(diskName string) *Edge {
	e.loader.Unmount(diskName)
	return e
}

// Global sets a global, visible to every template rendered afterwards.
func (e *Edge) Global(name string, value any) *Edge {
	if err := edgert.Realm().Set(e.globals, name, value, true); err != nil {
		// The globals are a plain object, which accepts every assignment.
		panic(err)
	}
	return e
}

// Globals returns the globals object, edge.globals.
func (e *Edge) Globals() *js.Object { return e.globals }

// RegisterTag registers a tag, after calling its Boot function. A Boot that
// fails panics with its error, as registerTag throws it.
func (e *Edge) RegisterTag(tag *Tag) *Edge {
	if tag.Boot != nil {
		if err := tag.Boot(); err != nil {
			panic(err)
		}
	}
	e.tags[tag.TagName] = tag
	return e
}

// RegisterTemplate registers an in-memory template with the loader. Its
// Template must be a string, and a path cannot be registered twice.
func (e *Edge) RegisterTemplate(templatePath string, contents LoaderTemplate) (*Edge, error) {
	if err := e.loader.Register(templatePath, contents); err != nil {
		return nil, err
	}
	return e, nil
}

// RemoveTemplate removes an in-memory template and its cached compiled
// functions.
func (e *Edge) RemoveTemplate(templatePath string) *Edge {
	e.loader.Remove(templatePath)
	e.compiler.cacheManager.Delete(templatePath)
	e.asyncCompiler.cacheManager.Delete(templatePath)
	return e
}

// OnRender registers a callback run with every renderer CreateRenderer
// returns.
func (e *Edge) OnRender(callback func(r *Renderer) error) *Edge {
	e.renderCallbacks = append(e.renderCallbacks, callback)
	return e
}

// CreateRenderer runs the plugins that are due and returns a new renderer,
// after the OnRender callbacks have seen it.
func (e *Edge) CreateRenderer() (*Renderer, error) {
	if err := e.executePlugins(); err != nil {
		return nil, err
	}
	r := newRenderer(e.compiler, e.asyncCompiler, e.processor, e.globals)
	for _, callback := range e.renderCallbacks {
		if err := callback(r); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Render renders a template with the async compiler and returns what the
// promise settles with.
func (e *Edge) Render(templatePath string, state any) (string, error) {
	r, err := e.CreateRenderer()
	if err != nil {
		return "", err
	}
	return r.Render(templatePath, state)
}

// RenderSync renders a template synchronously.
func (e *Edge) RenderSync(templatePath string, state any) (string, error) {
	r, err := e.CreateRenderer()
	if err != nil {
		return "", err
	}
	return r.RenderSync(templatePath, state)
}

// RenderRaw renders template source with the async compiler. templatePath is
// js.Undefined for the default path "eval.edge", or a string.
func (e *Edge) RenderRaw(contents string, state any, templatePath any) (string, error) {
	r, err := e.CreateRenderer()
	if err != nil {
		return "", err
	}
	return r.RenderRaw(contents, state, templatePath)
}

// RenderRawSync renders template source synchronously, as "eval.edge".
func (e *Edge) RenderRawSync(contents string, state any) (string, error) {
	r, err := e.CreateRenderer()
	if err != nil {
		return "", err
	}
	return r.RenderRawSync(contents, state, js.Undefined)
}

// Share returns a new renderer holding data as its locals.
func (e *Edge) Share(data any) (*Renderer, error) {
	r, err := e.CreateRenderer()
	if err != nil {
		return nil, err
	}
	return r.Share(data), nil
}

// Processor returns the processor of the instance.
func (e *Edge) Processor() *Processor { return e.processor }

// Loader returns the loader of the instance.
func (e *Edge) Loader() LoaderContract { return e.loader }

// Compiler returns the synchronous compiler.
func (e *Edge) Compiler() *Compiler { return e.compiler }

// AsyncCompiler returns the compiler of async templates.
func (e *Edge) AsyncCompiler() *Compiler { return e.asyncCompiler }
