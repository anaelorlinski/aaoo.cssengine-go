package color

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// testdata/spec_reference.json is generated from the CSS Color 4 sample
// code (conversions.js, hslToRgb.js, hwbToRgb.js, better-rgbToHsl.js) and
// the §13.2.2 gamut-mapping algorithm, for a grid of colors in every
// predefined space. A null target component is a missing (powerless) hue.
type specCase struct {
	Space      string
	Components []float64
	SRGBMapped [3]float64
	To         map[string][3]*float64
}

func loadSpecCases(t *testing.T) []specCase {
	t.Helper()
	b, err := os.ReadFile("testdata/spec_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []specCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func spaceByName(t *testing.T, name string) Space {
	t.Helper()
	for s := SRGB; s <= OKLCH; s++ {
		if s.String() == name {
			return s
		}
	}
	t.Fatalf("unknown space %q", name)
	return 0
}

func isHueIndex(s Space, i int) bool {
	switch s {
	case HSL, HWB:
		return i == 0
	case LCH, OKLCH:
		return i == 2
	}
	return false
}

// closeTo allows 1e-5 (relative above 1): the spec code and this package
// round differently (Go fuses multiply-adds on some platforms), and gamma
// exponents below 1 and the cube root of Oklab magnify the last bits of
// near-zero values. 1e-5 is still ~400 times finer than an 8-bit step.
func closeTo(got, want float64, isHue bool) bool {
	d := math.Abs(got - want)
	if isHue {
		d = math.Min(d, 360-d)
	}
	return d <= 1e-5*math.Max(1, math.Abs(want))
}

// achromatic reports whether the hue of a polar color is meaningless
// (negligible chroma or saturation), in which case rounding noise decides it.
func achromatic(s Space, c [3]*float64) bool {
	small := func(p *float64, limit float64) bool { return p == nil || math.Abs(*p) < limit }
	switch s {
	case HSL:
		return small(c[1], 1e-3)
	case HWB:
		return c[1] != nil && c[2] != nil && *c[1]+*c[2] >= 100-1e-3
	case LCH:
		return small(c[1], 1e-3)
	case OKLCH:
		return small(c[1], 1e-5)
	}
	return false
}

func TestConversionsMatchSpec(t *testing.T) {
	for _, c := range loadSpecCases(t) {
		v := New(spaceByName(t, c.Space), c.Components...)
		for targetName, want := range c.To {
			target := spaceByName(t, targetName)
			if target == v.Space {
				continue // To returns the value itself
			}
			got, ok := v.To(target)
			if !ok {
				t.Fatalf("%s%v -> %s: not converted", c.Space, c.Components, targetName)
			}
			for i, w := range want {
				hue := isHueIndex(target, i)
				if hue && (w == nil || got.IsMissing(i) || achromatic(target, want)) {
					continue // missing or meaningless hue
				}
				if target == HSL && i < 2 && want[2] != nil && (*want[2] < 1e-6 || *want[2] > 100-1e-6) {
					continue // white and black: saturation is 0/0, decided by rounding
				}
				if !closeTo(got.Components[i], *w, hue) {
					t.Errorf("%s%v -> %s[%d] = %v, spec %v", c.Space, c.Components, targetName, i, got.Components[i], *w)
				}
			}
		}
	}
}

func TestGamutMappingMatchesSpec(t *testing.T) {
	for _, c := range loadSpecCases(t) {
		v := New(spaceByName(t, c.Space), c.Components...)
		got, ok := v.SRGB()
		if !ok {
			t.Fatalf("%s%v: not rendered", c.Space, c.Components)
		}
		for i := range got {
			if !closeTo(got[i], c.SRGBMapped[i], false) {
				t.Errorf("%s%v -> sRGB = %v, spec %v", c.Space, c.Components, got, c.SRGBMapped)
				break
			}
		}
	}
}

func TestMissingComponentsReadAsZero(t *testing.T) {
	v := New(OKLCH, 0.7, 0.1, 123)
	v.Missing = 1 << 2 // hue: none
	got, _ := v.SRGB()
	want, _ := New(OKLCH, 0.7, 0.1, 0).SRGB()
	if got != want {
		t.Errorf("none hue: got %v, want %v", got, want)
	}
}

func TestDeviceCMYK(t *testing.T) {
	for _, tc := range []struct {
		cmyk [4]float64
		want [3]float64
	}{
		{[4]float64{0, 0, 0, 0}, [3]float64{1, 1, 1}},
		{[4]float64{0, 0, 0, 1}, [3]float64{0, 0, 0}},
		{[4]float64{1, 0, 0, 0}, [3]float64{0, 1, 1}},
		{[4]float64{0, 0.81, 0.81, 0.3}, [3]float64{0.7, 0.7 - 0.81*0.7, 0.7 - 0.81*0.7}},
	} {
		got, ok := New(DeviceCMYK, tc.cmyk[:]...).SRGB()
		if !ok {
			t.Fatal("device-cmyk not rendered")
		}
		for i := range got {
			if math.Abs(got[i]-tc.want[i]) > 1e-12 {
				t.Errorf("%v: got %v, want %v", tc.cmyk, got, tc.want)
				break
			}
		}
	}
	if _, ok := New(DeviceCMYK, 0, 0, 0, 0).To(SRGB); ok {
		t.Error("device-cmyk has no colorimetric conversion")
	}
}

func TestCustomProfileNotRendered(t *testing.T) {
	v := New(Custom, 0.1, 0.2, 0.3, 0.4)
	v.Profile = "--fogra39"
	if _, ok := v.SRGB(); ok {
		t.Error("custom profile rendered without the profile")
	}
}

func TestContrast(t *testing.T) {
	white, black := New(SRGB, 1, 1, 1), New(SRGB, 0, 0, 0)
	for _, tc := range []struct {
		bg   [3]float64
		want Value
	}{
		{[3]float64{0, 0, 0}, white},
		{[3]float64{0, 0, 0.5}, white},
		{[3]float64{1, 1, 1}, black},
		{[3]float64{1, 1, 0}, black},
		{[3]float64{128.0 / 255, 128.0 / 255, 128.0 / 255}, black},
	} {
		if got := Contrast(tc.bg); got != tc.want {
			t.Errorf("Contrast(%v) = %v, want %v", tc.bg, got.Components, tc.want.Components)
		}
	}
}

// Value must stay comparable: computed style values are stored in
// interfaces and compared with ==.
func TestValueComparable(t *testing.T) {
	var a, b any = New(Lab, 50, 20, 30), New(Lab, 50, 20, 30)
	if a != b {
		t.Error("equal values compare unequal")
	}
}
