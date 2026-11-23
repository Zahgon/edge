package lexer

import (
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
)

// Options are the tokenizer options.
type Options struct {
	// Filename is written on every token and error.
	Filename string
	// ClaimTag, when set, is asked about a name that is not a known tag; a claimed
	// tag is tokenized with the returned definition.
	ClaimTag TagLookup
	// OnLine, when set, rewrites every line before it is tokenized.
	OnLine func(line string) string
}

// tagStatement is a seekable tag whose argument is being scanned.
type tagStatement struct {
	tag     *runtimeTag
	scanner *Scanner
}

// mustacheStatement is a mustache statement or a comment being scanned.
type mustacheStatement struct {
	mustache *runtimeMustache
	scanner  *Scanner
}

// Tokenizer converts a template into tokens (edge-lexer's Tokenizer). Block tags
// collect the tokens up to their @end as children.
type Tokenizer struct {
	line              int
	isLastLineATag    bool
	dropNewLine       bool
	openedTags        []*TagToken
	options           Options
	template          string
	tagsDef           TagLookup
	tagStatement      *tagStatement
	mustacheStatement *mustacheStatement

	// Tokens are the top-level tokens; Parse fills them.
	Tokens []Token
}

// NewTokenizer is `new Tokenizer(template, tagsDef, options)`: tagsDef looks up the
// known tags.
func NewTokenizer(template string, tagsDef TagLookup, options Options) *Tokenizer {
	return &Tokenizer{template: template, tagsDef: tagsDef, options: options, Tokens: []Token{}}
}

func (t *Tokenizer) getRawNode(text string) *RawToken {
	return &RawToken{Type: Raw, Value: text, Filename: t.options.Filename, Line: t.line}
}

func (t *Tokenizer) getNewLineNode() *NewLineToken {
	return &NewLineToken{Type: NewLine, Filename: t.options.Filename, Line: t.line - 1}
}

func (t *Tokenizer) getTagNode(tag *runtimeTag, jsArg string, closingLoc LexerPosition) *TagToken {
	typ := Tag
	if tag.escaped {
		typ = ETag
	}
	return &TagToken{
		Type:       typ,
		Filename:   tag.filename,
		Properties: TagProperties{Name: tag.name, JsArg: jsArg, Selfclosed: tag.selfclosed},
		Loc:        LexerLoc{Start: LexerPosition{Line: tag.line, Col: tag.col}, End: closingLoc},
		Children:   []Token{},
	}
}

// consumeTag opens a block tag (until its @end) or adds any other tag as a token.
func (t *Tokenizer) consumeTag(tag *runtimeTag, jsArg string, loc LexerPosition) {
	if tag.block && !tag.selfclosed {
		t.openedTags = append(t.openedTags, t.getTagNode(tag, jsArg, loc))
		return
	}
	t.consumeNode(t.getTagNode(tag, jsArg, loc))
}

func (t *Tokenizer) handleTagOpening(line []uint16, tag *runtimeTag) error {
	if tag.seekable && !tag.hasBrace {
		return UnopenedParen(LexerPosition{Line: tag.line, Col: tag.col}, tag.filename)
	}
	// A tag without an argument is complete; the rest of the line is not read.
	if !tag.seekable {
		t.consumeTag(tag, "", LexerPosition{Line: tag.line, Col: tag.col})
		if tag.noNewLine || endsWithTilde(line) {
			t.dropNewLine = true
		}
		return nil
	}
	// Scan from after the opening parenthesis for the closing one.
	tag.col++
	t.tagStatement = &tagStatement{tag: tag, scanner: NewScanner(")", [2]string{"(", ")"}, t.line, tag.col)}
	return t.feedCharsToCurrentTag(sliceFrom(line, tag.col))
}

func (t *Tokenizer) feedCharsToCurrentTag(content []uint16) error {
	tag, scanner := t.tagStatement.tag, t.tagStatement.scanner
	scanner.scan(content)
	if !scanner.Closed {
		return nil
	}
	t.consumeTag(tag, scanner.Match(), scanner.Loc)
	leftOver := jsstr.TrimSpace(scanner.LeftOver())
	// A trailing ~ drops the line break after the tag.
	if leftOver == "~" {
		t.tagStatement = nil
		t.dropNewLine = true
		return nil
	}
	if leftOver != "" {
		return CannotSeekStatement(scanner.LeftOver(), scanner.Loc, tag.filename)
	}
	if tag.noNewLine {
		t.dropNewLine = true
	}
	t.tagStatement = nil
	return nil
}

func getMustacheType(m *runtimeMustache) string {
	if m.safe {
		if m.escaped {
			return ESMustache
		}
		return SMustache
	}
	if m.escaped {
		return EMustache
	}
	return Mustache
}

func (t *Tokenizer) getMustacheNode(m *runtimeMustache, jsArg string, closingLoc LexerPosition) *MustacheToken {
	token := &MustacheToken{
		Type:     getMustacheType(m),
		Filename: m.filename,
		Loc:      LexerLoc{Start: LexerPosition{Line: m.line, Col: m.col}, End: closingLoc},
	}
	token.Properties.JsArg = jsArg
	return token
}

func (t *Tokenizer) getCommentNode(m *runtimeMustache, value string, closingLoc LexerPosition) *CommentToken {
	return &CommentToken{
		Type:     Comment,
		Filename: m.filename,
		Value:    value,
		Loc:      LexerLoc{Start: LexerPosition{Line: m.line, Col: m.col}, End: closingLoc},
	}
}

func (t *Tokenizer) handleMustacheOpening(line []uint16, m *runtimeMustache) {
	pattern := "}}"
	switch {
	case m.isComment:
		pattern = "--}}"
	case m.safe:
		pattern = "}}}"
	}
	// The text before the statement (and before the "@" of an escaped one) is raw.
	textLeftIndex := m.realCol
	if !m.isComment && m.escaped {
		textLeftIndex--
	}
	if textLeftIndex > 0 {
		t.consumeNode(t.getRawNode(jsstr.FromUTF16(line[:textLeftIndex])))
	}
	// The opening braces are as long as the closing pattern.
	m.col += len(pattern)
	m.realCol += len(pattern)
	t.mustacheStatement = &mustacheStatement{mustache: m, scanner: NewScanner(pattern, [2]string{"{", "}"}, m.line, m.col)}
	t.feedCharsToCurrentMustache(sliceFrom(line, m.realCol))
}

func (t *Tokenizer) feedCharsToCurrentMustache(content []uint16) {
	m, scanner := t.mustacheStatement.mustache, t.mustacheStatement.scanner
	scanner.scan(content)
	if !scanner.Closed {
		return
	}
	if m.isComment {
		t.consumeNode(t.getCommentNode(m, scanner.Match(), scanner.Loc))
	} else {
		t.consumeNode(t.getMustacheNode(m, scanner.Match(), scanner.Loc))
	}
	// Text after the closing braces may hold more statements; white space alone is
	// dropped.
	if !isBlank(scanner.leftOver) {
		if another, ok := getMustache(scanner.leftOver, t.options.Filename, scanner.Loc.Line, scanner.Loc.Col); ok {
			t.handleMustacheOpening(scanner.leftOver, another)
			return
		}
		t.consumeNode(t.getRawNode(scanner.LeftOver()))
	}
	t.mustacheStatement = nil
}

// isClosingTag reports whether line closes the most recently opened block tag:
// @end, @end<name>, optionally followed by ~.
func (t *Tokenizer) isClosingTag(line string) bool {
	if len(t.openedTags) == 0 {
		return false
	}
	line = jsstr.TrimSpace(line)
	endStatement := "@end" + t.openedTags[len(t.openedTags)-1].Properties.Name
	return line == endStatement || line == endStatement+"~" || line == "@end" || line == "@end~"
}

// consumeNode adds a token to the children of the innermost opened tag, or to the
// top-level tokens.
func (t *Tokenizer) consumeNode(token Token) {
	if n := len(t.openedTags); n > 0 {
		t.openedTags[n-1].Children = append(t.openedTags[n-1].Children, token)
		return
	}
	t.Tokens = append(t.Tokens, token)
}

// pushNewLine adds the line break before the current line, except before the first
// line and when the previous statement asked to drop it.
func (t *Tokenizer) pushNewLine() {
	if t.line == 1 {
		return
	}
	if t.dropNewLine {
		t.dropNewLine = false
		return
	}
	t.consumeNode(t.getNewLineNode())
}

func (t *Tokenizer) processText(line string) error {
	if t.options.OnLine != nil {
		line = t.options.OnLine(line)
	}
	units := jsstr.ToUTF16(line)
	if t.tagStatement != nil {
		if err := t.feedCharsToCurrentTag([]uint16{'\n'}); err != nil {
			return err
		}
		return t.feedCharsToCurrentTag(units)
	}
	if t.mustacheStatement != nil {
		t.feedCharsToCurrentMustache([]uint16{'\n'})
		t.feedCharsToCurrentMustache(units)
		return nil
	}
	if t.isClosingTag(line) {
		n := len(t.openedTags)
		closed := t.openedTags[n-1]
		t.openedTags = t.openedTags[:n-1]
		t.consumeNode(closed)
		if endsWithTilde(units) {
			t.dropNewLine = true
		}
		return nil
	}
	if tag, ok := getTag(units, t.options.Filename, t.line, 0, t.tagsDef, t.options.ClaimTag); ok {
		// Two tags on consecutive lines keep the line break between them.
		if t.isLastLineATag {
			t.pushNewLine()
		}
		t.isLastLineATag = true
		return t.handleTagOpening(units, tag)
	}
	t.isLastLineATag = false
	if m, ok := getMustache(units, t.options.Filename, t.line, 0); ok {
		t.pushNewLine()
		t.handleMustacheOpening(units, m)
		return nil
	}
	t.pushNewLine()
	t.consumeNode(t.getRawNode(line))
	return nil
}

// checkForErrors reports what is still open at the end of the template.
func (t *Tokenizer) checkForErrors() error {
	if t.tagStatement != nil {
		tag := t.tagStatement.tag
		return UnclosedParen(LexerPosition{Line: tag.line, Col: tag.col}, tag.filename)
	}
	if t.mustacheStatement != nil {
		return UnclosedCurlyBrace(t.mustacheStatement.scanner.Loc, t.mustacheStatement.mustache.filename)
	}
	if n := len(t.openedTags); n > 0 {
		opened := t.openedTags[n-1]
		return UnclosedTag(opened.Properties.Name, opened.Loc.Start, opened.Filename)
	}
	return nil
}

// Parse tokenizes the template line by line (lines end at \r\n, \r or \n) into
// Tokens. On an error the tokens read so far stay in Tokens.
func (t *Tokenizer) Parse() error {
	lines := splitLines(t.template)
	for t.line < len(lines) {
		line := lines[t.line]
		t.line++
		if err := t.processText(line); err != nil {
			return err
		}
	}
	return t.checkForErrors()
}

// splitLines is `template.split(/\r\n|\r|\n/g)`.
func splitLines(s string) []string {
	lines := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			lines = append(lines, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		case '\n':
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}

// sliceFrom is `content.slice(index)`.
func sliceFrom(content []uint16, index int) []uint16 {
	return content[min(index, len(content)):]
}

// endsWithTilde is `line.endsWith("~")`.
func endsWithTilde(line []uint16) bool {
	return len(line) > 0 && line[len(line)-1] == '~'
}
