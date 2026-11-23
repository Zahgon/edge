package jsrt

import "github.com/edge-js/edge-go/js"

// parseAccessor parses a getter or setter of an object literal,
// "get name() {...}" or "set name(v) {...}", whose get or set is the current
// token. The function's source text starts at get or set, as V8 prints it.
func (p *parser) parseAccessor(binding bool) *property {
	pos := p.tok.pos
	kind := propGet
	if p.tok.text == "set" {
		kind = propSet
	}
	p.next()
	key, computed := p.parsePropertyName()
	if binding {
		p.fail("Invalid destructuring assignment target")
	}
	if !p.is("(") {
		p.unexpected(p.tok)
	}
	fi := p.parseFunctionRest(pos, key, true)
	p.checkAccessorParams(kind == propGet, fi)
	return &property{kind: kind, key: key, computed: computed, value: &funcLit{base{pos}, fi}, pos: pos}
}

// checkAccessorParams applies the early errors of a getter's or setter's
// parameter list, in an object literal or a class.
func (p *parser) checkAccessorParams(getter bool, fi *funcInfo) {
	switch {
	case getter && (len(fi.params) > 0 || fi.rest != nil):
		p.fail("Getter must not have any formal parameters.")
	case !getter && fi.rest != nil:
		p.fail("Setter function argument must not be a rest parameter")
	case !getter && len(fi.params) != 1:
		p.fail("Setter must have exactly one formal parameter.")
	}
}

// accessorPrefix is the prefix of a getter's or setter's function name.
func accessorPrefix(getter bool) string {
	if getter {
		return "get "
	}
	return "set "
}

// defineAccessor installs the getter or setter of an object literal on o,
// keeping the other half of an accessor already defined there; it replaces a
// data property of the same name.
func defineAccessor(o *js.Object, key string, kind propKind, f *js.Function) {
	acc, ok := o.Get(key).(*js.Accessor)
	if !ok || !o.Has(key) {
		acc = &js.Accessor{}
	} else {
		acc = &js.Accessor{Get: acc.Get, Set: acc.Set}
	}
	if kind == propGet {
		acc.Get = f
	} else {
		acc.Set = f
	}
	o.Define(key, acc, 0)
}
