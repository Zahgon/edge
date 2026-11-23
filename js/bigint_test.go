package js

import "testing"

// TestBigInt checks the BigInt operations of the value model. The expected
// values are what Node v24.22.1 prints for the same operations.
func TestBigInt(t *testing.T) {
	parse := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0x1F", "31", true},
		{" 12 ", "12", true},
		{"", "0", true},
		{"-7", "-7", true},
		{"+12", "12", true},
		{"  0b11  ", "3", true},
		{"0o17", "15", true},
		{"-0x1", "", false},
		{"+0x1", "", false},
		{"1e3", "", false},
		{"1.5", "", false},
		{"Infinity", "", false},
		{"12n", "", false},
	}
	for _, tt := range parse {
		got, ok := StringToBigInt(tt.in)
		if ok != tt.ok || ok && got.String() != tt.want {
			t.Errorf("StringToBigInt(%q) = %v, %v; want %s, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}

	big := func(s string) BigInt {
		b, ok := StringToBigInt(s)
		if !ok {
			t.Fatalf("StringToBigInt(%q) failed", s)
		}
		return b
	}
	binary := []struct {
		op, a, b string
		want     string
	}{
		{"/", "7", "2", "3"},
		{"/", "-7", "2", "-3"},
		{"%", "-7", "2", "-1"},
		{"%", "7", "-2", "1"},
		{"**", "2", "64", "18446744073709551616"},
		{"**", "-1", "3", "-1"},
		{"**", "0", "0", "1"},
		{"**", "1", "1000000000000", "1"},
		{">>", "-5", "1", "-3"},
		{"<<", "5", "2", "20"},
		{"<<", "1", "-1", "0"},
		{">>", "8", "-2", "32"},
		{">>", "-1", "1000", "-1"},
		{"&", "-1", "255", "255"},
		{"^", "6", "3", "5"},
		{"|", "6", "1", "7"},
		{"*", "0", "-1", "0"},
		{"/", "1", "0", "RangeError: Division by zero"},
		{"%", "1", "0", "RangeError: Division by zero"},
		{"**", "2", "-1", "RangeError: undefined must be positive"},
		{"**", "2", "10000000000", "RangeError: Maximum BigInt size exceeded"},
		{"<<", "1", "10000000000", "RangeError: Maximum BigInt size exceeded"},
		{">>>", "1", "0", "TypeError: BigInts have no unsigned right shift, use >> instead"},
	}
	for _, tt := range binary {
		got, err := BigIntBinary(tt.op, big(tt.a), big(tt.b))
		s := got.String()
		if err != nil {
			s = ToString(err.(*Error))
		}
		if s != tt.want {
			t.Errorf("%sn %s %sn = %s; want %s", tt.a, tt.op, tt.b, s, tt.want)
		}
	}

	if got := big("-25").AsIntN(3).String(); got != "-1" {
		t.Errorf("BigInt.asIntN(3, -25n) = %s; want -1", got)
	}
	if got := big("25").AsIntN(3).String(); got != "1" {
		t.Errorf("BigInt.asIntN(3, 25n) = %s; want 1", got)
	}
	if got, _ := big("-5").AsUintN(64); got.String() != "18446744073709551611" {
		t.Errorf("BigInt.asUintN(64, -5n) = %s; want 18446744073709551611", got)
	}
	if got := big("255").Text(16) + big("-255").Text(36); got != "ff-73" {
		t.Errorf("toString(16), toString(36) = %s; want ff-73", got)
	}
	if got := NumberToString(big("18446744073709551616").Float64()); got != "18446744073709552000" {
		t.Errorf("Number(2n ** 64n) = %s; want 18446744073709552000", got)
	}
	if _, err := NumberToBigInt(1.5); err == nil || err.Error() != "The number 1.5 cannot be converted to a BigInt because it is not an integer" {
		t.Errorf("BigInt(1.5) error = %v", err)
	}
	if cmp, ok := CompareBigIntNumber(big("9007199254740993"), 9007199254740992); !ok || cmp != 1 {
		t.Errorf("9007199254740993n > 9007199254740992 = %v, %v; want 1, true", cmp, ok)
	}
	for _, v := range []any{BigIntOf(0), BigIntOf(1)} {
		if TypeOf(v) != "bigint" || IsObject(v) {
			t.Errorf("TypeOf(%v) = %s, IsObject = %v", v, TypeOf(v), IsObject(v))
		}
	}
	if ToBoolean(BigIntOf(0)) || !ToBoolean(BigIntOf(-1)) {
		t.Error("ToBoolean(0n) must be false and ToBoolean(-1n) true")
	}
	if !StrictEquals(big("10"), BigIntOf(10)) {
		t.Error("10n === 10n must hold")
	}
}
