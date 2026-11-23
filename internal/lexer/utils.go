package lexer

// IsTag reports whether token is a tag or an escaped tag, named name when name is
// not empty (edge-lexer's isTag).
func IsTag(token Token, name string) bool {
	t, ok := token.(*TagToken)
	if !ok || (t.Type != Tag && t.Type != ETag) {
		return false
	}
	return name == "" || t.Properties.Name == name
}

// IsEscapedTag reports whether token is an escaped tag, named name when name is not
// empty (edge-lexer's isEscapedTag).
func IsEscapedTag(token Token, name string) bool {
	t, ok := token.(*TagToken)
	if !ok || t.Type != ETag {
		return false
	}
	return name == "" || t.Properties.Name == name
}

// IsMustache reports whether the token's type is one of the mustache types.
func IsMustache(token Token) bool {
	switch token.TokenType() {
	case EMustache, ESMustache, Mustache, SMustache:
		return true
	}
	return false
}

// IsSafeMustache reports whether the token is a {{{ }}} mustache, escaped or not.
func IsSafeMustache(token Token) bool {
	switch token.TokenType() {
	case ESMustache, SMustache:
		return true
	}
	return false
}

// IsEscapedMustache reports whether the token is an escaped (@{{ }}) mustache.
func IsEscapedMustache(token Token) bool {
	switch token.TokenType() {
	case EMustache, ESMustache:
		return true
	}
	return false
}

// GetLineAndColumn returns the line and the column a token starts at: column 0 for
// new lines and raw text.
func GetLineAndColumn(token Token) (line, col int) {
	switch t := token.(type) {
	case *NewLineToken:
		return t.Line, 0
	case *RawToken:
		return t.Line, 0
	case *TagToken:
		return t.Loc.Start.Line, t.Loc.Start.Col
	case *MustacheToken:
		return t.Loc.Start.Line, t.Loc.Start.Col
	case *CommentToken:
		return t.Loc.Start.Line, t.Loc.Start.Col
	}
	panic("lexer: unknown token type")
}
