package edge

import (
	"unsafe"

	"github.com/edge-js/edge-go/js"
)

// ProcessorHandler processes one event. Returning replace=false leaves the
// value unchanged, as a handler returning undefined does in the original;
// replace=true substitutes value. A "tag" handler's result is ignored: it
// changes the token in place.
type ProcessorHandler func(data ProcessorData) (value string, replace bool, err error)

// ProcessorData is what a handler receives. Handlers of one event run in
// registration order and each sees the value the previous ones left.
type ProcessorData struct {
	Event    string     // "raw", "tag", "compiled" or "output"
	Path     string     // raw, tag, compiled: the template path
	Raw      string     // raw
	Tag      *TagToken  // tag: the token itself; changing Properties.Name or JsArg changes what is compiled
	Compiled string     // compiled
	Output   string     // output
	State    *js.Object // output: the template state
}

// Processor holds the handlers that process templates at the different
// stages of compiling and rendering.
type Processor struct {
	handlers map[string][]ProcessorHandler
}

// newProcessor returns a processor without handlers.
func newProcessor() *Processor {
	return &Processor{handlers: map[string][]ProcessorHandler{}}
}

// Process registers a handler for an event. The handlers of an event form a
// set: registering the same function value again has no effect.
func (p *Processor) Process(event string, handler ProcessorHandler) *Processor {
	for _, h := range p.handlers[event] {
		if sameFunc(h, handler) {
			return p
		}
	}
	p.handlers[event] = append(p.handlers[event], handler)
	return p
}

// sameFunc reports whether two handlers are the same function value. Go does
// not compare func values, but a func value is a pointer to its closure
// record, which identifies the value as a JavaScript function object's
// reference does.
func sameFunc(a, b ProcessorHandler) bool {
	return *(*unsafe.Pointer)(unsafe.Pointer(&a)) == *(*unsafe.Pointer)(unsafe.Pointer(&b))
}

// executeTag runs the "tag" handlers on a token before it is compiled.
func (p *Processor) executeTag(tag *TagToken, path string) error {
	for _, h := range p.handlers["tag"] {
		if _, _, err := h(ProcessorData{Event: "tag", Tag: tag, Path: path}); err != nil {
			return err
		}
	}
	return nil
}

// executeRaw runs the "raw" handlers on a template's source.
func (p *Processor) executeRaw(raw, path string) (string, error) {
	for _, h := range p.handlers["raw"] {
		v, replace, err := h(ProcessorData{Event: "raw", Raw: raw, Path: path})
		if err != nil {
			return "", err
		}
		if replace {
			raw = v
		}
	}
	return raw, nil
}

// executeCompiled runs the "compiled" handlers on a template's generated code.
func (p *Processor) executeCompiled(compiled, path string) (string, error) {
	for _, h := range p.handlers["compiled"] {
		v, replace, err := h(ProcessorData{Event: "compiled", Compiled: compiled, Path: path})
		if err != nil {
			return "", err
		}
		if replace {
			compiled = v
		}
	}
	return compiled, nil
}

// executeOutput runs the "output" handlers on a rendered template.
func (p *Processor) executeOutput(output string, state *js.Object) (string, error) {
	for _, h := range p.handlers["output"] {
		v, replace, err := h(ProcessorData{Event: "output", Output: output, State: state})
		if err != nil {
			return "", err
		}
		if replace {
			output = v
		}
	}
	return output, nil
}
