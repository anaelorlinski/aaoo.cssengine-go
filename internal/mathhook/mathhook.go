// Package mathhook connects package properties to the math evaluator of
// package validation, which imports it: layout-time code finishes the
// evaluation of a math function deferred at computed-value time (a
// percentage of a box dimension) without re-walking the cascade.
//
// It is internal: only this module sets the hook, once, at initialization.
package mathhook

// Eval evaluates a *properties.PendingMath against the percentage
// reference referTo (unusable when hasRef is false: a percentage operand
// then fails the evaluation). Set by package validation; nil until then.
var Eval func(pm any, referTo float32, hasRef bool) (float32, error)
