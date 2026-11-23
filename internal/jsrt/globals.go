package jsrt

import (
	"slices"

	"github.com/edge-js/edge-go/js"
)

// GlobalDefined reports whether `globalThis[name] !== undefined` holds in a Node
// 24 ES module whose global object is this realm's, the test edge-parser applies
// to keep an identifier as it is instead of reading it from the template state.
//
// A name the realm's global object has, or inherits, answers by its value now,
// so globals that code assigned (`globalThis.x = 1`, or an assignment to an
// undeclared name in sloppy mode) count from then on, and one set to undefined
// does not. Node's other globals (process, Buffer, fetch, setTimeout, ...),
// which the interpreter does not implement, are recorded in nodeGlobals and
// count until code deletes them. An accessor counts as defined when it has a
// getter, without running it.
func (r *Realm) GlobalDefined(name string) bool {
	if slot, _, ok := r.findProperty(r.global, name); ok {
		if acc, isAcc := slot.(*js.Accessor); isAcc {
			return acc.Get != nil
		}
		return !isUndefined(slot)
	}
	if r.deletedGlobals[name] {
		return false
	}
	_, found := slices.BinarySearch(nodeGlobals[:], name)
	return found
}
