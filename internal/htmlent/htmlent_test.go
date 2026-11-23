package htmlent

import "testing"

// The expected values were printed by he 1.2.0's escape on Node 24.
func TestEscape(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"a<b>&\"c'd`e", "a&lt;b&gt;&amp;&quot;c&#x27;d&#x60;e"},
		{"é😀<&amp;>", "é😀&lt;&amp;amp;&gt;"},
	} {
		if got := Escape(c.in); got != c.want {
			t.Errorf("Escape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
