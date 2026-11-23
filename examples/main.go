// Command examples serves the welcome page of examples/views: a compat-mode
// template (the migrate plugin) that dumps the user, renders the modal
// component with its slots and includes a partial. Run it from the module
// root with `go run ./examples` and open http://127.0.0.1:3000; every
// rendered page is also printed. With -once the page is rendered once to
// standard output instead.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	edge "github.com/edge-js/edge-go"
	"github.com/edge-js/edge-go/js"
	"github.com/edge-js/edge-go/plugins/migrate"
)

func main() {
	views := flag.String("views", filepath.Join("examples", "views"), "the directory of the views")
	addr := flag.String("addr", "127.0.0.1:3000", "the address to listen on")
	once := flag.Bool("once", false, "render the page once to standard output and exit")
	flag.Parse()

	// Edge resolves template paths against the mounted directory as given,
	// so it is mounted by its absolute path.
	dir, err := filepath.Abs(*views)
	if err != nil {
		log.Fatal(err)
	}
	e := edge.Create(edge.Options{})
	e.Use(migrate.Migrate, js.Undefined)
	e.Mount(dir)

	user := edge.O(
		"isModel", true,
		"foo", true,
		"attributes", edge.O(
			"username", "virk",
			"email", "virk@adonisjs.com",
			"isAdmin", true,
			"profile", edge.O("avatarUrl", "foo"),
			"lastLoginAt", nil,
		),
	)
	user.Set("parent", user)

	render := func() (string, error) { return e.Render("welcome", edge.O("user", user)) }
	if *once {
		html, err := render()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(html)
		return
	}

	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		html, err := render()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println(html)
		w.Header().Set("content-type", "text/html")
		if _, err := fmt.Fprint(w, html); err != nil {
			log.Print(err)
		}
	})
	fmt.Println("Listening on " + *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
