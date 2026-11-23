package strutil

import (
	"github.com/edge-js/edge-go/internal/jsregexp"
	"github.com/edge-js/edge-go/internal/jsrt/jsstr"
	"github.com/edge-js/edge-go/internal/jsval"
	"github.com/edge-js/edge-go/js"
)

// The pluralize 8.0.0 package with the rules @poppinss/string adds to it.

// pluralRule is a [RegExp, replacement] pair of pluralize's rule lists.
type pluralRule struct {
	re          *jsregexp.Regexp
	replacement string
}

// irregulars are pluralize's irregular rules, [single, plural], already lowercase
// as addIrregularRule stores them.
var irregulars = [][2]string{
	// Pronouns.
	{"i", "we"}, {"me", "us"}, {"he", "they"}, {"she", "they"}, {"them", "them"},
	{"myself", "ourselves"}, {"yourself", "yourselves"}, {"itself", "themselves"},
	{"herself", "themselves"}, {"himself", "themselves"}, {"themself", "themselves"},
	{"is", "are"}, {"was", "were"}, {"has", "have"}, {"this", "these"}, {"that", "those"},
	// Words ending in with a consonant and `o`.
	{"echo", "echoes"}, {"dingo", "dingoes"}, {"volcano", "volcanoes"}, {"tornado", "tornadoes"},
	{"torpedo", "torpedoes"},
	// Ends with `us`.
	{"genus", "genera"}, {"viscus", "viscera"},
	// Ends with `ma`.
	{"stigma", "stigmata"}, {"stoma", "stomata"}, {"dogma", "dogmata"}, {"lemma", "lemmata"},
	{"schema", "schemata"}, {"anathema", "anathemata"},
	// Other irregular rules.
	{"ox", "oxen"}, {"axe", "axes"}, {"die", "dice"}, {"yes", "yeses"}, {"foot", "feet"},
	{"eave", "eaves"}, {"goose", "geese"}, {"tooth", "teeth"}, {"quiz", "quizzes"},
	{"human", "humans"}, {"proof", "proofs"}, {"carve", "carves"}, {"valve", "valves"},
	{"looey", "looies"}, {"thief", "thieves"}, {"groove", "grooves"}, {"pickaxe", "pickaxes"},
	{"passerby", "passersby"},
}

// irregularSingles maps a single to its plural, irregularPlurals a plural to its
// single; a later rule overwrites an earlier one, as with the objects of the
// package.
var irregularSingles, irregularPlurals = func() (map[string]string, map[string]string) {
	singles, plurals := map[string]string{}, map[string]string{}
	for _, r := range irregulars {
		singles[r[0]] = r[1]
		plurals[r[1]] = r[0]
	}
	return singles, plurals
}()

// uncountableWords are the string uncountable rules (stored lowercase).
var uncountableWords = map[string]bool{}

func init() {
	for _, w := range []string{
		"adulthood", "advice", "agenda", "aid", "aircraft", "alcohol", "ammo", "analytics", "anime",
		"athletics", "audio", "bison", "blood", "bream", "buffalo", "butter", "carp", "cash", "chassis",
		"chess", "clothing", "cod", "commerce", "cooperation", "corps", "debris", "diabetes", "digestion",
		"elk", "energy", "equipment", "excretion", "expertise", "firmware", "flounder", "fun", "gallows",
		"garbage", "graffiti", "hardware", "headquarters", "health", "herpes", "highjinks", "homework",
		"housework", "information", "jeans", "justice", "kudos", "labour", "literature", "machinery",
		"mackerel", "mail", "media", "mews", "moose", "music", "mud", "manga", "news", "only", "personnel",
		"pike", "plankton", "pliers", "police", "pollution", "premises", "rain", "research", "rice",
		"salmon", "scissors", "series", "sewage", "shambles", "shrimp", "software", "species", "staff",
		"swine", "tennis", "traffic", "transportation", "trout", "tuna", "wealth", "welfare", "whiting",
		"wildebeest", "wildlife", "you",
	} {
		uncountableWords[w] = true
	}
}

// rules builds a rule list from [source, replacement] pairs of regular
// expression literals with the i flag.
func rules(pairs ...[2]string) []pluralRule {
	out := make([]pluralRule, len(pairs))
	for i, p := range pairs {
		out[i] = pluralRule{jsval.MustCompile(p[0], "i"), p[1]}
	}
	return out
}

// uncountableRules are the regular expression uncountable rules, which
// addUncountableRule adds to both lists with the replacement "$0".
var uncountableRules = rules(
	[2]string{`pok[eé]mon$`, "$0"},
	[2]string{`[^aeiou]ese$`, "$0"}, // "chinese", "japanese"
	[2]string{`deer$`, "$0"},        // "deer", "reindeer"
	[2]string{`fish$`, "$0"},        // "fish", "blowfish", "angelfish"
	[2]string{`measles$`, "$0"},
	[2]string{`o[iu]s$`, "$0"}, // "carnivorous"
	[2]string{`pox$`, "$0"},    // "chickpox", "smallpox"
	[2]string{`sheep$`, "$0"},
)

// poppinssWords are the words @poppinss/string's pluralize.ts adds rules for: a
// plural rule `(singular)$` → `$1suffix` and a singular rule `(singular)suffix$`
// → `$1` (none of the words holds a character escapeRegExp changes).
var poppinssWords = [][2]string{
	{"bias", "biases"}, {"canvas", "canvases"}, {"cache", "caches"}, {"cliche", "cliches"},
	{"fez", "fezzes"}, {"lens", "lenses"}, {"niche", "niches"}, {"quiche", "quiches"},
	{"veto", "vetoes"},
}

// pluralRules and singularRules are the rule lists in the order they were added
// (the package's rules, its uncountable expressions, then @poppinss/string's);
// sanitizeWord tries them from the last.
var pluralRules, singularRules = func() ([]pluralRule, []pluralRule) {
	plural := rules(
		[2]string{`s?$`, "s"},
		[2]string{`[^\u0000-\u007F]$`, "$0"},
		[2]string{`([^aeiou]ese)$`, "$1"},
		[2]string{`(ax|test)is$`, "$1es"},
		[2]string{`(alias|[^aou]us|t[lm]as|gas|ris)$`, "$1es"},
		[2]string{`(e[mn]u)s?$`, "$1s"},
		[2]string{`([^l]ias|[aeiou]las|[ejzr]as|[iu]am)$`, "$1"},
		[2]string{`(alumn|syllab|vir|radi|nucle|fung|cact|stimul|termin|bacill|foc|uter|loc|strat)(?:us|i)$`, "$1i"},
		[2]string{`(alumn|alg|vertebr)(?:a|ae)$`, "$1ae"},
		[2]string{`(seraph|cherub)(?:im)?$`, "$1im"},
		[2]string{`(her|at|gr)o$`, "$1oes"},
		[2]string{`(agend|addend|millenni|dat|extrem|bacteri|desiderat|strat|candelabr|errat|ov|symposi|curricul|automat|quor)(?:a|um)$`, "$1a"},
		[2]string{`(apheli|hyperbat|periheli|asyndet|noumen|phenomen|criteri|organ|prolegomen|hedr|automat)(?:a|on)$`, "$1a"},
		[2]string{`sis$`, "ses"},
		[2]string{`(?:(kni|wi|li)fe|(ar|l|ea|eo|oa|hoo)f)$`, "$1$2ves"},
		[2]string{`([^aeiouy]|qu)y$`, "$1ies"},
		[2]string{`([^ch][ieo][ln])ey$`, "$1ies"},
		[2]string{`(x|ch|ss|sh|zz)$`, "$1es"},
		[2]string{`(matr|cod|mur|sil|vert|ind|append)(?:ix|ex)$`, "$1ices"},
		[2]string{`\b((?:tit)?m|l)(?:ice|ouse)$`, "$1ice"},
		[2]string{`(pe)(?:rson|ople)$`, "$1ople"},
		[2]string{`(child)(?:ren)?$`, "$1ren"},
		[2]string{`eaux$`, "$0"},
		[2]string{`m[ae]n$`, "men"},
		// The string rule 'thou', which sanitizeRule turns into new RegExp('^thou$', 'i').
		[2]string{`^thou$`, "you"},
	)
	singular := rules(
		[2]string{`s$`, ""},
		[2]string{`(ss)$`, "$1"},
		[2]string{`(wi|kni|(?:after|half|high|low|mid|non|night|[^\w]|^)li)ves$`, "$1fe"},
		[2]string{`(ar|(?:wo|[ae])l|[eo][ao])ves$`, "$1f"},
		[2]string{`ies$`, "y"},
		[2]string{`\b([pl]|zomb|(?:neck|cross)?t|coll|faer|food|gen|goon|group|lass|talk|goal|cut)ies$`, "$1ie"},
		[2]string{`\b(mon|smil)ies$`, "$1ey"},
		[2]string{`\b((?:tit)?m|l)ice$`, "$1ouse"},
		[2]string{`(seraph|cherub)im$`, "$1"},
		[2]string{`(x|ch|ss|sh|zz|tto|go|cho|alias|[^aou]us|t[lm]as|gas|(?:her|at|gr)o|[aeiou]ris)(?:es)?$`, "$1"},
		[2]string{`(analy|diagno|parenthe|progno|synop|the|empha|cri|ne)(?:sis|ses)$`, "$1sis"},
		[2]string{`(movie|twelve|abuse|e[mn]u)s$`, "$1"},
		[2]string{`(test)(?:is|es)$`, "$1is"},
		[2]string{`(alumn|syllab|vir|radi|nucle|fung|cact|stimul|termin|bacill|foc|uter|loc|strat)(?:us|i)$`, "$1us"},
		[2]string{`(agend|addend|millenni|dat|extrem|bacteri|desiderat|strat|candelabr|errat|ov|symposi|curricul|quor)a$`, "$1um"},
		[2]string{`(apheli|hyperbat|periheli|asyndet|noumen|phenomen|criteri|organ|prolegomen|hedr|automat)a$`, "$1on"},
		[2]string{`(alumn|alg|vertebr)ae$`, "$1a"},
		[2]string{`(cod|mur|sil|vert|ind)ices$`, "$1ex"},
		[2]string{`(matr|append)ices$`, "$1ix"},
		[2]string{`(pe)(rson|ople)$`, "$1rson"},
		[2]string{`(child)ren$`, "$1"},
		[2]string{`(eau)x?$`, "$1"},
		[2]string{`men$`, "man"},
	)
	plural = append(plural, uncountableRules...)
	singular = append(singular, uncountableRules...)
	for _, w := range poppinssWords {
		suffix := w[1][len(w[0]):]
		plural = append(plural, pluralRule{jsval.MustCompile("("+w[0]+")$", "i"), "$1" + suffix})
		singular = append(singular, pluralRule{jsval.MustCompile("("+w[0]+")"+suffix+"$", "i"), "$1"})
	}
	return plural, singular
}()

// interpolateRef is pluralize's interpolate pattern: $ and one or two digits.
var interpolateRef = jsval.MustCompile(`\$(\d{1,2})`, "g")

// Plural is pluralize.plural(word).
func Plural(word string) string {
	return mustString(replaceWord(nil, word, irregularSingles, irregularPlurals, pluralRules))
}

// Singular is pluralize.singular(word).
func Singular(word string) string {
	return mustString(replaceWord(nil, word, irregularPlurals, irregularSingles, singularRules))
}

// Pluralize is string.pluralize(word, count, inclusive): the singular form when
// count is 1, the plural otherwise, prefixed with the count and a space when
// inclusive is set.
func Pluralize(word string, count float64, inclusive bool) string {
	out := Plural(word)
	if count == 1 {
		out = Singular(word)
	}
	if inclusive {
		return js.NumberToString(count) + " " + out
	}
	return out
}

// mustString unwraps the result of a pluralize function applied to a string,
// which runs no JavaScript code and so cannot throw.
func mustString(v any, err error) string {
	if err != nil {
		panic(err)
	}
	return v.(string)
}

// JSPluralize is the pluralize global: string.pluralize(word, count, inclusive),
// that is `(inclusive ? count + " " : "") + (count === 1 ? singular(word) :
// plural(word))`.
func JSPluralize(ops *jsval.Ops, args []any) (any, error) {
	word, count, inclusive := js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2)
	var (
		out any
		err error
	)
	if js.StrictEquals(count, 1.0) {
		out, err = replaceWord(ops, word, irregularPlurals, irregularSingles, singularRules)
	} else {
		out, err = replaceWord(ops, word, irregularSingles, irregularPlurals, pluralRules)
	}
	if err != nil {
		return nil, err
	}
	prefix := any("")
	if js.ToBoolean(inclusive) {
		if prefix, err = ops.Add(count, " "); err != nil {
			return nil, err
		}
	}
	return ops.Add(prefix, out)
}

// replaceWord is the function pluralize's replaceWord returns, applied to word.
// ops may be nil when word is a string.
func replaceWord(ops *jsval.Ops, word any, replaceMap, keepMap map[string]string, rules []pluralRule) (any, error) {
	lowered, err := stringCall(ops, word, "toLowerCase", "word.toLowerCase", nil, func(s string) any { return jsval.ToLowerCase(s) })
	if err != nil {
		return nil, err
	}
	token, err := templateString(ops, lowered)
	if err != nil {
		return nil, err
	}
	if _, ok := keepMap[token]; ok {
		return restoreCase(ops, word, token)
	}
	if repl, ok := replaceMap[token]; ok {
		return restoreCase(ops, word, repl)
	}
	return sanitizeWord(ops, token, word, rules)
}

// sanitizeWord is pluralize's sanitizeWord: word unchanged when the token is empty
// or uncountable, else word rewritten by the last rule that matches it.
func sanitizeWord(ops *jsval.Ops, token string, word any, rules []pluralRule) (any, error) {
	if token == "" || uncountableWords[token] {
		return word, nil
	}
	for i := len(rules) - 1; i >= 0; i-- {
		subject, err := templateString(ops, word)
		if err != nil {
			return nil, err
		}
		if rules[i].re.Test(subject) {
			return applyRule(ops, word, rules[i])
		}
	}
	return word, nil
}

// applyRule is pluralize's replace: word.replace(rule, fn) where fn interpolates
// the rule's replacement with the replacer's arguments and restores the case of
// the match (or, for an empty match, of the character before it).
func applyRule(ops *jsval.Ops, word any, rule pluralRule) (any, error) {
	replacer := func(arguments []any) (string, error) {
		result, err := interpolate(rule.replacement, arguments)
		if err != nil {
			return "", err
		}
		match := arguments[0]
		if js.StrictEquals(match, "") {
			index := js.ToNumber(js.Arg(arguments, len(arguments)-2))
			before, err := elementOf(ops, word, index-1)
			if err != nil {
				return "", err
			}
			return restoreCase(ops, before, result)
		}
		return restoreCase(ops, match, result)
	}
	if s, ok := word.(string); ok {
		return ReplaceFunc(rule.re, s, func(captures []any, index int) (string, error) {
			return replacer(append(captures, float64(index), s))
		})
	}
	reObj, err := realmRegExp(ops, rule.re)
	if err != nil {
		return nil, err
	}
	fn := jsval.NewHostFunction("", 2, func(_ any, arguments []any) (any, error) {
		return replacer(arguments)
	})
	return ops.Invoke(word, "replace", []any{reObj, fn}, "word.replace")
}

// elementOf is word[index].
func elementOf(ops *jsval.Ops, word any, index float64) (any, error) {
	if s, ok := word.(string); ok {
		if index < 0 || index != float64(int(index)) {
			return js.Undefined, nil
		}
		return unitAt(jsstr.ToUTF16(s), int(index)), nil
	}
	return ops.Get(word, js.NumberToString(index))
}

// interpolate is pluralize's interpolate: each $n of str replaced by args[n],
// or by the empty string when that is falsy.
func interpolate(str string, args []any) (string, error) {
	return ReplaceFunc(interpolateRef, str, func(c []any, _ int) (string, error) {
		i, ok := js.ArrayIndex(c[1].(string))
		if !ok || i >= len(args) || !js.ToBoolean(args[i]) {
			return "", nil
		}
		return js.ToString(args[i]), nil
	})
}

// restoreCase is pluralize's restoreCase: token cased like word (exact, lower,
// upper or title case). word is any value; ops may be nil when it is a string.
func restoreCase(ops *jsval.Ops, word any, token string) (string, error) {
	if js.StrictEquals(word, token) {
		return token, nil
	}
	lowered, err := stringCall(ops, word, "toLowerCase", "word.toLowerCase", nil, func(s string) any { return jsval.ToLowerCase(s) })
	if err != nil {
		return "", err
	}
	if js.StrictEquals(word, lowered) {
		return jsval.ToLowerCase(token), nil
	}
	uppered, err := stringCall(ops, word, "toUpperCase", "word.toUpperCase", nil, func(s string) any { return jsval.ToUpperCase(s) })
	if err != nil {
		return "", err
	}
	if js.StrictEquals(word, uppered) {
		return jsval.ToUpperCase(token), nil
	}
	first, err := elementOf(ops, word, 0)
	if err != nil {
		return "", err
	}
	firstUpper, err := stringCall(ops, first, "toUpperCase", "word[0].toUpperCase", nil, func(s string) any { return jsval.ToUpperCase(s) })
	if err != nil {
		return "", err
	}
	if js.StrictEquals(first, firstUpper) {
		u := jsstr.ToUTF16(token)
		return jsval.ToUpperCase(charAt(u, 0)) + jsval.ToLowerCase(jsstr.FromUTF16(u[min(1, len(u)):])), nil
	}
	return jsval.ToLowerCase(token), nil
}
