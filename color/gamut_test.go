package color

import (
	"math"
	"testing"
)

// The reference values are produced by color.js's toGamutCSS ("Binary
// Search Gamut Mapping with Local MINDE"), maintained by the CSS Color spec
// editors. They pin the exact output so that a regression to plain clipping
// or to pure chroma reduction is caught.
func TestGamutMapOKLCHColorJSValues(t *testing.T) {
	const tol = 0.005
	for _, tc := range []struct {
		l, c, h    float64
		wR, wG, wB float64
	}{
		{0.10, 0.05, 240, 0.0000, 0.0134, 0.0696}, // near-black blue
		{0.70, 0.40, 30, 1.0000, 0.3451, 0.2646},  // vivid red-orange
		{0.50, 0.30, 150, 0.0000, 0.4848, 0.1461}, // vivid green
		{0.60, 0.37, 264, 0.1922, 0.4363, 1.0000}, // vivid blue
		{0.85, 0.20, 90, 0.9975, 0.7743, 0.0000},  // light yellow-green
	} {
		got, _ := New(OKLCH, tc.l, tc.c, tc.h).SRGB()
		if math.Abs(got[0]-tc.wR) > tol || math.Abs(got[1]-tc.wG) > tol || math.Abs(got[2]-tc.wB) > tol {
			t.Errorf("oklch(%.2f %.2f %g) -> %.4f, want (%.4f %.4f %.4f)",
				tc.l, tc.c, tc.h, got, tc.wR, tc.wG, tc.wB)
		}
	}
}

// CSS Lab uses the D50 white point: the Lab coordinates of the sRGB
// primaries, as reported by browsers' getComputedStyle.
func TestSRGBPrimariesInLab(t *testing.T) {
	const tol = 0.01
	for _, tc := range []struct {
		rgb [3]float64
		lab [3]float64
	}{
		{[3]float64{1, 0, 0}, [3]float64{54.2905, 80.8049, 69.8910}},
		{[3]float64{0, 1, 0}, [3]float64{87.8185, -79.2711, 80.9946}},
		{[3]float64{0, 0, 1}, [3]float64{29.5683, 68.2874, -112.0297}},
		{[3]float64{1, 1, 1}, [3]float64{100, 0, 0}},
	} {
		got, _ := New(SRGB, tc.rgb[:]...).To(Lab)
		for i := range tc.lab {
			if math.Abs(got.Components[i]-tc.lab[i]) > tol {
				t.Errorf("srgb%v -> lab = %v, want %v", tc.rgb, got.Components[:3], tc.lab)
				break
			}
		}
	}
}

// Conversions to and back from Lab are exact inverses (machine precision).
func TestLabRoundTrip(t *testing.T) {
	for _, rgb := range [][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {0.5, 0.3, 0.7}, {0.2, 0.8, 0.4}, {1, 1, 1}} {
		lab, _ := New(SRGB, rgb[:]...).To(Lab)
		back, _ := lab.To(SRGB)
		for i := range rgb {
			if math.Abs(back.Components[i]-rgb[i]) > 1e-9 {
				t.Errorf("round trip of %v: got %v", rgb, back.Components[:3])
				break
			}
		}
	}
}
