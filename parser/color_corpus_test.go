package parser

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/color"
)

// The css-parsing-tests color corpora, run the way tinycss2 runs them
// (tests/test_tinycss2.py, the "_with_5" variants: one parser for every
// level). Each file is a JSON list of (input, expected) pairs; expected is
// null for an invalid color.

// pyNumber formats like tinycss2's _number: round to 6 decimals (after
// adding 1e-7), then Python's str() of the float, or "none".
func pyNumber(v float64, missing bool) string {
	if missing {
		return "none"
	}
	v, _ = strconv.ParseFloat(strconv.FormatFloat(v+0.0000001, 'f', 6, 64), 64)
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	if a := math.Abs(v); a < 1e-4 || a >= 1e16 {
		return strconv.FormatFloat(v, 'g', -1, 64) // like Python, e.g. 5e-05
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// rgbString formats an sRGB value as tinycss2's Color 3 tests do.
func rgbString(v color.Value) string {
	parts := make([]string, 3)
	for i := range parts {
		parts[i] = pyNumber(v.Component(i)*255, false)
	}
	if v.Alpha != 1 {
		return fmt.Sprintf("rgba(%s, %s)", strings.Join(parts, ", "), pyNumber(v.Alpha, false))
	}
	return fmt.Sprintf("rgb(%s)", strings.Join(parts, ", "))
}

// functionString formats "name(c1 c2 c3[ / alpha])", or with lead
// "name(lead c1 …)".
func functionString(name, lead string, v color.Value) string {
	parts := make([]string, 0, v.Count+1)
	if lead != "" {
		parts = append(parts, lead)
	}
	for i := 0; i < int(v.Count); i++ {
		parts = append(parts, pyNumber(v.Components[i], v.IsMissing(i)))
	}
	s := name + "(" + strings.Join(parts, " ")
	if v.Alpha != 1 {
		s += " / " + pyNumber(v.Alpha, false)
	}
	return s + ")"
}

// colorFunctionString is tinycss2's _build_color: color(<space> …).
func colorFunctionString(v color.Value) string {
	space := v.Space.String()
	if v.Space == color.Custom {
		space = v.Profile
	}
	return functionString("color", space, v)
}

func parseCorpusColor(input string, scheme ColorScheme) (color.Value, ColorType) {
	return ParseColorValue(ParseOneComponentValue(Tokenize([]byte(input), true)), scheme)
}

// srgbResult converts to sRGB (hsl and hwb corpora) and formats.
func srgbResult(want color.Space) func(t *testing.T, input string) any {
	return func(t *testing.T, input string) any {
		v, typ := parseCorpusColor(input, ColorSchemeLight)
		switch typ {
		case ColorInvalid:
			return nil
		case ColorCurrentColor:
			return "currentcolor"
		}
		if v.Space != want {
			t.Errorf("%q: parsed in %s, want %s", input, v.Space, want)
			return nil
		}
		rgb, _ := v.To(color.SRGB)
		return rgbString(rgb)
	}
}

func nativeResult(want color.Space) func(t *testing.T, input string) any {
	return func(t *testing.T, input string) any {
		v, typ := parseCorpusColor(input, ColorSchemeLight)
		switch typ {
		case ColorInvalid:
			return nil
		case ColorCurrentColor:
			return "currentcolor"
		}
		if v.Space != want {
			t.Errorf("%q: parsed in %s, want %s", input, v.Space, want)
			return nil
		}
		return functionString(want.String(), "", v)
	}
}

// specClamped adapts an expectation of the lab-like corpora to CSS Color 4,
// which clamps lightness to [0%, 100%] and chroma to positive values at
// parsed-value time (§8, §9). tinycss2 keeps them unclamped, and the
// corpus with it: its generator (generate_color_json.py) feeds lightness
// 110 and -10 and chroma -75 through unchanged. Only that range check
// differs; the syntax is checked as is.
func specClamped(space color.Space) func(want any) any {
	maxLightness := 100.0
	if space == color.OKLab || space == color.OKLCH {
		maxLightness = 1
	}
	polar := space == color.LCH || space == color.OKLCH
	return func(want any) any {
		s, ok := want.(string)
		if !ok {
			return want // null: invalid color
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(s, space.String()+"("), ")")
		channels, alpha, hasAlpha := strings.Cut(inner, " / ")
		parts := strings.Fields(channels)
		clamp := func(i int, lo, hi float64) {
			if v, err := strconv.ParseFloat(parts[i], 64); err == nil { // not "none"
				parts[i] = pyNumber(math.Max(lo, math.Min(hi, v)), false)
			}
		}
		clamp(0, 0, maxLightness)
		if polar {
			clamp(1, 0, math.Inf(1))
		}
		out := space.String() + "(" + strings.Join(parts, " ")
		if hasAlpha {
			out += " / " + alpha
		}
		return out + ")"
	}
}

func colorFunctionResult(scheme ColorScheme, input string) any {
	v, typ := parseCorpusColor(input, scheme)
	switch typ {
	case ColorInvalid:
		return nil
	case ColorCurrentColor:
		return "currentcolor"
	}
	return colorFunctionString(v)
}

func TestColorCorpora(t *testing.T) {
	// expectations adapted to the spec (see specClamped)
	expect := map[string]func(want any) any{
		"color_lab_4.json":   specClamped(color.Lab),
		"color_lch_4.json":   specClamped(color.LCH),
		"color_oklab_4.json": specClamped(color.OKLab),
		"color_oklch_4.json": specClamped(color.OKLCH),
	}
	for _, tc := range []struct {
		file   string
		result func(t *testing.T, input string) any
	}{
		{"color_hexadecimal_3.json", srgbResult(color.SRGB)},
		{"color_hexadecimal_4.json", srgbResult(color.SRGB)},
		{"color_keywords_3.json", srgbResult(color.SRGB)},
		{"color_keywords_4.json", srgbResult(color.SRGB)},
		{"color_hsl_3.json", srgbResult(color.HSL)},
		{"color_hsl_4.json", srgbResult(color.HSL)},
		{"color_hwb_4.json", srgbResult(color.HWB)},
		{"color_lab_4.json", nativeResult(color.Lab)},
		{"color_lch_4.json", nativeResult(color.LCH)},
		{"color_oklab_4.json", nativeResult(color.OKLab)},
		{"color_oklch_4.json", nativeResult(color.OKLCH)},
		{"color_function_4.json", func(t *testing.T, input string) any {
			return colorFunctionResult(ColorSchemeLight, input)
		}},
		{"color_functions_5.json", func(t *testing.T, input string) any {
			if strings.HasPrefix(input, "light-dark") {
				return []any{colorFunctionResult(ColorSchemeLight, input), colorFunctionResult(ColorSchemeDark, input)}
			}
			return colorFunctionResult(ColorSchemeLight, input)
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("css-parsing-tests", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			var data []any
			if err := json.Unmarshal(b, &data); err != nil {
				t.Fatal(err)
			}
			failures := 0
			for i := 0; i+1 < len(data); i += 2 {
				input, want := data[i].(string), data[i+1]
				if adapt := expect[tc.file]; adapt != nil {
					want = adapt(want)
				}
				// round-trip through JSON to compare with the decoded expectation
				got := tc.result(t, input)
				gotJSON, _ := json.Marshal(got)
				var gotDecoded any
				_ = json.Unmarshal(gotJSON, &gotDecoded)
				if !reflect.DeepEqual(gotDecoded, want) {
					failures++
					if failures <= 10 {
						t.Errorf("%q: got %v, want %v", input, got, want)
					}
				}
			}
			if failures > 10 {
				t.Errorf("... %d failures in total out of %d", failures, len(data)/2)
			}
		})
	}
}

func TestColorCurrentColorCase(t *testing.T) {
	for _, input := range []string{"currentcolor", "currentColor", "CURRENTCOLOR"} {
		if _, typ := parseCorpusColor(input, ColorSchemeLight); typ != ColorCurrentColor {
			t.Errorf("%q: not currentcolor", input)
		}
	}
}
