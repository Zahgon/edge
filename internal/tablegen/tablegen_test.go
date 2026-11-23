package tablegen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		field  string
		lo, hi rune
		ok     bool
	}{
		{"0041..005A", 0x41, 0x5A, true},
		{"00E9", 0xE9, 0xE9, true},
		{"1F600", 0x1F600, 0x1F600, true},
		{"110000", 0, 0, false},
		{"00G1", 0, 0, false},
	}
	for _, tt := range tests {
		lo, hi, err := ParseRange(tt.field)
		if (err == nil) != tt.ok || tt.ok && (lo != tt.lo || hi != tt.hi) {
			t.Errorf("ParseRange(%q) = %U, %U, %v", tt.field, lo, hi, err)
		}
	}
	if got, err := ParseCodePoints("0041 0301"); err != nil || !reflect.DeepEqual(got, []rune{0x41, 0x301}) {
		t.Errorf("ParseCodePoints = %U, %v", got, err)
	}
}

func TestReadRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Props.txt")
	data := "# a comment\n\n0041..005A ; Upper # LATIN\n00DF;Lower\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	var got [][]string
	err := ReadRecords(path, func(fields []string) error {
		got = append(got, fields)
		return nil
	})
	want := [][]string{{"0041..005A", "Upper"}, {"00DF", "Lower"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ReadRecords = %q, %v, want %q", got, err, want)
	}
}

func TestJSONAndGo(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(jsonPath, []byte(`{"names": ["a", "b"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var v struct{ Names []string }
	if err := ReadJSON(jsonPath, &v); err != nil || !reflect.DeepEqual(v.Names, []string{"a", "b"}) {
		t.Errorf("ReadJSON = %v, %v", v, err)
	}
	goPath := filepath.Join(dir, "x_gen.go")
	if err := WriteGo(goPath, []byte("package x\nvar  a = map[string]int{\"b\":1,\n}\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(goPath)
	if want := "package x\n\nvar a = map[string]int{\"b\": 1}\n"; err != nil || string(got) != want {
		t.Errorf("WriteGo wrote %q, %v, want %q", got, err, want)
	}
}
