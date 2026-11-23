package inspect

import (
	"strings"

	"github.com/edge-js/edge-go/js"
)

// builtInConstructors are the constructors whose toString util.format's %s
// treats as built in (Node's builtInObjects, restricted to the globals the
// interpreter has).
var builtInConstructors = map[string]bool{
	"Object": true, "Function": true, "Array": true, "Number": true, "Boolean": true,
	"String": true, "Date": true, "RegExp": true, "Error": true, "TypeError": true,
	"RangeError": true, "SyntaxError": true, "ReferenceError": true, "URIError": true,
	"EvalError": true, "AggregateError": true, "Map": true, "Set": true, "WeakMap": true,
	"WeakSet": true, "Promise": true,
}

// Format is util.format(...args), what console.log prints (without the line
// break): a first string argument is a format whose %s %d %i %f %j %o %O %c
// and %% are replaced by the following arguments, and the remaining
// arguments follow, strings as they are and other values inspected. The
// conversions may call JavaScript code (toString, valueOf, toJSON), whose
// errors are returned.
func Format(rt Runtime, args []any) (string, error) {
	var b strings.Builder
	a := 0
	join := ""
	if first, ok := js.Arg(args, 0).(string); ok {
		if len(args) == 1 {
			return first, nil
		}
		lastPos := 0
		for i := 0; i < len(first)-1; i++ {
			if first[i] != '%' {
				continue
			}
			i++
			next := first[i]
			if a+1 == len(args) {
				if next == '%' {
					b.WriteString(first[lastPos:i])
					lastPos = i + 1
				}
				continue
			}
			var s string
			var err error
			switch next {
			case 's':
				a++
				s, err = formatString(rt, args[a])
			case 'j':
				a++
				s, err = formatJSON(rt, args[a])
			case 'd':
				a++
				if b, ok := args[a].(js.BigInt); ok {
					s = formatBigInt(b)
					break
				}
				var n float64
				n, err = rt.ToNumber(args[a])
				s = formatNumber(n)
			case 'i':
				a++
				if b, ok := args[a].(js.BigInt); ok {
					s = formatBigInt(b)
					break
				}
				var n float64
				n, err = rt.ParseInt(args[a])
				s = formatNumber(n)
			case 'f':
				a++
				var n float64
				n, err = rt.ParseFloat(args[a])
				s = formatNumber(n)
			case 'O':
				a++
				s = Inspect(rt, args[a])
			case 'o':
				a++
				s = inspectWith(rt, args[a], options{showHidden: true, depth: 4, compact: 3, breakLength: 80})
			case 'c':
				a++
			case '%':
				b.WriteString(first[lastPos:i])
				lastPos = i + 1
				continue
			default:
				continue
			}
			if err != nil {
				return "", err
			}
			if lastPos != i-1 {
				b.WriteString(first[lastPos : i-1])
			}
			b.WriteString(s)
			lastPos = i + 1
		}
		if lastPos != 0 {
			a++
			join = " "
			if lastPos < len(first) {
				b.WriteString(first[lastPos:])
			}
		}
	}
	for ; a < len(args); a++ {
		b.WriteString(join)
		if s, ok := args[a].(string); ok {
			b.WriteString(s)
		} else {
			b.WriteString(Inspect(rt, args[a]))
		}
		join = " "
	}
	return b.String(), nil
}

// formatString is %s: numbers and BigInts as inspect prints them, objects
// whose toString is a builtin one inspected to depth 0, everything else
// String(v).
func formatString(rt Runtime, v any) (string, error) {
	switch x := v.(type) {
	case float64:
		return formatNumber(x), nil
	case js.BigInt:
		return formatBigInt(x), nil
	}
	if _, isFunc := v.(*js.Function); isFunc || !js.IsObject(v) || !hasBuiltInToString(rt, v) {
		return rt.ToString(v)
	}
	return inspectWith(rt, v, options{depth: 0, compact: 3, breakLength: 80}), nil
}

// hasBuiltInToString reports whether the toString v would use is a builtin's:
// v has none, or the nearest object defining one has a builtin constructor.
func hasBuiltInToString(rt Runtime, v any) bool {
	holder := v
	for ; holder != nil; holder = rt.Prototype(holder) {
		if p, ok := rt.OwnProperty(holder, "toString"); ok {
			if p.Getter || p.Setter {
				return false
			}
			if _, isFunc := p.Value.(*js.Function); !isFunc {
				return true
			}
			break
		}
	}
	if holder == nil {
		return true
	}
	if holder == v {
		return false
	}
	p, ok := rt.OwnProperty(holder, "constructor")
	if !ok {
		return false
	}
	f, isFunc := p.Value.(*js.Function)
	return isFunc && builtInConstructors[funcName(rt, f)]
}

// formatJSON is %j: JSON.stringify(v), "[Circular]" for a cyclic value.
func formatJSON(rt Runtime, v any) (string, error) {
	s, ok, err := rt.JSONStringify(v)
	if err != nil {
		if e, isErr := err.(*js.Error); isErr && strings.HasPrefix(e.Message, "Converting circular structure to JSON") {
			return "[Circular]", nil
		}
		return "", err
	}
	if !ok {
		return "undefined", nil
	}
	return s, nil
}
