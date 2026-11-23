package edge

import (
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/lodash"
	"github.com/edge-js/edge-go/js"
)

// Renderer renders templates with the globals of its Edge instance and the
// locals shared with it: EdgeRenderer.
type Renderer struct {
	compiler      *Compiler
	asyncCompiler *Compiler
	processor     *Processor
	locals        *js.Object
	globals       *js.Object
}

func newRenderer(compiler, asyncCompiler *Compiler, processor *Processor, globals *js.Object) *Renderer {
	return &Renderer{
		compiler:      compiler,
		asyncCompiler: asyncCompiler,
		processor:     processor,
		locals:        js.NewObject(),
		globals:       globals,
	}
}

// Clone returns a renderer with a deep copy of this renderer's locals.
func (r *Renderer) Clone() *Renderer {
	return newRenderer(r.compiler, r.asyncCompiler, r.processor, r.globals).Share(r.locals)
}

// Share merges data into the locals, deeply, as lodash's merge does. A getter
// of data that throws makes Share panic with the error, as share throws it.
func (r *Renderer) Share(data any) *Renderer {
	if _, err := lodash.Merge(edgert.Ops(), r.locals, data); err != nil {
		panic(err)
	}
	return r
}

// GetState returns { ...globals, ...locals }.
func (r *Renderer) GetState() *js.Object {
	state := js.NewObject()
	for _, source := range []*js.Object{r.globals, r.locals} {
		if err := edgert.Spread(state, source); err != nil {
			// The globals and the locals are plain objects of data
			// properties, which spread without running code.
			panic(err)
		}
	}
	return state
}

// Render renders a template with the async compiler and returns what the
// promise settles with.
func (r *Renderer) Render(templatePath string, state any) (string, error) {
	return awaitOutput(func() (any, error) {
		t, err := r.template(r.asyncCompiler)
		if err != nil {
			return nil, err
		}
		return t.render(templatePath, defaultState(state))
	})
}

// RenderSync renders a template synchronously.
func (r *Renderer) RenderSync(templatePath string, state any) (string, error) {
	t, err := r.template(r.compiler)
	if err != nil {
		return renderOutput(nil, err)
	}
	return renderOutput(t.render(templatePath, defaultState(state)))
}

// RenderRaw renders template source with the async compiler. templatePath is
// js.Undefined for the default "eval.edge", or a string.
func (r *Renderer) RenderRaw(contents string, state any, templatePath any) (string, error) {
	return awaitOutput(func() (any, error) {
		t, err := r.template(r.asyncCompiler)
		if err != nil {
			return nil, err
		}
		return t.renderRaw(contents, defaultState(state), templatePath)
	})
}

// RenderRawSync renders template source synchronously. templatePath is
// js.Undefined for the default "eval.edge", or a string.
func (r *Renderer) RenderRawSync(contents string, state any, templatePath any) (string, error) {
	t, err := r.template(r.compiler)
	if err != nil {
		return renderOutput(nil, err)
	}
	return renderOutput(t.renderRaw(contents, defaultState(state), templatePath))
}

// template returns a new Template rendering with compiler.
func (r *Renderer) template(compiler *Compiler) (*template, error) {
	return newTemplate(compiler, r.globals, r.locals, r.processor)
}

// defaultState is the `state = {}` default of the render methods.
func defaultState(state any) any {
	if _, undefined := state.(js.UndefinedType); undefined {
		return js.NewObject()
	}
	return state
}

// awaitOutput runs an async render, as the original's async render methods
// do: an error thrown before the template function runs rejects the promise
// too. It returns what the promise settles with.
func awaitOutput(render func() (any, error)) (string, error) {
	promise, err := render()
	if err != nil {
		return "", edgert.FromJS(err)
	}
	return renderOutput(edgert.Realm().Await(promise))
}

// renderOutput returns a template's output as a Go string, with errors as
// the API reports them. The string is UTF-8: a lone surrogate of the
// JavaScript string becomes U+FFFD, as it does when Node encodes a string as
// UTF-8.
func renderOutput(output any, err error) (string, error) {
	if err != nil {
		return "", edgert.FromJS(err)
	}
	s, err := outputString(output)
	if err != nil {
		return "", edgert.FromJS(err)
	}
	return jsstr.ToWellFormed(s), nil
}
