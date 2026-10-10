package stylesheet

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/cascade"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

// recorder records the style rules (their prelude) with their layer name
// and stylesheet URL, and handles @page only.
type recorder struct {
	layers *cascade.Layers
	rules  []string
}

func (r *recorder) StyleRule(rule parser.QualifiedRule, layer int, baseURL string) bool {
	entry := strings.TrimSpace(parser.Serialize(rule.Prelude))
	if name := r.layers.Name(layer); len(name) > 0 {
		entry += " @" + strings.Join(name, ".")
	}
	r.rules = append(r.rules, entry+" <"+baseURL[strings.LastIndexByte(baseURL, '/')+1:]+">")
	return true
}

func (r *recorder) AtRule(rule parser.AtRule, layer int, baseURL string) bool {
	if rule.AtKeyword == "page" {
		r.rules = append(r.rules, "@page")
		return true
	}
	return false
}

// walkCSS walks css located at http://x/main.css, with sheets served at
// http://x/<name>, and returns the recorded rules and the fetched URLs.
func walkCSS(t *testing.T, css string, sheets map[string]string, opts Options) (rules, fetched []string) {
	t.Helper()
	var layers cascade.Layers
	rec := &recorder{layers: &layers}
	opts.Layers = &layers
	if opts.Fetch == nil && sheets != nil {
		opts.Fetch = func(url string) ([]byte, string, error) {
			fetched = append(fetched, url)
			name := url[strings.LastIndexByte(url, '/')+1:]
			content, ok := sheets[name]
			if !ok {
				return nil, "", fmt.Errorf("not found: %s", url)
			}
			return []byte(content), url, nil
		}
	}
	Walk(parser.ParseStylesheetBytes([]byte(css), false, false), "http://x/main.css", opts, rec)
	return rec.rules, fetched
}

func assertRules(t *testing.T, css string, sheets map[string]string, want ...string) {
	t.Helper()
	got, _ := walkCSS(t, css, sheets, Options{})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s\n got  %q\n want %q", css, got, want)
	}
}

func TestWalkImportForms(t *testing.T) {
	sheets := map[string]string{"a.css": "a {}"}
	for _, imp := range []string{`@import "a.css";`, `@import url(a.css);`, `@import url("a.css");`, `@import url( 'a.css' );`} {
		assertRules(t, imp+" b {}", sheets, "a <a.css>", "b <main.css>")
	}
	assertRules(t, `@import url(a.css b.css); b {}`, sheets, "b <main.css>")
	assertRules(t, `@import 12; b {}`, sheets, "b <main.css>")
}

func TestWalkImportOnlyFirst(t *testing.T) {
	sheets := map[string]string{"a.css": "a {}"}
	// @charset and @layer statements may come before
	assertRules(t, `@charset "utf-8"; @layer x; @import "a.css";`, sheets, "a <a.css>")
	// but not between imports
	assertRules(t, `@import "a.css"; @layer x; @import "a.css";`, sheets, "a <a.css>")
	assertRules(t, `@layer x; @import "a.css"; @import "a.css";`, sheets, "a <a.css>", "a <a.css>")
	assertRules(t, `b {} @import "a.css";`, sheets, "b <main.css>")
	assertRules(t, `@media print { b {} } @import "a.css";`, sheets, "b <main.css>")
	assertRules(t, `@layer x { b {} } @import "a.css";`, sheets, "b @x <main.css>")
	// a rule handled by the host counts; an unknown one does not
	assertRules(t, `@page {} @import "a.css";`, sheets, "@page")
	assertRules(t, `@unknown; @import "a.css";`, sheets, "a <a.css>")
	// nor inside conditional rules
	assertRules(t, `@media print { @import "a.css"; b {} }`, sheets, "b <main.css>")
}

func TestWalkImportConditions(t *testing.T) {
	sheets := map[string]string{"a.css": "a {}"}
	assertRules(t, `@import "a.css" print;`, sheets, "a <a.css>")
	assertRules(t, `@import "a.css" screen;`, sheets)
	assertRules(t, `@import "a.css" supports(display: flex);`, sheets, "a <a.css>")
	assertRules(t, `@import "a.css" supports(display: banana);`, sheets)
	assertRules(t, `@import "a.css" supports((display: flex) and (color: red)) print;`, sheets, "a <a.css>")
	assertRules(t, `@import "a.css" supports(not (display: flex));`, sheets)
	// layer, then supports, then media
	assertRules(t, `@import "a.css" layer(l) supports(display: grid) print;`, sheets, "a @l <a.css>")
}

func TestWalkImportLayers(t *testing.T) {
	sheets := map[string]string{"a.css": "a {} @layer inner { i {} }", "missing.css": ""}
	assertRules(t, `@import "a.css" layer(outer);`, sheets, "a @outer <a.css>", "i @outer.inner <a.css>")
	got, _ := walkCSS(t, `@import "a.css" layer;`, sheets, Options{})
	if len(got) != 2 || !strings.Contains(got[0], "@ <a.css>") {
		t.Errorf("anonymous layer: %q", got)
	}
	// an invalid layer() makes the @import invalid
	assertRules(t, `@import "a.css" layer(a b); b {}`, sheets, "b <main.css>")
	// the layer is declared even if the import fails
	var layers cascade.Layers
	rec := &recorder{layers: &layers}
	Walk(parser.ParseStylesheetBytes([]byte(`@import "nope.css" layer(first); @layer second { b {} }`), false, false),
		"http://x/main.css", Options{Layers: &layers, Fetch: func(string) ([]byte, string, error) { return nil, "", fmt.Errorf("404") }}, rec)
	if layers.Len() != 3 {
		t.Errorf("layers %d, want 3 (outer, first, second)", layers.Len())
	}
	// but not under a false condition
	layers = cascade.Layers{}
	Walk(parser.ParseStylesheetBytes([]byte(`@import "a.css" layer(first) screen;`), false, false),
		"http://x/main.css", Options{Layers: &layers}, rec)
	if layers.Len() != 1 {
		t.Errorf("layer declared under a false condition")
	}
}

func TestWalkImportLimits(t *testing.T) {
	// cycles end the chain; a stylesheet may be imported twice from distinct places
	sheets := map[string]string{
		"main.css": "",
		"a.css":    `@import "b.css"; @import "main.css"; a {}`,
		"b.css":    `@import "a.css"; @import "b.css"; b {}`,
	}
	got, fetched := walkCSS(t, `@import "a.css"; m {}`, sheets, Options{})
	if !reflect.DeepEqual(got, []string{"b <b.css>", "a <a.css>", "m <main.css>"}) || len(fetched) != 2 {
		t.Errorf("cycle: rules %q, fetched %q", got, fetched)
	}
	// depth
	deep := map[string]string{}
	for i := range 40 {
		deep[fmt.Sprintf("s%d.css", i)] = fmt.Sprintf(`@import "s%d.css";`, i+1)
	}
	_, fetched = walkCSS(t, `@import "s0.css";`, deep, Options{MaxImportDepth: 5})
	if len(fetched) != 5 {
		t.Errorf("depth 5: %d fetches", len(fetched))
	}
	// count: a fan-out of 2 per level, 20 levels deep
	fan := map[string]string{}
	for i := range 20 {
		fan[fmt.Sprintf("f%d.css", i)] = fmt.Sprintf(`@import "f%d.css"; @import "f%d.css";`, i+1, i+1)
	}
	_, fetched = walkCSS(t, `@import "f0.css";`, fan, Options{MaxImports: 100})
	if len(fetched) != 100 {
		t.Errorf("count 100: %d fetches", len(fetched))
	}
	// no fetcher: ignored
	got, _ = walkCSS(t, `@import "a.css"; m {}`, nil, Options{})
	if !reflect.DeepEqual(got, []string{"m <main.css>"}) {
		t.Errorf("no fetcher: %q", got)
	}
}

func TestWalkImportRelativeURLs(t *testing.T) {
	sheets := map[string]string{"a.css": `@import "b.css";`, "b.css": "b {}"}
	got, fetched := walkCSS(t, `@import "sub/a.css";`, sheets, Options{})
	if !reflect.DeepEqual(fetched, []string{"http://x/sub/a.css", "http://x/sub/b.css"}) || len(got) != 1 {
		t.Errorf("relative URLs resolve against the importing stylesheet: %q %q", fetched, got)
	}
}

func TestWalkConditionalRules(t *testing.T) {
	assertRules(t, `@media print { a {} } @media screen { b {} } c {}`, nil, "a <main.css>", "c <main.css>")
	assertRules(t, `@supports (display: flex) { a {} } @supports (display: banana) { b {} }`, nil, "a <main.css>")
	assertRules(t, `@media print { @supports (display: flex) { @layer x { a {} } } }`, nil, "a @x <main.css>")
	// with a device
	got, _ := walkCSS(t, `@media (min-width: 500px) { a {} }`, nil, Options{Device: Device{Width: 400, Height: 800}})
	if len(got) != 0 {
		t.Errorf("min-width on a 400px device: %q", got)
	}
}

func TestWalkLayers(t *testing.T) {
	var layers cascade.Layers
	rec := &recorder{layers: &layers}
	css := `@layer b, a; @layer a { x {} } @media screen { @layer never { y {} } } @layer { z {} }`
	Walk(parser.ParseStylesheetBytes([]byte(css), false, false), "http://x/main.css", Options{Layers: &layers}, rec)
	if !reflect.DeepEqual(rec.rules, []string{"x @a <main.css>", "z @ <main.css>"}) {
		t.Errorf("rules %q", rec.rules)
	}
	// b, a, the anonymous layer and the outer layer: "never" is in a false condition
	if layers.Len() != 4 {
		t.Errorf("%d layers", layers.Len())
	}
	assertRules(t, `@layer initial { a {} } b {}`, nil, "b <main.css>")
}

func TestWalkHostAtRules(t *testing.T) {
	assertRules(t, `@page { margin: 0 } @unknown {} a {}`, nil, "@page", "a <main.css>")
}

func TestWalkReportsSizeDependence(t *testing.T) {
	var layers cascade.Layers
	walk := func(css string) Report {
		return Walk(parser.ParseStylesheetBytes([]byte(css), false, false), "http://x/main.css",
			Options{Layers: &layers}, &recorder{layers: &layers})
	}
	for css, want := range map[string]bool{
		`a {} @media print { b {} }`:                  false,
		`@media (min-width: 100px) { a {} }`:          true,
		`@media print and (orientation: portrait) {}`: true,
		`@media (aspect-ratio > 1) {}`:                true,
		`@media (color) {}`:                           false,
		`@media screen { @media (width < 10px) {} }`:  false, // never evaluated
	} {
		if got := walk(css).SizeDependent; got != want {
			t.Errorf("%s: size dependent %v, want %v", css, got, want)
		}
	}
}

func TestWalkReportsSizeQueries(t *testing.T) {
	var layers cascade.Layers
	css := `@media (min-width: 100px) { a {} @media (orientation: portrait) { b {} } } @media print { c {} }`
	report := Walk(parser.ParseStylesheetBytes([]byte(css), false, false), "http://x/main.css",
		Options{Layers: &layers, Device: Device{Width: 200, Height: 400}}, &recorder{layers: &layers})
	if len(report.SizeQueries) != 2 {
		t.Fatalf("%d size queries, want 2 (the nested one is evaluated)", len(report.SizeQueries))
	}
	narrow := Device{Width: 50, Height: 400}
	if EvaluateMediaQuery(report.SizeQueries[0], narrow) {
		t.Error("the recorded query evaluates against another device")
	}
}
