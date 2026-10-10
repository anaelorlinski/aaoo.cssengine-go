package cascade

import (
	"slices"

	"github.com/anaelorlinski/aaoo.cssengine-go/internal/utils"
	pa "github.com/anaelorlinski/aaoo.cssengine-go/parser"
)

// The layer syntax of CSS Cascade 5, shared by the stylesheet walkers
// (html and svg) declaring layers in a [Layers].

// ParseLayerName parses a <layer-name>: identifiers separated by '.',
// without whitespace inside, the CSS-wide keywords being reserved
// (https://www.w3.org/TR/css-cascade-5/#typedef-layer-name).
func ParseLayerName(tokens []pa.Token) ([]string, bool) {
	tokens = pa.TrimWhitespace(tokens)
	if len(tokens)%2 == 0 {
		return nil, false // empty, or ending with '.'
	}
	var name []string
	for i, token := range tokens {
		if i%2 == 1 {
			if !pa.IsLiteral(token, ".") {
				return nil, false
			}
			continue
		}
		ident, ok := token.(pa.Ident)
		if !ok {
			return nil, false
		}
		switch utils.AsciiLower(ident.Value) {
		case "initial", "inherit", "unset", "revert", "revert-layer", "default":
			return nil, false
		}
		name = append(name, ident.Value)
	}
	return name, true
}

// LayerRule is the layer part of an @layer rule.
type LayerRule struct {
	// Names are the layers declared by an @layer statement, in order.
	Names [][]string
	// Block reports an @layer block, whose rules go to Names[0], or to a
	// new anonymous layer when Names is empty.
	Block bool
}

// ParseLayerRule parses the prelude of an @layer rule with (block) or
// without (statement) content.
func ParseLayerRule(prelude []pa.Token, block bool) (LayerRule, bool) {
	if block {
		if len(pa.TrimWhitespace(prelude)) == 0 {
			return LayerRule{Block: true}, true
		}
		name, ok := ParseLayerName(prelude)
		return LayerRule{Names: [][]string{name}, Block: true}, ok
	}
	var out LayerRule
	for _, part := range pa.SplitOnComma(prelude) {
		name, ok := ParseLayerName(part)
		if !ok {
			return LayerRule{}, false
		}
		out.Names = append(out.Names, name)
	}
	return out, len(out.Names) > 0
}

// Declare declares the layers of the rule nested in parent, and returns
// the layer of the rules of a block (parent for a statement).
func (r LayerRule) Declare(layers *Layers, parent int) int {
	if r.Block && len(r.Names) == 0 {
		return layers.Anonymous(parent)
	}
	layer := parent
	for _, name := range r.Names {
		layer = layers.Declare(parent, name)
	}
	if !r.Block {
		return parent
	}
	return layer
}

// ParseImportLayer reads the optional layer of an @import, at the start of
// the tokens following its URL (without whitespace): `layer` for a new
// anonymous layer, `layer(name)` for a named one, as a block [LayerRule]
// to [LayerRule.Declare], and the remaining tokens (the import
// conditions). ok is false for an invalid layer(), which makes the whole
// @import invalid.
//
// The layer is declared even if the import fails, but only when its
// conditions hold: callers check them before declaring it.
func ParseImportLayer(tokens []pa.Token) (rule LayerRule, hasLayer bool, rest []pa.Token, ok bool) {
	if len(tokens) == 0 {
		return LayerRule{}, false, tokens, true
	}
	switch first := tokens[0].(type) {
	case pa.Ident:
		if utils.AsciiLower(first.Value) == "layer" {
			return LayerRule{Block: true}, true, tokens[1:], true
		}
	case pa.FunctionBlock:
		if utils.AsciiLower(first.Name) == "layer" {
			name, ok := ParseLayerName(first.Arguments)
			return LayerRule{Names: [][]string{name}, Block: true}, true, tokens[1:], ok
		}
	}
	return LayerRule{}, false, tokens, true
}

// SortByLayer sorts matched declarations, in the order of [Matcher.Match],
// by cascade layer, given the ranks of the layers (see [Layers.Ranks]):
// declarations of later layers come last for normal declarations, of
// earlier layers for !important ones, so that applying them in order makes
// the winning one last. Within a layer the order is kept.
func SortByLayer[D any](matched []Matched[D], ranks []int, important bool) {
	key := func(m Matched[D]) int {
		if important {
			return -ranks[m.Layer]
		}
		return ranks[m.Layer]
	}
	slices.SortStableFunc(matched, func(a, b Matched[D]) int { return key(a) - key(b) })
}
