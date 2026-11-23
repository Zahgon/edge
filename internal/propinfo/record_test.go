package propinfo

import (
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/jsval/jsvaltest"
	"github.com/edge-js/edge-go/js"
)

// describe renders an info as testdata/record.mjs does: property, attribute,
// space ("undefined" when there is none) and the names of the flags set.
func describe(info Info, ok bool) string {
	if !ok {
		return "undefined"
	}
	space := info.Space
	if space == "" {
		space = "undefined"
	}
	parts := []string{info.Property, info.Attribute, space}
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"boolean", info.Boolean}, {"booleanish", info.Booleanish}, {"overloadedBoolean", info.OverloadedBoolean},
		{"number", info.Number}, {"spaceSeparated", info.SpaceSeparated}, {"commaSeparated", info.CommaSeparated},
		{"commaOrSpaceSeparated", info.CommaOrSpaceSeparated}, {"defined", info.Defined},
		{"mustUseProperty", info.MustUseProperty},
	} {
		if f.set {
			parts = append(parts, f.name)
		}
	}
	return strings.Join(parts, "|")
}

// TestRecorded replays testdata/find.json.gz, recorded from Node 24 by
// testdata/record.mjs, in its order: find's dash expression keeps its
// lastIndex from one call to the next.
func TestRecorded(t *testing.T) {
	resetDash()
	ops := jsvaltest.NewOps(t)
	cases := jsvaltest.Load(t, "testdata/find.json.gz")
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	jsvaltest.Replay(t, cases, func(src string) (string, error) {
		name, err := jsvaltest.Eval(ops.Realm, src)
		if err != nil {
			return "", err
		}
		return describe(Find(js.ToString(name))), nil
	})
}
