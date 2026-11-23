package lexer

// TagDef is what the tokenizer reads from a tag definition: whether the tag takes an
// argument in parentheses (Seekable), has a body closed by @end (Block) and swallows
// the line break after it (NoNewLine).
type TagDef struct {
	Seekable  bool
	Block     bool
	NoNewLine bool
}

// TagLookup finds the definition of a tag by name; ok is false when no such tag
// exists (the original's `tags[name]` or `claimTag(name)` being falsy).
type TagLookup func(name string) (def TagDef, ok bool)

// runtimeTag is the tag detected at the start of a line (edge-lexer's getTag
// result). Col is the column after the tag name and the white space that follows
// it, then after the opening parenthesis once the argument is scanned.
type runtimeTag struct {
	name       string
	filename   string
	seekable   bool
	selfclosed bool
	block      bool
	line       int
	col        int
	escaped    bool
	hasBrace   bool
	noNewLine  bool
}

// tagMatch is a match of the tag regular expression
// /^(\s*)(@{1,2})(!)?([a-zA-Z._]+)(\s{0,2})/: the lengths of its groups and the name.
type tagMatch struct {
	whitespaceLeft  int
	ats             int
	selfclosed      bool
	name            string
	whitespaceRight int
}

func isTagNameUnit(c uint16) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '.' || c == '_'
}

// matchTag runs the tag regular expression on content. The leading white space is
// greedy and cannot give anything back to "@", so only the "@" count and the "!"
// need backtracking, which the loops below try in the regular expression's order.
func matchTag(content []uint16) (tagMatch, bool) {
	i := 0
	for i < len(content) && isSpaceUnit(content[i]) {
		i++
	}
	ats := 0
	for i+ats < len(content) && ats < 2 && content[i+ats] == '@' {
		ats++
	}
	for ; ats >= 1; ats-- {
		for _, bang := range []bool{true, false} {
			j := i + ats
			if bang {
				if j >= len(content) || content[j] != '!' {
					continue
				}
				j++
			}
			start := j
			for j < len(content) && isTagNameUnit(content[j]) {
				j++
			}
			if j == start {
				continue
			}
			end := j
			for j < len(content) && j-end < 2 && isSpaceUnit(content[j]) {
				j++
			}
			return tagMatch{
				whitespaceLeft:  i,
				ats:             ats,
				selfclosed:      bang,
				name:            string(utf16ASCII(content[start:end])),
				whitespaceRight: j - end,
			}, true
		}
	}
	return tagMatch{}, false
}

// utf16ASCII converts code units known to be ASCII to bytes.
func utf16ASCII(u []uint16) []byte {
	b := make([]byte, len(u))
	for i, c := range u {
		b[i] = byte(c)
	}
	return b
}

// getTag returns the tag a line starts with, when it is a known or claimed tag.
func getTag(content []uint16, filename string, line, col int, tags, claimTag TagLookup) (*runtimeTag, bool) {
	match, ok := matchTag(content)
	if !ok {
		return nil, false
	}
	def, ok := tags(match.name)
	if !ok && claimTag != nil {
		def, ok = claimTag(match.name)
	}
	if !ok {
		return nil, false
	}
	col += match.whitespaceLeft + match.ats + len(match.name) + match.whitespaceRight
	if match.selfclosed {
		col++
	}
	return &runtimeTag{
		name:       match.name,
		filename:   filename,
		seekable:   def.Seekable,
		selfclosed: match.selfclosed,
		block:      def.Block,
		line:       line,
		col:        col,
		escaped:    match.ats == 2,
		hasBrace:   def.Seekable && col < len(content) && content[col] == '(',
		noNewLine:  def.NoNewLine,
	}, true
}

// runtimeMustache is a mustache statement or a comment found in a line
// (edge-lexer's getMustache result). Col is the column in the template, realCol the
// index in the scanned content.
type runtimeMustache struct {
	isComment bool
	safe      bool
	escaped   bool
	filename  string
	line      int
	col       int
	realCol   int
}

// getMustache returns the first {{ in content: a comment ({{--), a safe mustache
// ({{{) or a mustache, escaped when preceded by "@".
func getMustache(content []uint16, filename string, line, col int) (*runtimeMustache, bool) {
	index := -1
	for i := 0; i+1 < len(content); i++ {
		if content[i] == '{' && content[i+1] == '{' {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, false
	}
	at := func(i int) uint16 {
		if i < 0 || i >= len(content) {
			return 0
		}
		return content[i]
	}
	if at(index+2) == '-' && at(index+3) == '-' {
		return &runtimeMustache{isComment: true, filename: filename, line: line, col: col + index, realCol: index}, true
	}
	return &runtimeMustache{
		safe:     at(index+2) == '{',
		filename: filename,
		escaped:  at(index-1) == '@',
		line:     line,
		col:      col + index,
		realCol:  index,
	}, true
}
