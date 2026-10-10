package parser

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Cases from web-platform-tests css/css-color/parsing/color-valid-lab.html
// (3-clause BSD), the conformance tests linked by CSS Color 4 for lab(),
// lch(), oklab() and oklch(): [input, serialization]. Those using calc(),
// none or infinities are left out (math is resolved by the validation
// package). Unlike the css-parsing-tests corpus, they check that
// lightness is clamped to [0%, 100%] and chroma to positive values.
var wptLabCases = [][2]string{
	{"lab(0 0 0)", "lab(0 0 0)"},
	{"lab(0 0 0 / 1)", "lab(0 0 0)"},
	{"lab(0 0 0 / 0.5)", "lab(0 0 0 / 0.5)"},
	{"lab(20 0 10/0.5)", "lab(20 0 10 / 0.5)"},
	{"lab(20 0 10/50%)", "lab(20 0 10 / 0.5)"},
	{"lab(400 0 10/50%)", "lab(100 0 10 / 0.5)"},
	{"lab(50 -160 160)", "lab(50 -160 160)"},
	{"lab(50 -200 200)", "lab(50 -200 200)"},
	{"lab(0 0 0 / -10%)", "lab(0 0 0 / 0)"},
	{"lab(0 0 0 / 110%)", "lab(0 0 0)"},
	{"lab(0 0 0 / 300%)", "lab(0 0 0)"},
	{"lab(-40 0 0)", "lab(0 0 0)"},
	{"lab(50 -20 0)", "lab(50 -20 0)"},
	{"lab(50 0 -20)", "lab(50 0 -20)"},
	{"lab(50% 50% -20%)", "lab(50 62.5 -25)"},
	{"lab(50 -20% -20%)", "lab(50 -25 -25)"},
	{"oklab(0 0 0)", "oklab(0 0 0)"},
	{"oklab(0 0 0 / 1)", "oklab(0 0 0)"},
	{"oklab(0 0 0 / 0.5)", "oklab(0 0 0 / 0.5)"},
	{"oklab(0.2 0 0.1/0.5)", "oklab(0.2 0 0.1 / 0.5)"},
	{"oklab(0.2 0 0.1/50%)", "oklab(0.2 0 0.1 / 0.5)"},
	{"oklab(4 0 0.1/50%)", "oklab(1 0 0.1 / 0.5)"},
	{"oklab(0.5 -1.6 1.6)", "oklab(0.5 -1.6 1.6)"},
	{"oklab(0.5 -2 2)", "oklab(0.5 -2 2)"},
	{"oklab(0 0 0 / -10%)", "oklab(0 0 0 / 0)"},
	{"oklab(0 0 0 / 110%)", "oklab(0 0 0)"},
	{"oklab(0 0 0 / 300%)", "oklab(0 0 0)"},
	{"oklab(-0.4 0 0)", "oklab(0 0 0)"},
	{"oklab(0.5 -2 0)", "oklab(0.5 -2 0)"},
	{"oklab(0.5 0 -2)", "oklab(0.5 0 -2)"},
	{"oklab(50% 50% -20%)", "oklab(0.5 0.2 -0.08)"},
	{"oklab(0.5 -20% -20%)", "oklab(0.5 -0.08 -0.08)"},
	{"lab(20% -50% 90%/0.5)", "lab(20 -62.5 112.5 / 0.5)"},
	{"oklab(20% 70% -80%/0.5)", "oklab(0.2 0.28 -0.32 / 0.5)"},
	{"lch(0 0 0deg)", "lch(0 0 0)"},
	{"lch(0 0 0deg / 1)", "lch(0 0 0)"},
	{"lch(0 0 0deg / 0.5)", "lch(0 0 0 / 0.5)"},
	{"lch(100 230 0deg / 0.5)", "lch(100 230 0 / 0.5)"},
	{"lch(20 50 20deg/0.5)", "lch(20 50 20 / 0.5)"},
	{"lch(20 50 20deg/50%)", "lch(20 50 20 / 0.5)"},
	{"lch(10 20 20deg / -10%)", "lch(10 20 20 / 0)"},
	{"lch(10 20 20deg / 110%)", "lch(10 20 20)"},
	{"lch(10 20 1.28rad)", "lch(10 20 73.3386)"},
	{"lch(10 20 380deg)", "lch(10 20 20)"},
	{"lch(10 20 -340deg)", "lch(10 20 20)"},
	{"lch(10 20 740deg)", "lch(10 20 20)"},
	{"lch(10 20 -700deg)", "lch(10 20 20)"},
	{"lch(-40 0 0)", "lch(0 0 0)"},
	{"lch(20 -20 0)", "lch(20 0 0)"},
	{"lch(0 0 0 / 0.5)", "lch(0 0 0 / 0.5)"},
	{"lch(10 20 20 / 110%)", "lch(10 20 20)"},
	{"lch(10 20 -700)", "lch(10 20 20)"},
	{"lch(50% 50% 20)", "lch(50 75 20)"},
	{"lch(0.5 -20% -20)", "lch(0.5 0 340)"},
	{"oklch(0 0 0deg)", "oklch(0 0 0)"},
	{"oklch(0 0 0deg / 1)", "oklch(0 0 0)"},
	{"oklch(0 0 0deg / 0.5)", "oklch(0 0 0 / 0.5)"},
	{"oklch(1 2.3 0deg / 0.5)", "oklch(1 2.3 0 / 0.5)"},
	{"oklch(0.2 0.5 20deg/0.5)", "oklch(0.2 0.5 20 / 0.5)"},
	{"oklch(0.2 0.5 20deg/50%)", "oklch(0.2 0.5 20 / 0.5)"},
	{"oklch(0.1 0.2 20deg / -10%)", "oklch(0.1 0.2 20 / 0)"},
	{"oklch(0.1 0.2 20deg / 110%)", "oklch(0.1 0.2 20)"},
	{"oklch(0.1 0.2 1.28rad)", "oklch(0.1 0.2 73.3386)"},
	{"oklch(0.1 0.2 380deg)", "oklch(0.1 0.2 20)"},
	{"oklch(0.1 0.2 -340deg)", "oklch(0.1 0.2 20)"},
	{"oklch(0.1 0.2 740deg)", "oklch(0.1 0.2 20)"},
	{"oklch(0.1 0.2 -700deg)", "oklch(0.1 0.2 20)"},
	{"oklch(-4 0 0)", "oklch(0 0 0)"},
	{"oklch(0.2 -0.2 0)", "oklch(0.2 0 0)"},
	{"oklch(0 0 0 / 0.5)", "oklch(0 0 0 / 0.5)"},
	{"oklch(0.1 0.2 20 / 110%)", "oklch(0.1 0.2 20)"},
	{"oklch(0.1 0.2 -700)", "oklch(0.1 0.2 20)"},
	{"oklch(50% 50% 20)", "oklch(0.5 0.2 20)"},
	{"oklch(0.5 -20% -20)", "oklch(0.5 0 340)"},
	{"lch(20% 80% 10/0.5)", "lch(20 120 10 / 0.5)"},
	{"oklch(20% 60% 10/0.5)", "oklch(0.2 0.24 10 / 0.5)"},
}

func TestColorWPTLab(t *testing.T) {
	for _, tc := range wptLabCases {
		v, typ := parseCorpusColor(tc[0], ColorSchemeLight)
		if typ != ColorRGBA {
			t.Errorf("%s: invalid", tc[0])
			continue
		}
		// compare numerically: WPT serializes with 6 significant digits
		name, args, _ := strings.Cut(strings.TrimSuffix(tc[1], ")"), "(")
		channels, alpha, hasAlpha := strings.Cut(args, " / ")
		fields := strings.Fields(channels)
		if name != v.Space.String() || len(fields) != int(v.Count) {
			t.Errorf("%s: got %s, want %s", tc[0], functionString(v.Space.String(), "", v), tc[1])
			continue
		}
		want := make([]float64, 0, 4)
		for _, f := range fields {
			n, _ := strconv.ParseFloat(f, 64)
			want = append(want, n)
		}
		wantAlpha := 1.0
		if hasAlpha {
			wantAlpha, _ = strconv.ParseFloat(alpha, 64)
		}
		for i, w := range want {
			if math.Abs(v.Components[i]-w) > 1e-4*math.Max(1, math.Abs(w)) {
				t.Errorf("%s: got %s, want %s", tc[0], functionString(v.Space.String(), "", v), tc[1])
				break
			}
		}
		if math.Abs(v.Alpha-wantAlpha) > 1e-6 {
			t.Errorf("%s: alpha %v, want %v", tc[0], v.Alpha, wantAlpha)
		}
	}
}

// CSS Color 4 clamps at parsed-value time, so out-of-range values render
// like their clamped equivalents: a negative chroma is not the opposite
// hue, a negative saturation is gray (§7, §9).
func TestColorParsedValueClamping(t *testing.T) {
	for _, tc := range [][2]string{
		{"lch(50 -30 0)", "lch(50 0 0)"},
		{"oklch(0.5 -0.1 120)", "oklch(0.5 0 120)"},
		{"lab(110 50 0)", "lab(100 50 0)"},
		{"oklab(-0.2 0.1 0)", "oklab(0 0.1 0)"},
		{"hsl(120 -50% 50%)", "hsl(120 0% 50%)"},
		{"hsl(120, -50%, 50%)", "hsl(120, 0%, 50%)"},
	} {
		got, want := ParseColorString(tc[0]), ParseColorString(tc[1])
		if got.IsNone() || got.RGBA != want.RGBA {
			t.Errorf("%s renders %v, want %v like %s", tc[0], got.RGBA, want.RGBA, tc[1])
		}
	}
}
