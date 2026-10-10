package parser

import (
	"math"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/color"
)

// approxEqual checks if two float32 values are within tolerance.
func approxEqual(a, b, tol float32) bool {
	return float32(math.Abs(float64(a-b))) <= tol
}

// assertRGBA checks that each channel of the parsed color matches the
// expected values within tolerance. Alpha is compared with a tighter
// tolerance since it's not subject to color-space conversion rounding.
func assertRGBA(t *testing.T, input string, expR, expG, expB, expA float32) {
	t.Helper()
	c := ParseColorString(input)
	if c.IsNone() {
		t.Fatalf("%s: expected valid color, got invalid", input)
	}
	if c.Type != ColorRGBA {
		t.Fatalf("%s: expected ColorRGBA, got type %d", input, c.Type)
	}
	tol := float32(0.02) // ~5/255, generous for float32 color-space round-trips
	if !approxEqual(c.RGBA.R, expR, tol) {
		t.Errorf("%s: R=%.4f, want %.4f", input, c.RGBA.R, expR)
	}
	if !approxEqual(c.RGBA.G, expG, tol) {
		t.Errorf("%s: G=%.4f, want %.4f", input, c.RGBA.G, expG)
	}
	if !approxEqual(c.RGBA.B, expB, tol) {
		t.Errorf("%s: B=%.4f, want %.4f", input, c.RGBA.B, expB)
	}
	if !approxEqual(c.RGBA.A, expA, 0.001) {
		t.Errorf("%s: A=%.4f, want %.4f", input, c.RGBA.A, expA)
	}
}

// assertInvalid checks that the input does not parse as a valid color.
func assertInvalid(t *testing.T, input string) {
	t.Helper()
	c := ParseColorString(input)
	if !c.IsNone() {
		t.Errorf("%s: expected invalid color, got R=%.4f G=%.4f B=%.4f A=%.4f", input, c.RGBA.R, c.RGBA.G, c.RGBA.B, c.RGBA.A)
	}
}

// --- oklch ---

func TestOKLCH(t *testing.T) {
	// oklch(0.7 0.2 40) → a vivid orange, in sRGB gamut.
	assertRGBA(t, "oklch(0.7 0.2 40)", 1.0, 0.403, 0.158, 1.0)

	// oklch(0.5 0.1 200) → a muted blue-green.
	assertRGBA(t, "oklch(0.5 0.1 200)", 0.0, 0.453, 0.477, 1.0)

	// oklch(1 0 0) → white (no chroma, full lightness).
	assertRGBA(t, "oklch(1 0 0)", 1.0, 1.0, 1.0, 1.0)

	// oklch(0 0 0) → black.
	assertRGBA(t, "oklch(0 0 0)", 0.0, 0.0, 0.0, 1.0)

	// oklch(0.7 0.2 40 / 0.5) → same color with 50% alpha.
	assertRGBA(t, "oklch(0.7 0.2 40 / 0.5)", 1.0, 0.403, 0.158, 0.5)

	// oklch with percentages: 70% L, 50% C (→ 0.2), 40deg H.
	assertRGBA(t, "oklch(70% 50% 40)", 1.0, 0.403, 0.158, 1.0)

	// oklch with none keyword for hue → hue is 0 (magenta-ish).
	assertRGBA(t, "oklch(0.7 0.2 none)", 0.986, 0.362, 0.601, 1.0)

	// oklch with none for L → L=0, chroma reduced to 0 by gamut mapping (black).
	assertRGBA(t, "oklch(none 0.2 40)", 0.0, 0.0, 0.0, 1.0)

	// Angle units: degrees, gradians, turns are equivalent.
	// 90deg = 0.25turn = 100grad
	// oklch(0.7 0.2 90) is out of sRGB gamut (B<0); chroma reduced from 0.2 to ~0.143.
	assertRGBA(t, "oklch(0.7 0.2 90deg)", 0.756, 0.600, 0.0, 1.0)
	assertRGBA(t, "oklch(0.7 0.2 0.25turn)", 0.756, 0.600, 0.0, 1.0)
	assertRGBA(t, "oklch(0.7 0.2 100grad)", 0.756, 0.600, 0.0, 1.0)

	// Invalid: wrong number of channels.
	assertInvalid(t, "oklch(0.7 0.2)")
	assertInvalid(t, "oklch(0.7 0.2 40 10)")

	// Invalid: non-numeric channel.
	assertInvalid(t, "oklch(red 0.2 40)")
}

func TestOKLCHGamutMapping(t *testing.T) {
	// oklch(0.7 0.25 30) is slightly out of sRGB gamut (vivid orange).
	// Gamut mapping should reduce chroma while preserving hue and lightness.
	// The result should still be orange-ish (high R, moderate G, lower B).
	c := ParseColorString("oklch(0.7 0.25 30)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	// R should be high (orange).
	if c.RGBA.R < 0.9 {
		t.Errorf("gamut-mapped vivid orange: R=%.4f, expected >= 0.9", c.RGBA.R)
	}
	// G should be between B and R (orange, not red or yellow).
	if c.RGBA.G <= c.RGBA.B {
		t.Errorf("gamut-mapped vivid orange: G=%.4f should be > B=%.4f", c.RGBA.G, c.RGBA.B)
	}
	// Should be in gamut (all channels in [0,1]).
	if c.RGBA.R > 1.0 || c.RGBA.G > 1.0 || c.RGBA.B > 1.0 {
		t.Errorf("gamut-mapped color should be in [0,1]: R=%.4f G=%.4f B=%.4f", c.RGBA.R, c.RGBA.G, c.RGBA.B)
	}
}

// --- oklab ---

func TestOKLab(t *testing.T) {
	// oklab(0.5 0 0) → perceptual mid-gray (~rgb(99,99,99)).
	// OKLab L=0.5 is perceptually 50% gray, which maps to sRGB ~0.389.
	assertRGBA(t, "oklab(0.5 0 0)", 0.389, 0.389, 0.389, 1.0)

	// oklab(1 0 0) → white.
	assertRGBA(t, "oklab(1 0 0)", 1.0, 1.0, 1.0, 1.0)

	// oklab(0 0 0) → black.
	assertRGBA(t, "oklab(0 0 0)", 0.0, 0.0, 0.0, 1.0)

	// oklab with alpha.
	assertRGBA(t, "oklab(0.5 0.1 0.05 / 0.5)", 0.599, 0.276, 0.250, 0.5)

	// oklab with percentages: 50% L, 25% a (→ 0.1), 12.5% b (→ 0.05).
	assertRGBA(t, "oklab(50% 25% 12.5%)", 0.599, 0.276, 0.250, 1.0)

	// none keyword for L → L=0, chroma reduced to 0 by gamut mapping (black).
	assertRGBA(t, "oklab(none 0.1 0.05)", 0.0, 0.0, 0.0, 1.0)

	// Invalid.
	assertInvalid(t, "oklab(0.5 0.1)")
	assertInvalid(t, "oklab(0.5 0.1 0.05 0.1)")
}

// --- lab ---

func TestLab(t *testing.T) {
	// lab(100 0 0) → white (L=100, no a/b).
	assertRGBA(t, "lab(100 0 0)", 1.0, 1.0, 1.0, 1.0)

	// lab(0 0 0) → black.
	assertRGBA(t, "lab(0 0 0)", 0.0, 0.0, 0.0, 1.0)

	// lab(50 0 0) → mid gray.
	assertRGBA(t, "lab(50 0 0)", 0.4663, 0.4663, 0.4663, 1.0)

	// lab with alpha. Values follow the CSS Color 4 chain: Lab (D50) → XYZ →
	// Bradford adapt to D65 → sRGB.
	assertRGBA(t, "lab(50 40 -30 / 0.5)", 0.6481, 0.3579, 0.671, 0.5)

	// lab with percentages: 50% L (→ 50), 32% a (→ 40), -24% b (→ -30).
	assertRGBA(t, "lab(50% 32% -24%)", 0.6481, 0.3579, 0.671, 1.0)

	// none keyword for L → L=0. lab(0 40 -30) has zero luminance (Y=0) but
	// non-zero X and Z, so it is a very dark purple, not black. Per CSS
	// Color 4 the color is gamut-mapped in OKLCH (MINDE), which keeps a
	// trace of chroma rather than collapsing to black.
	assertRGBA(t, "lab(none 40 -30)", 0.0814, 0.0, 0.0824, 1.0)

	// Invalid.
	assertInvalid(t, "lab(50 40)")
	assertInvalid(t, "lab(50 40 -30 10)")
}

// --- lch ---

func TestLCH(t *testing.T) {
	// lch(100 0 0) → white.
	assertRGBA(t, "lch(100 0 0)", 1.0, 1.0, 1.0, 1.0)

	// lch(0 0 0) → black.
	assertRGBA(t, "lch(0 0 0)", 0.0, 0.0, 0.0, 1.0)

	// lch(50 40 30) → orange-ish.
	assertRGBA(t, "lch(50 40 30)", 0.6987, 0.3662, 0.3414, 1.0)

	// lch with alpha.
	assertRGBA(t, "lch(50 40 30 / 0.5)", 0.6987, 0.3662, 0.3414, 0.5)

	// lch with angle unit.
	assertRGBA(t, "lch(50 40 30deg)", 0.6987, 0.3662, 0.3414, 1.0)

	// lch with percentages: 50% L (→ 50), 26.67% C (→ 40), 30deg H.
	assertRGBA(t, "lch(50% 26.67% 30)", 0.6987, 0.3662, 0.3414, 1.0)

	// none keyword for L → L=0. lch(0 40 30) resolves to zero luminance and,
	// with the correct D50 white point, to zero XYZ overall — i.e. black.
	assertRGBA(t, "lch(none 40 30)", 0.0, 0.0, 0.0, 1.0)

	// Invalid.
	assertInvalid(t, "lch(50 40)")
	assertInvalid(t, "lch(50 40 30 10)")
}

// --- hwb ---

func TestHWB(t *testing.T) {
	// hwb(0 0% 0%) → pure red.
	assertRGBA(t, "hwb(0 0% 0%)", 1.0, 0.0, 0.0, 1.0)

	// hwb(120 0% 0%) → pure green.
	assertRGBA(t, "hwb(120 0% 0%)", 0.0, 1.0, 0.0, 1.0)

	// hwb(240 0% 0%) → pure blue.
	assertRGBA(t, "hwb(240 0% 0%)", 0.0, 0.0, 1.0, 1.0)

	// hwb(0 100% 0%) → white.
	assertRGBA(t, "hwb(0 100% 0%)", 1.0, 1.0, 1.0, 1.0)

	// hwb(0 0% 100%) → black.
	assertRGBA(t, "hwb(0 0% 100%)", 0.0, 0.0, 0.0, 1.0)

	// hwb(0 20% 20%) → a muted red.
	// Pure red (1,0,0) mixed with 20% white and 20% black:
	// factor = 1 - 0.2 - 0.2 = 0.6; R = 1*0.6 + 0.2 = 0.8; G=B=0*0.6+0.2=0.2.
	assertRGBA(t, "hwb(0 20% 20%)", 0.8, 0.2, 0.2, 1.0)

	// hwb with alpha.
	assertRGBA(t, "hwb(0 20% 20% / 0.5)", 0.8, 0.2, 0.2, 0.5)

	// hwb with angle unit.
	assertRGBA(t, "hwb(120deg 0% 0%)", 0.0, 1.0, 0.0, 1.0)

	// hwb with none keyword (hue=0 → red).
	assertRGBA(t, "hwb(none 0% 0%)", 1.0, 0.0, 0.0, 1.0)

	// W+B > 1: normalize. hwb(0 80% 80%) → W/(W+B)=0.5, B/(W+B)=0.5.
	// R = 1*(1-0.5) + 0.5 = 1.0; G=B = 0*(1-0.5)+0.5 = 0.5.
	// Library normalizes to gray (0.5, 0.5, 0.5).
	assertRGBA(t, "hwb(0 80% 80%)", 0.5, 0.5, 0.5, 1.0)

	// Invalid.
	assertInvalid(t, "hwb(0 20%)")
	assertInvalid(t, "hwb(0 20% 20% 10%)")
}

// --- case insensitivity ---

func TestColorL4CaseInsensitive(t *testing.T) {
	// Function names should be case-insensitive.
	assertRGBA(t, "OKLCH(0.7 0.2 40)", 1.0, 0.403, 0.158, 1.0)
	assertRGBA(t, "Oklch(0.7 0.2 40)", 1.0, 0.403, 0.158, 1.0)
	assertRGBA(t, "OKLAB(0.5 0 0)", 0.389, 0.389, 0.389, 1.0)
	assertRGBA(t, "LAB(50 0 0)", 0.466, 0.466, 0.466, 1.0)
	assertRGBA(t, "LCH(100 0 0)", 1.0, 1.0, 1.0, 1.0)
	assertRGBA(t, "HWB(0 0% 0%)", 1.0, 0.0, 0.0, 1.0)

	// "none" keyword should be case-insensitive.
	assertRGBA(t, "oklch(0.7 0.2 NONE)", 0.986, 0.362, 0.601, 1.0)
}

// --- regression: existing colors still work ---

func TestColorL4Regression(t *testing.T) {
	// Named colors.
	assertRGBA(t, "red", 1.0, 0.0, 0.0, 1.0)
	assertRGBA(t, "blue", 0.0, 0.0, 1.0, 1.0)
	assertRGBA(t, "transparent", 0.0, 0.0, 0.0, 0.0)

	// Hex.
	assertRGBA(t, "#ff0000", 1.0, 0.0, 0.0, 1.0)
	assertRGBA(t, "#f00", 1.0, 0.0, 0.0, 1.0)

	// RGB.
	assertRGBA(t, "rgb(255, 0, 0)", 1.0, 0.0, 0.0, 1.0)
	assertRGBA(t, "rgb(100% 0% 0%)", 1.0, 0.0, 0.0, 1.0)

	// HSL.
	assertRGBA(t, "hsl(0, 100%, 50%)", 1.0, 0.0, 0.0, 1.0)
	assertRGBA(t, "hsl(120, 100%, 50%)", 0.0, 1.0, 0.0, 1.0)

	// currentcolor.
	c := ParseColorString("currentcolor")
	if c.Type != ColorCurrentColor {
		t.Errorf("currentcolor: expected type %d, got %d", ColorCurrentColor, c.Type)
	}
}

// PC3–PC7: the legacy (comma) and modern syntaxes of CSS Color 4 and 5.
func TestColorSyntaxes(t *testing.T) {
	for input, want := range map[string]*RGBA{
		// PC3: the modern syntax mixes numbers and percentages
		"rgb(100% 0 0)":    {R: 1, A: 1},
		"rgb(255 50% 0)":   {R: 1, G: 0.5, A: 1},
		"rgb(255, 50%, 0)": nil, // not the legacy one
		// PC4: rgb() channels clamp at parsed-value time
		"rgb(300 0 0)":    {R: 1, A: 1},
		"rgb(-50 255 0)":  {G: 1, A: 1},
		"rgb(300, 0, -5)": {R: 1, A: 1},
		"rgb(150% 0% 0%)": {R: 1, A: 1},
		// PC5: none as alpha, outside the legacy syntax
		"rgb(0 0 0 / none)":   {},
		"hsl(0 0% 0% / none)": {},
		"rgba(0, 0, 0, none)": nil,
		// PC6: the legacy syntax has no none, needs hsl percentages, and
		// does not exist for hwb
		"rgb(none, 0, 0)":    nil,
		"hsl(120, 50, 50)":   nil,
		"hwb(120, 0%, 0%)":   nil,
		"hsl(120 50 50)":     {R: 0.25, G: 0.75, B: 0.25, A: 1},
		"hsl(120, 50%, 50%)": {R: 0.25, G: 0.75, B: 0.25, A: 1},
		// PC7: light-dark(<color>, <color>)
		"light-dark(red, blue)":      {R: 1, A: 1},
		"light-dark(red blue)":       nil,
		"light-dark(red, foo, blue)": nil,
		"light-dark(red, foo)":       nil,
	} {
		c := ParseColorString(input)
		if want == nil {
			if c.Type != ColorInvalid {
				t.Errorf("%s: valid %v, want invalid", input, c.RGBA)
			}
			continue
		}
		if c.Type != ColorRGBA {
			t.Errorf("%s: invalid", input)
			continue
		}
		got := c.RGBA
		if d := math.Abs(float64(got.R-want.R)) + math.Abs(float64(got.G-want.G)) + math.Abs(float64(got.B-want.B)) + math.Abs(float64(got.A-want.A)); d > 0.01 {
			t.Errorf("%s: %v, want %v", input, got, *want)
		}
	}
	// the missing alpha is recorded
	if v := ParseColorString("rgb(0 0 0 / none)").Value; !v.IsMissing(color.AlphaMissing) || v.Alpha != 0 {
		t.Errorf("alpha none: %+v", v)
	}
	// PC7: color(--profile …) takes no commas and at least one component
	// (ParseColorValue: rendering a profile color needs its @color-profile)
	profile := func(input string) ColorType {
		_, typ := ParseColorValue(ParseOneComponentValue(Tokenize([]byte(input), true)), ColorSchemeLight)
		return typ
	}
	for _, input := range []string{"color(--p 1, 2, 3)", "color(--p)"} {
		if profile(input) != ColorInvalid {
			t.Errorf("%s: valid", input)
		}
	}
	if profile("color(--p 1 2 3)") != ColorRGBA || profile("color(--p 1 2 3 / none)") != ColorRGBA {
		t.Errorf("color(--p 1 2 3): invalid")
	}
}

// P2-3 to P2-5: Color 5 grammars.
func TestColor5Grammar(t *testing.T) {
	valid := func(input string) bool {
		_, typ := ParseColorValue(ParseOneComponentValue(Tokenize([]byte(input), true)), ColorSchemeLight)
		return typ != ColorInvalid
	}
	for input, want := range map[string]bool{
		"color(--p 1)": true, "color(--p none)": true, "color(--p 100%)": true, "color(--p 1 / .5)": true,
		"color(--p 1 2 /)": false, "color(--p 1 2 3 /)": false, "device-cmyk(0 0 0 0 /)": false,
		"device-cmyk(0, 0, 0, none)": false, "device-cmyk(none, 0, 0, 0)": false,
		"device-cmyk(0 0 0 none)": true, "device-cmyk(0, 0, 0, 1)": true,
	} {
		if got := valid(input); got != want {
			t.Errorf("%s: valid %v, want %v", input, got, want)
		}
	}
}

// color-mix() (CSS Color 5 §2), against the spec examples.
func TestColorMix(t *testing.T) {
	for input, want := range map[string]RGBA{
		"color-mix(in srgb, red, blue)":             {R: 0.5, B: 0.5, A: 1},
		"color-mix(in srgb, red 25%, blue)":         {R: 0.25, B: 0.75, A: 1},
		"color-mix(in srgb, red, blue 25%)":         {R: 0.75, B: 0.25, A: 1},
		"color-mix(in srgb, 25% red, blue)":         {R: 0.25, B: 0.75, A: 1},
		"color-mix(in srgb, red 20%, blue 20%)":     {R: 0.5, B: 0.5, A: 0.4}, // sum < 100: alpha
		"color-mix(in srgb, red 50%, transparent)":  {R: 1, A: 0.5},           // premultiplied
		"color-mix(in oklab, red 50%, transparent)": {R: 1, A: 0.5},
		"color-mix(in srgb, red 150%, blue)":        {},
		"color-mix(in srgb, red 0%, blue 0%)":       {},
		"color-mix(in hsl longer hue, red, red)":    {R: 0, G: 1, B: 1, A: 1}, // the other way round the wheel
		"color-mix(in srgb longer hue, red, blue)":  {},                       // not a polar space
		"color-mix(in srgb, red, blue, green)":      {},
		"color-mix(srgb, red, blue)":                {},
	} {
		c := ParseColorString(input)
		if want == (RGBA{}) {
			if c.Type != ColorInvalid {
				t.Errorf("%s: valid %v", input, c.RGBA)
			}
			continue
		}
		if c.Type != ColorRGBA {
			t.Errorf("%s: invalid", input)
			continue
		}
		got := c.RGBA
		if d := math.Abs(float64(got.R-want.R)) + math.Abs(float64(got.G-want.G)) + math.Abs(float64(got.B-want.B)) + math.Abs(float64(got.A-want.A)); d > 0.03 {
			t.Errorf("%s: %v, want %v", input, got, want)
		}
	}
	// hue interpolation: hsl(200deg 50% 80%) is at about 240° in LCH,
	// coral at 42°: the shorter arc goes through 320°, the longer one
	// through 141°
	hueOf := func(input string) float64 {
		v, _ := ParseColorValue(ParseOneComponentValue(Tokenize([]byte(input), true)), ColorSchemeLight)
		return v.Components[2]
	}
	if h := hueOf("color-mix(in lch longer hue, hsl(200deg 50% 80%), coral)"); h < 130 || h > 150 {
		t.Errorf("longer hue: %g", h)
	}
	if h := hueOf("color-mix(in lch, hsl(200deg 50% 80%), coral)"); h < 310 || h > 330 {
		t.Errorf("shorter hue: %g", h)
	}
	// the worked examples of CSS Color 5 §2.1
	for input, want := range map[string]string{
		"color-mix(in lch, peru 40%, palegoldenrod)": "lch(79.7256 40.448 84.771)",
	} {
		got, ref := ParseColorString(input).RGBA, ParseColorString(want).RGBA
		if d := math.Abs(float64(got.R-ref.R)) + math.Abs(float64(got.G-ref.G)) + math.Abs(float64(got.B-ref.B)); d > 0.01 {
			t.Errorf("%s: %v, want %v (%s)", input, got, ref, want)
		}
	}
}
