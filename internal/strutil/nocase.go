package strutil

import (
	"strings"
	"sync"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// @poppinss/string's change_case.ts: noCase (from the no-case package) and
// titleCase (from the title-case package).
var (
	noCaseSplit = []*jsregexp.Regexp{
		jsval.MustCompile(`([a-z0-9])([A-Z])`, "g"),
		jsval.MustCompile(`([A-Z])([A-Z][a-z])`, "g"),
	}
	noCaseStrip = jsval.MustCompile(`[^A-Z0-9]+`, "gi")

	smallWords   = jsval.MustCompile(`\b(?:an?d?|a[st]|because|but|by|en|for|i[fn]|neither|nor|o[fnr]|only|over|per|so|some|tha[tn]|the|to|up|upon|vs?\.?|versus|via|when|with|without|yet)\b`, "i")
	titleTokens  = jsval.MustCompile(`[^\s:–—-]+|.`, "g")
	isManualCase = jsval.MustCompile(`.(?=[A-Z]|\..)`, "")
	alphanumeric = jsval.MustCompile(`[A-Za-z0-9\u00C0-\u00FF]`, "")
)

// noCaseSplitReplacement is the replacement noCase inserts between two words: a
// NUL separator.
const noCaseSplitReplacement = "$1\x00$2"

// noCaseWords is noCase up to its split: the words of value separated by NULs,
// with the separators at both ends removed.
func noCaseWords(s string) []string {
	for _, re := range noCaseSplit {
		s = re.ReplaceString(s, noCaseSplitReplacement)
	}
	s = noCaseStrip.ReplaceString(s, "\x00")
	return jsstr.Split(strings.Trim(s, "\x00"), "\x00", 1<<32-1)
}

// NoCase is string.noCase(value): the words of value, lowercased and joined with
// spaces.
func NoCase(s string) string {
	words := noCaseWords(s)
	for i, w := range words {
		words[i] = jsval.ToLowerCase(w)
	}
	return strings.Join(words, " ")
}

// SentenceCase is string.sentenceCase(value): noCase with the first word's first
// character uppercased (the rest of that word as it is) and the other words
// lowercased.
func SentenceCase(s string) string {
	words := noCaseWords(s)
	for i, w := range words {
		words[i] = sentenceWord(w, i)
	}
	return strings.Join(words, " ")
}

func sentenceWord(w string, i int) string {
	if i == 0 {
		u := jsstr.ToUTF16(w)
		return jsval.ToUpperCase(charAt(u, 0)) + jsstr.FromUTF16(u[min(1, len(u)):])
	}
	return jsval.ToLowerCase(w)
}

// noCaseInput runs noCase's three replace calls on any JavaScript value: the first
// is `input.replace` on the value itself.
func noCaseInput(ops *jsval.Ops, v any) (string, error) {
	callees := []string{"input.replace", "input.replace", "result.replace"}
	patterns := append(append([]*jsregexp.Regexp{}, noCaseSplit...), noCaseStrip)
	replacements := []string{noCaseSplitReplacement, noCaseSplitReplacement, "\x00"}
	for i, re := range patterns {
		if s, ok := v.(string); ok {
			v = re.ReplaceString(s, replacements[i])
			continue
		}
		reObj, err := realmRegExp(ops, re)
		if err != nil {
			return "", err
		}
		if v, err = ops.Invoke(v, "replace", []any{reObj, replacements[i]}, callees[i]); err != nil {
			return "", err
		}
	}
	return templateString(ops, v)
}

// JSNoCase is the noCase global: string.noCase(value, transform). A transform
// other than a falsy value is mapped over the words as Array.prototype.map calls
// it (word, index, words), and the results are joined as Array.prototype.join
// converts them.
func JSNoCase(ops *jsval.Ops, args []any) (any, error) {
	s, err := noCaseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	words := jsstr.Split(strings.Trim(s, "\x00"), "\x00", 1<<32-1)
	transform := js.Arg(args, 1)
	if !js.ToBoolean(transform) {
		for i, w := range words {
			words[i] = jsval.ToLowerCase(w)
		}
		return strings.Join(words, " "), nil
	}
	items := make([]any, len(words))
	for i, w := range words {
		items[i] = w
	}
	mapFn, err := ops.BuiltinFunction("Array", "prototype", "map")
	if err != nil {
		return nil, err
	}
	mapped, err := ops.Realm.Call(mapFn, js.NewArray(items...), []any{transform})
	if err != nil {
		return nil, err
	}
	return ops.Invoke(mapped, "join", []any{" "}, "result.slice(...).split(...).map(...).join")
}

// JSSentenceCase is the sentenceCase global: string.sentenceCase(value).
func JSSentenceCase(ops *jsval.Ops, args []any) (any, error) {
	s, err := noCaseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	words := jsstr.Split(strings.Trim(s, "\x00"), "\x00", 1<<32-1)
	for i, w := range words {
		words[i] = sentenceWord(w, i)
	}
	return strings.Join(words, " "), nil
}

// titleTokensLastIndex is the lastIndex of titleCase's global TOKENS expression.
// The module keeps one expression for every call, so a call that throws midway
// (on a value that is not a string) leaves lastIndex where it stopped, and the
// next call starts its search there.
var titleTokensLastIndex struct {
	sync.Mutex
	n int
}

// TitleCase is string.titleCase(value): every word capitalized, except small
// words (articles, conjunctions, short prepositions) inside the sentence and
// words that already carry inner capitals or dots.
func TitleCase(s string) string {
	out, err := titleCase(nil, s)
	if err != nil {
		// A string input calls no JavaScript code, so nothing can throw.
		panic(err)
	}
	return out
}

// JSTitleCase is the titleCase global: string.titleCase(value).
func JSTitleCase(ops *jsval.Ops, args []any) (any, error) {
	return titleCase(ops, js.Arg(args, 0))
}

// titleCase runs the title-case loop on any value: TOKENS.exec converts the input
// with ToString on every iteration, and input.length and input.charAt are read
// from the value itself. ops may be nil when input is a string.
func titleCase(ops *jsval.Ops, input any) (string, error) {
	titleTokensLastIndex.Lock()
	defer titleTokensLastIndex.Unlock()
	var b strings.Builder
	for {
		s, err := templateString(ops, input)
		if err != nil {
			return "", err
		}
		units := jsstr.ToUTF16(s)
		last := titleTokensLastIndex.n
		var caps []int
		if last <= len(units) {
			caps = titleTokens.Exec(units, last)
		}
		if caps == nil {
			titleTokensLastIndex.n = 0
			return b.String(), nil
		}
		titleTokensLastIndex.n = caps[1]
		token := jsstr.FromUTF16(units[caps[0]:caps[1]])
		index := caps[0]
		end := index + caps[1] - caps[0]
		capitalize, err := titleCapitalizes(ops, input, token, index, end)
		if err != nil {
			return "", err
		}
		if capitalize {
			token, err = ReplaceFunc(alphanumeric, token, func(c []any, _ int) (string, error) {
				return jsval.ToUpperCase(c[0].(string)), nil
			})
			if err != nil {
				return "", err
			}
		}
		b.WriteString(token)
	}
}

// titleCapitalizes evaluates title-case's condition for the token at
// [index, end) of input:
//
//	!IS_MANUAL_CASE.test(token) &&
//	  (!SMALL_WORDS.test(token) || index === 0 || index + token.length === input.length) &&
//	  (input.charAt(index + token.length) !== ":" || WHITESPACE.test(input.charAt(index + token.length + 1)))
func titleCapitalizes(ops *jsval.Ops, input any, token string, index, end int) (bool, error) {
	if isManualCase.Test(token) {
		return false, nil
	}
	if smallWords.Test(token) && index != 0 {
		length, err := lengthOf(ops, input)
		if err != nil {
			return false, err
		}
		if !js.StrictEquals(float64(end), length) {
			return false, nil
		}
	}
	next, err := inputCharAt(ops, input, end)
	if err != nil {
		return false, err
	}
	if !js.StrictEquals(next, ":") {
		return true, nil
	}
	after, err := inputCharAt(ops, input, end+1)
	if err != nil {
		return false, err
	}
	afterStr, err := templateString(ops, after)
	if err != nil {
		return false, err
	}
	return whitespace.Test(afterStr), nil
}

// inputCharAt is input.charAt(i).
func inputCharAt(ops *jsval.Ops, input any, i int) (any, error) {
	return stringCall(ops, input, "charAt", "input.charAt", []any{float64(i)}, func(s string) any {
		return charAt(jsstr.ToUTF16(s), i)
	})
}
