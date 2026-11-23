package strutil

import (
	"math"
	"testing"

	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// globals maps the names of Edge's string globals to their ports.
var globals = map[string]func(ops *jsval.Ops, args []any) (any, error){
	"camelCase":    JSCamelCase,
	"snakeCase":    JSSnakeCase,
	"dashCase":     JSDashCase,
	"pascalCase":   JSPascalCase,
	"capitalCase":  JSCapitalCase,
	"sentenceCase": JSSentenceCase,
	"dotCase":      JSDotCase,
	"noCase":       JSNoCase,
	"titleCase":    JSTitleCase,
	"pluralize":    JSPluralize,
	"sentence":     JSSentence,
	"prettyMs":     JSFormatMilliseconds,
	"toMs":         JSParseMilliseconds,
	"prettyBytes":  JSFormatBytes,
	"toBytes":      JSParseBytes,
	"ordinal":      JSOrdinal,
	"truncate":     JSTruncate,
	"excerpt":      JSExcerpt,
}

// describe renders a result as testdata/record.mjs does.
func describe(ops *jsval.Ops, v any) (string, error) {
	switch x := v.(type) {
	case string:
		return "string " + x, nil
	case float64:
		if x == 0 && math.Signbit(x) {
			return "number -0", nil
		}
		return "number " + js.ToString(x), nil
	}
	if !js.IsObject(v) {
		return js.TypeOf(v) + " " + js.ToString(v), nil
	}
	tag, err := ops.Tag(v)
	if err != nil {
		return "", err
	}
	json, _, err := ops.Realm.JSONStringify(v, js.Undefined, js.Undefined)
	return "object " + tag + " " + json, err
}

// TestRecorded replays testdata/globals.json.gz, recorded from Node 24 by
// testdata/record.mjs, in its order and in one realm, as the recorder ran it.
func TestRecorded(t *testing.T) {
	ops := jsvaltest.NewOps(t)
	cases := jsvaltest.Load(t, "testdata/globals.json.gz")
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	jsvaltest.Replay(t, cases, func(src string) (string, error) {
		pair, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		items := pair.(*js.Array).Items
		global := globals[items[0].(string)]
		out, err := global(ops, items[1].(*js.Array).Items)
		if err != nil {
			return "", err
		}
		return describe(ops, out)
	})
}
