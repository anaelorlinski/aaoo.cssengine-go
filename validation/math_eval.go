package validation

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/mathhook"
	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// CSS math evaluator. Ports WeasyPrint's `_resolve_calc_*` and
// `resolve_math` from weasyprint/css/__init__.py.
//
// Strategy:
//   1. Parse the validated FunctionBlock token list with a recursive-
//      descent parser respecting CSS arithmetic precedence:
//          sum     := product (('+' | '-') product)*
//          product := value   (('*' | '/') value)*
//          value   := <number> | <dimension> | <percentage> | <ident-const>
//                  | '(' sum ')' | <math-func>
//   2. Each sub-expression resolves to a [resolvedValue] carrying a Float
//      and a Unit category. Adding mismatched categories returns an error.
//   3. The dispatcher applies the function semantics (min, max, clamp,
//      round, etc.) on top of resolved sub-expressions.

// MathContext supplies the dynamic information required to resolve a
// PendingMath at compute or layout time.
type MathContext struct {
	// FontSize is the computed `font-size` of the element in pixels.
	FontSize float32
	// RootFontSize is the document root font-size in pixels.
	RootFontSize float32
	// ExRatio / ChRatio are the 1ex / font-size and 1ch / font-size
	// ratios for the element's font; 0 falls back to the canonical 0.5/0.5.
	ExRatio float32
	ChRatio float32
	// LineHeight / RootLineHeight are the computed line heights of the
	// element and of the root, in pixels, for lh and rlh; 0 falls back to
	// 1.2 times the font size.
	LineHeight     float32
	RootLineHeight float32
	// HasLineHeight and HasRootLineHeight tell the line heights are known
	// (0 being a valid line height); else lh and rlh fall back to 1.2em.
	HasLineHeight, HasRootLineHeight bool

	// ViewportSizes carries the page box and page area rectangles that
	// viewport-relative units (vw, pvh, …) resolve against. A nil value
	// resolves them against the initial A4 page, with a warning.
	ViewportSizes *pr.ViewportSizes

	// PercentageReference, when non-zero, is the dimension (in pixels)
	// that 100% resolves to. When zero, percentages remain unresolved
	// and any percentage operand causes an error — callers that don't
	// have a reference yet must defer evaluation to layout time.
	PercentageReference float32
	HasPercentageRef    bool
}

// resolvedValue is the intermediate currency of the evaluator. The Unit
// category is one of: number, length (pixels), percentage, angle (radians),
// resolution (dppx). Lengths are normalised to pixels using the context.
type resolvedValue struct {
	V    float32
	Unit pr.Unit
}

// unitDppx is the evaluator-local unit category for resolutions. The
// properties package has no resolution unit, and reusing [pr.Px] would let
// a resolution add up with a length.
const unitDppx pr.Unit = math.MaxUint8

func (r resolvedValue) isNumber() bool     { return r.Unit == pr.Scalar }
func (r resolvedValue) isLength() bool     { return r.Unit == pr.Px }
func (r resolvedValue) isPercentage() bool { return r.Unit == pr.Perc }
func (r resolvedValue) isAngle() bool      { return r.Unit == pr.Rad }
func (r resolvedValue) isResolution() bool { return r.Unit == unitDppx }
func (r resolvedValue) isNaN() bool        { return math.IsNaN(float64(r.V)) }

var (
	errMathTypeMismatch = errors.New("incompatible operand types in math expression")
	errMathSyntax       = errors.New("invalid math expression syntax")
	errMathTooDeep      = errors.New("math expression is nested too deeply")
	// ErrMathNeedPercRef is returned by EvalMath when the expression
	// contains a percentage but the context provides no reference value.
	// Callers wishing to defer evaluation to layout time should detect
	// this error specifically (with [errors.Is]) rather than falling back.
	ErrMathNeedPercRef = errors.New("percentage in math expression has no resolution context")
)

// sanitize applies the CSS Values 4 top-level rules for non-finite
// results: NaN becomes 0, ±∞ is clamped to ±[limits.Length]. It must only
// be applied to the result of a top-level calculation, since infinities
// and NaN are meaningful inside an expression.
func sanitize(v float32) float32 {
	switch {
	case math.IsNaN(float64(v)):
		return 0
	case v > limits.Length:
		return limits.Length
	case v < -limits.Length:
		return -limits.Length
	}
	return v
}

// EvalMath evaluates pm using ctx and returns a resolved value matching
// pm.ResolveTo. Length results are in pixels, angle in radians.
func EvalMath(pm pr.PendingMath, ctx MathContext) (float32, error) {
	rv, err := evalMathFunction(pm.Expr, pm.ResolveTo, ctx, 0)
	if err != nil {
		return 0, err
	}
	v, err := castToShape(rv, pm.ResolveTo, ctx)
	if err != nil {
		return 0, err
	}
	return sanitize(v), nil
}

func init() {
	// Wire layout-time deferred math resolution back into package properties
	// (which cannot import validation due to the dependency direction).
	mathhook.Eval = func(pm any, referTo float32, hasRef bool) (float32, error) {
		return evalDeferred(pm.(*pr.PendingMath), referTo, hasRef)
	}
}

// evalDeferred completes a layout-deferred PendingMath. The font-size
// context was baked at compute time; only the percentage reference is
// supplied now. Returns pixels for length-percentage shapes.
//
// hasRef false means the caller has no percentage reference at all (an
// indefinite containing block): the evaluation then succeeds only for
// percentage-free expressions and returns [ErrMathNeedPercRef] otherwise,
// which callers read as "still behaves as auto".
func evalDeferred(pm *pr.PendingMath, referTo float32, hasRef bool) (float32, error) {
	ctx := MathContext{
		FontSize:            pm.FontSize,
		RootFontSize:        pm.RootFontSize,
		ViewportSizes:       pm.ViewportSizes,
		ExRatio:             pm.ExRatio,
		ChRatio:             pm.ChRatio,
		LineHeight:          pm.LineHeight,
		RootLineHeight:      pm.RootLineHeight,
		HasLineHeight:       pm.HasLineHeight,
		HasRootLineHeight:   pm.HasRootLineHeight,
		PercentageReference: referTo,
		HasPercentageRef:    hasRef,
	}
	return EvalMath(*pm, ctx)
}

// EvalMathDim is like EvalMath but returns a [pr.Dimension] preserving
// percentage units when the context has no percentage reference. This is
// useful for compute-time resolution: percentages may need to remain
// percentages until layout, while lengths and numbers can be reduced now.
func EvalMathDim(pm pr.PendingMath, ctx MathContext) (pr.Dimension, error) {
	rv, err := evalMathFunction(pm.Expr, pm.ResolveTo, ctx, 0)
	if err != nil {
		return pr.Dimension{}, err
	}
	v := pr.Float(sanitize(rv.V))
	switch rv.Unit {
	case pr.Px, pr.Perc, pr.Scalar, pr.Rad:
		return pr.NewDim(v, rv.Unit), nil
	}
	return pr.Dimension{}, fmt.Errorf("unexpected resolved unit %s", rv.Unit)
}

func castToShape(rv resolvedValue, shape pr.MathType, ctx MathContext) (float32, error) {
	switch shape {
	case pr.MathNumber, pr.MathInteger:
		if !rv.isNumber() {
			return 0, errMathTypeMismatch
		}
		return rv.V, nil
	case pr.MathLength:
		if !rv.isLength() {
			return 0, errMathTypeMismatch
		}
		return rv.V, nil
	case pr.MathPercentage:
		if rv.isPercentage() {
			if !ctx.HasPercentageRef {
				return 0, ErrMathNeedPercRef
			}
			return rv.V * ctx.PercentageReference / 100, nil
		}
		return 0, errMathTypeMismatch
	case pr.MathLengthPercentage:
		if rv.isLength() {
			return rv.V, nil
		}
		if rv.isPercentage() {
			if !ctx.HasPercentageRef {
				return 0, ErrMathNeedPercRef
			}
			return rv.V * ctx.PercentageReference / 100, nil
		}
		return 0, errMathTypeMismatch
	case pr.MathAngle:
		if !rv.isAngle() {
			return 0, errMathTypeMismatch
		}
		return rv.V, nil
	case pr.MathResolution:
		if !rv.isResolution() {
			return 0, errMathTypeMismatch
		}
		return rv.V, nil
	}
	return 0, fmt.Errorf("unsupported math shape %s", shape)
}

// splitMathFunction returns the lower-cased name, the round() rounding
// strategy and the comma-separated arguments of the math function fb,
// checking the argument count. It is shared by [checkMath] and the
// evaluator, which must not trust its callers to have shape-checked the
// expression (relative colours evaluate math directly).
func splitMathFunction(fb pa.FunctionBlock) (name, strategy string, args [][]pa.Token, err error) {
	name = strings.ToLower(fb.Name)
	arity, ok := mathFunctionArity[name]
	if !ok {
		return "", "", nil, errMathBadFunction
	}
	args, err = splitMathArguments(fb.Arguments)
	if err != nil {
		return "", "", nil, err
	}
	if name == "round" {
		// round() may take a leading rounding-strategy ident; strip it so
		// the remaining count is the numeric one, which is 1 or 2.
		strategy = "nearest"
		if len(args) > 0 {
			if leadKw := singleIdent(args[0]); roundingStrategies[leadKw] {
				strategy = leadKw
				args = args[1:]
			}
		}
		arity = [2]int{1, 2}
	}
	if len(args) < arity[0] || (arity[1] >= 0 && len(args) > arity[1]) {
		return "", "", nil, errMathBadArity
	}
	return name, strategy, args, nil
}

// unifyUnits brings vals, the arguments of a function or the operands of
// a sum, to a single unit category. Lengths and percentages are fused
// into pixels when shape is <length-percentage> and the context provides
// a percentage reference; without a reference [ErrMathNeedPercRef] is
// returned so callers can defer to layout. Any other mix is a type error.
func unifyUnits(vals []resolvedValue, shape pr.MathType, ctx MathContext) error {
	mixed := false
	for _, v := range vals[1:] {
		if v.Unit != vals[0].Unit {
			mixed = true
			break
		}
	}
	if !mixed {
		return nil
	}
	for _, v := range vals {
		if v.Unit != pr.Px && v.Unit != pr.Perc {
			return errMathTypeMismatch
		}
	}
	if shape != pr.MathLengthPercentage {
		return errMathTypeMismatch
	}
	if !ctx.HasPercentageRef {
		return ErrMathNeedPercRef
	}
	for i, v := range vals {
		if v.Unit == pr.Perc {
			vals[i] = resolvedValue{V: v.V * ctx.PercentageReference / 100, Unit: pr.Px}
		}
	}
	return nil
}

// unifyLengthPerc is [unifyUnits] for the two operands of a sum.
func unifyLengthPerc(a, b resolvedValue, shape pr.MathType, ctx MathContext) (resolvedValue, resolvedValue, error) {
	vals := [2]resolvedValue{a, b}
	err := unifyUnits(vals[:], shape, ctx)
	return vals[0], vals[1], err
}

func nan(unit pr.Unit) resolvedValue {
	return resolvedValue{V: float32(math.NaN()), Unit: unit}
}

// evalMathFunction evaluates the math function fb. depth is the current
// nesting level of functions and parentheses.
func evalMathFunction(fb pa.FunctionBlock, shape pr.MathType, ctx MathContext, depth int) (resolvedValue, error) {
	if depth > limits.MathDepth {
		return resolvedValue{}, errMathTooDeep
	}
	name, strategy, args, err := splitMathFunction(fb)
	if err != nil {
		return resolvedValue{}, err
	}
	// Every argument is evaluated exactly once: the function semantics
	// below only inspect the resolved values.
	// sign() and atan2() take any type, whatever the type of the result
	// (checkMath checks their arguments the same way)
	argShape := shape
	if name == "sign" || name == "atan2" {
		argShape = signArgumentShape(shape)
	}
	vals := make([]resolvedValue, len(args))
	for i, arg := range args {
		if vals[i], err = evalSum(arg, argShape, ctx, depth); err != nil {
			return resolvedValue{}, err
		}
	}

	switch name {
	case "calc":
		return vals[0], nil

	case "min", "max", "hypot":
		if err := unifyUnits(vals, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		out := vals[0]
		if name == "hypot" {
			sum := 0.0
			for _, v := range vals {
				sum += float64(v.V) * float64(v.V)
			}
			out.V = float32(math.Sqrt(sum))
			return out, nil
		}
		for _, v := range vals {
			// NaN in any argument makes the result NaN, whatever its
			// position.
			if v.isNaN() {
				return v, nil
			}
			if (name == "min" && v.V < out.V) || (name == "max" && v.V > out.V) {
				out = v
			}
		}
		return out, nil

	case "clamp":
		if err := unifyUnits(vals, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		lo, val, hi := vals[0], vals[1], vals[2]
		if lo.isNaN() || val.isNaN() || hi.isNaN() {
			return nan(val.Unit), nil
		}
		// clamp(MIN, VAL, MAX) is max(MIN, min(VAL, MAX)): MIN wins
		// when it exceeds MAX.
		val.V = max(lo.V, min(val.V, hi.V))
		return val, nil

	case "round":
		if len(vals) == 1 {
			// WeasyPrint accepts round(A) for any type of A, with a B
			// of 1 in A's unit.
			vals = append(vals, resolvedValue{V: 1, Unit: vals[0].Unit})
		}
		if err := unifyUnits(vals, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		a, b := vals[0], vals[1]
		a.V = roundStep(strategy, float64(a.V), float64(b.V))
		return a, nil

	case "mod", "rem":
		if err := unifyUnits(vals, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		a, b := vals[0], vals[1]
		a.V = modRem(name == "mod", float64(a.V), float64(b.V))
		return a, nil

	case "sin", "cos", "tan":
		// Numbers are accepted and treated as radians.
		a := vals[0]
		if !a.isAngle() && !a.isNumber() {
			return resolvedValue{}, errMathTypeMismatch
		}
		var v float64
		switch name {
		case "sin":
			v = math.Sin(float64(a.V))
		case "cos":
			v = math.Cos(float64(a.V))
		case "tan":
			v = math.Tan(float64(a.V))
			// the asymptotes are infinite (CSS Values 4 §10.5): tan(90deg)
			// is +∞, tan(-90deg) −∞, which the rounding of 90deg in
			// radians would lose (it gives a huge finite value, of either
			// sign)
			deg := math.Mod(float64(a.V)*180/math.Pi, 360)
			if deg < 0 {
				deg += 360
			}
			switch {
			case math.Abs(deg-90) < 1e-4:
				v = math.Inf(1)
			case math.Abs(deg-270) < 1e-4:
				v = math.Inf(-1)
			}
		}
		return resolvedValue{V: float32(v), Unit: pr.Scalar}, nil

	case "atan2":
		// atan2(A, B) accepts any two values of the same type.
		if err := unifyUnits(vals, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		v := math.Atan2(float64(vals[0].V), float64(vals[1].V))
		return resolvedValue{V: float32(v), Unit: pr.Rad}, nil

	case "asin", "acos", "atan", "pow", "sqrt", "log", "exp":
		// Number-only functions.
		for _, v := range vals {
			if !v.isNumber() {
				return resolvedValue{}, errMathTypeMismatch
			}
		}
		a := float64(vals[0].V)
		var v float64
		unit := pr.Scalar
		switch name {
		case "asin":
			v, unit = math.Asin(a), pr.Rad
		case "acos":
			v, unit = math.Acos(a), pr.Rad
		case "atan":
			v, unit = math.Atan(a), pr.Rad
		case "pow":
			v = math.Pow(a, float64(vals[1].V))
		case "sqrt":
			v = math.Sqrt(a)
		case "log":
			v = math.Log(a)
			if len(vals) == 2 {
				v /= math.Log(float64(vals[1].V))
			}
		case "exp":
			v = math.Exp(a)
		}
		return resolvedValue{V: float32(v), Unit: unit}, nil

	case "abs":
		a := vals[0]
		a.V = float32(math.Abs(float64(a.V)))
		return a, nil

	case "sign":
		a := vals[0]
		switch {
		case a.isNaN():
			return nan(pr.Scalar), nil
		case a.V > 0:
			return resolvedValue{V: 1, Unit: pr.Scalar}, nil
		case a.V < 0:
			return resolvedValue{V: -1, Unit: pr.Scalar}, nil
		}
		return resolvedValue{V: 0, Unit: pr.Scalar}, nil
	}

	return resolvedValue{}, fmt.Errorf("unsupported math function %s", name)
}

// roundStep implements round(strategy, A, B) per CSS Values 4 §10.3,
// including the infinite and zero step cases.
func roundStep(strategy string, a, b float64) float32 {
	switch {
	case b == 0, math.IsNaN(a), math.IsNaN(b), math.IsInf(a, 0) && math.IsInf(b, 0):
		return float32(math.NaN())
	case math.IsInf(a, 0):
		return float32(a)
	case math.IsInf(b, 0):
		// Lower multiple 0, upper multiple ±∞.
		switch {
		case strategy == "up" && a > 0:
			return float32(math.Inf(1))
		case strategy == "down" && a < 0:
			return float32(math.Inf(-1))
		}
		return float32(math.Copysign(0, a))
	}
	// the multiples of b around a: with a negative b, Floor(a/b)*b is the
	// upper one
	q := a / b
	lower, upper := math.Floor(q)*b, math.Ceil(q)*b
	if lower > upper {
		lower, upper = upper, lower
	}
	switch strategy {
	case "up":
		return float32(upper)
	case "down":
		return float32(lower)
	case "to-zero":
		return float32(math.Trunc(q) * b)
	default: // nearest: ties go toward +∞
		if a-lower < upper-a {
			return float32(lower)
		}
		return float32(upper)
	}
}

// modRem implements mod() (sign of the divisor) and rem() (sign of the
// dividend) per CSS Values 4 §10.3.
func modRem(isMod bool, a, b float64) float32 {
	if b == 0 || math.IsInf(a, 0) || math.IsNaN(a) || math.IsNaN(b) {
		return float32(math.NaN())
	}
	if math.IsInf(b, 0) {
		if isMod && a != 0 && math.Signbit(a) != math.Signbit(b) {
			return float32(math.NaN())
		}
		return float32(a)
	}
	v := math.Mod(a, b)
	if isMod && v != 0 && math.Signbit(v) != math.Signbit(b) {
		v += b
	}
	return float32(v)
}

// --- Recursive descent over a single argument's token slice ----------------

type mathParser struct {
	toks  []pa.Token
	pos   int
	depth int
}

func isMathWhitespace(t pa.Token) bool {
	switch t.(type) {
	case pa.Whitespace, pa.Comment:
		return true
	}
	return false
}

func (p *mathParser) skipWS() {
	for p.pos < len(p.toks) && isMathWhitespace(p.toks[p.pos]) {
		p.pos++
	}
}

func (p *mathParser) peek() pa.Token {
	p.skipWS()
	if p.pos >= len(p.toks) {
		return nil
	}
	return p.toks[p.pos]
}

func (p *mathParser) next() pa.Token {
	tok := p.peek()
	if tok != nil {
		p.pos++
	}
	return tok
}

// evalSum evaluates one argument (a <calc-sum>) of a math function.
func evalSum(arg []pa.Token, shape pr.MathType, ctx MathContext, depth int) (resolvedValue, error) {
	if depth > limits.MathDepth {
		return resolvedValue{}, errMathTooDeep
	}
	p := &mathParser{toks: arg, depth: depth}
	rv, err := p.parseSum(shape, ctx)
	if err != nil {
		return resolvedValue{}, err
	}
	p.skipWS()
	if p.pos != len(p.toks) {
		return resolvedValue{}, errMathSyntax
	}
	return rv, nil
}

// isSumOperator reports whether toks[i] is a '+' or '-' operator.
func isSumOperator(toks []pa.Token, i int) (string, bool) {
	lit, ok := toks[i].(pa.Literal)
	if !ok || (lit.Value != "+" && lit.Value != "-") {
		return "", false
	}
	return lit.Value, true
}

// spacedOperator reports whether toks[i] has the whitespace on both sides
// that CSS Values 4 requires around '+' and '-'.
func spacedOperator(toks []pa.Token, i int) bool {
	if i == 0 || i+1 >= len(toks) {
		return false
	}
	_, before := toks[i-1].(pa.Whitespace)
	_, after := toks[i+1].(pa.Whitespace)
	return before && after
}

func (p *mathParser) parseSum(shape pr.MathType, ctx MathContext) (resolvedValue, error) {
	left, err := p.parseProduct(shape, ctx)
	if err != nil {
		return resolvedValue{}, err
	}
	for {
		if p.peek() == nil {
			return left, nil
		}
		op, ok := isSumOperator(p.toks, p.pos)
		if !ok {
			return left, nil
		}
		// In CSS Values 4, '+' and '-' MUST be surrounded by whitespace.
		if !spacedOperator(p.toks, p.pos) {
			return resolvedValue{}, errMathSyntax
		}
		p.pos++
		right, err := p.parseProduct(shape, ctx)
		if err != nil {
			return resolvedValue{}, err
		}
		if left, right, err = unifyLengthPerc(left, right, shape, ctx); err != nil {
			return resolvedValue{}, err
		}
		if op == "+" {
			left.V += right.V
		} else {
			left.V -= right.V
		}
	}
}

func (p *mathParser) parseProduct(shape pr.MathType, ctx MathContext) (resolvedValue, error) {
	left, err := p.parseValue(shape, ctx)
	if err != nil {
		return resolvedValue{}, err
	}
	for {
		op := p.peek()
		lit, ok := op.(pa.Literal)
		if !ok || (lit.Value != "*" && lit.Value != "/") {
			return left, nil
		}
		p.pos++
		right, err := p.parseValue(shape, ctx)
		if err != nil {
			return resolvedValue{}, err
		}
		if lit.Value == "*" {
			// One side must be a scalar.
			switch {
			case right.isNumber():
				left.V *= right.V
			case left.isNumber():
				right.V *= left.V
				left = right
			default:
				return resolvedValue{}, errMathTypeMismatch
			}
		} else {
			// Division: by a number, or by a value of the same type, which
			// gives a number (CSS Values 4 §10.9: calc(10px / 2px) is 5).
			// Dividing by zero yields ±∞ or NaN, which are sanitised at the
			// top level.
			switch {
			case right.isNumber():
				left.V /= right.V
			case left.Unit == right.Unit:
				left = resolvedValue{V: left.V / right.V, Unit: pr.Scalar}
			default:
				return resolvedValue{}, errMathTypeMismatch
			}
		}
	}
}

func (p *mathParser) parseValue(shape pr.MathType, ctx MathContext) (resolvedValue, error) {
	tok := p.next()
	if tok == nil {
		return resolvedValue{}, errMathSyntax
	}
	switch v := tok.(type) {
	case pa.Number:
		return resolvedValue{V: v.ValueF, Unit: pr.Scalar}, nil
	case pa.Percentage:
		return resolvedValue{V: v.ValueF, Unit: pr.Perc}, nil
	case pa.Dimension:
		return resolveDimension(v, ctx)
	case pa.Ident:
		switch strings.ToLower(v.Value) {
		case "pi":
			return resolvedValue{V: float32(math.Pi), Unit: pr.Scalar}, nil
		case "e":
			return resolvedValue{V: float32(math.E), Unit: pr.Scalar}, nil
		case "infinity":
			return resolvedValue{V: float32(math.Inf(+1)), Unit: pr.Scalar}, nil
		case "-infinity":
			return resolvedValue{V: float32(math.Inf(-1)), Unit: pr.Scalar}, nil
		case "nan":
			return resolvedValue{V: float32(math.NaN()), Unit: pr.Scalar}, nil
		}
		return resolvedValue{}, errMathTypeMismatch
	case pa.ParenthesesBlock:
		// A bare parenthesised expression at the value level.
		return evalSum(v.Arguments, shape, ctx, p.depth+1)
	case pa.FunctionBlock:
		if !isMathFunction(v) {
			return resolvedValue{}, errMathTypeMismatch
		}
		return evalMathFunction(v, shape, ctx, p.depth+1)
	}
	// Unary operators are not part of the grammar: signs belong to the
	// numeric token itself (`-2px`).
	return resolvedValue{}, errMathSyntax
}

// resolveDimension converts a dimension token to pixels (length), radians
// (angle), or dppx (resolution) using ctx.
func resolveDimension(d pa.Dimension, ctx MathContext) (resolvedValue, error) {
	unit := strings.ToLower(string(d.Unit))
	if u, ok := lengthUnits[unit]; ok {
		return resolveLengthDim(d.ValueF, u, ctx)
	}
	if u, ok := angleUnits[unit]; ok {
		return resolvedValue{V: d.ValueF * angleToRadians[u], Unit: pr.Rad}, nil
	}
	if factor, ok := resolutionToDppx[unit]; ok {
		return resolvedValue{V: d.ValueF * factor, Unit: unitDppx}, nil
	}
	return resolvedValue{}, errMathTypeMismatch
}

func resolveLengthDim(v float32, unit pr.Unit, ctx MathContext) (resolvedValue, error) {
	switch unit {
	case pr.Px:
		return resolvedValue{V: v, Unit: pr.Px}, nil
	case pr.Em:
		return resolvedValue{V: v * ctx.FontSize, Unit: pr.Px}, nil
	case pr.Rem:
		return resolvedValue{V: v * ctx.RootFontSize, Unit: pr.Px}, nil
	case pr.Ex:
		ratio := ctx.ExRatio
		if ratio == 0 {
			ratio = 0.5
		}
		return resolvedValue{V: v * ctx.FontSize * ratio, Unit: pr.Px}, nil
	case pr.Ch:
		ratio := ctx.ChRatio
		if ratio == 0 {
			ratio = 0.5
		}
		return resolvedValue{V: v * ctx.FontSize * ratio, Unit: pr.Px}, nil
	case pr.Lh:
		line := ctx.LineHeight
		if !ctx.HasLineHeight {
			line = 1.2 * ctx.FontSize
		}
		return resolvedValue{V: v * line, Unit: pr.Px}, nil
	case pr.Rlh:
		line := ctx.RootLineHeight
		if !ctx.HasRootLineHeight {
			line = 1.2 * ctx.RootFontSize
		}
		return resolvedValue{V: v * line, Unit: pr.Px}, nil
	case pr.Pt, pr.Pc, pr.In, pr.Cm, pr.Mm, pr.Q:
		factor := pr.LengthToPixels(unit)
		return resolvedValue{V: v * float32(factor), Unit: pr.Px}, nil
	}
	if unit.IsViewport() {
		return resolvedValue{V: ctx.ViewportSizes.Resolve(v, unit), Unit: pr.Px}, nil
	}
	return resolvedValue{}, errMathTypeMismatch
}
