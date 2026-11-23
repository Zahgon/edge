package edge_test

// The expected values were recorded with edge.js 6.5.1 on Node 24.

import (
	"strconv"
	"strings"
	"testing"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
)

func TestGlobalFunctions(t *testing.T) {
	tests := []struct {
		path   string
		name   string
		length int
	}{
		{"nl2br", "nl2br", 1},
		{"inspect", "inspect", 1},
		{"truncate", "truncate", 1},
		{"excerpt", "excerpt", 1},
		{"html.escape", "escape", 1},
		{"html.safe", "htmlSafe", 1},
		{"html.classNames", "classNames", 0},
		{"html.attrs", "attrs", 1},
		{"js.stringify", "stringify", 1},
		{"camelCase", "camelCase", 1},
		{"snakeCase", "snakeCase", 1},
		{"dashCase", "dashCase", 2},
		{"pascalCase", "pascalCase", 1},
		{"capitalCase", "capitalCase", 1},
		{"sentenceCase", "sentenceCase", 1},
		{"dotCase", "dotCase", 2},
		{"noCase", "noCase", 2},
		{"titleCase", "titleCase", 1},
		{"pluralize", "pluralize", 3},
		{"sentence", "sentence", 2},
		{"prettyMs", "format", 2},
		{"toMs", "parse", 1},
		{"prettyBytes", "format", 2},
		{"toBytes", "parse", 1},
		{"ordinal", "ordinal", 1},
	}
	var keys []string
	for _, tt := range tests {
		var v any = edge.EdgeGlobals
		for _, key := range strings.Split(tt.path, ".") {
			v = v.(*js.Object).Get(key)
		}
		f, ok := v.(*js.Function)
		if !ok || f.Name != tt.name || f.Length != tt.length {
			t.Errorf("%s: got %#v, want %s/%d", tt.path, v, tt.name, tt.length)
		}
		if root, _, _ := strings.Cut(tt.path, "."); len(keys) == 0 || keys[len(keys)-1] != root {
			keys = append(keys, root)
		}
	}
	if got := strings.Join(edge.EdgeGlobals.Keys(), ","); got != strings.Join(keys, ",") {
		t.Errorf("globals in order %s, want %s", got, strings.Join(keys, ","))
	}
}

func TestLoaderMakePath(t *testing.T) {
	l := edge.NewLoader()
	l.Mount("default", "/tmp/edge/run/views")
	tests := []struct {
		path, want, wantErr string
	}{
		{path: "welcome", want: "/tmp/edge/run/views/welcome.edge"},
		{path: "users.list", want: "/tmp/edge/run/views/users.list.edge"},
		{path: "a.edge.bak", want: "/tmp/edge/run/views/a..bak"},
		{path: "a.edgex", want: "/tmp/edge/run/views/a.x"},
		{path: "/abs/x", want: "/abs/x"},
		{path: "x::y::z.edge", wantErr: `"x" namespace is not mounted`},
	}
	for _, tt := range tests {
		got, err := l.MakePath(tt.path)
		if tt.wantErr != "" {
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("MakePath(%q): got %q, %v; want error %q", tt.path, got, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("MakePath(%q) = %q, %v; want %q", tt.path, got, err, tt.want)
		}
	}
}

func TestLoaderRegister(t *testing.T) {
	l := edge.NewLoader()
	if err := l.Register("a", edge.LoaderTemplate{Template: 1.0}); err == nil || err.Error() != "Make sure to define the template content as a string" {
		t.Errorf("registering a number: %v", err)
	}
	if err := l.Register("a", edge.LoaderTemplate{Template: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := l.Register("a", edge.LoaderTemplate{Template: "B"}); err == nil || err.Error() != `Cannot override previously registered "a" template` {
		t.Errorf("registering twice: %v", err)
	}
	if got := l.Templates().Get("a").(*js.Object).Get("template"); got != "A" {
		t.Errorf("templates.a.template = %#v", got)
	}
	path, err := l.MakePath("a")
	if err != nil || path != "a" {
		t.Errorf("MakePath of a registered template = %q, %v", path, err)
	}
}

func TestPlugins(t *testing.T) {
	var log []string
	e := edge.Create(edge.Options{})
	e.Use(func(_ *edge.Edge, firstRun bool, _ any) error {
		log = append(log, "once "+strconv.FormatBool(firstRun))
		return nil
	}, js.Undefined)
	e.Use(func(_ *edge.Edge, firstRun bool, _ any) error {
		log = append(log, "recurring "+strconv.FormatBool(firstRun))
		return nil
	}, edge.O("recurring", true))
	for range 2 {
		if _, err := e.CreateRenderer(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.RenderRawSync("", js.Undefined); err != nil {
		t.Fatal(err)
	}
	want := "once true,recurring true,recurring false,recurring false"
	if got := strings.Join(log, ","); got != want {
		t.Errorf("plugin runs %s, want %s", got, want)
	}
}

func TestProcessors(t *testing.T) {
	e := edge.Create(edge.Options{})
	var compiled string
	e.Processor().
		Process("raw", func(d edge.ProcessorData) (string, bool, error) {
			return strings.Replace(d.Raw, "NAME", "{{ name }}", 1), true, nil
		}).
		Process("compiled", func(d edge.ProcessorData) (string, bool, error) {
			compiled = d.Compiled
			return "", false, nil
		}).
		Process("output", func(d edge.ProcessorData) (string, bool, error) {
			return "[" + d.Output + "|" + js.ToString(d.State.Get("name")) + "]", true, nil
		})
	got, err := e.RenderRawSync("Hi NAME", edge.O("name", "virk"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "[Hi virk|virk]"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
	wantCompiled := "let out = \"\";\nlet $lineNumber = 1;\nlet $filename = \"eval.edge\";\ntry {\nout += \"Hi \";\n" +
		"out += `${template.escape(state.name)}`;\n} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn out;"
	if compiled != wantCompiled {
		t.Errorf("compiled %q, want %q", compiled, wantCompiled)
	}
}

func TestCompiledFunctionSource(t *testing.T) {
	e := edge.Create(edge.Options{})
	body := "let out = \"\";\nlet $lineNumber = 1;\nlet $filename = \"%s\";\ntry {\nout += `${template.escape(state.a)}`;\n" +
		"} catch (error) {\ntemplate.reThrow(error, $filename, $lineNumber);\n}\nreturn out;\n}"
	tests := []struct {
		compiler *edge.Compiler
		path     any
		want     string
	}{
		{e.Compiler(), "x.edge", "function anonymous(template,state,$context\n) {\n" + strings.Replace(body, "%s", "x.edge", 1)},
		{e.AsyncCompiler(), js.Undefined, "async function anonymous(template,state,$context\n) {\n" + strings.Replace(body, "%s", "eval.edge", 1)},
	}
	for _, tt := range tests {
		f, err := tt.compiler.CompileRaw("{{ a }}", tt.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := js.ToString(f); got != tt.want {
			t.Errorf("source %q, want %q", got, tt.want)
		}
	}
}

func TestCache(t *testing.T) {
	e := edge.Create(edge.Options{Cache: true})
	if _, err := e.RegisterTemplate("t", edge.LoaderTemplate{Template: "cached"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Compiler().Compile("t", nil); err != nil {
		t.Fatal(err)
	}
	if !e.Compiler().CacheManager().Has("t") || e.Compiler().CacheManager().Get("t") == nil || e.AsyncCompiler().CacheManager().Has("t") {
		t.Error("the sync compiler alone caches t")
	}
	if _, err := e.Compiler().Compile("t", []string{}); err != nil {
		t.Fatal(err)
	}
	e.RemoveTemplate("t")
	if e.Compiler().CacheManager().Has("t") {
		t.Error("RemoveTemplate keeps the cached function")
	}
}

func TestRendererShareAndClone(t *testing.T) {
	r, err := edge.Create(edge.Options{}).CreateRenderer()
	if err != nil {
		t.Fatal(err)
	}
	r.Share(edge.O("a", edge.O("c", 2.0), "d", 3.0))
	clone := r.Clone().Share(edge.O("a", edge.O("b", 9.0)))
	keys := r.GetState().Keys()
	if got := strings.Join(keys[len(keys)-2:], ","); got != "a,d" {
		t.Errorf("last state keys %s, want a,d", got)
	}
	for _, tt := range []struct {
		r    *edge.Renderer
		want string
	}{{r, `{"c":2}`}, {clone, `{"c":2,"b":9}`}} {
		got, err := tt.r.RenderRawSync("{{{ JSON.stringify(a) }}}", js.Undefined, js.Undefined)
		if err != nil || got != tt.want {
			t.Errorf("a = %s, %v; want %s", got, err, tt.want)
		}
	}
}

func TestMacroAndGetter(t *testing.T) {
	edge.Macro("shout", &js.Function{Name: "shout", Length: 1, Call: func(_ any, args []any) (any, error) {
		return strings.ToUpper(js.ToString(js.Arg(args, 0))) + "!", nil
	}})
	n := 0
	edge.Getter("answer", &js.Function{Name: "answer", Call: func(any, []any) (any, error) {
		n++
		return float64(n), nil
	}}, true)
	e := edge.Create(edge.Options{})
	e.RegisterTag(&edge.Tag{TagName: "js", Seekable: true, Compile: func(_ *edge.Parser, b *edge.Buffer, token *edge.TagToken) error {
		b.OutputExpression(token.Properties.JsArg, token.Filename, token.Loc.Start.Line, true)
		return nil
	}})
	got, err := e.RenderRawSync("@js(template.shout(\"hi\"))\n@js(template.answer + template.answer)\n"+
		"@js(template.loop.name + template.size(\"a😀\"))\n@js(Object.keys(template))\n@js(template.constructor.name)", js.Undefined)
	if err != nil {
		t.Fatal(err)
	}
	if want := "HI!\n2\neach2\nstacks\nTemplate"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
