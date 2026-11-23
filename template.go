package edge

import (
	"strings"
	"sync"

	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt"
	"github.com/edge-js/edge-go/internal/lodash"
	"github.com/edge-js/edge-go/js"
)

// templateFields are the private fields of a Template instance: the
// compiler and processor it renders with (a *Compiler and a *Processor, or
// whatever template code passed to `new Template(...)`), and the state
// shared with its components: the globals and locals, merged deeply in
// compat mode.
type templateFields struct {
	compiler    any
	processor   any
	sharedState any
}

// templateState holds the private fields of Template instances.
var templateState edgert.Slots[jsrt.Derived, *templateFields]

// fields reads the private fields of a Template method's receiver, reading
// the private member name as V8 does.
func fields(this any, name string) (*templateFields, error) {
	return templateState.Get(this, name)
}

// macroableClass is @poppinss/macroable's Macroable, the base class of
// Template: its static macro and getter add properties to the prototype,
// instanceProperty adds a property every new instance receives.
var macroableClass = sync.OnceValue(func() *jsrt.Class {
	c := edgert.NewClass("Macroable", 0, nil, applyInstanceMacros)
	edgert.DefineMethod(c.Ctor, "Macroable.macro", "macro", 2, func(this any, args []any) (any, error) {
		return js.Undefined, defineMacro(this, js.Arg(args, 0), js.Arg(args, 1))
	})
	edgert.DefineMethod(c.Ctor, "Macroable.instanceProperty", "instanceProperty", 2, instanceProperty)
	edgert.DefineMethod(c.Ctor, "Macroable.getter", "getter", 2, func(this any, args []any) (any, error) {
		singleton := js.Arg(args, 2)
		if _, undefined := singleton.(js.UndefinedType); undefined {
			singleton = false
		}
		return js.Undefined, defineGetter(this, js.Arg(args, 0), js.Arg(args, 1), singleton)
	})
	set, err := newSet(js.Undefined)
	if err != nil {
		// The realm's Set constructor builds an empty set from undefined.
		panic(err)
	}
	if err := edgert.Realm().DefineValue(c.Ctor, "instanceMacros", set, 0); err != nil {
		// A new class constructor accepts new properties.
		panic(err)
	}
	return c
})

// newSet is `new Set(iterable)`.
func newSet(iterable any) (any, error) {
	ctor, err := edgert.Ops().BuiltinFunction("Set")
	if err != nil {
		return nil, err
	}
	return edgert.Realm().Construct(ctor, []any{iterable})
}

// applyInstanceMacros is the Macroable constructor: every instance property
// registered on the class is assigned to the new instance, functions bound to
// it (with their own enumerable properties bound too).
func applyInstanceMacros(this any, _ []any) error {
	r := edgert.Realm()
	ctor, err := r.Get(this, "constructor")
	if err != nil {
		return err
	}
	macros, err := r.Get(ctor, "instanceMacros")
	if err != nil {
		return err
	}
	apply := jsrt.NewNative("", 1, func(_ any, args []any) (any, error) {
		entry := js.Arg(args, 0)
		key, err := r.Get(entry, "key")
		if err != nil {
			return nil, err
		}
		value, err := r.Get(entry, "value")
		if err != nil {
			return nil, err
		}
		name, err := r.ToPropertyKey(key)
		if err != nil {
			return nil, err
		}
		if js.TypeOf(value) != "function" {
			return js.Undefined, r.Set(this, name, value, true)
		}
		bound, err := edgert.Ops().Invoke(value, "bind", []any{this}, "value.bind")
		if err != nil {
			return nil, err
		}
		props, err := r.OwnEnumerableKeys(value)
		if err != nil {
			return nil, err
		}
		for _, prop := range props {
			propValue, err := r.Get(value, prop)
			if err != nil {
				return nil, err
			}
			boundProp, err := edgert.Ops().Invoke(propValue, "bind", []any{this}, "propValue.bind")
			if err != nil {
				return nil, err
			}
			if err := r.Set(bound, prop, boundProp, true); err != nil {
				return nil, err
			}
		}
		return js.Undefined, r.Set(this, name, bound, true)
	})
	_, err = edgert.Ops().Invoke(macros, "forEach", []any{apply}, "Constructor.instanceMacros.forEach")
	return err
}

// defineMacro is Macroable.macro: `this.prototype[name] = value`.
func defineMacro(class, name, value any) error {
	proto, err := edgert.Realm().Get(class, "prototype")
	if err != nil {
		return err
	}
	key, err := edgert.Realm().ToPropertyKey(name)
	if err != nil {
		return err
	}
	return edgert.Realm().Set(proto, key, value, true)
}

// instanceProperty is Macroable.instanceProperty(name, value): the class gets
// its own copy of the inherited set of instance properties, and the property
// is added to it.
func instanceProperty(this any, args []any) (any, error) {
	r := edgert.Realm()
	own, err := edgert.Ops().Invoke(this, "hasOwnProperty", []any{"instanceMacros"}, "self.hasOwnProperty")
	if err != nil {
		return nil, err
	}
	if !js.ToBoolean(own) {
		inherited, err := r.Get(this, "instanceMacros")
		if err != nil {
			return nil, err
		}
		set, err := newSet(inherited)
		if err != nil {
			return nil, err
		}
		if err := r.DefineValue(this, "instanceMacros", set, 0); err != nil {
			return nil, err
		}
	}
	macros, err := r.Get(this, "instanceMacros")
	if err != nil {
		return nil, err
	}
	entry := js.ObjectOf("key", js.Arg(args, 0), "value", js.Arg(args, 1))
	_, err = edgert.Ops().Invoke(macros, "add", []any{entry}, "self.instanceMacros.add")
	return js.Undefined, err
}

// defineGetter is Macroable.getter: an accessor on the prototype whose getter
// calls accumulator with the instance; a singleton getter then defines the
// value as a read-only own property of the instance.
func defineGetter(class, name, accumulator, singleton any) error {
	r := edgert.Realm()
	proto, err := r.Get(class, "prototype")
	if err != nil {
		return err
	}
	key, err := r.ToPropertyKey(name)
	if err != nil {
		return err
	}
	get := edgert.NewFunction("Macroable.getter.get", "get", 0, func(this any, _ []any) (any, error) {
		value, err := edgert.Ops().Invoke(accumulator, "call", []any{this}, "accumulator.call")
		if err != nil {
			return nil, err
		}
		if js.ToBoolean(singleton) {
			if err := r.DefineValue(this, key, value, js.ReadOnly|js.Hidden|js.Fixed); err != nil {
				return nil, err
			}
		}
		return value, nil
	})
	return r.DefineAccessor(proto, key, get, nil, js.Hidden)
}

// templateClass is Template: the `template` object compiled templates
// receive, with the methods generated code calls to compile partials and
// components, escape output and raise errors, and the macros tags add.
var templateClass = sync.OnceValue(func() *jsrt.Class {
	c := edgert.NewClass("Template", 4, macroableClass(), initTemplate)
	method := func(name string, length int, fn jsrt.NativeFunc) {
		edgert.DefineMethod(c.Proto, "Template."+name, name, length, func(this any, args []any) (any, error) {
			v, err := fn(this, args)
			return v, edgert.ToJS(err)
		})
	}
	method("compilePartial", 1, func(this any, args []any) (any, error) {
		f, err := fields(this, "#compiler")
		if err != nil {
			return nil, err
		}
		locals := []any{}
		if len(args) > 1 {
			locals = append(locals, args[1:]...)
		}
		return compileWith(f.compiler, js.Arg(args, 0), locals)
	})
	method("compileComponent", 1, func(this any, args []any) (any, error) {
		f, err := fields(this, "#compiler")
		if err != nil {
			return nil, err
		}
		return compileWith(f.compiler, js.Arg(args, 0), nil)
	})
	method("getComponentState", 3, func(this any, args []any) (any, error) {
		f, err := fields(this, "#sharedState")
		if err != nil {
			return nil, err
		}
		return f.componentState(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
	})
	method("render", 2, func(this any, args []any) (any, error) {
		f, err := fields(this, "#compiler")
		if err != nil {
			return nil, err
		}
		compiled, err := compileWith(f.compiler, js.Arg(args, 0), nil)
		if err != nil {
			return nil, err
		}
		return renderCompiled(this, f, compiled, js.Arg(args, 1))
	})
	method("renderRaw", 3, func(this any, args []any) (any, error) {
		f, err := fields(this, "#compiler")
		if err != nil {
			return nil, err
		}
		compiled, err := compileRawWith(f.compiler, js.Arg(args, 0), js.Arg(args, 2))
		if err != nil {
			return nil, err
		}
		return renderCompiled(this, f, compiled, js.Arg(args, 1))
	})
	method("escape", 1, func(_ any, args []any) (any, error) {
		return edgert.Escape(js.Arg(args, 0))
	})
	method("createError", 4, func(_ any, args []any) (any, error) {
		return createError(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2), js.Arg(args, 3))
	})
	method("newError", 4, func(this any, args []any) (any, error) {
		errorArgs := []any{js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2), js.Arg(args, 3)}
		e, err := edgert.Ops().Invoke(this, "createError", errorArgs, "this.createError")
		if err != nil {
			return nil, err
		}
		return nil, jsrt.ThrowValue(e)
	})
	method("reThrow", 3, func(_ any, args []any) (any, error) {
		return nil, reThrow(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
	})
	return c
})

// initTemplate is the Template constructor after super(): the stacks class
// field, then the private fields from (compiler, globals, locals, processor).
func initTemplate(this any, args []any) error {
	stacks, err := newStacks()
	if err != nil {
		return err
	}
	this.(*jsrt.Derived).Own.Set("stacks", stacks)
	compiler, globals, locals, processor := js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2), js.Arg(args, 3)
	compat, err := compilerFlag(compiler, "compat")
	if err != nil {
		return err
	}
	shared := any(js.NewObject())
	if compat {
		if shared, err = lodash.Merge(edgert.Ops(), shared, globals, locals); err != nil {
			return err
		}
	} else {
		for _, source := range []any{globals, locals} {
			if err := edgert.Spread(shared.(*js.Object), source); err != nil {
				return err
			}
		}
	}
	templateState.Attach(this.(*jsrt.Derived), &templateFields{compiler: compiler, processor: processor, sharedState: shared})
	return nil
}

// template is a Template instance created to render one template.
type template struct {
	obj    any
	fields *templateFields
}

// newTemplate is `new Template(compiler, globals, locals, processor)`.
func newTemplate(compiler *Compiler, globals, locals *js.Object, processor *Processor) (*template, error) {
	obj, err := templateClass().Construct([]any{compiler, globals, locals, processor})
	if err != nil {
		return nil, err
	}
	f, _ := templateState.Lookup(obj)
	return &template{obj: obj, fields: f}, nil
}

// render is template.render(templatePath, state): the output, or a promise of
// it for the async compiler.
func (t *template) render(templatePath string, state any) (any, error) {
	compiled, err := compileWith(t.fields.compiler, templatePath, nil)
	if err != nil {
		return nil, err
	}
	return renderCompiled(t.obj, t.fields, compiled, state)
}

// renderRaw is template.renderRaw(contents, state, templatePath).
func (t *template) renderRaw(contents string, state, templatePath any) (any, error) {
	compiled, err := compileRawWith(t.fields.compiler, contents, templatePath)
	if err != nil {
		return nil, err
	}
	return renderCompiled(t.obj, t.fields, compiled, state)
}

// componentState is getComponentState(props, slots, caller): the shared
// state, the props, then $slots, $caller and $props (a ComponentProps, or a
// Props in compat mode).
func (f *templateFields) componentState(props, slots, caller any) (any, error) {
	state := js.NewObject()
	for _, source := range []any{f.sharedState, props} {
		if err := edgert.Spread(state, source); err != nil {
			return nil, err
		}
	}
	compat, err := compilerFlag(f.compiler, "compat")
	if err != nil {
		return nil, err
	}
	newProps := edgert.NewComponentProps
	if compat {
		newProps = edgert.NewProps
	}
	propsObject, err := newProps(props)
	if err != nil {
		return nil, err
	}
	state.Set("$slots", slots)
	state.Set("$caller", caller)
	state.Set("$props", propsObject)
	return state, nil
}

// renderCompiled runs a compiled template with { ...sharedState, ...state }
// and a new $context, then trims one line break at each end of the output,
// fills the stack placeholders and runs the output processors. With the
// async compiler the template function returns a promise, and so does
// renderCompiled.
func renderCompiled(this any, f *templateFields, compiled, state any) (any, error) {
	templateState := js.NewObject()
	for _, source := range []any{f.sharedState, state} {
		if err := edgert.Spread(templateState, source); err != nil {
			return nil, err
		}
	}
	context := js.NewObject()
	async, err := compilerFlag(f.compiler, "async")
	if err != nil {
		return nil, err
	}
	output, err := edgert.Ops().Call(compiled, js.Undefined, []any{this, templateState, context}, "compiledTemplate")
	if err != nil {
		return nil, err
	}
	finish := func(output any) (any, error) {
		output, err := trimTopBottomNewLines(output)
		if err != nil {
			return nil, err
		}
		stacks, err := edgert.Realm().Get(this, "stacks")
		if err != nil {
			return nil, err
		}
		if output, err = edgert.Ops().Invoke(stacks, "fillPlaceholders", []any{output}, "this.stacks.fillPlaceholders"); err != nil {
			return nil, err
		}
		return executeOutput(f.processor, output, this, templateState)
	}
	if !async {
		return finish(output)
	}
	then := jsrt.NewNative("", 1, func(_ any, args []any) (any, error) {
		v, err := finish(js.Arg(args, 0))
		return v, edgert.ToJS(err)
	})
	return edgert.Ops().Invoke(output, "then", []any{then}, "compiledTemplate(...).then")
}

// trimTopBottomNewLines removes one line break ("\n" or "\r\n") at the start
// and one at the end of the output:
// value.replace(/^\n|^\r\n/, "").replace(/\n$|\r\n$/, "").
func trimTopBottomNewLines(value any) (any, error) {
	if s, ok := value.(string); ok {
		if !strings.HasPrefix(s, "\n") {
			s = strings.TrimPrefix(s, "\r\n")
		} else {
			s = s[1:]
		}
		if !strings.HasSuffix(s, "\r\n") {
			s = strings.TrimSuffix(s, "\n")
		} else {
			s = s[:len(s)-2]
		}
		return s, nil
	}
	trimmed, err := edgert.Ops().Invoke(value, "replace", []any{&js.RegExp{Source: `^\n|^\r\n`}, ""}, "value.replace")
	if err != nil {
		return nil, err
	}
	return edgert.Ops().Invoke(trimmed, "replace", []any{&js.RegExp{Source: `\n$|\r\n$`}, ""}, "value.replace(...).replace")
}

// executeOutput is this.#processor.executeOutput({ output, template, state }).
func executeOutput(processor, output, template, state any) (any, error) {
	p, ok := processor.(*Processor)
	if !ok {
		data := js.ObjectOf("output", output, "template", template, "state", state)
		return edgert.Ops().Invoke(processor, "executeOutput", []any{data}, "this.#processor.executeOutput")
	}
	text, err := outputString(output)
	if err != nil {
		return nil, err
	}
	stateObject, _ := state.(*js.Object)
	return p.executeOutput(text, stateObject)
}

// outputString is the output of a template as a Go string: the string, or
// what String() makes of another value.
func outputString(output any) (string, error) {
	if s, ok := output.(string); ok {
		return s, nil
	}
	return edgert.Realm().ToString(output)
}

// compilerFlag reads compiler.compat or compiler.async.
func compilerFlag(compiler any, flag string) (bool, error) {
	if c, ok := compiler.(*Compiler); ok {
		if flag == "compat" {
			return c.Compat, nil
		}
		return c.async, nil
	}
	v, err := edgert.Realm().Get(compiler, flag)
	return js.ToBoolean(v), err
}

// compileWith is compiler.compile(templatePath, localVariables); nil
// localVariables are undefined.
func compileWith(compiler, templatePath any, localVariables []any) (any, error) {
	if c, ok := compiler.(*Compiler); ok {
		return c.compileValue(templatePath, localVariables)
	}
	args := []any{templatePath, js.Undefined}
	if localVariables != nil {
		args[1] = js.NewArray(localVariables...)
	}
	return edgert.Ops().Invoke(compiler, "compile", args, "this.#compiler.compile")
}

// compileRawWith is compiler.compileRaw(contents, templatePath).
func compileRawWith(compiler, contents, templatePath any) (any, error) {
	if c, ok := compiler.(*Compiler); ok {
		return c.compileRawValue(contents, templatePath)
	}
	return edgert.Ops().Invoke(compiler, "compileRaw", []any{contents, templatePath}, "this.#compiler.compileRaw")
}

// createError is `new EdgeError(errorMessage, "E_RUNTIME_EXCEPTION", {
// filename, line: lineNumber, col: column })`.
func createError(errorMessage, filename, lineNumber, column any) (any, error) {
	return edgert.NewEdgeError(errorMessage, "E_RUNTIME_EXCEPTION", js.ObjectOf("filename", filename, "line", lineNumber, "col", column))
}

// reThrow rethrows an exception of template code: an EdgeError as it is, any
// other error as an E_RUNTIME_EXCEPTION at the template's filename and line,
// its message without the first "state." the compiled code introduced.
func reThrow(err, filename, lineNumber any) error {
	if edgert.IsEdgeError(err) {
		return jsrt.ThrowValue(err)
	}
	message, e := edgert.Realm().Get(err, "message")
	if e != nil {
		return e
	}
	if s, ok := message.(string); ok {
		message = strings.Replace(s, "state.", "", 1)
	} else if message, e = edgert.Ops().Invoke(message, "replace", []any{&js.RegExp{Source: `state\.`}, ""}, "error.message.replace"); e != nil {
		return e
	}
	rethrown, e := edgert.NewEdgeError(message, "E_RUNTIME_EXCEPTION", js.ObjectOf("filename", filename, "line", lineNumber, "col", 0.0))
	if e != nil {
		return e
	}
	return jsrt.ThrowValue(rethrown)
}

// macro adds a macro to the Template class.
func macro(name string, value any) error {
	return defineMacro(templateClass().Ctor, name, value)
}

// Macro adds a property to Template.prototype, visible to every template as
// template.<name>: Template.macro(name, value).
func Macro(name string, value any) {
	if err := macro(name, value); err != nil {
		// Template.prototype is an ordinary extensible object whose own
		// properties are writable.
		panic(err)
	}
}

// Getter adds a getter to Template.prototype: template.<name> calls
// accumulator with the template, once when singleton is set (the value then
// becomes a read-only property of the template). Template.getter(name,
// accumulator, singleton).
func Getter(name string, accumulator *js.Function, singleton bool) {
	if err := defineGetter(templateClass().Ctor, name, accumulator, singleton); err != nil {
		// Template.prototype is an ordinary extensible object whose own
		// properties are configurable.
		panic(err)
	}
}
