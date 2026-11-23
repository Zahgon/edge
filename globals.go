package edge

import (
	"github.com/edge-js/edge-go/internal/edgert"
	"github.com/edge-js/edge-go/js"
)

// EdgeGlobals are the globals every new Edge instance starts with (Edge's
// edgeGlobals): nl2br, inspect, truncate and excerpt, the html helpers
// (escape, safe, classNames, attrs), js.stringify and the string helpers of
// @poppinss/string (camelCase ... ordinal). Instances copy the object, so
// they share the html and js objects.
var EdgeGlobals = newEdgeGlobals()

func newEdgeGlobals() *js.Object {
	globals := js.ObjectOf(
		"nl2br", edgert.NewNl2br("globals"),
		"inspect", edgert.NewInspect("globals"),
		"truncate", edgert.NewTruncate("globals"),
		"excerpt", edgert.NewExcerpt("globals"),
		"html", js.ObjectOf(
			"escape", edgert.EscapeFunction(),
			"safe", edgert.HTMLSafeFunction(),
			"classNames", edgert.ClassNames(),
			"attrs", edgert.Attrs(),
		),
		"js", js.ObjectOf("stringify", edgert.JSStringify()),
	)
	for _, helper := range edgert.StringHelpers() {
		globals.Set(helper.Name, helper.Value)
	}
	return globals
}
