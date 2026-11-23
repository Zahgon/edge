package edge_test

// The expected values were recorded by rendering the same templates with
// edge.js 6.5.1 on Node 24.

import (
	"errors"
	"reflect"
	"testing"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
	"github.com/edge-js/edge-go/plugins/migrate"
)

type renderCase struct {
	name string
	// src is rendered with RenderRaw/RenderRawSync, path with Render/RenderSync.
	src, path string
	state     *js.Object
	// templates are registered in memory, in order.
	templates [][2]string
	// async renders with the async compiler, migrate uses the migrate
	// plugin, mount mounts a default disk that does not exist.
	async, migrate, mount bool
	want                  string
	// wantErr is an *edge.EdgeError or a *js.Error (compared by name and
	// message).
	wantErr error
}

var renderCases = []renderCase{
	{
		name:  "escapes mustache output",
		src:   "Hello {{ name }}! {{{ name }}}",
		state: edge.O("name", "<b>virk</b> & 'co'"),
		want:  "Hello &lt;b&gt;virk&lt;/b&gt; &amp; &#x27;co&#x27;! <b>virk</b> & 'co'",
	},
	{
		name: "trims one line break at each end",
		src:  "\n\nbody\n\n",
		want: "\nbody\n",
	},
	{
		name: "trims CRLF at each end",
		src:  "\r\nbody\r\n",
		want: "body",
	},
	{
		name:  "each over array with index",
		src:   "@each((user, i) in users)\n{{ i }}:{{ user.name }}\n@end",
		state: edge.O("users", edge.A(edge.O("name", "ana"), edge.O("name", "bob"))),
		want:  "0:ana\n1:bob",
	},
	{
		name: "each over object and else",
		src: `@each((v, k) in obj)
{{ k }}={{ v }}
@else
empty
@end
@each(x in [])
x
@else
none
@end`,
		state: edge.O("obj", edge.O("b", float64(1), "a", float64(2), "1", float64(3))),
		want:  "1=3\nb=1\na=2\nnone",
	},
	{
		name:  "each over string code points",
		src:   "@each((c, i) in word)\n[{{ i }}{{ c }}]\n@end",
		state: edge.O("word", "a🎉b"),
		want:  "[0a]\n[1🎉]\n[2b]",
	},
	{
		name:  "if elseif else",
		src:   "@if(n > 10)\nbig\n@elseif(n > 5)\nmid\n@else\nsmall\n@end",
		state: edge.O("n", float64(7)),
		want:  "mid",
	},
	{
		name: "let and assign nested path",
		src: `@let(user = { name: 'a' })
@assign(user.address.city = 'Paris')
{{ JSON.stringify(user) }}`,
		wantErr: &edge.EdgeError{Message: "Cannot set properties of undefined (setting 'city')", Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: float64(2), Col: float64(0)},
	},
	{
		name:  "nl2br and safe",
		src:   "{{{ nl2br(text) }}}|{{ nl2br(text) }}|{{ html.safe('<i>') }}",
		state: edge.O("text", "a\nb\r\nc<br>\nd"),
		want:  "a<br>b<br>c<br><br>d|a&lt;br&gt;b&lt;br&gt;c&lt;br&gt;&lt;br&gt;d|<i>",
	},
	{
		name: "html attrs and classNames",
		src:  "<p {{ html.attrs({ class: ['a', { b: true, c: false }], id: 'x', hidden: true, disabled: false, 'data-n': 3, x: { data: '{ open: false }' }, xOn: { click: 'go()' } }) }}>{{ html.classNames('a', ['b', { c: 1 }], null) }}</p>",
		want: "<p class=\"a b\" id=\"x\" hidden data-n=\"3\" x-data=\"{ open: false }\" x-on:click=\"go()\">a b c</p>",
	},
	{
		name: "string helpers",
		src:  "{{ camelCase('hello world') }} {{ snakeCase('helloWorld') }} {{ dashCase('helloWorld') }} {{ pascalCase('a b') }} {{ titleCase('the art of war') }} {{ pluralize('box') }} {{ sentence(['a', 'b', 'c']) }} {{ prettyMs(90061000) }} {{ toMs('1h') }} {{ prettyBytes(1536) }} {{ toBytes('1KB') }} {{ ordinal(22) }}",
		want: "helloWorld hello_world hello-world AB The Art of War boxes a, b, and c 1d 3600000 1.5KB 1024 22nd",
	},
	{
		name: "truncate and excerpt",
		src:  "{{ truncate('This is a very long sentence', 12) }}|{{ excerpt('<p>This is a <b>long</b> sentence</p>', 12, { strict: true }) }}",
		want: "This is a very...|This is a lo...",
	},
	{
		name: "js stringify",
		src:  "{{{ js.stringify({ a: '</script>', d: new Date(0) }) }}}",
		want: "{\"a\":\"\\u003C\\u002Fscript\\u003E\",\"d\":\"1970-01-01T00:00:00.000Z\"}",
	},
	{
		name:    "runtime error is rethrown with the line",
		src:     "line 1\n{{ user.profile.name }}",
		state:   edge.O("user", edge.O()),
		wantErr: &edge.EdgeError{Message: "Cannot read properties of undefined (reading 'name')", Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: float64(2), Col: float64(0)},
	},
	{
		name:    "state prefix removed from messages",
		src:     "{{ missing() }}",
		wantErr: &edge.EdgeError{Message: "missing is not a function", Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: float64(1), Col: float64(0)},
	},
	{
		name:    "newError",
		src:     "@newError('Something went wrong', $caller.filename, 5, 2)",
		state:   edge.O("$caller", edge.O("filename", "caller.edge")),
		wantErr: &edge.EdgeError{Message: "Something went wrong", Code: "E_RUNTIME_EXCEPTION", Filename: "caller.edge", Line: float64(5), Col: float64(2)},
	},
	{
		name:    "lexer error",
		src:     "@if(\nyes\n@end",
		wantErr: &edge.EdgeError{Message: "Missing token \")\"", Code: "E_UNCLOSED_PAREN", Filename: "eval.edge", Line: float64(1), Col: float64(4)},
	},
	{
		name: "stacks fill placeholders",
		src: `@stack('js')
@pushTo('js')
<script src="a.js"></script>
@end
@pushOnceTo('js')
<script src="b.js"></script>
@end
@pushOnceTo('js')
<script src="b.js"></script>
@end
body`,
		want: `<script src="a.js"></script>
<script src="b.js"></script>
<script src="b.js"></script>

body`,
	},
	{
		name:    "stack declared twice",
		src:     "@stack('js')\n@stack('js')",
		wantErr: &edge.EdgeError{Message: "Cannot declare stack \"js\" for multiple times", Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: float64(2), Col: float64(0)},
	},
	{
		name:      "include partial with local scope",
		src:       "@let(title = 'Home')\n@include('partials/header')",
		state:     edge.O("name", "virk"),
		templates: [][2]string{{"partials/header", "<h1>{{ title }} - {{ name }}</h1>"}},
		want:      "<h1>Home - virk</h1>",
	},
	{
		name: "component with props and slots",
		src: `@component('components/card', { title: 'Hi', class: 'x' })
@slot('footer')
foot
@end
body
@end`,
		templates: [][2]string{{"components/card", `<div {{ $props.merge({ class: ['card'] }).toAttrs() }}>
<h3>{{ $props.get('title') }}</h3>
{{{ $slots.main() }}}
@if($slots.footer)
<footer>{{{ $slots.footer() }}}</footer>
@end
</div>`}},
		want: `<div class="card x" title="Hi">
<h3>Hi</h3>
body
<footer>foot</footer>
</div>`,
	},
	{
		name:      "component props helpers",
		src:       "@!component('components/props', { a: { b: 1 }, c: 2, type: 'submit' })",
		templates: [][2]string{{"components/props", "{{ $props.has('a.b') }} {{ $props.get('a.x', 'none') }} {{ JSON.stringify($props.only(['c']).all()) }} {{ JSON.stringify($props.except(['a']).all()) }} {{ Object.keys($props).join(',') }} {{ $props.toAttrs() }}"}},
		want:      "true none {&quot;c&quot;:2} {&quot;c&quot;:2,&quot;type&quot;:&quot;submit&quot;} a,c,type a=\"[object Object]\" c=\"2\" type=\"submit\"",
	},
	{
		name: "supercharged component tag",
		src: `@!components.badge({ text: 'new' })
@components.badge({ text: 'old' })
ignored
@end`,
		templates: [][2]string{{"components/badge", "<span>{{ text }}</span>"}},
		mount:     true,
		want:      "<span>new</span>\n<span>old</span>",
	},
	{
		name: "inject and context",
		src:  "@component('components/ctx')\n{{ $context.injected }}\n@end",
		templates: [][2]string{{"components/ctx", `@inject({ injected: 'from component' })
{{{ $slots.main() }}}`}},
		want: "from component",
	},
	{
		name:  "async render awaits",
		src:   "{{ await Promise.resolve('resolved') }} {{ await value }}",
		state: edge.O("value", "plain"),
		async: true,
		want:  "resolved plain",
	},
	{
		name:      "async component slot",
		src:       "@!component('components/async', {})",
		templates: [][2]string{{"components/async", "{{ await Promise.resolve(1 + 1) }}"}},
		async:     true,
		want:      "2",
	},
	{
		name: "template object",
		src:  "{{ typeof template }}",
		want: "undefined",
	},
	{
		name: "inspect dump",
		src:  "{{ inspect({ a: 1, b: [true, null], c: 'x' }) }}",
		want: `<pre style="
    padding: 30px 25px;
    background-color: rgb(6, 21, 38);
    color: rgb(214, 222, 235);
    border-radius: 6px;
    font-size: 14px;
    overflow: auto;
    white-space: pre;
    word-spacing: normal;
    word-break: normal;
    word-wrap: normal;
    tab-size: 4;
    line-height: 1.4;
    text-align: left;
  "><code style="font-family: JetBrains Mono, Menlo, Monaco, monospace;">{
  <span style="color: rgb(127, 219, 202);">"a"</span>: <span style="color: rgb(199, 146, 234);">1</span>,
  <span style="color: rgb(127, 219, 202);">"b"</span>: [
    <span style="color: rgb(247, 140, 108);">true</span>,
    <span style="color: rgb(255, 203, 139);">null</span>
  ],
  <span style="color: rgb(127, 219, 202);">"c"</span>: <span style="color: rgb(173, 219, 103);">'x'</span>
}</code></pre>`,
	},
	{
		name: "migrate set and globals",
		src: `@set('greeting', 'hi')
{{ greeting }} {{ e('<a>') }} {{ safe('<b>') }} {{{ stringify('</x>') }}}`,
		migrate: true,
		want:    "hi &amp;lt;a&amp;gt; <b> \"\\u003C\\u002Fx\\u003E\"",
	},
	{
		name:    "migrate raise",
		src:     "{{ raise('boom', { line: 3, col: 1, filename: 'x.edge' }) }}",
		migrate: true,
		wantErr: &edge.EdgeError{Message: "boom", Code: "E_RUNTIME_EXCEPTION", Filename: "x.edge", Line: float64(3), Col: float64(1)},
	},
	{
		name: "migrate layout and sections",
		src: `@layout('layouts/main')
@set('title', 'Page')
@section('body')
@super
page body {{ title }}
@end`,
		templates: [][2]string{{"layouts/main", "<title>{{ title }}</title>\n@section('body')\nlayout body\n@end"}},
		migrate:   true,
		want:      "<title>Page</title>\nlayout body\npage body Page",
	},
	{
		name:      "migrate compat props",
		src:       "@!component('components/compat', { className: 'b', class: 'a', id: 'x' })",
		templates: [][2]string{{"components/compat", "{{ $props.serialize({ type: 'button' }) }}|{{ $props.has('id') }}|{{ $props.serializeOnly(['id']) }}|{{ $props.next.get('id') }}"}},
		migrate:   true,
		want:      " class=\"a b\" id=\"x\" type=\"button\"|true| id=\"x\"|x",
	},
	{
		name:    "unknown template",
		path:    "missing",
		mount:   true,
		wantErr: &js.Error{Name: "Error", Message: "Cannot resolve \"/views-that-do-not-exist/missing.edge\". Make sure the file exists"},
	},
}

func newTestEdge(t *testing.T, c renderCase) *edge.Edge {
	t.Helper()
	e := edge.Create(edge.Options{})
	if c.migrate {
		e.Use(migrate.Migrate, js.Undefined)
	}
	if c.mount {
		e.Mount("/views-that-do-not-exist")
	}
	for _, tpl := range c.templates {
		if _, err := e.RegisterTemplate(tpl[0], edge.LoaderTemplate{Template: tpl[1]}); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func render(e *edge.Edge, c renderCase) (string, error) {
	var state any = js.Undefined
	if c.state != nil {
		state = c.state
	}
	switch {
	case c.path != "" && c.async:
		return e.Render(c.path, state)
	case c.path != "":
		return e.RenderSync(c.path, state)
	case c.async:
		return e.RenderRaw(c.src, state, js.Undefined)
	}
	return e.RenderRawSync(c.src, state)
}

func TestRender(t *testing.T) {
	for _, c := range renderCases {
		t.Run(c.name, func(t *testing.T) {
			got, err := render(newTestEdge(t, c), c)
			checkResult(t, got, err, c.want, c.wantErr)
		})
	}
}

// checkResult compares a result with the expected output or error.
func checkResult(t *testing.T, got string, err error, want string, wantErr error) {
	t.Helper()
	switch w := wantErr.(type) {
	case nil:
		if err != nil {
			t.Fatalf("unexpected error %#v", err)
		}
		if got != want {
			t.Fatalf("output\n got %q\nwant %q", got, want)
		}
	case *edge.EdgeError:
		var ee *edge.EdgeError
		if !errors.As(err, &ee) {
			t.Fatalf("got %#v, want an EdgeError", err)
		}
		if !reflect.DeepEqual(ee, w) {
			t.Fatalf("error\n got %#v\nwant %#v", ee, w)
		}
	case *js.Error:
		var je *js.Error
		if !errors.As(err, &je) || je.Name != w.Name || je.Message != w.Message {
			t.Fatalf("got %#v, want %s: %s", err, w.Name, w.Message)
		}
	}
}
