// Package prettyhtml ports @poppinss/inspect 1.0.1: inspect.js, its JSON-like
// inspector, and PrettyPrintHtml.js, which turns the inspector's output into the
// coloured HTML dump Edge's inspect() global renders.
package prettyhtml

import (
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Options are the options object of inspect.js.
type Options struct {
	// QuoteStyle is opts.quoteStyle, "single" or "double"; "" leaves it unset (strings
	// are single-quoted, element attributes double-quoted). Any other value makes
	// Inspect throw inspect.js's TypeError.
	QuoteStyle string
	// NoWrap is opts.noWrap: strings, dates, elements and the [Object] and
	// [Circular] markers are not wrapped in double quotes.
	NoWrap bool
	// Depth is opts.depth, nil for the default of 5: objects nested that deep print
	// as [Object]. A depth of 0 or less removes the limit.
	Depth *int
}

// mapEntryOptions are the options inspect.js passes for the keys and values of a
// Map: { quoteStyle: 'single', noWrap: true }, without a depth.
var mapEntryOptions = &Options{QuoteStyle: "single", NoWrap: true}

// functionName is nameOf's /^function\s*([\w$]+)/.
var functionName = jsval.MustCompile(`^function\s*([\w$]+)`, "")

// Printer holds the builtins inspect.js captures when its module loads
// (Map.prototype.forEach, the size getters of Map and Set, WeakMap.prototype.has,
// ...). Create it once per realm, before template code runs, to read them as the
// module does; HTML and Inspect create a Printer for each call.
type Printer struct {
	ops *jsval.Ops

	mapSize, mapForEach    *js.Function
	setSize, setForEach    *js.Function
	weakMapHas, weakSetHas *js.Function
	booleanValueOf         *js.Function
}

// NewPrinter reads the builtins of the ops' realm that inspect.js captures.
func NewPrinter(ops *jsval.Ops) (*Printer, error) {
	p := &Printer{ops: ops}
	var err error
	for _, b := range []struct {
		dst  **js.Function
		path []string
	}{
		{&p.mapForEach, []string{"Map", "prototype", "forEach"}},
		{&p.setForEach, []string{"Set", "prototype", "forEach"}},
		{&p.weakMapHas, []string{"WeakMap", "prototype", "has"}},
		{&p.weakSetHas, []string{"WeakSet", "prototype", "has"}},
		{&p.booleanValueOf, []string{"Boolean", "prototype", "valueOf"}},
	} {
		if *b.dst, err = ops.BuiltinFunction(b.path...); err != nil {
			return nil, err
		}
	}
	if p.mapSize, err = ops.SizeGetter("Map"); err != nil {
		return nil, err
	}
	if p.setSize, err = ops.SizeGetter("Set"); err != nil {
		return nil, err
	}
	return p, nil
}

// Inspect is inspect.js's export, inspect_(value, opts): value as the JSON-like
// text PrettyPrintHtml colours. Getters, toString and valueOf of the value run as
// they do in V8, and their errors are returned.
func Inspect(ops *jsval.Ops, value any, opts Options) (string, error) {
	p, err := NewPrinter(ops)
	if err != nil {
		return "", err
	}
	return p.Inspect(value, opts)
}

// Inspect is inspect_(value, opts) with the printer's builtins.
func (p *Printer) Inspect(value any, opts Options) (string, error) {
	return p.inspect(value, &opts, 0, nil, 0, true)
}

// inspect is inspect_(obj, opts, depth, seen, indentation, inNewLine). seen is nil
// for the outermost call, which inspect.js marks by leaving seen undefined; the
// nested calls receive the objects being inspected around them.
func (p *Printer) inspect(obj any, opts *Options, depth int, seen []any, indentation int, inNewLine bool) (string, error) {
	newLine := ""
	if inNewLine {
		newLine = "\n"
	}
	if opts.QuoteStyle != "" && opts.QuoteStyle != "single" && opts.QuoteStyle != "double" {
		return "", js.NewTypeError(`option "quoteStyle" must be "single" or "double"`)
	}

	switch x := obj.(type) {
	case js.UndefinedType:
		return `"undefined"`, nil
	case nil:
		return "null", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case string:
		if opts.NoWrap {
			return inspectString(x, opts), nil
		}
		return `"` + inspectString(x, opts) + `"`, nil
	case float64:
		if x == 0 {
			if math.Signbit(x) {
				return "-0", nil
			}
			return "0", nil
		}
		return js.NumberToString(x), nil
	}

	maxDepth := 5
	if opts.Depth != nil {
		maxDepth = *opts.Depth
	}
	isFunction := js.TypeOf(obj) == "function"
	if depth >= maxDepth && maxDepth > 0 && !isFunction {
		return marker("[Object]", opts), nil
	}
	if seen == nil {
		seen = []any{}
	} else if containsObject(seen, obj) {
		return marker("[Circular]", opts), nil
	}

	// child is inspect.js's inner inspect(value, from, options, indentation,
	// inNewLine) with from = obj: the nested value sees obj among the objects
	// around it.
	child := func(value any, options *Options, indentation int, inNewLine bool) (string, error) {
		if options == nil {
			options = opts
		}
		return p.inspect(value, options, depth+1, append(slices.Clone(seen), obj), indentation, inNewLine)
	}
	// primitive is the inner inspect called without from, for the primitive value
	// of a boxed Number or String.
	primitive := func(value any) (string, error) {
		return p.inspect(value, opts, depth+1, seen, 0, false)
	}

	if isFunction {
		name, err := p.nameOf(obj)
		if err != nil {
			return "", err
		}
		if !js.ToBoolean(name) {
			return `"[Function]"`, nil
		}
		s, err := p.ops.AddString(": ", name)
		if err != nil {
			return "", err
		}
		return `"[Function` + s + `]"`, nil
	}

	tag, err := p.ops.Tag(obj)
	if err != nil {
		return "", err
	}
	if tag == "[object Symbol]" {
		// Symbol.prototype.toString.call(obj) on an object that only claims to be a
		// symbol through Symbol.toStringTag.
		return "", js.NewTypeError("Symbol.prototype.toString requires that 'this' be a Symbol")
	}

	element, err := p.isElement(obj)
	if err != nil {
		return "", err
	}
	if element {
		s, err := p.element(obj, opts)
		if err != nil {
			return "", err
		}
		if opts.NoWrap {
			return s, nil
		}
		return `"` + s + `"`, nil
	}

	if tag == "[object Array]" {
		length, err := p.ops.Get(obj, "length")
		if err != nil {
			return "", err
		}
		if js.StrictEquals(length, 0.0) {
			return "[]", nil
		}
		children, end, start := arrayIndentation(indentation, inNewLine)
		xs, err := p.arrObjKeys(obj, tag, child, children, inNewLine)
		if err != nil {
			return "", err
		}
		val := strings.Join(xs, ","+newLine+space(children))
		return "[" + newLine + space(start) + val + newLine + space(end) + "]", nil
	}

	if tag == "[object Error]" {
		children, end, start := arrayIndentation(indentation, inNewLine)
		parts, err := p.arrObjKeys(obj, tag, child, children, inNewLine)
		if err != nil {
			return "", err
		}
		s, err := p.ops.ToString(obj)
		if err != nil {
			return "", err
		}
		head := `"Error [` + s + `]"`
		if len(parts) == 0 {
			return head, nil
		}
		val := strings.Join(parts, ","+newLine+space(children))
		return head + "{" + newLine + space(start) + val + newLine + space(end) + "}", nil
	}

	if p.isCollection(obj, p.mapSize, p.setSize, "Map", nil) {
		var parts []string
		err := p.forEach(p.mapForEach, obj, func(args []any) error {
			k, err := child(js.Arg(args, 1), mapEntryOptions, indentation, inNewLine)
			if err != nil {
				return err
			}
			v, err := child(js.Arg(args, 0), mapEntryOptions, indentation, inNewLine)
			if err != nil {
				return err
			}
			parts = append(parts, k+" => "+v)
			return nil
		})
		if err != nil {
			return "", err
		}
		return p.collectionOf("Map", obj, p.mapSize, parts)
	}

	if p.isCollection(obj, p.setSize, p.mapSize, "Set", nil) {
		var parts []string
		err := p.forEach(p.setForEach, obj, func(args []any) error {
			v, err := child(js.Arg(args, 0), nil, indentation, inNewLine)
			if err != nil {
				return err
			}
			parts = append(parts, v)
			return nil
		})
		if err != nil {
			return "", err
		}
		return p.collectionOf("Set", obj, p.setSize, parts)
	}

	if p.isCollection(obj, p.weakMapHas, p.weakSetHas, "WeakMap", p.weakMapHas) {
		return `"WeakMap {?}"`, nil
	}
	if p.isCollection(obj, p.weakSetHas, p.weakMapHas, "WeakSet", p.weakSetHas) {
		return `"WeakSet {?}"`, nil
	}

	switch tag {
	case "[object Number]":
		n, err := p.ops.ToNumber(obj)
		if err != nil {
			return "", err
		}
		s, err := primitive(n)
		if err != nil {
			return "", err
		}
		return markBoxed(s), nil
	case "[object BigInt]":
		// bigIntValueOf.call(obj) on an object that only claims to be a BigInt
		// through Symbol.toStringTag.
		return "", js.NewTypeError("BigInt.prototype.valueOf requires that 'this' be a BigInt")
	case "[object Boolean]":
		b, err := p.ops.Realm.Call(p.booleanValueOf, obj, nil)
		if err != nil {
			return "", err
		}
		return markBoxed(js.ToString(b)), nil
	case "[object String]":
		str, err := p.ops.ToString(obj)
		if err != nil {
			return "", err
		}
		s, err := primitive(str)
		if err != nil {
			return "", err
		}
		return markBoxed(s), nil
	case "[object RegExp]":
		s, err := p.ops.ToString(obj)
		if err != nil {
			return "", err
		}
		return `"RegExp ` + s + `"`, nil
	}

	// Buffer.isBuffer(obj) comes next in inspect.js; it is `obj instanceof Buffer`,
	// and the interpreter has no Buffer constructor, so no value it holds is one.

	if tag != "[object Date]" {
		children, end, start := arrayIndentation(indentation, inNewLine)
		xs, err := p.arrObjKeys(obj, tag, child, children, inNewLine)
		if err != nil {
			return "", err
		}
		if len(xs) == 0 {
			return "{}", nil
		}
		value := strings.Join(xs, ","+newLine+space(children))
		return "{" + newLine + space(start) + value + newLine + space(end) + "}", nil
	}

	s, err := p.ops.ToString(obj)
	if err != nil {
		return "", err
	}
	if opts.NoWrap {
		return s, nil
	}
	return `"'` + s + `'"`, nil
}

// containsObject is indexOf(seen, obj) >= 0: obj is one of the objects being
// inspected around it (=== compares objects by identity).
func containsObject(seen []any, obj any) bool {
	if !reflect.TypeOf(obj).Comparable() {
		return false
	}
	for _, s := range seen {
		if reflect.TypeOf(s).Comparable() && js.StrictEquals(s, obj) {
			return true
		}
	}
	return false
}

// marker returns an [Object] or [Circular] marker, quoted unless opts.noWrap.
func marker(s string, opts *Options) string {
	if opts.NoWrap {
		return s
	}
	return `"` + s + `"`
}

// markBoxed is inspect.js's markBoxed: the text of a boxed primitive.
func markBoxed(s string) string {
	return `"Object(` + s + `)"`
}

// nameOf is inspect.js's nameOf(f): f.name when it is truthy, else the name
// /^function\s*([\w$]+)/ finds in String(f), else null.
func (p *Printer) nameOf(f any) (any, error) {
	name, err := p.ops.Get(f, "name")
	if err != nil {
		return nil, err
	}
	if js.ToBoolean(name) {
		return name, nil
	}
	src, err := p.ops.ToString(f)
	if err != nil {
		return nil, err
	}
	units := jsstr.ToUTF16(src)
	if caps := functionName.Exec(units, 0); caps != nil {
		return jsstr.FromUTF16(units[caps[2]:caps[3]]), nil
	}
	return nil, nil
}

// isElement is inspect.js's isElement for an object that is not a function:
// typeof x.nodeName === 'string' && typeof x.getAttribute === 'function'.
// HTMLElement is not defined in Node, so the instanceof test never applies.
func (p *Printer) isElement(x any) (bool, error) {
	nodeName, err := p.ops.Get(x, "nodeName")
	if err != nil {
		return false, err
	}
	if _, ok := nodeName.(string); !ok {
		return false, nil
	}
	getAttribute, err := p.ops.Get(x, "getAttribute")
	if err != nil {
		return false, err
	}
	return js.TypeOf(getAttribute) == "function", nil
}

// lowerNodeName is String(obj.nodeName).toLowerCase().
func (p *Printer) lowerNodeName(obj any) (string, error) {
	nodeName, err := p.ops.Get(obj, "nodeName")
	if err != nil {
		return "", err
	}
	s, err := p.ops.ToString(nodeName)
	if err != nil {
		return "", err
	}
	return jsval.ToLowerCase(s), nil
}

// element renders an element-like object as inspect.js does: its lower-cased
// nodeName, its attributes as name="value" pairs and "..." when it has child
// nodes.
func (p *Printer) element(obj any, opts *Options) (string, error) {
	name, err := p.lowerNodeName(obj)
	if err != nil {
		return "", err
	}
	s := "<" + name
	attrs, err := p.ops.Get(obj, "attributes")
	if err != nil {
		return "", err
	}
	if !js.ToBoolean(attrs) {
		attrs = js.NewArray()
	}
	for i := 0; ; i++ {
		length, err := p.ops.Get(attrs, "length")
		if err != nil {
			return "", err
		}
		lt, ok, err := js.LessThan(float64(i), length, true, p.ops.ToPrimitive)
		if err != nil {
			return "", err
		}
		if !ok || !lt {
			break
		}
		index := strconv.Itoa(i)
		attr, err := p.ops.Get(attrs, index)
		if err != nil {
			return "", err
		}
		attrName, err := p.ops.Get(attr, "name")
		if err != nil {
			return "", err
		}
		part, err := p.ops.AddString(" ", attrName)
		if err != nil {
			return "", err
		}
		if attr, err = p.ops.Get(attrs, index); err != nil {
			return "", err
		}
		attrValue, err := p.ops.Get(attr, "value")
		if err != nil {
			return "", err
		}
		v, err := p.ops.ToString(attrValue)
		if err != nil {
			return "", err
		}
		s += part + "=" + wrapQuotes(strings.ReplaceAll(v, `"`, "&quot;"), "double", opts)
	}
	s += ">"
	childNodes, err := p.ops.Get(obj, "childNodes")
	if err != nil {
		return "", err
	}
	if js.ToBoolean(childNodes) {
		n, err := p.ops.Get(childNodes, "length")
		if err != nil {
			return "", err
		}
		if js.ToBoolean(n) {
			s += "..."
		}
	}
	if name, err = p.lowerNodeName(obj); err != nil {
		return "", err
	}
	return s + "</" + name + ">", nil
}

// isCollection is inspect.js's isMap, isSet, isWeakMap and isWeakSet: calling own
// (a size getter, or WeakMap/WeakSet.prototype.has with arg) on x must succeed and
// calling other must throw; when both succeed the answer is x instanceof ctor.
// Every error counts as false, as the original's try/catch has it.
func (p *Printer) isCollection(x any, own, other *js.Function, ctor string, arg *js.Function) bool {
	call := func(f *js.Function) error {
		var args []any
		if arg != nil {
			args = []any{f}
		}
		_, err := p.ops.Realm.Call(f, x, args)
		return err
	}
	if call(own) != nil {
		return false
	}
	if call(other) != nil {
		return true
	}
	c, err := p.ops.Builtin(ctor)
	if err != nil {
		return false
	}
	f, ok := c.(*js.Function)
	if !ok {
		// instanceof with a right-hand side that is not callable throws a TypeError.
		return false
	}
	is, err := p.ops.InstanceOf(x, f)
	return err == nil && is
}

// forEach is forEachFn.call(obj, callback): Map.prototype.forEach passes (value,
// key), Set.prototype.forEach (value, value).
func (p *Printer) forEach(forEachFn *js.Function, obj any, callback func(args []any) error) error {
	cb := jsval.NewHostFunction("", 1, func(_ any, args []any) (any, error) {
		return js.Undefined, callback(args)
	})
	_, err := p.ops.Realm.Call(forEachFn, obj, []any{cb})
	return err
}

// collectionOf is inspect.js's collectionOf(type, size.call(obj), entries).
func (p *Printer) collectionOf(kind string, obj any, size *js.Function, entries []string) (string, error) {
	n, err := p.ops.Realm.Call(size, obj, nil)
	if err != nil {
		return "", err
	}
	return `"` + kind + "(" + js.ToString(n) + ") [" + strings.Join(entries, ", ") + `]"`, nil
}

// arrObjKeys is inspect.js's arrObjKeys: the items of an array (with
// "<empty item>" for holes), then each own enumerable key for-in visits, skipping
// the array indexes already listed, as `"key": value` or, for keys holding a
// character outside [A-Za-z0-9_$], as the inspected key and the value.
func (p *Printer) arrObjKeys(obj any, tag string, child func(any, *Options, int, bool) (string, error), indentation int, inNewLine bool) ([]string, error) {
	isArr := tag == "[object Array]"
	var xs []string
	length := func() (float64, error) {
		n, err := p.ops.Get(obj, "length")
		if err != nil {
			return 0, err
		}
		return p.ops.ToNumber(n)
	}
	if isArr {
		n, err := length()
		if err != nil {
			return nil, err
		}
		// xs.length = obj.length: items the loop does not reach stay holes, which
		// join prints as empty strings.
		xs = make([]string, int(n))
		for i := 0; ; i++ {
			if n, err = length(); err != nil {
				return nil, err
			}
			if !(float64(i) < n) {
				break
			}
			index := strconv.Itoa(i)
			own, err := p.ops.HasOwn(obj, index)
			if err != nil {
				return nil, err
			}
			s := `"<empty item>"`
			if own {
				v, err := p.ops.Get(obj, index)
				if err != nil {
					return nil, err
				}
				if s, err = child(v, nil, indentation, inNewLine); err != nil {
					return nil, err
				}
			}
			if i < len(xs) {
				xs[i] = s
			} else {
				xs = append(xs, s)
			}
		}
	}
	err := p.ops.ForIn(obj, func(key string) (bool, error) {
		own, err := p.ops.HasOwn(obj, key)
		if err != nil || !own {
			return false, err
		}
		if isArr {
			if num := js.StringToNumber(key); js.NumberToString(num) == key {
				n, err := length()
				if err != nil {
					return false, err
				}
				if num < n {
					return false, nil
				}
			}
		}
		var head string
		if needsQuotes(key) {
			k, err := child(key, nil, 0, true)
			if err != nil {
				return false, err
			}
			head = k + ": "
		} else {
			head = `"` + key + `": `
		}
		v, err := p.ops.Get(obj, key)
		if err != nil {
			return false, err
		}
		s, err := child(v, nil, indentation, inNewLine)
		if err != nil {
			return false, err
		}
		xs = append(xs, head+s)
		return false, nil
	})
	return xs, err
}

// needsQuotes is /[^\w$]/.test(key).
func needsQuotes(key string) bool {
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$') {
			return true
		}
	}
	return false
}

// arrayIndentation is inspect.js's getArrayIndentation: the indentation of the
// children, of the closing bracket and of the first child.
func arrayIndentation(indentation int, inNewLine bool) (children, end, start int) {
	if !inNewLine {
		return 1, 0, 0
	}
	return indentation + 2, indentation, indentation + 2
}

// space is getSpace(n): n spaces.
func space(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// wrapQuotes is inspect.js's wrapQuotes.
func wrapQuotes(s, defaultStyle string, opts *Options) string {
	style := opts.QuoteStyle
	if style == "" {
		style = defaultStyle
	}
	q := "'"
	if style == "double" {
		q = `"`
	}
	return q + s + q
}

// inspectString escapes quotes, backslashes and control characters as inspect.js
// does and wraps the string in the quote style's quotes.
func inspectString(str string, opts *Options) string {
	var b strings.Builder
	for i := 0; i < len(str); i++ {
		c := str[i]
		switch {
		case c == '\'' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20:
			b.WriteString(lowbyte(c))
		default:
			b.WriteByte(c)
		}
	}
	return wrapQuotes(b.String(), "single", opts)
}

// lowbyte is inspect.js's escape for a control character.
func lowbyte(c byte) string {
	switch c {
	case 8:
		return `\b`
	case 9:
		return `\t`
	case 10:
		return `\n`
	case 12:
		return `\f`
	case 13:
		return `\r`
	}
	h := strconv.FormatInt(int64(c), 16)
	if c < 0x10 {
		h = "0" + h
	}
	return `\x` + h
}
