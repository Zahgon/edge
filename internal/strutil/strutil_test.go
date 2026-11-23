package strutil

import "testing"

// The expected values below were printed by @poppinss/utils 7.1.2's string
// helpers on Node 24.

func TestCases(t *testing.T) {
	// A titleCase that threw on a value that is not a string leaves its
	// expression's lastIndex behind; the cases start from a fresh module.
	titleTokensLastIndex.Lock()
	titleTokensLastIndex.n = 0
	titleTokensLastIndex.Unlock()
	type row struct{ camel, pascal, snake, capital, no, sentence, title, dash, dashCap, dot, dotLower string }
	for in, want := range map[string]row{
		"hello world":         {"helloWorld", "HelloWorld", "hello_world", "Hello World", "hello world", "Hello world", "Hello World", "hello-world", "Hello-World", "hello.world", "hello.world"},
		"XMLHttpRequest":      {"xmlHttpRequest", "XmlHttpRequest", "xml_http_request", "Xml Http Request", "xml http request", "XML http request", "XMLHttpRequest", "xml-http-request", "Xml-Http-Request", "XML.Http.Request", "xml.http.request"},
		"hello_world-foo.bar": {"helloWorldFooBar", "HelloWorldFooBar", "hello_world_foo_bar", "Hello_World-Foo.Bar", "hello world foo bar", "Hello world foo bar", "Hello_world-foo.bar", "hello-world-foo-bar", "Hello-World-Foo-Bar", "hello.world.foo.bar", "hello.world.foo.bar"},
		"Élan vital":          {"elanVital", "ElanVital", "elan_vital", "Élan Vital", "lan vital", "Lan vital", "Élan Vital", "elan-vital", "Elan-Vital", "Elan.vital", "elan.vital"},
		"  spaced  out ":      {"spacedOut", "SpacedOut", "spaced_out", "Spaced Out", "spaced out", "Spaced out", "  Spaced  Out ", "spaced-out", "Spaced-Out", "spaced.out", "spaced.out"},
	} {
		got := row{CamelCase(in), PascalCase(in), SnakeCase(in), CapitalCase(in), NoCase(in), SentenceCase(in), TitleCase(in),
			DashCase(in, false), DashCase(in, true), DotCase(in, false), DotCase(in, true)}
		if got != want {
			t.Errorf("%q:\n got %+v\nwant %+v", in, got, want)
		}
	}
	const title = "the lord of the rings: a tale of two towers"
	if got, want := TitleCase(title), "The Lord of the Rings: a Tale of Two Towers"; got != want {
		t.Errorf("TitleCase(%q) = %q, want %q", title, got, want)
	}
}

func TestPluralize(t *testing.T) {
	for _, c := range []struct{ word, plural, singular string }{
		{"person", "people", "person"},
		{"Child", "Children", "Child"},
		{"SHEEP", "SHEEP", "SHEEP"},
		{"bias", "biases", "bia"},
		{"veto", "vetoes", "veto"},
		{"matrix", "matrices", "matrix"},
		{"quiz", "quizzes", "quiz"},
		{"octopus", "octopuses", "octopus"},
		{"datum", "data", "datum"},
		{"people", "people", "person"},
		{"children", "children", "child"},
		{"vetoes", "vetoes", "veto"},
		{"buses", "buses", "bus"},
	} {
		if got := Plural(c.word); got != c.plural {
			t.Errorf("Plural(%q) = %q, want %q", c.word, got, c.plural)
		}
		if got := Singular(c.word); got != c.singular {
			t.Errorf("Singular(%q) = %q, want %q", c.word, got, c.singular)
		}
	}
	for _, c := range []struct {
		count     float64
		inclusive bool
		want      string
	}{{1, false, "box"}, {3, true, "3 boxes"}, {0, true, "0 boxes"}} {
		if got := Pluralize("box", c.count, c.inclusive); got != c.want {
			t.Errorf("Pluralize(box, %v, %v) = %q, want %q", c.count, c.inclusive, got, c.want)
		}
	}
}

func TestSentenceAndOrdinal(t *testing.T) {
	if got := Sentence([]string{"a", "b", "c"}, ", ", ", and ", " and "); got != "a, b, and c" {
		t.Errorf("Sentence = %q", got)
	}
	if got := Sentence([]string{"a", "b"}, ", ", ", and ", " and "); got != "a and b" {
		t.Errorf("Sentence pair = %q", got)
	}
	if got := Sentence([]string{"a", "b", "c"}, "; ", " or ", " and "); got != "a; b or c" {
		t.Errorf("Sentence with separators = %q", got)
	}
	for n, want := range map[float64]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th",
		21: "21st", 22: "22nd", 23: "23rd", 101: "101st", 111: "111th", -1: "-1st", 1.5: "1.5th"} {
		if got, err := Ordinal(n); err != nil || got != want {
			t.Errorf("Ordinal(%v) = %q, %v; want %q", n, got, err, want)
		}
	}
}

func TestMilliseconds(t *testing.T) {
	for _, c := range []struct {
		ms          float64
		short, long string
	}{
		{0, "0ms", "0 ms"}, {999, "999ms", "999 ms"}, {1000, "1s", "1 second"}, {1500, "2s", "2 seconds"},
		{60000, "1m", "1 minute"}, {3600000, "1h", "1 hour"}, {86400000, "1d", "1 day"},
		{31557600000, "1y", "1 year"}, {-1500, "-2s", "-2 seconds"}, {1.5, "1.5ms", "1.5 ms"},
	} {
		if got := FormatMilliseconds(c.ms, false); got != c.short {
			t.Errorf("FormatMilliseconds(%v) = %q, want %q", c.ms, got, c.short)
		}
		if got := FormatMilliseconds(c.ms, true); got != c.long {
			t.Errorf("FormatMilliseconds(%v, long) = %q, want %q", c.ms, got, c.long)
		}
	}
	for in, want := range map[string]float64{"1s": 1000, "1.5h": 5400000, "2 days": 172800000, "1y": 31557600000,
		"-1m": -60000, "100": 100, "1 week": 604800000} {
		if got, err := ParseMilliseconds(in); err != nil || got != want {
			t.Errorf("ParseMilliseconds(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", "0s"} {
		if _, err := ParseMilliseconds(in); err == nil || err.Error() != `Invalid duration expression "`+in+`"` {
			t.Errorf("ParseMilliseconds(%q) error = %v", in, err)
		}
	}
}

func TestBytes(t *testing.T) {
	plain := BytesOptions{DecimalPlaces: 2}
	for _, c := range []struct {
		value float64
		opts  BytesOptions
		want  string
	}{
		{0, plain, "0B"}, {1023, plain, "1023B"}, {1024, plain, "1KB"}, {1536, plain, "1.5KB"}, {1048576, plain, "1MB"},
		{1234567890, BytesOptions{DecimalPlaces: 3}, "1.15GB"},
		{1024, BytesOptions{DecimalPlaces: 2, FixedDecimals: true}, "1.00KB"},
		{1234567, BytesOptions{DecimalPlaces: 2, Unit: "KB", ThousandsSeparator: ","}, "1,205.63KB"},
		{1536, BytesOptions{DecimalPlaces: 2, UnitSeparator: " "}, "1.5 KB"},
		{-1024, plain, "-1KB"},
	} {
		if got, ok, err := FormatBytes(c.value, c.opts); err != nil || !ok || got != c.want {
			t.Errorf("FormatBytes(%v, %+v) = %q, %v, %v; want %q", c.value, c.opts, got, ok, err, c.want)
		}
	}
	for in, want := range map[string]float64{"1KB": 1024, "1.5 mb": 1572864, "1024": 1024, "10 GB": 10737418240,
		"1pb": 1125899906842624, "-1kb": -1024} {
		if got, ok := ParseBytes(in); !ok || got != want {
			t.Errorf("ParseBytes(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if got, ok := ParseBytes("abc"); ok {
		t.Errorf("ParseBytes(abc) = %v; want null", got)
	}
}

func TestTruncate(t *testing.T) {
	const s = "Lorem ipsum dolor sit amet, consectetur"
	for _, c := range []struct {
		limit         float64
		completeWords bool
		suffix        string
		want          string
	}{
		{10, false, "...", "Lorem ipsu..."},
		{10, true, "...", "Lorem ipsum..."},
		{10, false, "…", "Lorem ipsu…"},
		{100, false, "...", s},
		{11, true, " [more]", "Lorem ipsum [more]"},
	} {
		if got := Truncate(s, c.limit, c.completeWords, c.suffix); got != c.want {
			t.Errorf("Truncate(%v, %v, %q) = %q, want %q", c.limit, c.completeWords, c.suffix, got, c.want)
		}
		if got := Excerpt(s, c.limit, c.completeWords, c.suffix); got != c.want {
			t.Errorf("Excerpt(%v, %v, %q) = %q, want %q", c.limit, c.completeWords, c.suffix, got, c.want)
		}
	}
	const html = "<p>Hello <b>world</b> again</p>"
	if got, want := Truncate(html, 8, false, "..."), "<p>Hello <b>wo</b>...</p>"; got != want {
		t.Errorf("Truncate(html) = %q, want %q", got, want)
	}
	if got, want := Excerpt(html, 8, false, "..."), "Hello wo..."; got != want {
		t.Errorf("Excerpt(html) = %q, want %q", got, want)
	}
	if got, want := Excerpt("<p>one</p>\n\n<p>two words</p>", 5, false, "..."), "one tw..."; got != want {
		t.Errorf("Excerpt(paragraphs) = %q, want %q", got, want)
	}
	for in, want := range map[string]string{`a\b\c`: "a/b/c", `\\?\C:\x`: `\\?\C:\x`, `C:\x/y`: "C:/x/y"} {
		if got := ToUnixSlash(in); got != want {
			t.Errorf("ToUnixSlash(%q) = %q, want %q", in, got, want)
		}
	}
}
