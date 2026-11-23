package fixture

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeGzip writes data, gzip-compressed, to a new file and returns its path.
func writeGzip(t *testing.T, data string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cases.json.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDecodeGzipJSON(t *testing.T) {
	tests := []struct {
		name, data string
		want       any
	}{
		{"array of cases", `[["1 + 1", "2"], ["x", null, "ReferenceError: x is not defined"]]`,
			[]any{[]any{"1 + 1", "2"}, []any{"x", nil, "ReferenceError: x is not defined"}}},
		{"object", `{"file": "a.edge", "count": 3}`, map[string]any{"file": "a.edge", "count": 3.0}},
		{"empty array", `[]`, []any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			if err := DecodeGzipJSON(writeGzip(t, tt.data), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReadGzipErrors(t *testing.T) {
	notGzip := filepath.Join(t.TempDir(), "plain.json")
	if err := os.WriteFile(notGzip, []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing.json.gz"), notGzip} {
		if _, err := ReadGzip(path); err == nil {
			t.Errorf("ReadGzip(%s) succeeded", filepath.Base(path))
		}
	}
	var v any
	if err := DecodeGzipJSON(writeGzip(t, `[1,`), &v); err == nil {
		t.Error("DecodeGzipJSON of truncated JSON succeeded")
	}
}
