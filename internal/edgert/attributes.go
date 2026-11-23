package edgert

import (
	"strings"

	"github.com/edge-js/edge-go/internal/classnames"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/internal/propinfo"
	"github.com/edge-js/edge-go/js"
)

// alpineNamespaces are the keys whose object values are serialized as
// attributes with a prefix: { x: { data: '{}' } } is x-data="{}".
var alpineNamespaces = map[string]string{
	"x":           "x-",
	"xOn":         "x-on:",
	"xBind":       "x-bind:",
	"xTransition": "x-transition:",
}

// alpineNamespace is alpineNamespaces[key], read from a plain object: the
// prefix of a namespace key, or what the object inherits under the key.
func alpineNamespace(key string) (any, error) {
	if prefix, ok := alpineNamespaces[key]; ok {
		return prefix, nil
	}
	return InheritedProperty(key)
}

// InheritedProperty is what a plain object without own properties holds
// under key: the value of an Object.prototype property (Object.prototype
// itself for __proto__), undefined for other keys. The plain objects Edge
// uses as lookup tables find these for keys such as "constructor".
func InheritedProperty(key string) (any, error) {
	if !parser.IsObjectPrototypeKey(key) {
		return js.Undefined, nil
	}
	return Realm().Get(js.NewObject(), key)
}

// StringifyAttributes is Edge's stringifyAttributes(props), behind
// html.attrs and $props.toAttrs: the truthy props as HTML attributes named by
// property-information, separated by spaces. true values are bare
// attributes, class values go through classnames, arrays are joined with
// spaces (commas for comma-separated attributes), other values are
// converted with String(). Values are not escaped. Object values of the
// Alpine.js namespace keys (x, xOn, xBind, xTransition) are serialized with
// the namespace's prefix.
func StringifyAttributes(props any) (string, error) {
	return stringifyAttributes(props, js.Undefined)
}

func stringifyAttributes(props, namespace any) (string, error) {
	keys, err := Realm().OwnEnumerableKeys(props)
	if err != nil || len(keys) == 0 {
		return "", err
	}
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		value, err := Realm().Get(props, key)
		if err != nil {
			return "", err
		}
		if js.ToBoolean(namespace) {
			prefix, err := Realm().ToString(namespace)
			if err != nil {
				return "", err
			}
			key = prefix + key
		}
		if !js.ToBoolean(value) {
			continue
		}
		prefix, err := alpineNamespace(key)
		if err != nil {
			return "", err
		}
		if js.ToBoolean(prefix) && js.TypeOf(value) == "object" {
			nested, err := stringifyAttributes(value, prefix)
			if err != nil {
				return "", err
			}
			result = append(result, nested)
			continue
		}
		info, ok := propinfo.Find(key)
		if !ok {
			continue
		}
		if value == true {
			result = append(result, info.Attribute)
			continue
		}
		text, err := attributeValue(key, value, info)
		if err != nil {
			return "", err
		}
		result = append(result, info.Attribute+`="`+text+`"`)
	}
	return strings.Join(result, " "), nil
}

// attributeValue is the text between the quotes of an attribute.
func attributeValue(key string, value any, info propinfo.Info) (string, error) {
	if key == "class" {
		return classnames.ClassNames(Ops(), []any{value})
	}
	isArray, err := Ops().IsArray(value)
	if err != nil {
		return "", err
	}
	if !isArray {
		return Realm().ToString(value)
	}
	separator := " "
	if info.CommaSeparated {
		separator = ","
	}
	joined, err := Ops().Invoke(value, "join", []any{separator}, "value.join")
	if err != nil {
		return "", err
	}
	return Realm().ToString(joined)
}
