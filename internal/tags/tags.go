package tags

import "github.com/edge-js/edge-go/internal/parser"

// Builtin returns new definitions of Edge's built-in tags, in the order Edge
// registers them.
func Builtin() []*parser.Tag {
	return []*parser.Tag{
		Assign(), Component(), Debugger(), Each(), Else(), ElseIf(), Eval(), If(), Include(), IncludeIf(),
		Inject(), Let(), NewError(), PushOnceTo(), PushTo(), Slot(), Stack(), Unless(),
	}
}

// Migrate returns new definitions of the migrate plugin's tags, in the order the
// plugin registers them.
func Migrate() []*parser.Tag {
	return []*parser.Tag{Layout(), Section(), Set(), Super()}
}
