// Package cascade implements the selector-matching half of the CSS
// cascade: given rules compiled from stylesheets, it returns the
// declarations matching an element sorted in cascade order —
// (specificity, source order) ascending — so that applying them
// sequentially makes the most specific, and on ties the latest,
// declaration win.
//
// The package is document-agnostic: it depends only on package selector and
// the html node type it matches against, and is generic over the
// declaration payload, so it can back both a stringly styling layer
// (svg) and a typed one.
//
// The criteria above specificity and order belong to the caller, which
// ranks the matched declarations, from https://www.w3.org/TR/css-cascade-5/#cascade-sort:
//
//  1. origin and importance (user agent, user, author presentational hints,
//     author; !important reversed);
//  2. element-attached styles (the style attribute) over style rules of
//     the same importance;
//  3. cascade layers: each [Rule] belongs to a layer of its [Matcher]
//     ([Matched.Layer]); [Layers] gives the layer order. Later layers win
//     for normal declarations, earlier layers for !important ones;
//  4. specificity, then order of appearance: this package's sort.
//
// Matcher.Match returns the declarations of the element itself and those
// of its pseudo-elements in one slice. Each Matched carries the
// PseudoElement of its selector ("" for the element itself): callers must
// key or filter on it, never apply the whole slice to one box.
//
// Rules are indexed by the id, a class or the type of their subject (see
// [selector.Key]), so matching an element only tests the rules it may
// match.
package cascade

import (
	"cmp"
	"slices"

	"github.com/anaelorlinski/aaoo.cssengine-go/selector"
	"golang.org/x/net/html"
)

// Rule is one qualified rule: a compiled selector group and the
// declarations of its block. Order is the rule's position in the
// stylesheet source (across stylesheets, in load order); it breaks
// specificity ties. Layer is the cascade layer of the rule in the
// [Layers] of its [Matcher]: 0, the zero value, for unlayered rules.
type Rule[D any] struct {
	Selectors    selector.SelectorGroup
	Declarations []D
	Order        int
	Layer        int
}

// Matcher is an ordered set of rules and their cascade layers, indexed
// for matching. The zero value is empty and ready to use. A Matcher must
// not be modified once shared.
type Matcher[D any] struct {
	rules  []Rule[D]
	layers Layers
	index  map[selector.IndexKey][]entry
}

// entry locates one selector of one rule.
type entry struct{ rule, selector int32 }

// Add appends rule, after the rules already added.
func (m *Matcher[D]) Add(rule Rule[D]) {
	if m.index == nil {
		m.index = map[selector.IndexKey][]entry{}
	}
	r := int32(len(m.rules))
	m.rules = append(m.rules, rule)
	for s, sel := range rule.Selectors {
		key := selector.Key(sel)
		m.index[key] = append(m.index[key], entry{r, int32(s)})
	}
}

// Len returns the number of rules added.
func (m *Matcher[D]) Len() int { return len(m.rules) }

// Rules returns the rules added, in order. The slice must not be modified.
func (m *Matcher[D]) Rules() []Rule[D] { return m.rules }

// Layers returns the cascade layers of the rules, which the caller
// declares while adding rules.
func (m *Matcher[D]) Layers() *Layers { return &m.layers }

// Matched is a declaration selected for an element, carrying its
// cascade sort key and the pseudo-element of the selector that matched
// ("" for none).
type Matched[D any] struct {
	Declaration   D
	Specificity   selector.Specificity
	Order         int
	Layer         int
	PseudoElement string
}

// Match returns the declarations of every rule matching element or one of
// its pseudo-elements (see Matched.PseudoElement), in cascade order:
// (specificity, rule source order) ascending. When several selectors of
// one group match, the declarations participate once per matching
// selector, each with that selector's specificity — the ascending
// application keeps the most specific one winning.
func (m *Matcher[D]) Match(element *html.Node) []Matched[D] { return m.MatchWith(element, nil) }

// MatchWith is [Matcher.Match], sharing cache (which may be nil) between the
// matches: pass one [selector.Cache] for all the elements of a document,
// while it does not change, so that :nth-child(), :has() and the like stay
// linear in the document size.
func (m *Matcher[D]) MatchWith(element *html.Node, cache *selector.Cache) []Matched[D] {
	if element.Type != html.ElementNode || len(m.rules) == 0 {
		return nil
	}
	// the selectors the element may match: those of its keys
	var candidates []entry
	for _, key := range selector.ElementKeys(element) {
		candidates = append(candidates, m.index[key]...)
	}
	// source order, as if every rule had been tested in turn
	slices.SortFunc(candidates, func(a, b entry) int {
		if c := cmp.Compare(a.rule, b.rule); c != 0 {
			return c
		}
		return cmp.Compare(a.selector, b.selector)
	})

	var out []Matched[D]
	for _, c := range candidates {
		rule := &m.rules[c.rule]
		sel := rule.Selectors[c.selector]
		if !selector.MatchWith(sel, element, cache) {
			continue
		}
		spec, pseudo := sel.Specificity(), sel.PseudoElement()
		for _, decl := range rule.Declarations {
			out = append(out, Matched[D]{Declaration: decl, Specificity: spec, Order: rule.Order, Layer: rule.Layer, PseudoElement: pseudo})
		}
	}
	slices.SortStableFunc(out, func(a, b Matched[D]) int {
		if a.Specificity != b.Specificity {
			if a.Specificity.Less(b.Specificity) {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Order, b.Order)
	})
	return out
}
