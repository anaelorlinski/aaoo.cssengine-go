package cascade

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/selector"
	"golang.org/x/net/html"
)

// matchAll is the reference: every selector of every rule tested, in
// order, then the same sort as Match.
func matchAll(m *Matcher[decl], element *html.Node) []Matched[decl] {
	var out []Matched[decl]
	for _, rule := range m.Rules() {
		for _, sel := range rule.Selectors {
			if sel.Match(element) {
				for _, d := range rule.Declarations {
					out = append(out, Matched[decl]{Declaration: d, Specificity: sel.Specificity(), Order: rule.Order, Layer: rule.Layer, PseudoElement: sel.PseudoElement()})
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Matched[decl]) int {
		if a.Specificity != b.Specificity {
			if a.Specificity.Less(b.Specificity) {
				return -1
			}
			return 1
		}
		return a.Order - b.Order
	})
	return out
}

// The index must not change what Match returns.
func TestMatchIndexEquivalence(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`
	  <div id="a" class="b c  b" data-x="1"><p class="c">text<span id="s">x</span></p></div>
	  <section class="B"><p></p></section>
	  <svg><linearGradient id="g"></linearGradient><rect class="r"/></svg>
	  <ul><li class="	tabbed	"></li><li id=""></li></ul>`))
	if err != nil {
		t.Fatal(err)
	}
	sels := []string{
		"*", "div", "DIV", "p", "span", "section", "li", "rect", "linearGradient", "lineargradient",
		"#a", "#s", "#g", "#missing", ".b", ".c", ".B", ".r", ".tabbed",
		"div.b", "p.c", "div#a.b.c", "#a .c", "div > p", "p + p", "section p", "div span",
		"*|*", "|p", "svg|rect", ":is(.b, #s)", ":not(p)", "p:first-child", "div::before", ".c::after",
		"[data-x]", "li:nth-child(2)", "body > *", ":where(#a) .c", "div:has(> p)",
	}
	m := &Matcher[decl]{}
	for i, s := range sels {
		group, err := selector.ParseGroup(s)
		if err != nil {
			continue // e.g. undeclared namespace prefixes
		}
		m.Add(Rule[decl]{Selectors: group, Declarations: []decl{{s, "x"}}, Order: i, Layer: i % 3})
	}
	// a group mixing keys
	group, _ := selector.ParseGroup("#a, .c, p, *")
	m.Add(Rule[decl]{Selectors: group, Declarations: []decl{{"group", "x"}}, Order: len(sels)})

	elements := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			elements++
			if got, want := m.Match(n), matchAll(m, n); !reflect.DeepEqual(got, want) {
				t.Errorf("<%s>: index gives %d matches, reference %d", n.Data, len(got), len(want))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if elements < 10 {
		t.Fatalf("only %d elements", elements)
	}
}
