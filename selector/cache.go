package selector

import "golang.org/x/net/html"

// Cache holds results shared by the matches of a document: sibling
// positions (:nth-child() and its variants, :only-child), the results of
// :has() per anchor and of the S filter of :nth-child(An+B of S) per
// element. Matching every element of a document with one Cache makes
// these selectors linear in the document size instead of quadratic.
//
// A Cache is only valid while the document does not change, and is not
// safe for concurrent use. The zero value is not usable: see [NewCache].
type Cache struct {
	positions map[positionKey]position
	has       map[hasKey]bool
	forward   map[forwardKey]bool // see relative.go
	chains    map[contextKey]bool // see chainMatch, built on first use
	ctx       matchContext        // the context of the matches using the cache
	// the document scans of the HTML state pseudo-classes (see
	// html_states.go), built on first use
	ids              map[string]*html.Node
	idsRoot          *html.Node
	radioGroups      map[radioGroupKey]radioGroup
	defaultButtons   map[*html.Node]*html.Node
	positionRequests int // see positionOf
	// private is set for the cache of a single match (see newContext):
	// sibling positions computed for every sibling would then be wasted
	private bool
}

// context returns the match context of the cache.
func (c *Cache) context() *matchContext {
	c.ctx.cache = c
	return &c.ctx
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{positions: map[positionKey]position{}, has: map[hasKey]bool{}, forward: map[forwardKey]bool{}}
}

// MatchWith reports whether n matches sel, using and filling cache, which
// may be nil.
func MatchWith(sel Sel, n *html.Node, cache *Cache) bool {
	// the nested selectors memoize in the cache too, instead of a private
	// one per match
	if cache == nil || !usesCache(sel) && !hasNested(sel) {
		return sel.Match(n)
	}
	return matchIn(sel, n, cache.context())
}

// usesCache reports whether matching sel reads or fills a [Cache]: it has
// :nth-*(), :only-*() or :has(), possibly nested. It is computed once, when
// selectors are built: the other selectors match without a context.
func usesCache(sel Sel) bool {
	switch sel := sel.(type) {
	case compoundSelector:
		return sel.cacheable
	case complexSelector:
		return sel.cacheable
	case logicalPseudoClassSelector:
		return sel.cacheable
	case hasPseudoClassSelector, nthPseudoClassSelector, onlyChildPseudoClassSelector:
		return true
	case statePseudoClassSelector:
		return sel.usesDocumentCache()
	default:
		return false
	}
}

func groupUsesCache(group SelectorGroup) bool {
	for _, s := range group {
		if usesCache(s) {
			return true
		}
	}
	return false
}

// position is the 1-based position of an element among its sibling
// elements, from the start and from the end.
type position struct{ index, fromEnd int32 }

// positionKey identifies a position: of an element among all its sibling
// elements (filter nil, ofType false), those of its type (ofType), or
// those matching the S of :nth-child(An+B of S) (filter, by identity).
type positionKey struct {
	n      *html.Node
	filter *Sel
	ofType bool
}

// hasKey identifies the result of a relative selector of :has() at an
// anchor.
type hasKey struct {
	rel    *relativeSelector
	anchor *html.Node
}

// positionOf returns the position of n among its qualifying siblings, the
// ones of its type when ofType, or matching filter (may be nil), computing
// those of all its siblings in one pass.
func (c *Cache) positionOf(n *html.Node, ofType bool, filter SelectorGroup, mctx *matchContext) position {
	key := positionKey{n: n, ofType: ofType}
	if filter != nil {
		key.filter = &filter[0]
	}
	if p, ok := c.positions[key]; ok {
		return p
	}
	qualifies := func(e *html.Node) bool {
		if filter != nil {
			return filter.matchIn(e, mctx)
		}
		return true
	}
	c.positionRequests++
	if c.private && c.positionRequests == 1 && (filter == nil || !groupUsesCache(filter)) {
		// the first position of a single match: count its qualifying
		// siblings, storing nothing (the positions of all siblings pay off
		// across a document, or for nested filters asking again)
		var p position
		sameKind := func(e *html.Node) bool {
			return e.Type == html.ElementNode && (!ofType || e.Data == n.Data && e.Namespace == n.Namespace) && qualifies(e)
		}
		for e := n; e != nil; e = e.PrevSibling {
			if sameKind(e) {
				p.index++
			}
		}
		for e := n; e != nil; e = e.NextSibling {
			if sameKind(e) {
				p.fromEnd++
			}
		}
		return p
	}
	// counts per type, or overall
	type kind struct{ data, namespace string }
	counts := map[kind]int32{}
	kindOf := func(e *html.Node) kind {
		if ofType {
			return kind{e.Data, e.Namespace}
		}
		return kind{}
	}
	var siblings []*html.Node
	for e := n.Parent.FirstChild; e != nil; e = e.NextSibling {
		if e.Type == html.ElementNode && qualifies(e) {
			siblings = append(siblings, e)
		}
	}
	indexes := make([]int32, len(siblings))
	for i, e := range siblings {
		k := kindOf(e)
		counts[k]++
		indexes[i] = counts[k]
	}
	for i, e := range siblings {
		k := key
		k.n = e
		c.positions[k] = position{index: indexes[i], fromEnd: counts[kindOf(e)] - indexes[i] + 1}
	}
	if p, ok := c.positions[key]; ok {
		return p
	}
	// n itself does not qualify: its position counts it anyway (it is
	// only asked for once n matched the filter)
	return position{}
}
