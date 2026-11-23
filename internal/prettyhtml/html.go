package prettyhtml

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
)

// The styles of PrettyPrintHtml.js, verbatim.
const (
	styleString    = "color: rgb(173, 219, 103);"
	styleKey       = "color: rgb(127, 219, 202);"
	styleBoolean   = "color: rgb(247, 140, 108);"
	styleNumber    = "color: rgb(199, 146, 234);"
	styleBigInt    = "color: rgb(199, 146, 234);"
	styleSet       = "color: rgb(255, 203, 139);"
	styleMap       = "color: rgb(255, 203, 139);"
	styleSymbol    = "color: rgb(255, 203, 139);"
	styleNull      = "color: rgb(255, 203, 139);"
	styleFunction  = "color: rgb(255, 203, 139);"
	styleRegex     = "color: rgb(255, 86, 86);"
	styleError     = "color: rgb(255, 86, 86);"
	styleWeakMap   = "color: #f8f8f8;"
	styleWeakSet   = "color: #f8f8f8;"
	styleCircular  = "color: #f8f8f8;"
	styleUndefined = "color: #999;"
	stylePre       = `
    padding: 30px 25px;
    background-color: rgb(6, 21, 38);
    color: rgb(214, 222, 235);
    border-radius: 6px;
    font-size: 14px;
    overflow: auto;
    white-space: pre;
    word-spacing: normal;
    word-break: normal;
    word-wrap: normal;
    tab-size: 4;
    line-height: 1.4;
    text-align: left;
  `
	styleCode = `font-family: JetBrains Mono, Menlo, Monaco, monospace;`
)

var (
	// jsonLineRegex matches one line of the inspector's output: its indentation, an
	// optional key, an optional value and the trailing brackets and commas.
	jsonLineRegex = jsval.MustCompile(`^( *)("[^"]+": )?("[^"].*"|[\w.+-]*)?([{}[\],]*)?$`, "mg")
	// findNameRegex strips the ": " after a key.
	findNameRegex = jsval.MustCompile(`(.*)(): `, "")
)

// HTML is inspect.string.html(value), new PrettyPrintHtml().print(value): the
// inspector's text of value with every line coloured, in a <pre><code> block. It
// is the HTML Edge's inspect() global marks safe.
func HTML(ops *jsval.Ops, value any) (string, error) {
	p, err := NewPrinter(ops)
	if err != nil {
		return "", err
	}
	return p.HTML(value)
}

// HTML is print(value) with the printer's builtins.
func (p *Printer) HTML(value any) (string, error) {
	text, err := p.Inspect(value, Options{})
	if err != nil {
		return "", err
	}
	return `<pre style="` + stylePre + `"><code style="` + styleCode + `">` + processJSON(text) + `</code></pre>`, nil
}

// processJSON is PrettyPrintHtml's processJson: value.replace(jsonLineRegex,
// replacer), where the replacer builds the HTML of a line from its four groups.
func processJSON(value string) string {
	units := jsstr.ToUTF16(value)
	var out []uint16
	last, pos := 0, 0
	for pos <= len(units) {
		caps := jsonLineRegex.Exec(units, pos)
		if caps == nil {
			break
		}
		groups := jsregexp.Captures(units, caps)
		out = append(out, units[last:caps[0]]...)
		out = append(out, jsstr.ToUTF16(replaceLine(groups[1], groups[2], groups[3], groups[4]))...)
		last = caps[1]
		if caps[1] == caps[0] {
			pos = jsonLineRegex.AdvanceStringIndex(units, caps[1])
		} else {
			pos = caps[1]
		}
	}
	out = append(out, units[last:]...)
	return jsstr.FromUTF16(out)
}

// replaceLine is PrettyPrintHtml's replacer: each group is a string or
// js.Undefined when it did not participate; empty groups count as absent.
func replaceLine(indent, key, value, end any) string {
	var b strings.Builder
	if s, ok := indent.(string); ok {
		b.WriteString(s)
	}
	if s, ok := key.(string); ok && s != "" {
		b.WriteString(`<span style="` + styleKey + `">` + findNameRegex.ReplaceString(s, "$1$2") + `</span>: `)
	}
	if s, ok := value.(string); ok && s != "" {
		b.WriteString(buildValueHTML(s))
	}
	if s, ok := end.(string); ok {
		b.WriteString(s)
	}
	return b.String()
}

// buildValueHTML is PrettyPrintHtml's buildValueHtml: the value of a line in a
// span coloured by its kind, with the inspector's quotes and type prefixes
// removed.
func buildValueHTML(value string) string {
	style := styleNumber
	switch {
	case strings.HasPrefix(value, `"[Function`):
		style, value = styleFunction, unquote(value)
	case value == `"undefined"`:
		style, value = styleUndefined, "undefined"
	case value == `"[Circular]"`:
		style, value = styleCircular, "[Circular]"
	case value == `"WeakSet {?}"`:
		style, value = styleWeakSet, "WeakSet {?}"
	case value == `"WeakMap {?}"`:
		style, value = styleWeakMap, "WeakMap {?}"
	case strings.HasPrefix(value, `"Symbol`):
		style, value = styleSymbol, unquote(value)
	case strings.HasPrefix(value, `"Error`):
		style, value = styleError, strings.Replace(unquote(value), "Error ", "", 1)
	case strings.HasPrefix(value, `"RegExp`):
		style, value = styleRegex, strings.Replace(unquote(value), "RegExp ", "", 1)
	case strings.HasSuffix(value, `n"`):
		style, value = styleBigInt, unquote(value)
	case strings.HasPrefix(value, `"Set(`):
		if html, ok := collectionHTML(value, styleSet); ok {
			return html
		}
		style = styleSet
	case strings.HasPrefix(value, `"Map(`):
		if html, ok := collectionHTML(value, styleMap); ok {
			return html
		}
		style = styleSet
	case value == `"<empty item>"`:
		style, value = styleUndefined, "<empty item>"
	case strings.HasPrefix(value, `"`):
		style, value = styleString, unquote(value)
	case value == "true" || value == "false":
		style = styleBoolean
	case value == "null":
		style = styleNull
	}
	return `<span style="` + style + `">` + encodeHTML(value) + `</span>`
}

// collectionHTML renders a "Set(n) [...]" or "Map(n) [...]" value whose text holds
// ") [" exactly once: the size in the undefined style, the entries unescaped.
func collectionHTML(value, entriesStyle string) (string, bool) {
	parts := strings.Split(value, ") [")
	if len(parts) != 2 {
		return "", false
	}
	prefix := `<span style="` + styleUndefined + `">` + substr1(parts[0]) + `)</span>`
	values := `<span style="` + entriesStyle + `">[` + dropLast(parts[1]) + `</span>`
	return prefix + " " + values, true
}

// unquote is value.substring(1, value.length - 1), which swaps its bounds when
// the second is the smaller.
func unquote(value string) string {
	u := jsstr.ToUTF16(value)
	start, end := min(1, len(u)), max(len(u)-1, 0)
	if start > end {
		start, end = end, start
	}
	return jsstr.FromUTF16(u[start:end])
}

// substr1 is s.substr(1): s without its first code unit.
func substr1(s string) string {
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		return ""
	}
	return jsstr.FromUTF16(u[1:])
}

// dropLast is s.slice(0, -1): s without its last code unit.
func dropLast(s string) string {
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		return ""
	}
	return jsstr.FromUTF16(u[:len(u)-1])
}

// htmlReplacer is encodeHTML's chain of replacements, applied in order.
var htmlReplacer = []struct{ from, to string }{
	{"&", "&amp;"},
	{`\"`, "&bsol;&quot;"},
	{"<", "&lt;"},
	{">", "&gt;"},
}

// encodeHTML is PrettyPrintHtml's encodeHTML.
func encodeHTML(value string) string {
	for _, r := range htmlReplacer {
		value = strings.ReplaceAll(value, r.from, r.to)
	}
	return value
}
