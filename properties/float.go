package properties

import (
	"math"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/mathhook"
	"github.com/anaelorlinski/aaoo.cssengine-go/logger"
)

// During layout, float numbers sometimes need special values like "auto" or nil (None in Python).
// This file define a float64-like type handling these cases.

const (
	// AutoF indicates a value specified as "auto", which will
	// be resolved during layout.
	AutoF special = true
)

type MaybeFloat interface {
	V() Float
}

func (f Float) V() Float { return f }

type special bool

func (f special) V() Float { return 0 }

func (f special) String() string {
	if f {
		return "auto"
	}
	return "-"
}

// Is returns true except for 0 or nil.
func Is(m MaybeFloat) bool {
	if m == nil {
		return false
	}
	if f, ok := m.(Float); ok {
		return f != 0
	}
	return false
}

// MaybeFloatToFloat is the same as MaybeFloat.V(),
// but handles nil values
func MaybeFloatToFloat(mf MaybeFloat) Float {
	if mf == nil {
		return 0
	}
	return mf.V()
}

func Floor(x Float) Float {
	return Float(math.Floor(float64(x)))
}

func Maxs(values ...Float) Float {
	max := -Inf
	for _, w := range values {
		if w > max {
			max = w
		}
	}
	return max
}

func Mins(values ...Float) Float {
	min := Inf
	for _, w := range values {
		if w < min {
			min = w
		}
	}
	return min
}

func Hypot(a, b Float) Float {
	return Float(math.Hypot(float64(a), float64(b)))
}

func Abs(x Float) Float {
	if x < 0 {
		return -x
	}
	return x
}

// ResolvePercentage returns the percentage of the reference value, or the value unchanged.
// “referTo“ is the length for 100%. If “referTo“ is not a number, it
// just replaces percentages.
func ResolvePercentage(value TaggedDim, referTo Float) MaybeFloat {
	if value.IsNone() {
		return nil
	} else if value.Tag == Auto {
		return AutoF
	} else if value.Dimension.Math != nil {
		v, ok := evalDeferredMath(value.Dimension.Math, referTo, true)
		if !ok {
			return Float(0)
		}
		return Float(v)
	} else if value.Unit == Px {
		return value.Value
	} else {
		if value.Unit != Perc {
			logger.L().Warn("unexpected unit where a length or a percentage was expected", "unit", value.Unit)
			return Float(0)
		}
		return referTo * value.Value / 100.
	}
}

// IsDefinite reports whether v has a used value that does not depend on a
// containing-block dimension: an absolute length, or a math expression
// (calc/min/clamp/…) with no percentage operand. "auto", a plain
// percentage, and a math expression containing a percentage are *not*
// definite — CSS says such values behave as "auto" when the reference is
// itself indefinite.
//
// The returned length is in pixels and only meaningful when ok is true.
func (v TaggedDim) IsDefinite() (Float, bool) {
	if v.IsNone() || v.Tag != 0 {
		// nil, "auto", "content", "from-font", …
		return 0, false
	}
	if v.Dimension.Math != nil {
		// A percentage-free expression resolves without a reference; one
		// with a percentage reports ErrMathNeedPercRef and stays pending.
		f, ok := evalDeferredMath(v.Dimension.Math, 0, false)
		return Float(f), ok
	}
	switch v.Unit {
	case Px, Scalar:
		// Scalar is how "pixels only" properties store their computed
		// value (see asPixels): still a definite pixel length.
		return v.Value, true
	case Perc:
		return 0, false
	default:
		logger.L().Warn("unexpected unit where a computed length was expected", "unit", v.Unit)
		return 0, false
	}
}

// evalDeferredMath finishes a math function deferred to layout time,
// through the evaluator of package validation (see [mathhook]), which a
// program parsing stylesheets imports. hasRef reports whether referTo is a
// usable percentage reference; when false, a percentage operand makes the
// evaluation fail instead of silently resolving against zero.
func evalDeferredMath(pm *PendingMath, referTo Float, hasRef bool) (Fl, bool) {
	if mathhook.Eval == nil {
		logger.L().Warn("math function not evaluated: package validation is not linked", "expression", pm.Expr.Name)
		return 0, false
	}
	v, err := mathhook.Eval(pm, Fl(referTo), hasRef)
	if err != nil {
		return 0, false
	}
	if pm.NonNegative && v < 0 {
		v = 0
	}
	return v, true
}
