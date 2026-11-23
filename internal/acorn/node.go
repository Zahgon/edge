package acorn

// Position is a line (1-based) and a column (0-based, counted in UTF-16 code units)
// in the parsed input.
//
// Positions are shared the way acorn shares them: every token gets a fresh start and
// end Position, a node's Loc.Start is the Position its first token started at, and
// its Loc.End is the end Position of the last token it consumed. An OnToken callback
// that mutates the Position values of a token therefore changes every node that
// starts or ends with that token, as edge-parser relies on.
type Position struct {
	Line   int
	Column int
}

// SourceLocation is the loc property of a node or token.
type SourceLocation struct {
	Start *Position
	End   *Position
}

// RegexLiteral is the regex property of a regular expression Literal and the value
// of a regexp token.
type RegexLiteral struct {
	Pattern string
	Flags   string
}

// TemplateValue is the value property of a TemplateElement: the raw source text
// (with line terminators normalised to "\n") and the cooked string. Cooked is nil
// when the element of a tagged template holds an invalid escape sequence (ESTree's
// cooked: null).
type TemplateValue struct {
	Raw    string
	Cooked *string
}

// Node is an ESTree node as acorn 8.19.0 produces it.
//
// One struct covers every node type; Type names it and only the fields that ESTree
// defines for that type are meaningful. Fields carry the names of the ESTree
// properties, except where ESTree gives one property name several shapes:
//
//   - body: Body for the single-node bodies (FunctionDeclaration,
//     FunctionExpression, ArrowFunctionExpression, ClassDeclaration,
//     ClassExpression, CatchClause, LabeledStatement, WithStatement, WhileStatement,
//     DoWhileStatement, ForStatement, ForInStatement, ForOfStatement); BodyList for
//     the lists of Program, BlockStatement and ClassBody.
//   - value: Value for the node of Property and MethodDefinition; LiteralValue for
//     the primitive of Literal; TemplateValue for TemplateElement's {raw, cooked}.
//   - consequent: Consequent for IfStatement and ConditionalExpression;
//     ConsequentList for the statements of SwitchCase.
//   - expression: Expression for the node of ExpressionStatement and
//     ChainExpression; ExpressionBody for the boolean of the function nodes (true
//     for an arrow function whose body is an expression).
//
// Absent ESTree properties are Go zero values: a nil *Node for null (Id of an
// anonymous function, Alternate without else, Param of catch without binding, Label,
// Init, Test, Update, Argument, SuperClass, Handler, Finalizer) and nil entries in
// Elements for array holes. Literal fields follow astring's reading of them: Raw ""
// means the node has no raw text (a node built by hand), Regex nil means it is not a
// regular expression, Bigint "" means it is not a BigInt.
//
// LiteralValue holds nil (null), bool, float64, a string, or a js.BigInt for a
// BigInt. For a regular expression it is nil and Regex holds the pattern and flags
// (acorn stores a RegExp object, or null where the host cannot build one).
//
// Strings (Name, LiteralValue, Raw, Cooked, Directive) are JavaScript strings in the
// WTF-8 form of package jsstr: a lone surrogate written with \u in the source
// survives as its generalized UTF-8 encoding. Start and End are offsets in UTF-16
// code units, as JavaScript string indexes are.
type Node struct {
	Type       string
	Start, End int
	// Loc is nil when Options.Locations is off and for nodes built by hand.
	Loc *SourceLocation

	// Program
	SourceType string // "script"

	// Identifier
	Name string

	// Literal
	LiteralValue any
	Raw          string
	Regex        *RegexLiteral
	Bigint       string

	// ExpressionStatement: Directive is the raw text of a directive's string
	// literal without its quotes, nil for statements outside a directive prologue.
	Expression *Node
	Directive  *string

	// Function nodes and classes
	Id             *Node
	Params         []*Node
	Body           *Node
	BodyList       []*Node
	ExpressionBody bool
	Generator      bool
	Async          bool
	SuperClass     *Node

	// Operators: Unary, Update, Binary, Logical, Assignment
	Operator string
	Prefix   bool
	Left     *Node
	Right    *Node
	Argument *Node

	// YieldExpression
	Delegate bool

	// Calls, members, chains
	Callee    *Node
	Arguments []*Node
	Object    *Node
	Property  *Node
	Computed  bool
	Optional  bool

	// MetaProperty: Meta and Property
	Meta *Node

	// ImportExpression
	Source *Node

	// Templates
	Expressions   []*Node // also SequenceExpression
	Quasis        []*Node
	Tag           *Node
	Quasi         *Node
	TemplateValue *TemplateValue
	Tail          bool

	// Arrays, objects, properties, patterns
	Elements   []*Node
	Properties []*Node
	Key        *Node
	Value      *Node
	Kind       string // Property: init, get, set; MethodDefinition: constructor, method, get, set; VariableDeclaration: var, let, const
	Method     bool
	Shorthand  bool
	Static     bool

	// Variables
	Declarations []*Node
	Init         *Node

	// Control flow
	Test           *Node
	Consequent     *Node
	ConsequentList []*Node
	Alternate      *Node
	Update         *Node
	Await          bool // ForOfStatement: for await
	Label          *Node
	Discriminant   *Node
	Cases          []*Node
	Block          *Node
	Handler        *Node
	Finalizer      *Node
	Param          *Node
}

// Token is what acorn hands to the onToken callback.
//
// Type is the label of acorn's token type ("name", "num", "string", "regexp",
// "template", "invalidTemplate", "eof", a keyword such as "function", or the
// punctuation such as "(", "?.", "=>", "+/-", "</>/<=/>=", "==/!=/===/!==").
//
// Value is nil where acorn's is undefined (punctuation without a value, "`", "${",
// eof), a string for names, keywords, strings, template chunks and operators, a
// float64 for numbers, a js.BigInt for BigInt literals and a *RegexLiteral for
// regular expressions.
type Token struct {
	Type  string
	Value any
	Start int
	End   int
	Loc   *SourceLocation
}

// Options are the subset of acorn's options Edge passes, with acorn's defaults for
// the others (sourceType "script", no reserved-word or hashbang override, no
// preserved parentheses, no ranges).
type Options struct {
	// EcmaVersion is 3, 5, 6 to 11, or a year from 2015 to 2020. Zero selects
	// 2020, acorn 8's default.
	EcmaVersion int
	// Locations attaches Loc to every node and token.
	Locations bool
	// AllowAwaitOutsideFunction parses await at the top level as an await
	// expression.
	AllowAwaitOutsideFunction bool
	// OnToken, if set, receives every token as the parser consumes it, including
	// the final eof token.
	OnToken func(*Token)
}

// SyntaxError is the error acorn raises: Message ends with " (line:column)", Pos is
// the offset the error is reported at, Loc its line and column, and RaisedAt the
// tokenizer position when it was raised.
type SyntaxError struct {
	Message  string
	Pos      int
	Loc      *Position
	RaisedAt int
}

// Error returns the message, with its " (line:column)" suffix.
func (e *SyntaxError) Error() string { return e.Message }
