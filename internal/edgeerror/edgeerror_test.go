package edgeerror

import (
	"testing"

	"github.com/edge-js/edge-go/js"
)

// The messages are String(error.message) for `new EdgeError(message, 'E_X',
// { line: 3, col: 4, filename: 'a.edge' })`, recorded with edge-error 4.0.2 on
// Node 24.
func TestError(t *testing.T) {
	tests := []struct {
		name    string
		message any
		want    string
	}{
		{"string", "boom", "boom"},
		{"number", 42.0, "42"},
		{"undefined", js.Undefined, "undefined"},
		{"null", nil, "null"},
		{"object", js.NewObject(), "[object Object]"},
		{"array", js.NewArray(1.0, 2.0), "1,2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.message, "E_X", Options{Line: 3.0, Col: 4.0, Filename: "a.edge"})
			if got := err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
			if err.Code != "E_X" || err.Line != 3.0 || err.Col != 4.0 || err.Filename != "a.edge" {
				t.Errorf("fields %+v", err)
			}
		})
	}
}

func TestAt(t *testing.T) {
	err := At(`Unclosed tag if`, "E_UNCLOSED_TAG", "eval.edge", 4, 5)
	want := EdgeError{Message: "Unclosed tag if", Code: "E_UNCLOSED_TAG", Filename: "eval.edge", Line: 4.0, Col: 5.0}
	if *err != want {
		t.Errorf("At = %+v, want %+v", *err, want)
	}
}
