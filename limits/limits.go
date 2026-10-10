// Package limits gathers the bounds the CSS engine puts on its input.
//
// Stylesheets may come from untrusted documents. Every construct whose cost
// can grow faster than the size of its source, or which recurses on it, is
// bounded here, in one place, so that the bounds can be reviewed together.
// Going over a bound is never a crash: the construct is invalid or cut,
// with a warning.
//
// The values are generous: real stylesheets stay far below them.
package limits

const (
	// NestingDepth bounds the nesting of blocks, functions and selectors:
	// ( [ { in the tokenizer, :is(:not(…)) in selectors. Deeper input is a
	// parse error (it would otherwise exhaust the stack).
	NestingDepth = 512

	// MathDepth bounds the nesting of math functions (calc(min(…))) and
	// parentheses in one expression.
	MathDepth = 64

	// ImportDepth bounds a chain of @import; Imports bounds the stylesheets
	// fetched through @import from one stylesheet (fan-out). Defaults of
	// stylesheet.Options.
	ImportDepth = 16
	Imports     = 256

	// SubstitutionTokens bounds the size of a value after var()
	// substitution, nested tokens included: each level referencing the
	// previous one several times would otherwise grow exponentially
	// ("billion laughs"). Above it, the value is invalid at computed-value
	// time.
	SubstitutionTokens = 1 << 16

	// CounterRepresentation bounds the length, in grapheme clusters, of a
	// rendered counter value (a huge counter in an additive system).
	CounterRepresentation = 60

	// NthCoefficient bounds the a and b of :nth-child(an+b), so the matching
	// arithmetic cannot overflow.
	NthCoefficient = 1<<31 - 1

	// Length is the largest length, in pixels, a value resolves to: an
	// infinite math result (calc(1px / 0), infinity) and overflowing
	// lengths are clamped to ±Length (CSS Values 4 §10.9 leaves the bound
	// to the user agent; 2^25 is what browsers use).
	Length = 1 << 25
)
