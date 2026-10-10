package selector

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Matcher is the interface for basic selector functionality.
// Match returns whether a selector matches n.
type Matcher interface {
	Match(n *html.Node) bool
}

// Sel is the interface for all the functionality provided by selectors.
type Sel interface {
	Matcher
	Specificity() Specificity

	// Returns a CSS input compiling to this selector.
	String() string

	// Returns a pseudo-element, or an empty string.
	PseudoElement() string
}

// Parse parses a single selector,
// with support for pseudo-element.
func Parse(sel string) (Sel, error) {
	p := &parser{s: sel, acceptPseudoElements: true}
	compiled, err := p.parseSelector()
	if err != nil {
		return nil, err
	}

	if p.i < len(sel) {
		return nil, fmt.Errorf("parsing %q: %d bytes left over", sel, len(sel)-p.i)
	}

	return compiled, nil
}

// ParseGroup parses a selector, or a group of selectors separated by commas.
// It supports pseudo-elements.
func ParseGroup(sel string) (SelectorGroup, error) {
	p := &parser{s: sel, acceptPseudoElements: true}
	compiled, err := p.parseSelectorGroup()
	if err != nil {
		return nil, err
	}

	if p.i < len(sel) {
		return nil, fmt.Errorf("parsing %q: %d bytes left over", sel, len(sel)-p.i)
	}

	return compiled, nil
}

// ParseGroupStrict is [ParseGroup], an invalid item of a forgiving list
// (:is(), :where()) being an error instead of being dropped: the test of
// @supports selector() (CSS Conditional 4 §2.1, "unsupported" selectors).
func ParseGroupStrict(sel string) (SelectorGroup, error) {
	p := &parser{s: sel, acceptPseudoElements: true, strict: true}
	compiled, err := p.parseSelectorGroup()
	if err != nil {
		return nil, err
	}
	if p.i < len(sel) {
		return nil, fmt.Errorf("parsing %q: %d bytes left over", sel, len(sel)-p.i)
	}
	return compiled, nil
}

// MustCompile is like ParseGroup, but panics instead of returning an error.
func MustCompile(sel string) SelectorGroup {
	compiled, err := ParseGroup(sel)
	if err != nil {
		panic(err)
	}
	return compiled
}

// MatchAll returns a slice of the nodes that match the selector,
// from n and its children.
func MatchAll(n *html.Node, m Matcher) []*html.Node {
	return matchAllInto(n, nil, m)
}

func matchAllInto(n *html.Node, storage []*html.Node, m Matcher) []*html.Node {
	if m.Match(n) {
		storage = append(storage, n)
	}

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		storage = matchAllInto(child, storage, m)
	}

	return storage
}

func queryInto(n *html.Node, m Matcher, storage []*html.Node) []*html.Node {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if m.Match(child) {
			storage = append(storage, child)
		}
		storage = queryInto(child, m, storage)
	}

	return storage
}

// QueryAll returns a slice of all the nodes that match m, from the descendants
// of n.
func QueryAll(n *html.Node, m Matcher) []*html.Node {
	return queryInto(n, m, nil)
}

// MatchFirst returns the first node that matches s, from n and its children.
func MatchFirst(n *html.Node, m Matcher) *html.Node {
	if m.Match(n) {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		m := MatchFirst(c, m)
		if m != nil {
			return m
		}
	}
	return nil
}

// Query returns the first node that matches m, from the descendants of n.
// If none matches, it returns nil.
func Query(n *html.Node, m Matcher) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if m.Match(c) {
			return c
		}
		if matched := Query(c, m); matched != nil {
			return matched
		}
	}

	return nil
}

// Filter returns the nodes that match m.
func Filter(nodes []*html.Node, m Matcher) (result []*html.Node) {
	for _, n := range nodes {
		if m.Match(n) {
			result = append(result, n)
		}
	}
	return result
}

// Namespaces.
//
// The engine does not model @namespace rules, so a selector without a
// namespace prefix matches elements in any namespace (the behaviour without a
// default namespace) and attributes in no namespace. Explicit prefixes are
// resolved against the fixed table below, which uses the namespace names of
// golang.org/x/net/html: HTML elements carry the empty Namespace, SVG "svg",
// MathML "math", and adjusted foreign attributes "xlink", "xml" or "xmlns".
// Any other prefix is an undeclared prefix, and makes the selector invalid.

// anyNamespace is the namespace constraint of "*|" (and of an element
// selector without prefix).
const anyNamespace = "*"

// htmlNamespace is the name used for the HTML namespace in selectors, whose
// elements have an empty Namespace in x/net/html.
const htmlNamespace = "html"

var knownNamespacePrefixes = map[string]string{
	"html":   htmlNamespace,
	"svg":    "svg",
	"math":   "math",
	"mathml": "math",
	"xlink":  "xlink",
	"xml":    "xml",
	"xmlns":  "xmlns",
}

// isHTMLElement reports whether n is an element of the HTML namespace, whose
// tag and attribute names are matched ASCII case-insensitively.
func isHTMLElement(n *html.Node) bool {
	return n.Namespace == "" || n.Namespace == htmlNamespace
}

func elementNamespace(n *html.Node) string {
	if n.Namespace == "" {
		return htmlNamespace
	}
	return n.Namespace
}

// tagSelector matches elements by namespace and local name. An empty name
// is the universal selector restricted to a namespace (e.g. "svg|*").
type tagSelector struct {
	tagS  string    // the name as written; matched as is outside the HTML namespace
	lower string    // the ASCII-lowercased name, matched against HTML elements
	tag   atom.Atom // the atom of lower, or 0
	ns    string    // anyNamespace, "" for no namespace, or a namespace name
}

func newTagSelector(tag string) tagSelector {
	lower := toLowerASCII(tag)
	return tagSelector{tagS: tag, lower: lower, tag: atom.Lookup([]byte(lower)), ns: anyNamespace}
}

// Matches elements with a given tag name: ASCII case-insensitively for HTML
// elements, case-sensitively otherwise (SVG keeps camelCase names such as
// linearGradient).
func (t tagSelector) Match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if t.ns != anyNamespace && elementNamespace(n) != t.ns {
		return false
	}
	if t.tagS == "" {
		return true
	}
	if isHTMLElement(n) {
		if t.tag != 0 && n.DataAtom != 0 {
			return n.DataAtom == t.tag
		}
		return n.Data == t.lower
	}
	return n.Data == t.tagS
}

func (c tagSelector) Specificity() Specificity {
	if c.tagS == "" {
		return Specificity{}
	}
	return Specificity{0, 0, 1}
}

func (c tagSelector) PseudoElement() string {
	return ""
}

type classSelector struct {
	class string
}

// Matches elements by class attribute.
func (t classSelector) Match(n *html.Node) bool {
	return matchAttribute(n, "class", func(s string) bool {
		return matchInclude(t.class, s, false)
	})
}

func (c classSelector) Specificity() Specificity {
	return Specificity{0, 1, 0}
}

func (c classSelector) PseudoElement() string {
	return ""
}

type idSelector struct {
	id string
}

// Matches elements by id attribute.
func (t idSelector) Match(n *html.Node) bool {
	return matchAttribute(n, "id", func(s string) bool {
		return s == t.id
	})
}

func (c idSelector) Specificity() Specificity {
	return Specificity{1, 0, 0}
}

func (c idSelector) PseudoElement() string {
	return ""
}

type attrSelector struct {
	regexp              *regexp.Regexp
	key, val, operation string
	lowerKey            string // the ASCII-lowercased key, matched on HTML elements
	ns                  string // anyNamespace, "" (no namespace) or a namespace name
	ignoreCase          bool   // the "i" flag
	caseSensitive       bool   // the "s" flag (only kept for serialization)
	// without the "s" flag, an attribute of HTML whose value is compared
	// ASCII case-insensitively on HTML elements
	htmlIgnoreCase bool
}

// htmlCaseInsensitiveAttributes are the attributes whose values selectors
// match ASCII case-insensitively on HTML elements, without the "s" flag
// (HTML §4.16.2, https://html.spec.whatwg.org/multipage/semantics-other.html#case-sensitivity-of-selectors).
var htmlCaseInsensitiveAttributes = map[string]bool{
	"accept": true, "accept-charset": true, "align": true, "alink": true, "axis": true,
	"bgcolor": true, "charset": true, "checked": true, "clear": true, "codetype": true,
	"color": true, "compact": true, "declare": true, "defer": true, "dir": true,
	"direction": true, "disabled": true, "enctype": true, "face": true, "frame": true,
	"hreflang": true, "http-equiv": true, "lang": true, "language": true, "link": true,
	"media": true, "method": true, "multiple": true, "nohref": true, "noresize": true,
	"noshade": true, "nowrap": true, "readonly": true, "rel": true, "rev": true,
	"rules": true, "scope": true, "scrolling": true, "selected": true, "shape": true,
	"target": true, "text": true, "type": true, "valign": true, "valuetype": true,
	"vlink": true,
}

// matchAttr reports whether n has an attribute selected by the name and
// namespace of t, whose value satisfies f. Names are matched ASCII
// case-insensitively on HTML elements, case-sensitively otherwise.
func (t attrSelector) matchAttr(n *html.Node, f func(string) bool) bool {
	isHTML := isHTMLElement(n)
	for _, a := range n.Attr {
		if t.ns != anyNamespace && a.Namespace != t.ns {
			continue
		}
		if (isHTML && a.Key == t.lowerKey || !isHTML && a.Key == t.key) && f(a.Val) {
			return true
		}
	}
	return false
}

// Matches elements by attribute value.
func (t attrSelector) Match(n *html.Node) bool {
	val, ignoreCase := t.val, t.ignoreCase || t.htmlIgnoreCase && isHTMLElement(n)
	switch t.operation {
	case "":
		return t.matchAttr(n, func(string) bool { return true })
	case "=":
		return t.matchAttr(n, func(s string) bool { return matchInsensitiveValue(s, val, ignoreCase) })
	case "!=":
		// matches elements where the attribute does not have the value val
		return n.Type == html.ElementNode && !t.matchAttr(n, func(s string) bool { return matchInsensitiveValue(s, val, ignoreCase) })
	case "~=":
		// matches elements where the attribute is a whitespace-separated list that includes val.
		return t.matchAttr(n, func(s string) bool { return matchInclude(val, s, ignoreCase) })
	case "|=":
		// matches elements where the attribute equals val or starts with val plus a hyphen.
		return t.matchAttr(n, func(s string) bool {
			return matchInsensitiveValue(s, val, ignoreCase) ||
				len(s) > len(val) && s[len(val)] == '-' && matchInsensitiveValue(s[:len(val)], val, ignoreCase)
		})
	case "^=":
		return val != "" && t.matchAttr(n, func(s string) bool {
			return len(s) >= len(val) && matchInsensitiveValue(s[:len(val)], val, ignoreCase)
		})
	case "$=":
		return val != "" && t.matchAttr(n, func(s string) bool {
			return len(s) >= len(val) && matchInsensitiveValue(s[len(s)-len(val):], val, ignoreCase)
		})
	case "*=":
		return val != "" && t.matchAttr(n, func(s string) bool {
			if ignoreCase {
				return strings.Contains(toLowerASCII(s), toLowerASCII(val))
			}
			return strings.Contains(s, val)
		})
	case "#=":
		return t.matchAttr(n, t.regexp.MatchString)
	default:
		panic(fmt.Sprintf("unsuported operation : %s", t.operation))
	}
}

// check for equality between `s1` and `s2`, ignoring ASCII case if
// `ignoreCase` is true (Selectors 4 §6.3: the "i" flag is ASCII
// case-insensitive, not Unicode case folding).
func matchInsensitiveValue(s1 string, s2 string, ignoreCase bool) bool {
	if ignoreCase {
		return equalFoldASCII(s1, s2)
	}
	return s1 == s2
}

// equalFoldASCII reports whether s and t are equal under ASCII case folding.
func equalFoldASCII(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		a, b := s[i], t[i]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

// matches elements where the attribute named key (in no namespace)
// satisifes the function f.
func matchAttribute(n *html.Node, key string, f func(string) bool) bool {
	for _, a := range n.Attr {
		if a.Key == key && a.Namespace == "" && f(a.Val) {
			return true
		}
	}
	return false
}

// asciiSet is a 32-byte value, where each bit represents the presence of a
// given ASCII character in the set. The 128-bits of the lower 16 bytes,
// starting with the least-significant bit of the lowest word to the
// most-significant bit of the highest word, map to the full range of all
// 128 ASCII characters. The 128-bits of the upper 16 bytes will be zeroed,
// ensuring that any non-ASCII character will be reported as not in the set.
type asciiSet [8]uint32

// makeASCIISet creates a set of ASCII characters and reports whether all
// characters in chars are ASCII.
func makeASCIISet(chars string) (as asciiSet) {
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if c >= utf8.RuneSelf {
			panic("non ascii input")
		}
		as[c>>5] |= 1 << uint(c&31)
	}
	return as
}

// contains reports whether c is inside the set.
func (as *asciiSet) contains(c byte) bool {
	return (as[c>>5] & (1 << uint(c&31))) != 0
}

func (as *asciiSet) index(s string) int {
	for i := 0; i < len(s); i++ {
		if as.contains(s[i]) {
			return i
		}
	}
	return -1
}

var spaceAsciiSet = makeASCIISet(" \t\r\n\f")

// returns true if s is a whitespace-separated list that includes val.
// An empty val, or one containing whitespace, never matches.
func matchInclude(val, s string, ignoreCase bool) bool {
	if val == "" || spaceAsciiSet.index(val) != -1 {
		return false
	}
	for s != "" {
		i := spaceAsciiSet.index(s)
		if i == -1 {
			return matchInsensitiveValue(s, val, ignoreCase)
		}
		if matchInsensitiveValue(s[:i], val, ignoreCase) {
			return true
		}
		s = s[i+1:]
	}
	return false
}

func (c attrSelector) Specificity() Specificity {
	return Specificity{0, 1, 0}
}

func (c attrSelector) PseudoElement() string {
	return ""
}

// see pseudo_classes.go for pseudo classes selectors

// on a static context, some selectors can't match anything
type neverMatchSelector struct {
	value string
}

func (s neverMatchSelector) Match(n *html.Node) bool {
	return false
}

func (s neverMatchSelector) Specificity() Specificity {
	return Specificity{0, 0, 0}
}

func (c neverMatchSelector) PseudoElement() string {
	return ""
}

type compoundSelector struct {
	pseudoElement string
	selectors     []Sel
	nested        bool // see [hasNested]
	cacheable     bool // see [usesCache]
}

func newCompoundSelector(selectors []Sel, pseudoElement string) compoundSelector {
	out := compoundSelector{selectors: selectors, pseudoElement: pseudoElement}
	for _, s := range selectors {
		out.nested = out.nested || hasNested(s)
		out.cacheable = out.cacheable || usesCache(s)
	}
	return out
}

// Matches elements if each sub-selectors matches.
func (t compoundSelector) Match(n *html.Node) bool { return t.matchIn(n, newContext(t.nested)) }

func (t compoundSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	if len(t.selectors) == 0 {
		return n.Type == html.ElementNode
	}

	for _, sel := range t.selectors {
		if !matchIn(sel, n, mctx) {
			return false
		}
	}
	return true
}

func (s compoundSelector) Specificity() Specificity {
	var out Specificity
	for _, sel := range s.selectors {
		out = out.Add(sel.Specificity())
	}
	if s.pseudoElement != "" {
		// https://drafts.csswg.org/selectors-3/#specificity
		out = out.Add(Specificity{0, 0, 1})
	}
	return out
}

func (c compoundSelector) PseudoElement() string {
	return c.pseudoElement
}

// complexSelector is a sequence of compound selectors joined by
// combinators: compounds[0] combinators[0] compounds[1] ... compounds[k].
// The last compound is the subject.
type complexSelector struct {
	compounds   []Sel
	combinators []byte // len(compounds)-1 of ' ', '>', '+', '~'
	branching   int    // number of ' ' and '~' combinators
	nested      bool   // a compound has complex selector arguments (see [hasNested])
	cacheable   bool   // see [usesCache]
}

func newComplexSelector(compounds []Sel, combinators []byte) complexSelector {
	out := complexSelector{compounds: compounds, combinators: combinators}
	for _, c := range combinators {
		if c == ' ' || c == '~' {
			out.branching++
		}
	}
	for _, c := range compounds {
		out.nested = out.nested || hasNested(c)
		out.cacheable = out.cacheable || usesCache(c)
	}
	return out
}

// asComplex returns sel as a complex selector (of one compound, if needed).
func asComplex(sel Sel) complexSelector {
	if c, ok := sel.(complexSelector); ok {
		return c
	}
	return complexSelector{compounds: []Sel{sel}}
}

func (s complexSelector) Match(n *html.Node) bool {
	return s.matchIn(n, newContext(s.nested))
}

func (s complexSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	// memoizing pays off with several branching combinators, with nested
	// selectors, or for a chain nested in another match (evaluated at many
	// elements); not for a plain top-level match
	useMemo := s.branching >= 2 || s.nested || mctx != nil && s.branching >= 1
	m := chainMatch{complexSelector: s, mctx: mctx, useMemo: useMemo}
	return m.matches(len(s.compounds)-1, n)
}

// matchContext is shared by the selectors nested in one top-level match: the
// arguments of :is(), :where(), :not(), :has() and :nth-child(... of S),
// whose chains memoise into one table.
//
// Without it, every evaluation of a nested complex selector starts from an
// empty memo, so a match costs O(depth^(k+1)) for k nested levels, such as
// ":is(:is(:is(x div) div) div) span" (26 s for one element at depth 200,
// k = 4). With it, each (chain, compound, element) is evaluated at most once
// per top-level match.
//
// The DOM does not change during a match and selectors are pure, so the
// results do not depend on where in the match they are computed; contextKey
// carries what they do depend on (the chain).
type matchContext struct {
	cache *Cache // results valid across the matches of a document (chains too)
}

// contextKey is a chainKey in a [matchContext], which entries of distinct
// chains share: it adds the chain, by identity (its compounds).
type contextKey struct {
	chain *Sel
	chainKey
}

// nestedMatcher is implemented by the selectors that may contain other
// selectors, to share the context of the match they are part of.
type nestedMatcher interface {
	matchIn(n *html.Node, mctx *matchContext) bool
}

// matchIn matches sel at n within mctx (nil outside a nested match).
func matchIn(sel Sel, n *html.Node, mctx *matchContext) bool {
	if mctx != nil {
		if nm, ok := sel.(nestedMatcher); ok {
			return nm.matchIn(n, mctx)
		}
	}
	return sel.Match(n)
}

// hasNested reports whether sel has selector arguments with combinators
// (or relative ones, for :has()), which a [matchContext] makes linear:
// arguments made of compounds only cost the same with or without it. It is
// computed once, when selectors are built.
func hasNested(sel Sel) bool {
	switch sel := sel.(type) {
	case compoundSelector:
		return sel.nested
	case complexSelector:
		return sel.nested
	case logicalPseudoClassSelector:
		return sel.nested
	case hasPseudoClassSelector:
		return true
	case nthPseudoClassSelector:
		return sel.nested
	default:
		return false
	}
}

func groupHasNested(group SelectorGroup) bool {
	for _, s := range group {
		if c, ok := s.(complexSelector); ok && len(c.compounds) > 1 || hasNested(s) {
			return true
		}
	}
	return false
}

// newContext returns a context for a top-level match of a selector, or nil
// if it does not need one (see [hasNested]).
func newContext(nested bool) *matchContext {
	if nested {
		cache := NewCache()
		cache.private = true
		return cache.context()
	}
	return nil
}

// chainMatch matches a complex selector right to left.
//
// Combinators that can be satisfied by several elements (descendant ' ' and
// subsequent-sibling '~') make naive backtracking exponential in their
// number: "x div div div span" re-runs "x div div" at every ancestor of
// every candidate. During a single match the DOM does not change and
// selectors are pure, so chainMatch memoises:
//
//   - matches(i, e): compounds[0..i] match with compounds[i] at e;
//   - anyAncestor(i, e): matches(i, a) for some ancestor a of e;
//   - anyPrevSibling(i, e): matches(i, s) for some preceding sibling s of e.
//
// Each entry is computed at most once per (i, element), so a match costs
// O(compounds × visited elements) instead of O(depth^compounds). The memo
// is only allocated when it can pay off: with at least two branching
// combinators, or in a nested match; and only once the subject compound has
// matched. (Relative selectors, of :has(), are evaluated left to right
// instead: see relative.go.)
type chainMatch struct {
	complexSelector
	useMemo bool
	memo    map[chainKey]bool // when mctx is nil
	mctx    *matchContext     // the context of a nested match, or nil
}

type chainKey struct {
	n    *html.Node
	i    int32
	kind uint8
}

const (
	keyMatches uint8 = iota
	keyAncestor
	keyPrevSibling
)

func (m *chainMatch) lookup(k chainKey) (v, ok bool) {
	if m.mctx != nil {
		v, ok = m.mctx.cache.chains[contextKey{&m.compounds[0], k}]
	} else {
		v, ok = m.memo[k]
	}
	return v, ok
}

func (m *chainMatch) store(k chainKey, v bool) bool {
	if !m.useMemo {
		return v
	}
	if m.mctx != nil {
		if m.mctx.cache.chains == nil {
			m.mctx.cache.chains = make(map[contextKey]bool)
		}
		m.mctx.cache.chains[contextKey{&m.compounds[0], k}] = v
	} else {
		if m.memo == nil {
			m.memo = make(map[chainKey]bool)
		}
		m.memo[k] = v
	}
	return v
}

// parent returns the parent element of e, or nil at the top of the tree.
func (m *chainMatch) parent(e *html.Node) *html.Node {
	p := e.Parent
	if p == nil || p.Type != html.ElementNode {
		return nil
	}
	return p
}

// matches reports whether compounds[0..i] match, with compounds[i] at e.
func (m *chainMatch) matches(i int, e *html.Node) bool {
	last := i == len(m.compounds)-1
	k := chainKey{e, int32(i), keyMatches}
	if !last {
		if v, ok := m.lookup(k); ok {
			return v
		}
	}
	res := matchIn(m.compounds[i], e, m.mctx)
	if res && i > 0 {
		{
			switch m.combinators[i-1] {
			case ' ':
				res = m.anyAncestor(i-1, e)
			case '>':
				p := m.parent(e)
				res = p != nil && m.matches(i-1, p)
			case '+':
				p := prevElementSibling(e)
				res = p != nil && m.matches(i-1, p)
			case '~':
				res = m.anyPrevSibling(i-1, e)
			default:
				panic("unknown combinator")
			}
		}
	}
	if last {
		return res
	}
	return m.store(k, res)
}

func (m *chainMatch) anyAncestor(i int, e *html.Node) bool {
	p := m.parent(e)
	if p == nil {
		return false
	}
	k := chainKey{e, int32(i), keyAncestor}
	if v, ok := m.lookup(k); ok {
		return v
	}
	return m.store(k, m.matches(i, p) || m.anyAncestor(i, p))
}

func (m *chainMatch) anyPrevSibling(i int, e *html.Node) bool {
	p := prevElementSibling(e)
	if p == nil {
		return false
	}
	k := chainKey{e, int32(i), keyPrevSibling}
	if v, ok := m.lookup(k); ok {
		return v
	}
	return m.store(k, m.matches(i, p) || m.anyPrevSibling(i, p))
}

func prevElementSibling(n *html.Node) *html.Node {
	for p := n.PrevSibling; p != nil; p = p.PrevSibling {
		if p.Type == html.ElementNode {
			return p
		}
	}
	return nil
}

func (s complexSelector) Specificity() Specificity {
	var spec Specificity
	for _, c := range s.compounds {
		spec = spec.Add(c.Specificity())
	}
	return spec
}

// a pseudo-element only makes sense on the last compound (the parser
// rejects it elsewhere).
func (c complexSelector) PseudoElement() string {
	return c.compounds[len(c.compounds)-1].PseudoElement()
}

// A SelectorGroup is a list of selectors, which matches if any of the
// individual selectors matches.
type SelectorGroup []Sel

// Match returns true if the node matches one of the single selectors.
func (s SelectorGroup) Match(n *html.Node) bool {
	for _, sel := range s {
		if sel.Match(n) {
			return true
		}
	}
	return false
}

func (s SelectorGroup) matchIn(n *html.Node, mctx *matchContext) bool {
	for _, sel := range s {
		if matchIn(sel, n, mctx) {
			return true
		}
	}
	return false
}
