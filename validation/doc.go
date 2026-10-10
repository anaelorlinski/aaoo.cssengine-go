// Package validation turns the declarations of a stylesheet into typed
// property values: it validates longhands, expands shorthands, resolves
// logical properties, parses math functions (calc(), min(), round(), ...,
// CSS Values 4 §10) and the descriptors of @font-face and @counter-style.
// It also evaluates @supports declaration tests ([Supports]).
//
// It is ported from WeasyPrint's css/validation module.
package validation
