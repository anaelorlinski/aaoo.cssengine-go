// Package color models CSS colors (CSS Color Levels 4 and 5) independently
// of their syntax: a color space, the components in that space, and an alpha
// value. It converts between the predefined color spaces and renders colors
// to sRGB with the CSS gamut-mapping algorithm.
//
// Parsing CSS <color> syntax lives in the parser package, which produces
// [Value]s. Output backends that support other color spaces (CMYK, wide
// gamut) can use the specified [Value] directly; the others use [Value.SRGB].
package color

import "math"

// Space identifies a color space.
type Space uint8

const (
	// SpaceNone is the zero value: no color.
	SpaceNone Space = iota

	// RGB spaces, components in [0, 1].
	SRGB
	SRGBLinear
	DisplayP3
	A98RGB
	ProPhotoRGB
	Rec2020

	// CIE XYZ, relative to the D50 or D65 white point.
	XYZD50
	XYZD65

	// HSL and HWB: hue in degrees [0, 360), the other two components in
	// [0, 100].
	HSL
	HWB

	// CIE Lab and LCH (D50): L in [0, 100], a and b around [-125, 125],
	// C around [0, 150], hue in degrees.
	Lab
	LCH

	// Oklab and Oklch: L in [0, 1], a and b around [-0.4, 0.4], C around
	// [0, 0.4], hue in degrees.
	OKLab
	OKLCH

	// DeviceCMYK is device-cmyk(): cyan, magenta, yellow, black in [0, 1].
	DeviceCMYK

	// Custom is color(--profile …): a space defined by an @color-profile
	// rule, named by [Value.Profile]. It cannot be converted without the
	// profile.
	Custom
)

var spaceNames = [...]string{
	SpaceNone:   "",
	SRGB:        "srgb",
	SRGBLinear:  "srgb-linear",
	DisplayP3:   "display-p3",
	A98RGB:      "a98-rgb",
	ProPhotoRGB: "prophoto-rgb",
	Rec2020:     "rec2020",
	XYZD50:      "xyz-d50",
	XYZD65:      "xyz-d65",
	HSL:         "hsl",
	HWB:         "hwb",
	Lab:         "lab",
	LCH:         "lch",
	OKLab:       "oklab",
	OKLCH:       "oklch",
	DeviceCMYK:  "device-cmyk",
	Custom:      "custom",
}

// String returns the CSS name of the space, as used in color().
func (s Space) String() string {
	if int(s) < len(spaceNames) {
		return spaceNames[s]
	}
	return "<invalid color space>"
}

// PredefinedSpace returns the space named by a color() function, like
// "display-p3". "xyz" is an alias for "xyz-d65".
func PredefinedSpace(name string) (Space, bool) {
	switch name {
	case "srgb":
		return SRGB, true
	case "srgb-linear":
		return SRGBLinear, true
	case "display-p3":
		return DisplayP3, true
	case "a98-rgb":
		return A98RGB, true
	case "prophoto-rgb":
		return ProPhotoRGB, true
	case "rec2020":
		return Rec2020, true
	case "xyz", "xyz-d65":
		return XYZD65, true
	case "xyz-d50":
		return XYZD50, true
	}
	return SpaceNone, false
}

// MaxComponents is the largest number of components a [Value] holds.
const MaxComponents = 4

// Value is a specified color: a color space, its components and an alpha.
//
// Value is comparable, so it can be stored in interfaces that are compared
// with ==; components are therefore a fixed-size array, and "none" is a
// bitmask rather than NaN.
type Value struct {
	Space Space
	// Profile names the @color-profile of a [Custom] space.
	Profile string
	// Components in the reference ranges of Space; only the first Count
	// are meaningful.
	Components [MaxComponents]float64
	Count      uint8
	// Missing has bit i set when component i is "none", and bit
	// [AlphaMissing] when the alpha is "none" (Alpha is then 0).
	Missing uint8
	// Alpha is in [0, 1].
	Alpha float64
}

// New returns an opaque color with the given components, none missing.
func New(space Space, components ...float64) Value {
	v := Value{Space: space, Count: uint8(len(components)), Alpha: 1}
	copy(v.Components[:], components)
	return v
}

// AlphaMissing is the index of the alpha in [Value.Missing]: a value has at
// most 4 components.
const AlphaMissing = 4

// IsMissing reports whether component i is "none".
func (v Value) IsMissing(i int) bool { return v.Missing&(1<<i) != 0 }

// Component returns component i, with "none" read as 0.
func (v Value) Component(i int) float64 {
	if v.IsMissing(i) {
		return 0
	}
	return v.Components[i]
}

func (v Value) components3() [3]float64 {
	return [3]float64{v.Component(0), v.Component(1), v.Component(2)}
}

func normalizeHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}
