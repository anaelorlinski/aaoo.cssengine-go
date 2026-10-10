package stylesheet

import (
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

func evalSupports(t *testing.T, cond string) bool {
	t.Helper()
	return EvaluateSupports(parser.Tokenize([]byte(cond), true))
}

func TestSupports(t *testing.T) {
	for _, test := range []struct {
		cond string
		want bool
	}{
		{"(display: flex)", true},
		{"(display: banana)", false},
		{"(frobnicate: 12px)", false},
		{"(color: red)", true},
		{"(margin-block: 1px)", true},
		{"not (frobnicate: 12px)", true},
		{"not (color: red)", false},
		{"(display: flex) and (color: red)", true},
		{"(display: flex) and (frobnicate: 1)", false},
		{"(frobnicate: 1) or (color: red)", true},
		{"(frobnicate: 1) or (blorb: 2)", false},
		{"((display: flex))", true},
		{"(not (frobnicate: 1))", true},
		{"((frobnicate: 1) or (color: red))", true},
		// custom properties and var() are supported by definition
		{"(--x: 1)", true},
		{"(margin: var(--x))", true},
		// general-enclosed evaluates to false but stays valid
		{"selector(h1)", true}, // CSS Conditional 4
		{"(display: flex) and selector(h1)", true},
		{"(display: flex) or selector(h1)", true},
		// malformed
		{"", false},
		{"(display: flex) and", false},
		{"(display: flex) and (color: red) or (width: 1px)", false},
		{"banana", false},
	} {
		if got := evalSupports(t, test.cond); got != test.want {
			t.Errorf("@supports %s: got %v, want %v", test.cond, got, test.want)
		}
	}
}

// CS7: selector() tests whether the selector is supported.
func TestSupportsSelector(t *testing.T) {
	for cond, want := range map[string]bool{
		"selector(p)":                     true,
		"selector(div > p:has(span))":     true,
		"selector(:is(a, b))":             true,
		"not selector(p)":                 false,
		"selector(p:unknown)":             false,
		"not selector(p:unknown)":         true,
		"selector(a, b)":                  false, // one complex selector
		"selector()":                      false,
		"selector(p) and (display: flex)": true,
		"font-tech(color-COLRv1)":         false, // unsupported functions are false
		// forgiving lists: the dropped parts are unsupported (ST3-6)
		"selector(:is(:foo))":       false,
		"selector(:where(a, :foo))": false,
		"selector(:is(a, b) p)":     true,
	} {
		if got := EvaluateSupports(parser.Tokenize([]byte(cond), true)); got != want {
			t.Errorf("%s: %v, want %v", cond, got, want)
		}
	}
}

// An empty custom property is a valid declaration (CS4).
func TestSupportsEmptyCustomProperty(t *testing.T) {
	if !EvaluateSupports(parser.Tokenize([]byte("(--foo: )"), true)) {
		t.Error("(--foo: ) is supported")
	}
}

// C2-1: selector() inside parentheses, and in @import supports().
func TestSupportsNestedSelector(t *testing.T) {
	for cond, want := range map[string]bool{
		"(selector(p))":                         true,
		"((selector(p)))":                       true,
		"(selector(p) and (display: block))":    true,
		"(not selector(p))":                     false,
		"(selector(p:unknown) or (color: red))": true,
	} {
		if got := EvaluateSupports(parser.Tokenize([]byte(cond), true)); got != want {
			t.Errorf("%s: %v, want %v", cond, got, want)
		}
	}
	sheets := map[string]string{"a.css": "a {}"}
	assertRules(t, `@import "a.css" supports(selector(p));`, sheets, "a <a.css>")
	assertRules(t, `@import "a.css" supports(not selector(p));`, sheets)
}
