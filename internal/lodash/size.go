package lodash

import (
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// The patterns lodash's size uses to count the symbols of a string: reHasUnicode
// finds strings that need counting, reUnicode matches one symbol (a surrogate
// pair, a regional-indicator pair, a character with its combining marks,
// variation selectors and zero-width-joiner sequences).
const (
	rsCombo   = `[̀-ͯ︠-︯⃐-⃿]`
	rsFitz    = `\ud83c[\udffb-\udfff]`
	rsVarSel  = `[︎️]?`
	rsSymbol  = `(?:[^\ud800-\udfff]` + rsCombo + `?|` + rsCombo + `|(?:\ud83c[\udde6-\uddff]){2}|[\ud800-\udbff][\udc00-\udfff]|[\ud800-\udfff])`
	rsSeqTail = rsVarSel + `(?:` + rsCombo + `|` + rsFitz + `)?(?:‍(?:[^\ud800-\udfff]|(?:\ud83c[\udde6-\uddff]){2}|[\ud800-\udbff][\udc00-\udfff])` + rsVarSel + `(?:` + rsCombo + `|` + rsFitz + `)?)*`
)

var (
	reUnicode    = jsval.MustCompile(rsFitz+`(?=`+rsFitz+`)|`+rsSymbol+rsSeqTail, "g")
	reHasUnicode = jsval.MustCompile(`[‍\ud800-\udfff̀-ͯ︠-︯⃐-⃿︎️]`, "")
)

// Size is _.size(collection): 0 for null and undefined; the length of an
// array-like value, strings (and String objects) counted in symbols rather than
// code units; a Map's or Set's size; otherwise the number of own enumerable
// keys (without constructor for a prototype object). The result is the value
// lodash reads, normally a number.
func Size(ops *jsval.Ops, collection any) (any, error) {
	if js.IsNullish(collection) {
		return 0.0, nil
	}
	e, err := newEnv(ops)
	if err != nil {
		return nil, err
	}
	arrayLike, err := e.isArrayLike(collection)
	if err != nil {
		return nil, err
	}
	if arrayLike {
		isStr, err := e.isString(collection)
		if err != nil {
			return nil, err
		}
		if !isStr {
			return e.length(collection)
		}
		s, err := ops.ToString(collection)
		if err != nil {
			return nil, err
		}
		if !reHasUnicode.Test(s) {
			return e.length(collection)
		}
		return e.countSymbols(collection)
	}
	tag, err := e.tag(collection)
	if err != nil {
		return nil, err
	}
	if tag == "[object Map]" || tag == "[object Set]" {
		return ops.Get(collection, "size")
	}
	keys, err := e.baseKeys(collection)
	if err != nil {
		return nil, err
	}
	return float64(len(keys)), nil
}

// isString is lodash's isString: a string, or an object tagged
// "[object String]" that is not an array.
func (e *env) isString(v any) (bool, error) {
	if _, ok := v.(string); ok {
		return true, nil
	}
	if isArr, err := e.isArray(v); err != nil || isArr || !isObjectLike(v) {
		return false, err
	}
	t, err := e.tag(v)
	return t == "[object String]", err
}

// countSymbols is lodash's unicodeSize: the number of times the global
// reUnicode's test succeeds on the string, starting from lastIndex 0, each test
// converting the value to a string again.
func (e *env) countSymbols(v any) (float64, error) {
	count, lastIndex := 0.0, 0
	for {
		s, err := e.ops.ToString(v)
		if err != nil {
			return 0, err
		}
		units := jsstr.ToUTF16(s)
		caps := reUnicode.Exec(units, lastIndex)
		if caps == nil {
			return count, nil
		}
		lastIndex = caps[1]
		count++
	}
}
