package edge_test

// The expected values were recorded with edge.js 6.5.1 on Node 24, with the
// same templates and tags written in JavaScript against edge-parser's API.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
)

// describeToken writes a lexer token on one line: its type, its properties
// and its position, then its children.
func describeToken(token edge.Token) string {
	pos := func(l edge.LexerLoc) string {
		return fmt.Sprintf("%d:%d-%d:%d", l.Start.Line, l.Start.Col, l.End.Line, l.End.Col)
	}
	switch t := token.(type) {
	case *edge.RawToken:
		return fmt.Sprintf("raw %s line %d", strconv.Quote(t.Value), t.Line)
	case *edge.NewLineToken:
		return fmt.Sprintf("newline line %d", t.Line)
	case *edge.CommentToken:
		return fmt.Sprintf("comment %s %s", strconv.Quote(t.Value), pos(t.Loc))
	case *edge.MustacheToken:
		return fmt.Sprintf("%s %s %s", t.Type, strconv.Quote(t.Properties.JsArg), pos(t.Loc))
	case *edge.TagToken:
		s := fmt.Sprintf("%s %s(%s)", t.Type, t.Properties.Name, t.Properties.JsArg)
		if t.Properties.Selfclosed {
			s += " selfclosed"
		}
		s += " " + pos(t.Loc)
		if len(t.Children) > 0 {
			children := make([]string, len(t.Children))
			for i, c := range t.Children {
				children[i] = describeToken(c)
			}
			s += " [" + strings.Join(children, "; ") + "]"
		}
		return s
	}
	return fmt.Sprintf("%T", token)
}

func describeTokens(tokens []edge.Token) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = describeToken(t)
	}
	return out
}

func checkLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}

func TestTokenizeRaw(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{
			src: "Hello {{ name }}!\n@if(user.isAdmin)\n  {{{ html }}}\n@end",
			want: []string{
				`raw "Hello " line 1`,
				`mustache " name " 1:8-1:16`,
				`raw "!" line 1`,
				`tag if(user.isAdmin) 2:4-2:17 [newline line 2; raw "  " line 3; s__mustache " html " 3:5-3:14]`,
			},
		},
		{
			src: "{{-- a comment --}}\n@!component('card', { title: 'x' })\n@{{ kept }}\n@@include('raw')",
			want: []string{
				`comment " a comment " 1:4-1:19`,
				"tag component('card', { title: 'x' }) selfclosed 2:12-2:35",
				"newline line 2",
				`e__mustache " kept " 3:3-3:11`,
				"e__tag include('raw') 4:10-4:16",
			},
		},
		{
			src: "{{ a }}{{\n  b\n}}\n@each(item in items)\n{{ item }}\n@else\nnone\n@end",
			want: []string{
				`mustache " a " 1:2-1:7`,
				`mustache "\n  b\n" 1:9-3:2`,
				`tag each(item in items) 4:6-4:20 [newline line 4; mustache " item " 5:2-5:10; tag else() 6:5-6:5; newline line 6; raw "none" line 7]`,
			},
		},
	}
	e := edge.Create(edge.Options{})
	for _, tt := range tests {
		tokens, err := e.Compiler().TokenizeRaw(tt.src, js.Undefined)
		if err != nil {
			t.Fatalf("%q: %v", tt.src, err)
		}
		checkLines(t, describeTokens(tokens), tt.want)
	}
}

func TestTokenizeRegisteredTemplate(t *testing.T) {
	e := edge.Create(edge.Options{})
	if _, err := e.RegisterTemplate("greeting", edge.LoaderTemplate{Template: "Hi {{ name }}"}); err != nil {
		t.Fatal(err)
	}
	tokens, err := e.Compiler().Tokenize("greeting")
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, describeTokens(tokens), []string{`raw "Hi " line 1`, `mustache " name " 1:5-1:13`})
	if f := tokens[1].(*edge.MustacheToken).Filename; f != "greeting" {
		t.Errorf("filename %q", f)
	}
}

func TestClaimTag(t *testing.T) {
	e := edge.Create(edge.Options{})
	e.Compiler().ClaimTag(func(name string) *edge.Tag {
		if name == "svg" {
			return &edge.Tag{Seekable: true}
		}
		return nil
	})
	tokens, err := e.Compiler().TokenizeRaw("@svg('home')\n@other('x')", js.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, describeTokens(tokens), []string{
		"tag svg('home') 1:5-1:12",
		"newline line 1",
		`raw "@other('x')" line 2`,
	})
}

// TestParserUtilities defines a tag that parses its argument with acorn,
// rewrites it with edge-parser's transforms and writes it back as source.
func TestParserUtilities(t *testing.T) {
	var astType string
	e := edge.Create(edge.Options{})
	e.RegisterTag(&edge.Tag{TagName: "upper", Seekable: true, Compile: func(p *edge.Parser, b *edge.Buffer, token *edge.TagToken) error {
		ast, err := p.GenerateAST(token.Properties.JsArg, token.Loc, token.Filename)
		if err != nil {
			return err
		}
		astType = ast.Type
		expr, err := p.TransformAST(ast, token.Filename)
		if err != nil {
			return err
		}
		b.OutputExpression("("+p.Stringify(expr)+").toUpperCase()", token.Filename, token.Loc.Start.Line, true)
		return nil
	}})
	got, err := e.RenderRawSync("@upper(user.name + '!')", edge.O("user", edge.O("name", "virk")))
	if err != nil || got != "VIRK!" {
		t.Errorf("render = %q, %v", got, err)
	}
	if astType != "ExpressionStatement" {
		t.Errorf("generateAST returned a %s", astType)
	}
	f, err := e.Compiler().CompileRaw("@upper(user.name + '!')", js.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := js.ToString(f), "function anonymous(template,state,$context\n) {\nlet out = \"\";\nlet $lineNumber = 1;\nlet $filename = \"eval.edge\";\ntry {\nout += `${(state.user.name + '!').toUpperCase()}`;\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn out;\n}"; got != want {
		t.Errorf("compiled\n got %q\nwant %q", got, want)
	}
}

// TestStackAndProcessToken defines a block tag that declares a local
// variable for its children: inside, the variable is not read from state.
func TestStackAndProcessToken(t *testing.T) {
	var list []string
	var hasLocal, hasOther, hasAfterClear bool
	e := edge.Create(edge.Options{})
	e.RegisterTag(&edge.Tag{TagName: "local", Block: true, Seekable: true, Compile: func(p *edge.Parser, b *edge.Buffer, token *edge.TagToken) error {
		name := token.Properties.JsArg
		p.Stack.DefineScope()
		p.Stack.DefineVariable(name)
		b.WriteStatement("let "+name+" = \"local\";", token.Filename, token.Loc.Start.Line)
		for _, child := range token.Children {
			if err := p.ProcessToken(child, b); err != nil {
				return err
			}
		}
		list, hasLocal, hasOther = p.Stack.List(), p.Stack.Has(name), p.Stack.Has("m")
		p.Stack.ClearScope()
		hasAfterClear = p.Stack.Has(name)
		return nil
	}})
	got, err := e.RenderRawSync("@local(n)\n{{ n }} {{ m }}\n@end\n{{ n }}", edge.O("n", "state", "m", "M"))
	if want := "local M\nstate"; err != nil || got != want {
		t.Errorf("render = %q, %v; want %q", got, err, want)
	}
	if strings.Join(list, ",") != "n" || !hasLocal || hasOther || hasAfterClear {
		t.Errorf("stack list %v, has(n) %v, has(m) %v, has(n) after clearScope %v", list, hasLocal, hasOther, hasAfterClear)
	}
	f, err := e.Compiler().CompileRaw("@local(n)\n{{ n }}\n@end", js.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := js.ToString(f), "function anonymous(template,state,$context\n) {\nlet out = \"\";\nlet $lineNumber = 1;\nlet $filename = \"eval.edge\";\ntry {\nlet n = \"local\";\nout += \"\\n\";\n$lineNumber = 2;\nout += `${template.escape(n)}`;\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn out;\n}"; got != want {
		t.Errorf("compiled\n got %q\nwant %q", got, want)
	}
}

// TestBuffer creates buffers from a tag, as tags do for slots and
// components, and flushes them with different options.
func TestBuffer(t *testing.T) {
	var size int
	var name, full, bare, wrapped string
	e := edge.Create(edge.Options{})
	e.RegisterTag(&edge.Tag{TagName: "buf", Compile: func(_ *edge.Parser, buffer *edge.Buffer, token *edge.TagToken) error {
		b := buffer.Create(token.Filename, "slot")
		b.OutputRaw(`a "q"`)
		b.OutputExpression("1 + 1", token.Filename, 2, true)
		b.WriteStatement(`slot += "!";`, token.Filename, 3)
		b.WriteExpression("state.count++", token.Filename, 4)
		size, name, full = b.Size(), b.OutputVariableName(), b.Flush()
		b2 := buffer.Create(token.Filename, "x")
		b2.OutputRaw("y")
		bare = b2.DisableFileAndLineVariables().DisableTryCatchBlock().DisableReturnStatement().Flush()
		b3 := buffer.Create(token.Filename, "z")
		b3.OutputRaw("w")
		wrapped = b3.Wrap("(function () {", "})()").DisableOutVariable().Flush()
		buffer.OutputRaw("buf")
		return nil
	}})
	got, err := e.RenderRawSync("@buf()", js.Undefined)
	if err != nil || got != "buf" {
		t.Errorf("render = %q, %v", got, err)
	}
	if size != 7 || name != "slot" {
		t.Errorf("size %d, outputVariableName %q", size, name)
	}
	for _, tt := range []struct{ got, want string }{
		{full, "let slot = \"\";\nlet $lineNumber = 1;\nlet $filename = \"eval.edge\";\ntry {\nslot += \"a \\\"q\\\"\";\n$lineNumber = 2;\nslot += `${1 + 1}`;\n$lineNumber = 3;\nslot += \"!\";\n$lineNumber = 4;\nstate.count++;\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn slot;"},
		{bare, "let x = \"\";\nx += \"y\";"},
		{wrapped, "(function () {\nlet $lineNumber = 1;\nlet $filename = \"eval.edge\";\ntry {\nz += \"w\";\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn z;\n})()"},
	} {
		if tt.got != tt.want {
			t.Errorf("flush\n got %q\nwant %q", tt.got, tt.want)
		}
	}
}

func TestUnmount(t *testing.T) {
	e := edge.Create(edge.Options{})
	e.MountDisk("views", "/tmp/does-not-matter")
	path, err := e.Loader().MakePath("views::home")
	if err != nil || path != "/tmp/does-not-matter/home.edge" {
		t.Errorf("MakePath = %q, %v", path, err)
	}
	e.Unmount("views")
	if _, err := e.RenderSync("views::home", js.Undefined); err == nil || err.Error() != `"views" namespace is not mounted` {
		t.Errorf("rendering after Unmount: %v", err)
	}
	if keys := e.Loader().Mounted().Keys(); len(keys) != 0 {
		t.Errorf("mounted disks %v", keys)
	}
}

// TestTagEvent changes a tag's argument from the "tag" processor, before the
// tag is compiled.
func TestTagEvent(t *testing.T) {
	var seen []string
	e := edge.Create(edge.Options{})
	e.Processor().Process("tag", func(d edge.ProcessorData) (string, bool, error) {
		seen = append(seen, d.Tag.Properties.Name+" "+d.Path)
		if d.Tag.Properties.Name == "if" {
			d.Tag.Properties.JsArg = "!" + d.Tag.Properties.JsArg
		}
		return "", false, nil
	})
	got, err := e.RenderRawSync("@if(show)\nshown\n@else\nhidden\n@end\n@let(x = 1)", edge.O("show", false))
	if err != nil || got != "shown" {
		t.Errorf("render = %q, %v", got, err)
	}
	checkLines(t, seen, []string{"if eval.edge", "else eval.edge", "let eval.edge"})
}
