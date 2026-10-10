package selector

import "golang.org/x/net/html"

// Relative selectors (:has()) are anchored on their leftmost compound, so
// they are evaluated left to right: "compounds[i:] match starting at e" does
// not depend on the anchor, and is memoized per document in the [Cache].
// Every element then costs O(compounds) for each relative selector, whatever
// the combinators and the number of anchors.

// forwardKey identifies a memoized result of a relative chain.
type forwardKey struct {
	chain *Sel // the chain, by identity (its compounds)
	i     int32
	n     *html.Node
	kind  uint8
}

const (
	fromHere     uint8 = iota // compounds[i:] match with compounds[i] at n
	inSubtree                 // ... at a strict descendant of n
	inFollowings              // ... at a following sibling of n
)

// relChain evaluates a relative chain left to right.
type relChain struct {
	complexSelector
	mctx *matchContext // with a cache
}

func (rc relChain) key(i int, n *html.Node, kind uint8) forwardKey {
	return forwardKey{chain: &rc.compounds[0], i: int32(i), n: n, kind: kind}
}

func (rc relChain) lookup(k forwardKey) (bool, bool) {
	v, ok := rc.mctx.cache.forward[k]
	return v, ok
}

func (rc relChain) store(k forwardKey, v bool) bool {
	rc.mctx.cache.forward[k] = v
	return v
}

// from reports whether compounds[i:] match, with compounds[i] at e.
func (rc relChain) from(i int, e *html.Node) bool {
	if i == len(rc.compounds)-1 { // a compound alone: nothing to memoize
		return matchIn(rc.compounds[i], e, rc.mctx)
	}
	k := rc.key(i, e, fromHere)
	if v, ok := rc.lookup(k); ok {
		return v
	}
	res := matchIn(rc.compounds[i], e, rc.mctx)
	if res && i < len(rc.compounds)-1 {
		switch rc.combinators[i] {
		case ' ':
			res = rc.inSubtree(i+1, e)
		case '>':
			res = rc.inChildren(i+1, e)
		case '+':
			next := nextElementSibling(e)
			res = next != nil && rc.from(i+1, next)
		case '~':
			res = rc.inFollowing(i+1, e)
		}
	}
	return rc.store(k, res)
}

// inChildren reports whether compounds[i:] match starting at a child of e.
func (rc relChain) inChildren(i int, e *html.Node) bool {
	for c := e.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && rc.from(i, c) {
			return true
		}
	}
	return false
}

// inSubtree reports whether compounds[i:] match starting at a strict
// descendant of e.
func (rc relChain) inSubtree(i int, e *html.Node) bool {
	k := rc.key(i, e, inSubtree)
	if v, ok := rc.lookup(k); ok {
		return v
	}
	res := false
	for c := e.FirstChild; c != nil && !res; c = c.NextSibling {
		if c.Type == html.ElementNode {
			res = rc.from(i, c) || rc.inSubtree(i, c)
		}
	}
	return rc.store(k, res)
}

// inFollowing reports whether compounds[i:] match starting at a following
// sibling of e. The siblings are walked from the last one, iteratively: a
// sibling list may be long.
func (rc relChain) inFollowing(i int, e *html.Node) bool {
	if rc.mctx.cache.private && i == len(rc.compounds)-1 {
		// one anchor, the last compound: scan forward, stopping at the
		// first match (the suffix table below pays off across the anchors
		// of a document, and keeps a longer chain linear)
		for s := nextElementSibling(e); s != nil; s = nextElementSibling(s) {
			if rc.from(i, s) {
				return true
			}
		}
		return false
	}
	var pending []*html.Node
	for s := e; s != nil; s = nextElementSibling(s) {
		if _, ok := rc.lookup(rc.key(i, s, inFollowings)); ok {
			break
		}
		pending = append(pending, s)
	}
	for j := len(pending) - 1; j >= 0; j-- {
		s := pending[j]
		res := false
		if next := nextElementSibling(s); next != nil {
			v, _ := rc.lookup(rc.key(i, next, inFollowings))
			res = rc.from(i, next) || v
		}
		rc.store(rc.key(i, s, inFollowings), res)
	}
	v, _ := rc.lookup(rc.key(i, e, inFollowings))
	return v
}

// relativeMatch reports whether some element matches the relative selector
// r anchored at anchor (https://drafts.csswg.org/selectors-4/#relative): its
// leftmost compound stands in the combinator relation to anchor.
func relativeMatch(anchor *html.Node, combinator byte, r *relativeSelector, mctx *matchContext) bool {
	rc := relChain{complexSelector: r.chain, mctx: mctx}
	switch combinator {
	case ' ':
		return rc.inSubtree(0, anchor)
	case '>':
		return rc.inChildren(0, anchor)
	case '+':
		next := nextElementSibling(anchor)
		return next != nil && rc.from(0, next)
	case '~':
		return rc.inFollowing(0, anchor)
	}
	return false
}

func nextElementSibling(n *html.Node) *html.Node {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode {
			return s
		}
	}
	return nil
}
