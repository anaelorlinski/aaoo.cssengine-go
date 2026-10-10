package validation

import (
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

// CSS logical properties, resolved for the horizontal-tb writing mode —
// the only one the engine supports — by mapping each logical property
// onto its physical equivalent before validation:
// block-start → top, block-end → bottom, inline-start → left,
// inline-end → right, inline-size → width, block-size → height.
// See https://www.w3.org/TR/css-logical-1/

type logicalDecl struct {
	name   string
	tokens []pa.Token
}

// logicalAliases are 1:1 renames: same value, physical name.
var logicalAliases = map[string]string{
	"margin-block-start":  "margin-top",
	"margin-block-end":    "margin-bottom",
	"margin-inline-start": "margin-left",
	"margin-inline-end":   "margin-right",

	"padding-block-start":  "padding-top",
	"padding-block-end":    "padding-bottom",
	"padding-inline-start": "padding-left",
	"padding-inline-end":   "padding-right",

	"inset-block-start":  "top",
	"inset-block-end":    "bottom",
	"inset-inline-start": "left",
	"inset-inline-end":   "right",

	"inline-size":     "width",
	"block-size":      "height",
	"min-inline-size": "min-width",
	"min-block-size":  "min-height",
	"max-inline-size": "max-width",
	"max-block-size":  "max-height",

	"border-block-start":  "border-top",
	"border-block-end":    "border-bottom",
	"border-inline-start": "border-left",
	"border-inline-end":   "border-right",

	"border-block-start-width":  "border-top-width",
	"border-block-start-style":  "border-top-style",
	"border-block-start-color":  "border-top-color",
	"border-block-end-width":    "border-bottom-width",
	"border-block-end-style":    "border-bottom-style",
	"border-block-end-color":    "border-bottom-color",
	"border-inline-start-width": "border-left-width",
	"border-inline-start-style": "border-left-style",
	"border-inline-start-color": "border-left-color",
	"border-inline-end-width":   "border-right-width",
	"border-inline-end-style":   "border-right-style",
	"border-inline-end-color":   "border-right-color",

	"border-start-start-radius": "border-top-left-radius",
	"border-start-end-radius":   "border-top-right-radius",
	"border-end-start-radius":   "border-bottom-left-radius",
	"border-end-end-radius":     "border-bottom-right-radius",
}

// logicalSplitPairs are two-value shorthands: one value applies to
// both physical sides, two values distribute start/end.
var logicalSplitPairs = map[string][2]string{
	"margin-block":   {"margin-top", "margin-bottom"},
	"margin-inline":  {"margin-left", "margin-right"},
	"padding-block":  {"padding-top", "padding-bottom"},
	"padding-inline": {"padding-left", "padding-right"},
	"inset-block":    {"top", "bottom"},
	"inset-inline":   {"left", "right"},
}

// logicalCopyPairs are shorthands whose full value applies to both
// physical sides (border-block: 1px solid red sets both borders).
var logicalCopyPairs = map[string][2]string{
	"border-block-width":  {"border-top-width", "border-bottom-width"},
	"border-block-style":  {"border-top-style", "border-bottom-style"},
	"border-block-color":  {"border-top-color", "border-bottom-color"},
	"border-inline-width": {"border-left-width", "border-right-width"},
	"border-inline-style": {"border-left-style", "border-right-style"},
	"border-inline-color": {"border-left-color", "border-right-color"},
	"border-block":        {"border-top", "border-bottom"},
	"border-inline":       {"border-left", "border-right"},
}

// insetSides expands the `inset` shorthand like `margin`.
var insetSides = [4]string{"top", "right", "bottom", "left"}

// resolveLogical maps a (possibly logical) declaration to the physical
// declarations it stands for. Non-logical names pass through untouched;
// a nil result flags an invalid value count on a logical shorthand.
func resolveLogical(name string, tokens []pa.Token) []logicalDecl {
	if physical, ok := logicalAliases[name]; ok {
		return []logicalDecl{{physical, tokens}}
	}

	if sides, ok := logicalSplitPairs[name]; ok {
		switch len(tokens) {
		case 1:
			return []logicalDecl{{sides[0], tokens}, {sides[1], tokens}}
		case 2:
			return []logicalDecl{{sides[0], tokens[0:1]}, {sides[1], tokens[1:2]}}
		default:
			return nil
		}
	}

	if sides, ok := logicalCopyPairs[name]; ok {
		return []logicalDecl{{sides[0], tokens}, {sides[1], tokens}}
	}

	if name == "inset" {
		var t, r, b, l []pa.Token
		switch len(tokens) {
		case 1:
			t, r, b, l = tokens, tokens, tokens, tokens
		case 2:
			t, r, b, l = tokens[0:1], tokens[1:2], tokens[0:1], tokens[1:2]
		case 3:
			t, r, b, l = tokens[0:1], tokens[1:2], tokens[2:3], tokens[1:2]
		case 4:
			t, r, b, l = tokens[0:1], tokens[1:2], tokens[2:3], tokens[3:4]
		default:
			return nil
		}
		return []logicalDecl{
			{insetSides[0], t}, {insetSides[1], r}, {insetSides[2], b}, {insetSides[3], l},
		}
	}

	return []logicalDecl{{name, tokens}}
}
