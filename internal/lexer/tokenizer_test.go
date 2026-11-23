package lexer

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/js"
)

// tokenizerCase is a template and the tokens edge-lexer makes of it.
type tokenizerCase struct {
	name  string
	src   string
	claim bool
	want  []Token
	err   string
}

// edgeTags are the definitions of Edge's built-in tags.
var edgeTags = map[string]TagDef{
	"assign": {Seekable: true, NoNewLine: true}, "component": {Block: true, Seekable: true}, "debugger": {NoNewLine: true},
	"each": {Block: true, Seekable: true}, "else": {}, "elseif": {Seekable: true}, "eval": {Seekable: true, NoNewLine: true},
	"if": {Block: true, Seekable: true}, "include": {Seekable: true}, "includeIf": {Seekable: true},
	"inject": {Seekable: true, NoNewLine: true}, "let": {Seekable: true, NoNewLine: true},
	"newError": {Seekable: true, NoNewLine: true}, "pushOnceTo": {Block: true, Seekable: true, NoNewLine: true},
	"pushTo": {Block: true, Seekable: true, NoNewLine: true}, "slot": {Block: true, Seekable: true, NoNewLine: true},
	"stack": {Seekable: true}, "unless": {Block: true, Seekable: true},
}

// objectPrototypeKeys are the names Edge's tags object inherits from
// Object.prototype: tags without any of the tag properties.
var objectPrototypeKeys = []string{
	"constructor", "__defineGetter__", "__defineSetter__", "hasOwnProperty", "__lookupGetter__", "__lookupSetter__",
	"isPrototypeOf", "propertyIsEnumerable", "toString", "valueOf", "__proto__", "toLocaleString",
}

// lookupEdgeTag is `tags[name]` on Edge's tags object.
func lookupEdgeTag(name string) (TagDef, bool) {
	if def, ok := edgeTags[name]; ok {
		return def, true
	}
	for _, key := range objectPrototypeKeys {
		if key == name {
			return TagDef{}, true
		}
	}
	return TagDef{}, false
}

// claimTestTag claims x<Upper> names as block tags with an argument, inl<...>
// names as inline tags without one that drop their line break and sk<...> names as
// inline tags with an argument.
func claimTestTag(name string) (TagDef, bool) {
	switch {
	case len(name) > 1 && name[0] == 'x' && name[1] >= 'A' && name[1] <= 'Z':
		return TagDef{Seekable: true, Block: true}, true
	case strings.HasPrefix(name, "inl"):
		return TagDef{NoNewLine: true}, true
	case strings.HasPrefix(name, "sk"):
		return TagDef{Seekable: true}, true
	}
	return TagDef{}, false
}

func loc(startLine, startCol, endLine, endCol int) LexerLoc {
	return LexerLoc{Start: LexerPosition{Line: startLine, Col: startCol}, End: LexerPosition{Line: endLine, Col: endCol}}
}

func raw(value string, line int) Token {
	return &RawToken{Type: Raw, Value: value, Filename: "eval.edge", Line: line}
}

func newline(line int) Token {
	return &NewLineToken{Type: NewLine, Filename: "eval.edge", Line: line}
}

func comment(value string, l LexerLoc) Token {
	return &CommentToken{Type: Comment, Filename: "eval.edge", Value: value, Loc: l}
}

func mustache(typ, jsArg string, l LexerLoc) Token {
	token := &MustacheToken{Type: typ, Filename: "eval.edge", Loc: l}
	token.Properties.JsArg = jsArg
	return token
}

func tag(typ, name, jsArg string, selfclosed bool, l LexerLoc, children ...Token) Token {
	if children == nil {
		children = []Token{}
	}
	return &TagToken{
		Type: typ, Filename: "eval.edge", Loc: l, Children: children,
		Properties: TagProperties{Name: name, JsArg: jsArg, Selfclosed: selfclosed},
	}
}

// errorText formats an EdgeError as the cases record it.
func errorText(t *testing.T, err error) string {
	t.Helper()
	var edgeError *edgeerror.EdgeError
	if !errors.As(err, &edgeError) {
		t.Fatalf("unexpected error %T %v", err, err)
	}
	if edgeError.Filename != "eval.edge" {
		t.Errorf("error filename %v", edgeError.Filename)
	}
	return edgeError.Code + " " + js.ToString(edgeError.Line) + ":" + js.ToString(edgeError.Col) + " " + js.ToString(edgeError.Message)
}

func TestTokenizer(t *testing.T) {
	for _, c := range tokenizerCases {
		t.Run(c.name, func(t *testing.T) {
			options := Options{Filename: "eval.edge"}
			if c.claim {
				options.ClaimTag = claimTestTag
			}
			tokenizer := NewTokenizer(c.src, lookupEdgeTag, options)
			err := tokenizer.Parse()
			if c.err != "" {
				if err == nil {
					t.Fatalf("no error, want %s", c.err)
				}
				if text := errorText(t, err); text != c.err {
					t.Fatalf("error\n got %s\nwant %s", text, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %s", errorText(t, err))
			}
			if !reflect.DeepEqual(tokenizer.Tokens, c.want) {
				t.Errorf("tokens\n got %s\nwant %s", dump(tokenizer.Tokens), dump(c.want))
			}
		})
	}
}

// dump prints tokens for a failure message.
func dump(tokens []Token) string {
	var b strings.Builder
	for _, token := range tokens {
		switch t := token.(type) {
		case *TagToken:
			b.WriteString("\n" + t.Type + " " + t.Properties.Name + "(" + t.Properties.JsArg + ")")
			b.WriteString(strings.ReplaceAll(dump(t.Children), "\n", "\n  "))
		case *MustacheToken:
			b.WriteString("\n" + t.Type + " " + t.Properties.JsArg)
		default:
			b.WriteString("\n" + token.TokenType())
		}
		line, col := GetLineAndColumn(token)
		b.WriteString(" @" + js.ToString(float64(line)) + ":" + js.ToString(float64(col)))
	}
	return b.String()
}

func TestTokenizerOnLine(t *testing.T) {
	// Every line goes through onLine before it is tokenized.
	tokenizer := NewTokenizer("@when(a)\nx\n@endif", lookupEdgeTag, Options{
		Filename: "eval.edge",
		OnLine:   func(line string) string { return strings.Replace(line, "@when(", "@if(", 1) },
	})
	if err := tokenizer.Parse(); err != nil {
		t.Fatal(err)
	}
	want := []Token{tag("tag", "if", "a", false, loc(1, 4, 1, 6), newline(1), raw("x", 2))}
	if !reflect.DeepEqual(tokenizer.Tokens, want) {
		t.Errorf("tokens %s", dump(tokenizer.Tokens))
	}
}

func TestScanner(t *testing.T) {
	tests := []struct {
		name     string
		chunks   []string
		closed   bool
		match    string
		leftOver string
		loc      LexerPosition
	}{
		{"closing parenthesis", []string{"2 + 2 * (3)) after"}, true, "2 + 2 * (3)", " after", LexerPosition{Line: 1, Col: 12}},
		{"over several lines", []string{"a +", "\n", "  b)"}, true, "a +\n  b", "", LexerPosition{Line: 2, Col: 4}},
		{"blank line", []string{"a", "\n", "   ", "\n", ")"}, true, "a\n\n", "", LexerPosition{Line: 3, Col: 1}},
		{"still open", []string{"fn(a"}, false, "fn(a", "", LexerPosition{Line: 1, Col: 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := NewScanner(")", [2]string{"(", ")"}, 1, 0)
			for _, chunk := range test.chunks {
				s.Scan(chunk)
			}
			if s.Closed != test.closed || s.Match() != test.match || s.LeftOver() != test.leftOver || s.Loc != test.loc {
				t.Errorf("got closed=%v match=%q leftOver=%q loc=%v", s.Closed, s.Match(), s.LeftOver(), s.Loc)
			}
		})
	}
}
