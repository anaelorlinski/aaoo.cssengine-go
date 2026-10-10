package cascade

import (
	"strings"
	"testing"

	"github.com/anaelorlinski/aaoo.cssengine-go/selector"
	"golang.org/x/net/html"
)

type decl struct{ prop, value string }

func rule(t *testing.T, sel string, order int, decls ...decl) Rule[decl] {
	t.Helper()
	group, err := selector.ParseGroup(sel)
	if err != nil {
		t.Fatalf("parse %q: %s", sel, err)
	}
	return Rule[decl]{Selectors: group, Declarations: decls, Order: order}
}

// winner applies matched declarations in order and returns the final
// value per property, mimicking a stringly styling layer.
func winner(ms []Matched[decl]) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		out[m.Declaration.prop] = m.Declaration.value
	}
	return out
}

func matcherOf(rules ...Rule[decl]) *Matcher[decl] {
	m := &Matcher[decl]{}
	for _, r := range rules {
		m.Add(r)
	}
	return m
}

func parseDoc(t *testing.T, doc string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	// html > body > first child
	var find func(*html.Node) *html.Node
	find = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode && n.Data == "div" {
			return n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if r := find(c); r != nil {
				return r
			}
		}
		return nil
	}
	el := find(root)
	if el == nil {
		t.Fatal("no div in test document")
	}
	return el
}

func TestCascadeOrder(t *testing.T) {
	el := parseDoc(t, `<div id="a" class="b" data-x="1"></div>`)

	m := matcherOf(
		// source order: the id rule comes FIRST; it must still win
		rule(t, "#a", 0, decl{"fill", "id"}),
		rule(t, ".b", 1, decl{"fill", "class"}, decl{"stroke", "class"}),
		rule(t, "div", 2, decl{"fill", "tag"}),
		// equal specificity to .b, later in source: wins over .b
		rule(t, "[data-x]", 3, decl{"stroke", "attr"}),
	)

	got := winner(m.Match(el))
	if got["fill"] != "id" {
		t.Errorf("fill: got %q, want id (specificity)", got["fill"])
	}
	if got["stroke"] != "attr" {
		t.Errorf("stroke: got %q, want attr (source order on tie)", got["stroke"])
	}
}

func TestCascadeWhereZero(t *testing.T) {
	el := parseDoc(t, `<div id="a"></div>`)
	m := matcherOf(
		rule(t, ":where(#a)", 0, decl{"fill", "where"}),
		rule(t, "div", 1, decl{"fill", "tag"}),
	)
	if got := winner(m.Match(el)); got["fill"] != "tag" {
		t.Errorf("fill: got %q, want tag (:where has zero specificity)", got["fill"])
	}
}

func TestCascadeNoMatch(t *testing.T) {
	el := parseDoc(t, `<div></div>`)
	m := matcherOf(rule(t, "#nope", 0, decl{"fill", "x"}))
	if ms := m.Match(el); len(ms) != 0 {
		t.Errorf("expected no matches, got %d", len(ms))
	}
}

// CAS1: declarations for pseudo-elements come back mixed with the element's
// own, tagged with their pseudo-element; callers must split them.
func TestCascadePseudoElementsAreTagged(t *testing.T) {
	m := matcherOf(
		rule(t, "div", 0, decl{"color", "red"}),
		rule(t, "div::before", 1, decl{"content", "x"}),
	)
	got := m.Match(parseDoc(t, `<div></div>`))
	if len(got) != 2 {
		t.Fatalf("got %d declarations", len(got))
	}
	pseudo := map[string]string{}
	for _, d := range got {
		pseudo[d.Declaration.prop] = d.PseudoElement
	}
	if pseudo["color"] != "" || pseudo["content"] != "before" {
		t.Errorf("pseudo-elements: %v", pseudo)
	}
}
