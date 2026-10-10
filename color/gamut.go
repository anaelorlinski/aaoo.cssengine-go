package color

import "math"

// SRGB renders v in sRGB, with components in [0, 1].
//
// Colors outside the sRGB gamut are mapped with the CSS gamut-mapping
// algorithm (CSS Color 4 §13.2.2, "binary search gamut mapping with local
// MINDE"): chroma is reduced in Oklch until the clipped color is within a
// just-noticeable difference of the reduced one.
//
// device-cmyk() uses the naive conversion of CSS Color 5 §14.2. It reports
// false for a [Custom] space, which needs its color profile.
func (v Value) SRGB() (rgb [3]float64, ok bool) {
	switch {
	case v.Space == DeviceCMYK:
		return cmykToSRGB(v), true
	case !v.Space.Convertible():
		return rgb, false
	}
	// sRGB-based spaces render without the XYZ round trip, whose rounding
	// would push in-gamut colors like rgb(255 0 0) slightly out of gamut.
	switch v.Space {
	case SRGB:
		rgb = v.components3()
	case HSL:
		rgb = hslToSRGB(v.components3())
	case HWB:
		rgb = hwbToSRGB(v.components3())
	}
	if (v.Space == SRGB || v.Space == HSL || v.Space == HWB) && inSRGBGamut(rgb) {
		return rgb, true
	}
	xyz, _ := v.toXYZD65()
	// the Oklab spaces give the OKLCH coordinates of the mapping without
	// the XYZ round trip, which loses the lightness of huge chromas
	switch v.Space {
	case OKLCH:
		lch := v.components3()
		return gamutMapSRGBFrom(xyz, &lch), true
	case OKLab:
		lch, _ := rectangularToPolar(v.components3(), 0)
		return gamutMapSRGBFrom(xyz, &lch), true
	}
	return gamutMapSRGB(xyz), true
}

func cmykToSRGB(v Value) [3]float64 {
	c, m, y, k := v.Component(0), v.Component(1), v.Component(2), v.Component(3)
	return [3]float64{
		1 - math.Min(1, c*(1-k)+k),
		1 - math.Min(1, m*(1-k)+k),
		1 - math.Min(1, y*(1-k)+k),
	}
}

func inSRGBGamut(rgb [3]float64) bool {
	for _, c := range rgb {
		// written so that NaN is out of gamut
		if !(c >= 0 && c <= 1) {
			return false
		}
	}
	return true
}

func clipSRGB(rgb [3]float64) [3]float64 {
	for i, c := range rgb {
		if !(c > 0) { // also maps NaN to 0
			c = 0
		}
		rgb[i] = math.Min(1, c)
	}
	return rgb
}

// deltaEOK is the color difference of CSS Color 4 §13.1.1: the euclidean
// distance in Oklab.
func deltaEOK(lab1, lab2 [3]float64) float64 {
	return math.Hypot(lab1[0]-lab2[0], math.Hypot(lab1[1]-lab2[1], lab1[2]-lab2[2]))
}

func srgbToOKLab(rgb [3]float64) [3]float64 {
	return xyzToOKLab(linSRGBToXYZ.mul(linSRGB(rgb)))
}

func okLCHToSRGB(lch [3]float64) [3]float64 {
	return gamSRGB(xyzToLinSRGB.mul(okLabToXYZ(polarToRectangular(lch))))
}

// gamutMapSRGB implements CSS Color 4 §13.2.2 with sRGB as destination.
func gamutMapSRGB(xyz [3]float64) [3]float64 { return gamutMapSRGBFrom(xyz, nil) }

// gamutMapSRGBFrom is [gamutMapSRGB], with the OKLCH coordinates of the
// color when known (else computed from xyz).
func gamutMapSRGBFrom(xyz [3]float64, oklch *[3]float64) [3]float64 {
	origin := gamSRGB(xyzToLinSRGB.mul(xyz))
	var lch [3]float64
	if oklch != nil {
		lch = *oklch
	} else {
		lch, _ = rectangularToPolar(xyzToOKLab(xyz), 0)
	}
	switch {
	case math.IsNaN(lch[0]) || math.IsNaN(lch[1]) || math.IsNaN(lch[2]):
		return clipSRGB(origin)
	case lch[0] >= 1:
		return [3]float64{1, 1, 1}
	case lch[0] <= 0:
		return [3]float64{0, 0, 0}
	case inSRGBGamut(origin):
		return origin
	}
	// An infinite chroma (a, b overflowing in hypot) would keep the
	// bisection below from narrowing: any huge chroma maps the same way.
	if math.IsInf(lch[1], 0) {
		lch[1] = math.MaxFloat64
	}

	const (
		jnd     = 0.02
		epsilon = 0.0001
	)
	current := lch
	clipped := clipSRGB(okLCHToSRGB(current))
	if deltaEOK(srgbToOKLab(clipped), polarToRectangular(current)) < jnd {
		return clipped
	}
	min, max, minInGamut := 0.0, lch[1], true
	// at most ~1040 halvings bring any finite chroma below epsilon
	for i := 0; max-min > epsilon && i < 2000; i++ {
		chroma := (min + max) / 2
		current[1] = chroma
		rgb := okLCHToSRGB(current)
		if minInGamut && inSRGBGamut(rgb) {
			min = chroma
			continue
		}
		clipped = clipSRGB(rgb)
		e := deltaEOK(srgbToOKLab(clipped), polarToRectangular(current))
		if e < jnd {
			if jnd-e < epsilon {
				return clipped
			}
			minInGamut = false
			min = chroma
		} else {
			max = chroma
		}
	}
	return clipped
}

// Contrast returns white or black, whichever has the higher WCAG 2.1
// contrast ratio against background (sRGB components in [0, 1]), as
// contrast-color() of CSS Color 5. White wins ties.
func Contrast(background [3]float64) Value {
	lin := linSRGB(background)
	lum := 0.2126*lin[0] + 0.7152*lin[1] + 0.0722*lin[2]
	// contrast against white: (1 + 0.05) / (lum + 0.05)
	// contrast against black: (lum + 0.05) / 0.05
	if 1.05/(lum+0.05) >= (lum+0.05)/0.05 {
		return New(SRGB, 1, 1, 1)
	}
	return New(SRGB, 0, 0, 0)
}
