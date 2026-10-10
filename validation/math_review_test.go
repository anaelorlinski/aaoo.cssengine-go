package validation

import (
	"errors"
	"math"
	"strings"
	"testing"

	tu "github.com/anaelorlinski/aaoo.cssengine-go/internal/testutils"
	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Regression tests for the validation review findings (webrender docs/css-review-bugs.md) (V1–V15).

// tryEval parses expr (a single math function), shape-checks it and
// evaluates it, returning the evaluator error instead of failing.
func tryEval(t *testing.T, expr string, shape pr.MathType, ctx MathContext) (float32, error) {
	t.Helper()
	tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(expr), false))
	if len(tokens) != 1 {
		t.Fatalf("expected exactly one token in %q, got %d", expr, len(tokens))
	}
	pm, err := parseMath(tokens[0], shape, "")
	if err != nil {
		return 0, err
	}
	return EvalMath(pm, ctx)
}

// rawEval evaluates expr without the checkMath shape check, as the
// relative-colour code path does.
func rawEval(expr string, shape pr.MathType, ctx MathContext) (float32, error) {
	tokens := parser.RemoveWhitespace(parser.Tokenize([]byte(expr), false))
	fb, _ := tokens[0].(parser.FunctionBlock)
	return EvalMath(pr.PendingMath{Expr: fb, ResolveTo: shape}, ctx)
}

// V1: min/max/clamp/hypot (and the stepped functions) must unify
// lengths and percentages against the percentage reference.
func TestMathMixedLengthPercentage(t *testing.T) {
	defer tu.CaptureLogs(t).AssertNoLogs(t)
	ctx := MathContext{FontSize: 16, RootFontSize: 16, PercentageReference: 1000, HasPercentageRef: true}
	for _, c := range []struct {
		expr string
		want float32
	}{
		{"min(100%, 600px)", 600},
		{"min(50%, 600px)", 500},
		{"max(50%, 10px)", 500},
		{"max(10px, 50%)", 500},
		{"clamp(10px, 50%, 600px)", 500},
		{"clamp(10px, 80%, 600px)", 600},
		{"clamp(10%, 5px, 600px)", 100},
		{"hypot(30%, 400px)", 500},
		{"calc(min(10%, 1em) * 2)", 32},
		{"round(55%, 100px)", 600},
		{"mod(55%, 100px)", 50},
		{"rem(55%, 100px)", 50},
		{"min(100%, 20em, 100px)", 100},
	} {
		got, err := tryEval(t, c.expr, pr.MathLengthPercentage, ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if !near(float32(got), float32(c.want), 0.01) {
			t.Fatalf("%s: expected %g, got %g", c.expr, c.want, got)
		}
	}

	// Without a reference the evaluation must report ErrMathNeedPercRef
	// so layout can defer it.
	for _, expr := range []string{"min(100%, 600px)", "clamp(10px, 50%, 600px)", "hypot(3%, 4px)"} {
		_, err := tryEval(t, expr, pr.MathLengthPercentage, MathContext{})
		if !errors.Is(err, ErrMathNeedPercRef) {
			t.Fatalf("%s: expected ErrMathNeedPercRef, got %v", expr, err)
		}
	}

	// End to end: the layout-deferred path used for `width`.
	d := expandToDict(t, "width: min(100%, 600px)", "")
	pm, ok := d[pr.PWidth].(pr.PendingMath)
	if !ok {
		t.Fatalf("expected a PendingMath for width, got %T", d[pr.PWidth])
	}
	pm.FontSize, pm.RootFontSize = 16, 16
	got := pr.ResolvePercentage(pr.Dimension{Math: &pm}.Tagged(), 1000)
	if f, ok := got.(pr.Float); !ok || !near(float32(f), 600, 0.01) {
		t.Fatalf("width: min(100%%, 600px) in a 1000px block: expected 600, got %v", got)
	}
}

// V2: the evaluator must not trust its caller to have run checkMath.
func TestMathEvaluatorArityGuards(t *testing.T) {
	for _, expr := range []string{
		"clamp(1)", "clamp(1, 2)", "round(up)", "round()", "mod(1)", "rem(1)",
		"atan2(1)", "pow(2)", "calc(1, 2)", "sin()", "log(1, 2, 3)",
		"round(nearest, 1, 2, 3)", "min()", "hypot()",
	} {
		if _, err := rawEval(expr, pr.MathNumber, MathContext{}); err == nil {
			t.Fatalf("%s: expected an arity error", expr)
		}
	}
	// Through the relative-colour / colour resolution path.
	for _, css := range []string{
		"color: rgb(clamp(1) 0 0)",
		"background: round(up)",
		"color: rgb(from red clamp(1) g b)",
		"color: rgb(from red round(up) g b)",
	} {
		// Only the absence of a panic is asserted here: whether the
		// colour is then rejected is up to the colour parser.
		capt := tu.CaptureLogs(t)
		PreprocessDeclarations("", parser.ParseDeclarationListString(css, false, false))
		_ = capt.Logs()
	}
}

// V3: font shorthand ending with a bare slash.
func TestFontShorthandTrailingSlash(t *testing.T) {
	for _, css := range []string{"font: 12px/", "font: bold 12px/", "font: 12px /"} {
		assertInvalid(t, css, "invalid")
	}
}

// nestedFunctions builds name(name(…(inner)…)) as a token tree, without
// going through the tokenizer (whose nesting limit is not under test).
func nestedFunctions(name string, depth int, inner []parser.Token, wrap func(parser.FunctionBlock) []parser.Token) parser.FunctionBlock {
	fb := parser.NewFunctionBlock(parser.Pos{}, name, inner)
	for i := 1; i < depth; i++ {
		args := []parser.Token{fb}
		if wrap != nil {
			args = wrap(fb)
		}
		fb = parser.NewFunctionBlock(parser.Pos{}, name, args)
	}
	return fb
}

// V4: a failing argument of sin/cos/tan used to be evaluated twice,
// doubling the work at every nesting level (depth 30 took minutes).
// At depth 60 the old code would never finish.
func TestMathTrigNotExponential(t *testing.T) {
	tokens := parser.Tokenize([]byte("1px+1"), false)
	fb := nestedFunctions("sin", 60, tokens, nil)
	pm := pr.PendingMath{Expr: fb, ResolveTo: pr.MathNumber}
	if _, err := EvalMath(pm, MathContext{}); err == nil {
		t.Fatal("expected an error for sin(…(1px+1)…)")
	}
	// A valid deep chain still evaluates.
	fb = nestedFunctions("sin", 60, parser.Tokenize([]byte("30deg"), false), nil)
	if _, err := EvalMath(pr.PendingMath{Expr: fb, ResolveTo: pr.MathNumber}, MathContext{}); err != nil {
		t.Fatal(err)
	}
}

// V5: HasVar used to re-parse the whole subtree at every node, which
// is quadratic in the nesting depth (10,000 levels took 12 s).
func TestHasVarLinear(t *testing.T) {
	varFn := parser.NewFunctionBlock(parser.Pos{}, "var", parser.Tokenize([]byte("--x"), false))
	deep := nestedFunctions("calc", 200000, []parser.Token{varFn}, nil)
	if !HasVar(deep) {
		t.Fatal("expected var() to be found at depth 200000")
	}
	deep = nestedFunctions("calc", 200000, parser.Tokenize([]byte("1px"), false), nil)
	if HasVar(deep) {
		t.Fatal("unexpected var()")
	}
	// Detection semantics.
	for _, c := range []struct {
		css  string
		want bool
	}{
		{"var(--a)", true},
		{"VAR(--a)", true},
		{"var( --a )", true},
		{"var(--a,)", true},
		{"var(--a, 1px)", true},
		{"var(a)", false},
		{"var()", false},
		{"calc(1px + var(--a))", true},
		{"[var(--a)]", true},
		{"foo(1)", false},
	} {
		toks := parser.RemoveWhitespace(parser.Tokenize([]byte(c.css), false))
		if got := HasVar(toks[0]); got != c.want {
			t.Fatalf("HasVar(%s) = %v, want %v", c.css, got, c.want)
		}
	}
}

// Depth limit: absurdly nested math is rejected by checkMath and by the
// evaluator rather than recursing without bound.
func TestMathDepthLimit(t *testing.T) {
	fb := nestedFunctions("calc", 10000, parser.Tokenize([]byte("1px"), false), nil)
	if _, err := parseMath(fb, pr.MathLength, ""); err == nil {
		t.Fatal("expected checkMath to reject 10000 nested calc()")
	}
	if _, err := EvalMath(pr.PendingMath{Expr: fb, ResolveTo: pr.MathLength}, MathContext{}); err == nil {
		t.Fatal("expected the evaluator to reject 10000 nested calc()")
	}
	paren := nestedFunctions("calc", 1, parser.Tokenize([]byte(strings.Repeat("(", 10000)+"1px"+strings.Repeat(")", 10000)), false), nil)
	if _, err := EvalMath(pr.PendingMath{Expr: paren, ResolveTo: pr.MathLength}, MathContext{}); err == nil {
		t.Fatal("expected the evaluator to reject 10000 nested parentheses")
	}
	// Reasonable nesting is fine.
	fb = nestedFunctions("calc", 30, parser.Tokenize([]byte("1px"), false), nil)
	if _, err := parseMath(fb, pr.MathLength, ""); err != nil {
		t.Fatal(err)
	}
}

// V6: NaN → 0 and ±∞ clamped to a finite value at the top level.
func TestMathNonFiniteSanitised(t *testing.T) {
	ctx := MathContext{FontSize: 16, PercentageReference: 100, HasPercentageRef: true}
	finite := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
	for _, c := range []struct {
		expr string
		sign int // expected sign of the clamped result; 0 means exactly 0
	}{
		{"calc(infinity * 1px)", 1},
		{"calc(-infinity * 1px)", -1},
		{"calc(nan * 1px)", 0},
		{"calc(log(8, 1) * 1px)", 1},
		{"mod(-7px, calc(infinity * 1px))", 0},
		{"calc(1px / 0)", 1},
		{"calc(-1px / 0)", -1},
		{"calc(0px / 0)", 0},
		{"calc(sqrt(-1) * 1px)", 0},
		{"calc(infinity * 1%)", 1},
	} {
		got, err := tryEval(t, c.expr, pr.MathLengthPercentage, ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if !finite(got) {
			t.Fatalf("%s: expected a finite result, got %g", c.expr, got)
		}
		switch {
		case c.sign == 0 && got != 0, c.sign > 0 && got <= 0, c.sign < 0 && got >= 0:
			t.Fatalf("%s: unexpected result %g", c.expr, got)
		}
	}
	// EvalMathDim sanitises too.
	tokens := parser.RemoveWhitespace(parser.Tokenize([]byte("calc(infinity * 1%)"), false))
	pm, _ := parseMath(tokens[0], pr.MathLengthPercentage, "")
	dim, err := EvalMathDim(pm, MathContext{})
	if err != nil || !finite(float32(dim.Value)) || dim.Unit != pr.Perc {
		t.Fatalf("EvalMathDim: %v %v", dim, err)
	}
	// Infinity still works inside the expression.
	got, err := tryEval(t, "clamp(calc(-infinity * 1px), 10em, calc(infinity * 1px))", pr.MathLength, ctx)
	if err != nil || got != 160 {
		t.Fatalf("clamp with infinite bounds: %g %v", got, err)
	}
	got, err = tryEval(t, "calc(1px / infinity)", pr.MathLength, ctx)
	if err != nil || got != 0 {
		t.Fatalf("1px / infinity: %g %v", got, err)
	}
}

// V7: NaN in any argument of min/max/clamp makes the result NaN (→ 0).
func TestMathNaNPropagates(t *testing.T) {
	ctx := MathContext{}
	for _, expr := range []string{
		"min(5px, calc(nan * 1px))", "min(calc(nan * 1px), 5px)",
		"max(5px, calc(nan * 1px))", "max(calc(nan * 1px), 5px)",
		"clamp(calc(nan * 1px), 5px, 10px)", "clamp(1px, calc(nan * 1px), 10px)",
		"clamp(1px, 5px, calc(nan * 1px))", "calc(min(nan, 2) * 7px)",
		"hypot(calc(nan * 1px), 5px)",
	} {
		got, err := tryEval(t, expr, pr.MathLength, ctx)
		if err != nil || got != 0 {
			t.Fatalf("%s: expected 0 (sanitised NaN), got %g, %v", expr, got, err)
		}
	}
	// clamp(): MIN wins over MAX.
	got, err := tryEval(t, "clamp(100px, 50px, 10px)", pr.MathLength, ctx)
	if err != nil || got != 100 {
		t.Fatalf("clamp(100px, 50px, 10px): expected 100, got %g %v", got, err)
	}
}

// V8: number-only functions reject dimensions; trig accepts number or angle.
func TestMathNumberOnlyFunctions(t *testing.T) {
	ctx := MathContext{FontSize: 16}
	for _, expr := range []string{
		"calc(sqrt(4px) * 1px)", "calc(pow(2px, 3) * 1px)", "calc(pow(2, 3px) * 1px)",
		"calc(asin(30deg) * 1px)", "calc(sin(90px) * 1px)", "calc(exp(1px) * 1px)",
		"calc(log(1px) * 1px)", "calc(log(2, 1px) * 1px)", "calc(acos(1px) * 1px)",
		"calc(atan(1deg) * 1px)", "calc(tan(1px) * 1px)", "calc(atan2(1px, 1) * 1px)",
	} {
		if _, err := rawEval(expr, pr.MathLength, ctx); err == nil {
			t.Fatalf("%s: expected a type error", expr)
		}
	}
	for _, c := range []struct {
		expr string
		want float32
	}{
		{"calc(sin(90deg) * 1px)", 1},
		{"calc(sin(calc(pi / 2)) * 1px)", 1},
		{"calc(sqrt(4) * 1px)", 2},
		{"calc(tan(atan2(1px, 1px)) * 1px)", 1},
		{"calc(tan(atan2(1em, 16px)) * 1px)", 1},
	} {
		got, err := rawEval(c.expr, pr.MathLength, ctx)
		if err != nil || !near(float32(got), float32(c.want), 1e-5) {
			t.Fatalf("%s: expected %g, got %g %v", c.expr, c.want, got, err)
		}
	}
}

// V9: round(nearest) breaks ties toward +∞; unit mismatches are type
// errors, not division by zero.
func TestMathRound(t *testing.T) {
	ctx := MathContext{}
	for _, c := range []struct {
		expr string
		want float32
	}{
		{"round(2.5px, 1px)", 3},
		{"round(-2.5px, 1px)", -2},
		{"round(3.5px, 1px)", 4},
		{"round(2.4px, 1px)", 2},
		{"round(nearest, 150px, 100px)", 200},
		{"round(-150px, 100px)", -100},
		{"round(up, 101px, 100px)", 200},
		{"round(down, -101px, 100px)", -200},
		{"round(to-zero, -199px, 100px)", -100},
		{"round(5px, calc(infinity * 1px))", 0},
		{"round(up, 5px, calc(infinity * 1px))", math.MaxFloat32},
	} {
		got, err := tryEval(t, c.expr, pr.MathLength, ctx)
		if c.want == math.MaxFloat32 {
			if err != nil || got <= 1e6 {
				t.Fatalf("%s: expected a clamped +infinity, got %g %v", c.expr, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("%s: expected %g, got %g %v", c.expr, c.want, got, err)
		}
	}
	_, err := rawEval("round(5px, 1)", pr.MathLength, ctx)
	if !errors.Is(err, errMathTypeMismatch) {
		t.Fatalf("round(5px, 1): expected a type mismatch, got %v", err)
	}
	_, err = rawEval("mod(5px, 1deg)", pr.MathLength, ctx)
	if !errors.Is(err, errMathTypeMismatch) {
		t.Fatalf("mod(5px, 1deg): expected a type mismatch, got %v", err)
	}
	// round(A, 0) and mod(A, 0) are NaN, which sanitises to 0.
	for _, expr := range []string{"round(5px, 0px)", "mod(5px, 0px)", "rem(5px, 0px)"} {
		got, err := tryEval(t, expr, pr.MathLength, ctx)
		if err != nil || got != 0 {
			t.Fatalf("%s: expected 0, got %g %v", expr, got, err)
		}
	}
}

// V10: a nested calc() inside a trig function may contain angles.
func TestMathNestedAngleInTrig(t *testing.T) {
	for _, css := range []string{
		"width: calc(sin(calc(30deg)) * 1px)",
		"width: calc(cos((60deg)) * 200px)",
		"width: calc(tan(min(45deg, 1rad)) * 1px)",
		"width: calc(sin(calc(10deg + 20deg)) * 1px)",
	} {
		d := expandToDict(t, css, "")
		if len(d) == 0 {
			t.Fatalf("%s rejected", css)
		}
	}
	got := evalForTest(t, "calc(sin(calc(30deg)) * 200px)", pr.MathLength, MathContext{})
	if !near(got, 100, 0.01) {
		t.Fatalf("expected 100, got %g", got)
	}
	// Angles are still rejected outside trig.
	assertInvalid(t, "width: calc(min(30deg, 1px))", "Ignored")
}

// V11: `infinite` in first position of a counter-style range is -∞.
func TestCounterStyleRangeInfinite(t *testing.T) {
	toks := func(s string) []Token { return parser.RemoveWhitespace(parser.Tokenize([]byte(s), false)) }
	got, err := range_(toks("infinite 10"))
	if err != nil || got != [2]int{math.MinInt32, 10} {
		t.Fatalf("infinite 10: %v %v", got, err)
	}
	got, err = range_(toks("2 infinite"))
	if err != nil || got != [2]int{2, math.MaxInt32} {
		t.Fatalf("2 infinite: %v %v", got, err)
	}
	got, err = range_(toks("infinite infinite"))
	if err != nil || got != [2]int{math.MinInt32, math.MaxInt32} {
		t.Fatalf("infinite infinite: %v %v", got, err)
	}
}

// V12: whitespace is required around + and -; unary operators on
// parentheses are not part of the grammar.
func TestMathGrammarStrict(t *testing.T) {
	for _, css := range []string{
		"width: calc(1px +(2px))",
		"width: calc(1px+ 2px)",
		"width: calc(1px -(2px))",
		"width: calc(-(2px))",
		"width: calc(+ 2px)",
		"width: calc(1px + 2px -)",
	} {
		assertInvalid(t, css, "Ignored")
	}
	for _, expr := range []string{"calc(1px +(2px))", "calc(-(2px))", "calc(1px - -(2px))"} {
		if _, err := rawEval(expr, pr.MathLength, MathContext{}); err == nil {
			t.Fatalf("%s: expected a syntax error", expr)
		}
	}
	for _, c := range []struct {
		expr string
		want float32
	}{
		{"calc(1px + (2px))", 3},
		{"calc(1px - -2px)", 3},
		{"calc((1px + 2px) * 2)", 6},
		{"calc(1px*2 + 1px/1)", 3},
		{"calc(\n1px\t-\n2px)", -1},
	} {
		if got := evalForTest(t, c.expr, pr.MathLength, MathContext{}); got != c.want {
			t.Fatalf("%s: expected %g, got %g", c.expr, c.want, got)
		}
	}
}

// V15: resolution values do not share the length slot.
func TestMathResolutionSlot(t *testing.T) {
	if _, err := rawEval("calc(1dppx + 1px)", pr.MathLength, MathContext{}); err == nil {
		t.Fatal("calc(1dppx + 1px) must not add a resolution to a length")
	}
	if _, err := rawEval("calc(1dppx)", pr.MathLength, MathContext{}); err == nil {
		t.Fatal("calc(1dppx) is not a length")
	}
	got, err := rawEval("calc(1dppx + 96dpi)", pr.MathResolution, MathContext{})
	if err != nil || got != 2 {
		t.Fatalf("calc(1dppx + 96dpi): expected 2dppx, got %g %v", got, err)
	}
}
