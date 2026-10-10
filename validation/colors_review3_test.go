package validation

import (
	"strings"
	"testing"

	tu "github.com/anaelorlinski/aaoo.cssengine-go/internal/testutils"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

func parseColorText(s string) pa.Color {
	return parseColorResolved(pa.ParseOneComponentValue(pa.Tokenize([]byte(s), true)))
}

// Third review: VA3-4, VA3-8, and relative colors from a device-cmyk()
// currentcolor (PA3-2).
func TestRelativeColorsReview3(t *testing.T) {
	red := pa.ParseColorString("red")
	for input, want := range map[string]pa.RGBA{
		"color-mix(in srgb, rgb(from red r g b), blue)":          {R: 0.5, B: 0.5, A: 1},
		"light-dark(rgb(from red r g b), blue)":                  {R: 1, A: 1},
		"color-mix(in srgb, rgb(from currentcolor r g b), blue)": {R: 0.5, B: 0.5, A: 1},
		"contrast-color(rgb(from currentcolor r g b))":           {A: 1},
	} {
		c := parseColorText(input)
		if c.Type == pa.ColorInvalid {
			t.Errorf("%s: invalid", input)
			continue
		}
		if got := c.Resolve(red).RGBA; !nearRGBA(got, want) {
			t.Errorf("%s: got %v, want %v", input, got, want)
		}
	}
	for _, input := range []string{"hsl(from red 50% s l)", "rgb(from red r g b / )", "rgb(from red r g b / alpha alpha)"} {
		if c := parseColorText(input); c.Type != pa.ColorInvalid {
			t.Errorf("%s: should be invalid", input)
		}
	}
	cmyk := pa.ParseColorString("device-cmyk(1 0 0 0)")
	if got := parseColorText("rgb(from currentcolor r g b)").Resolve(cmyk); got.Type != pa.ColorRGBA {
		t.Errorf("relative color from a cmyk current color: %v", got)
	}
}

func nearRGBA(a, b pa.RGBA) bool {
	d := func(x, y float32) bool { return x-y < 1e-3 && y-x < 1e-3 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// Third review, VA3-6: like-type division gives a <number>, also where
// the property takes numbers only.
func TestLikeTypeDivisionNumber(t *testing.T) {
	for _, c := range []struct{ prop, value string }{
		{"z-index", "calc(10px / 2px)"},
		{"line-height", "calc(10px / 5px)"},
		{"order", "calc(180deg / 90deg)"},
		{"orphans", "calc(1in / 48px)"},
	} {
		if !isValidDeclaration(t, c.prop, c.value) {
			t.Errorf("%s: %s should be valid", c.prop, c.value)
		}
	}
	for _, c := range []struct{ prop, value string }{
		{"z-index", "calc(10px)"},
		{"z-index", "calc(10px * 2px)"},
		{"order", "calc(1em / 1px)"}, // evaluated without the element
	} {
		if isValidDeclaration(t, c.prop, c.value) {
			t.Errorf("%s: %s should be invalid", c.prop, c.value)
		}
	}
}

func isValidDeclaration(t *testing.T, prop, value string) bool {
	tu.CaptureLogs(t) // the invalid ones log
	declarations := pa.ParseDeclarationListString(prop+": "+value, false, false)
	return len(PreprocessDeclarations("", declarations)) > 0
}

// Third review, VA3-7: viewport units are refused in @page size, also in
// math.
func TestPageSizeViewportMath(t *testing.T) {
	for _, value := range []string{"10vw", "calc(10vw)", "calc(1cm + 10vh) 20cm"} {
		if size(pa.RemoveWhitespace(pa.Tokenize([]byte(value), true)), "") != nil {
			t.Errorf("size: %s should be invalid", value)
		}
	}
	if size(pa.RemoveWhitespace(pa.Tokenize([]byte("calc(10cm) 20cm"), true)), "") == nil {
		t.Error("size: calc(10cm) 20cm should be valid")
	}
}

// Third review, ST3-1: the all shorthand.
func TestAllShorthand(t *testing.T) {
	// expandToDict leaves out the initial values
	got := expandToDict(t, "all: inherit", "")
	if got[pr.PDisplay] != pr.Inherit || got[pr.PColor] != pr.Inherit || got[pr.PMarginTop] != pr.Inherit {
		t.Errorf("all: inherit → display %v, color %v", got[pr.PDisplay], got[pr.PColor])
	}
	if _, has := got[pr.PDirection]; has {
		t.Error("all must not set direction")
	}
	got = expandToDict(t, "all: unset", "")
	if _, has := got[pr.PDisplay]; has || got[pr.PColor] != pr.Inherit {
		t.Errorf("all: unset → display %v, color %v", got[pr.PDisplay], got[pr.PColor])
	}
	assertInvalid(t, "all: red", "all only takes a CSS-wide keyword")
}

// Third review, MO3-2: relative urls substituted through var() resolve
// against the sheet that declares them.
func TestURLThroughVar(t *testing.T) {
	declarations := pa.ParseDeclarationListString("--i: url(a.png); background-image: var(--i), url(\"b.png\")", false, false)
	got := PreprocessDeclarations("http://example.com/dir/page.css", declarations)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	for i, want := range []string{"http://example.com/dir/a.png", "http://example.com/dir/b.png"} {
		if s := pa.Serialize(got[i].Value.(pr.RawTokens)); !strings.Contains(s, want) {
			t.Errorf("declaration %d: %s, want %s", i, s, want)
		}
	}
}
