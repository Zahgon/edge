package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/edge-js/edge-go/internal/jsstringify"
)

// EOL is os.EOL on Linux, the line separator of the compiled code.
const EOL = "\n"

// BufferOptions are the options of `new EdgeBuffer(filename, options)`.
type BufferOptions struct {
	// OutputVar is the variable the output is appended to.
	OutputVar string
	// RethrowCallPath is the function the catch block calls with the error, the
	// filename and the line number; its parts are joined with ".".
	RethrowCallPath []string
}

// EdgeBuffer is edge-parser's EdgeBuffer: the lines of a compiled template, written
// by the parser and the tags, and wrapped into the body of the template function by
// Flush.
type EdgeBuffer struct {
	outputFileAndLineNumber bool
	outputOutVariable       bool
	outputReturnStatement   bool
	wrapInsideTryCatch      bool

	outputVar       string
	rethrowCallPath string
	fileNameVar     string
	lineVar         string

	prefix []string
	suffix []string
	buffer []string

	currentLineNumber int
	filename          string
	currentFileName   string

	compiledOutput *string

	outputVariableName string
}

// NewEdgeBuffer is `new EdgeBuffer(filename, { outputVar, rethrowCallPath })`.
func NewEdgeBuffer(filename string, options BufferOptions) *EdgeBuffer {
	return newBuffer(filename, options.OutputVar, strings.Join(options.RethrowCallPath, "."))
}

func newBuffer(filename, outputVar, rethrowCallPath string) *EdgeBuffer {
	return &EdgeBuffer{
		outputFileAndLineNumber: true,
		outputOutVariable:       true,
		outputReturnStatement:   true,
		wrapInsideTryCatch:      true,
		outputVar:               outputVar,
		rethrowCallPath:         rethrowCallPath,
		fileNameVar:             "$filename",
		lineVar:                 "$lineNumber",
		currentLineNumber:       1,
		filename:                filename,
		currentFileName:         filename,
		outputVariableName:      outputVar,
		buffer:                  []string{},
	}
}

// OutputVariableName is the name of the output variable.
func (b *EdgeBuffer) OutputVariableName() string { return b.outputVariableName }

// Size is the number of lines written so far.
func (b *EdgeBuffer) Size() int { return len(b.buffer) }

// Create returns a new buffer for filename with this buffer's options and another
// output variable (buffer.create(filename, { outputVar })).
func (b *EdgeBuffer) Create(filename, outputVar string) *EdgeBuffer {
	return newBuffer(filename, outputVar, b.rethrowCallPath)
}

func (b *EdgeBuffer) setup(buffer []string) []string {
	if b.outputOutVariable {
		buffer = append(buffer, "let "+b.outputVariableName+` = "";`)
	}
	if b.outputFileAndLineNumber {
		buffer = append(buffer, "let "+b.lineVar+" = 1;")
		buffer = append(buffer, "let "+b.fileNameVar+" = "+jsstringify.String(b.filename)+";")
	}
	if b.wrapInsideTryCatch {
		buffer = append(buffer, "try {")
	}
	return buffer
}

func (b *EdgeBuffer) teardown(buffer []string) []string {
	if b.wrapInsideTryCatch {
		buffer = append(buffer, "} catch (error) {")
		buffer = append(buffer, fmt.Sprintf("%s(error, %s, %s);", b.rethrowCallPath, b.fileNameVar, b.lineVar))
		buffer = append(buffer, "}")
	}
	if b.outputReturnStatement {
		buffer = append(buffer, "return "+b.outputVariableName+";")
	}
	return buffer
}

// updateFileName writes an assignment to the filename variable when the filename
// changes.
func (b *EdgeBuffer) updateFileName(filename string) {
	if b.currentFileName != filename {
		b.currentFileName = filename
		b.buffer = append(b.buffer, b.fileNameVar+" = "+jsstringify.String(filename)+";")
	}
}

// updateLineNumber writes an assignment to the line number variable when a positive
// line number differs from the current one.
func (b *EdgeBuffer) updateLineNumber(lineNumber int) {
	if lineNumber > 0 && b.currentLineNumber != lineNumber {
		b.currentLineNumber = lineNumber
		b.buffer = append(b.buffer, b.lineVar+" = "+strconv.Itoa(lineNumber)+";")
	}
}

// OutputRaw appends text, as a string literal, to the output.
func (b *EdgeBuffer) OutputRaw(text string) *EdgeBuffer {
	b.buffer = append(b.buffer, b.outputVariableName+" += "+jsstringify.String(text)+";")
	return b
}

// OutputExpression appends the value of a JavaScript expression to the output,
// inside a template literal when templateLiteral is true.
func (b *EdgeBuffer) OutputExpression(text, filename string, lineNumber int, templateLiteral bool) *EdgeBuffer {
	b.updateFileName(filename)
	b.updateLineNumber(lineNumber)
	if templateLiteral {
		text = "`${" + text + "}`"
	}
	b.buffer = append(b.buffer, b.outputVariableName+" += "+text+";")
	return b
}

// WriteExpression writes a JavaScript expression followed by a semicolon.
func (b *EdgeBuffer) WriteExpression(text, filename string, lineNumber int) *EdgeBuffer {
	b.updateFileName(filename)
	b.updateLineNumber(lineNumber)
	b.buffer = append(b.buffer, text+";")
	return b
}

// WriteStatement writes JavaScript code as it is, for statements such as if/else.
func (b *EdgeBuffer) WriteStatement(text, filename string, lineNumber int) *EdgeBuffer {
	b.updateFileName(filename)
	b.updateLineNumber(lineNumber)
	b.buffer = append(b.buffer, text)
	return b
}

// Wrap adds a prefix and a suffix around the flushed code.
func (b *EdgeBuffer) Wrap(prefix, suffix string) *EdgeBuffer {
	b.prefix = append(b.prefix, prefix)
	b.suffix = append(b.suffix, suffix)
	return b
}

// DisableFileAndLineVariables leaves out the $filename and $lineNumber declarations.
func (b *EdgeBuffer) DisableFileAndLineVariables() *EdgeBuffer {
	b.outputFileAndLineNumber = false
	return b
}

// DisableOutVariable leaves out the declaration of the output variable.
func (b *EdgeBuffer) DisableOutVariable() *EdgeBuffer {
	b.outputOutVariable = false
	return b
}

// DisableReturnStatement leaves out the final return statement.
func (b *EdgeBuffer) DisableReturnStatement() *EdgeBuffer {
	b.outputReturnStatement = false
	return b
}

// DisableTryCatchBlock leaves out the try/catch block around the code.
func (b *EdgeBuffer) DisableTryCatchBlock() *EdgeBuffer {
	b.wrapInsideTryCatch = false
	return b
}

// Flush returns the code; the first call fixes it, later calls return the same text.
func (b *EdgeBuffer) Flush() string {
	if b.compiledOutput != nil {
		return *b.compiledOutput
	}
	buffer := []string{}
	for _, text := range b.prefix {
		buffer = append(buffer, strings.Split(text, EOL)...)
	}
	buffer = b.setup(buffer)
	buffer = append(buffer, b.buffer...)
	buffer = b.teardown(buffer)
	for _, text := range b.suffix {
		buffer = append(buffer, strings.Split(text, EOL)...)
	}
	out := strings.Join(buffer, EOL)
	b.compiledOutput = &out
	return out
}
