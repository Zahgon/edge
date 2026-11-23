// Package js models the JavaScript values that Edge templates, their state and
// globals pass around, with the conversions JavaScript applies to them.
package js

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// UndefinedType is the type of Undefined.
type UndefinedType struct{}

// Undefined is JavaScript's undefined. JavaScript null is Go nil.
var Undefined = UndefinedType{}

// Array is a JavaScript array. It is a reference: copies of the pointer share items.
type Array struct {
	Items []any
}

// NewArray returns an array holding items.
func NewArray(items ...any) *Array {
	return &Array{Items: items}
}

// Date is a JavaScript Date object: its time value in milliseconds since the
// epoch, NaN when invalid. It is a reference, as Date objects are; the setters
// change Ms in place.
type Date struct {
	Ms float64
}

// Function is a callable JavaScript value. A function the host writes in Go
// needs only Name, Length and Call.
type Function struct {
	Name string
	// Length is the declared parameter count (Function.prototype.length).
	Length int
	Call   func(this any, args []any) (any, error)
	// Internal holds the internal slots the interpreter keeps for the functions
	// it creates (the code and scope of a function defined in JavaScript, the
	// target of a bound function, the [[Construct]] behaviour of a
	// constructor); nil for a plain host function.
	Internal any
}

// Error is a JavaScript Error object, as thrown.
//
// Message is the message property when it is a string, which it is for every
// error the Error constructors and the runtime create. Code that assigns
// error.message another value stores it in Props under "message", which then
// takes precedence.
type Error struct {
	Name    string
	Message string
	// Orig is the Go error this one reports, when it did not come from
	// JavaScript.
	Orig error
	// Props holds the error's other own properties, such as code or path.
	Props *Object
}

// Error returns the error's message.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the Go error e reports, if any.
func (e *Error) Unwrap() error { return e.Orig }

// Thrown is a JavaScript exception whose value is not an Error object, such as
// `throw 'boom'`. Errors themselves propagate as *Error.
type Thrown struct {
	Value any
}

// Error returns String(value), as an uncaught exception prints it.
func (t *Thrown) Error() string { return ToString(t.Value) }

// Arg returns args[i], or Undefined when the call passed fewer arguments: how a
// JavaScript function reads a missing parameter.
func Arg(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return Undefined
}

// NewError returns an Error with name "Error".
func NewError(msg string) *Error { return &Error{Name: "Error", Message: msg} }

// NewTypeError returns an Error with name "TypeError".
func NewTypeError(msg string) *Error { return &Error{Name: "TypeError", Message: msg} }

// NewReferenceError returns an Error with name "ReferenceError".
func NewReferenceError(msg string) *Error { return &Error{Name: "ReferenceError", Message: msg} }

// NewSyntaxError returns an Error with name "SyntaxError".
func NewSyntaxError(msg string) *Error { return &Error{Name: "SyntaxError", Message: msg} }

// NotAFunction is V8's TypeError for a call whose callee is not a function:
// callee is the callee as V8 prints it, for example NotAFunction("value.join").
func NotAFunction(callee string) *Error { return NewTypeError(callee + " is not a function") }

// CannotReadProperties is V8's TypeError for reading the property key of of,
// which is null or undefined.
func CannotReadProperties(of any, key string) *Error {
	return NewTypeError("Cannot read properties of " + ToString(of) + " (reading '" + key + "')")
}

// Normalize converts Go literals a caller may pass as template data (int, int64,
// float32, []any, map[string]any) into the value model. Maps become objects with
// sorted keys, since a Go map has no order; callers that care about key order
// use *Object.
func Normalize(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case []any:
		items := make([]any, len(x))
		for i, it := range x {
			items[i] = Normalize(it)
		}
		return &Array{Items: items}
	case []string:
		items := make([]any, len(x))
		for i, it := range x {
			items[i] = it
		}
		return &Array{Items: items}
	case map[string]any:
		o := NewObject()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			o.Set(k, Normalize(x[k]))
		}
		return o
	}
	return v
}

// TypeOf is the typeof operator.
func TypeOf(v any) string {
	switch v.(type) {
	case UndefinedType:
		return "undefined"
	case nil:
		return "object"
	case float64:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	case BigInt:
		return "bigint"
	case *Function:
		return "function"
	}
	return "object"
}

// ClassOf is the tag Object.prototype.toString reports for v, as in
// "[object Array]": "Undefined", "Null", "Number", "String", "Boolean", "Array",
// "BigInt", "Function", "Date", "RegExp", "Error", and "Object" for everything
// else.
func ClassOf(v any) string {
	switch v.(type) {
	case UndefinedType:
		return "Undefined"
	case nil:
		return "Null"
	case float64:
		return "Number"
	case string:
		return "String"
	case bool:
		return "Boolean"
	case BigInt:
		return "BigInt"
	case *Array:
		return "Array"
	case *Function:
		return "Function"
	case *Date:
		return "Date"
	case *RegExp:
		return "RegExp"
	case *Error:
		return "Error"
	}
	return "Object"
}

// IsNullish reports whether v is null or undefined.
func IsNullish(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(UndefinedType)
	return ok
}

// ToBoolean is the ToBoolean abstract operation.
func ToBoolean(v any) bool {
	switch x := v.(type) {
	case UndefinedType, nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	case BigInt:
		return x.Sign() != 0
	}
	return true
}

// NumberToString is Number::toString with radix 10.
func NumberToString(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v == 0:
		return "0"
	case v < 0:
		return "-" + NumberToString(-v)
	case math.IsInf(v, 1):
		return "Infinity"
	}
	digits, x := ShortestDigits(v)
	k := len(digits)
	n := x + 1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	magnitude := strconv.Itoa(max(n-1, 1-n))
	if k == 1 {
		return digits + "e" + sign + magnitude
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + magnitude
}

// ShortestDigits returns the shortest decimal digits d1d2...dk that read back
// as the positive finite number x, and the exponent e of x = d1.d2...dk × 10^e:
// the digits Number::toString and toExponential() without an argument use.
func ShortestDigits(x float64) (digits string, exp int) {
	s := strconv.FormatFloat(x, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(s, "e")
	e, err := strconv.Atoi(exponent)
	if err != nil {
		panic("js: strconv wrote the exponent of " + s + " in another form")
	}
	return strings.Replace(mantissa, ".", "", 1), e
}

// RegExp is a JavaScript regular expression object: its source and flags.
// Matching is done by the interpreter, which keeps the compiled matcher and the
// lastIndex property in Internal.
type RegExp struct {
	Source string
	Flags  string
	// Internal holds the interpreter's internal slots for the object; nil
	// until it first uses it.
	Internal any
}
