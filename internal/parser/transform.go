package parser

import (
	"github.com/edge-js/edge-go/internal/acorn"
	"github.com/edge-js/edge-go/internal/edgeerror"
	"github.com/edge-js/edge-go/internal/lexer"
	"github.com/edge-js/edge-go/js"
)

// TransformAST rewrites an acorn node into what Edge writes to the template: free
// identifiers become reads of the state property, and the nodes edge-parser does
// not support raise E_UNALLOWED_EXPRESSION. Nodes are changed in place. A nil node
// is JavaScript's undefined (the statement of an empty expression).
func (p *Parser) TransformAST(node *acorn.Node, filename string) (*acorn.Node, error) {
	if node == nil {
		return nil, js.CannotReadProperties(js.Undefined, "type")
	}
	return p.transform(node, filename)
}

// transformNullable transforms a child acorn sets to null when it is absent (an
// array hole, the argument of a bare return).
func (p *Parser) transformNullable(node *acorn.Node, filename string) (*acorn.Node, error) {
	if node == nil {
		return nil, js.CannotReadProperties(nil, "type")
	}
	return p.transform(node, filename)
}

// transformList transforms every node of a list, stopping at the first error.
func (p *Parser) transformList(nodes []*acorn.Node, filename string, nullable bool) ([]*acorn.Node, error) {
	out := make([]*acorn.Node, len(nodes))
	for i, node := range nodes {
		var err error
		if nullable {
			out[i], err = p.transformNullable(node, filename)
		} else {
			out[i], err = p.transform(node, filename)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// transformPair transforms the left and the right operand of a node.
func (p *Parser) transformPair(node *acorn.Node, filename string) (*acorn.Node, error) {
	var err error
	if node.Left, err = p.transform(node.Left, filename); err != nil {
		return nil, err
	}
	if node.Right, err = p.transform(node.Right, filename); err != nil {
		return nil, err
	}
	return node, nil
}

// transformArgument transforms the argument of a unary, update, await or spread
// node.
func (p *Parser) transformArgument(node *acorn.Node, filename string) (*acorn.Node, error) {
	var err error
	if node.Argument, err = p.transform(node.Argument, filename); err != nil {
		return nil, err
	}
	return node, nil
}

func (p *Parser) transform(node *acorn.Node, filename string) (*acorn.Node, error) {
	var err error
	switch node.Type {
	case "Identifier":
		return p.transformIdentifier(node), nil
	case "Literal", "ThisExpression", "FunctionDeclaration":
		return node, nil
	case "MemberExpression":
		if node.Object, err = p.transform(node.Object, filename); err != nil {
			return nil, err
		}
		// a[b], a.b.c() and [1, 2].filter(...) need the property transformed
		// too, a.b does not.
		if node.Computed || node.Property.Type != "Identifier" {
			if node.Property, err = p.transform(node.Property, filename); err != nil {
				return nil, err
			}
		}
		return node, nil
	case "ExpressionStatement":
		return p.transform(node.Expression, filename)
	case "CallExpression":
		if node.Callee, err = p.transform(node.Callee, filename); err != nil {
			return nil, err
		}
		if node.Arguments, err = p.transformList(node.Arguments, filename, false); err != nil {
			return nil, err
		}
		return node, nil
	case "ArrowFunctionExpression":
		return p.transformArrowFunction(node, filename)
	case "TemplateLiteral", "SequenceExpression":
		if node.Expressions, err = p.transformList(node.Expressions, filename, false); err != nil {
			return nil, err
		}
		return node, nil
	case "BinaryExpression", "LogicalExpression", "AssignmentExpression":
		return p.transformPair(node, filename)
	case "ArrayExpression":
		if node.Elements, err = p.transformList(node.Elements, filename, true); err != nil {
			return nil, err
		}
		return node, nil
	case "ObjectExpression":
		return p.transformObject(node, filename)
	case "UnaryExpression", "UpdateExpression", "SpreadElement":
		return p.transformArgument(node, filename)
	case "ConditionalExpression":
		if node.Test, err = p.transform(node.Test, filename); err != nil {
			return nil, err
		}
		if node.Consequent, err = p.transform(node.Consequent, filename); err != nil {
			return nil, err
		}
		if node.Alternate, err = p.transform(node.Alternate, filename); err != nil {
			return nil, err
		}
		return node, nil
	case "AwaitExpression":
		if !p.options.Async {
			loc, err := GetExpressionLoc(node)
			if err != nil {
				return nil, err
			}
			return nil, edgeerror.At("Make sure to render template in async mode before using await expression", "E_PARSER_ERROR", filename, loc.Line, loc.Col)
		}
		return p.transformArgument(node, filename)
	case "NewExpression":
		// The callee is kept as written.
		if node.Arguments, err = p.transformList(node.Arguments, filename, false); err != nil {
			return nil, err
		}
		return node, nil
	case "BlockStatement":
		if node.BodyList, err = p.transformList(node.BodyList, filename, false); err != nil {
			return nil, err
		}
		return node, nil
	case "ReturnStatement":
		if node.Argument, err = p.transformNullable(node.Argument, filename); err != nil {
			return nil, err
		}
		return node, nil
	case "ChainExpression":
		if node.Expression, err = p.transform(node.Expression, filename); err != nil {
			return nil, err
		}
		return node, nil
	}
	if node.Loc == nil {
		return nil, js.CannotReadProperties(js.Undefined, "start")
	}
	return nil, edgeerror.At(`"`+node.Type+`" is not supported`, "E_UNALLOWED_EXPRESSION", filename, node.Loc.Start.Line, node.Loc.Start.Column)
}

// GetExpressionLoc returns the template position of a node, or of its property
// for a node built by MakeMemberAccessor, which has no location of its own.
func GetExpressionLoc(expression *acorn.Node) (lexer.LexerPosition, error) {
	loc := expression.Loc
	if loc == nil && expression.Property != nil {
		loc = expression.Property.Loc
	}
	if loc == nil {
		return lexer.LexerPosition{}, js.CannotReadProperties(js.Undefined, "start")
	}
	return lexer.LexerPosition{Line: loc.Start.Line, Col: loc.Start.Column}, nil
}

// transformIdentifier leaves local variables, stack variables and defined globals
// alone and reads every other identifier from the state property.
func (p *Parser) transformIdentifier(node *acorn.Node) *acorn.Node {
	name := node.Name
	for _, local := range p.options.LocalVariables {
		if local == name {
			return node
		}
	}
	if p.Stack.Has(name) || p.options.GlobalDefined != nil && p.options.GlobalDefined(name) {
		return node
	}
	return MakeMemberAccessor(p.options.StatePropertyName, node)
}

// transformArrowFunction defines the parameters in a new scope while the body is
// transformed.
func (p *Parser) transformArrowFunction(node *acorn.Node, filename string) (*acorn.Node, error) {
	p.Stack.DefineScope()
	for _, param := range node.Params {
		switch param.Type {
		case "Identifier":
			p.Stack.DefineVariable(param.Name)
		case "ObjectPattern", "ArrayPattern":
			collect := CollectObjectExpressionProperties
			if param.Type == "ArrayPattern" {
				collect = CollectArrayExpressionProperties
			}
			names, err := collect(param)
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				p.Stack.DefineVariable(name)
			}
		default:
			loc, err := GetExpressionLoc(param)
			if err != nil {
				return nil, err
			}
			return nil, edgeerror.At("Report this error to the maintainers: Unexpected arrow function property type "+param.Type, "E_PARSER_ERROR", filename, loc.Line, loc.Col)
		}
	}
	body, err := p.transform(node.Body, filename)
	if err != nil {
		return nil, err
	}
	node.Body = body
	p.Stack.ClearScope()
	return node, nil
}

// transformObject transforms the values (and computed keys) of the properties and
// the spread elements of an object expression.
func (p *Parser) transformObject(node *acorn.Node, filename string) (*acorn.Node, error) {
	for i, property := range node.Properties {
		var err error
		switch property.Type {
		case "Property":
			// The value changes shape, so a shorthand property must be printed
			// with its key.
			property.Shorthand = false
			if property.Computed {
				if property.Key, err = p.transform(property.Key, filename); err != nil {
					return nil, err
				}
			}
			if property.Value, err = p.transform(property.Value, filename); err != nil {
				return nil, err
			}
		case "SpreadElement":
			if node.Properties[i], err = p.transform(property, filename); err != nil {
				return nil, err
			}
		default:
			loc, err := GetExpressionLoc(property)
			if err != nil {
				return nil, err
			}
			return nil, edgeerror.At(`Report this error to the maintainers: Unexpected object property type "`+property.Type+`"`, "E_PARSER_ERROR", filename, loc.Line, loc.Col)
		}
	}
	return node, nil
}

// MakeMemberAccessor returns the node of `propertyName.property`.
func MakeMemberAccessor(propertyName string, property *acorn.Node) *acorn.Node {
	return &acorn.Node{
		Type:     "MemberExpression",
		Object:   &acorn.Node{Type: "Identifier", Name: propertyName},
		Computed: false,
		Property: property,
	}
}

// MakeCallable returns the node of a call of paths with args: `paths[0](...args)`
// for a single name, `paths[0].paths[1](...args)` otherwise.
func MakeCallable(paths []string, args []*acorn.Node) *acorn.Node {
	if len(paths) == 1 {
		return &acorn.Node{Type: "CallExpression", Callee: &acorn.Node{Type: "Identifier", Name: paths[0]}, Arguments: args}
	}
	return &acorn.Node{
		Type: "CallExpression",
		Callee: &acorn.Node{
			Type:     "MemberExpression",
			Object:   &acorn.Node{Type: "Identifier", Name: paths[0]},
			Property: &acorn.Node{Type: "Identifier", Name: paths[1]},
		},
		Arguments: args,
	}
}

// CollectObjectExpressionProperties returns the names an object pattern binds; a
// property whose value is not an identifier is an error.
func CollectObjectExpressionProperties(pattern *acorn.Node) ([]string, error) {
	names := make([]string, len(pattern.Properties))
	for i, property := range pattern.Properties {
		// A rest element has no value.
		if property.Type == "RestElement" {
			return nil, js.CannotReadProperties(js.Undefined, "type")
		}
		if property.Value.Type != "Identifier" {
			return nil, js.NewError("Object destructuring should not reference dynamic properties")
		}
		names[i] = property.Value.Name
	}
	return names, nil
}

// CollectArrayExpressionProperties returns the names an array pattern binds; an
// element that is not an identifier is an error.
func CollectArrayExpressionProperties(pattern *acorn.Node) ([]string, error) {
	names := make([]string, len(pattern.Elements))
	for i, element := range pattern.Elements {
		if element == nil {
			return nil, js.CannotReadProperties(nil, "type")
		}
		if element.Type != "Identifier" {
			return nil, js.NewError("Array destructuring should not reference dynamic properties")
		}
		names[i] = element.Name
	}
	return names, nil
}
