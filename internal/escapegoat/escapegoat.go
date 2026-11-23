// Package escapegoat is escape-goat 4.0.0's htmlEscape on a string, the
// escaping stringify-attributes applies to attribute names and values.
package escapegoat

import "strings"

// escaper performs escape-goat's chain of replacements: & first, then ", ', <
// and >. Each replacement's output holds no character a later one matches, so
// one pass gives the same text.
var escaper = strings.NewReplacer(
	"&", "&amp;",
	`"`, "&quot;",
	"'", "&#39;",
	"<", "&lt;",
	">", "&gt;",
)

// HTMLEscape is htmlEscape(s) for a string s.
func HTMLEscape(s string) string {
	return escaper.Replace(s)
}
