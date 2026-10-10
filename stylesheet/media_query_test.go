package stylesheet

import (
	"testing"

	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// evalQuery parses a media query list from a string and evaluates it.
func evalQuery(t *testing.T, query string, device Device) bool {
	t.Helper()
	return EvaluateMediaQuery(ParseMediaQueryString(query), device)
}

func TestMediaQueryTypes(t *testing.T) {
	printDev := Device{MediaType: "print"}
	screenDev := Device{MediaType: "screen"}

	for _, test := range []struct {
		query  string
		device Device
		want   bool
	}{
		{"", printDev, true},
		{"all", printDev, true},
		{"all", screenDev, true},
		{"print", printDev, true},
		{"print", screenDev, false},
		{"screen", screenDev, true},
		{"screen, print", printDev, true},
		{"tv, projection", printDev, false},
		{"only print", printDev, true},
		{"not print", printDev, false},
		{"not print", screenDev, true},
		{"not screen", printDev, true},
		// default media type is print
		{"print", Device{}, true},
	} {
		if got := evalQuery(t, test.query, test.device); got != test.want {
			t.Errorf("query %q on %+v: got %v, want %v", test.query, test.device, got, test.want)
		}
	}
}

func TestMediaQuerySizeFeatures(t *testing.T) {
	// 800x1200 CSS px, portrait
	dev := Device{MediaType: "print", Width: 800, Height: 1200}

	for _, test := range []struct {
		query string
		want  bool
	}{
		// plain / min / max, colon form
		{"(width: 800px)", true},
		{"(width: 700px)", false},
		{"(min-width: 800px)", true},
		{"(min-width: 801px)", false},
		{"(max-width: 800px)", true},
		{"(max-width: 799px)", false},
		{"(min-height: 1000px)", true},
		{"(max-height: 1000px)", false},
		// units: 50em = 800px at the initial font size
		{"(min-width: 50em)", true},
		{"(min-width: 50.1rem)", false},
		{"(max-width: 30cm)", true},
		{"(width: 0)", false}, // unitless zero is valid syntax
		// combined with a type
		{"print and (min-width: 400px)", true},
		{"screen and (min-width: 400px)", false},
		{"print and (min-width: 400px) and (max-width: 900px)", true},
		{"print and (min-width: 400px) and (max-width: 700px)", false},
		// comma list: one alternative suffices
		{"screen and (min-width: 400px), (max-height: 2000px)", true},
		// negation applies to the full query
		{"not print and (min-width: 900px)", true},
		{"not print and (min-width: 400px)", false},
		// boolean feature
		{"(width)", true},
		{"(orientation)", true},
		// orientation
		{"(orientation: portrait)", true},
		{"(orientation: landscape)", false},
		// aspect-ratio (800/1200 = 2/3)
		{"(aspect-ratio: 2/3)", true},
		{"(min-aspect-ratio: 1/2)", true},
		{"(max-aspect-ratio: 1/2)", false},
		// range syntax
		{"(width >= 800px)", true},
		{"(width > 800px)", false},
		{"(width < 900px)", true},
		{"(800px <= width)", true},
		{"(900px <= width)", false},
		{"(400px < width <= 800px)", true},
		{"(400px < width < 800px)", false},
		{"(width = 800px)", true},
		// or
		{"(min-width: 900px) or (max-width: 900px)", true},
		{"(min-width: 900px) or (min-height: 2000px)", false},
	} {
		if got := evalQuery(t, test.query, dev); got != test.want {
			t.Errorf("query %q: got %v, want %v", test.query, got, test.want)
		}
	}
}

func TestMediaQueryLandscape(t *testing.T) {
	dev := Device{MediaType: "print", Width: 1200, Height: 800}
	if !evalQuery(t, "(orientation: landscape)", dev) {
		t.Error("expected landscape to match on 1200x800")
	}
	if evalQuery(t, "(orientation: portrait)", dev) {
		t.Error("expected portrait not to match on 1200x800")
	}
}

func TestMediaQueryDefaultsToA4(t *testing.T) {
	// no size set: A4 portrait, 210mm x 297mm = 793.7 x 1122.5 px
	dev := Device{MediaType: "print"}
	if !evalQuery(t, "(min-width: 205mm)", dev) {
		t.Error("A4 default width should satisfy (min-width: 205mm)")
	}
	if evalQuery(t, "(min-width: 215mm)", dev) {
		t.Error("A4 default width should not satisfy (min-width: 215mm)")
	}
	if !evalQuery(t, "(orientation: portrait)", dev) {
		t.Error("A4 default should be portrait")
	}
	if w, h := dev.size(); w != pr.InitialPageSize()[0] || h != pr.InitialPageSize()[1] {
		t.Errorf("unexpected default size %gx%g", w, h)
	}
}

func TestMediaQueryInvalid(t *testing.T) {
	dev := Device{MediaType: "print", Width: 800, Height: 1200}

	for _, test := range []struct {
		query string
		want  bool
	}{
		// unknown feature: that alternative never matches...
		{"(min-monochrome: 1)", false},
		{"print and (foo: bar)", false},
		// ...even negated (spec: unknown => "not all")
		{"not (foo: bar)", false},
		// ...but sibling alternatives still work
		{"(foo: bar), print", true},
		// syntax errors
		{"(min-width: )", false},
		{"(min-width: blue)", false},
		{"(min-width: 50%)", false},
		{"print and or (width)", false},
		{"(width) or print and (height)", false},
		// mixing and/or in one query is invalid
		{"(width) and (height) or (orientation)", false},
	} {
		if got := evalQuery(t, test.query, dev); got != test.want {
			t.Errorf("query %q: got %v, want %v", test.query, got, test.want)
		}
	}
}

// T9: a leading "not" applies to a condition without media type.
func TestMediaQueryNotCondition(t *testing.T) {
	dev := Device{MediaType: "print", Width: 800, Height: 1200}
	for _, test := range []struct {
		query string
		want  bool
	}{
		{"not (max-width: 100px)", true},
		{"not (min-width: 100px)", false},
		{"not (width < 100px)", true},
		{"not (width > 100px)", false},
		{"not print and (max-width: 100px)", true},
		{"not (width > 100px) and (height > 100px)", false}, // invalid
		{"(width > 100px) and (height > 100px)", true},
	} {
		if got := evalQuery(t, test.query, dev); got != test.want {
			t.Errorf("query %q: got %v, want %v", test.query, got, test.want)
		}
	}
}

// T10: color and monochrome features.
func TestMediaQueryColor(t *testing.T) {
	dev := Device{MediaType: "print"}
	for _, test := range []struct {
		query string
		want  bool
	}{
		{"(color)", true},
		{"print and (color)", true},
		{"not print and (color)", false},
		{"(min-color: 1)", true},
		{"(min-color: 9)", false},
		{"(color > 4)", true},
		{"(monochrome)", false},
		{"(monochrome: 0)", true},
		{"(min-monochrome: 1)", false},
		{"not (monochrome)", true},
		{"(color-index)", false},
		{"(color: 1.5)", false},
	} {
		if got := evalQuery(t, test.query, dev); got != test.want {
			t.Errorf("query %q: got %v, want %v", test.query, got, test.want)
		}
	}
}

// Media Queries 4: malformed queries are "not all" (never a panic), nested
// conditions and <general-enclosed> parse, unknown tests are "unknown".
func TestMediaQueries4(t *testing.T) {
	device := Device{Width: 800, Height: 600} // print, landscape, color
	for query, want := range map[string]bool{
		// CS1: a dangling connective
		"print and":         false,
		"(color) and":       false,
		"(color) or":        false,
		"not print and":     false,
		"print and (color)": true,
		// CS5: empty items, bare only
		"screen, ":       false,
		",":              false,
		"only":           false,
		"only (color)":   false,
		"only print":     true,
		"not only print": false,
		"print, ":        true, // the valid item still applies
		// CS6: nested conditions
		"not ((max-width: 1px) and (color))":                       true,
		"print and (not (monochrome))":                             true,
		"((color) or (max-width: 1px))":                            true,
		"((color) and (max-width: 1px))":                           false,
		"(not (color)) or (min-width: 100px)":                      true,
		"(color) and (width > 100px) and (orientation: landscape)": true,
		"(color) and (width > 100px) or (monochrome)":              false, // mixing is invalid
		"print and (color) or (monochrome)":                        false, // no or after a type
		// CS6: unknown is neither true nor false
		"(width > 1px) or (foo: bar)":  true,
		"(width > 1px) and (foo: bar)": false,
		"not (foo: bar)":               false,
		"not (resolution: 2dppx)":      true, // a 1dppx device
		"(color) or unknown-fn(x)":     true,
		"(width: red) or (color)":      true,
		// CS11: "< =" is not an operator
		"(width < = 99999px)": false,
		"(width <= 99999px)":  true,
		// reserved words are not media types
		"layer": false,
		"and":   false,
	} {
		list := ParseMediaQueryString(query)
		if got := EvaluateMediaQuery(list, device); got != want {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
	// size dependence looks inside nested conditions
	if !DependsOnSize(ParseMediaQueryString("not ((color) and (min-width: 1px))")) {
		t.Error("nested size feature")
	}
}

// C2-2: an exact ratio of the device size matches.
func TestMediaAspectRatioExact(t *testing.T) {
	a4 := Device{} // A4 portrait, 793.7 x 1122.5 px
	for query, want := range map[string]bool{
		"(aspect-ratio: 210/297)": true, "(min-aspect-ratio: 210/297)": true, "(max-aspect-ratio: 210/297)": true,
		"(aspect-ratio > 210/297)": false, "(aspect-ratio < 210/297)": false, "(aspect-ratio: 1/1)": false,
	} {
		if got := EvaluateMediaQuery(ParseMediaQueryString(query), a4); got != want {
			t.Errorf("%s on A4: %v, want %v", query, got, want)
		}
	}
	if !EvaluateMediaQuery(ParseMediaQueryString("(aspect-ratio: 4/3)"), Device{Width: 800, Height: 600}) {
		t.Error("4/3 on 800x600")
	}
}

// C2-4: the discrete features have the values of a static paged device.
func TestMediaDiscreteFeatures(t *testing.T) {
	for query, want := range map[string]bool{
		"(scripting: none)": true, "(scripting: enabled)": false, "(scripting)": false,
		"(update: none)": true, "(hover: none)": true, "(hover: hover)": false, "(hover)": false,
		"(pointer: none)": true, "(any-pointer: fine)": false,
		"(overflow-block: paged)": true, "(overflow-block)": true, "(overflow-inline: none)": true,
		"(prefers-color-scheme: light)": true, "(prefers-color-scheme: dark)": false,
		"not (prefers-color-scheme: dark)": true, "(prefers-color-scheme)": true,
		"(prefers-reduced-motion: no-preference)": true, "(prefers-reduced-motion)": false,
		"(color-gamut: srgb)": true, "(color-gamut: p3)": false, "(forced-colors: none)": true,
		"(resolution: 96dpi)": true, "(resolution: 1dppx)": true, "(min-resolution: 2dppx)": false,
		"(max-resolution: 1x)": true, "(resolution > 0.5dppx)": true,
		"(grid: 0)": true, "(grid)": false,
		// an unknown value is unknown
		"(hover: banana) or (color)": true, "not (hover: banana)": false,
	} {
		if got := EvaluateMediaQuery(ParseMediaQueryString(query), Device{}); got != want {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
}
