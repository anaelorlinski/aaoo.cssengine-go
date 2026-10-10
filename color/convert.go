package color

import "math"

// Conversions between the predefined color spaces, ported from the sample
// code of CSS Color 4, §18 "Sample code for color conversions"
// (https://drafts.csswg.org/css-color-4/conversions.js), see NOTICE.
// Every space converts through CIE XYZ relative to D65.

type matrix [3][3]float64

func (m *matrix) mul(v [3]float64) [3]float64 {
	return [3]float64{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

// The D50 white point, as XYZ normalized to Y = 1.
var d50 = [3]float64{0.3457 / 0.3585, 1, (1.0 - 0.3457 - 0.3585) / 0.3585}

var (
	linSRGBToXYZ = matrix{
		{506752.0 / 1228815, 87881.0 / 245763, 12673.0 / 70218},
		{87098.0 / 409605, 175762.0 / 245763, 12673.0 / 175545},
		{7918.0 / 409605, 87881.0 / 737289, 1001167.0 / 1053270},
	}
	xyzToLinSRGB = matrix{
		{12831.0 / 3959, -329.0 / 214, -1974.0 / 3959},
		{-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810},
		{705.0 / 12673, -2585.0 / 12673, 705.0 / 667},
	}
	linP3ToXYZ = matrix{
		{608311.0 / 1250200, 189793.0 / 714400, 198249.0 / 1000160},
		{35783.0 / 156275, 247089.0 / 357200, 198249.0 / 2500400},
		{0, 32229.0 / 714400, 5220557.0 / 5000800},
	}
	xyzToLinP3 = matrix{
		{446124.0 / 178915, -333277.0 / 357830, -72051.0 / 178915},
		{-14852.0 / 17905, 63121.0 / 35810, 423.0 / 17905},
		{11844.0 / 330415, -50337.0 / 660830, 316169.0 / 330415},
	}
	// ProPhoto is relative to D50.
	linProPhotoToXYZ = matrix{
		{0.79776664490064230, 0.13518129740053308, 0.03134773412839220},
		{0.28807482881940130, 0.71183523424187300, 0.00008993693872564},
		{0, 0, 0.82510460251046020},
	}
	xyzToLinProPhoto = matrix{
		{1.34578688164715830, -0.25557208737979464, -0.05110186497554526},
		{-0.54463070512490190, 1.50824774284514680, 0.02052744743642139},
		{0, 0, 1.21196754563894520},
	}
	linA98ToXYZ = matrix{
		{573536.0 / 994567, 263643.0 / 1420810, 187206.0 / 994567},
		{591459.0 / 1989134, 6239551.0 / 9945670, 374412.0 / 4972835},
		{53769.0 / 1989134, 351524.0 / 4972835, 4929758.0 / 4972835},
	}
	xyzToLinA98 = matrix{
		{1829569.0 / 896150, -506331.0 / 896150, -308931.0 / 896150},
		{-851781.0 / 878810, 1648619.0 / 878810, 36519.0 / 878810},
		{16779.0 / 1248040, -147721.0 / 1248040, 1266979.0 / 1248040},
	}
	lin2020ToXYZ = matrix{
		{63426534.0 / 99577255, 20160776.0 / 139408157, 47086771.0 / 278816314},
		{26158966.0 / 99577255, 472592308.0 / 697040785, 8267143.0 / 139408157},
		{0, 19567812.0 / 697040785, 295819943.0 / 278816314},
	}
	xyzToLin2020 = matrix{
		{30757411.0 / 17917100, -6372589.0 / 17917100, -4539589.0 / 17917100},
		{-19765991.0 / 29648200, 47925759.0 / 29648200, 467509.0 / 29648200},
		{792561.0 / 44930125, -1921689.0 / 44930125, 42328811.0 / 44930125},
	}
	// Bradford chromatic adaptation.
	d65ToD50 = matrix{
		{1.0479297925449969, 0.022946870601609652, -0.05019226628920524},
		{0.02962780877005599, 0.9904344267538799, -0.017073799063418826},
		{-0.009243040646204504, 0.015055191490298152, 0.7518742814281371},
	}
	d50ToD65 = matrix{
		{0.955473421488075, -0.02309845494876471, 0.06325924320057072},
		{-0.0283697093338637, 1.0099953980813041, 0.021041441191917323},
		{0.012314014864481998, -0.020507649298898964, 1.330365926242124},
	}
	xyzToLMS = matrix{
		{0.8190224379967030, 0.3619062600528904, -0.1288737815209879},
		{0.0329836539323885, 0.9292868615863434, 0.0361446663506424},
		{0.0481771893596242, 0.2642395317527308, 0.6335478284694309},
	}
	lmsToOKLab = matrix{
		{0.2104542683093140, 0.7936177747023054, -0.0040720430116193},
		{1.9779985324311684, -2.4285922420485799, 0.4505937096174110},
		{0.0259040424655478, 0.7827717124575296, -0.8086757549230774},
	}
	lmsToXYZ = matrix{
		{1.2268798758459243, -0.5578149944602171, 0.2813910456659647},
		{-0.0405757452148008, 1.1122868032803170, -0.0717110580655164},
		{-0.0763729366746601, -0.4214933324022432, 1.5869240198367816},
	}
	okLabToLMS = matrix{
		{1, 0.3963377773761749, 0.2158037573099136},
		{1, -0.1055613458156586, -0.0638541728258133},
		{1, -0.0894841775298119, -1.2914855480194092},
	}
)

// Transfer functions. Each extends to negative values by symmetry.

func mapSigned(c [3]float64, f func(float64) float64) [3]float64 {
	for i, v := range c {
		c[i] = math.Copysign(f(math.Abs(v)), v)
	}
	return c
}

func linSRGB(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 {
		if a <= 0.04045 {
			return a / 12.92
		}
		return math.Pow((a+0.055)/1.055, 2.4)
	})
}

func gamSRGB(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 {
		if a > 0.0031308 {
			return 1.055*math.Pow(a, 1/2.4) - 0.055
		}
		return 12.92 * a
	})
}

func linProPhoto(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 {
		if a <= 16.0/512 {
			return a / 16
		}
		return math.Pow(a, 1.8)
	})
}

func gamProPhoto(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 {
		if a >= 1.0/512 {
			return math.Pow(a, 1/1.8)
		}
		return 16 * a
	})
}

func linA98(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 { return math.Pow(a, 563.0/256) })
}

func gamA98(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 { return math.Pow(a, 256.0/563) })
}

func lin2020(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 { return math.Pow(a, 2.4) })
}

func gam2020(c [3]float64) [3]float64 {
	return mapSigned(c, func(a float64) float64 { return math.Pow(a, 1/2.4) })
}

// CIE Lab (D50) and LCH.

const (
	labKappa   = 24389.0 / 27
	labEpsilon = 216.0 / 24389
)

func xyzToLab(xyz [3]float64) [3]float64 {
	var f [3]float64
	for i, v := range xyz {
		v /= d50[i]
		if v > labEpsilon {
			f[i] = math.Cbrt(v)
		} else {
			f[i] = (labKappa*v + 16) / 116
		}
	}
	return [3]float64{116*f[1] - 16, 500 * (f[0] - f[1]), 200 * (f[1] - f[2])}
}

func labToXYZ(lab [3]float64) [3]float64 {
	var f [3]float64
	f[1] = (lab[0] + 16) / 116
	f[0] = lab[1]/500 + f[1]
	f[2] = f[1] - lab[2]/200
	xyz := [3]float64{
		(116*f[0] - 16) / labKappa,
		lab[0] / labKappa,
		(116*f[2] - 16) / labKappa,
	}
	if f0 := f[0] * f[0] * f[0]; f0 > labEpsilon {
		xyz[0] = f0
	}
	if lab[0] > labKappa*labEpsilon {
		xyz[1] = math.Pow((lab[0]+16)/116, 3)
	}
	if f2 := f[2] * f[2] * f[2]; f2 > labEpsilon {
		xyz[2] = f2
	}
	return [3]float64{xyz[0] * d50[0], xyz[1] * d50[1], xyz[2] * d50[2]}
}

// rectangularToPolar converts (L, a, b) to (L, C, h); the hue of an
// achromatic color (C <= epsilon) is reported as missing.
func rectangularToPolar(lab [3]float64, epsilon float64) (lch [3]float64, hueMissing bool) {
	c := math.Hypot(lab[1], lab[2])
	h := normalizeHue(math.Atan2(lab[2], lab[1]) * 180 / math.Pi)
	return [3]float64{lab[0], c, h}, c <= epsilon
}

func polarToRectangular(lch [3]float64) [3]float64 {
	h := lch[2] * math.Pi / 180
	return [3]float64{lch[0], lch[1] * math.Cos(h), lch[1] * math.Sin(h)}
}

// Oklab.

func xyzToOKLab(xyz [3]float64) [3]float64 {
	lms := xyzToLMS.mul(xyz)
	for i, v := range lms {
		lms[i] = math.Cbrt(v)
	}
	return lmsToOKLab.mul(lms)
}

func okLabToXYZ(lab [3]float64) [3]float64 {
	lms := okLabToLMS.mul(lab)
	for i, v := range lms {
		lms[i] = v * v * v
	}
	return lmsToXYZ.mul(lms)
}

// HSL and HWB (CSS Color 4 §7.1 and §8.1).

func hslToSRGB(hsl [3]float64) [3]float64 {
	hue := normalizeHue(hsl[0])
	sat, light := hsl[1]/100, hsl[2]/100
	f := func(n float64) float64 {
		k := math.Mod(n+hue/30, 12)
		a := sat * math.Min(light, 1-light)
		return light - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	return [3]float64{f(0), f(8), f(4)}
}

func hwbToSRGB(hwb [3]float64) [3]float64 {
	white, black := hwb[1]/100, hwb[2]/100
	if white+black >= 1 {
		gray := white / (white + black)
		return [3]float64{gray, gray, gray}
	}
	rgb := hslToSRGB([3]float64{hwb[0], 100, 50})
	for i := range rgb {
		rgb[i] = rgb[i]*(1-white-black) + white
	}
	return rgb
}

// srgbToHSL follows the spec's better-rgbToHsl.js; the hue of an
// achromatic color is reported as missing.
func srgbToHSL(rgb [3]float64) (hsl [3]float64, hueMissing bool) {
	r, g, b := rgb[0], rgb[1], rgb[2]
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	light := (min + max) / 2
	d := max - min
	var hue, sat float64
	hueMissing = true
	if d != 0 {
		if light != 0 && light != 1 {
			sat = (max - light) / math.Min(light, 1-light)
		}
		switch max {
		case r:
			hue = (g - b) / d
			if g < b {
				hue += 6
			}
		case g:
			hue = (b-r)/d + 2
		default:
			hue = (r-g)/d + 4
		}
		hue *= 60
		hueMissing = false
	}
	if sat < 0 {
		hue += 180
		sat = -sat
	}
	if hue >= 360 {
		hue -= 360
	}
	if sat <= 1e-5 {
		hueMissing = true
	}
	return [3]float64{hue, sat * 100, light * 100}, hueMissing
}

func srgbToHWB(rgb [3]float64) (hwb [3]float64, hueMissing bool) {
	hsl, hueMissing := srgbToHSL(rgb)
	white := math.Min(rgb[0], math.Min(rgb[1], rgb[2]))
	black := 1 - math.Max(rgb[0], math.Max(rgb[1], rgb[2]))
	if white+black >= 1-1e-5 {
		hueMissing = true
	}
	return [3]float64{hsl[0], white * 100, black * 100}, hueMissing
}

// toXYZD65 converts the three components of v (with "none" read as 0) to
// XYZ D65. It reports false for spaces that have no defined conversion
// (device-cmyk, custom profiles).
func (v Value) toXYZD65() ([3]float64, bool) {
	c := v.components3()
	switch v.Space {
	case SRGB:
		return linSRGBToXYZ.mul(linSRGB(c)), true
	case SRGBLinear:
		return linSRGBToXYZ.mul(c), true
	case DisplayP3:
		return linP3ToXYZ.mul(linSRGB(c)), true
	case A98RGB:
		return linA98ToXYZ.mul(linA98(c)), true
	case ProPhotoRGB:
		return d50ToD65.mul(linProPhotoToXYZ.mul(linProPhoto(c))), true
	case Rec2020:
		return lin2020ToXYZ.mul(lin2020(c)), true
	case XYZD50:
		return d50ToD65.mul(c), true
	case XYZD65:
		return c, true
	case HSL:
		return linSRGBToXYZ.mul(linSRGB(hslToSRGB(c))), true
	case HWB:
		return linSRGBToXYZ.mul(linSRGB(hwbToSRGB(c))), true
	case Lab:
		return d50ToD65.mul(labToXYZ(c)), true
	case LCH:
		return d50ToD65.mul(labToXYZ(polarToRectangular(c))), true
	case OKLab:
		return okLabToXYZ(c), true
	case OKLCH:
		return okLabToXYZ(polarToRectangular(c)), true
	}
	return [3]float64{}, false
}

// fromXYZD65 converts XYZ D65 to the components of space; hueMissing
// reports an achromatic result in a polar space.
func fromXYZD65(space Space, xyz [3]float64) (c [3]float64, hueMissing bool) {
	switch space {
	case SRGB:
		return gamSRGB(xyzToLinSRGB.mul(xyz)), false
	case SRGBLinear:
		return xyzToLinSRGB.mul(xyz), false
	case DisplayP3:
		return gamSRGB(xyzToLinP3.mul(xyz)), false
	case A98RGB:
		return gamA98(xyzToLinA98.mul(xyz)), false
	case ProPhotoRGB:
		return gamProPhoto(xyzToLinProPhoto.mul(d65ToD50.mul(xyz))), false
	case Rec2020:
		return gam2020(xyzToLin2020.mul(xyz)), false
	case XYZD50:
		return d65ToD50.mul(xyz), false
	case XYZD65:
		return xyz, false
	case HSL:
		return srgbToHSL(gamSRGB(xyzToLinSRGB.mul(xyz)))
	case HWB:
		return srgbToHWB(gamSRGB(xyzToLinSRGB.mul(xyz)))
	case Lab:
		return xyzToLab(d65ToD50.mul(xyz)), false
	case LCH:
		return rectangularToPolar(xyzToLab(d65ToD50.mul(xyz)), 0.0015)
	case OKLab:
		return xyzToOKLab(xyz), false
	case OKLCH:
		return rectangularToPolar(xyzToOKLab(xyz), 0.000004)
	}
	panic("color: no conversion to " + space.String())
}

// Convertible reports whether the space has a defined conversion to the
// other predefined spaces (all but device-cmyk and custom profiles).
func (s Space) Convertible() bool {
	return s >= SRGB && s <= OKLCH
}

// To converts v to the target space, without gamut mapping. "none"
// components are read as 0; an achromatic result in a polar space (hsl,
// hwb, lch, oklch) has a missing hue. It reports false when either space
// is not [Space.Convertible].
func (v Value) To(target Space) (Value, bool) {
	if !v.Space.Convertible() || !target.Convertible() {
		return Value{}, false
	}
	if v.Space == target {
		return v, true
	}
	xyz, _ := v.toXYZD65()
	c, hueMissing := fromXYZD65(target, xyz)
	out := Value{Space: target, Count: 3, Alpha: v.Alpha}
	copy(out.Components[:], c[:])
	if hueMissing {
		hueIndex := 2
		if target == HSL || target == HWB {
			hueIndex = 0
		}
		out.Components[hueIndex] = 0
		out.Missing = 1 << hueIndex
	}
	return out, true
}
