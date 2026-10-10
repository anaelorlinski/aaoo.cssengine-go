package validation

import (
	"slices"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
	pr "github.com/anaelorlinski/aaoo.cssengine-go/properties"
)

// Supports reports whether the declaration `name: tokens` would be
// accepted by the validator — the test behind `@supports (name: value)`.
// Custom properties and declarations containing var() are supported by
// definition (they only need to parse).
func Supports(name string, tokens []pa.Token) bool {
	name = utils.AsciiLower(name)
	if len(name) >= 2 && name[:2] == "--" {
		return true // any value, the empty one included
	}
	tokens = pa.RemoveWhitespace(tokens)
	if len(tokens) == 0 {
		return false
	}
	if name == "all" {
		_, err := expandAll(tokens)
		return err == nil
	}
	physicals := resolveLogical(name, tokens)
	if physicals == nil {
		return false
	}
	// with var(), the value is only known at computed-value time: any value
	// is supported, by a known property
	if slices.ContainsFunc(tokens, HasVar) {
		for _, ld := range physicals {
			if pr.PropertyByName(ld.name) == 0 && pr.NewShorthand(ld.name) == 0 {
				return false
			}
		}
		return true
	}
	for _, ld := range physicals {
		if sh := pr.NewShorthand(ld.name); sh != 0 {
			if _, err := expanders[sh]("", sh, ld.tokens); err != nil {
				return false
			}
		} else if _, err := validateNonShorthand("", ld.name, ld.tokens, false); err != nil {
			return false
		}
	}
	return true
}
