package unorm

import "testing"

// The expected values are s.normalize("NFD") and s.normalize("NFC") recorded on
// Node 24 (ICU 78.3, Unicode 17.0).
func TestNormalize(t *testing.T) {
	tests := []struct {
		name, s, nfd, nfc string
	}{
		{"precomposed e with acute", "\u00e9", "e\u0301", "\u00e9"},
		{"e and a combining acute", "e\u0301", "e\u0301", "\u00e9"},
		{"ANGSTROM SIGN, a singleton decomposition", "\u212b", "A\u030a", "\u00c5"},
		{"a Hangul syllable", "\ud55c", "\u1112\u1161\u11ab", "\ud55c"},
		{"conjoining Hangul jamo", "\u1112\u1161\u11ab", "\u1112\u1161\u11ab", "\ud55c"},
		{"marks reordered by combining class", "d\u0307\u0323", "d\u0323\u0307", "\u1e0d\u0307"},
		{"long s with dot above, and dot below", "\u1e9b\u0323", "\u017f\u0323\u0307", "\u1e9b\u0323"},
		{"COMBINING GREEK DIALYTIKA TONOS", "\u0344", "\u0308\u0301", "\u0308\u0301"},
		{"a composition exclusion", "\u0958", "\u0915\u093c", "\u0915\u093c"},
		{"a compatibility ligature, unchanged", "\ufb01", "\ufb01", "\ufb01"},
		{"a lone surrogate", "\xed\xa0\x80\u00e9", "\xed\xa0\x80e\u0301", "\xed\xa0\x80\u00e9"},
		{"ASCII", "plain ascii", "plain ascii", "plain ascii"},
		{"A with ring above and cedilla", "A\u030a\u0327", "A\u0327\u030a", "\u00c5\u0327"},
		{"A WITH RING ABOVE and a cedilla", "\u00c5\u0327", "A\u0327\u030a", "\u00c5\u0327"},
		{"a leading and a vowel jamo", "\u1100\u1161", "\u1100\u1161", "\uac00"},
		{"marks reordered after a letter that composes with none", "q\u0307\u0323\u0301", "q\u0323\u0307\u0301", "q\u0323\u0307\u0301"},
		{"a non-starter decomposition", "\u0f73", "\u0f71\u0f72", "\u0f71\u0f72"},
		{"MUSICAL SYMBOL HALF NOTE, excluded from composition", "\U0001d15e", "\U0001d157\U0001d165", "\U0001d157\U0001d165"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NFD(tt.s); got != tt.nfd {
				t.Errorf("NFD(%+q) = %+q, want %+q", tt.s, got, tt.nfd)
			}
			if got := NFC(tt.s); got != tt.nfc {
				t.Errorf("NFC(%+q) = %+q, want %+q", tt.s, got, tt.nfc)
			}
		})
	}
}
