// Package parser is edge-parser 9.1.1: it turns the tokens of edge-lexer into the
// JavaScript code of a template function. Mustache statements and tag arguments are
// parsed with acorn, rewritten so that free identifiers read the template state,
// and printed back with astring into an EdgeBuffer.
package parser

import (
	"strings"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/astring"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/js"
)

// Tag is a tag definition: its name, whether it takes an argument in parentheses
// (Seekable), has a body closed by @end (Block) and swallows the line break after it
// (NoNewLine), the function that compiles a token of the tag, and Boot, called with
// the Template class when the tag is registered (Edge attaches it; nil means the tag
// has no boot method).
type Tag struct {
	TagName   string
	Block     bool
	Seekable  bool
	NoNewLine bool
	Compile   func(parser *Parser, buffer *EdgeBuffer, token *lexer.TagToken) error
	Boot      func() error
}

// ClaimTagFn claims a name that is not a registered tag; the tokenizer then reads it
// with the returned tag's Seekable, Block and NoNewLine. nil leaves the name to be
// raw text.
type ClaimTagFn func(name string) *Tag

// Options are the parser options.
type Options struct {
	// ClaimTag and OnLine are handed to the tokenizer.
	ClaimTag ClaimTagFn
	OnLine   func(line string) string
	// Async allows await expressions.
	Async bool
	// StatePropertyName is the object free identifiers are read from ("state").
	StatePropertyName string
	// EscapeCallPath is the function {{ }} output goes through: one name, or an
	// object and a method (["template", "escape"]).
	EscapeCallPath []string
	// LocalVariables are names never read from the state.
	LocalVariables []string
	// OnTag and OnMustache are called with each tag and each {{ }}/{{{ }}} token
	// before it is compiled; an error stops the compilation.
	OnTag      func(token *lexer.TagToken) error
	OnMustache func(token *lexer.MustacheToken) error
	// GlobalDefined reports whether globalThis[name] is not undefined: such
	// identifiers are left alone. nil means no name is a global.
	GlobalDefined func(name string) bool
}

// Parser is edge-parser's Parser. The methods named after parser.utils (Stringify,
// TransformAST, MakeCallable, ...) are the utilities tags use.
type Parser struct {
	tags    map[string]*Tag
	Stack   *Stack
	options Options
}

// New is `new Parser(tags, stack, options)`. tags is the registry of the Edge
// instance and is read at every lookup, so tags registered later are seen; a nil
// stack is a new Stack.
func New(tags map[string]*Tag, stack *Stack, options Options) *Parser {
	if stack == nil {
		stack = NewStack()
	}
	return &Parser{tags: tags, Stack: stack, options: options}
}

// AsyncMode reports whether the parser compiles for an async template function.
func (p *Parser) AsyncMode() bool { return p.options.Async }

// Options returns the parser options.
func (p *Parser) Options() Options { return p.options }

// objectPrototypeKeys are the own properties of Object.prototype in V8. The tags
// registry is a plain object, so `tags[name]` finds them for any of these names.
var objectPrototypeKeys = map[string]bool{
	"constructor": true, "__defineGetter__": true, "__defineSetter__": true, "hasOwnProperty": true,
	"__lookupGetter__": true, "__lookupSetter__": true, "isPrototypeOf": true, "propertyIsEnumerable": true,
	"toString": true, "valueOf": true, "__proto__": true, "toLocaleString": true,
}

// IsObjectPrototypeKey reports whether name is an own property of Object.prototype,
// which a lookup by name in a plain JavaScript object finds when the object has no
// property of that name.
func IsObjectPrototypeKey(name string) bool { return objectPrototypeKeys[name] }

// lexerDef is what the tokenizer reads from a tag.
func lexerDef(tag *Tag) lexer.TagDef {
	return lexer.TagDef{Seekable: tag.Seekable, Block: tag.Block, NoNewLine: tag.NoNewLine}
}

// lookupTagDef is `tags[name]` as the tokenizer sees it: a registered tag, or an
// inherited Object.prototype member, which has none of the tag properties.
func (p *Parser) lookupTagDef(name string) (lexer.TagDef, bool) {
	if tag, ok := p.tags[name]; ok && tag != nil {
		return lexerDef(tag), true
	}
	return lexer.TagDef{}, objectPrototypeKeys[name]
}

// tokenizerOptions are the options the parser hands to the tokenizer.
func (p *Parser) tokenizerOptions(filename string) lexer.Options {
	options := lexer.Options{Filename: filename, OnLine: p.options.OnLine}
	if claim := p.options.ClaimTag; claim != nil {
		options.ClaimTag = func(name string) (lexer.TagDef, bool) {
			tag := claim(name)
			if tag == nil {
				return lexer.TagDef{}, false
			}
			return lexerDef(tag), true
		}
	}
	return options
}

// Tokenize converts a template to tokens with the parser's tags and options.
func (p *Parser) Tokenize(template, filename string) ([]lexer.Token, error) {
	tokenizer := lexer.NewTokenizer(template, p.lookupTagDef, p.tokenizerOptions(filename))
	if err := tokenizer.Parse(); err != nil {
		return nil, err
	}
	return tokenizer.Tokens, nil
}

// processEscapedTagToken writes an escaped tag (@@name) as it was written, with its
// children compiled in between and an @end after them.
func (p *Parser) processEscapedTagToken(token *lexer.TagToken, buffer *EdgeBuffer) error {
	for _, line := range strings.Split("@"+token.Properties.Name+"("+token.Properties.JsArg+")", "\n") {
		buffer.OutputRaw(line)
	}
	for _, child := range token.Children {
		if err := p.ProcessToken(child, buffer); err != nil {
			return err
		}
	}
	buffer.OutputRaw("@end" + token.Properties.Name)
	return nil
}

// processEscapedMustache writes an escaped mustache (@{{ }}) as it was written.
func (p *Parser) processEscapedMustache(token *lexer.MustacheToken, buffer *EdgeBuffer) {
	text := "{{{" + token.Properties.JsArg + "}}}"
	if token.Type == lexer.EMustache {
		text = "{{" + token.Properties.JsArg + "}}"
	}
	for _, line := range strings.Split(text, "\n") {
		buffer.OutputRaw(line)
	}
}

// processMustache outputs the value of a {{ }} (escaped) or {{{ }}} expression.
func (p *Parser) processMustache(token *lexer.MustacheToken, buffer *EdgeBuffer) error {
	ast, err := p.GenerateAST(token.Properties.JsArg, token.Loc, token.Filename)
	if err != nil {
		return err
	}
	node, err := p.TransformAST(ast, token.Filename)
	if err != nil {
		return err
	}
	expression := node
	if token.Type == lexer.Mustache {
		expression = MakeCallable(p.options.EscapeCallPath, []*acorn.Node{node})
	}
	// A template literal is already a template string and is not wrapped in
	// another one; a function declaration is written as it is.
	templateLiteral := true
	switch node.Type {
	case "TemplateLiteral":
		templateLiteral = false
	case "FunctionDeclaration":
		expression, templateLiteral = node, false
	}
	code, err := astring.Generate(expression)
	if err != nil {
		return err
	}
	buffer.OutputExpression(code, token.Filename, token.Loc.Start.Line, templateLiteral)
	return nil
}

// ProcessToken compiles a token into the buffer.
func (p *Parser) ProcessToken(token lexer.Token, buffer *EdgeBuffer) error {
	switch token.TokenType() {
	case lexer.Raw:
		buffer.OutputRaw(token.(*lexer.RawToken).Value)
	case lexer.NewLine:
		buffer.OutputRaw(EOL)
	case lexer.Tag:
		tagToken := token.(*lexer.TagToken)
		if p.options.OnTag != nil {
			if err := p.options.OnTag(tagToken); err != nil {
				return err
			}
		}
		return p.compileTag(tagToken, buffer)
	case lexer.ETag:
		return p.processEscapedTagToken(token.(*lexer.TagToken), buffer)
	case lexer.EMustache, lexer.ESMustache:
		p.processEscapedMustache(token.(*lexer.MustacheToken), buffer)
	case lexer.SMustache, lexer.Mustache:
		mustache := token.(*lexer.MustacheToken)
		if p.options.OnMustache != nil {
			if err := p.options.OnMustache(mustache); err != nil {
				return err
			}
		}
		return p.processMustache(mustache, buffer)
	}
	return nil
}

// compileTag is `this.tags[token.properties.name].compile(this, buffer, token)`,
// with V8's TypeErrors when the name is not a tag (a claimed tag nothing renamed)
// or names something without a compile function.
func (p *Parser) compileTag(token *lexer.TagToken, buffer *EdgeBuffer) (err error) {
	name := token.Properties.Name
	tag, ok := p.tags[name]
	switch {
	case ok && tag != nil && tag.Compile != nil:
		defer recoverStringifyError(&err)
		return tag.Compile(p, buffer, token)
	case ok && tag != nil || objectPrototypeKeys[name]:
		return js.NotAFunction("this.tags[token.properties.name].compile")
	}
	return js.CannotReadProperties(js.Undefined, "compile")
}
