package inspect

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/edge-js/edge-go/internal/jsdate"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/js"
)

// options are the util.inspect options console.log and util.format vary.
type options struct {
	showHidden  bool
	depth       float64 // math.Inf(1) stands for depth: null
	compact     int
	breakLength int
}

// defaultOptions are Node's inspectDefaultOptions, which util.format uses for
// %O and for the arguments past the format string.
var defaultOptions = options{depth: 2, compact: 3, breakLength: 80}

const (
	maxArrayLength  = 100
	maxStringLength = 10000
	minLineWidth    = 16 // kMinLineWidth
)

// The kinds of output reduceToSingleString lays out: an object's entries, or an
// array's, which may be grouped in columns.
const (
	kObjectType = iota
	kArrayExtrasType
)

// Inspect is util.inspect(v) with Node 24's default options.
//
// Two renderings differ from Node because Node's output is not deterministic
// or depends on state the interpreter does not keep: an error prints as
// [Name: message] (what Node prints for an error whose stack has no frames;
// Node prints err.stack, with file paths), and showHidden (%o) lists the stack
// of an error as a data property.
func Inspect(rt Runtime, v any) string {
	return inspectWith(rt, v, defaultOptions)
}

func inspectWith(rt Runtime, v any, opts options) string {
	c := &inspector{rt: rt, opts: opts}
	return c.formatValue(v, 0)
}

type inspector struct {
	rt             Runtime
	opts           options
	seen           []any
	circular       map[any]int
	indentationLvl int
	currentDepth   int
}

func (c *inspector) formatValue(v any, recurseTimes int) string {
	switch v.(type) {
	case nil:
		return "null"
	case string, float64, bool, js.BigInt, js.UndefinedType:
		return c.formatPrimitive(v)
	}
	if slices.Contains(c.seen, v) {
		return "[Circular *" + strconv.Itoa(c.circularIndex(v)) + "]"
	}
	return c.formatRaw(v, recurseTimes)
}

func (c *inspector) circularIndex(v any) int {
	if c.circular == nil {
		c.circular = map[any]int{}
	}
	if i, ok := c.circular[v]; ok {
		return i
	}
	i := len(c.circular) + 1
	c.circular[v] = i
	return i
}

func (c *inspector) formatPrimitive(v any) string {
	switch x := v.(type) {
	case string:
		u := jsstr.ToUTF16(x)
		trailer := ""
		if len(u) > maxStringLength {
			remaining := len(u) - maxStringLength
			u = u[:maxStringLength]
			trailer = "... " + strconv.Itoa(remaining) + " more character" + plural(remaining)
		}
		if len(u) > minLineWidth && len(u) > c.opts.breakLength-c.indentationLvl-4 {
			// Split after every line break: /(?<=\n)/.
			var parts []string
			start := 0
			for i, cu := range u {
				if cu == '\n' {
					parts = append(parts, strEscape(u[start:i+1]))
					start = i + 1
				}
			}
			if start < len(u) {
				parts = append(parts, strEscape(u[start:]))
			}
			return strings.Join(parts, " +\n"+strings.Repeat(" ", c.indentationLvl+2)) + trailer
		}
		return strEscape(u) + trailer
	case float64:
		return formatNumber(x)
	case js.BigInt:
		return formatBigInt(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return "undefined"
}

// formatNumber is inspect's formatNumber without numeric separators: -0 keeps its sign.
func formatNumber(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "-0"
	}
	return js.NumberToString(f)
}

// formatBigInt is inspect's formatBigInt without numeric separators: the
// digits and the n of a literal.
func formatBigInt(b js.BigInt) string { return b.String() + "n" }

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// ctorName is the result of getConstructorName: a name, or null (isNull) for
// an object whose prototype chain has no constructor.
type ctorName struct {
	name   string
	isNull bool
}

// getPrefix is inspect's getPrefix: "Name(size) [tag] " for a constructor
// name, "[fallback(size): null prototype] " without one.
func getPrefix(ctor ctorName, tag, fallback, size string) string {
	if ctor.isNull {
		if tag != "" && fallback != tag {
			return "[" + fallback + size + ": null prototype] [" + tag + "] "
		}
		return "[" + fallback + size + ": null prototype] "
	}
	if tag != "" && ctor.name != tag {
		return ctor.name + size + " [" + tag + "] "
	}
	return ctor.name + size + " "
}

// className is V8's class name of an object of the given kind, which
// getConstructorName falls back to.
func className(k Kind) string {
	switch k {
	case Array:
		return "Array"
	case Function:
		return "Function"
	case Error:
		return "Error"
	case Date:
		return "Date"
	case RegExp:
		return "RegExp"
	case Map:
		return "Map"
	case Set:
		return "Set"
	case WeakMap:
		return "WeakMap"
	case WeakSet:
		return "WeakSet"
	case Promise:
		return "Promise"
	}
	return "Object"
}

// constructorName is getConstructorName: the name of the first constructor
// on v's prototype chain that v is an instance of; for a chain without one,
// null when it ends right away and "Class <prototype>" otherwise.
func (c *inspector) constructorName(v any, recurseTimes int) ctorName {
	var firstProto any
	first := true
	for obj := v; obj != nil; {
		if p, ok := c.rt.OwnProperty(obj, "constructor"); ok && !p.Getter && !p.Setter {
			if f, ok := p.Value.(*js.Function); ok {
				if name := funcName(c.rt, f); name != "" && c.isInstance(v, f) {
					return ctorName{name: name}
				}
			}
		}
		obj = c.rt.Prototype(obj)
		if first {
			firstProto, first = obj, false
		}
	}
	if firstProto == nil {
		return ctorName{isNull: true}
	}
	res := className(c.rt.Describe(v).Kind)
	if float64(recurseTimes) > c.opts.depth {
		return ctorName{name: res + " <Complex prototype>"}
	}
	protoCtor := c.constructorName(firstProto, recurseTimes+1)
	if protoCtor.isNull {
		opts := c.opts
		opts.depth = -1
		return ctorName{name: res + " <" + inspectWith(c.rt, firstProto, opts) + ">"}
	}
	return ctorName{name: res + " <" + protoCtor.name + ">"}
}

// isInstance is v instanceof f for an ordinary function f.
func (c *inspector) isInstance(v any, f *js.Function) bool {
	p, ok := c.rt.OwnProperty(f, "prototype")
	if !ok || !js.IsObject(p.Value) {
		return false
	}
	for o := c.rt.Prototype(v); o != nil; o = c.rt.Prototype(o) {
		if o == p.Value {
			return true
		}
	}
	return false
}

// formatRaw is inspect's formatRaw for an object.
func (c *inspector) formatRaw(v any, recurseTimes int) string {
	desc := c.rt.Describe(v)
	ctor := c.constructorName(v, recurseTimes)
	tag := desc.Tag
	if tag == ctor.name && !ctor.isNull {
		tag = ""
	}
	keys := c.rt.OwnKeys(v, c.opts.showHidden)
	base := ""
	braces := [2]string{"{", "}"}
	extrasType := kObjectType
	var formatter func(recurseTimes int) []string
	var arr *js.Array
	switch desc.Kind {
	case Array:
		arr = v.(*js.Array)
		keys = nonIndexKeys(keys)
		prefix := ""
		if ctor.name != "Array" || tag != "" {
			prefix = getPrefix(ctor, tag, "Array", "("+strconv.Itoa(len(arr.Items))+")")
		}
		braces = [2]string{prefix + "[", "]"}
		if len(arr.Items) == 0 && len(keys) == 0 {
			return braces[0] + "]"
		}
		extrasType = kArrayExtrasType
		formatter = func(rt int) []string { return c.formatArray(arr, rt) }
	case Set, Map:
		size := len(desc.Values)
		fallback := "Set"
		if desc.Kind == Map {
			size /= 2
			fallback = "Map"
		}
		prefix := getPrefix(ctor, tag, fallback, "("+strconv.Itoa(size)+")")
		if size == 0 && len(keys) == 0 {
			return prefix + "{}"
		}
		braces = [2]string{prefix + "{", "}"}
		formatter = func(rt int) []string { return c.formatCollection(desc, rt) }
	case MapIterator, SetIterator:
		typ := "Set"
		if desc.Kind == MapIterator {
			typ = "Map"
		}
		t := desc.Tag
		if t != typ+" Iterator" {
			if t != "" {
				t += "] ["
			}
			t += typ + " Iterator"
		}
		braces = [2]string{"[" + t + "] {", "}"}
		if desc.KeyValue {
			braces[0] = strings.TrimSuffix(braces[0], " Iterator] {") + " Entries] {"
		}
		formatter = func(rt int) []string { return c.formatIterator(desc, rt) }
	default:
		switch {
		case desc.Kind == Function:
			base = c.functionBase(v.(*js.Function), desc, ctor, tag)
			if len(keys) == 0 {
				return base
			}
		case !ctor.isNull && ctor.name == "Object":
			if desc.Kind == Arguments {
				braces[0] = "[Arguments] {"
			} else if tag != "" {
				braces[0] = getPrefix(ctor, tag, "Object", "") + "{"
			}
			if len(keys) == 0 {
				return braces[0] + "}"
			}
		case desc.Kind == RegExp:
			re := v.(*js.RegExp)
			base = "/" + re.Source + "/" + re.Flags
			if prefix := getPrefix(ctor, tag, "RegExp", ""); prefix != "RegExp " {
				base = prefix + base
			}
			if len(keys) == 0 || float64(recurseTimes) > c.opts.depth {
				return base
			}
		case desc.Kind == Date:
			if iso, ok := jsdate.ISOString(v.(*js.Date).Ms); ok {
				base = iso
			} else {
				base = "Invalid Date"
			}
			if prefix := getPrefix(ctor, tag, "Date", ""); prefix != "Date " {
				base = prefix + base
			}
			if len(keys) == 0 {
				return base
			}
		case desc.Kind == Error:
			base = c.formatError(v.(*js.Error))
			keys = c.removeDuplicateErrorKeys(v, keys, base)
			if len(keys) == 0 {
				return base
			}
		case desc.Kind == Promise:
			braces[0] = getPrefix(ctor, tag, "Promise", "") + "{"
			formatter = func(rt int) []string { return c.formatPromise(desc, rt) }
		case desc.Kind == WeakSet || desc.Kind == WeakMap:
			braces[0] = getPrefix(ctor, tag, className(desc.Kind), "") + "{"
			formatter = func(int) []string {
				if c.opts.showHidden {
					return nil
				}
				return []string{"<items unknown>"}
			}
		case desc.Kind == Boxed:
			if s, ok := desc.Primitive.(string); ok {
				keys = slices.DeleteFunc(keys, func(k string) bool {
					i, isIndex := js.ArrayIndex(k)
					return isIndex && i < jsstr.Length(s)
				})
			}
			base = c.boxedBase(desc.Primitive, ctor, tag)
			if len(keys) == 0 {
				return base
			}
		default:
			if len(keys) == 0 {
				return getPrefix(ctor, tag, "Object", "") + "{}"
			}
			braces[0] = getPrefix(ctor, tag, "Object", "") + "{"
		}
	}

	if float64(recurseTimes) > c.opts.depth {
		name := strings.TrimSuffix(getPrefix(ctor, tag, "Object", ""), " ")
		if !ctor.isNull {
			name = "[" + name + "]"
		}
		return name
	}
	recurseTimes++
	c.seen = append(c.seen, v)
	c.currentDepth = recurseTimes
	var output []string
	if formatter != nil {
		output = formatter(recurseTimes)
	}
	for _, k := range keys {
		output = append(output, c.formatProperty(v, k, recurseTimes))
	}
	if index, ok := c.circular[v]; ok {
		reference := "<ref *" + strconv.Itoa(index) + ">"
		if base == "" {
			base = reference
		} else {
			base = reference + " " + base
		}
	}
	c.seen = c.seen[:len(c.seen)-1]
	return c.reduceToSingleString(output, base, braces, extrasType, recurseTimes, arr)
}

// nonIndexKeys drops the array-index keys, which formatArray prints.
func nonIndexKeys(keys []string) []string {
	return slices.DeleteFunc(keys, func(k string) bool {
		_, ok := js.ArrayIndex(k)
		return ok
	})
}

// functionBase is getFunctionBase: [Function: name], [AsyncFunction: name],
// [GeneratorFunction: name], [AsyncGeneratorFunction: name] or, for a class,
// [class name extends Parent], with the constructor and tag when they are
// unusual.
func (c *inspector) functionBase(f *js.Function, desc Object, ctor ctorName, tag string) string {
	if isClassSource(js.FunctionText(f)) {
		return c.classBase(f, ctor, tag)
	}
	typ := "Function"
	if desc.Generator {
		typ = "GeneratorFunction"
	}
	if desc.Async {
		typ = "Async" + typ
	}
	base := "[" + typ
	if ctor.isNull {
		base += " (null prototype)"
	}
	if name := funcName(c.rt, f); name == "" {
		base += " (anonymous)"
	} else {
		base += ": " + name
	}
	base += "]"
	if !ctor.isNull && ctor.name != typ {
		base += " " + ctor.name
	}
	if tag != "" && ctor.name != tag {
		base += " [" + tag + "]"
	}
	return base
}

// isClassSource is getFunctionBase's test of a function's source text for a
// class: it starts with "class", ends with "}", and the text between "class"
// and the first "{" has no "(" once comments are removed.
func isClassSource(src string) bool {
	if !strings.HasPrefix(src, "class") || !strings.HasSuffix(src, "}") {
		return false
	}
	head, _, found := strings.Cut(src[5:len(src)-1], "{")
	if !found {
		return false
	}
	return !strings.Contains(head, "(") || classHeadRegExp.MatchString(stripComments(src[5:len(src)-1]))
}

// classHeadRegExp is Node's classRegExp, /^(\s+[^(]*?)\s*{/: a class head
// with no parenthesis before its body.
var classHeadRegExp = regexp.MustCompile(`^(\s+[^(]*?)\s*\{`)

// stripComments removes // and /* */ comments, as Node's stripCommentsRegExp
// replacement /(\/\/.*?\n)|(\/\*(.|\n)*?\*\/)/g does.
func stripComments(s string) string {
	return commentRegExp.ReplaceAllString(s, "")
}

var commentRegExp = regexp.MustCompile(`(//.*?\n)|(/\*(?:.|\n)*?\*/)`)

// classBase is getClassBase: [class name], with the constructor when it is
// not Function, the tag, and the name of the parent class.
func (c *inspector) classBase(f *js.Function, ctor ctorName, tag string) string {
	name := "(anonymous)"
	if p, ok := c.rt.OwnProperty(f, "name"); ok {
		if s, ok := p.Value.(string); ok && s != "" {
			name = s
		}
	}
	base := "class " + name
	if !ctor.isNull && ctor.name != "Function" {
		base += " [" + ctor.name + "]"
	}
	if tag != "" && ctor.name != tag {
		base += " [" + tag + "]"
	}
	if ctor.isNull {
		base += " extends [null prototype]"
	} else if parent, ok := c.rt.Prototype(f).(*js.Function); ok {
		if superName := funcName(c.rt, parent); superName != "" {
			base += " extends " + superName
		}
	}
	return "[" + base + "]"
}

// boxedBase is getBoxedBase: [Number: 1], [String: 'a'], [Boolean: true],
// [BigInt: 1n].
func (c *inspector) boxedBase(prim any, ctor ctorName, tag string) string {
	typ := "Boolean"
	switch prim.(type) {
	case float64:
		typ = "Number"
	case string:
		typ = "String"
	case js.BigInt:
		typ = "BigInt"
	}
	base := "[" + typ
	if ctor.isNull {
		base += " (null prototype)"
	} else if ctor.name != typ {
		base += " (" + ctor.name + ")"
	}
	plain := &inspector{rt: c.rt, opts: c.opts}
	base += ": " + plain.formatPrimitive(prim) + "]"
	if tag != "" && tag != ctor.name {
		base += " [" + tag + "]"
	}
	return base
}

// formatError renders an error as Node does from its stack: a string stack
// that code assigned is printed as it is, in brackets when it has no "\n    at" frame
// lines after the message; the interpreter's own errors have no frames and
// print as [Name: message]. Lines are indented like Node indents a multi-line
// stack inside a nested value.
func (c *inspector) formatError(e *js.Error) string {
	var stack string
	assigned, _ := c.rt.OwnProperty(e, "stack")
	if s, ok := assigned.Value.(string); ok && s != "" && e.Props != nil && e.Props.Has("stack") {
		stack = s
		pos := -1
		if e.Message != "" {
			if i := strings.Index(stack, e.Message); i >= 0 {
				pos = i + len(e.Message)
			}
		}
		if !strings.Contains(stack[max(pos, 0):], "\n    at") {
			stack = "[" + stack + "]"
		}
	} else {
		name := e.Name
		if name == "" && e.Message == "" {
			name = "Error"
		}
		stack = "[" + js.ToString(&js.Error{Name: name, Message: e.Message}) + "]"
	}
	if c.indentationLvl != 0 {
		stack = strings.ReplaceAll(stack, "\n", "\n"+strings.Repeat(" ", c.indentationLvl))
	}
	return stack
}

// removeDuplicateErrorKeys drops the own name, message and stack properties
// whose value already shows in the rendered error.
func (c *inspector) removeDuplicateErrorKeys(v any, keys []string, base string) []string {
	if c.opts.showHidden {
		return keys
	}
	return slices.DeleteFunc(keys, func(k string) bool {
		if k != "name" && k != "message" && k != "stack" {
			return false
		}
		p, _ := c.rt.OwnProperty(v, k)
		s, ok := p.Value.(string)
		return ok && strings.Contains(base, s)
	})
}

func (c *inspector) formatArray(a *js.Array, recurseTimes int) []string {
	n := min(maxArrayLength, len(a.Items))
	output := make([]string, 0, n+1)
	for i := range n {
		c.indentationLvl += 2
		output = append(output, c.formatValue(a.Items[i], recurseTimes))
		c.indentationLvl -= 2
	}
	if remaining := len(a.Items) - n; remaining > 0 {
		output = append(output, remainingText(remaining))
	}
	return output
}

func remainingText(n int) string {
	return "... " + strconv.Itoa(n) + " more item" + plural(n)
}

// formatCollection is formatSet and formatMap: values, or "key => value".
func (c *inspector) formatCollection(desc Object, recurseTimes int) []string {
	step := 1
	if desc.Kind == Map {
		step = 2
	}
	size := len(desc.Values) / step
	n := min(maxArrayLength, size)
	output := make([]string, 0, n+1)
	c.indentationLvl += 2
	for i := range n {
		s := c.formatValue(desc.Values[i*step], recurseTimes)
		if step == 2 {
			s += " => " + c.formatValue(desc.Values[i*2+1], recurseTimes)
		}
		output = append(output, s)
	}
	c.indentationLvl -= 2
	if remaining := size - n; remaining > 0 {
		output = append(output, remainingText(remaining))
	}
	return output
}

// formatIterator is formatIterator for Map and Set iterators: the values not
// yet produced, entries as [ key, value ].
func (c *inspector) formatIterator(desc Object, recurseTimes int) []string {
	step := 1
	if desc.KeyValue {
		step = 2
	}
	size := len(desc.Values) / step
	n := min(maxArrayLength, size)
	output := make([]string, 0, n+1)
	c.indentationLvl += 2
	for i := range n {
		if step == 1 {
			output = append(output, c.formatValue(desc.Values[i], recurseTimes))
			continue
		}
		pair := []string{c.formatValue(desc.Values[i*2], recurseTimes), c.formatValue(desc.Values[i*2+1], recurseTimes)}
		output = append(output, c.reduceToSingleString(pair, "", [2]string{"[", "]"}, kArrayExtrasType, recurseTimes, nil))
	}
	c.indentationLvl -= 2
	if remaining := size - n; remaining > 0 {
		output = append(output, remainingText(remaining))
	}
	return output
}

// formatPromise shows a promise's state and result.
func (c *inspector) formatPromise(desc Object, recurseTimes int) []string {
	if desc.State == Pending {
		return []string{"<pending>"}
	}
	c.indentationLvl += 2
	s := c.formatValue(desc.Primitive, recurseTimes)
	c.indentationLvl -= 2
	if desc.State == Rejected {
		s = "<rejected> " + s
	}
	return []string{s}
}

// formatProperty formats the own property key of v: "key: value", with the
// key quoted when it is not an identifier and bracketed when it is not
// enumerable; accessors show as [Getter], [Setter] or [Getter/Setter].
func (c *inspector) formatProperty(v any, key string, recurseTimes int) string {
	p, ok := c.rt.OwnProperty(v, key)
	if !ok {
		p = Property{Value: js.Undefined, Enumerable: true}
	}
	var str string
	switch {
	case p.Getter && p.Setter:
		str = "[Getter/Setter]"
	case p.Getter:
		str = "[Getter]"
	case p.Setter:
		str = "[Setter]"
	default:
		c.indentationLvl += 2
		str = c.formatValue(p.Value, recurseTimes)
		c.indentationLvl -= 2
	}
	var name string
	switch {
	case key == "__proto__":
		name = "['__proto__']"
	case !p.Enumerable:
		name = "[" + key + "]"
	case isIdentifierKey(key):
		name = key
	default:
		name = strEscape(jsstr.ToUTF16(key))
	}
	return name + ": " + str
}

// isIdentifierKey is /^[a-zA-Z_][a-zA-Z_0-9]*$/.
func isIdentifierKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		ch := k[i]
		switch {
		case ch == '_', ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func (c *inspector) reduceToSingleString(output []string, base string, braces [2]string, extrasType, recurseTimes int, arr *js.Array) string {
	entries := len(output)
	if extrasType == kArrayExtrasType && entries > 6 {
		output = c.groupArrayElements(output, arr)
	}
	if c.currentDepth-recurseTimes < c.opts.compact && entries == len(output) {
		start := len(output) + c.indentationLvl + jsstr.Length(braces[0]) + jsstr.Length(base) + 10
		if c.isBelowBreakLength(output, start, base) {
			joined := strings.Join(output, ", ")
			if !strings.Contains(joined, "\n") {
				prefix := ""
				if base != "" {
					prefix = base + " "
				}
				return prefix + braces[0] + " " + joined + " " + braces[1]
			}
		}
	}
	indentation := "\n" + strings.Repeat(" ", c.indentationLvl)
	prefix := ""
	if base != "" {
		prefix = base + " "
	}
	return prefix + braces[0] + indentation + "  " + strings.Join(output, ","+indentation+"  ") + indentation + braces[1]
}

func (c *inspector) isBelowBreakLength(output []string, start int, base string) bool {
	total := len(output) + start
	if total+len(output) > c.opts.breakLength {
		return false
	}
	for _, o := range output {
		total += jsstr.Length(o)
		if total > c.opts.breakLength {
			return false
		}
	}
	return base == "" || !strings.Contains(base, "\n")
}

// groupArrayElements lays more than six array entries out in aligned columns.
func (c *inspector) groupArrayElements(output []string, arr *js.Array) []string {
	totalLength, maxLength := 0, 0
	outputLength := len(output)
	if maxArrayLength < len(output) {
		// Leave the "... n more items" entry out of the grouping.
		outputLength--
	}
	const separatorSpace = 2 // a comma and a space
	dataLen := make([]int, outputLength)
	for i := range outputLength {
		n := stringWidth(output[i])
		dataLen[i] = n
		totalLength += n + separatorSpace
		maxLength = max(maxLength, n)
	}
	actualMax := maxLength + separatorSpace
	if actualMax*3+c.indentationLvl >= c.opts.breakLength ||
		(float64(totalLength)/float64(actualMax) <= 5 && maxLength > 6) {
		return output
	}
	const approxCharHeights = 2.5
	averageBias := math.Sqrt(float64(actualMax) - float64(totalLength)/float64(len(output)))
	biasedMax := math.Max(float64(actualMax)-3-averageBias, 1)
	columns := min(
		int(math.Round(math.Sqrt(approxCharHeights*biasedMax*float64(outputLength))/biasedMax)),
		(c.opts.breakLength-c.indentationLvl)/actualMax,
		c.opts.compact*4,
		15,
	)
	if columns <= 1 {
		return output
	}
	maxLineLength := make([]int, 0, columns)
	for i := range columns {
		lineLength := 0
		for j := i; j < outputLength; j += columns {
			lineLength = max(lineLength, dataLen[j])
		}
		maxLineLength = append(maxLineLength, lineLength+separatorSpace)
	}
	// Numbers are right-aligned, anything else left-aligned. The check runs over
	// every output entry, so extra entries ([length], "more items") count as
	// non-numbers unless an array item sits at their index.
	numeric := arr != nil
	for i := 0; numeric && i < len(output); i++ {
		if i >= len(arr.Items) {
			numeric = false
		} else if _, ok := arr.Items[i].(float64); !ok {
			numeric = false
		}
	}
	var tmp []string
	for i := 0; i < outputLength; i += columns {
		maxIdx := min(i+columns, outputLength)
		var b strings.Builder
		j := i
		for ; j < maxIdx-1; j++ {
			padding := maxLineLength[j-i] + jsstr.Length(output[j]) - dataLen[j]
			if numeric {
				b.WriteString(padStart(output[j]+", ", padding))
			} else {
				b.WriteString(padEnd(output[j]+", ", padding))
			}
		}
		if numeric {
			padding := maxLineLength[j-i] + jsstr.Length(output[j]) - dataLen[j] - separatorSpace
			b.WriteString(padStart(output[j], padding))
		} else {
			b.WriteString(output[j])
		}
		tmp = append(tmp, b.String())
	}
	if maxArrayLength < len(output) {
		tmp = append(tmp, output[outputLength])
	}
	return tmp
}

// stringWidth approximates Node's getStringWidth (ICU's East Asian width): ASCII
// control characters, combining marks and format characters are zero wide, East
// Asian wide and fullwidth characters (and the common emoji blocks) are two, all
// else one.
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r < 0x20:
		case r < 0x7f:
			w++
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		case isWide(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}

// strEscape quotes a string the way inspect does: single quotes by default, double
// quotes when the string has a single quote but no double quote, backticks when it
// has both but no backtick or "${"; control characters, C1 controls, the backslash,
// the chosen quote (when it is the single quote) and lone surrogates are escaped.
func strEscape(u []uint16) string {
	quote := uint16('\'')
	if slices.Contains(u, '\'') {
		if !slices.Contains(u, '"') {
			quote = '"'
		} else if !slices.Contains(u, '`') && !containsDollarBrace(u) {
			quote = '`'
		}
	}
	out := make([]uint16, 0, len(u)+2)
	out = append(out, quote)
	for i := 0; i < len(u); i++ {
		point := u[i]
		switch {
		case (point == '\'' && quote == '\'') || point == '\\' || point < 0x20 || (point > 0x7e && point < 0xa0):
			out = append(out, jsstr.ToUTF16(escapeMeta(point))...)
		case point >= 0xd800 && point <= 0xdbff && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff:
			out = append(out, point, u[i+1])
			i++
		case point >= 0xd800 && point <= 0xdfff:
			out = append(out, jsstr.ToUTF16(`\u`+strconv.FormatUint(uint64(point), 16))...)
		default:
			out = append(out, point)
		}
	}
	out = append(out, quote)
	return jsstr.FromUTF16WellFormed(out)
}

// escapeMeta is inspect's meta table entry for an escaped code unit.
func escapeMeta(point uint16) string {
	switch point {
	case '\b':
		return `\b`
	case '\t':
		return `\t`
	case '\n':
		return `\n`
	case '\f':
		return `\f`
	case '\r':
		return `\r`
	case '\'':
		return `\'`
	case '\\':
		return `\\`
	}
	return `\x` + strings.ToUpper(zeroPadHex(point))
}

func zeroPadHex(point uint16) string {
	s := strconv.FormatUint(uint64(point), 16)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func containsDollarBrace(u []uint16) bool {
	for i := 0; i+1 < len(u); i++ {
		if u[i] == '$' && u[i+1] == '{' {
			return true
		}
	}
	return false
}
