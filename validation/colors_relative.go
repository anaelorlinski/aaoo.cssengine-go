package validation

import (
	"math"

	csscolor "github.com/anaelorlinski/aaoo.cssengine-go/color"
	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// CSS Color Level 4 relative color syntax.
//
//	oklch(from <color> <L> <C> <H> [/ <alpha>])
//	oklab(from <color> <L> <A> <B> [/ <alpha>])
//	lab(  from <color> <L> <A> <B> [/ <alpha>])
//	lch(  from <color> <L> <C> <H> [/ <alpha>])
//	hwb(  from <color> <H> <W> <B> [/ <alpha>])
//	rgb(  from <color> <R> <G> <B> [/ <alpha>])  (rgba too)
//	hsl(  from <color> <H> <S> <L> [/ <alpha>])  (hsla too)
//
// Channel values may reference the source color's channels by keyword
// (in the output color space) and may use math functions:
//
//	oklch(from var(--c) 10% calc(0.05 + sin(1.0 * pi) * c) h)
//
// Resolution happens here because the validation package hosts EvalMath.
// The source color is converted to the output space without gamut mapping,
// and the result is a [csscolor.Value] in the output space, rendered like the
// same color written directly.
//
// relSpace describes a color space for relative syntax.
type relSpace struct {
	space    csscolor.Space
	keywords [3]string  // channel keyword names
	scales   [3]float64 // 100% -> scales[i] (in channel units); 0 for hue channels
	hueIndex int        // index of the hue channel, or -1
	// unit is the channel value of a component of 1 (255 for rgb(),
	// whose components are in [0, 1]); 0 means 1
	unit float64
}

func (sp relSpace) channelUnit() float64 {
	if sp.unit == 0 {
		return 1
	}
	return sp.unit
}

var relSpaces = map[string]relSpace{
	"oklch": {space: csscolor.OKLCH, keywords: [3]string{"l", "c", "h"}, scales: [3]float64{1.0, 0.4, 0}, hueIndex: 2},
	"oklab": {space: csscolor.OKLab, keywords: [3]string{"l", "a", "b"}, scales: [3]float64{1.0, 0.4, 0.4}, hueIndex: -1},
	"lab":   {space: csscolor.Lab, keywords: [3]string{"l", "a", "b"}, scales: [3]float64{100.0, 125.0, 125.0}, hueIndex: -1},
	"lch":   {space: csscolor.LCH, keywords: [3]string{"l", "c", "h"}, scales: [3]float64{100.0, 150.0, 0}, hueIndex: 2},
	"hwb":   {space: csscolor.HWB, keywords: [3]string{"h", "w", "b"}, scales: [3]float64{0, 100.0, 100.0}, hueIndex: 0},
	"rgb":   {space: csscolor.SRGB, keywords: [3]string{"r", "g", "b"}, scales: [3]float64{255, 255, 255}, hueIndex: -1, unit: 255},
	"rgba":  {space: csscolor.SRGB, keywords: [3]string{"r", "g", "b"}, scales: [3]float64{255, 255, 255}, hueIndex: -1, unit: 255},
	"hsl":   {space: csscolor.HSL, keywords: [3]string{"h", "s", "l"}, scales: [3]float64{0, 100.0, 100.0}, hueIndex: 0},
	"hsla":  {space: csscolor.HSL, keywords: [3]string{"h", "s", "l"}, scales: [3]float64{0, 100.0, 100.0}, hueIndex: 0},
}

// isFromKeyword reports whether tok is the Ident "from".
func isFromKeyword(tok pa.Token) bool {
	ident, ok := tok.(pa.Ident)
	return ok && utils.AsciiLower(ident.Value) == "from"
}

// parseColorResolved is a drop-in replacement for pa.ParseColor that also
// understands the relative color syntax (oklch(from …), …) and resolves
// calc()/min()/max() expressions inside any color function.
//
// A color function using currentcolor is a [pa.ColorDependsOnCurrent]
// color, computed per element.
func parseColorResolved(token pa.Token) pa.Color {
	v, typ := resolveColorValue(token, nil)
	switch typ {
	case pa.ColorCurrentColor:
		return pa.Color{Type: pa.ColorCurrentColor}
	case pa.ColorDependsOnCurrent:
		return pa.NewDeferredColor(func(current csscolor.Value) (csscolor.Value, pa.ColorType) {
			return resolveColorValue(token, &current)
		})
	case pa.ColorRGBA:
		return pa.NewColor(v)
	}
	return pa.Color{}
}

// resolveColorValue parses a color, resolving relative syntax and math
// functions. current is the value of currentcolor, nil when unknown.
//
// The math resolution is generic: resolveCalcTokenRec walks the entire
// token tree and replaces math functions with their evaluated Number/
// Percentage/Dimension equivalents before the parser sees them, so it works
// for any color function without per-function special cases.
//
// contrast-color() is handled here because its <color> argument may use
// relative syntax or contain calc(), requiring recursive resolution.
func resolveColorValue(token pa.Token, current *csscolor.Value) (csscolor.Value, pa.ColorType) {
	if fb, ok := token.(pa.FunctionBlock); ok {
		name := utils.AsciiLower(fb.Name)

		// Relative color syntax: oklch(from <color> …), etc.
		if _, isRel := relSpaces[name]; isRel {
			args := pa.RemoveWhitespace(fb.Arguments)
			if len(args) > 0 && isFromKeyword(args[0]) {
				return resolveRelativeColor(name, args, current)
			}
		}

		if name == "contrast-color" {
			args := pa.RemoveWhitespace(fb.Arguments)
			if len(args) == 1 {
				bg, typ := resolveColorValue(args[0], current)
				if typ == pa.ColorCurrentColor || typ == pa.ColorDependsOnCurrent {
					return csscolor.Value{}, pa.ColorDependsOnCurrent
				}
				if typ == pa.ColorRGBA {
					if rgb, ok := bg.SRGB(); ok {
						return csscolor.Contrast(rgb), pa.ColorRGBA
					}
				}
			}
			return csscolor.Value{}, pa.ColorInvalid
		}

		if resolved, changed := resolveCalcTokenRec(token); changed {
			token = resolved
		}
	}
	return pa.ParseColorValueNested(token, pa.ColorSchemeLight, current, nestedColor)
}

// nestedColor parses the colors nested in color-mix(), light-dark() and
// contrast-color() which the parser does not know: relative colors.
func nestedColor(token pa.Token, current *csscolor.Value) (csscolor.Value, pa.ColorType, bool) {
	fb, ok := token.(pa.FunctionBlock)
	if !ok {
		return csscolor.Value{}, 0, false
	}
	if _, isRel := relSpaces[utils.AsciiLower(fb.Name)]; !isRel {
		return csscolor.Value{}, 0, false
	}
	if args := pa.RemoveWhitespace(fb.Arguments); len(args) == 0 || !isFromKeyword(args[0]) {
		return csscolor.Value{}, 0, false
	}
	v, typ := resolveColorValue(token, current)
	return v, typ, true
}

// resolveCalcTokenRec recursively walks a token tree and evaluates any
// calc()/min()/max()/clamp()/round()/mod()/rem()/sin()/cos()/etc. math
// functions to their primitive equivalents (Number, Percentage, or
// Dimension). It recurses through every compound token type (via
// pa.TransformTokens) so that nested math inside any color function —
// including contrast-color(oklch(calc(…))) — is resolved in a single pass.
//
// Returns the resolved token and a bool indicating whether any change
// was made. If no math functions are found, the original token is
// returned unchanged (allowing the caller to skip re-parsing).
func resolveCalcTokenRec(token pa.Token) (pa.Token, bool) {
	return pa.TransformTokens(token, func(t pa.Token) (pa.Token, bool, bool) {
		if !isMathFunction(t) {
			return t, false, false // descend into children
		}
		// This token is itself a math function — evaluate it. Its own
		// operands are handled by the evaluator, so stop descending.
		// isMathFunction guarantees t is a FunctionBlock.
		pm := pr.PendingMath{
			Expr:      t.(pa.FunctionBlock),
			ResolveTo: pr.MathLengthPercentage,
		}
		dim, err := EvalMathDim(pm, MathContext{})
		if err != nil {
			return t, false, true
		}
		switch dim.Unit {
		case pr.Scalar:
			return pa.NewNumber(float32(dim.Value), t.Pos()), true, true
		case pr.Perc:
			return pa.NewPercentage(float32(dim.Value), t.Pos()), true, true
		case pr.Rad: // a <hue> written as an angle calculation
			deg := pa.NewNumber(float32(float64(dim.Value)*180/math.Pi), t.Pos())
			return pa.NewDimension(deg, "deg"), true, true
		default:
			return t, false, true
		}
	})
}

// resolveRelativeColor resolves a relative color function whose args begin
// with the "from" keyword.
func resolveRelativeColor(name string, args []pa.Token, current *csscolor.Value) (csscolor.Value, pa.ColorType) {
	sp := relSpaces[name]

	// args[0] is "from"; args[1] is the source color; args[2:] are channels.
	if len(args) < 2 {
		return csscolor.Value{}, pa.ColorInvalid
	}

	// Parse the source color (recursively — may itself be relative).
	src, typ := resolveColorValue(args[1], current)
	switch typ {
	case pa.ColorInvalid:
		return csscolor.Value{}, pa.ColorInvalid
	case pa.ColorCurrentColor, pa.ColorDependsOnCurrent:
		// the channels are checked with a placeholder origin: the color is
		// computed again once currentcolor is known
		src = csscolor.New(csscolor.SRGB, 0, 0, 0)
	}

	// The source channels, in the output color space.
	origin, ok := src.To(sp.space)
	if !ok {
		return csscolor.Value{}, pa.ColorInvalid
	}
	unit := sp.channelUnit()
	kw := map[string]float64{
		sp.keywords[0]: origin.Component(0) * unit,
		sp.keywords[1]: origin.Component(1) * unit,
		sp.keywords[2]: origin.Component(2) * unit,
		"alpha":        origin.Alpha,
	}

	// Split channels from alpha on "/".
	channelArgs := args[2:]
	var alphaArgs []pa.Token
	for i, tok := range channelArgs {
		if pa.IsLiteral(tok, "/") {
			channelArgs, alphaArgs = channelArgs[:i], channelArgs[i+1:]
			if len(alphaArgs) != 1 { // "/" without alpha, or several
				return csscolor.Value{}, pa.ColorInvalid
			}
			break
		}
	}

	// Must have exactly 3 channel tokens.
	if len(channelArgs) != 3 {
		return csscolor.Value{}, pa.ColorInvalid
	}

	out := csscolor.Value{Space: sp.space, Count: 3, Alpha: origin.Alpha}
	for i := 0; i < 3; i++ {
		v, ok := evalChannel(channelArgs[i], kw, sp.scales[i], sp.hueIndex == i)
		if !ok {
			return csscolor.Value{}, pa.ColorInvalid
		}
		if sp.hueIndex != i {
			v /= unit
		}
		out.Components[i] = v
	}

	// Alpha defaults to the source alpha if omitted.
	if len(alphaArgs) > 0 {
		a, ok := evalChannel(alphaArgs[0], kw, 1.0, false)
		if !ok {
			return csscolor.Value{}, pa.ColorInvalid
		}
		out.Alpha = math.Max(0, math.Min(1, a))
	}
	if typ != pa.ColorRGBA {
		return csscolor.Value{}, pa.ColorDependsOnCurrent
	}
	return out, pa.ColorRGBA
}

// evalChannel evaluates a single channel token to a float64.
// For hue channels (isHue=true), Dimension tokens with angle units are
// accepted and converted to degrees. For non-hue channels, FunctionBlock
// tokens are evaluated via the existing EvalMath with MathNumber shape.
// For hue channels, FunctionBlock tokens are evaluated with MathAngle
// shape (returns radians) and then converted to degrees.
func evalChannel(tok pa.Token, kw map[string]float64, scale float64, isHue bool) (float64, bool) {
	switch v := tok.(type) {
	case pa.Number:
		return float64(v.ValueF), true

	case pa.Percentage:
		if isHue { // a <hue> is a number or an angle
			return 0, false
		}
		return float64(v.ValueF) / 100 * scale, true

	case pa.Dimension:
		if !isHue {
			return 0, false
		}
		val := float64(v.ValueF)
		switch utils.AsciiLower(v.Unit) {
		case "deg":
			return val, true
		case "grad":
			return val * 360 / 400, true
		case "rad":
			return val * 180 / math.Pi, true
		case "turn":
			return val * 360, true
		}
		return 0, false

	case pa.Ident:
		lower := utils.AsciiLower(v.Value)
		if lower == "none" {
			return 0, true
		}
		if val, ok := kw[lower]; ok {
			return val, true
		}
		return 0, false

	case pa.FunctionBlock:
		// Substitute channel keywords in the expression, then evaluate.
		substituted := substKeywords(v, kw).(pa.FunctionBlock)
		shape := pr.MathNumber
		if isHue {
			shape = pr.MathAngle
		}
		pm := pr.PendingMath{
			Expr:      substituted,
			ResolveTo: shape,
		}
		result, err := EvalMath(pm, MathContext{})
		if err != nil {
			if !isHue {
				return 0, false
			}
			// a <hue> may also be a <number> of degrees: calc(h + 180)
			pm.ResolveTo = pr.MathNumber
			if result, err = EvalMath(pm, MathContext{}); err != nil {
				return 0, false
			}
			return float64(result), true
		}
		if isHue {
			// EvalMath with MathAngle returns radians; convert to degrees.
			return float64(result) * 180 / math.Pi, true
		}
		return float64(result), true
	}
	return 0, false
}

// substKeywords recursively replaces channel-keyword Ident tokens with
// Number tokens carrying the source color's channel values. It recurses
// through every compound token type (via pa.TransformTokens) so that
// keywords inside calc()/min()/etc. — at any nesting depth — are
// substituted before evaluation.
func substKeywords(tok pa.Token, kw map[string]float64) pa.Token {
	out, _ := pa.TransformTokens(tok, func(t pa.Token) (pa.Token, bool, bool) {
		if ident, ok := t.(pa.Ident); ok {
			if val, ok := kw[utils.AsciiLower(ident.Value)]; ok {
				return pa.NewNumber(float32(val), ident.Pos()), true, true
			}
		}
		return t, false, false // leaf keeps as-is; compound descends
	})
	return out
}
