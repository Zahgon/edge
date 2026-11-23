package migrate_test

// The expected values were recorded by rendering the same templates with
// edge.js 6.5.1 and its migrate plugin on Node 24.

import (
	"errors"
	"reflect"
	"testing"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
	"github.com/edge-js/edge-go/plugins/migrate"
)

var cases = []struct {
	name string
	// templates are registered in memory before src is rendered.
	templates [][2]string
	src       string
	state     *js.Object
	want      string
	wantErr   *edge.EdgeError
}{
	{
		name: "layout, sections and super",
		templates: [][2]string{
			{"layouts/main", "<title>\n@section('title')\nDefault title\n@end\n</title>\n<main>\n@section('body')\n<p>base</p>\n@end\n</main>"},
		},
		src:  "@layout('layouts/main')\n@section('title')\nHome\n@end\n@section('body')\n@super\n<p>page</p>\n@end",
		want: "<title>\nHome\n</title>\n<main>\n<p>base</p>\n<p>page</p>\n</main>",
	},
	{
		name: "section kept from the layout",
		templates: [][2]string{
			{"layouts/main", "<title>\n@section('title')\nDefault title\n@end\n</title>\n<main>\n@section('body')\n<p>base</p>\n@end\n</main>"},
		},
		src:  "@layout('layouts/main')\n@section('title')\nAbout\n@end",
		want: "<title>\nAbout\n</title>\n<main>\n<p>base</p>\n</main>",
	},
	{
		name: "set and reassign",
		src:  "@set('total', 10)\n@set('total', total + 5)\n{{ total }}",
		want: "15",
	},
	{
		name:  "set on a path",
		src:   "@set(user, 'name', 'virk')\n{{ user.name }}",
		state: edge.O("user", edge.O("name", "x")),
		want:  "virk",
	},
	{
		name: "compat globals",
		src:  "{{ e('<b>') }} {{ safe('<i>') }} {{{ stringify({ a: '</x>' }) }}} {{ truncate('hello world wide', 8) }} {{ camelCase('a_b') }}",
		want: `&amp;lt;b&amp;gt; <i> {"a":"\u003C\u002Fx\u003E"} hello world... aB`,
	},
	{
		name: "nl2br and excerpt",
		src:  `{{{ nl2br('a\nb') }}} {{ excerpt('<p>Hello <b>world</b> again</p>', 8) }}`,
		want: "a<br>b Hello world...",
	},
	{
		name:    "raise without options",
		src:     "{{ raise('boom') }}",
		wantErr: &edge.EdgeError{Message: "boom", Code: "E_RUNTIME_EXCEPTION", Filename: "eval.edge", Line: 1.0, Col: 0.0},
	},
	{
		name:    "raise with options",
		src:     "{{ raise('bad input', { line: 3, col: 4, filename: 'x.edge' }) }}",
		wantErr: &edge.EdgeError{Message: "bad input", Code: "E_RUNTIME_EXCEPTION", Filename: "x.edge", Line: 3.0, Col: 4.0},
	},
	{
		name: "props serializeExcept",
		templates: [][2]string{
			{"components/button", "<button {{ $props.serializeExcept(['text']) }}>{{ text }}</button>"},
		},
		src:  "@!component('components/button', { text: 'Go', type: 'submit', className: 'big' })",
		want: `<button  type="submit" class="big">Go</button>`,
	},
	{
		name: "props serialize with merge",
		templates: [][2]string{
			{"components/a", "<a {{ $props.serialize({ class: 'm', href: '#' }) }}></a>|<a {{ $props.serialize({ href: '#' }, false) }}></a>"},
		},
		src:  "@!component('components/a', { href: '/home', class: ['x'] })",
		want: `<a  href="#" class="m"></a>|<a  href="/home" class="x"></a>`,
	},
	{
		name: "props serializeOnly",
		templates: [][2]string{
			{"components/i", "<input {{ $props.serializeOnly(['name', 'value'], { type: 'text' }) }}>"},
		},
		src:  `@!component('components/i', { name: 'q', value: 'a "b"', id: 'q1' })`,
		want: `<input  name="q" value="a &quot;b&quot;" type="text">`,
	},
	{
		name: "props readers",
		templates: [][2]string{
			{"components/r", "{{ $props.has('a') }} {{ $props.has('z') }} {{ $props.get('n.k', 'd') }} {{ $props.get('z', 'd') }} {{ JSON.stringify($props.only(['a'])) }} {{ JSON.stringify($props.except(['a'])) }} {{ a }}"},
		},
		src:  "@!component('components/r', { a: 1, n: { k: 'v' } })",
		want: "true false v d {&quot;a&quot;:1} {&quot;n&quot;:{&quot;k&quot;:&quot;v&quot;}} 1",
	},
	{
		name: "component next props",
		templates: [][2]string{
			{"components/n", "{{ $props.next.toAttrs() }}"},
		},
		src:  "@!component('components/n', { id: 'n1', disabled: true })",
		want: `id="n1" disabled`,
	},
	{
		name: "text outside sections is dropped",
		templates: [][2]string{
			{"layouts/main", "<title>\n@section('title')\nDefault title\n@end\n</title>\n<main>\n@section('body')\n<p>base</p>\n@end\n</main>"},
		},
		src:     "@layout('layouts/main')\nignored\n@section('body')\nmine\n@end",
		wantErr: &edge.EdgeError{Message: `Template extending a layout can only use "@section" or "@set" tags as top level nodes`, Code: "E_UNALLOWED_EXPRESSION", Filename: "eval.edge", Line: 2.0, Col: 0.0},
	},
	{
		name: "layout must come first",
		templates: [][2]string{
			{"layouts/main", "<title>\n@section('title')\nDefault title\n@end\n</title>\n<main>\n@section('body')\n<p>base</p>\n@end\n</main>"},
		},
		src:  "x\n@layout('layouts/main')",
		want: "x",
	},
}

func TestMigrate(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := edge.Create(edge.Options{})
			e.Use(migrate.Migrate, js.Undefined)
			for _, tpl := range c.templates {
				if _, err := e.RegisterTemplate(tpl[0], edge.LoaderTemplate{Template: tpl[1]}); err != nil {
					t.Fatal(err)
				}
			}
			var state any = js.Undefined
			if c.state != nil {
				state = c.state
			}
			got, err := e.RenderRawSync(c.src, state)
			if c.wantErr != nil {
				var ee *edge.EdgeError
				if !errors.As(err, &ee) || !reflect.DeepEqual(ee, c.wantErr) {
					t.Fatalf("got %q, %#v; want error %#v", got, err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got != c.want {
				t.Errorf("output\n got %q\nwant %q", got, c.want)
			}
		})
	}
}
