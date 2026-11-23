package propinfo

import "testing"

// The expected values were printed by property-information 7.2.0's
// find(html, name) on Node 24, after Edge's module had loaded.
func TestFind(t *testing.T) {
	for _, c := range []struct {
		name string
		want Info
	}{
		{"className", Info{Property: "className", Attribute: "class", Space: "html", SpaceSeparated: true, Defined: true}},
		{"class", Info{Property: "className", Attribute: "class", Space: "html", SpaceSeparated: true, Defined: true}},
		{"htmlFor", Info{Property: "htmlFor", Attribute: "for", Space: "html", SpaceSeparated: true, Defined: true}},
		{"acceptCharset", Info{Property: "acceptCharset", Attribute: "accept-charset", Space: "html", SpaceSeparated: true, Defined: true}},
		{"accept", Info{Property: "accept", Attribute: "accept", Space: "html", CommaSeparated: true, Defined: true}},
		{"coords", Info{Property: "coords", Attribute: "coords", Space: "html", Number: true, CommaSeparated: true, Defined: true}},
		{"checked", Info{Property: "checked", Attribute: "checked", Space: "html", Boolean: true, Defined: true, MustUseProperty: true}},
		{"download", Info{Property: "download", Attribute: "download", Space: "html", OverloadedBoolean: true, Defined: true}},
		{"value", Info{Property: "value", Attribute: "value", Space: "html", Booleanish: true, Defined: true}},
		{"onClick", Info{Property: "onClick", Attribute: "onclick", Space: "html", Defined: true}},
		{"tabIndex", Info{Property: "tabIndex", Attribute: "tabindex", Space: "html", Number: true, Defined: true}},
		{"data-foo-bar", Info{Property: "dataFooBar", Attribute: "data-foo-bar", Defined: true}},
		{"data-", Info{Property: "data", Attribute: "data-", Defined: true}},
		{"ariaLabel", Info{Property: "ariaLabel", Attribute: "aria-label", Defined: true}},
		{"aria-describedby", Info{Property: "ariaDescribedBy", Attribute: "aria-describedby", SpaceSeparated: true, Defined: true}},
		{"role", Info{Property: "role", Attribute: "role", Defined: true}},
		{"x-cloak", Info{Property: "x-cloak", Attribute: "x-cloak", Space: "html", Boolean: true}},
		{"x-transition:enterStart", Info{Property: "x-transition:enterStart", Attribute: "x-transition:enter-start", Space: "html", SpaceSeparated: true, CommaOrSpaceSeparated: true}},
		{"x-transition:enter-start", Info{Property: "x-transition:enter-start", Attribute: "x-transition:enter-start"}},
		{"xLinkHref", Info{Property: "xLinkHref", Attribute: "xlink:href", Space: "xlink", Defined: true}},
		{"xml:lang", Info{Property: "xmlLang", Attribute: "xml:lang", Space: "xml", Defined: true}},
		{"xmlnsXLink", Info{Property: "xmlnsXLink", Attribute: "xmlns:xlink", Space: "xmlns", Defined: true}},
		{"unknownThing", Info{Property: "unknownThing", Attribute: "unknownThing"}},
	} {
		if got, ok := Find(c.name); !ok || got != c.want {
			t.Errorf("Find(%q) = %+v, %v; want %+v", c.name, got, ok, c.want)
		}
	}
	for _, name := range []string{"constructor", "__proto__", "CONSTRUCTOR"} {
		if got, ok := Find(name); ok {
			t.Errorf("Find(%q) = %+v; want undefined", name, got)
		}
	}
}

// find's dash expression is global: a data property name with a dash after
// "data" leaves its lastIndex behind, and the next such name is searched from
// there.
func TestFindDashState(t *testing.T) {
	resetDash()
	for _, c := range []struct{ name, attribute string }{
		{"dataFoo-bar", "dataFoo-bar"},
		{"dataX-y", "data-x-y"},
		{"dataX-y", "dataX-y"},
	} {
		if got, _ := Find(c.name); got.Attribute != c.attribute {
			t.Errorf("Find(%q).Attribute = %q, want %q", c.name, got.Attribute, c.attribute)
		}
	}
}

// resetDash puts the dash expression's lastIndex back to 0, where it is when
// the module loads.
func resetDash() {
	dashLastIndex.Lock()
	dashLastIndex.n = 0
	dashLastIndex.Unlock()
}
