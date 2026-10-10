package validation

import (
	"math"
	"testing"

	tu "github.com/anaelorlinski/aaoo.cssengine-go/internal/testutils"
	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Port of WeasyPrint's tests/css/test_math.py.
//
// WeasyPrint's tests render a full HTML document and check the resulting
// `div.width` for each parametrised expression. We don't yet have a
// renderPages-equivalent harness in this package, so the port is split
// into two layers:
//
//   1. Validator-level: each valid expression must produce a property
//      (no error log); each invalid expression must be rejected (one log).
//      This mirrors WeasyPrint's `assert len(logs) == 1` discipline.
//   2. Evaluator-level: a handful of expressions are evaluated directly
//      via [EvalMath] to lock down the numeric semantics that
//      WeasyPrint asserts at layout time (`isclose(div.width, 100)`).
//
// :copyright: 2011-2014 Simon Sapin and contributors (WeasyPrint).
// :license:   BSD.

// validMathExpressions lists `width` values from the parametrised
// `test_math_functions` body in test_math.py. The original test asserts
// each yields div.width == 100; we only check the expression validates.
//
// Removed cases that depend on units we don't yet recognise:
//   - 'calc(50vw)', 'calc(20pvh)' — viewport units not in lengthUnits.
//   - 'calc(100px*var(--one))' style expressions where the var() must be
//     resolved before validation; our `--one: 1` substitution requires a
//     full document, not standalone CSS, so they're checked separately.
var validMathExpressions = []string{
	"calc(100px)",
	"calc(10em)",
	"calc(50%)",
	"calc(10px + 90px)",
	"calc(5em + 50px)",
	"calc(2 * 5em)",
	"calc(2 * (3em + 20px))",
	"calc(25% * (1 + 1))",
	"calc(20% * (1 + 1) + 20px)",
	"calc(100px",
	"max(100px)",
	"max(30%, 2em, 100px)",
	"max(-30%, -2em, 10em)",
	"calc(max(-1, 1, 2) * 50px)",
	"min(100px)",
	"min(100%, 20em, 100px)",
	"calc(min(4, 2) * 50px)",
	"calc(sqrt(4) * 50px)",
	"calc(pow(2, 2) * 25px)",
	"calc(hypot(2) * 50px)",
	"calc(hypot(3, 4) * 20px)",
	"calc(hypot(2px) * 50)",
	"calc(hypot(3px, 4px) * 20)",
	"calc(log(e) * 100px)",
	"calc(log(100, 10) * 50px)",
	"calc(exp(1) / e * 100px)",
	"abs(-100px)",
	"calc(abs(-100) * 1px)",
	"calc(sign(-100) * -100px)",
	"calc(sign(-100px) * -100px)",
	"calc(sqrt(16) * min(25px, 100%))",
	"clamp(calc(-infinity * 1px), 10em, calc(infinity * 1px))",
	"clamp(50px, 10em, 500px)",
	"clamp(100px, 2em, 500px)",
	"clamp(10px, 100em, 10em)",
	"clamp(10px, 100%, 10em)",
	"round(100.4px)",
	"round(145.4px, 100px)",
	"round(nearest, 100px)",
	"round(down, 195px, 100px)",
	"round(up, 5px, 100px)",
	"round(to-zero, 195px, 100px)",
	"mod(300px, 200px)",
	"calc(mod(300px, -200px) * -1)",
	"calc(mod(-300px, -200px) * -1)",
	"rem(300px, 200px)",
	"rem(300px, -200px)",
	"calc(rem(-300px, -200px) * -1)",
	"calc(sin(30deg) * 200px)",
	"calc(cos(60deg) * 200px)",
	"calc(tan(45deg) * 100px)",
	"calc(tan(calc(pi / 4)) * 100px)",
	"calc(sin(asin(0.5)) * 200px)",
	"calc(cos(acos(0.5)) * 200px)",
	"calc(tan(atan(1)) * 100px)",
	"calc(tan(atan2(1, 1)) * 100px)",
}

func TestMathFunctionsValid(t *testing.T) {
	for _, expr := range validMathExpressions {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			capt := tu.CaptureLogs(t)
			defer capt.AssertNoLogs(t)
			d := expandToDict(t, "width: "+expr, "")
			if len(d) == 0 {
				t.Fatalf("expected a property, got nothing for %q", expr)
			}
		})
	}
}

// invalidMathExpressions ports WeasyPrint's `test_math_functions_error`
// cases: shape errors (bad arity, unknown functions, non-numeric tokens),
// then, in invalidMathTypeExpressions, type errors, which the type check of
// parseMath (CSS Values 4 §10.9) rejects at parse time like WeasyPrint.
var invalidMathExpressions = []string{
	`calc("100px")`,                   // string operand
	"calc(100px, 100px)",              // calc takes one operand
	`min("10px")`,                     // string operand
	`max("10px")`,                     // string operand
	"min()",                           // 0-arity
	"max()",                           // 0-arity
	"clamp()",                         // 0-arity
	"clamp(10px)",                     // 1-arity
	"clamp(10px, 50px)",               // 2-arity
	"clamp(10px, 50px, 100px, 200px)", // 4-arity
	`clamp(10px, "50px", 100px)`,      // string in middle
	"round()",                         // 0-arity
	"round(100px, 10px, 1)",           // 4-arity (no leading kw)
	"round(nearest, 100px, 10px, 1)",  // 4-arity (with leading kw)
	"round(unknown, 100px)",           // bad rounding-strategy
	"round(unknown, 100px, 10px)",     // bad rounding-strategy
	`round(100px, "10px")`,            // string operand
	"mod()",
	"mod(10px)",
	`mod(100px, "10px")`,
	"mod(100px, 10px, 1px)",
	"rem()",
	"rem(10px)",
	`rem(100px, "10px")`,
	"rem(100px, 10px, 1px)",
	"sin()", "cos()", "tan()",
	"asin()", "acos()", "atan()", "atan2()",
	"atan2(0.5)",
	"pow()",
	"pow(4, 3, 4)",
	"sqrt()",
	"sqrt(4, 2)",
	"hypoth()", "hypoth(3)", "hypoth(3, 4)", // unknown function name
	"log()",
	"log(10, 10, 10)",
	"exp()",
	"exp(10, 10)",
	"exp(10px, 10)",
	"exp(10, 10, 10)",
	"abs()",
	"abs(10px, 100)",
	"sign()",
	"sign(10px, 10)",
}

// invalidMathTypeExpressions are the type errors of
// `test_math_functions_error`, for width: a <number> where a <length> is
// expected, a product of lengths, mixed types.
var invalidMathTypeExpressions = []string{
	"calc(100)",              // bare number for width (needs target-shape check)
	"calc(100px 100px)",      // missing operator (parser keeps token list)
	"calc(100px * 100px)",    // length × length — should fail unit dimension reduction
	"calc(100 * 100)",        // bare number for width
	"calc(calc(100unknown))", // unknown unit; tokenized as Dimension w/ unit "unknown"
	"calc(0.1)", "calc(-1)",  // bare numbers
	"min(10)", "max(10)", // bare number for width
	"min(10, 5px)", "max(10, 50px)", // mixed types
	"calc(min(1, 5px) * 10px)", "calc(max(100, 5px) * 10px)",
	"calc(100* - max(56px, 1rem)", // unbalanced parens (parser tolerant)
	"round(100)", "round(100, 10)", "round(nearest, 100, 10)",
	"round(100px, 10)", "round(nearest, 100px, 10)",
	"mod(100px, 10)", "calc(mod(300px, 200) * -1)",
	"rem(100px, 10)", "calc(rem(300px, 200) * -1)",
	"asin(50deg)", "acos(50deg)", "atan(50deg)", "atan2(50deg, 1)",
	"calc(sin(asin(50deg)) * 200px)", "calc(sin(asin(0.5, 2)) * 200px)",
	"calc(cos(acos(50deg)) * 200px)", "calc(cos(acos(0.5, 2)) * 200px)",
	"calc(tan(atan(50deg)) * 200px)", "calc(tan(atan(0.5, 2)) * 200px)",
	"calc(tan(atan2(50deg, 1)) * 200px)",
	"pow(4, 3)", "pow(4px, 3)",
	"sqrt(4)", "sqrt(4px)",
	"log(10)", "log(10px)", "log(10, 10)", "log(10px, 10)",
	"exp(10)", "exp(10px)",
	"abs(10)",
	"sign(10)", "sign(10px)",
}

func TestMathFunctionsInvalid(t *testing.T) {
	for _, expr := range append(invalidMathExpressions, invalidMathTypeExpressions...) {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			assertInvalid(t, "width: "+expr, "Ignored")
		})
	}
}

// --- Direct evaluator tests ---------------------------------------------

// evalForTest parses the given math expression (must be a single
// FunctionBlock) and evaluates it against ctx with the requested shape.
func evalForTest(t *testing.T, expr string, shape pr.MathType, ctx MathContext) float32 {
	t.Helper()
	tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(expr), false))
	if len(tokens) != 1 {
		t.Fatalf("expected exactly one token in %q, got %d", expr, len(tokens))
	}
	pm, err := parseMath(tokens[0], shape, "")
	if err != nil {
		t.Fatalf("parseMath(%q): %v", expr, err)
	}
	v, err := EvalMath(pm, ctx)
	if err != nil {
		t.Fatalf("EvalMath(%q): %v", expr, err)
	}
	return float32(v)
}

func near(a, b, eps float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}

// TestMathEvaluatorWidth100 asserts that each WeasyPrint-spec expression
// that the original test parametrises to width=100px actually evaluates
// to 100 pixels under the matching layout context (font-size: 10px,
// containing-block width: 200px).
func TestMathEvaluatorWidth100(t *testing.T) {
	ctx := MathContext{
		FontSize:            10,
		RootFontSize:        10,
		PercentageReference: 200,
		HasPercentageRef:    true,
	}

	cases := []struct{ expr string }{
		{"calc(100px)"},
		{"calc(10em)"},
		{"calc(50%)"},
		{"calc(10px + 90px)"},
		{"calc(5em + 50px)"},
		{"calc(2 * 5em)"},
		{"calc(2 * (3em + 20px))"},
		{"calc(25% * (1 + 1))"},
		{"calc(20% * (1 + 1) + 20px)"},
		{"max(100px)"},
		{"min(100px)"},
		{"calc(sqrt(4) * 50px)"},
		{"calc(pow(2, 2) * 25px)"},
		{"calc(hypot(2) * 50px)"},
		{"calc(hypot(3, 4) * 20px)"},
		{"calc(log(e) * 100px)"},
		{"calc(log(100, 10) * 50px)"},
		{"abs(-100px)"},
		{"calc(abs(-100) * 1px)"},
		{"calc(sign(-100) * -100px)"},
		{"calc(sign(-100px) * -100px)"},
		{"clamp(50px, 10em, 500px)"},
		{"round(100.4px)"},
		{"calc(sin(30deg) * 200px)"},
		{"calc(cos(60deg) * 200px)"},
		{"calc(tan(45deg) * 100px)"},
		{"calc(sin(asin(0.5)) * 200px)"},
		{"calc(cos(acos(0.5)) * 200px)"},
		{"calc(tan(atan(1)) * 100px)"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.expr, func(t *testing.T) {
			defer tu.CaptureLogs(t).AssertNoLogs(t)
			got := evalForTest(t, c.expr, pr.MathLengthPercentage, ctx)
			if !near(got, 100, 0.01) {
				t.Fatalf("%s: expected 100, got %g", c.expr, got)
			}
		})
	}
}

// TestMathEvaluatorAngles checks angle-shape expressions resolve to
// recognisable radian values.
func TestMathEvaluatorAngles(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)
	ctx := MathContext{}
	got := evalForTest(t, "calc(180deg)", pr.MathAngle, ctx)
	if !near(got, float32(math.Pi), 1e-5) {
		t.Fatalf("calc(180deg): expected π, got %g", got)
	}
}

// TestMathEvaluatorDivByZero checks that `calc(1px / 0)` follows CSS
// Values 4: the division yields +∞, which a top-level calculation clamps
// to a finite value instead of rejecting the expression.
func TestMathEvaluatorDivByZero(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)
	tokens := parser.RemoveWhitespace(parser.Tokenize([]byte("calc(1px / 0)"), false))
	pm, err := parseMath(tokens[0], pr.MathLength, "")
	if err != nil {
		t.Fatalf("parseMath: %v", err)
	}
	v, err := EvalMath(pm, MathContext{})
	if err != nil {
		t.Fatalf("EvalMath: %v", err)
	}
	if v != limits.Length {
		t.Fatalf("expected the clamped infinity %d, got %g", limits.Length, v)
	}
}

// TestDefiniteLength covers the distinction layout relies on when the
// containing block is indefinite: a percentage-free expression is fully
// determined and must resolve, one carrying a percentage behaves as
// `auto`. Neither may panic — see webrender docs/viewport-units-plan.md §1.1.
func TestDefiniteLength(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)

	for _, c := range []struct {
		expr string
		want pr.Float
		ok   bool
	}{
		{"calc(100px * 2)", 200, true},
		{"calc(100px + 100px)", 200, true},
		{"min(200px, 300px)", 200, true},
		{"clamp(50px, 200px, 300px)", 200, true},
		{"calc(10em)", 160, true},
		// Still depends on a percentage reference: behaves as auto.
		{"calc(50% + 10px)", 0, false},
		{"calc(50%)", 0, false},
		{"min(50%, 300px)", 0, false},
	} {
		tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(c.expr), false))
		pm, err := parseMath(tokens[0], pr.MathLengthPercentage, "layout")
		if err != nil {
			t.Fatalf("parseMath %s: %v", c.expr, err)
		}
		pm.FontSize, pm.RootFontSize = 16, 16
		got, ok := pr.Dimension{Math: &pm}.Tagged().IsDefinite()
		if ok != c.ok {
			t.Fatalf("%s: expected definite=%v, got %v (value %g)", c.expr, c.ok, ok, got)
		}
		if ok && !near(float32(got), float32(c.want), 0.01) {
			t.Fatalf("%s: expected %g, got %g", c.expr, c.want, got)
		}
	}

	// Plain computed values go through the same entry point.
	for _, c := range []struct {
		dim  pr.TaggedDim
		want pr.Float
		ok   bool
	}{
		{pr.NewDim(120, pr.Px).Tagged(), 120, true},
		{pr.NewDim(50, pr.Perc).Tagged(), 0, false},
		{pr.TaggedDim{Tag: pr.Auto}, 0, false},
		{pr.TaggedDim{}, 0, false},
	} {
		got, ok := c.dim.IsDefinite()
		tu.AssertEqual(t, ok, c.ok)
		tu.AssertEqual(t, got, c.want)
	}
}

// TestLayoutDeferredMath exercises the full layout-time path: a math
// value carrying a percentage whose reference is the containing-block
// width is parsed, baked with a font-size context, then resolved
// against a runtime referTo via [pr.ResolvePercentage].
//
// `calc(50% + 10px)` against a 200px containing block should yield
// 110px. `calc(50% + 1em)` against the same block with font-size 20
// should yield 120px (the em is baked in at compute time, the
// percentage is resolved at layout time).
func TestLayoutDeferredMath(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)

	cases := []struct {
		expr     string
		fontSize float32
		referTo  pr.Float
		want     pr.Float
	}{
		{"calc(50% + 10px)", 16, 200, 110},
		{"calc(50% + 1em)", 20, 200, 120},
		{"calc(100% - 20px)", 16, 50, 30},
	}
	for _, c := range cases {
		c := c
		t.Run(c.expr, func(t *testing.T) {
			tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(c.expr), false))
			pm, err := parseMath(tokens[0], pr.MathLengthPercentage, "layout")
			if err != nil {
				t.Fatalf("parseMath: %v", err)
			}
			pm.FontSize = c.fontSize
			pm.RootFontSize = c.fontSize
			value := pr.Dimension{Math: &pm}.Tagged()

			got := pr.ResolvePercentage(value, c.referTo)
			f, ok := got.(pr.Float)
			if !ok {
				t.Fatalf("expected pr.Float, got %T", got)
			}
			if !near(float32(f), float32(c.want), 0.01) {
				t.Fatalf("%s: expected %g, got %g", c.expr, c.want, f)
			}
		})
	}
}

// VP6, VP10: the type of a math expression is checked against the property
// at parse time: a <number> is not a <length>, and a product of lengths has
// no valid type.
func TestMathTypeChecked(t *testing.T) {
	for _, css := range []string{
		"font-size: calc(20)", "border-top-width: calc(5)", "letter-spacing: calc(3)",
		"width: calc(2 * 3)", "margin-left: min(1, 2)",
		"line-height: calc(1px * 2px)", "width: calc(10px * 2px)", "opacity: calc(1px)",
	} {
		t.Run(css, func(t *testing.T) { assertInvalid(t, css, "Ignored") })
	}
	for _, css := range []string{
		"line-height: calc(2)", "line-height: calc(2px)", "tab-size: calc(4)", "tab-size: calc(4px)",
		"width: calc(10px * 2)", "opacity: calc(0.5)", "z-index: calc(2)",
		// VP11: sign() and atan2() take any type
		"opacity: sign(10px)", "line-height: sign(1em)",
	} {
		t.Run(css, func(t *testing.T) {
			if d := expandToDict(t, css, ""); len(d) != 1 {
				t.Errorf("%s: %v", css, d)
			}
		})
	}
	// in shorthands, the type decides the longhand: a number is the count
	// (columns) or the grow factor (flex), not a length
	d := expandToDict(t, "columns: calc(3)", "")
	if _, isMath := d[pr.PColumnCount].(pr.PendingMath); !isMath && d[pr.PColumnCount] == nil {
		t.Errorf("columns: calc(3): %v", d)
	}
	if w, ok := d[pr.PColumnWidth].(pr.TaggedDim); ok && w.Tag != pr.Auto {
		t.Errorf("columns: calc(3) set column-width: %v", d)
	}
	if got := d[pr.PColumnCount]; got != (pr.IntString{Int: 3}) {
		t.Errorf("columns: calc(3): column-count %v", got)
	}
	d = expandToDict(t, "flex: calc(2)", "")
	if d[pr.PFlexGrow] != pr.Float(2) {
		t.Errorf("flex: calc(2): %v", d)
	}
	assertInvalid(t, "flex: -1", "Ignored")
	if basis, ok := d[pr.PFlexBasis].(pr.TaggedDim); ok && basis.Dimension.Math != nil {
		t.Errorf("flex: calc(2) set flex-basis to %v", basis)
	}
}

// VP8: round(up|down) with a negative step picks the upper or lower
// multiple, whatever the sign of the step.
func TestRoundNegativeStep(t *testing.T) {
	for expr, want := range map[string]float32{
		"round(down, 5px, -2px)":     4,
		"round(up, 5px, -2px)":       6,
		"round(up, -5px, -2px)":      -4,
		"round(down, -5px, -2px)":    -6,
		"round(to-zero, -5px, -2px)": -4,
		"round(nearest, 5px, -2px)":  6,
		"round(down, 5px, 2px)":      4,
		"round(up, 5px, 2px)":        6,
	} {
		if got := evalForTest(t, expr, pr.MathLength, MathContext{FontSize: 16}); got != want {
			t.Errorf("%s = %g, want %g", expr, got, want)
		}
	}
}

// V2-5, V2-6, V2-7.
func TestMathSecondReview(t *testing.T) {
	ctx := MathContext{FontSize: 16, RootFontSize: 16}
	for expr, want := range map[string]float32{
		"calc(tan(90deg) * 1px)":  limits.Length,
		"calc(tan(-90deg) * 1px)": -limits.Length,
		"calc(tan(450deg) * 1px)": limits.Length,
		"calc(tan(pi / 2) * 1px)": limits.Length,
		"calc(tan(45deg) * 1px)":  1,
		"calc(10px / 2px * 1em)":  80,
		"calc(1em / 1px * 1px)":   16,
		"calc(sign(-10px) * 3px)": -3,
	} {
		if got := evalForTest(t, expr, pr.MathLength, ctx); math.Abs(float64(got-want)) > 1e-3 {
			t.Errorf("%s = %g, want %g", expr, got, want)
		}
	}
	if got := evalForTest(t, "calc(50% / 25% * 1px)", pr.MathLengthPercentage, ctx); got != 2 {
		t.Errorf("calc(50%% / 25%% * 1px) = %g, want 2", got)
	}
	// sign() arguments mixing lengths and percentages, in a <length>
	// property as in a <length-percentage> one
	for _, css := range []string{"width: calc(10px * sign(min(10%, 1px)))", "border-top-width: calc(sign(1em - 1px) * 1px)"} {
		if d := expandToDict(t, css, ""); len(d) == 0 {
			t.Errorf("%s rejected", css)
		}
	}
	// no percentage basis: a percentage is invalid, in sign() too
	assertInvalid(t, "border-top-width: calc(sign(10% + 1px) * 1px)", "Ignored")
	assertInvalid(t, "z-index: sign(10%)", "Ignored")
	assertInvalid(t, "width: calc(1px / 1px)", "Ignored") // a <number>
	assertInvalid(t, "width: calc(1px / 1deg)", "Ignored")
}
