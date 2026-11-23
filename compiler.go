package edge

import (
	"strings"

	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// inlineVariables are the names compiled code defines itself, never read
// from the state.
var inlineVariables = []string{"$filename", "state", "$context"}

// templateParams are the parameters of every compiled template function.
var templateParams = []string{"template", "state", "$context"}

// Compiler compiles templates to JavaScript functions, called as
// compiled(template, state, $context, ...localVariables). The async compiler
// makes async functions, in which templates may await.
type Compiler struct {
	// Compat is set by the migrate plugin: templates may extend layouts,
	// and components get the compat Props as $props.
	Compat bool

	async        bool
	cacheManager *CacheManager
	claimTagFn   ClaimTagFn
	loader       LoaderContract
	tags         map[string]*Tag
	processor    *Processor
}

func newCompiler(loader LoaderContract, tags map[string]*Tag, processor *Processor, cache, async bool) *Compiler {
	return &Compiler{
		async:        async,
		cacheManager: newCacheManager(cache),
		loader:       loader,
		tags:         tags,
		processor:    processor,
	}
}

// Async reports whether the compiler makes async functions.
func (c *Compiler) Async() bool { return c.async }

// CacheManager returns the cache of compiled templates.
func (c *Compiler) CacheManager() *CacheManager { return c.cacheManager }

// ClaimTag sets the function that claims names that are not registered
// tags; parsers created afterwards use it.
func (c *Compiler) ClaimTag(fn ClaimTagFn) *Compiler {
	c.claimTagFn = fn
	return c
}

// Compile compiles a template file. localVariables are the names the
// function takes after (template, state, $context); with nil the result is
// cached (when caching is on) and taken from the cache.
func (c *Compiler) Compile(templatePath string, localVariables []string) (*js.Function, error) {
	var locals []any
	if localVariables != nil {
		locals = make([]any, len(localVariables))
		for i, name := range localVariables {
			locals[i] = name
		}
	}
	return c.compile(templatePath, locals)
}

// CompileRaw compiles template source. templatePath is js.Undefined for the
// default "eval.edge", or a string.
func (c *Compiler) CompileRaw(contents string, templatePath any) (*js.Function, error) {
	return c.compileRaw(contents, rawTemplatePath(templatePath))
}

// Tokenize returns the tokens of a template file.
func (c *Compiler) Tokenize(templatePath string) ([]Token, error) {
	return c.tokenize(templatePath, nil)
}

// TokenizeRaw returns the tokens of template source, after the raw
// processors. templatePath is js.Undefined for the default "eval.edge", or a
// string.
func (c *Compiler) TokenizeRaw(contents string, templatePath any) ([]Token, error) {
	return c.tokenizeRaw(contents, rawTemplatePath(templatePath), nil)
}

// rawTemplatePath is the path of raw template source: "eval.edge" by
// default, the string itself, or the text of another value.
func rawTemplatePath(templatePath any) string {
	switch p := templatePath.(type) {
	case js.UndefinedType:
		return "eval.edge"
	case string:
		return p
	}
	return js.ToString(templatePath)
}

// compileValue is compile for the arguments template code passes: a template
// path that is not a string fails where the default loader's path.isAbsolute
// rejects it.
func (c *Compiler) compileValue(templatePath any, localVariables []any) (*js.Function, error) {
	path, ok := templatePath.(string)
	if !ok {
		return nil, invalidPathType(templatePath)
	}
	return c.compile(path, localVariables)
}

// compileRawValue is compileRaw for the arguments template code passes.
func (c *Compiler) compileRawValue(contents, templatePath any) (*js.Function, error) {
	source, err := templateSource(contents)
	if err != nil {
		return nil, err
	}
	return c.compileRaw(source, rawTemplatePath(templatePath))
}

// parserFor returns a parser for a template, with the local variables
// defined on its stack.
func (c *Compiler) parserFor(templatePath string, localVariables []any) *Parser {
	p := parser.New(c.tags, parser.NewStack(), parser.Options{
		ClaimTag:          c.claimTagFn,
		Async:             c.async,
		StatePropertyName: "state",
		EscapeCallPath:    []string{"template", "escape"},
		LocalVariables:    inlineVariables,
		OnTag: func(tag *TagToken) error {
			return c.processor.executeTag(tag, templatePath)
		},
		GlobalDefined: edgert.Realm().GlobalDefined,
	})
	for _, name := range localVariables {
		p.Stack.DefineValue(name)
	}
	return p
}

// bufferFor returns the buffer a template is compiled into.
func bufferFor(templatePath string) *Buffer {
	return parser.NewEdgeBuffer(templatePath, parser.BufferOptions{
		OutputVar:       "out",
		RethrowCallPath: []string{"template", "reThrow"},
	})
}

// wrapToFunction turns compiled code into a function of the template
// parameters and the local variables, an async function for the async
// compiler, as `new Function(...args, template)` does.
func (c *Compiler) wrapToFunction(template string, localVariables []any) (*js.Function, error) {
	params := append([]string{}, templateParams...)
	for _, name := range localVariables {
		s, err := edgert.Realm().ToString(name)
		if err != nil {
			return nil, err
		}
		params = append(params, s)
	}
	return edgert.Realm().NewFunction(strings.Join(params, ","), template, c.async)
}

// compile compiles a template file; nil localVariables are undefined.
func (c *Compiler) compile(templatePath string, localVariables []any) (*js.Function, error) {
	absPath, err := c.loader.MakePath(templatePath)
	if err != nil {
		return nil, err
	}
	if localVariables == nil {
		if cached := c.cacheManager.Get(absPath); cached != nil {
			return cached, nil
		}
	}
	p := c.parserFor(absPath, localVariables)
	tokens, err := c.tokenize(absPath, p)
	if err != nil {
		return nil, err
	}
	compiled, err := c.compileTokens(tokens, p, absPath, localVariables)
	if err != nil {
		return nil, err
	}
	if localVariables == nil {
		c.cacheManager.Set(absPath, compiled)
	}
	return compiled, nil
}

// compileRaw compiles template source.
func (c *Compiler) compileRaw(contents, templatePath string) (*js.Function, error) {
	p := c.parserFor(templatePath, nil)
	tokens, err := c.tokenizeRaw(contents, templatePath, p)
	if err != nil {
		return nil, err
	}
	return c.compileTokens(tokens, p, templatePath, nil)
}

// compileTokens processes the tokens into a buffer, runs the compiled
// processors on its code and wraps it in a function.
func (c *Compiler) compileTokens(tokens []Token, p *Parser, templatePath string, localVariables []any) (*js.Function, error) {
	buffer := bufferFor(templatePath)
	for _, token := range tokens {
		if err := p.ProcessToken(token, buffer); err != nil {
			return nil, err
		}
	}
	template, err := c.processor.executeCompiled(buffer.Flush(), templatePath)
	if err != nil {
		return nil, err
	}
	return c.wrapToFunction(template, localVariables)
}

// tokenize resolves a template file and returns its tokens. A nil parser is
// a new one for the template.
func (c *Compiler) tokenize(templatePath string, p *Parser) ([]Token, error) {
	absPath, err := c.loader.MakePath(templatePath)
	if err != nil {
		return nil, err
	}
	resolved, err := c.loader.Resolve(absPath)
	if err != nil {
		return nil, err
	}
	contents, err := templateSource(resolved.Template)
	if err != nil {
		return nil, err
	}
	return c.tokenizeRaw(contents, absPath, p)
}

// tokenizeRaw runs the raw processors on template source and returns its
// tokens, with the layout merged in when a compat template extends one.
func (c *Compiler) tokenizeRaw(contents, templatePath string, p *Parser) ([]Token, error) {
	contents, err := c.processor.executeRaw(contents, templatePath)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = c.parserFor(templatePath, nil)
	}
	tokens, err := p.Tokenize(contents, templatePath)
	if err != nil {
		return nil, err
	}
	if !c.Compat {
		return tokens, nil
	}
	if len(tokens) == 0 {
		// isTag(templateTokens[0], "layout") reads the type of undefined.
		return nil, js.CannotReadProperties(js.Undefined, "type")
	}
	first := tokens[0]
	if !lexer.IsTag(first, "layout") {
		return tokens, nil
	}
	layoutName := strings.NewReplacer(`'`, "", `"`, "").Replace(first.(*TagToken).Properties.JsArg)
	layout, err := c.tokenize(layoutName, p)
	if err != nil {
		return nil, err
	}
	return mergeSections(layout, tokens)
}

// templateSource is the source of a template as edge-lexer reads it, which
// splits it into lines: a loader's template that is not a string cannot be
// tokenized.
func templateSource(template any) (string, error) {
	if s, ok := template.(string); ok {
		return s, nil
	}
	if js.IsNullish(template) {
		return "", js.CannotReadProperties(template, "split")
	}
	return "", js.NotAFunction("this.#template.split")
}

// mergeSections merges a compat template that extends a layout into the
// layout's tokens: its @set tags come first, then the layout, each of whose
// sections the template redefines replaced by the template's section. A
// section starting with @super (after a line break at most) keeps the
// layout's content before its own.
func mergeSections(base, extended []Token) ([]Token, error) {
	sections := map[string]*TagToken{}
	var setCalls []Token
	for _, node := range extended {
		switch {
		case lexer.IsTag(node, "layout"), node.TokenType() == lexer.NewLine, node.TokenType() == lexer.Comment:
			continue
		case node.TokenType() == lexer.Raw && jsstr.TrimSpace(node.(*RawToken).Value) == "":
			continue
		case lexer.IsTag(node, "section"):
			tag := node.(*TagToken)
			sections[jsstr.TrimSpace(tag.Properties.JsArg)] = tag
			continue
		case lexer.IsTag(node, "set"):
			setCalls = append(setCalls, node)
			continue
		}
		line, col := lexer.GetLineAndColumn(node)
		return nil, edgeerror.At(`Template extending a layout can only use "@section" or "@set" tags as top level nodes`,
			"E_UNALLOWED_EXPRESSION", tokenFilename(node), line, col)
	}
	merged := setCalls
	for _, node := range base {
		if !lexer.IsTag(node, "section") {
			merged = append(merged, node)
			continue
		}
		name := jsstr.TrimSpace(node.(*TagToken).Properties.JsArg)
		extendedNode, ok := sections[name]
		if !ok {
			if parser.IsObjectPrototypeKey(name) {
				// The sections are a plain object: a name it inherits from
				// Object.prototype finds a value without children.
				return nil, js.CannotReadProperties(js.Undefined, "length")
			}
			merged = append(merged, node)
			continue
		}
		if children := extendedNode.Children; len(children) > 0 {
			switch {
			case lexer.IsTag(children[0], "super"):
				extendedNode.Children = append(append([]Token{}, node.(*TagToken).Children...), children[1:]...)
			case len(children) < 2:
				// isTag(children[1], "super") reads the type of undefined.
				return nil, js.CannotReadProperties(js.Undefined, "type")
			case lexer.IsTag(children[1], "super"):
				extendedNode.Children = append(append([]Token{}, node.(*TagToken).Children...), children[2:]...)
			}
		}
		merged = append(merged, extendedNode)
	}
	return merged, nil
}

// tokenFilename returns the filename of a token.
func tokenFilename(token Token) string {
	switch t := token.(type) {
	case *TagToken:
		return t.Filename
	case *MustacheToken:
		return t.Filename
	case *RawToken:
		return t.Filename
	case *NewLineToken:
		return t.Filename
	case *CommentToken:
		return t.Filename
	}
	panic("edge: unknown token type")
}
