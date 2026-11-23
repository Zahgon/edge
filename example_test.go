package edge_test

import (
	"fmt"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
	"github.com/edge-js/edge-go/plugins/migrate"
)

func Example() {
	e := edge.Create(edge.Options{})
	e.Global("appName", "Edge")
	if _, err := e.RegisterTemplate("components/alert", edge.LoaderTemplate{
		Template: "<div {{ $props.merge({ class: ['alert'] }).toAttrs() }}>{{{ $slots.main() }}}</div>",
	}); err != nil {
		panic(err)
	}
	html, err := e.RenderRawSync(`Welcome to {{ appName }}, {{ user.name }}!
@each(task in tasks)
- {{ task }}
@end
@component('components/alert', { class: 'warning' })
  You have {{ tasks.length }} tasks.
@end`, edge.O(
		"user", edge.O("name", "<virk>"),
		"tasks", edge.A("Write docs", "Ship"),
	))
	if err != nil {
		panic(err)
	}
	fmt.Println(html)
	// Output:
	// Welcome to Edge, &lt;virk&gt;!
	// - Write docs
	// - Ship<div class="alert warning">  You have 2 tasks.</div>
}

func ExampleEdge_Render_errors() {
	e := edge.Create(edge.Options{})
	_, err := e.Render("missing", js.Undefined)
	fmt.Println(err)
	_, err = e.RenderRaw("Hello\n{{ user.name }}", nil, js.Undefined)
	if ee, ok := err.(*edge.EdgeError); ok {
		fmt.Println(ee.Code, ee.Filename, ee.Line, ee.Message)
	}
	// Output:
	// "default" namespace is not mounted
	// E_RUNTIME_EXCEPTION eval.edge 2 Cannot read properties of undefined (reading 'name')
}

func ExampleMigrate() {
	e := edge.Create(edge.Options{})
	e.Use(migrate.Migrate, js.Undefined)
	if _, err := e.RegisterTemplate("layouts/main", edge.LoaderTemplate{
		Template: "<title>{{ title }}</title>\n@section('body')\nlayout\n@end",
	}); err != nil {
		panic(err)
	}
	html, err := e.RenderRawSync("@layout('layouts/main')\n@set('title', 'Home')\n@section('body')\n@super\npage\n@end", js.Undefined)
	if err != nil {
		panic(err)
	}
	fmt.Println(html)
	// Output:
	// <title>Home</title>
	// layout
	// page
}
