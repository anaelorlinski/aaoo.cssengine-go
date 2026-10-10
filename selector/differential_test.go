package selector

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func allElems(n *html.Node, out []*html.Node) []*html.Node {
	if n.Type == html.ElementNode {
		out = append(out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = allElems(c, out)
	}
	return out
}

func isAncestor(a, n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

func naive(sel Sel, n *html.Node, doc []*html.Node) bool {
	switch s := sel.(type) {
	case complexSelector:
		return naiveChain(s, len(s.compounds)-1, n, doc, nil, 0)
	case compoundSelector:
		if n.Type != html.ElementNode {
			return false
		}
		for _, x := range s.selectors {
			if !naive(x, n, doc) {
				return false
			}
		}
		return true
	case logicalPseudoClassSelector:
		if n.Type != html.ElementNode {
			return false
		}
		any := false
		for _, x := range s.match {
			if naive(x, n, doc) {
				any = true
			}
		}
		if s.name == "not" {
			return !any
		}
		return any
	case hasPseudoClassSelector:
		for _, r := range s.rels {
			comb := r.combinator
			if comb == 0 {
				comb = ' '
				if s.name == "haschild" {
					comb = '>'
				}
			}
			c := asComplex(r.sel)
			for _, e := range doc {
				if naiveChain(c, len(c.compounds)-1, e, doc, n, comb) {
					return true
				}
			}
		}
		return false
	case nthPseudoClassSelector:
		if n.Type != html.ElementNode || n.Parent == nil {
			return false
		}
		if s.of != nil && !naiveG(s.of, n, doc) {
			return false
		}
		q := func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return false
			}
			if s.ofType {
				return c.Data == n.Data && c.Namespace == n.Namespace
			}
			return s.of == nil || naiveG(s.of, c, doc)
		}
		i := 0
		if s.last {
			for c := n; c != nil; c = c.NextSibling {
				if q(c) {
					i++
				}
			}
		} else {
			for c := n; c != nil; c = c.PrevSibling {
				if q(c) {
					i++
				}
			}
		}
		return nthMatches(s.a, s.b, i)
	default:
		return sel.Match(n)
	}
}

func rel(anchor *html.Node, comb byte, e *html.Node) bool {
	switch comb {
	case ' ':
		return isAncestor(anchor, e)
	case '>':
		return e.Parent == anchor
	case '+':
		return prevElementSibling(e) == anchor
	case '~':
		for p := prevElementSibling(e); p != nil; p = prevElementSibling(p) {
			if p == anchor {
				return true
			}
		}
	}
	return false
}

func naiveChain(c complexSelector, i int, e *html.Node, doc []*html.Node, anchor *html.Node, comb byte) bool {
	if !naive(c.compounds[i], e, doc) {
		return false
	}
	if i == 0 {
		return anchor == nil || rel(anchor, comb, e)
	}
	switch c.combinators[i-1] {
	case ' ':
		for p := e.Parent; p != nil && p.Type == html.ElementNode; p = p.Parent {
			if naiveChain(c, i-1, p, doc, anchor, comb) {
				return true
			}
		}
	case '>':
		p := e.Parent
		return p != nil && p.Type == html.ElementNode && naiveChain(c, i-1, p, doc, anchor, comb)
	case '+':
		p := prevElementSibling(e)
		return p != nil && naiveChain(c, i-1, p, doc, anchor, comb)
	case '~':
		for p := prevElementSibling(e); p != nil; p = prevElementSibling(p) {
			if naiveChain(c, i-1, p, doc, anchor, comb) {
				return true
			}
		}
	}
	return false
}

var tags = []string{"a", "b", "c"}
var classes = []string{"x", "y"}

func genTree(r *rand.Rand, depth int, b *strings.Builder) {
	n := r.Intn(4)
	if depth == 0 {
		n = 0
	}
	for i := 0; i < n; i++ {
		t := tags[r.Intn(len(tags))]
		cls := ""
		if r.Intn(2) == 0 {
			cls = fmt.Sprintf(` class="%s"`, classes[r.Intn(2)])
		}
		fmt.Fprintf(b, "<%s%s>", t, cls)
		genTree(r, depth-1, b)
		fmt.Fprintf(b, "</%s>", t)
	}
}

func genCompound(r *rand.Rand, d int, inHas bool) string {
	var parts []string
	switch r.Intn(3) {
	case 0:
		parts = append(parts, tags[r.Intn(len(tags))])
	case 1:
		parts = append(parts, "*")
	}
	k := r.Intn(2)
	if d <= 0 {
		k = 0
	}
	if r.Intn(2) == 0 {
		parts = append(parts, "."+classes[r.Intn(2)])
	}
	for i := 0; i < k; i++ {
		switch r.Intn(6) {
		case 0:
			parts = append(parts, ":is("+genList(r, d-1, inHas)+")")
		case 1:
			parts = append(parts, ":not("+genList(r, d-1, inHas)+")")
		case 2:
			if !inHas {
				combs := []string{"", "> ", "+ ", "~ "}
				parts = append(parts, ":"+[]string{"has", "haschild"}[r.Intn(2)]+"("+combs[r.Intn(4)]+genComplex(r, d-1, true)+")")
			}
		case 3:
			parts = append(parts, fmt.Sprintf(":nth-child(%dn+%d of %s)", r.Intn(3), r.Intn(3), genList(r, d-1, inHas)))
		case 4:
			parts = append(parts, ":first-child")
		case 5:
			parts = append(parts, ":where("+genList(r, d-1, inHas)+")")
		}
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, "")
}

func genComplex(r *rand.Rand, d int, inHas bool) string {
	s := genCompound(r, d, inHas)
	combs := []string{" ", " > ", " + ", " ~ "}
	for n := r.Intn(4); n > 0; n-- {
		s += combs[r.Intn(4)] + genCompound(r, d, inHas)
	}
	return s
}

func genList(r *rand.Rand, d int, inHas bool) string {
	s := genComplex(r, d, inHas)
	if r.Intn(3) == 0 {
		s += ", " + genComplex(r, d, inHas)
	}
	return s
}

// TestDifferential compares the matcher, alone and with a [Cache] shared by
// every selector and element of a document, against a naive one (no
// memoisation, no index, :has() testing every element) on random documents
// and selectors built from :is, :where, :not, :has, :nth-child(of S) and
// every combinator.
func TestDifferential(t *testing.T) {
	seeds := int64(4)
	if testing.Short() {
		seeds = 1
	}
	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed))
		fails := 0
		for iter := 0; iter < 2000 && fails < 5; iter++ {
			var b strings.Builder
			b.WriteString("<html><body>")
			genTree(r, 4, &b)
			b.WriteString("</body></html>")
			doc, _ := html.Parse(strings.NewReader(b.String()))
			elems := allElems(doc, nil)
			cache := NewCache() // shared by every selector and element of the document
			for j := 0; j < 10; j++ {
				src := genList(r, 3, false)
				g, err := ParseGroup(src)
				if err != nil {
					t.Fatalf("parse %q: %v", src, err)
				}
				for _, e := range elems {
					got := g.Match(e)
					want := naiveG(g, e, elems)
					gotCached := false
					for _, x := range g {
						if MatchWith(x, e, cache) {
							gotCached = true
						}
					}
					if gotCached != want {
						fails++
						t.Errorf("CACHED sel %q doc %s elem %s: got %v want %v", src, b.String(), e.Data, gotCached, want)
						break
					}
					if got != want {
						fails++
						t.Errorf("sel %q doc %s elem %s: got %v want %v", src, b.String(), e.Data, got, want)
						break
					}
				}
				// round-trip
				g2, err := ParseGroup(g.String())
				if err != nil {
					t.Errorf("reparse %q -> %q: %v", src, g.String(), err)
				} else if g2.String() != g.String() {
					t.Errorf("roundtrip %q != %q", g2.String(), g.String())
				}
			}
		}
	}

}

func naiveG(g SelectorGroup, n *html.Node, doc []*html.Node) bool {
	for _, x := range g {
		if naive(x, n, doc) {
			return true
		}
	}
	return false
}

func extGenCompound(r *rand.Rand, d int, inHas bool) string {
	var parts []string
	switch r.Intn(3) {
	case 0:
		parts = append(parts, tags[r.Intn(len(tags))])
	case 1:
		parts = append(parts, "*")
	}
	k := r.Intn(2)
	if d <= 0 {
		k = 0
	}
	if r.Intn(2) == 0 {
		parts = append(parts, "."+classes[r.Intn(2)])
	}
	nth := func() string {
		a := r.Intn(5) - 2
		b := r.Intn(5) - 1
		if r.Intn(4) == 0 {
			return []string{"odd", "even", "n", "-n+2"}[r.Intn(4)]
		}
		if b < 0 {
			return fmt.Sprintf("%dn%d", a, b)
		}
		return fmt.Sprintf("%dn+%d", a, b)
	}
	for i := 0; i < k; i++ {
		switch r.Intn(10) {
		case 0:
			parts = append(parts, ":is("+extGenList(r, d-1, inHas)+")")
		case 1:
			parts = append(parts, ":not("+extGenList(r, d-1, inHas)+")")
		case 2:
			if !inHas {
				combs := []string{"", "> ", "+ ", "~ "}
				parts = append(parts, ":"+[]string{"has", "haschild"}[r.Intn(2)]+"("+combs[r.Intn(4)]+extGenComplex(r, d-1, true)+")")
			}
		case 3:
			parts = append(parts, fmt.Sprintf(":nth-child(%s of %s)", nth(), extGenList(r, d-1, inHas)))
		case 4:
			parts = append(parts, fmt.Sprintf(":nth-last-child(%s of %s)", nth(), extGenList(r, d-1, inHas)))
		case 5:
			parts = append(parts, []string{":first-child", ":last-child", ":first-of-type", ":last-of-type", ":only-child", ":only-of-type"}[r.Intn(6)])
		case 6:
			parts = append(parts, fmt.Sprintf(":nth-child(%s)", nth()))
		case 7:
			parts = append(parts, fmt.Sprintf(":nth-last-child(%s)", nth()))
		case 8:
			parts = append(parts, fmt.Sprintf(":nth-of-type(%s)", nth()))
		case 9:
			parts = append(parts, fmt.Sprintf(":nth-last-of-type(%s)", nth()))
		}
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, "")
}

func extGenComplex(r *rand.Rand, d int, inHas bool) string {
	s := extGenCompound(r, d, inHas)
	combs := []string{" ", " > ", " + ", " ~ "}
	for n := r.Intn(4); n > 0; n-- {
		s += combs[r.Intn(4)] + extGenCompound(r, d, inHas)
	}
	return s
}

func extGenList(r *rand.Rand, d int, inHas bool) string {
	s := extGenComplex(r, d, inHas)
	if r.Intn(3) == 0 {
		s += ", " + extGenComplex(r, d, inHas)
	}
	return s
}

func extNaiveTop(sel Sel, n *html.Node, doc []*html.Node) bool {
	switch s := sel.(type) {
	case onlyChildPseudoClassSelector:
		if n.Type != html.ElementNode || n.Parent == nil {
			return false
		}
		c := 0
		for e := n.Parent.FirstChild; e != nil; e = e.NextSibling {
			if e.Type == html.ElementNode && (!s.ofType || e.Data == n.Data && e.Namespace == n.Namespace) {
				c++
			}
		}
		return c == 1
	}
	return naive(sel, n, doc)
}

// TestDifferentialExtended is TestDifferential with a wider generator: every
// :nth-*() form (negative a, b > 2, without "of"), :only-*, *-of-type, and a
// brute-force reference for an+b.
func TestDifferentialExtended(t *testing.T) {
	seeds := int64(2)
	if testing.Short() {
		seeds = 1
	}
	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed))
		fails := 0
		for iter := 0; iter < 1500 && fails < 5; iter++ {
			var b strings.Builder
			b.WriteString("<html><body>")
			genTree(r, 4, &b)
			b.WriteString("</body></html>")
			doc, _ := html.Parse(strings.NewReader(b.String()))
			elems := allElems(doc, nil)
			cache := NewCache()
			for j := 0; j < 10; j++ {
				src := extGenList(r, 3, false)
				g, err := ParseGroup(src)
				if err != nil {
					t.Fatalf("parse %q: %v", src, err)
				}
				for _, e := range elems {
					want := false
					for _, x := range g {
						// the naive matcher dispatches on the top-level type only;
						// only-child inside compounds goes through Match, which is
						// the implementation itself, so wrap at every level
						if extNaive(x, e, elems) {
							want = true
						}
					}
					got := g.Match(e)
					gotCached := false
					for _, x := range g {
						if MatchWith(x, e, cache) {
							gotCached = true
						}
					}
					if got != want || gotCached != want {
						fails++
						t.Errorf("sel %q doc %s elem %s: got %v cached %v want %v", src, b.String(), e.Data, got, gotCached, want)
						break
					}
				}
				g2, err := ParseGroup(g.String())
				if err != nil {
					t.Errorf("reparse %q -> %q: %v", src, g.String(), err)
				} else if g2.String() != g.String() {
					t.Errorf("roundtrip %q != %q", g2.String(), g.String())
				}
			}
		}
	}
}

// extNaive is naive, with compound dispatch covering only-child.
func extNaive(sel Sel, n *html.Node, doc []*html.Node) bool {
	switch s := sel.(type) {
	case compoundSelector:
		if n.Type != html.ElementNode {
			return false
		}
		for _, x := range s.selectors {
			if !extNaive(x, n, doc) {
				return false
			}
		}
		return true
	case complexSelector:
		return extNaiveChain(s, len(s.compounds)-1, n, doc, nil, 0)
	case logicalPseudoClassSelector:
		if n.Type != html.ElementNode {
			return false
		}
		any := false
		for _, x := range s.match {
			if extNaive(x, n, doc) {
				any = true
			}
		}
		if s.name == "not" {
			return !any
		}
		return any
	case hasPseudoClassSelector:
		for _, r := range s.rels {
			comb := r.combinator
			if comb == 0 {
				comb = ' '
				if s.name == "haschild" {
					comb = '>'
				}
			}
			c := asComplex(r.sel)
			for _, e := range doc {
				if extNaiveChain(c, len(c.compounds)-1, e, doc, n, comb) {
					return true
				}
			}
		}
		return false
	case nthPseudoClassSelector:
		if n.Type != html.ElementNode || n.Parent == nil {
			return false
		}
		if s.of != nil && !extNaiveG(s.of, n, doc) {
			return false
		}
		q := func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return false
			}
			if s.ofType {
				return c.Data == n.Data && c.Namespace == n.Namespace
			}
			return s.of == nil || extNaiveG(s.of, c, doc)
		}
		i := 0
		if s.last {
			for c := n; c != nil; c = c.NextSibling {
				if q(c) {
					i++
				}
			}
		} else {
			for c := n; c != nil; c = c.PrevSibling {
				if q(c) {
					i++
				}
			}
		}
		// reference: i = a*k+b, k >= 0, computed by brute force
		for k := 0; k <= 100; k++ {
			if s.a*k+s.b == i {
				return true
			}
		}
		return false
	default:
		return extNaiveTop(sel, n, doc)
	}
}

func extNaiveG(g SelectorGroup, n *html.Node, doc []*html.Node) bool {
	for _, x := range g {
		if extNaive(x, n, doc) {
			return true
		}
	}
	return false
}

func extNaiveChain(c complexSelector, i int, e *html.Node, doc []*html.Node, anchor *html.Node, comb byte) bool {
	if !extNaive(c.compounds[i], e, doc) {
		return false
	}
	if i == 0 {
		return anchor == nil || rel(anchor, comb, e)
	}
	switch c.combinators[i-1] {
	case ' ':
		for p := e.Parent; p != nil && p.Type == html.ElementNode; p = p.Parent {
			if extNaiveChain(c, i-1, p, doc, anchor, comb) {
				return true
			}
		}
	case '>':
		p := e.Parent
		return p != nil && p.Type == html.ElementNode && extNaiveChain(c, i-1, p, doc, anchor, comb)
	case '+':
		p := prevElementSibling(e)
		return p != nil && extNaiveChain(c, i-1, p, doc, anchor, comb)
	case '~':
		for p := prevElementSibling(e); p != nil; p = prevElementSibling(p) {
			if extNaiveChain(c, i-1, p, doc, anchor, comb) {
				return true
			}
		}
	}
	return false
}
