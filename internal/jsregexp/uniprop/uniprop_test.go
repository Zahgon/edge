package uniprop

import "testing"

// TestDerived checks the derived predicates on code points where they already
// agree with Node 24's \p{...} (the expected values are Node's), so no fix
// table is involved.
func TestDerived(t *testing.T) {
	tests := []struct {
		key  string
		r    rune
		want bool
	}{
		{"gc=Lu", 'A', true},
		{"gc=Lu", 'a', false},
		{"gc=LC", 0x01C5, true},
		{"gc=Cn", 0x0378, true},
		{"gc=Cn", 'A', false},
		{"bin=Alphabetic", 'é', true},
		{"bin=Alphabetic", '1', false},
		{"bin=ASCII", 0x7F, true},
		{"bin=ASCII", 0x80, false},
		{"bin=ID_Start", '_', false},
		{"bin=ID_Continue", '_', true},
		{"bin=ID_Start", 0x2118, true},
		{"bin=White_Space", 0x3000, true},
		{"bin=Changes_When_Uppercased", 'a', true},
		{"bin=Changes_When_Uppercased", 'A', false},
		{"bin=Math", '+', true},
		{"bin=Lowercase", 'ª', true},
		{"bin=Any", 0x10FFFF, true},
		{"sc=Greek", 'α', true},
		{"sc=Latin", 'α', false},
		{"sc=Unknown", 0x0378, true},
		{"sc=Unknown", 'a', false},
	}
	for _, tt := range tests {
		if got := Derived(tt.key)(tt.r); got != tt.want {
			t.Errorf("Derived(%q)(%U) = %v, want %v", tt.key, tt.r, got, tt.want)
		}
	}
}
