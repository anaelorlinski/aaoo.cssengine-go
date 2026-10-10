package parser

import (
	"testing"
)

// --- gamut mapping: in-gamut passthrough ---

func TestGamutMapInGamutPassthrough(t *testing.T) {
	// In-gamut OKLCH should pass through unchanged (no chroma reduction).
	// oklch(0.5 0.1 200) is a muted blue-green, well within sRGB.
	c := ParseColorString("oklch(0.5 0.1 200)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	// Should match the direct conversion (no gamut mapping needed).
	assertRGBA(t, "oklch(0.5 0.1 200)", 0.0, 0.453, 0.477, 1.0)
}

// --- gamut mapping: chroma reduction ---

func TestGamutMapChromaReduction(t *testing.T) {
	// oklch(0.7 0.3 30) — vivid orange, well out of sRGB gamut.
	// Gamut mapping should reduce chroma while keeping L=0.7 and H=30.
	c := ParseColorString("oklch(0.7 0.3 30)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}

	// Result must be in [0,1] for all channels.
	if c.RGBA.R < 0 || c.RGBA.R > 1 {
		t.Errorf("R=%.4f out of [0,1]", c.RGBA.R)
	}
	if c.RGBA.G < 0 || c.RGBA.G > 1 {
		t.Errorf("G=%.4f out of [0,1]", c.RGBA.G)
	}
	if c.RGBA.B < 0 || c.RGBA.B > 1 {
		t.Errorf("B=%.4f out of [0,1]", c.RGBA.B)
	}

	// Should still be orange-ish: R > G > B.
	if c.RGBA.R <= c.RGBA.G {
		t.Errorf("expected R > G for orange, got R=%.4f G=%.4f", c.RGBA.R, c.RGBA.G)
	}
	if c.RGBA.G <= c.RGBA.B {
		t.Errorf("expected G > B for orange, got G=%.4f B=%.4f", c.RGBA.G, c.RGBA.B)
	}

	// Compare with a less saturated version that's in-gamut:
	// oklch(0.7 0.15 30) should be closer to the mapped result than the original.
	c2 := ParseColorString("oklch(0.7 0.15 30)")
	if !approxEqual(c.RGBA.R, c2.RGBA.R, 0.1) {
		t.Errorf("mapped R=%.4f should be close to in-gamut R=%.4f", c.RGBA.R, c2.RGBA.R)
	}
}

func TestGamutMapExtremeChroma(t *testing.T) {
	// oklch(0.5 0.4 0) — extreme chroma, magenta direction.
	// Should be gamut-mapped to something in [0,1].
	c := ParseColorString("oklch(0.5 0.4 0)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	if c.RGBA.R < 0 || c.RGBA.R > 1 {
		t.Errorf("R=%.4f out of [0,1]", c.RGBA.R)
	}
	if c.RGBA.G < 0 || c.RGBA.G > 1 {
		t.Errorf("G=%.4f out of [0,1]", c.RGBA.G)
	}
	if c.RGBA.B < 0 || c.RGBA.B > 1 {
		t.Errorf("B=%.4f out of [0,1]", c.RGBA.B)
	}
}

// --- gamut mapping: achromatic ---

func TestGamutMapAchromatic(t *testing.T) {
	// C=0: no chroma, no gamut mapping needed. Should produce gray.
	assertRGBA(t, "oklch(0.5 0 0)", 0.389, 0.389, 0.389, 1.0)
	assertRGBA(t, "oklch(0.5 0 180)", 0.389, 0.389, 0.389, 1.0)
}

// --- gamut mapping: L=0 (black) ---

func TestGamutMapBlackLightness(t *testing.T) {
	// At L=0, any chroma is out of gamut. Gamut mapping reduces chroma to 0 → black.
	assertRGBA(t, "oklch(0 0.2 40)", 0.0, 0.0, 0.0, 1.0)
	assertRGBA(t, "oklch(0 0.4 120)", 0.0, 0.0, 0.0, 1.0)
}

// --- gamut mapping: L=1 (white) ---

func TestGamutMapWhiteLightness(t *testing.T) {
	// At L=1, any chroma is out of gamut. Gamut mapping reduces chroma to 0 → white.
	c := ParseColorString("oklch(1 0.2 40)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	// Should be white or very close to it.
	if c.RGBA.R < 0.95 {
		t.Errorf("L=1 with chroma should map to near-white, R=%.4f", c.RGBA.R)
	}
	if c.RGBA.G < 0.95 {
		t.Errorf("L=1 with chroma should map to near-white, G=%.4f", c.RGBA.G)
	}
	if c.RGBA.B < 0.95 {
		t.Errorf("L=1 with chroma should map to near-white, B=%.4f", c.RGBA.B)
	}
}

// --- gamut mapping: alpha preserved ---

func TestGamutMapAlphaPreserved(t *testing.T) {
	// Alpha should be preserved through gamut mapping.
	c := ParseColorString("oklch(0.7 0.3 30 / 0.5)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	if !approxEqual(c.RGBA.A, 0.5, 0.001) {
		t.Errorf("alpha not preserved: A=%.4f, want 0.5", c.RGBA.A)
	}
}

// --- gamut mapping: OKLAB ---

func TestGamutMapOKLAB(t *testing.T) {
	// oklab with extreme a/b should be gamut-mapped.
	c := ParseColorString("oklab(0.7 0.3 0)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	if c.RGBA.R < 0 || c.RGBA.R > 1 {
		t.Errorf("R=%.4f out of [0,1]", c.RGBA.R)
	}
	if c.RGBA.G < 0 || c.RGBA.G > 1 {
		t.Errorf("G=%.4f out of [0,1]", c.RGBA.G)
	}
	if c.RGBA.B < 0 || c.RGBA.B > 1 {
		t.Errorf("B=%.4f out of [0,1]", c.RGBA.B)
	}
}

// --- gamut mapping: LAB ---

func TestGamutMapLAB(t *testing.T) {
	// lab(50 100 100) — extreme chroma, should be gamut-mapped.
	c := ParseColorString("lab(50 100 100)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	if c.RGBA.R < 0 || c.RGBA.R > 1 {
		t.Errorf("R=%.4f out of [0,1]", c.RGBA.R)
	}
	if c.RGBA.G < 0 || c.RGBA.G > 1 {
		t.Errorf("G=%.4f out of [0,1]", c.RGBA.G)
	}
	if c.RGBA.B < 0 || c.RGBA.B > 1 {
		t.Errorf("B=%.4f out of [0,1]", c.RGBA.B)
	}
}

// --- gamut mapping: LCH ---

func TestGamutMapLCH(t *testing.T) {
	// lch(50 150 30) — extreme chroma, should be gamut-mapped.
	c := ParseColorString("lch(50 150 30)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	if c.RGBA.R < 0 || c.RGBA.R > 1 {
		t.Errorf("R=%.4f out of [0,1]", c.RGBA.R)
	}
	if c.RGBA.G < 0 || c.RGBA.G > 1 {
		t.Errorf("G=%.4f out of [0,1]", c.RGBA.G)
	}
	if c.RGBA.B < 0 || c.RGBA.B > 1 {
		t.Errorf("B=%.4f out of [0,1]", c.RGBA.B)
	}
}

// --- gamut mapping: hue preserved ---

func TestGamutMapHuePreserved(t *testing.T) {
	// Gamut mapping preserves hue, so two colors with the same hue
	// but different (out-of-gamut) chroma should map to similar hues.
	c1 := ParseColorString("oklch(0.6 0.3 60)")
	c2 := ParseColorString("oklch(0.6 0.2 60)")
	if c1.IsNone() || c2.IsNone() {
		t.Fatal("expected valid colors")
	}
	// Both should be yellow-ish (hue 60 = yellow).
	// Check that R and G are the dominant channels.
	if c1.RGBA.B > c1.RGBA.R || c1.RGBA.B > c1.RGBA.G {
		t.Errorf("hue 60 should have B as smallest channel: R=%.4f G=%.4f B=%.4f",
			c1.RGBA.R, c1.RGBA.G, c1.RGBA.B)
	}
	if c2.RGBA.B > c2.RGBA.R || c2.RGBA.B > c2.RGBA.G {
		t.Errorf("hue 60 should have B as smallest channel: R=%.4f G=%.4f B=%.4f",
			c2.RGBA.R, c2.RGBA.G, c2.RGBA.B)
	}
}

// --- gamut mapping: lightness preserved ---

func TestGamutMapLightnessPreserved(t *testing.T) {
	// Gamut mapping preserves L, so the mapped color should have
	// similar relative luminance to a gray of the same L.
	// oklch(0.5 0.3 0) maps to some color; oklch(0.5 0 0) is gray.
	gray := ParseColorString("oklch(0.5 0 0)")
	if gray.IsNone() {
		t.Fatal("expected valid gray")
	}
	// The mapped vivid color should not be dramatically darker or lighter
	// than the gray at the same L.
	c := ParseColorString("oklch(0.5 0.3 0)")
	if c.IsNone() {
		t.Fatal("expected valid color")
	}
	// Average of RGB channels should be roughly similar (within 0.15).
	avgGray := (float64(gray.RGBA.R) + float64(gray.RGBA.G) + float64(gray.RGBA.B)) / 3
	avgMapped := (float64(c.RGBA.R) + float64(c.RGBA.G) + float64(c.RGBA.B)) / 3
	if avgMapped < avgGray-0.15 || avgMapped > avgGray+0.15 {
		t.Errorf("lightness not preserved: gray avg=%.4f, mapped avg=%.4f", avgGray, avgMapped)
	}
}

// --- gamut mapping: all channels in [0,1] ---

func TestGamutMapAllInGamut(t *testing.T) {
	// Test a range of out-of-gamut colors and verify all map to [0,1].
	inputs := []string{
		"oklch(0.8 0.3 0)",
		"oklch(0.8 0.3 60)",
		"oklch(0.8 0.3 120)",
		"oklch(0.8 0.3 180)",
		"oklch(0.8 0.3 240)",
		"oklch(0.8 0.3 300)",
		"oklch(0.3 0.3 0)",
		"oklch(0.3 0.3 90)",
		"oklch(0.3 0.3 180)",
		"oklch(0.3 0.3 270)",
		"oklab(0.8 0.3 0.1)",
		"oklab(0.3 -0.3 -0.1)",
		"lab(80 100 -50)",
		"lab(20 -80 80)",
		"lch(70 150 0)",
		"lch(30 150 180)",
	}
	for _, input := range inputs {
		c := ParseColorString(input)
		if c.IsNone() {
			t.Errorf("%s: expected valid color", input)
			continue
		}
		if c.RGBA.R < 0 || c.RGBA.R > 1 {
			t.Errorf("%s: R=%.4f out of [0,1]", input, c.RGBA.R)
		}
		if c.RGBA.G < 0 || c.RGBA.G > 1 {
			t.Errorf("%s: G=%.4f out of [0,1]", input, c.RGBA.G)
		}
		if c.RGBA.B < 0 || c.RGBA.B > 1 {
			t.Errorf("%s: B=%.4f out of [0,1]", input, c.RGBA.B)
		}
	}
}
