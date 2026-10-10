package parser

import "testing"

// Color functions using currentcolor are computed once its value is known.
func TestColorDependsOnCurrent(t *testing.T) {
	red := ParseColorString("red")
	for input, want := range map[string]RGBA{
		"color-mix(in srgb, currentcolor, blue)":                {R: 0.5, B: 0.5, A: 1},
		"color-mix(in srgb, currentcolor 25%, blue)":            {R: 0.25, B: 0.75, A: 1},
		"contrast-color(currentcolor)":                          {A: 1},
		"light-dark(currentcolor, blue)":                        {R: 1, A: 1},
		"color-mix(in srgb, contrast-color(currentcolor), red)": {R: 0.5, A: 1},
	} {
		c := ParseColorString(input)
		if c.Type == ColorInvalid {
			t.Fatalf("%s: invalid", input)
		}
		got := c.Resolve(red).RGBA
		if !closeRGBA(got, want) {
			t.Errorf("%s: got %v, want %v", input, got, want)
		}
	}
	// the syntax is still checked
	for _, input := range []string{
		"color-mix(in srgb, currentcolor 150%, blue)",
		"color-mix(in srgb, currentcolor, foo)",
		"contrast-color(currentcolor, red)",
	} {
		if c := ParseColorString(input); c.Type != ColorInvalid {
			t.Errorf("%s: should be invalid", input)
		}
	}
	// currentcolor alone resolves to the current color, others are unchanged
	if got := ParseColorString("currentcolor").Resolve(red); got.RGBA != red.RGBA {
		t.Errorf("currentcolor: got %v", got.RGBA)
	}
	blue := ParseColorString("blue")
	if got := blue.Resolve(red); got.RGBA != blue.RGBA {
		t.Errorf("blue: got %v", got.RGBA)
	}
	// a current color built from an RGBA only
	if got := ParseColorString("color-mix(in srgb, currentcolor, blue)").Resolve(Color{Type: ColorRGBA, RGBA: RGBA{G: 1, A: 1}}); !closeRGBA(got.RGBA, RGBA{G: 0.5, B: 0.5, A: 1}) {
		t.Errorf("RGBA-only current: got %v", got.RGBA)
	}
}

func closeRGBA(a, b RGBA) bool {
	d := func(x, y float32) bool { return x-y < 1e-3 && y-x < 1e-3 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

// Third review: PA3-1, PA3-2, PA3-3, PA3-5, PA3-6.
func TestColorMixReview3(t *testing.T) {
	// an empty interpolation method is invalid, not a panic
	for _, input := range []string{"color-mix(,red,blue)", "color-mix( , red, blue)", "color-mix(in, red, blue)"} {
		if c := ParseColorString(input); c.Type != ColorInvalid {
			t.Errorf("%s: should be invalid", input)
		}
	}
	// a device-cmyk() current color mixes through its sRGB rendering
	cmyk := ParseColorString("device-cmyk(1 0 0 0)")
	for _, input := range []string{"color-mix(in srgb, currentcolor, blue)", "contrast-color(currentcolor)"} {
		if got := ParseColorString(input).Resolve(cmyk); got.Type != ColorRGBA {
			t.Errorf("%s with a cmyk current color: %v", input, got)
		}
	}
	// the other color is checked even when the mix waits for currentcolor
	if c := ParseColorString("color-mix(in srgb, currentcolor, color(--p 1))"); c.Type != ColorInvalid {
		t.Error("an unmixable color should make the mix invalid")
	}
	// two missing alphas render as zero
	if c := ParseColorString("color-mix(in srgb, rgb(255 0 0 / none), rgb(0 0 255 / none))"); c.RGBA.A != 0 {
		t.Errorf("missing alphas: %v", c.RGBA)
	}
	// a deferred color not built by NewDeferredColor is invalid, not a panic
	if got := (Color{Type: ColorDependsOnCurrent}).Resolve(cmyk); got.Type != ColorInvalid {
		t.Errorf("zero deferred color: %v", got)
	}
}
