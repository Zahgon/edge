package escapegoat

import "testing"

// The expected values were printed by escape-goat 4.0.0's htmlEscape on Node 24.
func TestHTMLEscape(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"a<b>&\"c'd`e", "a&lt;b&gt;&amp;&quot;c&#39;d`e"},
		{"é😀<&amp;>", "é😀&lt;&amp;amp;&gt;"},
	} {
		if got := HTMLEscape(c.in); got != c.want {
			t.Errorf("HTMLEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
