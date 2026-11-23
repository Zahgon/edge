// Package htmlent is he 1.2.0's escape: the HTML escaping Edge applies to the
// output of {{ }} mustaches and to html.escape.
package htmlent

import "strings"

// escaper replaces the characters he's regexEscape (/["&'<>`]/g) matches with
// the references of its escapeMap. Strings are WTF-8, so a byte-wise
// replacement of these ASCII characters never touches part of a longer
// character.
var escaper = strings.NewReplacer(
	`"`, "&quot;",
	"&", "&amp;",
	"'", "&#x27;",
	"<", "&lt;",
	// he escapes > and the backtick too: > for unquoted attribute values and
	// XML, ` because old Internet Explorer let it break out of attribute
	// values and comments.
	">", "&gt;",
	"`", "&#x60;",
)

// Escape is he.escape(s).
func Escape(s string) string {
	return escaper.Replace(s)
}
