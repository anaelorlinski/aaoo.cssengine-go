package validation

import (
	"errors"
	"math"
	"strings"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// CSS Values and Units 4 — Math Functions.
//
// This file provides:
//
//   - isMathFunction: cheap predicate matching the function name.
//   - parseMath:      shape-check + wrap into a [pr.PendingMath]. The check
//                     mirrors WeasyPrint's `check_math` in
//                     weasyprint/css/functions.py — it ensures the function
//                     name is known, the operand count fits the arity rules
//                     and every leaf is a recognisable token type for the
//                     requested resolved-to category. It does NOT evaluate
//                     anything; full evaluation happens at compute time.
//
// :copyright: 2011-2014 Simon Sapin and contributors (WeasyPrint).
// :license:   BSD.

// Math function names recognised by CSS Values 4.
//
// Arities documented inline; "*" means variadic.
var mathFunctionArity = map[string][2]int{
	// CSS Values 4 — basic
	"calc":  {1, 1},
	"min":   {1, -1}, // 1..*
	"max":   {1, -1},
	"clamp": {3, 3}, // (MIN, VAL, MAX)

	// CSS Values 4 — stepped
	"round": {1, 3}, // [<rounding-strategy>,]? A[, B]
	"mod":   {2, 2},
	"rem":   {2, 2},

	// CSS Values 4 — trig
	"sin":   {1, 1},
	"cos":   {1, 1},
	"tan":   {1, 1},
	"asin":  {1, 1},
	"acos":  {1, 1},
	"atan":  {1, 1},
	"atan2": {2, 2},

	// CSS Values 4 — exponential
	"pow":   {2, 2},
	"sqrt":  {1, 1},
	"hypot": {1, -1},
	"log":   {1, 2},
	"exp":   {1, 1},

	// CSS Values 4 — sign-related
	"abs":  {1, 1},
	"sign": {1, 1},
}

// roundingStrategies are the leading idents allowed inside round().
var roundingStrategies = map[string]bool{
	"nearest": true,
	"up":      true,
	"down":    true,
	"to-zero": true,
}

// errors surfaced from shape-checking. Validators usually convert these
// into a generic errInvalidValue, but the type lets the evaluator detect
// "obviously broken" expressions before resolution.
var (
	errMathBadFunction = errors.New("unknown math function")
	errMathBadArity    = errors.New("wrong number of arguments to math function")
	errMathBadOperand  = errors.New("invalid operand in math function")
)

// isMathFunction reports whether token is a FunctionBlock whose name is one
// of the math functions defined by CSS Values 4.
func isMathFunction(token pa.Token) bool {
	fb, ok := token.(pa.FunctionBlock)
	if !ok {
		return false
	}
	_, in := mathFunctionArity[strings.ToLower(fb.Name)]
	return in
}

// parseMath shape-checks the math function in token and returns a
// [pr.PendingMath] wrapping it. resolveTo says what unit shape the
// consuming property requires — the check accepts any operand that *might*
// resolve to that shape, deferring numeric reduction to evaluation time.
//
// percentageReferTo is propagated to the PendingMath so the evaluator
// knows which layout dimension to substitute for percentages (empty when
// percentages must remain symbolic until layout).
func parseMath(token pa.Token, resolveTo pr.MathType, percentageReferTo string) (pr.PendingMath, error) {
	fb, ok := token.(pa.FunctionBlock)
	if !ok {
		return pr.PendingMath{}, errMathBadFunction
	}
	name := strings.ToLower(fb.Name)
	if _, in := mathFunctionArity[name]; !in {
		return pr.PendingMath{}, errMathBadFunction
	}
	if err := checkMath(fb, resolveTo, resolveTo == pr.MathAngle, 0); err != nil {
		return pr.PendingMath{}, err
	}
	if err := checkMathType(fb, resolveTo); err != nil {
		return pr.PendingMath{}, err
	}
	return pr.PendingMath{
		Expr:              fb,
		ResolveTo:         resolveTo,
		PercentageReferTo: percentageReferTo,
	}, nil
}

// probeContext evaluates math at parse time, for its type only: any finite
// font metrics, percentage reference and viewport do.
var probeContext = func() MathContext {
	viewport := pr.NewViewportSizes()
	viewport.SetBox(1000, 1000)
	viewport.SetArea(1000, 1000)
	return MathContext{
		FontSize: 16, RootFontSize: 16, ViewportSizes: viewport,
		PercentageReference: 100, HasPercentageRef: true,
	}
}()

// checkMathType checks the type of the expression fb against the type the
// property requires (CSS Values 4 §10.9, type checking): `calc(20)` is a
// <number>, not a <length>, and `calc(1px * 2px)` has no valid type. The
// expression is evaluated once with [probeContext]; its value does not
// matter, only the type of the result.
func checkMathType(fb pa.FunctionBlock, resolveTo pr.MathType) error {
	ctx := probeContext
	if resolveTo != pr.MathLengthPercentage && resolveTo != pr.MathPercentage {
		ctx.HasPercentageRef = false
	}
	rv, err := evalMathFunction(fb, resolveTo, ctx, 0)
	if err != nil {
		return err
	}
	ok := false
	switch resolveTo {
	case pr.MathNumber, pr.MathInteger:
		ok = rv.isNumber()
	case pr.MathLength:
		ok = rv.isLength()
	case pr.MathPercentage:
		ok = rv.isPercentage()
	case pr.MathLengthPercentage:
		ok = rv.isLength() || rv.isPercentage()
	case pr.MathAngle:
		ok = rv.isAngle()
	case pr.MathResolution:
		ok = rv.isResolution()
	}
	if !ok {
		return errMathTypeMismatch
	}
	return nil
}

// checkMath walks fb to verify its shape. It does NOT evaluate operands
// nor resolve units; it only catches syntactic errors that the evaluator
// would otherwise have to re-discover later.
//
// allowAngle reports whether angle operands are acceptable: true for an
// angle-typed property and anywhere below a trigonometric function, so
// that `sin(calc(30deg))` is accepted. depth is the current nesting level
// of functions and parentheses, bounded by [limits.MathDepth].
func checkMath(fb pa.FunctionBlock, resolveTo pr.MathType, allowAngle bool, depth int) error {
	if depth > limits.MathDepth {
		return errMathTooDeep
	}
	name, _, args, err := splitMathFunction(fb)
	if err != nil {
		return err
	}

	// Per-function operand-shape constraints. The trig/exp functions take
	// numbers (or numeric calc()) and produce numbers/angles; the sign of
	// abs() preserves its argument's shape; etc. We only enforce the
	// strictest rules — leaving unit reduction to the evaluator.
	allowAngle = allowAngle || isTrigFunction(name)
	argShape := resolveTo
	if name == "sign" || name == "atan2" {
		// they reduce any type to a number or an angle (atan2(1px, 2px),
		// sign(1em)); a percentage only where the property has a
		// percentage basis to resolve it against
		argShape, allowAngle = signArgumentShape(resolveTo), true
	}
	for _, arg := range args {
		if err := checkMathOperand(arg, argShape, allowAngle, depth); err != nil {
			return err
		}
	}
	return nil
}

// splitMathArguments breaks a comma-separated argument list, stripping
// surrounding whitespace from each segment. An empty list (e.g. "calc()")
// is reported as a single empty segment so arity checks reject it.
func splitMathArguments(tokens []pa.Token) ([][]pa.Token, error) {
	var out [][]pa.Token
	var current []pa.Token
	flush := func() {
		out = append(out, trimWhitespace(current))
		current = nil
	}
	for _, t := range tokens {
		if lit, ok := t.(pa.Literal); ok && lit.Value == "," {
			flush()
			continue
		}
		current = append(current, t)
	}
	flush()
	for _, seg := range out {
		if len(seg) == 0 {
			return nil, errMathBadArity
		}
	}
	return out, nil
}

// trimWhitespace strips leading/trailing whitespace tokens.
func trimWhitespace(in []pa.Token) []pa.Token {
	start, end := 0, len(in)
	for start < end {
		if _, ws := in[start].(pa.Whitespace); !ws {
			break
		}
		start++
	}
	for end > start {
		if _, ws := in[end-1].(pa.Whitespace); !ws {
			break
		}
		end--
	}
	return in[start:end]
}

// singleIdent returns the lowercase value of a single Ident operand, or "".
func singleIdent(arg []pa.Token) string {
	if len(arg) != 1 {
		return ""
	}
	id, ok := arg[0].(pa.Ident)
	if !ok {
		return ""
	}
	return strings.ToLower(id.Value)
}

// checkMathOperand walks arg (one comma-separated argument of a math
// function, or the content of a parenthesised group) and validates that
// every leaf is a token type compatible with the resolved-to category. It
// recurses into nested calc()/min()/etc. It also enforces the operator
// layout rules that are cheap to check here: no operator at either end,
// and whitespace on both sides of '+' and '-'.
func checkMathOperand(arg []pa.Token, resolveTo pr.MathType, allowAngle bool, depth int) error {
	if depth > limits.MathDepth {
		return errMathTooDeep
	}
	arg = trimWhitespace(arg)
	if len(arg) == 0 {
		return errMathBadOperand
	}
	for i, t := range arg {
		if _, isOp := t.(pa.Literal); isOp && (i == 0 || i == len(arg)-1) {
			return errMathBadOperand
		}
		if _, isSum := isSumOperator(arg, i); isSum && !spacedOperator(arg, i) {
			return errMathBadOperand
		}
		if err := checkMathToken(t, resolveTo, allowAngle, depth); err != nil {
			return err
		}
	}
	return nil
}

// absoluteLengthUnits are the lengths that do not depend on the element.
var absoluteLengthUnits = map[string]bool{"px": true, "cm": true, "mm": true, "q": true, "in": true, "pt": true, "pc": true}

func checkMathToken(t pa.Token, resolveTo pr.MathType, allowAngle bool, depth int) error {
	switch v := t.(type) {
	case pa.Whitespace, pa.Comment:
		return nil
	case pa.Literal:
		// Operators + - * / and parens-implicit comma are accepted —
		// detailed precedence checking happens during evaluation.
		switch v.Value {
		case "+", "-", "*", "/":
			return nil
		}
		return errMathBadOperand
	case pa.Number:
		return nil
	case pa.Percentage:
		if !resolveTo.AcceptsPercentage() {
			return errMathBadOperand
		}
		return nil
	case pa.Dimension:
		unit := strings.ToLower(string(v.Unit))
		// a <number> may come from a like-type division (calc(10px / 2px)):
		// the type check after this one decides. Only absolute lengths:
		// numbers are evaluated without the element (contextFreeMath).
		numeric := resolveTo == pr.MathNumber || resolveTo == pr.MathInteger
		if _, isLength := lengthUnits[unit]; isLength {
			if !resolveTo.AcceptsLength() && !(numeric && absoluteLengthUnits[unit]) {
				return errMathBadOperand
			}
			return nil
		}
		if _, isAngle := angleUnits[unit]; isAngle {
			if allowAngle || numeric {
				return nil
			}
			return errMathBadOperand
		}
		if _, isResolution := resolutionToDppx[unit]; isResolution {
			if resolveTo == pr.MathResolution {
				return nil
			}
			return errMathBadOperand
		}
		return errMathBadOperand
	case pa.Ident:
		// Math constants — pi, e, infinity, -infinity, NaN.
		switch strings.ToLower(v.Value) {
		case "pi", "e", "infinity", "-infinity", "nan":
			return nil
		}
		return errMathBadOperand
	case pa.ParenthesesBlock:
		return checkMathOperand(v.Arguments, resolveTo, allowAngle, depth+1)
	case pa.FunctionBlock:
		if !isMathFunction(v) {
			return errMathBadFunction
		}
		return checkMath(v, resolveTo, allowAngle, depth+1)
	}
	return errMathBadOperand
}

func isTrigFunction(name string) bool {
	switch name {
	case "sin", "cos", "tan", "asin", "acos", "atan", "atan2":
		return true
	}
	return false
}

// --- Validator helpers ------------------------------------------------------
//
// These wrap getLength/getAngle/etc. so a property can transparently accept
// either a concrete value or a math function. A single nil return means the
// token is neither a valid concrete value nor a valid math function.

// getLengthOrCalc returns either a [pr.DimOrS] (concrete length) or a
// [pr.PendingMath] (deferred). negative and percentage flags propagate to the
// concrete-path [getLength]. percentageReferTo, when non-empty, marks the
// PendingMath so the evaluator knows which layout dimension to use.
func getLengthOrCalc(token pa.Token, negative, percentage bool, percentageReferTo string) pr.CssProperty {
	if l := getLength(token, negative, percentage); !l.IsNone() {
		return l.Tagged()
	}
	if !isMathFunction(token) {
		return nil
	}
	shape := pr.MathLength
	if percentage {
		shape = pr.MathLengthPercentage
	}
	pm, err := parseMath(token, shape, percentageReferTo)
	if err != nil {
		return nil
	}
	pm.NonNegative = !negative
	return pm
}

// getLengthPercentageOrCalc is a thin wrapper requiring percentage support.
func getLengthPercentageOrCalc(token pa.Token, negative bool, percentageReferTo string) pr.CssProperty {
	return getLengthOrCalc(token, negative, true, percentageReferTo)
}

// getNumberOrCalc accepts a <number> or a math function that resolves to a
// number. Returns pr.Float on the concrete path, pr.PendingMath on the math
// path, nil otherwise.
func getNumberOrCalc(token pa.Token) pr.CssProperty {
	if n, ok := token.(pa.Number); ok {
		return pr.Float(n.ValueF)
	}
	if !isMathFunction(token) {
		return nil
	}
	pm, err := parseMath(token, pr.MathNumber, "")
	if err != nil {
		return nil
	}
	return pm
}

// contextFreeMath evaluates a math function of shape <angle>, <resolution>,
// <number> or <integer> at parse time: these types have no relative units,
// so the value does not depend on the element. It returns false for
// anything else (not a math function, a type error).
func contextFreeMath(token pa.Token, shape pr.MathType) (float32, bool) {
	if !isMathFunction(token) {
		return 0, false
	}
	pm, err := parseMath(token, shape, "")
	if err != nil {
		return 0, false
	}
	v, err := EvalMath(pm, probeContext)
	if err != nil || math.IsNaN(float64(v)) {
		return 0, false
	}
	return v, true
}

// contextFreeInteger is [contextFreeMath] for an <integer>: the result is
// rounded to the nearest integer (CSS Values 4 §10.9).
func contextFreeInteger(token pa.Token) (int, bool) {
	v, ok := contextFreeMath(token, pr.MathInteger)
	if !ok {
		return 0, false
	}
	return int(math.Round(float64(max(min(v, math.MaxInt32), math.MinInt32)))), true
}

// getLengthDim is getLength, also accepting a math function, kept as a
// [pr.Dimension] with a pending value resolved at layout time (positions
// and sizes whose percentages refer to a box).
func getLengthDim(token pa.Token, negative, percentage bool) pr.Dimension {
	if l := getLength(token, negative, percentage); !l.IsNone() {
		return l
	}
	if pm, ok := getLengthOrCalc(token, negative, percentage, "layout").(pr.PendingMath); ok {
		return pr.Dimension{Math: &pm}
	}
	return pr.Dimension{}
}

// signArgumentShape is the type the arguments of sign() and atan2() may
// have, in a property of type resolveTo: any, percentages included only if
// the property resolves them.
func signArgumentShape(resolveTo pr.MathType) pr.MathType {
	if resolveTo.AcceptsPercentage() {
		return pr.MathLengthPercentage
	}
	return pr.MathLength
}
