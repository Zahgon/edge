// Package stringifyattrs is stringify-attributes 4.0.0, which the migrate
// plugin's Props.serialize, serializeOnly and serializeExcept use to turn props
// into HTML attributes.
package stringifyattrs

import (
	"strings"

	"github.com/edge-js/edge-go/internal/escapegoat"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// StringifyAttributes is stringifyAttributes(attributes): for each entry of
// Object.entries(attributes), in order, skipping the values false, undefined
// and null, the escaped key, followed by ="value" with the escaped String(value)
// unless the value is true; an array value is first joined with spaces
// (value.join(" "), through the realm). The attributes are joined with spaces
// and the result starts with a space, or is "" when there is none.
func StringifyAttributes(ops *jsval.Ops, attributes any) (string, error) {
	entries, err := ops.Realm.Entries(attributes)
	if err != nil {
		return "", err
	}
	var handled []string
	for _, entry := range entries {
		value := entry.Value
		if value == false || value == js.Undefined || value == nil {
			continue
		}
		isArray, err := ops.IsArray(value)
		if err != nil {
			return "", err
		}
		if isArray {
			if value, err = ops.Invoke(value, "join", []any{" "}, "value.join"); err != nil {
				return "", err
			}
		}
		attribute := escapegoat.HTMLEscape(entry.Key)
		if value != true {
			s, err := ops.ToString(value)
			if err != nil {
				return "", err
			}
			attribute += `="` + escapegoat.HTMLEscape(s) + `"`
		}
		handled = append(handled, attribute)
	}
	if len(handled) == 0 {
		return "", nil
	}
	return " " + strings.Join(handled, " "), nil
}
