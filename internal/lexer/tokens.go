// Package lexer is edge-lexer 6.0.5: it splits an Edge template into raw text, new
// lines, tags, mustache statements and comments.
//
// Strings are JavaScript strings in the WTF-8 form of package jsstr, and every
// column counts UTF-16 code units, as the JavaScript original indexes its strings.
package lexer

// Token types of the mustache statements (edge-lexer's MustacheTypes).
const (
	SMustache  = "s__mustache"  // {{{ }}}
	ESMustache = "es__mustache" // @{{{ }}}
	Mustache   = "mustache"     // {{ }}
	EMustache  = "e__mustache"  // @{{ }}
)

// Token types of the tags (edge-lexer's TagTypes).
const (
	Tag  = "tag"    // @tag
	ETag = "e__tag" // @@tag
)

// Token types of the other tokens.
const (
	Raw     = "raw"
	NewLine = "newline"
	Comment = "comment"
)

// Token is one of *TagToken, *MustacheToken, *RawToken, *NewLineToken and
// *CommentToken. TokenType returns its type property.
type Token interface {
	TokenType() string
}

// LexerPosition is a line (1-based) and a column (0-based, in UTF-16 code units).
type LexerPosition struct {
	Line, Col int
}

// LexerLoc is the start and the end of a tag, mustache or comment.
type LexerLoc struct {
	Start, End LexerPosition
}

// TagProperties are the properties of a tag token: its name, the source text of its
// argument and whether it was written self-closed (@!name).
type TagProperties struct {
	Name       string
	JsArg      string
	Selfclosed bool
}

// TagToken is a tag ("tag") or an escaped tag ("e__tag"). Children holds the tokens
// between a block tag and its @end.
type TagToken struct {
	Type       string
	Filename   string
	Properties TagProperties
	Loc        LexerLoc
	Children   []Token
}

// MustacheToken is a mustache statement of one of the four mustache types.
type MustacheToken struct {
	Type       string
	Filename   string
	Properties struct{ JsArg string }
	Loc        LexerLoc
}

// RawToken is text written as it is.
type RawToken struct {
	Type     string
	Value    string
	Filename string
	Line     int
}

// NewLineToken is the line break that ends line Line.
type NewLineToken struct {
	Type     string
	Filename string
	Line     int
}

// CommentToken is a {{-- --}} comment.
type CommentToken struct {
	Type     string
	Filename string
	Value    string
	Loc      LexerLoc
}

// TokenType returns the token's type.
func (t *TagToken) TokenType() string { return t.Type }

// TokenType returns the token's type.
func (t *MustacheToken) TokenType() string { return t.Type }

// TokenType returns the token's type.
func (t *RawToken) TokenType() string { return t.Type }

// TokenType returns the token's type.
func (t *NewLineToken) TokenType() string { return t.Type }

// TokenType returns the token's type.
func (t *CommentToken) TokenType() string { return t.Type }
