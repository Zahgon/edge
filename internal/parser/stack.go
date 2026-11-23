package parser

import (
	"strings"

	"github.com/edge-js/edge-go/js"
)

// Stack is edge-parser's Stack: the variables a template defines, in nested scopes,
// so that the parser leaves them alone instead of reading them from the state.
//
// Tags define variables by name. A few of them (@let and the migrate plugin's @set)
// define whatever value the expression gives them, which is not always a string
// (a nested destructuring pattern has no name, @set(1, 2) defines the number 1), so
// the stack keeps JavaScript values and compares them with ===.
type Stack struct {
	localVariables []any
	scopes         [][]any
}

// NewStack is `new Stack()`.
func NewStack() *Stack { return &Stack{localVariables: []any{}, scopes: [][]any{}} }

// DefineScope opens a new scope.
func (s *Stack) DefineScope() { s.scopes = append(s.scopes, []any{}) }

// ClearScope removes the most recent scope.
func (s *Stack) ClearScope() {
	if n := len(s.scopes); n > 0 {
		s.scopes = s.scopes[:n-1]
	}
}

// DefineVariable defines a variable in the most recent scope, or at the top level
// when no scope is open.
func (s *Stack) DefineVariable(name string) { s.DefineValue(name) }

// DefineValue is defineVariable with any JavaScript value, as @let and @set pass
// it: js.Undefined for a pattern without a name, or a literal's value.
func (s *Stack) DefineValue(v any) {
	if n := len(s.scopes); n > 0 {
		s.scopes[n-1] = append(s.scopes[n-1], v)
		return
	}
	s.localVariables = append(s.localVariables, v)
}

// Has reports whether a variable is defined in any scope.
func (s *Stack) Has(name string) bool { return s.HasValue(name) }

// HasValue is has() with any JavaScript value.
func (s *Stack) HasValue(v any) bool {
	if inList(s.localVariables, v) {
		return true
	}
	for _, scope := range s.scopes {
		if inList(scope, v) {
			return true
		}
	}
	return false
}

// inList is `!!list.find((listItem) => listItem === item)`: a falsy item is never
// found, since find returns it.
func inList(list []any, item any) bool {
	for _, listItem := range list {
		if js.StrictEquals(listItem, item) {
			return js.ToBoolean(listItem)
		}
	}
	return false
}

// Values returns the defined values, the scopes first (oldest scope first) and
// then the top-level variables, as stack.list() does.
func (s *Stack) Values() []any {
	out := []any{}
	for _, scope := range s.scopes {
		out = append(out, scope...)
	}
	return append(out, s.localVariables...)
}

// List returns the defined variables as strings (String(value) of each), in the
// order of Values.
func (s *Stack) List() []string {
	values := s.Values()
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = js.ToString(v)
	}
	return out
}

// JoinValues is `values.join(sep)`: null and undefined become empty strings.
func JoinValues(values []any, sep string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		if !js.IsNullish(v) {
			parts[i] = js.ToString(v)
		}
	}
	return strings.Join(parts, sep)
}
