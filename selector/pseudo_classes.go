package selector

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// This file implements the pseudo classes selectors,
// which share the implementation of PseudoElement() and Specificity()

type abstractPseudoClass struct{}

func (s abstractPseudoClass) Specificity() Specificity {
	return Specificity{0, 1, 0}
}

func (c abstractPseudoClass) PseudoElement() string {
	return ""
}

// logicalPseudoClassSelector implements :is(), :where() and :not(), whose
// argument is a plain selector list.
type logicalPseudoClassSelector struct {
	name      string // one of "is", "where", "not"
	match     SelectorGroup
	nested    bool // see [hasNested]
	cacheable bool // see [usesCache]
}

func (s logicalPseudoClassSelector) Match(n *html.Node) bool {
	return s.matchIn(n, newContext(s.nested))
}

func (s logicalPseudoClassSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch s.name {
	case "is", "where":
		return s.match.matchIn(n, mctx)
	case "not":
		// matches elements that do not match a.
		return !s.match.matchIn(n, mctx)
	default:
		panic(fmt.Sprintf("unsupported logical pseudo class selector : %s", s.name))
	}
}

// Specificity returns the specificity of the most specific selectors
// in the pseudo-class arguments.
// See https://www.w3.org/TR/selectors/#specificity-rules
func (s logicalPseudoClassSelector) Specificity() Specificity {
	// :where() never contributes to specificity.
	if s.name == "where" {
		return Specificity{}
	}
	return maxSpecificity(s.match)
}

func (c logicalPseudoClassSelector) PseudoElement() string {
	return ""
}

// relativeSelector is a selector that may start with a combinator, as used
// inside :has() / :haschild(). combinator 0 means the implicit combinator
// for that pseudo-class (descendant for :has, child for :haschild).
type relativeSelector struct {
	combinator byte
	sel        Sel
	chain      complexSelector // sel as a chain, built once (its identity keys the cache)
}

// hasPseudoClassSelector implements :has() and the non-standard :haschild(),
// whose argument is a list of relative selectors.
type hasPseudoClassSelector struct {
	name string // one of "has", "haschild"
	rels []relativeSelector
}

func (s hasPseudoClassSelector) Match(n *html.Node) bool {
	return s.matchIn(n, newContext(true))
}

func (s hasPseudoClassSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for i := range s.rels {
		r := &s.rels[i]
		comb := r.combinator
		if comb == 0 {
			if s.name == "haschild" {
				comb = '>'
			} else {
				comb = ' '
			}
		}
		if mctx == nil || mctx.cache == nil {
			mctx = newContext(true)
		}
		key := hasKey{rel: r, anchor: n}
		matched, known := mctx.cache.has[key]
		if !known {
			matched = relativeMatch(n, comb, r, mctx)
			mctx.cache.has[key] = matched
		}
		if matched {
			return true
		}
	}
	return false
}

// Specificity returns the specificity of the most specific selectors
// in the pseudo-class arguments.
// See https://www.w3.org/TR/selectors/#specificity-rules
func (s hasPseudoClassSelector) Specificity() Specificity {
	var max Specificity
	for _, r := range s.rels {
		newSpe := r.sel.Specificity()
		if max.Less(newSpe) {
			max = newSpe
		}
	}
	return max
}

func (c hasPseudoClassSelector) PseudoElement() string {
	return ""
}

func maxSpecificity(group SelectorGroup) Specificity {
	var max Specificity
	for _, sel := range group {
		newSpe := sel.Specificity()
		if max.Less(newSpe) {
			max = newSpe
		}
	}
	return max
}

type containsPseudoClassSelector struct {
	abstractPseudoClass
	value string
	own   bool
}

func (s containsPseudoClassSelector) Match(n *html.Node) bool {
	var text string
	if s.own {
		// matches nodes that directly contain the given text
		text = strings.ToLower(nodeOwnText(n))
	} else {
		// matches nodes that contain the given text.
		text = strings.ToLower(nodeText(n))
	}
	return strings.Contains(text, s.value)
}

type regexpPseudoClassSelector struct {
	abstractPseudoClass
	regexp *regexp.Regexp
	own    bool
}

func (s regexpPseudoClassSelector) Match(n *html.Node) bool {
	var text string
	if s.own {
		// matches nodes whose text directly matches the specified regular expression
		text = nodeOwnText(n)
	} else {
		// matches nodes whose text matches the specified regular expression
		text = nodeText(n)
	}
	return s.regexp.MatchString(text)
}

// writeNodeText writes the text contained in n and its descendants to b.
func writeNodeText(n *html.Node, b *bytes.Buffer) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
	case html.ElementNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			writeNodeText(c, b)
		}
	}
}

// nodeText returns the text contained in n and its descendants.
func nodeText(n *html.Node) string {
	var b bytes.Buffer
	writeNodeText(n, &b)
	return b.String()
}

// nodeOwnText returns the contents of the text nodes that are direct
// children of n.
func nodeOwnText(n *html.Node) string {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

type nthPseudoClassSelector struct {
	abstractPseudoClass
	a, b         int
	last, ofType bool
	of           SelectorGroup // the "of S" filter of :nth-child(An+B of S), or nil
	nested       bool          // see [hasNested]
}

// Match implements :nth-child(an+b [of S]) and its variants: n is selected if
// its 1-based position i among its qualifying siblings (counted from the end
// if last) is a*k+b for some k >= 0.
func (s nthPseudoClassSelector) Match(n *html.Node) bool {
	return s.matchIn(n, newContext(s.nested))
}

func (s nthPseudoClassSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	if n.Type != html.ElementNode {
		return false
	}
	parent := n.Parent
	if parent == nil {
		return false
	}
	if s.of != nil && !s.of.matchIn(n, mctx) {
		return false
	}
	// positions pay off with a document cache, or when the counting below
	// cannot stop early
	if mctx != nil && mctx.cache != nil && (!mctx.cache.private || s.a > 0) {
		p := mctx.cache.positionOf(n, s.ofType, s.of, mctx)
		i := p.index
		if s.last {
			i = p.fromEnd
		}
		return nthMatches(s.a, s.b, int(i))
	}
	qualifies := func(c *html.Node) bool {
		if c.Type != html.ElementNode {
			return false
		}
		if s.ofType {
			return c.Data == n.Data && c.Namespace == n.Namespace
		}
		return s.of == nil || c == n || s.of.matchIn(c, mctx)
	}

	// with a <= 0, the matching positions are at most b: counting can stop
	// past it (O(1) for :first-child, :last-child)
	bounded := s.a <= 0
	i := 0
	next := func(c *html.Node) *html.Node { return c.PrevSibling }
	if s.last {
		next = func(c *html.Node) *html.Node { return c.NextSibling }
	}
	for c := n; c != nil; c = next(c) {
		if qualifies(c) {
			i++
			if bounded && i > s.b {
				return false
			}
		}
	}
	return nthMatches(s.a, s.b, i)
}

// nthMatches reports whether i = a*k + b for some integer k >= 0. The
// arithmetic is done on 64 bits: the parser clamps a and b to 32 bits, so
// i - b cannot overflow.
func nthMatches(a, b, i int) bool {
	d := int64(i) - int64(b)
	if a == 0 {
		return d == 0
	}
	return d%int64(a) == 0 && d/int64(a) >= 0
}

// Specificity of :nth-child(An+B of S) is a pseudo-class plus the most
// specific selector of S.
func (s nthPseudoClassSelector) Specificity() Specificity {
	return Specificity{0, 1, 0}.Add(maxSpecificity(s.of))
}

type onlyChildPseudoClassSelector struct {
	abstractPseudoClass
	ofType bool
}

// Match implements :only-child.
// If `ofType` is true, it implements :only-of-type instead.
func (s onlyChildPseudoClassSelector) Match(n *html.Node) bool { return s.matchIn(n, nil) }

func (s onlyChildPseudoClassSelector) matchIn(n *html.Node, mctx *matchContext) bool {
	if n.Type != html.ElementNode {
		return false
	}

	parent := n.Parent
	if parent == nil {
		return false
	}
	if mctx != nil && mctx.cache != nil {
		p := mctx.cache.positionOf(n, s.ofType, nil, mctx)
		return p.index == 1 && p.fromEnd == 1
	}

	count := 0
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || s.ofType && (c.Data != n.Data || c.Namespace != n.Namespace) {
			continue
		}
		count++
		if count > 1 {
			return false
		}
	}

	return count == 1
}

type inputPseudoClassSelector struct {
	abstractPseudoClass
}

// Matches input, select, textarea and button elements.
func (s inputPseudoClassSelector) Match(n *html.Node) bool {
	return n.Type == html.ElementNode && (n.Data == "input" || n.Data == "select" || n.Data == "textarea" || n.Data == "button")
}

type emptyElementPseudoClassSelector struct {
	abstractPseudoClass
}

// Matches empty elements.
func (s emptyElementPseudoClassSelector) Match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.ElementNode:
			return false
		case html.TextNode:
			if strings.TrimSpace(c.Data) == "" {
				continue
			} else {
				return false
			}
		}
	}

	return true
}

type rootPseudoClassSelector struct {
	abstractPseudoClass
}

// Match implements :root
// "In HTML, :root represents the <html> element and is identical to the selector html"
func (s rootPseudoClassSelector) Match(n *html.Node) bool {
	return n.Type == html.ElementNode && n.DataAtom == atom.Html
}

func hasAttr(n *html.Node, attr string) bool {
	return matchAttribute(n, attr, func(string) bool { return true })
}

type linkPseudoClassSelector struct {
	abstractPseudoClass
}

// Match implements :link
func (s linkPseudoClassSelector) Match(n *html.Node) bool {
	// a, area and, as browsers and the W3C selectors suite do, link
	return (n.DataAtom == atom.A || n.DataAtom == atom.Area || n.DataAtom == atom.Link) && hasAttr(n, "href")
}

// langPseudoClassSelector implements :lang(), with Selectors 4 language
// ranges matched by implicit-wildcard prefix ("en" matches "en" and "en-US"),
// ASCII case-insensitively, against the content language of the element.
type langPseudoClassSelector struct {
	abstractPseudoClass
	ranges []string // ASCII-lowercased
}

func (s langPseudoClassSelector) Match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	lang, ok := contentLanguage(n)
	if !ok || lang == "" {
		return false
	}
	tag := strings.Split(toLowerASCII(lang), "-")
	for _, r := range s.ranges {
		if extendedFilter(strings.Split(r, "-"), tag) {
			return true
		}
	}
	return false
}

// extendedFilter reports whether the language tag matches the language
// range, both split in subtags (lowercased), by the extended filtering of
// RFC 4647 §3.3.2, which Selectors 4 uses for :lang(): a "*" subtag
// matches any sequence of subtags, and subtags of the tag may be skipped,
// but not across a singleton ("de-*-CH" matches "de-Latn-CH", "de-CH",
// not "de-x-CH").
func extendedFilter(langRange, tag []string) bool {
	if langRange[0] != "*" && langRange[0] != tag[0] {
		return false
	}
	i, j := 1, 1
	for i < len(langRange) {
		switch {
		case langRange[i] == "*":
			i++
		case j >= len(tag):
			return false
		case langRange[i] == tag[j]:
			i++
			j++
		case len(tag[j]) == 1: // a singleton ends the skippable subtags
			return false
		default:
			j++
		}
	}
	return true
}

// contentLanguage returns the language of n: the xml:lang or lang attribute
// of the nearest element, n included, which has one. An empty value means
// "unknown" and stops the search.
func contentLanguage(n *html.Node) (string, bool) {
	for e := n; e != nil && e.Type == html.ElementNode; e = e.Parent {
		var lang string
		found := false
		for _, a := range e.Attr {
			if a.Namespace == "xml" && a.Key == "lang" {
				return a.Val, true // xml:lang wins over lang
			}
			if a.Namespace == "" && a.Key == "lang" {
				lang, found = a.Val, true
			}
		}
		if found {
			return lang, true
		}
	}
	return "", false
}

type enabledPseudoClassSelector struct {
	abstractPseudoClass
}

// :enabled and :disabled match the elements that can be disabled, as
// defined by HTML §4.16.3 (https://html.spec.whatwg.org/multipage/semantics-other.html#concept-element-disabled):
// button, input, select, textarea, fieldset, optgroup, option (and the
// obsolete menuitem). Links are not.

func (s enabledPseudoClassSelector) Match(n *html.Node) bool {
	return n.Type == html.ElementNode && canBeDisabled(n) && !actuallyDisabled(n)
}

type disabledPseudoClassSelector struct {
	abstractPseudoClass
}

func (s disabledPseudoClassSelector) Match(n *html.Node) bool {
	return n.Type == html.ElementNode && canBeDisabled(n) && actuallyDisabled(n)
}

func canBeDisabled(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Button, atom.Input, atom.Select, atom.Textarea, atom.Fieldset,
		atom.Optgroup, atom.Option, atom.Menuitem:
		return true
	}
	return false
}

// actuallyDisabled reports whether the element is disabled: by its disabled
// attribute, by a disabled fieldset ancestor (outside its first legend) for
// form controls and fieldsets, or by a disabled optgroup parent for options.
func actuallyDisabled(n *html.Node) bool {
	if hasAttr(n, "disabled") {
		return true
	}
	switch n.DataAtom {
	case atom.Button, atom.Input, atom.Select, atom.Textarea, atom.Fieldset:
		return inDisabledFieldset(n)
	case atom.Option:
		p := n.Parent
		return p != nil && p.Type == html.ElementNode && p.DataAtom == atom.Optgroup && hasAttr(p, "disabled")
	}
	return false
}

func hasLegendInPreviousSiblings(n *html.Node) bool {
	for s := n.PrevSibling; s != nil; s = s.PrevSibling {
		if s.DataAtom == atom.Legend {
			return true
		}
	}
	return false
}

func inDisabledFieldset(n *html.Node) bool {
	if n.Parent == nil {
		return false
	}
	if n.Parent.DataAtom == atom.Fieldset && hasAttr(n.Parent, "disabled") &&
		(n.DataAtom != atom.Legend || hasLegendInPreviousSiblings(n)) {
		return true
	}
	return inDisabledFieldset(n.Parent)
}

type checkedPseudoClassSelector struct {
	abstractPseudoClass
}

func (s checkedPseudoClassSelector) Match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.Input, atom.Menuitem:
		return hasAttr(n, "checked") && matchAttribute(n, "type", func(val string) bool {
			t := toLowerASCII(val)
			return t == "checkbox" || t == "radio"
		})
	case atom.Option:
		return optionSelected(n)
	}
	return false
}
