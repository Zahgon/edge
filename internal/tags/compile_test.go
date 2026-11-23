package tags

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// testFile is the template path of the cases: it holds both path separators, which
// @pushOnceTo replaces in its source id, and a "/" that js-stringify escapes.
const testFile = `views\users/list.edge`

// compileCase is a template and what Edge's compileRaw makes of it.
type compileCase struct {
	name    string
	src     string
	async   bool
	migrate bool // the migrate plugin's tags are registered
	claim   bool // claimXTags claims the unknown tags
	want    []string
	err     string
}

// claimXTags claims the names starting with "x" and an upper-case letter as block
// tags with an argument, without registering a tag for them.
func claimXTags(name string) *parser.Tag {
	if len(name) > 1 && name[0] == 'x' && name[1] >= 'A' && name[1] <= 'Z' {
		return &parser.Tag{Seekable: true, Block: true}
	}
	return nil
}

// registry returns the tags of a new Edge instance, with the migrate plugin's tags
// when migrate is true.
func registry(migrate bool) map[string]*parser.Tag {
	tags := map[string]*parser.Tag{}
	for _, tag := range Builtin() {
		tags[tag.TagName] = tag
	}
	if migrate {
		for _, tag := range Migrate() {
			tags[tag.TagName] = tag
		}
	}
	return tags
}

// compileRaw compiles a template as Edge's Compiler.compileRaw does: a parser with
// the compiler's options and a new stack, and a buffer with its output variable and
// rethrow call. globalDefined reports the names globalThis defines.
func compileRaw(src, filename string, async, migrate bool, claim parser.ClaimTagFn, globalDefined func(string) bool) (string, error) {
	p := parser.New(registry(migrate), parser.NewStack(), parser.Options{
		ClaimTag:          claim,
		Async:             async,
		StatePropertyName: "state",
		EscapeCallPath:    []string{"template", "escape"},
		LocalVariables:    []string{"$filename", "state", "$context"},
		GlobalDefined:     globalDefined,
	})
	buffer := parser.NewEdgeBuffer(filename, parser.BufferOptions{OutputVar: "out", RethrowCallPath: []string{"template", "reThrow"}})
	tokens, err := p.Tokenize(src, filename)
	if err != nil {
		return "", err
	}
	for _, token := range tokens {
		if err := p.ProcessToken(token, buffer); err != nil {
			return "", err
		}
	}
	return buffer.Flush(), nil
}

// caseGlobals are the names of Node's globalThis the cases use.
var caseGlobals = map[string]bool{"Math": true, "JSON": true, "Object": true, "Date": true}

// errorText formats an error as the cases record it.
func errorText(t *testing.T, err error, filename string) string {
	t.Helper()
	var edgeError *edgeerror.EdgeError
	var jsError *js.Error
	switch {
	case errors.As(err, &edgeError):
		if edgeError.Filename != filename {
			t.Errorf("error filename %v, want %q", edgeError.Filename, filename)
		}
		return edgeError.Code + " " + js.ToString(edgeError.Line) + ":" + js.ToString(edgeError.Col) + " " + js.ToString(edgeError.Message)
	case errors.As(err, &jsError):
		return jsError.Name + ": " + jsError.Message
	}
	t.Fatalf("unexpected error %T %v", err, err)
	return ""
}

// stackIDs are the random variable names of @pushTo and @pushOnceTo.
var stackIDs = regexp.MustCompile(`stack_[A-Za-z0-9_]+`)

func TestCompile(t *testing.T) {
	for _, c := range compileCases {
		name := c.name
		if c.async {
			name += " (async)"
		}
		t.Run(name, func(t *testing.T) {
			var claim parser.ClaimTagFn
			if c.claim {
				claim = claimXTags
			}
			got, err := compileRaw(c.src, testFile, c.async, c.migrate, claim, func(name string) bool { return caseGlobals[name] })
			if c.err != "" {
				if err == nil {
					t.Fatalf("compiled\n%s\nwant error %s", got, c.err)
				}
				if text := errorText(t, err, testFile); text != c.err {
					t.Fatalf("error\n got %s\nwant %s", text, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %s", errorText(t, err, testFile))
			}
			want := strings.Join(append(append(append([]string{}, compiledPrologue...), c.want...), compiledEpilogue...), "\n")
			if got = stackIDs.ReplaceAllString(got, "stack_ID"); got != want {
				t.Errorf("compiled\n%s\nwant\n%s", got, want)
			}
		})
	}
}
