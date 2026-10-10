package selector

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// Deterministic complexity checks for SEL1/SEL2: count the compound
// matches instead of measuring wall-clock time.

// countingSel wraps a selector and counts its Match calls.
type countingSel struct {
	Sel
	n *int
}

func (c countingSel) Match(n *html.Node) bool { *c.n++; return c.Sel.Match(n) }

// instrument wraps every compound of a complex selector (and of the relative
// selectors of a :has()) so that their Match calls are counted.
func instrument(s Sel, counter *int) Sel {
	switch s := s.(type) {
	case complexSelector:
		var compounds []Sel
		for _, c := range s.compounds {
			compounds = append(compounds, instrument(c, counter))
		}
		return newComplexSelector(compounds, s.combinators)
	case logicalPseudoClassSelector:
		out := logicalPseudoClassSelector{name: s.name, nested: s.nested}
		for _, c := range s.match {
			out.match = append(out.match, instrument(c, counter))
		}
		return out
	case compoundSelector:
		var selectors []Sel
		for _, c := range s.selectors {
			selectors = append(selectors, instrument(c, counter))
		}
		return newCompoundSelector(selectors, s.pseudoElement)
	case hasPseudoClassSelector:
		out := hasPseudoClassSelector{name: s.name}
		for _, r := range s.rels {
			sel := instrument(r.sel, counter)
			out.rels = append(out.rels, relativeSelector{combinator: r.combinator, sel: sel, chain: asComplex(sel)})
		}
		return out
	default:
		return countingSel{Sel: s, n: counter}
	}
}

func TestSEL1DescendantMatchCallCount(t *testing.T) {
	const depth = 200
	_, span := nestedDivs(t, depth)
	for _, input := range []string{
		"x div div div div span",
		"x div > div div ~ div div + div span",
		"x div div div div div div div div div div span",
	} {
		sel, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		inst := instrument(sel, &calls)
		if inst.Match(span) {
			t.Errorf("%s: should not match", input)
		}
		// each (compound, element) pair is evaluated at most once
		compounds := len(sel.(complexSelector).compounds)
		if bound := compounds * (depth + 3); calls > bound {
			t.Errorf("%s: %d compound matches, want at most %d", input, calls, bound)
		}
	}
}

func TestSEL2HasCallCount(t *testing.T) {
	const depth = 200
	doc, _ := nestedDivs(t, depth)
	// call count for a single anchor: linear in the subtree size
	body := MatchFirst(doc, MustCompile("body"))
	s, _ := Parse("body:has(x div div div div span)")
	var calls int
	inst := instrument(s, &calls)
	if inst.Match(body) {
		t.Error("should not match")
	}
	if bound := 6 * (depth + 3) * 2; calls > bound {
		t.Errorf("%d compound matches, want at most %d", calls, bound)
	}

}

// Selectors nested in :is() / :not() share the memo of the match: without
// it, k nested levels cost O(depth^(k+1)) (26 s for k = 4 at depth 200).
func TestNestedLogicalCallCount(t *testing.T) {
	const depth = 200
	_, span := nestedDivs(t, depth)
	for _, input := range []string{
		":is(:is(:is(:is(x div) div) div) div) span",
		":not(:not(:not(:not(x div) div) div) div) span",
		":is(x div div, :not(:is(y div div) div)) div span",
		":nth-child(1 of :is(x div div)) div span",
	} {
		sel, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		inst := instrument(sel, &calls)
		want := MustCompile(input).Match(span)
		if inst.Match(span) != want {
			t.Errorf("%s: instrumented result differs", input)
		}
		// 10 compounds at most, each evaluated at most once per element
		if bound := 10 * (depth + 3); calls > bound {
			t.Errorf("%s: %d compound matches, want at most %d", input, calls, bound)
		}
	}
}

// SC1, SC3: :nth-child() and its variants are linear in the number of
// siblings with a document cache, nested :nth-child(of S) included; :first-child
// and :last-child stop counting at once even without one.
func TestNthLinear(t *testing.T) {
	const n = 2000
	doc, err := html.Parse(strings.NewReader("<ul>" + strings.Repeat(`<li class="a"></li>`, n) + "</ul>"))
	if err != nil {
		t.Fatal(err)
	}
	items := QueryAll(doc, MustCompile("li"))
	for _, input := range []string{
		"li:last-child", "li:nth-child(even)", "li:nth-of-type(2n+1)", "li:nth-last-child(3)",
		"li:only-of-type", "li:nth-child(n of :nth-child(n of :nth-child(n of :nth-child(n of .a))))",
	} {
		sel := MustCompile(input)
		cache := NewCache()
		start := time.Now()
		var matched int
		for _, li := range items {
			for _, s := range sel {
				if MatchWith(s, li, cache) {
					matched++
				}
			}
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: %s for %d items", input, d, n)
		}
		// the same results as without the cache
		var plain int
		for _, li := range items[:50] {
			if sel.Match(li) {
				plain++
			}
		}
		var cached int
		for _, li := range items[:50] {
			if MatchWith(sel[0], li, NewCache()) {
				cached++
			}
		}
		if plain != cached {
			t.Errorf("%s: %d matches without cache, %d with", input, plain, cached)
		}
	}
	// without a cache, :last-child does not count the siblings
	start := time.Now()
	sel := MustCompile("li:last-child")
	for _, li := range items {
		sel.Match(li)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf(":last-child without cache: %s", d)
	}
}

// SC2: :has() is evaluated once per anchor with a document cache, and :has(>
// …) only scans children.
func TestHasLinear(t *testing.T) {
	const n = 8000
	doc, err := html.Parse(strings.NewReader(`<div class="page">` + strings.Repeat(`<p><span>x</span></p>`, n) + `</div>`))
	if err != nil {
		t.Fatal(err)
	}
	all := QueryAll(doc, MustCompile("*"))
	for _, input := range []string{"body:has(.modal-open) span", ".page:has(> .missing) p", "p:has(> span) span"} {
		sel := MustCompile(input)
		cache := NewCache()
		start := time.Now()
		for _, e := range all {
			MatchWith(sel[0], e, cache)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: %s for %d elements", input, d, len(all))
		}
	}
	// the depth bound: > span matches children only
	p := Query(doc, MustCompile("p"))
	if !MustCompile(":has(> span)").Match(p) || MustCompile("div:has(> span)").Match(Query(doc, MustCompile("div"))) {
		t.Error(":has(> span)")
	}
	if !MustCompile("div:has(> p > span)").Match(Query(doc, MustCompile("div"))) {
		t.Error(":has(> p > span)")
	}
}

// S2-1, S2-2: :has(+ S) only looks at the next sibling, :has(~ S) is one
// pass per sibling list.
func TestHasSiblingLinear(t *testing.T) {
	doc, err := html.Parse(strings.NewReader("<ul>" + strings.Repeat(`<li><span><em>x</em></span></li>`, 10_000) + `<li class="x"></li></ul>`))
	if err != nil {
		t.Fatal(err)
	}
	items := QueryAll(doc, MustCompile("li"))
	for input, want := range map[string]int{
		"li:has(+ li .zz)": 0, "li:has(+ li > .zz)": 0, "li:has(~ .zz)": 0, "li:has(~ .x)": 10_000,
		"li:has(+ .x)": 1, "li:has(~ li em)": 9_999, "li:has(+ li ~ .x)": 9_999,
	} {
		sel := MustCompile(input)
		cache := NewCache()
		start := time.Now()
		got := 0
		for _, li := range items {
			if MatchWith(sel[0], li, cache) {
				got++
			}
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: %s", input, d)
		}
		if got != want {
			t.Errorf("%s: %d matches, want %d", input, got, want)
		}
	}
}

// S2-9: with a document cache, selectors not using it match without a
// context or memo allocation.
func TestMatchWithAllocations(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<div class="a">` + strings.Repeat("<p>x</p>", 1000) + `</div>`))
	if err != nil {
		t.Fatal(err)
	}
	ps := QueryAll(doc, MustCompile("p"))
	cache := NewCache()
	for _, input := range []string{".a p", "p", "div > p", "p.x, div p"} {
		sel := MustCompile(input)
		allocs := testing.AllocsPerRun(5, func() {
			for _, p := range ps {
				for _, s := range sel {
					MatchWith(s, p, cache)
				}
			}
		})
		if allocs > 10 {
			t.Errorf("%s: %g allocations for %d elements", input, allocs, len(ps))
		}
	}
}

// S2-10: through Match, :nth-child(b of S) with a ≤ 0 stops counting early.
func TestNthOfMatchEarlyExit(t *testing.T) {
	doc, err := html.Parse(strings.NewReader("<ul>" + strings.Repeat(`<li class="a"></li>`, 5000) + "</ul>"))
	if err != nil {
		t.Fatal(err)
	}
	items := QueryAll(doc, MustCompile("li"))
	start := time.Now()
	sel := MustCompile("li:nth-child(1 of .a)")
	n := 0
	for _, li := range items {
		if sel.Match(li) {
			n++
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond || n != 1 {
		t.Errorf("%d matches in %s", n, d)
	}
}
