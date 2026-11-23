package acorn

import "strings"

// tokenType is acorn's TokenType: the kind of a token and what the parser and the
// regular-expression detection need to know about it.
type tokenType struct {
	label   string
	keyword string
	// beforeExpr marks tokens after which a slash starts a regular expression.
	beforeExpr bool
	// startsExpr marks tokens that can start or continue an expression; it ends a
	// yield without argument otherwise.
	startsExpr bool
	isLoop     bool
	isAssign   bool
	prefix     bool
	postfix    bool
	// binop is the binary operator precedence, 0 for none.
	binop int
}

func punct(label string, beforeExpr, startsExpr bool) *tokenType {
	return &tokenType{label: label, beforeExpr: beforeExpr, startsExpr: startsExpr}
}

func binop(label string, prec int) *tokenType {
	return &tokenType{label: label, beforeExpr: true, binop: prec}
}

// keywordTypes maps each keyword to its token type.
var keywordTypes = map[string]*tokenType{}

func kw(t tokenType) *tokenType {
	t.label = t.keyword
	keywordTypes[t.keyword] = &t
	return &t
}

var (
	tokNum             = punct("num", false, true)
	tokRegexp          = punct("regexp", false, true)
	tokString          = punct("string", false, true)
	tokName            = punct("name", false, true)
	tokEOF             = punct("eof", false, false)
	tokBracketL        = punct("[", true, true)
	tokBracketR        = punct("]", false, false)
	tokBraceL          = punct("{", true, true)
	tokBraceR          = punct("}", false, false)
	tokParenL          = punct("(", true, true)
	tokParenR          = punct(")", false, false)
	tokComma           = punct(",", true, false)
	tokSemi            = punct(";", true, false)
	tokColon           = punct(":", true, false)
	tokDot             = punct(".", false, false)
	tokQuestion        = punct("?", true, false)
	tokQuestionDot     = punct("?.", false, false)
	tokArrow           = punct("=>", true, false)
	tokTemplate        = punct("template", false, false)
	tokInvalidTemplate = punct("invalidTemplate", false, false)
	tokEllipsis        = punct("...", true, false)
	tokBackQuote       = punct("`", false, true)
	tokDollarBraceL    = punct("${", true, true)
	tokEq              = &tokenType{label: "=", beforeExpr: true, isAssign: true}
	tokAssign          = &tokenType{label: "_=", beforeExpr: true, isAssign: true}
	tokIncDec          = &tokenType{label: "++/--", prefix: true, postfix: true, startsExpr: true}
	tokPrefix          = &tokenType{label: "!/~", beforeExpr: true, prefix: true, startsExpr: true}
	tokLogicalOR       = binop("||", 1)
	tokLogicalAND      = binop("&&", 2)
	tokBitwiseOR       = binop("|", 3)
	tokBitwiseXOR      = binop("^", 4)
	tokBitwiseAND      = binop("&", 5)
	tokEquality        = binop("==/!=/===/!==", 6)
	tokRelational      = binop("</>/<=/>=", 7)
	tokBitShift        = binop("<</>>/>>>", 8)
	tokPlusMin         = &tokenType{label: "+/-", beforeExpr: true, binop: 9, prefix: true, startsExpr: true}
	tokModulo          = binop("%", 10)
	tokStar            = binop("*", 10)
	tokSlash           = binop("/", 10)
	tokStarstar        = &tokenType{label: "**", beforeExpr: true}
	tokCoalesce        = binop("??", 1)

	tokBreak      = kw(tokenType{keyword: "break"})
	tokCase       = kw(tokenType{keyword: "case", beforeExpr: true})
	tokCatch      = kw(tokenType{keyword: "catch"})
	tokContinue   = kw(tokenType{keyword: "continue"})
	tokDebugger   = kw(tokenType{keyword: "debugger"})
	tokDefault    = kw(tokenType{keyword: "default", beforeExpr: true})
	tokDo         = kw(tokenType{keyword: "do", isLoop: true, beforeExpr: true})
	tokElse       = kw(tokenType{keyword: "else", beforeExpr: true})
	tokFinally    = kw(tokenType{keyword: "finally"})
	tokFor        = kw(tokenType{keyword: "for", isLoop: true})
	tokFunction   = kw(tokenType{keyword: "function", startsExpr: true})
	tokIf         = kw(tokenType{keyword: "if"})
	tokReturn     = kw(tokenType{keyword: "return", beforeExpr: true})
	tokSwitch     = kw(tokenType{keyword: "switch"})
	tokThrow      = kw(tokenType{keyword: "throw", beforeExpr: true})
	tokTry        = kw(tokenType{keyword: "try"})
	tokVar        = kw(tokenType{keyword: "var"})
	tokConst      = kw(tokenType{keyword: "const"})
	tokWhile      = kw(tokenType{keyword: "while", isLoop: true})
	tokWith       = kw(tokenType{keyword: "with"})
	tokNew        = kw(tokenType{keyword: "new", beforeExpr: true, startsExpr: true})
	tokThis       = kw(tokenType{keyword: "this", startsExpr: true})
	tokSuper      = kw(tokenType{keyword: "super", startsExpr: true})
	tokClass      = kw(tokenType{keyword: "class", startsExpr: true})
	tokExtends    = kw(tokenType{keyword: "extends", beforeExpr: true})
	tokExport     = kw(tokenType{keyword: "export"})
	tokImport     = kw(tokenType{keyword: "import", startsExpr: true})
	tokNull       = kw(tokenType{keyword: "null", startsExpr: true})
	tokTrue       = kw(tokenType{keyword: "true", startsExpr: true})
	tokFalse      = kw(tokenType{keyword: "false", startsExpr: true})
	tokIn         = kw(tokenType{keyword: "in", beforeExpr: true, binop: 7})
	tokInstanceof = kw(tokenType{keyword: "instanceof", beforeExpr: true, binop: 7})
	tokTypeof     = kw(tokenType{keyword: "typeof", beforeExpr: true, prefix: true, startsExpr: true})
	tokVoid       = kw(tokenType{keyword: "void", beforeExpr: true, prefix: true, startsExpr: true})
	tokDelete     = kw(tokenType{keyword: "delete", beforeExpr: true, prefix: true, startsExpr: true})
)

// wordSet is acorn's wordsRegexp: a set of words separated by spaces. Like the
// regular expression ^(?:...)$ built from an empty list, the set of "" holds the
// empty word.
type wordSet map[string]bool

func words(list string) wordSet {
	s := wordSet{}
	for _, w := range strings.Split(list, " ") {
		s[w] = true
	}
	return s
}

const ecma5AndLessKeywords = "break case catch continue debugger default do else finally for function if return switch throw try var while with null true false instanceof typeof void delete new in this"

var (
	keywords5 = words(ecma5AndLessKeywords)
	keywords6 = words(ecma5AndLessKeywords + " const class extends export import super")
)

// reservedWordSets are the words acorn reserves for one ecmaVersion: always, in
// strict mode, and as binding names in strict mode.
type reservedWordSets struct {
	reserved, strict, strictBind wordSet
}

func newReservedWordSets(reserved string) *reservedWordSets {
	const (
		strict     = "implements interface let package private protected public static yield"
		strictBind = "eval arguments"
	)
	if reserved != "" {
		reserved += " "
	}
	return &reservedWordSets{
		reserved:   words(strings.TrimSuffix(reserved, " ")),
		strict:     words(reserved + strict),
		strictBind: words(reserved + strict + " " + strictBind),
	}
}

// Before ECMAScript 5 acorn allows reserved words (its allowReserved option
// defaults to true there), so only the strict-mode words remain.
var (
	reservedWordsBefore5 = newReservedWordSets("")
	reservedWords5       = newReservedWordSets("class enum extends super const export import")
	reservedWords6       = newReservedWordSets("enum")
)
