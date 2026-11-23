package strutil

import (
	"slices"
	"strings"

	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// Truncation: truncatise 0.0.8 as @poppinss/string bundles it, called by
// string.truncate and string.excerpt with TruncateBy "characters".

// defaultSuffix is truncatise's Suffix when the option is not a string.
const defaultSuffix = "..."

// truncatiseLength is truncatise's TruncateLength when the option is not a
// number.
const truncatiseLength = 50

// selfClosingTags are the tags truncatise neither pushes on nor pops from its
// stack of open tags.
var selfClosingTags = []string{
	"area", "base", "br", "col", "embed", "hr", "img", "input", "keygen", "link",
	"menuitem", "meta", "param", "source", "track", "wbr",
}

var (
	reBreakTag       = jsval.MustCompile(`<br( \/)?>`, "gi")
	reComment        = jsval.MustCompile(`<!--(.*?)-->`, "gm")
	reTag            = jsval.MustCompile(`<\/?[^>]+>`, "gi")
	reParagraphGap   = jsval.MustCompile(`<\/p>(\r?\n)+<p>`, "gm")
	reBlankLine      = jsval.MustCompile(`\r?\n\r?\n`, "")
	reParagraphs     = jsval.MustCompile(`((.+)(\r?\n\r?\n|$))`, "gi")
	reWordChar       = jsval.MustCompile(`[a-zA-ZÇ-Ü']`, "i")
	reClosingPara    = jsval.MustCompile(`<\/p>$`, "gi")
	reClosingParaCap = jsval.MustCompile(`(<\/p>)$`, "gi")
)

// truncatise is truncatise(text, options) for TruncateBy "characters", with
// the options already normalized (limit is TruncateLength, strict is Strict,
// stripHTML is StripHTML, suffix is Suffix). text is the value after
// `(text || "").trim()`; a value that is not a string (a user-defined trim may
// return anything) is returned as it is when it is "" or short enough, and
// otherwise read as its string conversion.
func truncatise(ops *jsval.Ops, text any, limit float64, strict, stripHTML bool, suffix string) (any, error) {
	if s, ok := text.(string); ok && s == "" {
		return text, nil
	}
	length, err := lengthOf(ops, text)
	if err != nil {
		return nil, err
	}
	if n, ok := length.(float64); ok && n <= limit && !stripHTML {
		return text, nil
	}
	s, err := templateString(ops, text)
	if err != nil {
		return nil, err
	}
	if stripHTML {
		s = reBreakTag.ReplaceString(s, " ")
		s = reComment.ReplaceString(s, "")
		s = reTag.ReplaceString(s, "")
	}
	s = reParagraphGap.ReplaceString(s, "</p><p>")
	if stripHTML && reBlankLine.Test(s) {
		s = reParagraphs.ReplaceString(s, "<p>$2</p>")
	}
	return truncateUnits(jsstr.ToUTF16(s), limit, strict, stripHTML, suffix), nil
}

// The states of truncatise's scanner.
const (
	notTag = iota
	tagStart
	tagAttributes
)

// truncateUnits is truncatise's scanning loop over the code units of text and
// what follows it: counting the characters outside tags, stopping once limit
// of them are taken (at the end of a word unless strict), closing the tags
// left open and appending the suffix when text was cut.
func truncateUnits(text []uint16, limit float64, strict, stripHTML bool, suffix string) string {
	var (
		out      []uint16
		state    = notTag
		tag      []uint16
		tagStack []string
		chars    float64
		pointer  int
	)
	for ; pointer < len(text); pointer++ {
		c := text[pointer]
		switch c {
		case '<':
			if state == notTag {
				state = tagStart
				tag = tag[:0]
			}
			if !stripHTML {
				out = append(out, c)
			}
		case '>':
			if state == tagStart || state == tagAttributes {
				state = notTag
				name := jsval.ToLowerCase(jsstr.FromUTF16(tag))
				if name == "/p" && stripHTML {
					out = append(out, ' ')
				}
				if !slices.Contains(selfClosingTags, name) && !slices.Contains(selfClosingTags, name+"/") {
					if strings.Contains(name, "/") {
						if len(tagStack) > 0 {
							tagStack = tagStack[:len(tagStack)-1]
						}
					} else {
						tagStack = append(tagStack, name)
					}
				}
			}
			if !stripHTML {
				out = append(out, c)
			}
		case ' ':
			if state == tagStart {
				state = tagAttributes
			}
			if state == notTag {
				chars++
			}
			if state == notTag || !stripHTML {
				out = append(out, c)
			}
		default:
			if state == notTag {
				chars++
			}
			if state == tagStart {
				tag = append(tag, c)
			}
			if state == notTag || !stripHTML {
				out = append(out, c)
			}
		}
		if limit <= chars && (strict || endsWord(text, pointer)) {
			break
		}
	}
	if !stripHTML {
		for len(tagStack) > 0 {
			name := tagStack[len(tagStack)-1]
			tagStack = tagStack[:len(tagStack)-1]
			if name != "!--" {
				out = append(out, jsstr.ToUTF16("</"+name+">")...)
			}
		}
	}
	result := jsstr.FromUTF16(out)
	if pointer < len(text)-1 {
		if reClosingPara.Test(result) {
			result = reClosingParaCap.ReplaceString(result, suffix+"$1")
		} else {
			result += suffix
		}
	}
	return jsstr.TrimSpace(result)
}

// endsWord is truncatise's isEndOfWord when Strict is false: the code unit at
// i or the one after it (or "" past the end) is not a word character.
func endsWord(text []uint16, i int) bool {
	next := ""
	if i+1 < len(text) {
		next = jsstr.FromUTF16(text[i+1 : i+2])
	}
	return !reWordChar.Test(jsstr.FromUTF16(text[i:i+1])) || !reWordChar.Test(next)
}

// Truncate is string.truncate(sentence, limit, {completeWords, suffix}) on a
// string: sentence trimmed and cut after limit characters (counted outside
// HTML tags, in UTF-16 code units, the tags it leaves open closed), at the end
// of the word when completeWords is set, suffix appended when it was cut.
// @poppinss/string's suffix is "...".
func Truncate(sentence string, limit float64, completeWords bool, suffix string) string {
	return mustString(truncatise(nil, jsstr.TrimSpace(sentence), limit, !completeWords, false, suffix))
}

// Excerpt is string.excerpt(sentence, limit, {completeWords, suffix}) on a
// string: Truncate on the text with its HTML tags and comments removed.
func Excerpt(sentence string, limit float64, completeWords bool, suffix string) string {
	return mustString(truncatise(nil, jsstr.TrimSpace(sentence), limit, !completeWords, true, suffix))
}

// ToUnixSlash is string.toUnixSlash(path): backslashes turned into forward
// slashes, except in a Windows extended-length path (one starting with \\?\).
func ToUnixSlash(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	return strings.ReplaceAll(path, `\`, "/")
}

// JSTruncate is the truncate global: (value, length = 20, options) =>
// string.truncate(value, length, {completeWords: options.completeWords !==
// undefined ? options.completeWords : !options.strict, suffix: options.suffix})
// with options defaulting to {}.
func JSTruncate(ops *jsval.Ops, args []any) (any, error) {
	return truncateGlobal(ops, args, false)
}

// JSExcerpt is the excerpt global, JSTruncate's twin calling string.excerpt.
func JSExcerpt(ops *jsval.Ops, args []any) (any, error) {
	return truncateGlobal(ops, args, true)
}

// truncateGlobal runs the truncate or excerpt global: the global's option
// reads (completeWords twice when it is defined, then strict, then suffix),
// truncatise's `(text || "").trim()` and its option normalization.
func truncateGlobal(ops *jsval.Ops, args []any, stripHTML bool) (any, error) {
	value, length, options := js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2)
	if length == js.Undefined {
		length = 20.0
	}
	if !js.ToBoolean(options) {
		options = js.NewObject()
	}
	completeWords, err := ops.Get(options, "completeWords")
	if err != nil {
		return nil, err
	}
	if completeWords != js.Undefined {
		if completeWords, err = ops.Get(options, "completeWords"); err != nil {
			return nil, err
		}
	} else {
		strictOption, err := ops.Get(options, "strict")
		if err != nil {
			return nil, err
		}
		completeWords = !js.ToBoolean(strictOption)
	}
	suffixOption, err := ops.Get(options, "suffix")
	if err != nil {
		return nil, err
	}
	if !js.ToBoolean(value) {
		value = ""
	}
	text, err := stringCall(ops, value, "trim", `(text || "").trim`, nil, func(s string) any { return jsstr.TrimSpace(s) })
	if err != nil {
		return nil, err
	}
	limit := float64(truncatiseLength)
	if n, ok := length.(float64); ok {
		limit = n
	}
	suffix := defaultSuffix
	if s, ok := suffixOption.(string); ok {
		suffix = s
	}
	return truncatise(ops, text, limit, completeWords != true, stripHTML, suffix)
}
