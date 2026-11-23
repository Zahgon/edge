package tags

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/fixture"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// jsString is a string of the recording: a JSON string, or { "units": [...] } for a
// string that is not well-formed UTF-16.
type jsString string

func (s *jsString) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*s = jsString(text)
		return nil
	}
	var units struct{ Units []uint16 }
	if err := json.Unmarshal(data, &units); err != nil {
		return err
	}
	*s = jsString(jsstr.FromUTF16(units.Units))
	return nil
}

// recordedTemplate is a recorded template (testdata/record_templates.mjs).
type recordedTemplate struct {
	Src      jsString
	Claim    bool
	Migrate  bool
	Async    bool
	Tokens   string
	Compiled *jsString
	Error    *struct {
		Code     string
		Name     string
		Message  jsString
		Filename string
		Line     float64
		Col      float64
	}
}

// claimRecordedTag is the claimTag function of the recording: x<Upper> and comp.<...>
// names are block tags with an argument, inl<...> inline tags without one that
// drop their line break, sk<...> inline tags with an argument.
func claimRecordedTag(name string) *parser.Tag {
	switch {
	case claimXTags(name) != nil || strings.HasPrefix(name, "comp."):
		return &parser.Tag{Seekable: true, Block: true}
	case strings.HasPrefix(name, "inl"):
		return &parser.Tag{NoNewLine: true}
	case strings.HasPrefix(name, "sk"):
		return &parser.Tag{Seekable: true}
	}
	return nil
}

// TestRecordedTemplates tokenizes and compiles the recorded templates and
// compares the tokens, the compiled code and the errors with edge.js.
func TestRecordedTemplates(t *testing.T) {
	var recording struct {
		File    string
		Globals []string
		Cases   []recordedTemplate
	}
	if err := fixture.DecodeGzipJSON("testdata/templates.json.gz", &recording); err != nil {
		t.Fatal(err)
	}
	globals := map[string]bool{}
	for _, name := range recording.Globals {
		globals[name] = true
	}
	failures := 0
	for i, c := range recording.Cases {
		if msg := checkRecordedTemplate(c, recording.File, func(name string) bool { return globals[name] }); msg != "" {
			failures++
			if failures <= 10 {
				t.Errorf("case %d (claim=%v migrate=%v async=%v) %q\n%s", i, c.Claim, c.Migrate, c.Async, c.Src, msg)
			}
		}
	}
	t.Logf("%d templates, %d mismatches", len(recording.Cases), failures)
}

// checkRecordedTemplate returns what differs from the recording, "" when nothing does.
func checkRecordedTemplate(c recordedTemplate, file string, globalDefined func(string) bool) string {
	registry := registry(c.Migrate)
	options := lexer.Options{Filename: file}
	if c.Claim {
		options.ClaimTag = func(name string) (lexer.TagDef, bool) {
			if tag := claimRecordedTag(name); tag != nil {
				return lexer.TagDef{Seekable: tag.Seekable, Block: tag.Block, NoNewLine: tag.NoNewLine}, true
			}
			return lexer.TagDef{}, false
		}
	}
	lookup := func(name string) (lexer.TagDef, bool) {
		if tag, ok := registry[name]; ok {
			return lexer.TagDef{Seekable: tag.Seekable, Block: tag.Block, NoNewLine: tag.NoNewLine}, true
		}
		return lexer.TagDef{}, parser.IsObjectPrototypeKey(name)
	}
	tokenizer := lexer.NewTokenizer(string(c.Src), lookup, options)
	// The tokens read before an error are compared as well; the error itself is
	// compared as compileRaw throws it.
	if err := tokenizer.Parse(); err != nil && c.Error == nil {
		return "tokenizer error " + recordedErrorText(err)
	}
	if got := tokensJSON(tokenizer.Tokens); got != c.Tokens {
		return "tokens\n got " + got + "\nwant " + c.Tokens
	}

	var claim parser.ClaimTagFn
	if c.Claim {
		claim = claimRecordedTag
	}
	got, err := compileRaw(string(c.Src), file, c.Async, c.Migrate, claim, globalDefined)
	switch {
	case c.Error != nil && err == nil:
		return "compiled\n" + got + "\nwant an error"
	case c.Error != nil:
		want := c.Error.Name + ": " + string(c.Error.Message)
		if c.Error.Code != "" {
			want = fmt.Sprintf("%s %s %v:%v %s", c.Error.Code, c.Error.Filename, c.Error.Line, c.Error.Col, c.Error.Message)
		}
		if text := recordedErrorText(err); text != want {
			return "error\n got " + text + "\nwant " + want
		}
	case err != nil:
		return "error " + recordedErrorText(err)
	case stackIDs.ReplaceAllString(got, "stack_ID") != string(*c.Compiled):
		return "compiled\n" + stackIDs.ReplaceAllString(got, "stack_ID") + "\nwant\n" + string(*c.Compiled)
	}
	return ""
}

// recordedErrorText formats an error as checkRecordedTemplate compares it.
func recordedErrorText(err error) string {
	var edgeError *edgeerror.EdgeError
	var jsError *js.Error
	switch {
	case errors.As(err, &edgeError):
		return fmt.Sprintf("%s %v %v:%v %s", edgeError.Code, edgeError.Filename, edgeError.Line, edgeError.Col, js.ToString(edgeError.Message))
	case errors.As(err, &jsError):
		return jsError.Name + ": " + jsError.Message
	}
	return fmt.Sprintf("%T: %v", err, err)
}

// jsonString is JSON.stringify of a string.
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	u := jsstr.ToUTF16(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(byte(c))
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			fmt.Fprintf(&b, `\u%04x`, c)
		case c >= 0xD800 && c < 0xDC00 && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] < 0xE000:
			b.WriteString(jsstr.FromUTF16(u[i : i+2]))
			i++
		case c >= 0xD800 && c < 0xE000:
			fmt.Fprintf(&b, `\u%04x`, c)
		default:
			b.WriteString(jsstr.FromUTF16(u[i : i+1]))
		}
	}
	b.WriteByte('"')
	return b.String()
}

func locJSON(l lexer.LexerLoc) string {
	return fmt.Sprintf(`{"start":{"line":%d,"col":%d},"end":{"line":%d,"col":%d}}`, l.Start.Line, l.Start.Col, l.End.Line, l.End.Col)
}

// tokensJSON is JSON.stringify of edge-lexer's tokens.
func tokensJSON(tokens []lexer.Token) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		switch t := token.(type) {
		case *lexer.TagToken:
			parts[i] = `{"type":` + jsonString(t.Type) + `,"filename":` + jsonString(t.Filename) +
				`,"properties":{"name":` + jsonString(t.Properties.Name) + `,"jsArg":` + jsonString(t.Properties.JsArg) +
				`,"selfclosed":` + strconv.FormatBool(t.Properties.Selfclosed) + `},"loc":` + locJSON(t.Loc) +
				`,"children":` + tokensJSON(t.Children) + `}`
		case *lexer.MustacheToken:
			parts[i] = `{"type":` + jsonString(t.Type) + `,"filename":` + jsonString(t.Filename) +
				`,"properties":{"jsArg":` + jsonString(t.Properties.JsArg) + `},"loc":` + locJSON(t.Loc) + `}`
		case *lexer.RawToken:
			parts[i] = `{"type":` + jsonString(t.Type) + `,"value":` + jsonString(t.Value) + `,"filename":` + jsonString(t.Filename) +
				`,"line":` + strconv.Itoa(t.Line) + `}`
		case *lexer.NewLineToken:
			parts[i] = `{"type":` + jsonString(t.Type) + `,"filename":` + jsonString(t.Filename) + `,"line":` + strconv.Itoa(t.Line) + `}`
		case *lexer.CommentToken:
			parts[i] = `{"type":` + jsonString(t.Type) + `,"filename":` + jsonString(t.Filename) + `,"value":` + jsonString(t.Value) +
				`,"loc":` + locJSON(t.Loc) + `}`
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}
