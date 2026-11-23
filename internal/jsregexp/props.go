package jsregexp

import (
	"slices"
	"strings"
	"sync"

	"github.com/edge-js/edge-go/internal/jsregexp/uniprop"
)

// propFix lists the ranges, as lo, hi pairs, that Node 24 adds to or removes from
// a property set uniprop.Derived derives.
type propFix struct {
	add, del []rune
}

// stringPropertyValue is the value of a property of strings: the members that are
// one code point, and the longer ones.
type stringPropertyValue struct {
	set  charSet
	strs [][]rune
}

// stringProperties decodes stringPropertyMembers on first use into the properties
// of strings that V8 accepts in \p{...} with the v flag. RGI_Emoji is the union of
// the other six, as UTS #51 defines it.
var stringProperties = sync.OnceValue(func() map[string]stringPropertyValue {
	props := make(map[string]stringPropertyValue, len(stringPropertyMembers)+1)
	var all stringPropertyValue
	for name, members := range stringPropertyMembers {
		var v stringPropertyValue
		var pairs []rune
		for _, m := range strings.Split(members, " ") {
			if rs := []rune(m); len(rs) == 1 {
				pairs = append(pairs, rs[0], rs[0])
			} else {
				v.strs = append(v.strs, rs)
			}
		}
		v.set = newCharSet(pairs...)
		props[name] = v
		all.set = all.set.union(v.set)
		all.strs = append(all.strs, v.strs...)
	}
	props["RGI_Emoji"] = all
	return props
})

// stringProperty returns the code points and the longer strings of the property of
// strings name, reporting false when name is not one. The caller owns the returned
// list of strings but must not modify the strings themselves.
func stringProperty(name string) (charSet, [][]rune, bool) {
	v, ok := stringProperties()[name]
	if !ok {
		return nil, nil, false
	}
	return v.set, slices.Clone(v.strs), true
}

var (
	propMu    sync.Mutex
	propCache = map[string]charSet{}
)

// propertyKey resolves the body of \p{name} or \p{name=value} (value is "" for
// the first form) to its key ("gc=Lu", "bin=Alphabetic", "sc=Latin",
// "scx=Latin"), reporting false when V8 rejects the name.
func propertyKey(name, value string) (string, bool) {
	if value == "" {
		if gc, ok := gcAliases[name]; ok {
			return "gc=" + gc, true
		}
		if b, ok := binaryAliases[name]; ok {
			return "bin=" + b, true
		}
		return "", false
	}
	switch name {
	case "General_Category", "gc":
		if gc, ok := gcAliases[value]; ok {
			return "gc=" + gc, true
		}
	case "Script", "sc":
		if s, ok := scriptAliases[value]; ok {
			return "sc=" + s, true
		}
	case "Script_Extensions", "scx":
		if s, ok := scriptAliases[value]; ok {
			return "scx=" + s, true
		}
	}
	return "", false
}

// propertySet returns the code points of a property key, building and caching
// the set on first use.
func propertySet(key string) charSet {
	propMu.Lock()
	defer propMu.Unlock()
	if s, ok := propCache[key]; ok {
		return s
	}
	pred := uniprop.Derived(key)
	var pairs []rune
	start := rune(-1)
	for r := rune(0); r <= 0x110000; r++ {
		in := r <= 0x10FFFF && pred(r)
		switch {
		case in && start < 0:
			start = r
		case !in && start >= 0:
			pairs = append(pairs, start, r-1)
			start = -1
		}
	}
	s := charSet(pairs)
	if fix, ok := propFixes[key]; ok {
		s = s.union(newCharSet(slices.Clone(fix.add)...)).minus(newCharSet(slices.Clone(fix.del)...), 0x10FFFF)
	}
	propCache[key] = s
	return s
}
