package validation

import (
	"math"
	"testing"

	tu "github.com/anaelorlinski/aaoo.cssengine-go/internal/testutils"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

var fuzzMathShapes = []pr.MathType{
	pr.MathNumber, pr.MathInteger, pr.MathLength, pr.MathPercentage,
	pr.MathLengthPercentage, pr.MathAngle, pr.MathResolution,
}

// FuzzMath feeds arbitrary text to the math checker and evaluator (with
// and without the checkMath shape check, as relative colours do), and to
// a few property validators that accept math functions. Nothing may
// panic, and a successful top-level evaluation must be finite.
func FuzzMath(f *testing.F) {
	for _, s := range validMathExpressions {
		f.Add(s)
	}
	for _, s := range invalidMathExpressions {
		f.Add(s)
	}
	for _, s := range []string{
		"min(100%, 600px)", "clamp(1)", "round(up)", "calc(1px / 0)",
		"calc(sin(calc(30deg)) * 1px)", "mod(-7px, calc(infinity * 1px))",
		"rgb(from red calc(r / 2) g b)", "12px/", "bold 12px/ serif",
		"var(--a, calc(1px + 2px))", "calc(1dppx + 96dpi)",
	} {
		f.Add(s)
	}
	ctxs := []MathContext{
		{},
		{FontSize: 16, RootFontSize: 16, PercentageReference: 1000, HasPercentageRef: true},
	}
	f.Fuzz(func(t *testing.T, input string) {
		capt := tu.CaptureLogs(t)
		defer capt.Logs()

		for _, tok := range parser.Tokenize([]byte(input), false) {
			HasVar(tok)
			fb, ok := tok.(parser.FunctionBlock)
			if !ok {
				continue
			}
			for _, shape := range fuzzMathShapes {
				for _, ctx := range ctxs {
					raw := pr.PendingMath{Expr: fb, ResolveTo: shape}
					checkFinite(t, input, raw, ctx)
					if pm, err := parseMath(fb, shape, ""); err == nil {
						checkFinite(t, input, pm, ctx)
					}
				}
			}
		}
		for _, prop := range []string{"width", "font", "font-size", "color", "background", "opacity", "line-height"} {
			PreprocessDeclarations("", parser.ParseDeclarationListString(prop+": "+input, false, false))
		}
	})
}

func checkFinite(t *testing.T, input string, pm pr.PendingMath, ctx MathContext) {
	if v, err := EvalMath(pm, ctx); err == nil && (math.IsNaN(float64(v)) || math.IsInf(float64(v), 0)) {
		t.Fatalf("%q: EvalMath returned a non-finite %g", input, v)
	}
	if d, err := EvalMathDim(pm, ctx); err == nil && (math.IsNaN(float64(d.Value)) || math.IsInf(float64(d.Value), 0)) {
		t.Fatalf("%q: EvalMathDim returned a non-finite %g", input, d.Value)
	}
}
