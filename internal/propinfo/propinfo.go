// Package propinfo is property-information 7.2.0 as Edge uses it: the merged
// html schema (aria, html, xlink, xmlns and xml) with the Alpine.js entries
// Edge's definePropertyInformation adds to it when its module loads, and find,
// which html.attrs and $props.toAttrs call to name each attribute.
package propinfo

import (
	"slices"
	"strings"
	"sync"

	"github.com/edge-js/edge-go/internal/jsval"
)

// Info is the information find returns about a property: the fields of
// lib/util/info.js's Info (false and "" where Info.prototype supplies the
// default, Space "" for undefined).
type Info struct {
	Property              string
	Attribute             string
	Space                 string
	Boolean               bool
	Booleanish            bool
	OverloadedBoolean     bool
	Number                bool
	SpaceSeparated        bool
	CommaSeparated        bool
	CommaOrSpaceSeparated bool
	// Defined is set for the properties of a schema and for data attributes
	// (DefinedInfo), clear for unknown names and Edge's additions.
	Defined         bool
	MustUseProperty bool
}

// schema is lib/util/schema.js's Schema: the infos by property name, and the
// property name by normalized property or attribute name.
type schema struct {
	property map[string]*Info
	normal   map[string]string
}

// create is lib/util/create.js's create(definition).
func create(def definition) schema {
	s := schema{property: map[string]*Info{}, normal: map[string]string{}}
	attributes := def.attributes
	if attributes == nil {
		attributes = map[string]string{}
	}
	for _, p := range def.properties {
		info := &Info{
			Property:              p.name,
			Attribute:             def.transform(attributes, p.name),
			Space:                 def.space,
			Defined:               true,
			Boolean:               p.mask&boolean != 0,
			Booleanish:            p.mask&booleanish != 0,
			OverloadedBoolean:     p.mask&overloadedBoolean != 0,
			Number:                p.mask&number != 0,
			SpaceSeparated:        p.mask&spaceSeparated != 0,
			CommaSeparated:        p.mask&commaSeparated != 0,
			CommaOrSpaceSeparated: p.mask&commaOrSpaceSeparated != 0,
			MustUseProperty:       slices.Contains(def.mustUseProperty, p.name),
		}
		s.property[p.name] = info
		s.normal[normalize(p.name)] = p.name
		s.normal[normalize(info.Attribute)] = p.name
	}
	return s
}

// merge is lib/util/merge.js's merge(definitions): later definitions win.
func merge(definitions ...definition) schema {
	merged := schema{property: map[string]*Info{}, normal: map[string]string{}}
	for _, def := range definitions {
		s := create(def)
		for k, v := range s.property {
			merged.property[k] = v
		}
		for k, v := range s.normal {
			merged.normal[k] = v
		}
	}
	return merged
}

// htmlSchema is the html export of index.js, merge([aria, html, xlink, xmlns,
// xml], 'html'), after Edge's src/utils.ts has run definePropertyInformation
// for x-cloak, x-ignore and the x-transition enter/leave classes.
var htmlSchema = sync.OnceValue(func() schema {
	s := merge(aria, html, xlink, xmlns, xml)
	define := func(property string, info Info) {
		s.normal[property] = property
		s.property[property] = &info
	}
	define("x-cloak", Info{Attribute: "x-cloak", Property: "x-cloak", Boolean: true, Space: "html"})
	define("x-ignore", Info{Attribute: "x-ignore", Property: "x-ignore", Boolean: true, Space: "html"})
	for _, transition := range []struct{ normal, attribute, property string }{
		{"x-transition:enterstart", "x-transition:enter-start", "x-transition:enterStart"},
		{"x-transition:enterend", "x-transition:enter-end", "x-transition:enterEnd"},
		{"x-transition:leavestart", "x-transition:leave-start", "x-transition:leaveStart"},
		{"x-transition:leaveend", "x-transition:leave-end", "x-transition:leaveEnd"},
	} {
		define(transition.normal, Info{
			Attribute:             transition.attribute,
			Property:              transition.property,
			Space:                 "html",
			SpaceSeparated:        true,
			CommaOrSpaceSeparated: true,
		})
	}
	return s
})

// normalize is lib/normalize.js's normalize(value): value.toLowerCase().
func normalize(value string) string { return jsval.ToLowerCase(value) }

// isValidData is find.js's valid.test(value), /^data[-\w.:]+$/i: "data" in
// any case followed by at least one ASCII letter, digit, "_", "-", "." or ":".
func isValidData(value string) bool {
	if len(value) <= 4 || !strings.EqualFold(value[:4], "data") {
		return false
	}
	for i := 4; i < len(value); i++ {
		c := value[i]
		if !isDashLetter(c) && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && !strings.ContainsRune("_-.:", rune(c)) {
			return false
		}
	}
	return true
}

// isDashLetter reports whether c is in [a-z], the letter of find.js's dash
// expression /-[a-z]/g.
func isDashLetter(c byte) bool { return c >= 'a' && c <= 'z' }

// dashLastIndex is the lastIndex of find.js's global dash expression, shared
// by every find in the process: find tests it with dash.test(rest), which
// leaves lastIndex after a match, so the next test starts its search there.
var dashLastIndex struct {
	sync.Mutex
	n int
}

// testDash is dash.test(rest), starting at and updating dash's lastIndex.
func testDash(rest string) bool {
	dashLastIndex.Lock()
	defer dashLastIndex.Unlock()
	for i := dashLastIndex.n; i+1 < len(rest); i++ {
		if rest[i] == '-' && isDashLetter(rest[i+1]) {
			dashLastIndex.n = i + 2
			return true
		}
	}
	dashLastIndex.n = 0
	return false
}

// camelDashes is rest.replace(dash, camelcase): every "-" followed by a
// lowercase letter becomes that letter uppercased. A replace with the global
// dash expression leaves its lastIndex at 0.
func camelDashes(rest string) string {
	dashLastIndex.Lock()
	dashLastIndex.n = 0
	dashLastIndex.Unlock()
	var b strings.Builder
	for i := 0; i < len(rest); i++ {
		if rest[i] == '-' && i+1 < len(rest) && isDashLetter(rest[i+1]) {
			b.WriteByte(rest[i+1] - 'a' + 'A')
			i++
			continue
		}
		b.WriteByte(rest[i])
	}
	return b.String()
}

// kebabCaps is rest.replace(cap, kebab): every uppercase letter becomes "-"
// and the letter lowercased.
func kebabCaps(rest string) string {
	var b strings.Builder
	for i := 0; i < len(rest); i++ {
		if c := rest[i]; c >= 'A' && c <= 'Z' {
			b.WriteByte('-')
			b.WriteByte(c - 'A' + 'a')
			continue
		}
		b.WriteByte(rest[i])
	}
	return b.String()
}

// Find is find(html, name): the info of the property or attribute name in the
// html schema (looked up by its lowercase form); for a valid data attribute or
// property (data-foo-bar or dataFooBar) a defined info with both spellings;
// for any other name an info whose property and attribute are name itself. ok
// is false when find returns undefined, as it does for the names that lowercase
// to "constructor" or "__proto__": find tests `normal in schema.normal`, which
// those inherited keys pass, and finds no info under the key they read as.
func Find(name string) (info Info, ok bool) {
	s := htmlSchema()
	normal := normalize(name)
	if normal == "constructor" || normal == "__proto__" {
		return Info{}, false
	}
	if property, found := s.normal[normal]; found {
		return *s.property[property], true
	}
	// find also requires normal to start with "data" and be longer than four
	// code units, which every valid data name does.
	if !isValidData(name) {
		return Info{Property: name, Attribute: name}, true
	}
	property, attribute := name, name
	if name[4] == '-' {
		rest := camelDashes(name[5:])
		if rest != "" {
			rest = strings.ToUpper(rest[:1]) + rest[1:]
		}
		property = "data" + rest
	} else if rest := name[4:]; !testDash(rest) {
		dashes := kebabCaps(rest)
		if !strings.HasPrefix(dashes, "-") {
			dashes = "-" + dashes
		}
		attribute = "data" + dashes
	}
	return Info{Property: property, Attribute: attribute, Defined: true}, true
}
