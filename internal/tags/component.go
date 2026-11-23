package tags

import (
	"slices"
	"strconv"

	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/internal/parser"
	"github.com/edge-js/edge-go/js"
)

// componentNameExpressions are the expressions a component's name may be.
var componentNameExpressions = []string{
	"Identifier", "Literal", "LogicalExpression", "MemberExpression", "ConditionalExpression", "CallExpression", "TemplateLiteral",
}

// getComponentNameAndProps returns the source of the component name and of its
// props: the first and the second expression of a sequence, or the expression and
// an empty object.
func getComponentNameAndProps(p *parser.Parser, expression *acorn.Node, filename string) (name, props string, err error) {
	nameNode := expression
	if expression.Type == "SequenceExpression" {
		nameNode = expression.Expressions[0]
		expression.Expressions = expression.Expressions[1:]
	}
	err = IsSubsetOf(nameNode, componentNameExpressions, func() error {
		return unallowedAt(`"`+p.Stringify(nameNode)+`" is not a valid argument for component name`, filename, nameNode)
	})
	if err != nil {
		return "", "", err
	}
	// Components take at most two arguments: the rest of the sequence is ignored.
	if expression.Type == "SequenceExpression" {
		return p.Stringify(nameNode), p.Stringify(expression.Expressions[0]), nil
	}
	return p.Stringify(nameNode), "{}", nil
}

// getSlotNameAndProps returns the raw source of a slot's name (a literal) and the
// name of its props parameter ("" for none). The argument is not transformed.
func getSlotNameAndProps(p *parser.Parser, token *lexer.TagToken) (name, props string, err error) {
	statement, err := p.GenerateAST(token.Properties.JsArg, token.Loc, token.Filename)
	if err != nil {
		return "", "", err
	}
	if statement == nil {
		return "", "", js.CannotReadProperties(js.Undefined, "expression")
	}
	parsed, err := statementExpression(statement)
	if err != nil {
		return "", "", err
	}
	err = IsSubsetOf(parsed, []string{"Literal", "SequenceExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid argument type for the @slot tag`, token.Filename, parsed)
	})
	if err != nil {
		return "", "", err
	}
	nameNode := parsed
	if parsed.Type == "SequenceExpression" {
		nameNode = parsed.Expressions[0]
	}
	err = IsSubsetOf(nameNode, []string{"Literal"}, func() error {
		return unallowedAt("slot name must be a valid string literal", token.Filename, nameNode)
	})
	if err != nil {
		return "", "", err
	}
	if parsed.Type == "Literal" {
		return nameNode.Raw, "", nil
	}
	if len(parsed.Expressions) > 2 {
		return "", "", edgeerror.At("maximum of 2 arguments are allowed for @slot tag", "E_MAX_ARGUMENTS", token.Filename, parsed.Loc.Start.Line, parsed.Loc.Start.Column)
	}
	propsNode := parsed.Expressions[1]
	err = IsSubsetOf(propsNode, []string{"Identifier"}, func() error {
		return unallowedAt(`"`+p.Stringify(propsNode)+`" is not valid prop identifier for @slot tag`, token.Filename, propsNode)
	})
	if err != nil {
		return "", "", err
	}
	return nameNode.Raw, propsNode.Name, nil
}

// slot is a slot of a component being compiled.
type slot struct {
	outputVar string
	props     string
	buffer    *parser.EdgeBuffer
}

// slotMap is the object the slots are collected in: Object.keys lists array-index
// keys (a slot named by a number literal) in ascending order before the others in
// insertion order.
type slotMap struct {
	keys  []string
	slots map[string]*slot
}

func (m *slotMap) set(key string, s *slot) {
	m.keys = append(m.keys, key)
	m.slots[key] = s
}

func (m *slotMap) orderedKeys() []string {
	keys := slices.Clone(m.keys)
	slices.SortStableFunc(keys, func(a, b string) int {
		i, aIndex := js.ArrayIndex(a)
		j, bIndex := js.ArrayIndex(b)
		switch {
		case aIndex && bIndex:
			return i - j
		case aIndex:
			return -1
		case bIndex:
			return 1
		}
		return 0
	})
	return keys
}

// Component is the @component tag: it renders a component with props and slots.
func Component() *parser.Tag {
	return &parser.Tag{TagName: "component", Block: true, Seekable: true, Compile: compileComponent}
}

func compileComponent(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	asyncKeyword, awaitKeyword := "", ""
	if p.AsyncMode() {
		asyncKeyword, awaitKeyword = "async ", "await "
	}
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, append(slices.Clone(componentNameExpressions), "SequenceExpression"), func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid argument type for the @component tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	name, props, err := getComponentNameAndProps(p, parsed, token.Filename)
	if err != nil {
		return err
	}
	slots := &slotMap{slots: map[string]*slot{}}
	// The main slot collects everything outside the named slots.
	mainSlot := &slot{outputVar: "slot_main", buffer: buffer.Create(token.Filename, "slot_main")}
	slotsCounter := 0
	for _, child := range token.Children {
		if !lexer.IsTag(child, "slot") {
			// The first line break of the main slot is dropped.
			if mainSlot.buffer.Size() == 0 && child.TokenType() == lexer.NewLine {
				continue
			}
			if err := p.ProcessToken(child, mainSlot.buffer); err != nil {
				return err
			}
			continue
		}
		slotToken := child.(*lexer.TagToken)
		slotName, slotProps, err := getSlotNameAndProps(p, slotToken)
		if err != nil {
			return err
		}
		slotsCounter++
		if slots.slots[slotName] == nil {
			// Slots write to the component's file: they have no file of their own.
			outputVar := "slot_" + strconv.Itoa(slotsCounter)
			slots.set(slotName, &slot{outputVar: outputVar, props: slotProps, buffer: buffer.Create(token.Filename, outputVar)})
			if slotProps != "" {
				p.Stack.DefineScope()
				p.Stack.DefineVariable(slotProps)
			}
		}
		if err := processChildren(p, slotToken.Children, slots.slots[slotName].buffer); err != nil {
			return err
		}
		if slotProps != "" {
			p.Stack.ClearScope()
		}
	}
	obj := &StringifiedObject{}
	// The component and its slots get a shallow copy of the context.
	obj.Add("$context", "Object.assign({}, $context)", false)
	if slots.slots["main"] == nil {
		if mainSlot.buffer.Size() > 0 {
			mainSlot.buffer.Wrap(asyncKeyword+"function () { const $context = this.$context;", "}")
			obj.Add("main", mainSlot.buffer.DisableFileAndLineVariables().Flush(), false)
		} else {
			obj.Add("main", `function () { return "" }`, false)
		}
	}
	for _, slotName := range slots.orderedKeys() {
		s := slots.slots[slotName]
		if s.buffer.Size() == 0 {
			obj.Add(slotName, `function () { return "" }`, false)
			continue
		}
		fnCall := asyncKeyword + "function () { const $context = this.$context;"
		if s.props != "" {
			fnCall = asyncKeyword + "function (" + s.props + ") { const $context = this.$context;"
		}
		s.buffer.Wrap(fnCall, "}")
		obj.Add(slotName, s.buffer.DisableFileAndLineVariables().Flush(), false)
	}
	caller := &StringifiedObject{}
	caller.Add("filename", "$filename", false)
	caller.Add("line", "$lineNumber", false)
	caller.Add("col", "0", false)
	buffer.OutputExpression(awaitKeyword+"template.compileComponent("+name+")(template, template.getComponentState("+props+", "+obj.Flush()+", "+caller.Flush()+"), $context)", token.Filename, token.Loc.Start.Line, false)
	return nil
}

// Slot is the @slot tag. Slots are compiled by @component; one anywhere else is
// an error.
func Slot() *parser.Tag {
	return &parser.Tag{TagName: "slot", Block: true, Seekable: true, NoNewLine: true, Compile: compileSlot}
}

func compileSlot(_ *parser.Parser, _ *parser.EdgeBuffer, token *lexer.TagToken) error {
	return edgeerror.At("@slot tag must appear as top level tag inside the @component tag", "E_ORPHAN_SLOT_TAG", token.Filename, token.Loc.Start.Line, token.Loc.Start.Col)
}

// Inject is the @inject tag: a component shares values with its caller's slots.
func Inject() *parser.Tag {
	return &parser.Tag{TagName: "inject", Seekable: true, NoNewLine: true, Compile: compileInject}
}

func compileInject(p *parser.Parser, buffer *parser.EdgeBuffer, token *lexer.TagToken) error {
	token.Properties.JsArg = "(" + token.Properties.JsArg + ")"
	parsed, err := ParseJsArg(p, token)
	if err != nil {
		return err
	}
	err = IsSubsetOf(parsed, []string{"ObjectExpression", "Identifier", "CallExpression"}, func() error {
		return unallowedAt(`"`+token.Properties.JsArg+`" is not a valid key-value pair for the @inject tag`, token.Filename, parsed)
	})
	if err != nil {
		return err
	}
	line := token.Loc.Start.Line
	// The slots' context must exist before the shared values are merged into it.
	buffer.WriteStatement("if (!state.$slots || !state.$slots.$context) {", token.Filename, line)
	buffer.WriteExpression(`throw new Error('Cannot use "@inject" outside of a component scope')`, token.Filename, line)
	buffer.WriteStatement("}", token.Filename, line)
	buffer.WriteExpression("Object.assign(state.$slots.$context, "+p.Stringify(parsed)+")", token.Filename, line)
	return nil
}
