package strutil

import (
	"strings"

	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsrt/unicase"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/internal/strutil/unorm"
	"github.com/edge-js/edge-go/js"
)

// case-anything's word splitters (dist/utils.js). magicSplit's ranges cover the
// Latin-1 letters and the basic Cyrillic alphabet.
var (
	magicSplit = jsval.MustCompile(`^[a-zà-öø-ÿа-я]+|[A-ZÀ-ÖØ-ßА-Я][a-zà-öø-ÿа-я]+|[a-zà-öø-ÿа-я]+|[0-9]+|[A-ZÀ-ÖØ-ßА-Я]+(?![a-zà-öø-ÿа-я])`, "g")
	spaceSplit = jsval.MustCompile(`\S+`, "g")
	// specialChars is what splitAndPrefix strips from a decomposed word when
	// special characters are not kept.
	specialChars = jsval.MustCompile(`[^a-zA-ZØßø0-9]`, "g")
	whitespace   = jsval.MustCompile(`\s`, "")
)

// caseInput is the start of case-anything's splitAndPrefix,
// `string.trim().normalize('NFC')`, for any JavaScript value.
func caseInput(ops *jsval.Ops, v any) (string, error) {
	trimmed, err := stringCall(ops, v, "trim", "string.trim", nil, func(s string) any { return jsstr.TrimSpace(s) })
	if err != nil {
		return "", err
	}
	normal, err := stringCall(ops, trimmed, "normalize", "string.trim(...).normalize", []any{"NFC"}, func(s string) any { return unorm.NFC(s) })
	if err != nil {
		return "", err
	}
	return templateString(ops, normal)
}

// splitAndPrefix is case-anything's splitAndPrefix after its first line: the words
// of the trimmed, NFC-normalized string, each with the separator that goes before
// it (the special characters found between words when keepSpecial is set, else
// prefix), empty words dropped. Edge's helpers never pass case-anything's `keep`
// option.
func splitAndPrefix(normal string, keepSpecial bool, prefix string) []string {
	hasSpaces := strings.Contains(normal, " ")
	split := magicSplit
	if hasSpaces {
		split = spaceSplit
	}
	parts, prefixes := partsAndIndexes(normal, split)
	out := make([]string, 0, len(parts))
	for i, part := range parts {
		found := prefixes[i]
		if !keepSpecial {
			part = specialChars.ReplaceString(unorm.NFD(part), "")
			found = ""
		}
		var word string
		switch {
		case i == 0:
			word = found + part
		case found == "" && part == "":
			word = ""
		case !hasSpaces:
			word = or(found, prefix) + part
		case found == "" && whitespace.Test(prefix):
			word = " " + part
		default:
			word = or(found, prefix) + part
		}
		if word != "" {
			out = append(out, word)
		}
	}
	return out
}

// or is `a || b` for strings.
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// partsAndIndexes is case-anything's getPartsAndIndexes: the matches of split in
// s, and before each the trimmed text since the previous match; trailing text
// becomes an empty part with that text as its prefix.
func partsAndIndexes(s string, split *jsregexp.Regexp) (parts, prefixes []string) {
	units := jsstr.ToUTF16(s)
	last := 0
	for _, caps := range matchAll(split, units) {
		parts = append(parts, jsstr.FromUTF16(units[caps[0]:caps[1]]))
		prefixes = append(prefixes, jsstr.TrimSpace(jsstr.FromUTF16(units[last:caps[0]])))
		last = caps[1]
	}
	if tail := jsstr.TrimSpace(jsstr.FromUTF16(units[last:])); tail != "" {
		parts = append(parts, "")
		prefixes = append(prefixes, tail)
	}
	return parts, prefixes
}

// capitaliseWord is case-anything's capitaliseWord: uppercase up to and including
// the first character of the first word match, lowercase after it.
func capitaliseWord(s string) string {
	units := jsstr.ToUTF16(s)
	first := 0
	if caps := magicSplit.Exec(units, 0); caps != nil {
		first = caps[0]
	}
	cut := min(first+1, len(units))
	// The cut may fall inside a surrogate pair; joining the code units pairs
	// the halves again, as JavaScript's concatenation does.
	return jsstr.FromUTF16(append(unicase.ToUpper(units[:cut]), unicase.ToLower(units[cut:])...))
}

// camel, pascal, ... are case-anything's converters over the words of
// splitAndPrefix.
func camel(words []string) string {
	var b strings.Builder
	for i, w := range words {
		if i == 0 || !magicSplit.Test(charAt(jsstr.ToUTF16(w), 0)) {
			b.WriteString(jsval.ToLowerCase(w))
		} else {
			b.WriteString(capitaliseWord(w))
		}
	}
	return b.String()
}

func capitalised(words []string) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(capitaliseWord(w))
	}
	return b.String()
}

// CamelCase is string.camelCase: case-anything's camelCase.
func CamelCase(s string) string {
	return camel(splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), false, ""))
}

// PascalCase is string.pascalCase: case-anything's pascalCase.
func PascalCase(s string) string {
	return capitalised(splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), false, ""))
}

// SnakeCase is string.snakeCase: case-anything's snakeCase.
func SnakeCase(s string) string {
	return jsval.ToLowerCase(strings.Join(splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), false, "_"), ""))
}

// DashCase is string.dashCase: case-anything's trainCase when capitalize is set,
// its kebabCase otherwise.
func DashCase(s string, capitalize bool) string {
	words := splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), false, "-")
	if capitalize {
		return capitalised(words)
	}
	return jsval.ToLowerCase(strings.Join(words, ""))
}

// CapitalCase is string.capitalCase: case-anything's capitalCase, which keeps
// special characters.
func CapitalCase(s string) string {
	return capitalised(splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), true, " "))
}

// DotCase is string.dotCase: case-anything's dotNotation, lowercased when
// lowerCase is set.
func DotCase(s string, lowerCase bool) string {
	out := strings.Join(splitAndPrefix(unorm.NFC(jsstr.TrimSpace(s)), false, "."), "")
	if lowerCase {
		return jsval.ToLowerCase(out)
	}
	return out
}

// JSCamelCase is the camelCase global: string.camelCase(value).
func JSCamelCase(ops *jsval.Ops, args []any) (any, error) {
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	return camel(splitAndPrefix(normal, false, "")), nil
}

// JSPascalCase is the pascalCase global: string.pascalCase(value).
func JSPascalCase(ops *jsval.Ops, args []any) (any, error) {
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	return capitalised(splitAndPrefix(normal, false, "")), nil
}

// JSSnakeCase is the snakeCase global: string.snakeCase(value).
func JSSnakeCase(ops *jsval.Ops, args []any) (any, error) {
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	return jsval.ToLowerCase(strings.Join(splitAndPrefix(normal, false, "_"), "")), nil
}

// JSDashCase is the dashCase global: string.dashCase(value, options), which
// reads options.capitalize first.
func JSDashCase(ops *jsval.Ops, args []any) (any, error) {
	capitalize, err := getIfTruthy(ops, js.Arg(args, 1), "capitalize")
	if err != nil {
		return nil, err
	}
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	words := splitAndPrefix(normal, false, "-")
	if js.ToBoolean(capitalize) {
		return capitalised(words), nil
	}
	return jsval.ToLowerCase(strings.Join(words, "")), nil
}

// JSCapitalCase is the capitalCase global: string.capitalCase(value).
func JSCapitalCase(ops *jsval.Ops, args []any) (any, error) {
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	return capitalised(splitAndPrefix(normal, true, " ")), nil
}

// JSDotCase is the dotCase global: string.dotCase(value, options), which reads
// options.lowerCase after converting.
func JSDotCase(ops *jsval.Ops, args []any) (any, error) {
	normal, err := caseInput(ops, js.Arg(args, 0))
	if err != nil {
		return nil, err
	}
	out := strings.Join(splitAndPrefix(normal, false, "."), "")
	lowerCase, err := getIfTruthy(ops, js.Arg(args, 1), "lowerCase")
	if err != nil {
		return nil, err
	}
	if js.ToBoolean(lowerCase) {
		return jsval.ToLowerCase(out), nil
	}
	return out, nil
}
