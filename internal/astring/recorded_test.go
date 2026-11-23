package astring

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// recordedCase is one input of testdata/astring.json.gz with astring 1.9.0's
// output under Node (see testdata/record.mjs). Strings are UTF-16 code units, so
// that unpaired surrogates survive JSON. For each output the recording holds the
// text or, in the field with the Error suffix, the message of what was thrown.
type recordedCase struct {
	Src        []uint16 `json:"src"`
	Ecma       int      `json:"ecma"`
	Await      bool     `json:"await"`
	Gen        []uint16 `json:"gen"`
	GenError   []uint16 `json:"genError"`
	Regen      []uint16 `json:"regen"`
	RegenError []uint16 `json:"regenError"`
	Bare       []uint16 `json:"bare"`
	BareError  []uint16 `json:"bareError"`
}

func TestRecorded(t *testing.T) {
	paths, err := filepath.Glob("testdata/astring*.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no recordings in testdata")
	}
	total, failed := 0, 0
	for _, path := range paths {
		var cases []recordedCase
		if err := fixture.DecodeGzipJSON(path, &cases); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for i, c := range cases {
			total++
			if msg := c.check(); msg != "" {
				failed++
				if failed <= 30 {
					t.Errorf("%s #%d (ecmaVersion %d) %q: %s", filepath.Base(path), i, c.Ecma, jsstr.FromUTF16(c.Src), msg)
				}
			}
		}
	}
	t.Logf("%d recorded cases, %d mismatches", total, failed)
}

func (c *recordedCase) options() acorn.Options {
	return acorn.Options{EcmaVersion: c.Ecma, Locations: true, AllowAwaitOutsideFunction: c.Await}
}

// outcome is a recorded output: the text, or "throws " and the message.
func outcome(text, errorMessage []uint16) string {
	if errorMessage != nil {
		return "throws " + jsstr.FromUTF16(errorMessage)
	}
	return jsstr.FromUTF16(text)
}

// generated is Generate's output in the form of outcome.
func generated(program *acorn.Node) string {
	code, err := Generate(program)
	if err != nil {
		return "throws " + err.Error()
	}
	return code
}

func (c *recordedCase) check() string {
	program, err := acorn.Parse(jsstr.FromUTF16(c.Src), c.options())
	if err != nil {
		return "parse: " + err.Error()
	}
	gen := generated(program)
	if want := outcome(c.Gen, c.GenError); gen != want {
		return fmt.Sprintf("generate:\n got  %q\n want %q", gen, want)
	}
	regen := ""
	if reparsed, err := acorn.Parse(gen, c.options()); err != nil {
		regen = "throws " + err.Error()
	} else {
		regen = generated(reparsed)
	}
	if want := outcome(c.Regen, c.RegenError); regen != want {
		return fmt.Sprintf("generate(parse(generated)):\n got  %q\n want %q", regen, want)
	}
	stripRawAndShorthand(program)
	if bare, want := generated(program), outcome(c.Bare, c.BareError); bare != want {
		return fmt.Sprintf("generate(without raw):\n got  %q\n want %q", bare, want)
	}
	return ""
}

var nodeType = reflect.TypeOf((*acorn.Node)(nil))

// stripRawAndShorthand removes the raw text of every Literal and makes every
// Property non-shorthand, as the recorder does to acorn's tree to have astring
// print nodes the way it prints nodes built by hand.
func stripRawAndShorthand(n *acorn.Node) {
	if n == nil {
		return
	}
	if n.Type == "Literal" {
		n.Raw = ""
	}
	if n.Type == "Property" {
		n.Shorthand = false
	}
	v := reflect.ValueOf(n).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch {
		case f.Type() == nodeType:
			stripRawAndShorthand(f.Interface().(*acorn.Node))
		case f.Kind() == reflect.Slice && f.Type().Elem() == nodeType:
			for _, child := range f.Interface().([]*acorn.Node) {
				stripRawAndShorthand(child)
			}
		}
	}
}
