package selector

import (
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Tests for the findings of the CSS review (webrender docs/css-review-bugs.md,
// section "css/selector … and css/cascade").

// nestedDivs returns a document made of depth nested <div>s around a <span>,
// with no <x> element, and the innermost <span>.
func nestedDivs(t *testing.T, depth int) (*html.Node, *html.Node) {
	t.Helper()
	src := strings.Repeat("<div>", depth) + "<span></span>" + strings.Repeat("</div>", depth)
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	span := MatchFirst(doc, MustCompile("span"))
	if span == nil {
		t.Fatal("no span")
	}
	return doc, span
}

// withTimeout runs f and fails the test if it does not finish within a
// (very generous) delay. The pathological cases below took hours before the
// SEL1/SEL2 fixes, and take milliseconds after; the call-count tests are the
// precise, deterministic checks.
func withTimeout(t *testing.T, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("matching did not finish: exponential behaviour")
	}
}

// SEL1: descendant matching re-ran the left side at every ancestor, which
// is exponential in the number of descendant combinators.
func TestSEL1DescendantMatchingIsNotExponential(t *testing.T) {
	const depth = 200
	doc, span := nestedDivs(t, depth)
	for k := 1; k <= 8; k++ {
		sel := MustCompile("x" + strings.Repeat(" div", k) + " span")
		withTimeout(t, func() {
			if sel.Match(span) {
				t.Errorf("k=%d: should not match (no x element)", k)
			}
			if got := len(MatchAll(doc, sel)); got != 0 {
				t.Errorf("k=%d: got %d matches", k, got)
			}
		})
	}
}

// Mixed combinators: the greedy "nearest matching ancestor" shortcut would
// get these wrong, the memoised matching must not.
func TestSEL1MixedCombinators(t *testing.T) {
	for _, test := range []selectorTest{
		// the nearest b ancestor of c is not a child of a, a farther one is
		{`<a><b><b><c></c></b></b></a>`, "a > b c", []string{"<c></c>"}},
		{`<a><b><i><b><c></c></b></i></b></a>`, "a > b c", []string{"<c></c>"}},
		{`<a><i><b><b><c></c></b></b></i></a>`, "a > b c", []string{}},
		{`<a><b><c></c></b></a><d><b><c></c></b></d>`, "a b c", []string{"<c></c>"}},
		{`<div><p></p><h1></h1><p><i></i></p></div>`, "p ~ h1 + p i", []string{"<i></i>"}},
		{`<div class=x><div><div class=x><p><span></span></p></div></div></div>`, ".x > div .x p span", []string{"<span></span>"}},
		{`<div class=x><div><div><p><span></span></p></div></div></div>`, ".x > div .x p span", []string{}},
	} {
		s, doc, err := setup(test.selector, test.HTML)
		if err != nil {
			t.Fatal(err)
		}
		matches := MatchAll(doc, s)
		if len(matches) != len(test.results) {
			t.Errorf("%s on %s: got %d matches, want %d", test.selector, test.HTML, len(matches), len(test.results))
		}
	}
}

// SEL2: :has() scanned the anchor subtree and re-ran an exponential
// anchored match at every candidate.
func TestSEL2HasSubtreeSearchIsBounded(t *testing.T) {
	const depth = 200
	doc, _ := nestedDivs(t, depth)
	sel := MustCompile("div:has(x div div div div span)")
	withTimeout(t, func() {
		if got := len(MatchAll(doc, sel)); got != 0 {
			t.Errorf("got %d matches", got)
		}
	})

	// and it still matches when it should
	body := MatchFirst(doc, MustCompile("body"))
	s2 := MustCompile("body:has(div div div div span)")
	if !s2.Match(body) {
		t.Error("body:has(div div div div span) should match")
	}
}

// SEL3: SVG keeps camelCase tag and attribute names.
func TestSEL3SVGCamelCase(t *testing.T) {
	doc := `<svg viewBox="0 0 1 1"><defs><linearGradient id=g></linearGradient><clipPath id=c></clipPath></defs></svg><div viewbox=x></div>`
	for _, test := range []struct {
		sel  string
		want int
	}{
		{"linearGradient", 1},
		{"clipPath", 1},
		{"lineargradient", 0}, // case-sensitive outside the HTML namespace
		{"svg [viewBox]", 0},
		{"[viewBox]", 2}, // the svg, and the html div (case-insensitive)
		{"svg[viewBox]", 1},
		{"svg[viewbox]", 0},
		{"DIV", 1}, // HTML elements stay case-insensitive
		{"div[VIEWBOX]", 1},
		{"defs > clipPath#c", 1},
	} {
		s, d, err := setup(test.sel, doc)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(MatchAll(d, s)); got != test.want {
			t.Errorf("%s: got %d matches, want %d", test.sel, got, test.want)
		}
	}
}

// SEL5: :lang uses the nearest lang, case-insensitively, as a prefix.
func TestSEL5Lang(t *testing.T) {
	for _, test := range []struct {
		html, sel string
		want      int
	}{
		{`<div lang=fr><p lang=en></p></div>`, "p:lang(fr)", 0},
		{`<div lang=fr><p lang=en></p></div>`, "p:lang(en)", 1},
		{`<p lang=EN></p>`, "p:lang(en)", 1},
		{`<p lang=en-US></p>`, "p:lang(en)", 1},
		{`<p lang=en-US></p>`, "p:lang(EN-us)", 1},
		{`<p lang=english></p>`, "p:lang(en)", 0},
		{`<div lang=fr><p></p></div>`, "p:lang(fr)", 1},
		{`<div lang=fr><p lang=""></p></div>`, "p:lang(fr)", 0},
		{`<div lang=fr><p></p></div>`, "p:lang(de, fr)", 1},
		{`<div lang=fr><p></p></div>`, `p:lang("fr")`, 1},
	} {
		s, d, err := setup(test.sel, test.html)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(MatchAll(d, s)); got != test.want {
			t.Errorf("%s on %s: got %d matches, want %d", test.sel, test.html, got, test.want)
		}
	}
}

// SEL7: namespace syntax and attribute namespaces.
func TestSEL7Namespaces(t *testing.T) {
	doc := `<p></p><svg><rect></rect><a xlink:href="#x"></a></svg>`
	for _, test := range []struct {
		sel  string
		want int
	}{
		{"*|*", 7}, // html head body p svg rect a
		{"svg|rect", 1},
		{"svg|*", 3},
		{"html|p", 1},
		{"svg|p", 0},
		{"*|rect", 1},
		{"|rect", 0},
		{"[href]", 0}, // xlink:href is in the xlink namespace
		{"[xlink|href]", 1},
		{"[*|href]", 1},
		{"[|href]", 0},
		{`[xlink|href="#x"]`, 1},
		{`[lang|=en]`, 0}, // still the dash-match operator
	} {
		s, d, err := setup(test.sel, doc)
		if err != nil {
			t.Errorf("%s: %s", test.sel, err)
			continue
		}
		if got := len(MatchAll(d, s)); got != test.want {
			t.Errorf("%s: got %d matches, want %d", test.sel, got, test.want)
		}
		// round trip
		if _, err := ParseGroup(s.String()); err != nil {
			t.Errorf("%s: serialized %q does not parse: %s", test.sel, s.String(), err)
		}
	}
	if _, err := ParseGroup("foo|p"); err == nil {
		t.Error("an undeclared namespace prefix should be rejected")
	}
	// specificity: the namespace does not count
	for sel, spec := range map[string]Specificity{"svg|rect": {0, 0, 1}, "*|*": {}, "[xlink|href]": {0, 1, 0}} {
		s, err := Parse(sel)
		if err != nil {
			t.Errorf("%s: %s", sel, err)
		} else if s.Specificity() != spec {
			t.Errorf("%s: specificity %v, want %v", sel, s.Specificity(), spec)
		}
	}
}

// SEL8: the s flag, :nth-child(An+B of S), ASCII case folding.
func TestSEL8AttributeCaseFlags(t *testing.T) {
	doc := "<p a=Foo></p><p a=foo></p><p a=\"\u017f\"></p><p a=\"\u212a\"></p>"
	for _, test := range []struct {
		sel  string
		want int
	}{
		{"[a=foo s]", 1},
		{"[a=foo S]", 1},
		{"[a=foo i]", 2},
		{"[a=FOO i]", 2},
		{"[a=s i]", 0},          // U+017F LONG S does not fold to s in ASCII
		{"[a=k i]", 0},          // U+212A KELVIN SIGN does not fold to k in ASCII
		{"[a^=fo i]", 2},        // prefix
		{"[a=\"\u017f\" i]", 1}, // exact match still works
		{"[a~=FOO i]", 2},       // include
		{"[a$=OO i]", 2},        // suffix
		{"[a*=O i]", 2},         // substring
		{"[a|=FOO i]", 2},       // dash
		{"[a!=FOO i]", 5},       // html head body + the two non-ASCII values
	} {
		s, d, err := setup(test.sel, doc)
		if err != nil {
			t.Errorf("%s: %s", test.sel, err)
			continue
		}
		if got := len(MatchAll(d, s)); got != test.want {
			t.Errorf("%s: got %d matches, want %d", test.sel, got, test.want)
		}
		if _, err := ParseGroup(s.String()); err != nil {
			t.Errorf("%s: serialized %q does not parse: %s", test.sel, s.String(), err)
		}
	}
	if s := MustCompile("[a=b s]"); s.String() != `[a="b" s]` {
		t.Errorf("serialized as %q", s.String())
	}
}

func TestSEL8NthChildOf(t *testing.T) {
	doc := `<ul><li class=a>1</li><li>2</li><li class=a>3</li><li class=a>4</li><li>5</li></ul>`
	for _, test := range []struct {
		sel  string
		want []string
	}{
		{"li:nth-child(2 of .a)", []string{"3"}},
		{"li:nth-child(odd of .a)", []string{"1", "4"}},
		{"li:nth-last-child(1 of .a)", []string{"4"}},
		{"li:nth-child(1 of :not(.a))", []string{"2"}},
		{"li:nth-child(2 of .a, :not(.a))", []string{"2"}},
		{"li:nth-child(n of li)", []string{"1", "2", "3", "4", "5"}},
	} {
		s, d, err := setup(test.sel, doc)
		if err != nil {
			t.Errorf("%s: %s", test.sel, err)
			continue
		}
		var got []string
		for _, m := range MatchAll(d, s) {
			got = append(got, nodeText(m))
		}
		if strings.Join(got, ",") != strings.Join(test.want, ",") {
			t.Errorf("%s: got %v, want %v", test.sel, got, test.want)
		}
		if _, err := ParseGroup(s.String()); err != nil {
			t.Errorf("%s: serialized %q does not parse: %s", test.sel, s.String(), err)
		}
	}
	for sel, spec := range map[string]Specificity{
		":nth-child(2 of .a)":       {0, 2, 0},
		":nth-child(2 of #x, .a)":   {1, 1, 0},
		":nth-child(2 of li)":       {0, 1, 1},
		":nth-child(2n+1)":          {0, 1, 0},
		":nth-last-child(2 of p.a)": {0, 2, 1},
	} {
		s, err := Parse(sel)
		if err != nil {
			t.Errorf("%s: %s", sel, err)
			continue
		}
		if s.Specificity() != spec {
			t.Errorf("%s: specificity %v, want %v", sel, s.Specificity(), spec)
		}
	}
	for _, bad := range []string{":nth-of-type(1 of p)", ":nth-child(1 of)", ":nth-child(1 of ::before)"} {
		if _, err := ParseGroup(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// SEL9: huge an+b coefficients must not overflow.
func TestSEL9NthOverflow(t *testing.T) {
	doc := `<ul><li>1</li><li>2</li><li>3</li></ul>`
	for _, test := range []struct {
		sel  string
		want int
	}{
		{"li:nth-child(n-9223372036854775807)", 3},
		{"li:nth-child(n-99999999999999999999)", 3},
		{"li:nth-child(-n+9223372036854775807)", 3},
		{"li:nth-child(9223372036854775807n+1)", 1},
		{"li:nth-child(-9223372036854775807n+3)", 1},
		{"li:nth-last-child(n-9223372036854775807)", 3},
		{"li:nth-child(99999999999999999999)", 0},
	} {
		s, d, err := setup(test.sel, doc)
		if err != nil {
			t.Errorf("%s: %s", test.sel, err)
			continue
		}
		if got := len(MatchAll(d, s)); got != test.want {
			t.Errorf("%s: got %d matches, want %d", test.sel, got, test.want)
		}
	}
}

// SC9: the index keys of hand-built nodes agree with the matcher.
func TestElementKeysHandBuilt(t *testing.T) {
	twoIDs := &html.Node{Type: html.ElementNode, Data: "p", DataAtom: atom.P,
		Attr: []html.Attribute{{Key: "id", Val: "a"}, {Key: "id", Val: "a"}}}
	var ids int
	for _, k := range ElementKeys(twoIDs) {
		if k.Kind == KeyID {
			ids++
		}
	}
	if ids != 1 {
		t.Errorf("duplicate id attributes: %d id keys", ids)
	}
	// the type key follows the atom, as tagSelector does
	odd := &html.Node{Type: html.ElementNode, Data: "x-div", DataAtom: atom.Div}
	sel := MustCompile("div")
	if !sel.Match(odd) || !slices.Contains(ElementKeys(odd), Key(sel[0])) {
		t.Errorf("type key %v, selector key %v", ElementKeys(odd), Key(sel[0]))
	}
}
