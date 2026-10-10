package color

import "math"

// HueMethod is the hue interpolation method of CSS Color 4 §12.4.
type HueMethod uint8

const (
	HueShorter HueMethod = iota // the default
	HueLonger
	HueIncreasing
	HueDecreasing
)

// IsPolar reports whether the space has a hue component: HSL, HWB, LCH and
// OKLCH.
func (s Space) IsPolar() bool {
	switch s {
	case HSL, HWB, LCH, OKLCH:
		return true
	}
	return false
}

// hueIndex returns the index of the hue in a polar space.
func (s Space) hueIndex() int {
	if s == HSL || s == HWB {
		return 0
	}
	return 2
}

// componentKind classifies the components for carrying missing values
// forward between spaces (CSS Color 4 §12.2): analogous components share a
// kind, 0 means none.
type componentKind uint8

const (
	kindNone componentKind = iota
	kindRed
	kindGreen
	kindBlue
	kindLightness
	kindColorfulness
	kindHue
	kindOpponentA
	kindOpponentB
)

func (s Space) kinds() [3]componentKind {
	switch s {
	case SRGB, SRGBLinear, DisplayP3, A98RGB, ProPhotoRGB, Rec2020, XYZD50, XYZD65:
		return [3]componentKind{kindRed, kindGreen, kindBlue}
	case HSL:
		return [3]componentKind{kindHue, kindColorfulness, kindLightness}
	case HWB:
		return [3]componentKind{kindHue, kindNone, kindNone}
	case Lab, OKLab:
		return [3]componentKind{kindLightness, kindOpponentA, kindOpponentB}
	case LCH, OKLCH:
		return [3]componentKind{kindLightness, kindColorfulness, kindHue}
	}
	return [3]componentKind{}
}

// toInterpolation converts v to space, carrying its missing components
// forward to the analogous components of space (CSS Color 4 §12.2).
func (v Value) toInterpolation(space Space) (Value, bool) {
	out, ok := v.To(space)
	if !ok {
		return Value{}, false
	}
	if v.Space != space {
		from, to := v.Space.kinds(), space.kinds()
		// a hue made powerless by the conversion (an achromatic color) is
		// missing; the other components are missing only when carried
		powerless := out.Missing
		out.Missing = 0
		if space.IsPolar() {
			out.Missing |= powerless & (1 << space.hueIndex())
		}
		for i, k := range from {
			if k == kindNone || !v.IsMissing(i) {
				continue
			}
			for j, kt := range to {
				if kt == k {
					out.Missing |= 1 << j
				}
			}
		}
	}
	if v.IsMissing(AlphaMissing) {
		out.Missing |= 1 << AlphaMissing
	}
	return out, true
}

// Mix interpolates a and b in space (CSS Color 5 §2, color-mix()): t is the
// weight of b, in [0, 1], and the result's alpha is multiplied by
// alphaMultiplier. hue is the hue interpolation method, for polar spaces.
// It returns false if a color cannot be converted (a custom profile).
func Mix(space Space, hue HueMethod, a, b Value, t, alphaMultiplier float64) (Value, bool) {
	ca, ok1 := a.toInterpolation(space)
	cb, ok2 := b.toInterpolation(space)
	if !ok1 || !ok2 {
		return Value{}, false
	}
	out := Value{Space: space, Count: 3}

	// missing components take the other color's value (§12.3)
	for i := 0; i < 3; i++ {
		ma, mb := ca.IsMissing(i), cb.IsMissing(i)
		switch {
		case ma && mb:
			out.Missing |= 1 << i
		case ma:
			ca.Components[i] = cb.Components[i]
		case mb:
			cb.Components[i] = ca.Components[i]
		}
	}
	alphaA, alphaB := ca.Alpha, cb.Alpha
	switch ma, mb := ca.IsMissing(AlphaMissing), cb.IsMissing(AlphaMissing); {
	case ma && mb:
		alphaA, alphaB = 1, 1
		out.Missing |= 1 << AlphaMissing
	case ma:
		alphaA = alphaB
	case mb:
		alphaB = alphaA
	}

	hueI := -1
	if space.IsPolar() {
		hueI = space.hueIndex()
		ca.Components[hueI], cb.Components[hueI] = fixupHues(ca.Components[hueI], cb.Components[hueI], hue)
	}

	// premultiplied interpolation (§12.3), the hue excluded
	alpha := alphaA*(1-t) + alphaB*t
	for i := 0; i < 3; i++ {
		x, y := ca.Components[i], cb.Components[i]
		if i == hueI {
			out.Components[i] = normalizeHue(x*(1-t) + y*t)
			continue
		}
		v := (x*alphaA)*(1-t) + (y*alphaB)*t
		if alpha != 0 {
			v /= alpha
		} else {
			v = x*(1-t) + y*t
		}
		out.Components[i] = v
	}
	out.Alpha = alpha * alphaMultiplier
	if out.IsMissing(AlphaMissing) { // rendered as zero (§4.4)
		out.Alpha = 0
	}
	return out, true
}

// fixupHues adjusts the two hues, in degrees, for the interpolation method
// (CSS Color 4 §12.4).
func fixupHues(h1, h2 float64, method HueMethod) (float64, float64) {
	h1, h2 = math.Mod(h1, 360), math.Mod(h2, 360)
	if h1 < 0 {
		h1 += 360
	}
	if h2 < 0 {
		h2 += 360
	}
	d := h2 - h1
	switch method {
	case HueShorter:
		if d > 180 {
			h1 += 360
		} else if d < -180 {
			h2 += 360
		}
	case HueLonger:
		if 0 < d && d < 180 {
			h1 += 360
		} else if -180 < d && d <= 0 {
			h2 += 360
		}
	case HueIncreasing:
		if d < 0 {
			h2 += 360
		}
	case HueDecreasing:
		if d > 0 {
			h1 += 360
		}
	}
	return h1, h2
}
