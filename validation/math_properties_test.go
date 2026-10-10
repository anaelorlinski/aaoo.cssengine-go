package validation

import (
	"testing"

	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// T13 (webrender docs/css-review-bugs.md): properties whose grammar takes a <length>,
// <number> or <integer> also take a math function resolving to it.
func TestMathAcceptedByValidators(t *testing.T) {
	for _, tc := range []struct {
		css  string
		prop pr.KnownProp
	}{
		{"z-index: calc(1 + 1)", pr.PZIndex},
		{"border-top-width: calc(1px + 1px)", pr.PBorderTopWidth},
		{"outline-width: max(1px, 2px)", pr.POutlineWidth},
		{"column-width: calc(10px * 2)", pr.PColumnWidth},
		{"vertical-align: calc(1px + 1px)", pr.PVerticalAlign},
		{"hyphenate-limit-zone: calc(1px + 1px)", pr.PHyphenateLimitZone},
		{"tab-size: calc(2 + 2)", pr.PTabSize},
		{"tab-size: calc(3px * 2)", pr.PTabSize},
	} {
		d := expandToDict(t, tc.css, "")
		if _, ok := d[tc.prop].(pr.PendingMath); !ok {
			t.Errorf("%s: got %v (%T), want a math function", tc.css, d[tc.prop], d[tc.prop])
		}
	}
	// a <number> math expression without relative units is resolved at
	// once: the flex factors
	for css, want := range map[string]pr.Float{"flex-grow: calc(1 + 1)": 2, "flex-shrink: calc(2 * 2)": 4} {
		d := expandToDict(t, css, "")
		for _, v := range d {
			if v != want {
				t.Errorf("%s: got %v, want %v", css, v, want)
			}
		}
	}
}

// tab-size used to accept any invalid value as 0.
func TestTabSizeRejectsInvalidValues(t *testing.T) {
	for _, css := range []string{"tab-size: foo", "tab-size: -2", "tab-size: 10%"} {
		assertInvalid(t, css, "invalid")
	}
	d := expandToDict(t, "tab-size: 4", "")
	if d[pr.PTabSize] != pr.NewDim(4, 0).Tagged() {
		t.Errorf("tab-size: 4 -> %v", d[pr.PTabSize])
	}
}

// hyphenate-limit-zone takes math for lengths only: the text layout reads
// a resolved length or a plain percentage.
func TestHyphenateLimitZoneMath(t *testing.T) {
	assertInvalid(t, "hyphenate-limit-zone: calc(10% + 1px)", "invalid")
	d := expandToDict(t, "hyphenate-limit-zone: 10%", "")
	if d[pr.PHyphenateLimitZone] != pr.PercToD(10).Tagged() {
		t.Errorf("hyphenate-limit-zone: 10%% -> %v", d[pr.PHyphenateLimitZone])
	}
}

// V2-4: math accepted where WeasyPrint's get_length / get_angle /
// get_number / get_resolution resolve it.
func TestMathAcceptedByValidators2(t *testing.T) {
	for _, css := range []string{
		"transform: rotate(calc(45deg))", "transform: rotate(calc(1turn / 8))", "transform: translate(calc(1px + 2px))",
		"transform: translate(calc(50% - 1px), 2px)", "transform: scale(calc(1 + 1))", "transform: rotate(asin(1))",
		"transform: matrix(1, 0, 0, calc(2 * 1), 0, 0)", "transform: skewX(calc(10deg))",
		"background-size: calc(50%) calc(10px)", "background-position: calc(50% - 10px) calc(1em)",
		"background-position: right calc(10px + 1em) top 5px",
		"orphans: calc(2)", "widows: calc(1 + 1)", "order: calc(-2)",
		"image-resolution: calc(2dppx)", "image-resolution: calc(96dpi * 2)",
		"vertical-align: calc(50%)", "vertical-align: calc(1em + 50%)", "hyphenate-limit-zone: calc(10%)",
		"background-image: linear-gradient(red calc(10% + 5px), blue)",
		"transform: rotate(90DEG)",
	} {
		t.Run(css, func(t *testing.T) {
			if d := expandToDict(t, css, ""); len(d) == 0 {
				t.Errorf("%s rejected", css)
			}
		})
	}
	for _, css := range []string{"orphans: calc(1px)", "transform: scale(calc(1px))", "transform: rotate(calc(1px))", "hyphenate-limit-zone: calc(10% + 1px)"} {
		t.Run(css, func(t *testing.T) { assertInvalid(t, css, "Ignored") })
	}
	// integers resolve at once, clamped
	if d := expandToDict(t, "orphans: calc(0.4)", ""); d[pr.POrphans] != pr.Int(1) {
		t.Errorf("orphans: calc(0.4): %v", d)
	}
}

// V2-8: tab-size is a <number>, not only an integer.
func TestTabSizeNumber(t *testing.T) {
	d := expandToDict(t, "tab-size: 1.5", "")
	if d[pr.PTabSize] != pr.NewDim(1.5, 0).Tagged() {
		t.Errorf("tab-size: 1.5: %v", d)
	}
}
