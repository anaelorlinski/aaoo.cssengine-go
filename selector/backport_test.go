package selector

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/anaelorlinski/aaoo.cssengine-go/limits"
)

// Tests for the fixes backported from andybalholm/cascadia (up to v1.3.5,
// commit d561140), adapted to this package, and for the related fixes made
// alongside them.

// cascadia 6d90623
func TestEmptyAttributeSelectorValues(t *testing.T) {
	for _, op := range []string{"^=", "$=", "*=", "~="} {
		for _, flag := range []string{"", " i"} {
			selector, err := Parse(`[data` + op + `""` + flag + `]`)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"", "text", " text ", " ", "\ttext"} {
				t.Run(fmt.Sprintf("%s%s/%q", op, flag, value), func(t *testing.T) {
					node := &html.Node{Type: html.ElementNode, Data: "p", Attr: []html.Attribute{{Key: "data", Val: value}}}
					if selector.Match(node) {
						t.Errorf("empty selector value matched attribute %q", value)
					}
				})
			}
		}
	}
}

// "~=" matches a whitespace-separated word, so a value containing
// whitespace can never match.
func TestIncludeSelectorWithWhitespace(t *testing.T) {
	selector, err := Parse(`[data~="a b"]`)
	if err != nil {
		t.Fatal(err)
	}
	node := &html.Node{Type: html.ElementNode, Data: "p", Attr: []html.Attribute{{Key: "data", Val: "a b"}}}
	if selector.Match(node) {
		t.Error(`[data~="a b"] should never match`)
	}
}

// cascadia d561140
func TestNullEscapeUsesReplacementCharacter(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<p id="&#0;" class="&#0;">text</p>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`#\0`, `#\000000`, `#\0 `, `.\0`, `[id="\0"]`} {
		t.Run(input, func(t *testing.T) {
			selector, err := Parse(input)
			if err != nil {
				t.Fatal(err)
			}
			matches := MatchAll(document, selector)
			if len(matches) != 1 || matches[0].Data != "p" {
				t.Fatalf("got %d matches, want the paragraph with replacement character", len(matches))
			}
		})
	}
}

// cascadia d561140
func TestEscapedCodePointControls(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`\41`, "A"}, {`\10ffff`, "\U0010ffff"}, {`\110000`, "�"}, {`\d800`, "�"}, {`\000041B`, "A"},
	} {
		p := parser{s: tc.input}
		got, err := p.parseEscape()
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
}

// cascadia caa0b27: deep nesting is an error, not a stack overflow.
func TestNestingLimit(t *testing.T) {
	// (:has cannot nest at all, so it is covered by TestInvalidPseudoClassArguments)
	for _, fn := range []string{":not(", ":is("} {
		depth := limits.NestingDepth + 10
		sel := strings.Repeat(fn, depth) + "p" + strings.Repeat(")", depth)
		if _, err := ParseGroup(sel); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
			t.Errorf("%s x %d: expected a nesting error, got %v", fn, depth, err)
		}
	}
	// moderate nesting still parses
	sel := strings.Repeat(":not(", 50) + "p" + strings.Repeat(")", 50)
	if _, err := ParseGroup(sel); err != nil {
		t.Errorf("50 levels should parse: %s", err)
	}
}

// cascadia 8b5adc2 (relative :has), with the leftmost compound anchored at
// the :has() element as Selectors 4 requires.
var relativeHasTests = []selectorTest{
	// upstream cases
	{`<html><a><img></img></a></html>`, "a:has(> img)", []string{"<a><img/></a>"}},
	{`<html><a>no image</a><p><img></img></p></html>`, "a:has(> img)", []string{}},
	{`<html><div><a><img></img></a></div></html>`, "div:has(> img)", []string{}},
	{`<html><p><img></img></p><p>x</p></html>`, `p:has(+ p)`, []string{"<p><img/></p>"}},
	// implicit combinators
	{`<html><div><a><img></img></a></div></html>`, "div:has(img)", []string{"<div><a><img/></a></div>"}},
	{`<html><div><a><img></img></a></div></html>`, "div:haschild(img)", []string{}},
	{`<html><div><img></img></div></html>`, "div:haschild(img)", []string{"<div><img/></div>"}},
	// ~ reaches any following sibling
	{`<html><h1></h1><p></p><span></span></html>`, "h1:has(~ span)", []string{"<h1></h1>"}},
	{`<html><h1></h1><p></p><span></span></html>`, "h1:has(+ span)", []string{}},
	// the subject may be a descendant of the sibling
	{`<html><h1></h1><div><em></em></div></html>`, "h1:has(+ div em)", []string{"<h1></h1>"}},
	// combinators after the anchored compound
	{`<html><div><a></a><b></b></div></html>`, "div:has(> a + b)", []string{"<div><a></a><b></b></div>"}},
	{`<html><div><b></b><a></a></div></html>`, "div:has(> a + b)", []string{}},
	// anchoring: the leftmost compound must be inside the :has() element
	{`<html><a><div><b></b></div></a></html>`, "div:has(a b)", []string{}},
	{`<html><div><a><b></b></a></div></html>`, "div:has(a b)", []string{"<div><a><b></b></a></div>"}},
	{`<html><div><span><a><b></b></a></span></div></html>`, "div:has(> a b)", []string{}},
	{`<html><div><a><span><b></b></span></a></div></html>`, "div:has(> a b)", []string{"<div><a><span><b></b></span></a></div>"}},
	// several relative selectors
	{`<html><div><i></i></div><div><b></b></div></html>`, "div:has(> i, > b)", []string{"<div><i></i></div>", "<div><b></b></div>"}},
}

func TestRelativeHas(t *testing.T) {
	for _, test := range relativeHasTests {
		s, doc, err := setup(test.selector, test.HTML)
		if err != nil {
			t.Error(err)
			continue
		}
		matches := MatchAll(doc, s)
		if len(matches) != len(test.results) {
			t.Errorf("selector %s on %s: wanted %d elements, got %d", test.selector, test.HTML, len(test.results), len(matches))
			continue
		}
		for i, m := range matches {
			if got := nodeString(m); got != expectedString(test.results[i]) {
				t.Errorf("selector %s: wanted %s, got %s", test.selector, test.results[i], got)
			}
		}
	}
}

func TestRelativeHasSerialization(t *testing.T) {
	for _, input := range []string{"a:has(> img)", "p:has(+ p, ~ div)", "div:has(img)", ":is(a, b)", ":where(a)", ":not(.c)"} {
		sel, err := ParseGroup(input)
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := ParseGroup(sel.String())
		if err != nil {
			t.Fatalf("%s: serialized form %q does not parse: %s", input, sel.String(), err)
		}
		if reparsed.String() != sel.String() {
			t.Errorf("%s: round trip %q -> %q", input, sel.String(), reparsed.String())
		}
	}
}

func TestInvalidPseudoClassArguments(t *testing.T) {
	for _, input := range []string{
		// :has() cannot be nested (Selectors 4 §4.5)
		"div:has(:has(p))",
		// pseudo-elements are not valid inside functional pseudo-classes
		":not(p::after)",
		"div:has(p::before)",
		// a pseudo-element must end the selector
		"div::before p",
		"div::before > p",
	} {
		if _, err := ParseGroup(input); err == nil {
			t.Errorf("%q should be rejected", input)
		}
	}
	// still valid
	for _, input := range []string{"p::before", "div p::after", ":is(a, b) > p", "div:has(p):not(:has(span))"} {
		if _, err := ParseGroup(input); err != nil {
			t.Errorf("%q should parse: %s", input, err)
		}
	}
}

// cascadia 9c17a91: behaviour of :empty is unchanged by the allocation fix.
func TestEmptyIgnoresWhitespaceText(t *testing.T) {
	s, doc, err := setup("p:empty", `<p> </p><p>x</p><p><!-- c --></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(MatchAll(doc, s)); got != 2 {
		t.Errorf("expected 2 empty paragraphs, got %d", got)
	}
}

// :is() and :where() take a forgiving selector list (Selectors 4 §3.5): an
// invalid item is dropped instead of invalidating the selector, and the list
// may be empty. :not() and :has() stay unforgiving.
func TestForgivingSelectorList(t *testing.T) {
	doc := MustParseHTML(`<p class="a">x</p>`)
	p := Query(doc, MustCompile("p"))
	for input, want := range map[string]bool{
		":is(.a, :unknown)":           true,
		":where(:unknown, .a)":        true,
		":is(.a, ::before)":           true,
		":is(.a, p:nth-child(x), .b)": true,
		`:is(.a, "x,y")`:              true,
		":is(.a, :unknown(1, 2))":     true,
		":is()":                       false,
		":where(:unknown)":            false,
		":is(::before)":               false,
		"p:has(:is(:has(p)))":         false, // :has() inside :has() is dropped
		"p:not(:is(:unknown))":        true,
	} {
		sel, err := ParseGroup(input)
		if err != nil {
			t.Errorf("%s: %s", input, err)
			continue
		}
		if got := sel.Match(p); got != want {
			t.Errorf("%s: match %v, want %v", input, got, want)
		}
		// the serialization parses again
		if _, err := ParseGroup(sel.String()); err != nil {
			t.Errorf("%s: serialized as %q: %s", input, sel.String(), err)
		}
	}
	for _, input := range []string{":not(.a, :unknown)", ":is(.a", ":is(.a, (", "p:has(.a, :unknown)"} {
		if _, err := ParseGroup(input); err == nil {
			t.Errorf("%s should be invalid", input)
		}
	}
}

// :lang() uses extended filtering (RFC 4647 §3.3.2): wildcards and skipped
// subtags.
func TestLangExtendedFiltering(t *testing.T) {
	for _, tc := range []struct {
		langRange, lang string
		want            bool
	}{
		{`"*-CH"`, "de-CH", true},
		{`"*-CH"`, "fr-Latn-CH", true},
		{`"*-CH"`, "de-DE", false},
		{"de", "de-CH", true},
		{"de", "de", true},
		{"de", "deu", false},
		{`"de-*-CH"`, "de-Latn-CH", true},
		{"de-CH", "de-Latn-CH", true},
		{"de-CH", "de-x-CH", false},
		{`"*"`, "en", true},
		{"en-US", "en", false},
		{"EN", "en-us", true},
	} {
		doc := MustParseHTML(`<p lang="` + tc.lang + `">x</p>`)
		p := Query(doc, MustCompile("p"))
		if got := MustCompile(":lang(" + tc.langRange + ")").Match(p); got != tc.want {
			t.Errorf(":lang(%s) on %q: %v, want %v", tc.langRange, tc.lang, got, tc.want)
		}
	}
}

// :enabled and :disabled follow HTML §4.16.3.
func TestEnabledDisabled(t *testing.T) {
	doc := MustParseHTML(`
		<a id="link" href="x">l</a>
		<fieldset disabled><fieldset id="inner"><input id="in1"></fieldset>
			<legend><input id="legend1"></legend></fieldset>
		<fieldset disabled><legend>a</legend><legend><input id="legend2"></legend></fieldset>
		<select><optgroup disabled><option id="opt1">a</option></optgroup>
			<optgroup><option id="opt2">b</option></optgroup></select>
		<button id="b1"></button><button id="b2" disabled></button>`)
	enabled, disabled := MustCompile(":enabled"), MustCompile(":disabled")
	for id, want := range map[string]string{
		"link": "neither", "inner": "disabled", "in1": "disabled",
		"opt1": "disabled", "opt2": "enabled", "b1": "enabled", "b2": "disabled",
		// in the first legend of a disabled fieldset, not in a later one
		"legend1": "enabled", "legend2": "disabled",
	} {
		e := Query(doc, MustCompile("#"+id))
		got := "neither"
		switch {
		case enabled.Match(e) && disabled.Match(e):
			got = "both"
		case enabled.Match(e):
			got = "enabled"
		case disabled.Match(e):
			got = "disabled"
		}
		if got != want {
			t.Errorf("#%s: %s, want %s", id, got, want)
		}
	}
}

// Second review: S2-3, S2-4, S2-6, S2-7, S2-8.
func TestSelectorEdges2(t *testing.T) {
	// type selectors are escaped when serialized
	for _, input := range []string{`a\.b`, `\31 23`, `a\ b`, `a\>b`} {
		sel := MustCompile(input)
		again, err := ParseGroup(sel.String())
		if err != nil || again.String() != sel.String() {
			t.Errorf("%s: serialized %q, reparsed %v %v", input, sel.String(), again, err)
		}
	}
	doc := MustParseHTML(`<a class="b">x</a>`)
	if MustCompile(MustCompile(`a\.b`).String()).Match(Query(doc, MustCompile("a"))) {
		t.Error(`a\.b serialized as a.b`)
	}
	// :only-of-type compares namespaces, with or without a cache
	parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	a1 := &html.Node{Type: html.ElementNode, Data: "a", DataAtom: atom.A}
	a2 := &html.Node{Type: html.ElementNode, Data: "a", Namespace: "svg"}
	parent.AppendChild(a1)
	parent.AppendChild(a2)
	if sel := MustCompile(":only-of-type"); !sel.Match(a1) || !MatchWith(sel[0], a1, NewCache()) {
		t.Error(":only-of-type and namespaces")
	}
	// forgiving lists skip stray brackets and comments
	for _, input := range []string{":is(.a], .b)", ":is(! /* ) */, .b)", ":is(.a(], .b)"} {
		if _, err := ParseGroup(input); err != nil && input != ":is(.a(], .b)" {
			t.Errorf("%s: %s", input, err)
		}
	}
	// -- is an identifier; #1 is not an id selector; 1of is a dimension
	if _, err := ParseGroup(".--"); err != nil {
		t.Errorf(".--: %s", err)
	}
	for _, input := range []string{"#1", "#-1", ":nth-child(2n+1of .a)"} {
		if _, err := ParseGroup(input); err == nil {
			t.Errorf("%s should be invalid", input)
		}
	}
	for _, input := range []string{"#a1", "#-a", "#--", ":nth-child(2n+1 of .a)", ":nth-child(n of a)"} {
		if _, err := ParseGroup(input); err != nil {
			t.Errorf("%s: %s", input, err)
		}
	}
}
